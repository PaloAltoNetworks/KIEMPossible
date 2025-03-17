package kube_collection

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Golansami125/kiempossible/pkg/log_parsing"
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

// Collect role bindings, normalize their content, and insert to DB per subject in the binding
func CollectRoleBindings(client *kubernetes.Clientset, db *sql.DB, clusterRoles map[string]*rbacv1.ClusterRole, roles map[string]*rbacv1.Role) error {
	namespaces, _ := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})

	resourceTypes, err := GetResourceTypesAndAPIGroups(client)
	if err != nil {
		return err
	}

	stmt, err := db.Prepare(`
                INSERT INTO permission (entity_name, entity_type, api_group, resource_type, verb, permission_scope, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time, last_used_resource)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON DUPLICATE KEY UPDATE last_used_time = last_used_time AND last_used_resource = last_used_resource
        `)
	if err != nil {
		return err
	}
	defer stmt.Close()
	subresources, err := GetSubresources(client)
	if err != nil {
		return err
	}

	log_parsing.GlobalProgressBar.Start("roles and roleBindings processed")

	// Iterate over roles by namespace, get subjects and role/clusterRole
	for _, namespace := range namespaces.Items {
		rbList, err := client.RbacV1().RoleBindings(namespace.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, rb := range rbList.Items {
			for _, subject := range rb.Subjects {
				entityName := subject.Name
				if subject.Kind == "ServiceAccount" {
					entityName = fmt.Sprintf("%s:%s", namespace.Name, subject.Name)
				}

				var role *rbacv1.Role
				var clusterRole *rbacv1.ClusterRole
				if rb.RoleRef.Kind == "Role" {
					key := fmt.Sprintf("%s/%s", namespace.Name, rb.RoleRef.Name)
					role = roles[key]
				} else if rb.RoleRef.Kind == "ClusterRole" {
					clusterRole = clusterRoles[rb.RoleRef.Name]
				} else {
					fmt.Printf("Unsupported RoleRef kind: %s\n", rb.RoleRef.Kind)
					continue
				}
				// Send to function to handle normalizing the data
				if role != nil {
					processRoleRules(stmt, entityName, subject.Kind, role.Rules, resourceTypes, namespace.Name, role.Name, rb.Name, subresources)
				} else if clusterRole != nil {
					processClusterRoleRules(stmt, entityName, subject.Kind, clusterRole.Rules, resourceTypes, clusterRole.Name, rb.Name, subresources)
				}
			}
			log_parsing.GlobalProgressBar.Add(1)
		}
	}
	log_parsing.GlobalProgressBar.Stop()
	println()
	fmt.Printf("Inserted RoleBinding Permissions!\n")
	return nil
}

