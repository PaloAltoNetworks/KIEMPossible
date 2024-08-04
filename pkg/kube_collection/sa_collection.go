package kube_collection

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CollectServiceAccounts(client *kubernetes.Clientset, serviceAccounts *map[string]interface{}) error {
	nsList, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, ns := range nsList.Items {
		nsName := ns.Name

		// Get service accounts for the current namespace
		saList, err := client.CoreV1().ServiceAccounts(nsName).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		// Iterate through service accounts and build key-value pairs
		for _, sa := range saList.Items {
			saName := sa.Name
			saJSON, err := json.Marshal(sa)
			if err != nil {
				return err
			}
			var jsonValue interface{}
			err = json.Unmarshal(saJSON, &jsonValue)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("%s-%s", nsName, saName)
			(*serviceAccounts)[key] = jsonValue
		}
	}

	return nil
}
