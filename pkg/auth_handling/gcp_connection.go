package auth_handling

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2/google"
)

func GCPAuth(credentialsPath CredentialsPath) (*google.Credentials, error) {
	filePath := credentialsPath.FilePath

	jsonKey, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service account key file: %v", err)
	}
	creds, err := google.CredentialsFromJSON(context.Background(), jsonKey, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to create Google Credentials: %v", err)
	}
	return creds, nil
}
