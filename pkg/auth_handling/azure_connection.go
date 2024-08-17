package auth_handling

import (
	"bufio"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

func AzureAuth(credentialsPath CredentialsPath) (*azidentity.ClientSecretCredential, error) {
	filePath := credentialsPath.FilePath

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var tenantID, clientID, clientSecret string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := strings.TrimSpace(parts[1])
			switch key {
			case "AZURE_TENANT_ID":
				tenantID = value
			case "AZURE_CLIENT_ID":
				clientID = value
			case "AZURE_CLIENT_SECRET":
				clientSecret = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, err
	}

	return cred, nil
}
