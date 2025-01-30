package kube_collection

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// Get the resource types in the cluster, their API groups and whether or not they're namespaced
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

// Logic to flatten out wildcards into the smallest possible permission subset (instead of 1 line for *, have 1 line X [individual permissions granted by wildcard])
func FlattenWildcards(resourceTypes []ResourceType, verb, resource, apiGroup string) ([]ResourceType, error) {
	var flattenedResourceTypes []ResourceType
	// Handle full wildcard in verb and resource - get all resources and the verbs that can be performed on them
	if verb == "*" && resource == "*" {
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
		// Handle wildcard in verb - get all verbs for resources in the resource field
	} else if verb == "*" {
		for _, rt := range resourceTypes {
			if rt.ResourceType == resource {
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
		// Handle wildcard in resource - get all resources which the verb can be performed on
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
		// Handle the leftover cases including subresources
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

// Get verbs for each resource type - fallback to "generic" verbs for unmapped resources
func GetVerbsForResourceType(resourceType string) ([]string, error) {
	resourceVerbsMap := map[string][]string{
		"certificatesigningrequests": {"approve", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"roles":                      {"bind", "escalate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"clusterroles":               {"bind", "escalate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"serviceaccounts":            {"impersonate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"users":                      {"impersonate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		"groups":                     {"impersonate", "create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		// To add more resource types and their verbs
	}

	genericVerbs := []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"}

	if verbs, ok := resourceVerbsMap[resourceType]; ok {
		return verbs, nil
	}

	return genericVerbs, nil
}

func ContainsVerb(resourceType, verb string) bool {
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

	resourceVerbMap := map[string][]string{
		"certificatesigningrequests": {"approve"},
		"roles":                      {"bind", "escalate"},
		"clusterroles":               {"bind", "escalate"},
		"serviceaccounts":            {"impersonate"},
		"users":                      {"impersonate"},
		"groups":                     {"impersonate"},
		// Add more resource types and their verbs
	}

	if _, ok := standardVerbs[verb]; ok {
		return true
	}

	verbs, ok := resourceVerbMap[resourceType]
	if !ok {
		return false
	}

	for _, v := range verbs {
		if v == verb {
			return true
		}
	}
	return false
}

// Get all the subresources for a given resources
func GetSubresources(client *kubernetes.Clientset) (map[string]string, error) {
	_, apiResourceLists, err := client.Discovery().ServerGroupsAndResources()
	if err != nil {
		return nil, err
	}

	resources := make(map[string]string)
	for _, apiResourceList := range apiResourceLists {
		groupVersion, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
		if err != nil {
			return nil, err
		}

		groupVersionString := fmt.Sprintf("%s/%s", groupVersion.Group, groupVersion.Version)
		if groupVersion.Group == "" {
			groupVersionString = "v1"
		}

		for _, apiResource := range apiResourceList.APIResources {
			if strings.Contains(apiResource.Name, "/") {
				resources[apiResource.Name] = groupVersionString
			}
		}
	}

	return resources, nil
}
