package kube_collection

import (
	"context"
	"encoding/json"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CollectRoles(client *kubernetes.Clientset, roles *map[string]interface{}) error {
	roleList, err := client.RbacV1().Roles("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, role := range roleList.Items {
		roleJSON, err := json.Marshal(role)
		if err != nil {
			return err
		}
		var jsonValue interface{}
		err = json.Unmarshal(roleJSON, &jsonValue)
		if err != nil {
			return err
		}
		(*roles)[role.Name] = jsonValue
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
