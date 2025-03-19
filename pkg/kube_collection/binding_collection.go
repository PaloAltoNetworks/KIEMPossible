package kube_collection

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Golansami125/kiempossible/pkg/log_parsing"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ResourceType struct {
	APIGroup     string
	ResourceType string
	SubResource  string
	Verb         string
	Namespaced   bool
	ResourceName string
}

type PermissionContext struct {
	EntityName   string
	EntityType   string
	ResourceType ResourceType
	Verb         string
	Scope        string
	SourceName   string
	SourceType   string
	BindingName  string
	BindingType  string
}

func preparePermissionStatement(db *sql.DB) (*sql.Stmt, error) {
	return db.Prepare(`
        INSERT INTO permission (
            entity_name, entity_type, api_group, resource_type, 
            verb, permission_scope, permission_source, 
            permission_source_type, permission_binding, 
            permission_binding_type, last_used_time, last_used_resource
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON DUPLICATE KEY UPDATE 
            last_used_time = last_used_time 
            AND last_used_resource = last_used_resource
    `)
}

func prepareResources(client *kubernetes.Clientset) ([]ResourceType, map[string]string, error) {
	resourceTypes, err := GetResourceTypesAndAPIGroups(client)
	if err != nil {
		return nil, nil, err
	}

	subresources, err := GetSubresources(client)
	if err != nil {
		return nil, nil, err
	}

	return resourceTypes, subresources, nil
}

