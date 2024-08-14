package kube_collection

import (
	"context"
	"encoding/json"
	"strings"

	"database/sql"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	rbacv1 "k8s.io/api/rbac/v1"
)

type ResourceType struct {
	APIGroup     string
	ResourceType string
	SubResource  string
	Verb         string
	Namespaced   bool
}

func CollectRoleBindings(client *kubernetes.Clientset, roleBindings *map[string]interface{}) error {
	rbList, err := client.RbacV1().RoleBindings("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, rb := range rbList.Items {
		rbJSON, err := json.Marshal(rb)
		if err != nil {
			return err
		}
		var jsonValue interface{}
		err = json.Unmarshal(rbJSON, &jsonValue)
		if err != nil {
			return err
		}
		(*roleBindings)[rb.Name] = jsonValue
	}

	return nil
}

func CollectClusterRoleBindings(client *kubernetes.Clientset, db *sql.DB, clusterRoles map[string]*rbacv1.ClusterRole) error {
	crbList, err := client.RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Get a list of all namespaces in the cluster
	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Get a list of all resource types and their API groups
	resourceTypes, err := getResourceTypesAndAPIGroups(client)
	if err != nil {
		return err
	}

	// Prepare the SQL statement to insert permissions
	stmt, err := db.Prepare(`
        INSERT INTO permission (entity_name, entity_type, api_group, resource_type, verb, permission_scope, last_used_time, last_used_resource)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON DUPLICATE KEY UPDATE last_used_time = last_used_time
    `)
	if err != nil {
		return err
	}
	defer stmt.Close()

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

			for _, rule := range clusterRole.Rules {
				for _, apiGroup := range rule.APIGroups {
					for _, resource := range rule.Resources {
						for _, verb := range rule.Verbs {
							resourceTypes, err := flattenWildcards(resourceTypes, verb, resource, apiGroup)
							if err != nil {
								return err
							}
							for _, resourceType := range resourceTypes {

								if resourceType.Namespaced {
									for _, namespace := range namespaces.Items {
										_, err = stmt.Exec(entityName, subject.Kind, resourceType.APIGroup, resourceType.ResourceType, resourceType.Verb, namespace.Name, nil, nil)
										if err != nil {
											return err
										}

										fmt.Printf("Inserted permission for %s/%s: %s %s/%s in namespace %s\n", subject.Kind, entityName, resourceType.Verb, resourceType.APIGroup, resourceType.ResourceType, namespace.Name)
									}
								} else {

									_, err = stmt.Exec(entityName, subject.Kind, resourceType.APIGroup, resourceType.ResourceType, resourceType.Verb, "cluster-wide", nil, nil)
									if err != nil {
										return err
									}

									//fmt.Printf("Inserted permission for %s/%s: %s %s/%s in cluster-wide scope\n", subject.Kind, entityName, resourceType.Verb, resourceType.APIGroup, resourceType.ResourceType)
								}
							}
						}
					}
				}
			}
		}
	}
	fmt.Printf("Inserted Permissions to database")
	return nil
}

func getResourceTypesAndAPIGroups(client *kubernetes.Clientset) ([]ResourceType, error) {
	resourceTypes := []ResourceType{}

	apiResourceList, err := client.Discovery().ServerPreferredResources()
	if err != nil {
		return nil, err
	}

	for _, apiResourceGroup := range apiResourceList {
		groupVersion, err := schema.ParseGroupVersion(apiResourceGroup.GroupVersion)
		if err != nil {
			return nil, err
		}

		for _, apiResource := range apiResourceGroup.APIResources {
			apiGroup := groupVersion.Group
			if apiGroup == "" {
				apiGroup = "v1"
			} else {
				apiGroup = fmt.Sprintf("%s/%s", apiGroup, groupVersion.Version)
			}

			resourceTypes = append(resourceTypes, ResourceType{
				APIGroup:     apiGroup,
				ResourceType: apiResource.Name,
				Namespaced:   apiResource.Namespaced,
			})

		}
	}

	return resourceTypes, nil
}

func flattenWildcards(resourceTypes []ResourceType, verb, resource, apiGroup string) ([]ResourceType, error) {
	var flattenedResourceTypes []ResourceType

	if verb == "*" && resource == "*" {
		// Return all possible combinations of verbs and resource types
		for _, rt := range resourceTypes {
			verbs, err := getVerbsForResourceType(rt.ResourceType)
			if err != nil {
				return nil, err
			}

			for _, v := range verbs {
				flattenedResourceTypes = append(flattenedResourceTypes, ResourceType{
					APIGroup:     rt.APIGroup,
					ResourceType: rt.ResourceType,
					Verb:         v,
					Namespaced:   rt.Namespaced,
				})
			}
		}
	} else if verb == "*" {
		// Flatten the verb to all possible verbs for the given resource type
		for _, rt := range resourceTypes {
			if (rt.ResourceType == resource || resource == "*") && (resource == "*") {
				// Get the list of verbs for this resource type
				verbs, err := getVerbsForResourceType(rt.ResourceType)
				if err != nil {
					return nil, err
				}

				for _, v := range verbs {
					flattenedResourceTypes = append(flattenedResourceTypes, ResourceType{
						APIGroup:     rt.APIGroup,
						ResourceType: rt.ResourceType,
						Verb:         v,
						Namespaced:   rt.Namespaced,
					})
				}
			}
		}
	} else if resource == "*" {
		for _, rt := range resourceTypes {
			if rt.APIGroup == apiGroup || apiGroup == "" || apiGroup == "*" {
				flattenedResourceTypes = append(flattenedResourceTypes, ResourceType{
					APIGroup:     rt.APIGroup,
					ResourceType: rt.ResourceType,
					Verb:         verb,
					Namespaced:   rt.Namespaced,
				})
			}
		}
	} else {
		resourceParts := strings.Split(resource, "/")
		parentResource := resourceParts[0]
		subResource := ""
		if len(resourceParts) > 1 {
			subResource = resourceParts[1]
		}

		if apiGroup == "" || apiGroup == "*" {
			for _, rt := range resourceTypes {
				if subResource != "" {
					parentResource = parentResource + "/" + subResource
					flattenedResourceTypes = append(flattenedResourceTypes, ResourceType{
						APIGroup:     rt.APIGroup,
						ResourceType: parentResource,
						Verb:         verb,
						Namespaced:   rt.Namespaced,
					})
					break
				}
			}
		} else {
			for _, rt := range resourceTypes {
				if rt.APIGroup == apiGroup && rt.ResourceType == parentResource {
					if subResource != "" {
						parentResource = parentResource + "/" + subResource
						flattenedResourceTypes = append(flattenedResourceTypes, ResourceType{
							APIGroup:     rt.APIGroup,
							ResourceType: parentResource,
							Verb:         verb,
							Namespaced:   rt.Namespaced,
						})
						break
					}
				}
			}
		}
	}
	return flattenedResourceTypes, nil
}

func getVerbsForResourceType(resourceType string) ([]string, error) {
	// Define a map of resource types and their corresponding verbs
	resourceVerbsMap := map[string][]string{
		"certificatesigningrequests": {"approve", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"signers":                    {"sign", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"roles":                      {"bind", "escalate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"clusterroles":               {"bind", "escalate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"serviceaccounts":            {"impersonate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		// Add more resource types and their verbs here
	}

	// Define a list of generic verbs
	genericVerbs := []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"}

	// Check if the resource type is in the map
	if verbs, ok := resourceVerbsMap[resourceType]; ok {
		return verbs, nil
	}

	// If the resource type is not in the map, return the generic verbs
	return genericVerbs, nil
}
