package log_parsing

import (
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	v1 "k8s.io/api/core/v1"
)

var processedEntities []string

func retryEKSWithBackoff(input *eks.ListAssociatedAccessPoliciesInput) (*eks.ListAssociatedAccessPoliciesOutput, error) {
	var attempt int
	for attempt = 0; attempt < 5; attempt++ {
		eksSvc := eks.New(GetSession())
		result, err := eksSvc.ListAssociatedAccessPolicies(input)
		if err == nil {
			return result, nil
		}
		if IsThrottlingError(err) { // Use the exported version
			sleepTime := time.Duration((1<<attempt)*100+rand.Intn(100)) * time.Millisecond
			fmt.Printf("Throttling detected, retrying in %v...\n", sleepTime)
			time.Sleep(sleepTime)
			continue
		}
		if IsExpiredCredentialsError(err) {
			// Session management as in extract_aws.go
			sessionMutex.Lock()
			if sessionRef == nil {
				sessionCond.Wait()
				sessionMutex.Unlock()
				attempt = 0
				continue
			}
			GlobalProgressBar.Pause()
			sessionRef = nil
			sessionMutex.Unlock()
			if err := Reauthenticate(); err != nil {
				return nil, err
			}
			GlobalProgressBar.Resume()
			attempt = 0
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("max retries exceeded")
}

// accessPolicy flow to handle EKS Access Policy
func handleEKSAccessPolicy(entityName, reason, clusterName string, sess *session.Session, db *sql.DB, namespaces *v1.NamespaceList) {
	// Keep track of processed entities to avoid duplicates
	for _, name := range processedEntities {
		if name == entityName {
			return
		}
	}

	var accessEntryArn string
	var parseSuccess bool

	if strings.Contains(reason, "allowed by ClusterRoleBinding ") {
		parts := strings.Split(reason, "allowed by ClusterRoleBinding ")
		if len(parts) >= 2 {
			binding := strings.Split(parts[1], " of ClusterRole ")
			if len(binding) >= 2 {
				roleBindingParts := strings.Split(binding[0], "+")
				if len(roleBindingParts) >= 2 {
					accessEntryArn = strings.Trim(roleBindingParts[0], "\"")
					parseSuccess = true
				}
			}
		}
	} else if strings.Contains(reason, "allowed by RoleBinding ") {
		parts := strings.Split(reason, "allowed by RoleBinding ")
		if len(parts) >= 2 {
			binding := strings.Split(parts[1], " of Role ")
			if len(binding) >= 2 {
				roleBindingParts := strings.Split(binding[0], "+")
				if len(roleBindingParts) >= 2 {
					accessEntryArn = strings.Trim(roleBindingParts[0], "\"")
					parseSuccess = true
				}
			}
		}
	}

	if !parseSuccess {
		return
	}

	policyNames, accessScopes := listAssociatedAccessPolicies(clusterName, accessEntryArn)
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
func listAssociatedAccessPolicies(clusterName, principalArn string) ([]string, []string) {
	input := &eks.ListAssociatedAccessPoliciesInput{
		ClusterName:  aws.String(clusterName),
		PrincipalArn: aws.String(principalArn),
	}
	result, err := retryEKSWithBackoff(input)
	if err != nil {
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

func isResourceTypeNamespaced(resourceType string, db *sql.DB, namespaces *v1.NamespaceList) (bool, error) {
	namespaceMap := make(map[string]struct{})
	for _, ns := range namespaces.Items {
		namespaceMap[ns.Name] = struct{}{}
	}

	// No namespaces found, so resource cannot be namespaced
	if len(namespaceMap) == 0 {
		return false, nil
	}

	query := `
        SELECT DISTINCT permission_scope
        FROM permission
        WHERE resource_type = ?
    `
	rows, err := db.Query(query, resourceType)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var scope string
		if err := rows.Scan(&scope); err != nil {
			return false, err
		}

		// If a permission's scope is a valid namespace, then the resource is namespaced
		scopeParts := strings.Split(scope, "/")
		if _, exists := namespaceMap[scopeParts[0]]; exists {
			return true, nil
		}
	}
	if err = rows.Err(); err != nil {
		return false, err
	}

	return false, nil
}

func insertExpandedPermission(entityName, apiGroup, resourceType, verb, scope, accessEntryArn, policyName string, db *sql.DB) {
	_, err := db.Exec(`
        INSERT INTO permission (
            entity_name, entity_type, api_group, resource_type, verb, permission_scope,
            permission_source, permission_source_type, permission_binding, permission_binding_type,
            last_used_time, last_used_resource
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, entityName, "User", apiGroup, resourceType, verb, scope, policyName, "EKS Access Policy", accessEntryArn, "EKS Access Entry", nil, nil)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return
		}
		fmt.Println("Error inserting row:", err)
	}
}

// Helper to get subresources for a resourceType
func getSubresources(resourceType, apiGroup string, db *sql.DB) ([]string, error) {
	query := `
		SELECT DISTINCT resource_type
		FROM permission
		WHERE resource_type LIKE ? AND api_group = ? AND resource_type LIKE '%/%'
	`
	rows, err := db.Query(query, resourceType+"/%", apiGroup)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subresources := []string{}
	for rows.Next() {
		var subresource string
		if err := rows.Scan(&subresource); err != nil {
			return nil, err
		}
		subresources = append(subresources, subresource)
	}
	return subresources, nil
}

// Handle permissions from the static adminView policy by getting every permission in the cluster
func handleEKSClusterAdminPolicy(entityName, accessEntryArn, accessScope string, namespaces *v1.NamespaceList, db *sql.DB) {
	if accessScope == "cluster" {
		query := `
			SELECT DISTINCT api_group, resource_type, verb
			FROM permission
		`
		rows, err := db.Query(query)
		if err != nil {
			fmt.Println("Error executing query:", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var apiGroup, resourceType, verb string
			if err := rows.Scan(&apiGroup, &resourceType, &verb); err != nil {
				fmt.Println("Error scanning row:", err)
				continue
			}

			isNamespaced, err := isResourceTypeNamespaced(resourceType, db, namespaces)
			if err != nil {
				fmt.Printf("Error checking if resource %s is namespaced: %v\n", resourceType, err)
				continue
			}

			policyName := "AmazonEKSClusterAdminPolicy"
			if isNamespaced {
				for _, ns := range namespaces.Items {
					insertExpandedPermission(entityName, apiGroup, resourceType, verb, ns.Name, accessEntryArn, policyName, db)
					// Insert subresources
					subresources, err := getSubresources(resourceType, apiGroup, db)
					if err == nil {
						for _, subresource := range subresources {
							insertExpandedPermission(entityName, apiGroup, subresource, verb, ns.Name, accessEntryArn, policyName, db)
						}
					}
				}
			} else {
				insertExpandedPermission(entityName, apiGroup, resourceType, verb, "cluster-wide", accessEntryArn, policyName, db)
				// Insert subresources
				subresources, err := getSubresources(resourceType, apiGroup, db)
				if err == nil {
					for _, subresource := range subresources {
						insertExpandedPermission(entityName, apiGroup, subresource, verb, "cluster-wide", accessEntryArn, policyName, db)
					}
				}
			}
		}
		if err = rows.Err(); err != nil {
			fmt.Println("Error iterating rows:", err)
		}
	} else {
		targetNamespaces := strings.Split(accessScope, ",")
		for _, ns := range targetNamespaces {
			query := `
				SELECT api_group, resource_type, verb, permission_scope
				FROM permission
				WHERE permission_scope = ? OR permission_scope LIKE ?
				GROUP BY api_group, resource_type, verb, permission_scope
			`

			rows, err := db.Query(query, ns, ns+"/%")
			if err != nil {
				fmt.Println("Error executing query:", err)
				continue
			}
			defer rows.Close()

			for rows.Next() {
				var apiGroup, resourceType, verb, permissionScope string
				err = rows.Scan(&apiGroup, &resourceType, &verb, &permissionScope)
				if err != nil {
					fmt.Println("Error scanning row:", err)
					continue
				}
				policyName := "AmazonEKSClusterAdminPolicy"
				insertExpandedPermission(entityName, apiGroup, resourceType, verb, permissionScope, accessEntryArn, policyName, db)
				// Insert subresources
				subresources, err := getSubresources(resourceType, apiGroup, db)
				if err == nil {
					for _, subresource := range subresources {
						insertExpandedPermission(entityName, apiGroup, subresource, verb, permissionScope, accessEntryArn, policyName, db)
					}
				}
			}

			if err = rows.Err(); err != nil {
				fmt.Println("Error iterating rows:", err)
			}
		}
	}
}

// Handle permissions from the static adminView policy by getting every 'view' permission in the cluster
func handleEKSAdminViewPolicy(entityName, accessEntryArn, accessScope string, namespaces *v1.NamespaceList, db *sql.DB) {
	if accessScope == "cluster" {
		query := `
			SELECT DISTINCT api_group, resource_type, verb
			FROM permission
			WHERE verb IN ('get', 'list', 'watch')
		`
		rows, err := db.Query(query)
		if err != nil {
			fmt.Println("Error executing query:", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var apiGroup, resourceType, verb string
			if err := rows.Scan(&apiGroup, &resourceType, &verb); err != nil {
				fmt.Println("Error scanning row:", err)
				continue
			}

			isNamespaced, err := isResourceTypeNamespaced(resourceType, db, namespaces)
			if err != nil {
				fmt.Printf("Error checking if resource %s is namespaced: %v\n", resourceType, err)
				continue
			}

			policyName := "AmazonEKSAdminViewPolicy"
			if isNamespaced {
				for _, ns := range namespaces.Items {
					insertExpandedPermission(entityName, apiGroup, resourceType, verb, ns.Name, accessEntryArn, policyName, db)
					// Insert subresources
					subresources, err := getSubresources(resourceType, apiGroup, db)
					if err == nil {
						for _, subresource := range subresources {
							insertExpandedPermission(entityName, apiGroup, subresource, verb, ns.Name, accessEntryArn, policyName, db)
						}
					}
				}
			} else {
				insertExpandedPermission(entityName, apiGroup, resourceType, verb, "cluster-wide", accessEntryArn, policyName, db)
				// Insert subresources
				subresources, err := getSubresources(resourceType, apiGroup, db)
				if err == nil {
					for _, subresource := range subresources {
						insertExpandedPermission(entityName, apiGroup, subresource, verb, "cluster-wide", accessEntryArn, policyName, db)
					}
				}
			}
		}
		if err = rows.Err(); err != nil {
			fmt.Println("Error iterating rows:", err)
		}
	} else {
		targetNamespaces := strings.Split(accessScope, ",")
		for _, ns := range targetNamespaces {
			query := `
				SELECT api_group, resource_type, verb, permission_scope
				FROM permission
				WHERE (permission_scope = ? OR permission_scope LIKE ?) AND verb IN ('get', 'list', 'watch')
				GROUP BY api_group, resource_type, verb, permission_scope
			`

			rows, err := db.Query(query, ns, ns+"/%")
			if err != nil {
				fmt.Println("Error executing query:", err)
				continue
			}
			defer rows.Close()

			for rows.Next() {
				var apiGroup, resourceType, verb, permissionScope string
				err = rows.Scan(&apiGroup, &resourceType, &verb, &permissionScope)
				if err != nil {
					fmt.Println("Error scanning row:", err)
					continue
				}
				policyName := "AmazonEKSAdminViewPolicy"
				insertExpandedPermission(entityName, apiGroup, resourceType, verb, permissionScope, accessEntryArn, policyName, db)
				// Insert subresources
				subresources, err := getSubresources(resourceType, apiGroup, db)
				if err == nil {
					for _, subresource := range subresources {
						insertExpandedPermission(entityName, apiGroup, subresource, verb, permissionScope, accessEntryArn, policyName, db)
					}
				}
			}

			if err = rows.Err(); err != nil {
				fmt.Println("Error iterating rows:", err)
			}
		}
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

		isSubresource := strings.Contains(resourceType, "/")

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
						if strings.Contains(err.Error(), "Duplicate entry") {
							continue
						}
						fmt.Println("Error inserting row:", err)
						continue
					}
					// If not already a subresource, insert for all subresources
					if !isSubresource {
						subresources, err := getSubresources(resourceType, apiGroup, db)
						if err == nil {
							for _, subresource := range subresources {
								_, err := db.Exec(`
	                        INSERT INTO permission (
	                            entity_name, entity_type, api_group, resource_type, verb, permission_scope,
	                            permission_source, permission_source_type, permission_binding, permission_binding_type,
	                            last_used_time, last_used_resource
	                        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	                    `, entityName, "User", apiGroup, subresource, verb, nsName, "AmazonEKSEditPolicy", "EKS Access Policy", accessEntryArn, "EKS Access Entry", nil, nil)
								if err != nil {
									if strings.Contains(err.Error(), "Duplicate entry") {
										continue
									}
									fmt.Println("Error inserting subresource row:", err)
									continue
								}
							}
						}
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
					if strings.Contains(err.Error(), "Duplicate entry") {
						continue
					}
					fmt.Println("Error inserting row:", err)
					continue
				}
				// If not already a subresource, insert for all subresources
				if !isSubresource {
					subresources, err := getSubresources(resourceType, apiGroup, db)
					if err == nil {
						for _, subresource := range subresources {
							_, err := db.Exec(`
                        INSERT INTO permission (
                            entity_name, entity_type, api_group, resource_type, verb, permission_scope,
                            permission_source, permission_source_type, permission_binding, permission_binding_type,
                            last_used_time, last_used_resource
                        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    `, entityName, "User", apiGroup, subresource, verb, accessScope, "AmazonEKSEditPolicy", "EKS Access Policy", accessEntryArn, "EKS Access Entry", nil, nil)
							if err != nil {
								if strings.Contains(err.Error(), "Duplicate entry") {
									continue
								}
								fmt.Println("Error inserting subresource row:", err)
								continue
							}
						}
					}
				}
			}
		}
	}
}
