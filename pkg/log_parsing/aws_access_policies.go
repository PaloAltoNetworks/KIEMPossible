package log_parsing

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	v1 "k8s.io/api/core/v1"
)

var processedEntities []string

// accessPolicy flow to handle EKS Access Policy
func handleEKSAccessPolicy(entityName, reason, clusterName string, sess *session.Session, db *sql.DB, namespaces *v1.NamespaceList) {
	// Keep track of processed entities to avoid duplicates
	for _, name := range processedEntities {
		if name == entityName {
			return
		}
	}
	parts := strings.Split(reason, "allowed by ClusterRoleBinding ")
	if len(parts) < 2 {
		fmt.Println("Invalid reason format")
		return
	}
	binding := strings.Split(parts[1], " of ClusterRole ")
	if len(binding) < 2 {
		fmt.Println("Invalid reason format")
		return
	}
	roleBindingParts := strings.Split(binding[0], "+")
	if len(roleBindingParts) < 2 {
		fmt.Println("Invalid reason format")
		return
	}
	accessEntryArn := strings.Trim(roleBindingParts[0], "\"")
	policyNames, accessScopes := listAssociatedAccessPolicies(clusterName, accessEntryArn, sess)
	for i, policyName := range policyNames {
		if policyName == "AmazonEKSClusterAdminPolicy" {
			handleEKSClusterAdminPolicy(entityName, accessEntryArn, accessScopes[i], namespaces, db)
			continue
		}
		if policyName == "AmazonEKSAdminViewPolicy" {
			handleEKSAdminViewPolicy(entityName, accessEntryArn, accessScopes[i], namespaces, db)
			continue
		}
		if policyName == "AmazonEKSEditPolicy" {
			handleStaticPolicy(entityName, accessEntryArn, accessScopes[i], eksEditPolicyPermissions, namespaces, db)
			continue
		}
		if policyName == "AmazonEKSViewPolicy" {
			handleStaticPolicy(entityName, accessEntryArn, accessScopes[i], eksViewPolicyPermissions, namespaces, db)
			continue
		}
		if policyName == "AmazonEKSAdminPolicy" {
			handleStaticPolicy(entityName, accessEntryArn, accessScopes[i], eksAdminPolicyPermissions, namespaces, db)
			continue
		}
	}
	processedEntities = append(processedEntities, entityName)
}

// List the access policies associated with the entry
func listAssociatedAccessPolicies(clusterName, principalArn string, sess *session.Session) ([]string, []string) {
	eksSvc := eks.New(sess)
	input := &eks.ListAssociatedAccessPoliciesInput{
		ClusterName:  aws.String(clusterName),
		PrincipalArn: aws.String(principalArn),
	}
	result, err := eksSvc.ListAssociatedAccessPolicies(input)
	if err != nil {
		fmt.Println("Error listing associated access policies:", err)
		return nil, nil
	}

	policyNames := make([]string, 0, len(result.AssociatedAccessPolicies))
	accessScopes := make([]string, 0, len(result.AssociatedAccessPolicies))

	for _, policy := range result.AssociatedAccessPolicies {
		policyName := extractPolicyName(*policy.PolicyArn)
		policyNames = append(policyNames, policyName)

		accessScope := getAccessScope(policy.AccessScope)
		accessScopes = append(accessScopes, accessScope)
	}

	return policyNames, accessScopes
}

// Get the policy name from the accessEntry
func extractPolicyName(policyArn string) string {
	parts := strings.Split(policyArn, "/")
	return parts[len(parts)-1]
}

// Get the scope from the accessEntry
func getAccessScope(accessScope *eks.AccessScope) string {
	if *accessScope.Type == "cluster" {
		return "cluster"
	}

	namespaces := make([]string, 0, len(accessScope.Namespaces))
	for _, ns := range accessScope.Namespaces {
		namespaces = append(namespaces, *ns)
	}

	return strings.Join(namespaces, ",")
}

