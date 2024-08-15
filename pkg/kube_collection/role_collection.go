package kube_collection

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CollectRoles(client *kubernetes.Clientset, roles *map[string]*rbacv1.Role) error {
	// Get a list of all namespaces in the cluster
	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, namespace := range namespaces.Items {
		roleList, err := client.RbacV1().Roles(namespace.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, role := range roleList.Items {
			key := fmt.Sprintf("%s/%s", namespace.Name, role.Name)
			(*roles)[key] = &role
		}
	}

	return nil
}

func CollectClusterRoles(client *kubernetes.Clientset, clusterRoles *map[string]*rbacv1.ClusterRole) error {
	clusterRoleList, err := client.RbacV1().ClusterRoles().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, clusterRole := range clusterRoleList.Items {
		(*clusterRoles)[clusterRole.Name] = &clusterRole
	}

	return nil
}
