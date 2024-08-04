package kube_collection

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

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

func CollectClusterRoleBindings(client *kubernetes.Clientset, clusterRoleBindings *map[string]interface{}) error {
	crbList, err := client.RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, crb := range crbList.Items {
		crbJSON, err := json.Marshal(crb)
		if err != nil {
			return err
		}
		var jsonValue interface{}
		err = json.Unmarshal(crbJSON, &jsonValue)
		if err != nil {
			return err
		}
		(*clusterRoleBindings)[crb.Name] = jsonValue
	}

	return nil
}