// Handle permissions from the static adminView policy by getting every permission in the cluster
func handleEKSClusterAdminPolicy(entityName, accessEntryArn, accessScope string, namespaces *v1.NamespaceList, db *sql.DB) {
	query := `
        SELECT api_group, resource_type, permission_scope, verb
        FROM permission
        GROUP BY api_group, resource_type, permission_scope, verb
    `

	rows, err := db.Query(query)
	if err != nil {
		fmt.Println("Error executing query:", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var apiGroup, resourceType, permissionScope, verb string
		err = rows.Scan(&apiGroup, &resourceType, &permissionScope, &verb)
		if err != nil {
			fmt.Println("Error scanning row:", err)
			continue
		}

		isClusterWide := permissionScope == "cluster-wide"
		matchesAccessScope := permissionScope == accessScope
		if accessScope == "cluster" {
			matchesNamespace := false
			for _, ns := range namespaces.Items {
				if permissionScope == ns.Name {
					matchesNamespace = true
					break
				}
			}
			if !matchesNamespace && !isClusterWide {
				continue
			}
		} else if !matchesAccessScope {
			continue
		}

		_, err = db.Exec(`
            INSERT INTO permission (
                entity_name, entity_type, api_group, resource_type, verb, permission_scope,
                permission_source, permission_source_type, permission_binding, permission_binding_type,
                last_used_time, last_used_resource
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        `, entityName, "User", apiGroup, resourceType, verb, permissionScope, "AmazonEKSClusterAdminPolicy", "EKS Access Policy", accessEntryArn, "EKS Access Entry", nil, nil)
		if err != nil {
			fmt.Println("Error inserting row:", err)
			continue
		}
	}

	if err = rows.Err(); err != nil {
		fmt.Println("Error iterating rows:", err)
	}
}

// Handle permissions from the static adminView policy by getting every 'view' permission in the cluster
func handleEKSAdminViewPolicy(entityName, accessEntryArn, accessScope string, namespaces *v1.NamespaceList, db *sql.DB) {
	query := `
        SELECT api_group, resource_type, permission_scope, verb
        FROM permission
		WHERE verb in ('get', 'list', 'watch')
        GROUP BY api_group, resource_type, permission_scope, verb
    `

	rows, err := db.Query(query)
	if err != nil {
		fmt.Println("Error executing query:", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var apiGroup, resourceType, permissionScope, verb string
		err = rows.Scan(&apiGroup, &resourceType, &permissionScope, &verb)
		if err != nil {
			fmt.Println("Error scanning row:", err)
			continue
		}
		isClusterWide := permissionScope == "cluster-wide"
		matchesAccessScope := permissionScope == accessScope
		if accessScope == "cluster" {
			matchesNamespace := false
			for _, ns := range namespaces.Items {
				if permissionScope == ns.Name {
					matchesNamespace = true
					break
				}
			}
			if !matchesNamespace && !isClusterWide {
				continue
			}
		} else if !matchesAccessScope {
			continue
		}

		_, err = db.Exec(`
            INSERT INTO permission (
                entity_name, entity_type, api_group, resource_type, verb, permission_scope,
                permission_source, permission_source_type, permission_binding, permission_binding_type,
                last_used_time, last_used_resource
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        `, entityName, "User", apiGroup, resourceType, verb, permissionScope, "AmazonEKSClusterAdminPolicy", "EKS Access Policy", accessEntryArn, "EKS Access Entry", nil, nil)
		if err != nil {
			fmt.Println("Error inserting row:", err)
			continue
		}
	}

	if err = rows.Err(); err != nil {
		fmt.Println("Error iterating rows:", err)
	}
}

// Handle permissions from the static policies
func handleStaticPolicy(entityName, accessEntryArn, accessScope string, eksEditPolicyPermissions []string, namespaces *v1.NamespaceList, db *sql.DB) {
	for _, permission := range eksEditPolicyPermissions {
		parts := strings.Split(permission, ":")
		if len(parts) != 3 {
			fmt.Println("Invalid permission format:", permission)
			continue
		}
		apiGroup := parts[0]
		resourceType := parts[1]
		verbs := strings.Split(parts[2], ",")

		if accessScope == "cluster" {
			for _, ns := range namespaces.Items {
				nsName := ns.Name
				// No need to handle cluster-wide scope from DB for now cause the static policies only have namespaced resources
				for _, verb := range verbs {
					_, err := db.Exec(`
                        INSERT INTO permission (
                            entity_name, entity_type, api_group, resource_type, verb, permission_scope,
                            permission_source, permission_source_type, permission_binding, permission_binding_type,
                            last_used_time, last_used_resource
                        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    `, entityName, "User", apiGroup, resourceType, verb, nsName, "AmazonEKSEditPolicy", "EKS Access Policy", accessEntryArn, "EKS Access Entry", nil, nil)
					if err != nil {
						fmt.Println("Error inserting row:", err)
						continue
					}
				}
			}
		} else {
			for _, verb := range verbs {
				_, err := db.Exec(`
                    INSERT INTO permission (
                        entity_name, entity_type, api_group, resource_type, verb, permission_scope,
                        permission_source, permission_source_type, permission_binding, permission_binding_type,
                        last_used_time, last_used_resource
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                `, entityName, "User", apiGroup, resourceType, verb, accessScope, "AmazonEKSEditPolicy", "EKS Access Policy", accessEntryArn, "EKS Access Entry", nil, nil)
				if err != nil {
					fmt.Println("Error inserting row:", err)
					continue
				}
			}
		}
	}
}
