/* Existing tests
- Unused Service Accounts
- Use of Default Service Account
*/

/*
	Logic to add

1. Check API server logs to see if I can figure out when actions were last done by different entities (potentially even cloud provider logs?)
2. Check for pods with bind/escalate/impersonate - dangerous verbs
3. Check for privileged containers, host mounts, different breakout techniques etc
4. Add project ID for GCP, subscription for Azure
5. Add log parsing for AWS
6. Add neo4j logic

!!!??? Maybe map permissions as nodes and users/SAs as attributes with a "last used" date
*/

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Golansami125/clusterlogo/pkg/kube_collection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// Kube Logic ------------- move out of main once neo4j logic done
	// Create Kubernetes client
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("error getting user home dir: %v\n", err)
		os.Exit(1)
	}
	kubeConfigPath := filepath.Join(userHomeDir, ".kube", "config")
	fmt.Printf("Using kubeconfig: %s\n", kubeConfigPath)

	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		fmt.Printf("error getting Kubernetes config: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		fmt.Printf("error getting Kubernetes clientset: %v\n", err)
		os.Exit(1)
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
