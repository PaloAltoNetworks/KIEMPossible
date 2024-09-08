package auth_handling

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
)

func connectToEKS(sess *session.Session, clusterName string) (client *kubernetes.Clientset, err error) {
	// Try connecting using InClusterConfig
	config, err := rest.InClusterConfig()
	if err == nil {
		clientset, err := kubernetes.NewForConfig(config)
		if err == nil {
			return clientset, nil
		}
		fmt.Printf("Failed to create Kubernetes client using InClusterConfig: %v\n", err)
	} else {
		fmt.Printf("No InCluster Config, Trying EKS Flow\n")
	}
	eksSvc := eks.New(sess)
	input := &eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	}
	result, err := eksSvc.DescribeCluster(input)
	if err != nil {
		fmt.Printf("Failed to describe EKS cluster: %v\n", err)
		os.Exit(1)
	}
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, err
	}
	opts := &token.GetTokenOptions{
		ClusterID: aws.StringValue(result.Cluster.Name),
	}
	token, err := gen.GetWithOptions(opts)
	if err != nil {
		return nil, err
	}
	ca, err := base64.StdEncoding.DecodeString(aws.StringValue(result.Cluster.CertificateAuthority.Data))
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(&rest.Config{
		Host:        aws.StringValue(result.Cluster.Endpoint),
		BearerToken: token.Token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: ca,
		},
	})
	if err != nil {
		return nil, err
	}
	fmt.Printf("Connected to %+v successfully!\n", clusterName)
	return clientset, nil
}
