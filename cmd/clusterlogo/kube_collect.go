package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Golansami125/clusterlogo/pkg/auth_handling"
	"github.com/Golansami125/clusterlogo/pkg/kube_collection"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func ConnectAndCollect() map[string]string {
	// Kube Logic ------------- move out of main once db logic done
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
	clusterRoles := make(map[string]*rbacv1.ClusterRole)
	roleBindings := make(map[string]interface{})
	//clusterRoleBindings := make(map[string]interface{})
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

	// err = kube_collection.CollectClusterRoleBindings(clientset, &clusterRoleBindings)
	// if err != nil {
	// 	panic(err.Error())
	// }

	// Create DB connection
	DB, err := auth_handling.DBConnect()
	if err != nil {
		fmt.Println("Error in DB Connection", err)
	}
	defer DB.Close()

	// Store Service Accounts in the sa_permissions database in a <namespace>:<sa_name> format
	// err = kube_collection.CollectServiceAccounts(clientset, DB)
	// if err != nil {
	// 	fmt.Println("Error storing service accounts in the database:", err)
	// }

	err = kube_collection.CollectClusterRoleBindings(clientset, DB, clusterRoles)
	if err != nil {
		fmt.Println("Error storing permissions in the database:", err)
	}

	return issues

}
