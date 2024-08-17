package main

import (
	"fmt"
	"os"

	"github.com/Golansami125/clusterlogo/pkg/auth_handling"
	"github.com/Golansami125/clusterlogo/pkg/kube_collection"
	rbacv1 "k8s.io/api/rbac/v1"
)

func KubeCollect() {
	clientset, err := auth_handling.KubeConnect()
	if err != nil {
		fmt.Printf("error getting Kubernetes clientset: %v\n", err)
		os.Exit(1)
	}

	// Create  maps to store resources
	roles := make(map[string]*rbacv1.Role)
	clusterRoles := make(map[string]*rbacv1.ClusterRole)

	// Collect resources
	err = kube_collection.CollectRoles(clientset, &roles)
	if err != nil {
		panic(err.Error())
	}

	err = kube_collection.CollectClusterRoles(clientset, &clusterRoles)
	if err != nil {
		panic(err.Error())
	}

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
		fmt.Println("Error storing clusterRoleBindings permissions in the database:", err)
	}

	err = kube_collection.CollectRoleBindings(clientset, DB, clusterRoles, roles)
	if err != nil {
		fmt.Println("Error storing RoleBindings permissions in the database:", err)
	}

}
