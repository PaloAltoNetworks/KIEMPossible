package auth_handling

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	container "cloud.google.com/go/container/apiv1"
	"cloud.google.com/go/container/apiv1/containerpb"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	container2 "google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func connectToGKE(cred *google.Credentials, clusterName, region, projectID string, cred_file CredentialsPath) (client *kubernetes.Clientset, err error) {
	// Try connecting using InClusterConfig
	config, err := rest.InClusterConfig()
	if err == nil {
		clientset, err := kubernetes.NewForConfig(config)
		if err == nil {
			return clientset, nil
		}
		fmt.Printf("Failed to create Kubernetes client using InClusterConfig: %v\n", err)
	} else {
		fmt.Printf("No InCluster Config, Trying GKE Flow...\n")
	}
	containerClient, err := container.NewClusterManagerClient(context.Background(), option.WithCredentials(cred))
	if err != nil {
		return nil, err
	}
	cluster, err := containerClient.GetCluster(context.Background(), &containerpb.GetClusterRequest{
		Name:      fmt.Sprintf("projects/%s/locations/%s/clusters/%s", projectID, region, clusterName),
		ProjectId: projectID,
	})
	if err != nil {
		return nil, err
	}

	ca, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, err
	}

	token, err := getToken(cred_file)

	clientset, err := kubernetes.NewForConfig(&rest.Config{
		Host:        cluster.Endpoint,
		BearerToken: token.AccessToken,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: ca,
		},
	})
	fmt.Printf("Connected to %+v successfully!\n", clusterName)
	return clientset, nil
}

type sa struct {
	Type                    string `json:"type"`
	ProjectId               string `json:"project_id"`
	PrivateKeyId            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientId                string `json:"client_id"`
	TokenUri                string `json:"auth_token_uri"`
	AuthProviderX509CertUrl string `json:"auth_provider_x509_cert_url"`
	ClientX509CertUrl       string `json:"client_x509_cert_url"`
}

func getToken(cred_file CredentialsPath) (*oauth2.Token, error) {
	sa := sa{}
	filePath := cred_file.FilePath

	jsonKey, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonKey, &sa)
	if err != nil {
		return nil, err
	}
	cfg := &jwt.Config{
		Email:        sa.ClientEmail,
		PrivateKey:   []byte(sa.PrivateKey),
		PrivateKeyID: sa.PrivateKeyId,
		Scopes:       []string{container2.CloudPlatformScope},
		TokenURL:     sa.TokenUri,
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = google.JWTTokenURL
	}
	ts := cfg.TokenSource(context.Background())
	token, err := ts.Token()
	if err != nil {
		return nil, err
	}
	return token, nil
}