// Normalize the data obtained from roles and insert to DB by subject
func processRoleRules(stmt *sql.Stmt, entityName, entityType string, rules []rbacv1.PolicyRule, resourceTypes []ResourceType, namespace, roleName, roleBindingName string, subresources map[string]string) error {
	// Iterate through every rule, flatten wildcard if they exist
	for _, rule := range rules {
		resourceNames := rule.ResourceNames
		for _, apiGroup := range rule.APIGroups {
			for _, resource := range rule.Resources {
				for _, verb := range rule.Verbs {
					resourceTypes, err := FlattenWildcards(resourceTypes, verb, resource, apiGroup)
					if err != nil {
						return err
					}

					for _, resourceType := range resourceTypes {
						// Handle any resourceNames if they exist so the scope is correct in the DB
						if len(resourceNames) > 0 {
							for _, resourceName := range resourceNames {
								scope := fmt.Sprintf("%s/%s", namespace, resourceName)
								for subresource, srapiGroup := range subresources {
									if strings.HasPrefix(subresource, resourceType.ResourceType) && srapiGroup == resourceType.APIGroup && !strings.Contains(resourceType.ResourceType, "/") {
										_, err = stmt.Exec(entityName, entityType, resourceType.APIGroup, subresource, verb, scope, roleName, "Role", roleBindingName, "RoleBinding", nil, nil)
										if err != nil {
											return err
										}
									}
								}
								_, err := stmt.Exec(entityName, entityType, resourceType.APIGroup, resourceType.ResourceType, verb, scope, roleName, "Role", roleBindingName, "RoleBinding", nil, nil)
								if err != nil {
									return err
								}
							}
						} else {
							// Else put the scope as namespace, unless the resource isn't namespaced (like node)
							scope := namespace
							if !resourceType.Namespaced {
								scope = "cluster-wide"
							}
							// Handle subresources and insert
							for subresource, srapiGroup := range subresources {
								if strings.HasPrefix(subresource, resourceType.ResourceType) && srapiGroup == resourceType.APIGroup && !strings.Contains(resourceType.ResourceType, "/") {
									_, err = stmt.Exec(entityName, entityType, resourceType.APIGroup, subresource, verb, scope, roleName, "Role", roleBindingName, "RoleBinding", nil, nil)
									if err != nil {
										return err
									}
								}
							}
							// DB insert
							_, err := stmt.Exec(entityName, entityType, resourceType.APIGroup, resourceType.ResourceType, verb, scope, roleName, "Role", roleBindingName, "RoleBinding", nil, nil)
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// Normalize the data obtained from clusterRoles and insert to DB by subject
func processClusterRoleRules(stmt *sql.Stmt, entityName, entityType string, rules []rbacv1.PolicyRule, resourceTypes []ResourceType, clusterRoleName, roleBindingName string, subresources map[string]string) error {
	// Iterate through every rule, flatten wildcard if they exist
	for _, rule := range rules {
		resourceNames := rule.ResourceNames
		for _, apiGroup := range rule.APIGroups {
			for _, resource := range rule.Resources {
				for _, verb := range rule.Verbs {
					resourceTypes, err := FlattenWildcards(resourceTypes, verb, resource, apiGroup)
					if err != nil {
						return err
					}

					for _, resourceType := range resourceTypes {
						// Handle any resourceNames if they exist so the scope is correct in the DB
						if len(resourceNames) > 0 {
							for _, resourceName := range resourceNames {
								scope := fmt.Sprintf("%s", resourceName)
								for subresource, srapiGroup := range subresources {
									if strings.HasPrefix(subresource, resourceType.ResourceType) && srapiGroup == resourceType.APIGroup && !strings.Contains(resourceType.ResourceType, "/") {
										_, err = stmt.Exec(entityName, entityType, resourceType.APIGroup, subresource, verb, scope, clusterRoleName, "ClusterRole", roleBindingName, "RoleBinding", nil, nil)
										if err != nil {
											return err
										}
									}
								}
								_, err := stmt.Exec(entityName, entityType, resourceType.APIGroup, resourceType.ResourceType, verb, scope, clusterRoleName, "ClusterRole", roleBindingName, "RoleBinding", nil, nil)
								if err != nil {
									return err
								}
							}
						} else {
							// Else put the scope as cluster-wide
							scope := "cluster-wide"
							// Handle subresources and insert
							for subresource, srapiGroup := range subresources {
								if strings.HasPrefix(subresource, resourceType.ResourceType) && srapiGroup == resourceType.APIGroup && !strings.Contains(resourceType.ResourceType, "/") {
									_, err = stmt.Exec(entityName, entityType, resourceType.APIGroup, subresource, verb, scope, clusterRoleName, "ClusterRole", roleBindingName, "RoleBinding", nil, nil)
									if err != nil {
										return err
									}
								}
							}
							// DB Insert
							_, err := stmt.Exec(entityName, entityType, resourceType.APIGroup, resourceType.ResourceType, verb, scope, clusterRoleName, "ClusterRole", roleBindingName, "RoleBinding", nil, nil)
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// Collect clusterRole bindings, normalize their content, and insert to DB per subject in the binding
func CollectClusterRoleBindings(client *kubernetes.Clientset, db *sql.DB, clusterRoles map[string]*rbacv1.ClusterRole) error {
	crbList, err := client.RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	resourceTypes, err := GetResourceTypesAndAPIGroups(client)
	if err != nil {
		return err
	}

	stmt, err := db.Prepare(`
        INSERT INTO permission (entity_name, entity_type, api_group, resource_type, verb, permission_scope, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time, last_used_resource)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON DUPLICATE KEY UPDATE last_used_time = last_used_time AND last_used_resource = last_used_resource
    `)
	if err != nil {
		return err
	}
	defer stmt.Close()
	subresources, _ := GetSubresources(client)

	log_parsing.GlobalProgressBar.Start("clusterRoles and clusterRoleBindings processed")

	// Iterate over clusterRoles, get subjects and clusterRole
	for _, crb := range crbList.Items {
		for _, subject := range crb.Subjects {
			clusterRole, ok := clusterRoles[crb.RoleRef.Name]
			if !ok {
				fmt.Printf("ClusterRole '%s' not found in the clusterRoles map\n", crb.RoleRef.Name)
				continue
			}

			entityName := subject.Name
			if subject.Kind == "ServiceAccount" {
				entityName = fmt.Sprintf("%s:%s", subject.Namespace, subject.Name)
			}
			// Iterate over rules in the clusterRole and flatten wildcards if necessary
			for _, rule := range clusterRole.Rules {
				resourceNames := rule.ResourceNames
				for _, apiGroup := range rule.APIGroups {
					for _, resource := range rule.Resources {
						for _, verb := range rule.Verbs {
							resourceTypes, err := FlattenWildcards(resourceTypes, verb, resource, apiGroup)
							if err != nil {
								return err
							}
							for _, resourceType := range resourceTypes {
								// Handle any resourceNames
								if len(resourceNames) > 0 {
									for _, resourceName := range resourceNames {
										scope := fmt.Sprintf("%s", resourceName)
										for subresource, srapiGroup := range subresources {
											if strings.HasPrefix(subresource, resourceType.ResourceType) && srapiGroup == resourceType.APIGroup && !strings.Contains(resourceType.ResourceType, "/") {
												_, err = stmt.Exec(entityName, subject.Kind, resourceType.APIGroup, subresource, resourceType.Verb, scope, clusterRole.Name, "ClusterRole", crb.Name, "ClusterRoleBinding", nil, nil)
												if err != nil {
													return err
												}
											}
										}
										_, err := stmt.Exec(entityName, subject.Kind, resourceType.APIGroup, resourceType.ResourceType, verb, scope, clusterRole.Name, "ClusterRole", crb.Name, "ClusterRoleBinding", nil, nil)
										if err != nil {
											return err
										}
									}
									// If resource is namespaced, make sure 1 entry per namespace in the cluster rather than 1 generic "wildcard" scope entry
								} else if resourceType.Namespaced {
									for _, namespace := range namespaces.Items {
										for subresource, srapiGroup := range subresources {
											if strings.HasPrefix(subresource, resourceType.ResourceType) && srapiGroup == resourceType.APIGroup && !strings.Contains(resourceType.ResourceType, "/") {
												_, err = stmt.Exec(entityName, subject.Kind, resourceType.APIGroup, subresource, resourceType.Verb, namespace.Name, clusterRole.Name, "ClusterRole", crb.Name, "ClusterRoleBinding", nil, nil)
												if err != nil {
													return err
												}
											}
										}
										_, err = stmt.Exec(entityName, subject.Kind, resourceType.APIGroup, resourceType.ResourceType, resourceType.Verb, namespace.Name, clusterRole.Name, "ClusterRole", crb.Name, "ClusterRoleBinding", nil, nil)
										if err != nil {
											return err
										}

									}
									// Handle subresources if necessary
								} else {
									for subresource, srapiGroup := range subresources {
										if strings.HasPrefix(subresource, resourceType.ResourceType) && srapiGroup == resourceType.APIGroup && !strings.Contains(resourceType.ResourceType, "/") {
											_, err = stmt.Exec(entityName, subject.Kind, resourceType.APIGroup, subresource, resourceType.Verb, "cluster-wide", clusterRole.Name, "ClusterRole", crb.Name, "ClusterRoleBinding", nil, nil)
											if err != nil {
												return err
											}
										}
									}
									// DB Insert
									_, err = stmt.Exec(entityName, subject.Kind, resourceType.APIGroup, resourceType.ResourceType, resourceType.Verb, "cluster-wide", clusterRole.Name, "ClusterRole", crb.Name, "ClusterRoleBinding", nil, nil)
									if err != nil {
										return err
									}
								}
							}
						}
					}
				}
			}
		}
		log_parsing.GlobalProgressBar.Add(1)
	}
	log_parsing.GlobalProgressBar.Stop()
	println()
	fmt.Printf("Inserted ClusterRoleBinding Permissions!\n")
	return nil
}