func processSubresource(
	stmt *sql.Stmt,
	ctx PermissionContext,
	verb string,
	scope string,
	subresources map[string]string,
) error {
	for subresource, srapiGroup := range subresources {
		if strings.HasPrefix(subresource, ctx.ResourceType.ResourceType) &&
			srapiGroup == ctx.ResourceType.APIGroup &&
			!strings.Contains(ctx.ResourceType.ResourceType, "/") {

			_, err := stmt.Exec(
				ctx.EntityName, ctx.EntityType, ctx.ResourceType.APIGroup,
				subresource, verb, scope, ctx.SourceName, ctx.SourceType,
				ctx.BindingName, ctx.BindingType, nil, nil,
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func executePermissionStatement(
	stmt *sql.Stmt,
	ctx PermissionContext,
	verb string,
	scope string,
) error {
	_, err := stmt.Exec(
		ctx.EntityName, ctx.EntityType, ctx.ResourceType.APIGroup,
		ctx.ResourceType.ResourceType, verb, scope,
		ctx.SourceName, ctx.SourceType,
		ctx.BindingName, ctx.BindingType,
		nil, nil,
	)
	return err
}

func processResourceType(
	stmt *sql.Stmt,
	ctx PermissionContext,
	resourceType ResourceType,
	verb string,
	resourceNames []string,
	namespace string,
	subresources map[string]string,
) error {
	ctx.ResourceType = resourceType

	if len(resourceNames) > 0 {
		return processWithResourceNames(stmt, ctx, verb, resourceNames, namespace, subresources)
	}
	return processWithoutResourceNames(stmt, ctx, verb, namespace, subresources)
}

func processWithResourceNames(
	stmt *sql.Stmt,
	ctx PermissionContext,
	verb string,
	resourceNames []string,
	namespace string,
	subresources map[string]string,
) error {
	for _, resourceName := range resourceNames {
		scope := resourceName
		if namespace != "" {
			scope = fmt.Sprintf("%s/%s", namespace, resourceName)
		}

		if err := processSubresource(stmt, ctx, verb, scope, subresources); err != nil {
			return err
		}

		if err := executePermissionStatement(stmt, ctx, verb, scope); err != nil {
			return err
		}
	}
	return nil
}

func processWithoutResourceNames(
	stmt *sql.Stmt,
	ctx PermissionContext,
	verb string,
	namespace string,
	subresources map[string]string,
) error {
	scope := "cluster-wide"
	if namespace != "" && ctx.ResourceType.Namespaced {
		scope = namespace
	}

	if err := processSubresource(stmt, ctx, verb, scope, subresources); err != nil {
		return err
	}

	return executePermissionStatement(stmt, ctx, verb, scope)
}

func processRule(
	stmt *sql.Stmt,
	ctx PermissionContext,
	rule rbacv1.PolicyRule,
	resourceTypes []ResourceType,
	namespace string,
	subresources map[string]string,
) error {
	for _, apiGroup := range rule.APIGroups {
		for _, resource := range rule.Resources {
			for _, verb := range rule.Verbs {
				flattenedTypes, err := FlattenWildcards(resourceTypes, verb, resource, apiGroup)
				if err != nil {
					return err
				}

				for _, resourceType := range flattenedTypes {
					if err := processResourceType(
						stmt, ctx, resourceType, verb,
						rule.ResourceNames, namespace, subresources,
					); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func CollectRoleBindings(
	client *kubernetes.Clientset,
	db *sql.DB,
	clusterRoles map[string]*rbacv1.ClusterRole,
	roles map[string]*rbacv1.Role,
) error {
	stmt, err := preparePermissionStatement(db)
	if err != nil {
		return err
	}
	defer stmt.Close()

	resourceTypes, subresources, err := prepareResources(client)
	if err != nil {
		return err
	}

	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	log_parsing.GlobalProgressBar.Start("roles and roleBindings processed")
	defer func() {
		log_parsing.GlobalProgressBar.Stop()
		println()
		fmt.Printf("Inserted RoleBinding Permissions!\n")
	}()

	for _, namespace := range namespaces.Items {
		rbList, err := client.RbacV1().RoleBindings(namespace.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, rb := range rbList.Items {
			if err := processRoleBinding(stmt, rb, namespace.Name, roles, clusterRoles, resourceTypes, subresources); err != nil {
				return err
			}
			log_parsing.GlobalProgressBar.Add(1)
		}
	}

	return nil
}

func processRoleBinding(
	stmt *sql.Stmt,
	rb rbacv1.RoleBinding,
	namespace string,
	roles map[string]*rbacv1.Role,
	clusterRoles map[string]*rbacv1.ClusterRole,
	resourceTypes []ResourceType,
	subresources map[string]string,
) error {
	for _, subject := range rb.Subjects {
		entityName := subject.Name
		if subject.Kind == "ServiceAccount" {
			entityName = fmt.Sprintf("%s:%s", namespace, subject.Name)
		}

		ctx := PermissionContext{
			EntityName:  entityName,
			EntityType:  subject.Kind,
			BindingName: rb.Name,
			BindingType: "RoleBinding",
		}

		if rb.RoleRef.Kind == "Role" {
			key := fmt.Sprintf("%s/%s", namespace, rb.RoleRef.Name)
			if role, exists := roles[key]; exists {
				ctx.SourceName = role.Name
				ctx.SourceType = "Role"
				if err := processRule(stmt, ctx, role.Rules[0], resourceTypes, namespace, subresources); err != nil {
					return err
				}
			}
		} else if rb.RoleRef.Kind == "ClusterRole" {
			if clusterRole, exists := clusterRoles[rb.RoleRef.Name]; exists {
				ctx.SourceName = clusterRole.Name
				ctx.SourceType = "ClusterRole"
				if err := processRule(stmt, ctx, clusterRole.Rules[0], resourceTypes, namespace, subresources); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func CollectClusterRoleBindings(
	client *kubernetes.Clientset,
	db *sql.DB,
	clusterRoles map[string]*rbacv1.ClusterRole,
) error {
	stmt, err := preparePermissionStatement(db)
	if err != nil {
		return err
	}
	defer stmt.Close()

	resourceTypes, subresources, err := prepareResources(client)
	if err != nil {
		return err
	}

	crbList, err := client.RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	log_parsing.GlobalProgressBar.Start("clusterRoles and clusterRoleBindings processed")
	defer func() {
		log_parsing.GlobalProgressBar.Stop()
		println()
		fmt.Printf("Inserted ClusterRoleBinding Permissions!\n")
	}()

	for _, crb := range crbList.Items {
		if err := processClusterRoleBinding(stmt, crb, clusterRoles, resourceTypes, subresources, namespaces.Items); err != nil {
			return err
		}
		log_parsing.GlobalProgressBar.Add(1)
	}

	return nil
}

func processClusterRoleBinding(
	stmt *sql.Stmt,
	crb rbacv1.ClusterRoleBinding,
	clusterRoles map[string]*rbacv1.ClusterRole,
	resourceTypes []ResourceType,
	subresources map[string]string,
	namespaces []v1.Namespace,
) error {
	clusterRole, ok := clusterRoles[crb.RoleRef.Name]
	if !ok {
		fmt.Printf("ClusterRole '%s' not found in the clusterRoles map\n", crb.RoleRef.Name)
		return nil
	}

	for _, subject := range crb.Subjects {
		entityName := subject.Name
		if subject.Kind == "ServiceAccount" {
			entityName = fmt.Sprintf("%s:%s", subject.Namespace, subject.Name)
		}

		ctx := PermissionContext{
			EntityName:  entityName,
			EntityType:  subject.Kind,
			SourceName:  clusterRole.Name,
			SourceType:  "ClusterRole",
			BindingName: crb.Name,
			BindingType: "ClusterRoleBinding",
		}

		for _, rule := range clusterRole.Rules {
			if err := processRule(stmt, ctx, rule, resourceTypes, "", subresources); err != nil {
				return err
			}
		}
	}
	return nil
}
