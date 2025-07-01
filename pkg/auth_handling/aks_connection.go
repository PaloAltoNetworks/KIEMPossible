package auth_handling

import (
	"context"
	"fmt"

	"encoding/base64"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
		fmt.Printf("No InCluster Config, Trying AKS Flow...\n")
	}
	// Revert to AKS flow dynamically creating kubeconfig
	clientFactory, err := armcontainerservice.NewClientFactory(subscription, cred, nil)
	if err != nil {
		return nil, err
	}
	res, err := clientFactory.NewManagedClustersClient().ListClusterUserCredentials(context.Background(), resourceGroup, clusterName, &armcontainerservice.ManagedClustersClientListClusterUserCredentialsOptions{ServerFqdn: nil})
	if err != nil {
		return nil, err
	}

	kubeConfigBytes := res.CredentialResults.Kubeconfigs[0].Value

	// Parse the kubeconfig
	var kubeConfig map[string]interface{}
	if err := yaml.Unmarshal(kubeConfigBytes, &kubeConfig); err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %v", err)
	}

	// Extract cluster info
	clusters := kubeConfig["clusters"].([]interface{})
	cluster := clusters[0].(map[string]interface{})
	clusterInfo := cluster["cluster"].(map[string]interface{})
	server := clusterInfo["server"].(string)
	caData := clusterInfo["certificate-authority-data"].(string)

	// Extract user info
	users := kubeConfig["users"].([]interface{})
	user := users[0].(map[string]interface{})
	userInfo := user["user"].(map[string]interface{})

	// Extract server-id from exec args for token
	var serverID string
	if exec, ok := userInfo["exec"].(map[string]interface{}); ok {
		if args, ok := exec["args"].([]interface{}); ok {
			for i, arg := range args {
				if argStr, ok := arg.(string); ok && argStr == "--server-id" && i+1 < len(args) {
					if serverIDArg, ok := args[i+1].(string); ok {
						serverID = serverIDArg
						break
					}
				}
			}
		}
	}

	if serverID == "" {
		return nil, fmt.Errorf("could not extract server-id from kubeconfig")
	}

	// Get Azure AD token using the server-id as the audience
	token, err := cred.GetToken(context.Background(), policy.TokenRequestOptions{
		Scopes: []string{serverID + "/.default"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure AD token: %v", err)
	}

	// Decode CA certificate
	caCert, err := base64.StdEncoding.DecodeString(caData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA certificate: %v", err)
	}

	// Create REST config directly
	restConfig := &rest.Config{
		Host: server,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caCert,
		},
		BearerToken: token.Token,
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Connected to %+v successfully!\n", clusterName)
	return clientset, nil
}
