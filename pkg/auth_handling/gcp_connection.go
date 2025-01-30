package auth_handling

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2/google"
)

// Connect to GCP using creds file which contains all necessary info
func GCPAuth(credentialsPath CredentialsPath) (*google.Credentials, CredentialsPath, error) {
	filePath := credentialsPath.FilePath

	jsonKey, err := os.ReadFile(filePath)
	if err != nil {
		return nil, credentialsPath, fmt.Errorf("failed to read service account key file: %v", err)
	}
	creds, err := google.CredentialsFromJSON(context.Background(), jsonKey, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, credentialsPath, fmt.Errorf("failed to create Google Credentials: %v", err)
	}
	return creds, credentialsPath, nil
}
