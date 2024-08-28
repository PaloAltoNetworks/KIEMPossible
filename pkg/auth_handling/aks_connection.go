package auth_handling

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func connectToAKS(cred *azidentity.ClientSecretCredential, clusterName, subscription, resourceGroup string) (client *kubernetes.Clientset, err error) {
	// Try connecting using InClusterConfig
	config, err := rest.InClusterConfig()
	if err == nil {
		clientset, err := kubernetes.NewForConfig(config)
		if err == nil {
			return clientset, nil
		}
		fmt.Printf("Failed to create Kubernetes client using InClusterConfig: %v\n", err)
	} else {
		fmt.Printf("No InCluster Config, Trying AKS Flow\n")
	}
	clientFactory, err := armcontainerservice.NewClientFactory(subscription, cred, nil)
	if err != nil {
		return nil, err
	}
	res, err := clientFactory.NewManagedClustersClient().ListClusterAdminCredentials(context.Background(), resourceGroup, clusterName, &armcontainerservice.ManagedClustersClientListClusterAdminCredentialsOptions{ServerFqdn: nil})
	if err != nil {
		return nil, err
	}
	kubeConfig := string(res.CredentialResults.Kubeconfigs[0].Value)

	tempFile, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write([]byte(kubeConfig)); err != nil {
		return nil, err
	}
	os.Setenv("KUBECONFIG", tempFile.Name())

	config, err = clientcmd.BuildConfigFromFlags("", tempFile.Name())
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}
