package auth_handling

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// Connect to Azure using creds file which contains all necessary info
func AzureAuth(credentialsPath CredentialsPath) (*azidentity.ClientSecretCredential, error) {
	tenantID := credentialsPath.TenantID
	clientID := credentialsPath.ClientID
	clientSecret := credentialsPath.ClientSecret

	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, err
	}

	return cred, nil
}
