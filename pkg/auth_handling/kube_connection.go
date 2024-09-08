package auth_handling

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/aws/aws-sdk-go/aws/session"
	"golang.org/x/oauth2/google"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func KubeConnect(clusterName string, clusterType string, aws_sess *session.Session, azure_cred *azidentity.ClientSecretCredential, Sub, RG string, gcp_cred *google.Credentials, region, projectID string, cred_file CredentialsPath) (client *kubernetes.Clientset, err error) {
	switch clusterType {
	case "EKS":
		return connectToEKS(aws_sess, clusterName)
	case "AKS":
		return connectToAKS(azure_cred, clusterName, Sub, RG)
	case "GKE":
		return connectToGKE(gcp_cred, clusterName, region, projectID, cred_file)
	case "LOCAL":
		return connectToLocal()
	default:
		return nil, fmt.Errorf("unsupported cluster type: %s", clusterType)
	}
}

func connectToLocal() (client *kubernetes.Clientset, err error) {
	// Try connecting using InClusterConfig
	config, err := rest.InClusterConfig()
	if err == nil {
		clientset, err := kubernetes.NewForConfig(config)
		if err == nil {
			return clientset, nil
		}
		fmt.Printf("Failed to create Kubernetes client using InClusterConfig: %v\n", err)
	} else {
		fmt.Printf("InClusterConfig is not available: %v\n", err)
	}

	// Fallback to kubeconfig
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
		return nil, err
	}

	fmt.Printf("Connected to Cluster successfully!\n")
	return clientset, nil
}
