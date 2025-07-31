package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/PaloAltoNetworks/KIEMPossible/pkg/auth_handling"
	"github.com/PaloAltoNetworks/KIEMPossible/pkg/kube_collection"
	"github.com/aws/aws-sdk-go/aws/session"
	"golang.org/x/oauth2/google"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Handling kube collection and processing from different cloud providers
// EKS, AKS, GKE, and local supported

func KubeCollect(clusterName, clusterType string, sess *session.Session, azure_cred *azidentity.ClientSecretCredential, subscriptionID, resourceGroup string, gcp_cred *google.Credentials, region, projectID string, cred_file auth_handling.CredentialsPath) *v1.NamespaceList {

	// Connect to the cluster
	clientset, err := auth_handling.KubeConnect(clusterName, clusterType, sess, azure_cred, subscriptionID, resourceGroup, gcp_cred, region, projectID, cred_file)
	if err != nil {
		fmt.Printf("error getting Kubernetes clientset: %v\n", err)
		os.Exit(1)
	}

	// Collect and normalize role and rolebinding information and insert into DB
	roles := make(map[string]*rbacv1.Role)
	clusterRoles := make(map[string]*rbacv1.ClusterRole)
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Println("Error in Namespace collection", err)
		return nil
	}

	err = kube_collection.CollectRoles(clientset, &roles)
	if err != nil {
		fmt.Println("Error in Role collection", err)
	}
	err = kube_collection.CollectClusterRoles(clientset, &clusterRoles)
	if err != nil {
		fmt.Println("Error in ClusterRole collection", err)
	}

	DB, err := auth_handling.DBConnect()
	if err != nil {
		fmt.Println("Error in DB Connection", err)
	}
	defer DB.Close()

	err = auth_handling.ClearDatabase(DB)
	if err != nil {
		fmt.Printf("Failed to clear database: %+v\n", err)
	}

	fmt.Printf("Calculating permissions and inserting into DB...\n")
	err = kube_collection.CollectClusterRoleBindings(clientset, DB, clusterRoles)
	if err != nil {
		fmt.Println("Error storing clusterRoleBindings permissions in the database:", err)
	}
	err = kube_collection.CollectRoleBindings(clientset, DB, clusterRoles, roles)
	if err != nil {
		fmt.Println("Error storing RoleBindings permissions in the database:", err)
	}

	// Collect workloads if flag is set
	if cred_file.CollectWorkloads {
		fmt.Printf("\nCollecting workload information...\n")
		if err := kube_collection.Collect_workloads(clientset, DB, clusterType, clusterName, sess); err != nil {
			fmt.Printf("Failed to collect workloads: %+v\n", err)
		}
		fmt.Printf("Workload information collected!\n\n")
	}

	return namespaces
}
