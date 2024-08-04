package kube_collection

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CollectPods(
	client *kubernetes.Clientset,
	pods *map[string]string,
	namespaces *map[string]interface{},
	podAttachedSAs *map[string]string,
	issues *map[string]string,
) error {
	nsList, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, ns := range nsList.Items {
		nsName := ns.Name
		nsJSON, err := json.Marshal(ns)
		if err != nil {
			return err
		}
		var jsonValue interface{}
		err = json.Unmarshal(nsJSON, &jsonValue)
		if err != nil {
			return err
		}
		(*namespaces)[nsName] = jsonValue

		// Get pods for the current namespace
		podList, err := client.CoreV1().Pods(nsName).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		// Iterate through pods and build key-value pairs
		for _, pod := range podList.Items {
			podName := pod.Name
			podJSON, err := json.Marshal(pod)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("%s-%s", nsName, podName)
			(*pods)[key] = string(podJSON)

			// Check for service account
			if pod.Spec.ServiceAccountName != "" {
				saName := pod.Spec.ServiceAccountName
				saKey := fmt.Sprintf("%s-%s", nsName, saName)
				(*podAttachedSAs)[key] = saKey

				if saName == "default" {
					defaultSAKey := fmt.Sprintf("%s-%s", nsName, podName)
					(*issues)["[HYGIENE] Default Service Account in Use (ns-pod)"] = defaultSAKey
				}
			}
		}
	}

	return nil
}
