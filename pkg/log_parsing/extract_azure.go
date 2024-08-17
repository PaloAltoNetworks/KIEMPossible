package log_parsing

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
)

func ExtractAzureLogs(cred *azidentity.ClientSecretCredential, clusterName string, workspaceID string) (azquery.LogsClientQueryWorkspaceResponse, error) {
	client, err := azquery.NewLogsClient(cred, nil)
	if err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, err
	}
	query := `
    	AKSAudit
    	| where TimeGenerated >= datetime(2024-01-01T00:00:00Z) and TimeGenerated <= datetime(2024-02-02T00:00:00Z)
    	| project TimeGenerated, Computer, OperationName, OperationStatus, OperationValue
	`
	logEvents, err := client.QueryWorkspace(context.Background(), workspaceID, azquery.Body{
		Query: to.Ptr(query),
	},
		nil)
	if err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, err
	}

	return logEvents, nil
}
