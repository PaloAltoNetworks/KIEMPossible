package main

import (
	"fmt"

	"github.com/Golansami125/clusterlogo/pkg/kube_collection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	// Create Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// Create  maps to store resources
	roles := make(map[string]interface{})
	clusterRoles := make(map[string]interface{})
	roleBindings := make(map[string]interface{})
	clusterRoleBindings := make(map[string]interface{})
	pods := make(map[string]string)
	namespaces := make(map[string]interface{})
	podAttachedSAs := make(map[string]string)
	serviceAccounts := make(map[string]interface{})
	issues := make(map[string]string)

	// Collect resources
	err = kube_collection.CollectRoles(clientset, &roles)
	if err != nil {
		panic(err.Error())
	}

	err = kube_collection.CollectClusterRoles(clientset, &clusterRoles)
	if err != nil {
		panic(err.Error())
	}

	err = kube_collection.CollectRoleBindings(clientset, &roleBindings)
	if err != nil {
		panic(err.Error())
	}

	err = kube_collection.CollectClusterRoleBindings(clientset, &clusterRoleBindings)
	if err != nil {
		panic(err.Error())
	}

	err = kube_collection.CollectPods(clientset, &pods, &namespaces, &podAttachedSAs, &issues)
	if err != nil {
		panic(err.Error())
	}

	err = kube_collection.CollectServiceAccounts(clientset, &serviceAccounts)
	if err != nil {
		panic(err.Error())
	}

	kube_collection.FindUnusedServiceAccounts(&serviceAccounts, &podAttachedSAs, &issues)

	// Print the collected resources
	for name, role := range roles {
		fmt.Printf("Role: %s, Value: %v\n", name, role)
	}
	for name, clusterRole := range clusterRoles {
		fmt.Printf("Cluster Role: %s, Value: %v\n", name, clusterRole)
	}
	for name, binding := range roleBindings {
		fmt.Printf("Role Binding: %s, Value: %v\n", name, binding)
	}
	for name, binding := range clusterRoleBindings {
		fmt.Printf("Cluster Role Binding: %s, Value: %v\n", name, binding)
	}
	for key, value := range pods {
		fmt.Printf("Pod: %s, Value: %s\n", key, value)
	}
	for key, value := range namespaces {
		fmt.Printf("Namespace: %s, Value: %v\n", key, value)
	}
	for key, value := range podAttachedSAs {
		fmt.Printf("Pod: %s, Attached Service Account: %s\n", key, value)
	}
	for key, sa := range serviceAccounts {
		fmt.Printf("Service Account: %s, Value: %v\n", key, sa)
	}
	for key, value := range issues {
		fmt.Printf("The following issues were found:\n%s: %s\n", key, value)
	}

}
