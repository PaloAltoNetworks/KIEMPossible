package kube_collection

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

func GetResourceTypesAndAPIGroups(client *kubernetes.Clientset) ([]ResourceType, error) {
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

func FlattenWildcards(resourceTypes []ResourceType, verb, resource, apiGroup string) ([]ResourceType, error) {
	var flattenedResourceTypes []ResourceType

	if verb == "*" && resource == "*" {
		// Return all possible combinations of verbs and resource types
		for _, rt := range resourceTypes {
			verbs, err := GetVerbsForResourceType(rt.ResourceType)
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
			if rt.ResourceType == resource {
				// Get the list of verbs for this resource type
				verbs, err := GetVerbsForResourceType(rt.ResourceType)
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
			if ContainsVerb(rt.ResourceType, verb) {
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
		for _, rt := range resourceTypes {
			if rt.ResourceType == parentResource {
				resourceType := ResourceType{
					APIGroup:     rt.APIGroup,
					ResourceType: parentResource,
					Verb:         verb,
					Namespaced:   rt.Namespaced,
				}
				if subResource != "" {
					resourceType.ResourceType = parentResource + "/" + subResource
				}
				flattenedResourceTypes = append(flattenedResourceTypes, resourceType)
			}
		}
	}
	return flattenedResourceTypes, nil
}

func GetVerbsForResourceType(resourceType string) ([]string, error) {
	// Define a map of resource types and their corresponding verbs
	resourceVerbsMap := map[string][]string{
		"certificatesigningrequests": {"approve", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"roles":                      {"bind", "escalate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"clusterroles":               {"bind", "escalate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"serviceaccounts":            {"impersonate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"users":                      {"impersonate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"groups":                     {"impersonate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
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

func ContainsVerb(resourceType, verb string) bool {
	// Define a map of standard verbs that apply to every resource
	standardVerbs := map[string]bool{
		"create":           true,
		"delete":           true,
		"deletecollection": true,
		"get":              true,
		"list":             true,
		"patch":            true,
		"update":           true,
		"watch":            true,
	}

	// Define a map of resource-specific verbs
	resourceVerbMap := map[string][]string{
		"certificatesigningrequests": {"approve"},
		"roles":                      {"bind", "escalate"},
		"clusterroles":               {"bind", "escalate"},
		"serviceaccounts":            {"impersonate"},
		"users":                      {"impersonate"},
		"groups":                     {"impersonate"},
		// Add more resource types and their associated verbs as needed
	}

	// Check if the verb is a standard verb
	if _, ok := standardVerbs[verb]; ok {
		return true
	}

	// Check if the verb is a resource-specific verb
	verbs, ok := resourceVerbMap[resourceType]
	if !ok {
		// If the resource type is not found in the map, assume no resource-specific verbs are applicable
		return false
	}

	// Check if the verb is present in the list of resource-specific verbs for the resource type
	for _, v := range verbs {
		if v == verb {
			return true
		}
	}
	return false
}
