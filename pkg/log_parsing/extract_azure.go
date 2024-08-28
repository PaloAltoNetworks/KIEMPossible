package log_parsing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/cheggaaa/pb"
)

func ExtractAzureLogs(cred *azidentity.ClientSecretCredential, clusterName string, workspaceID string) (azquery.LogsClientQueryWorkspaceResponse, error) {
	client, err := azquery.NewLogsClient(cred, nil)
	if err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, err
	}

	var logEvents azquery.LogsClientQueryWorkspaceResponse
	endTime := time.Now()
	startTime := endTime.Add(-1 * 24 * time.Hour) // 10 days ago
	totalDuration := endTime.Sub(startTime)

	progressBar := pb.StartNew(int(totalDuration.Hours()))

	for start := startTime; start.Before(endTime); start = start.Add(1 * time.Hour) {
		end := start.Add(1 * time.Hour)
		if end.After(endTime) {
			end = endTime
		}

		query := fmt.Sprintf(`
            AKSAudit
			| where ResponseStatus.code == 200 and Stage == 'ResponseComplete'
            | where TimeGenerated >= datetime(%v)
            | where TimeGenerated < datetime(%v)
            | project TimeGenerated, Verb, User, ObjectRef
        `, start.Format(time.RFC3339), end.Format(time.RFC3339))

		resp, err := client.QueryWorkspace(context.Background(), workspaceID, azquery.Body{
			Query: to.Ptr(query),
		}, nil)
		if err != nil {
			return azquery.LogsClientQueryWorkspaceResponse{}, err
		}
		if resp.Error != nil {
			return azquery.LogsClientQueryWorkspaceResponse{}, resp.Error
		}

		logEvents.Tables = append(logEvents.Tables, resp.Tables...)
		progressBar.Increment()
	}
	progressBar.Finish()

	return logEvents, nil
}

type UserInfo struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups"`
}

type objectRef struct {
	Resource    string `json:"resource"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	ApiGroup    string `json:"apiGroup"`
	ApiVersion  string `json:"apiVersion"`
	Subresource string `json:"subresource"`
}

func HandleAzureLogs(logEvents azquery.LogsClientQueryWorkspaceResponse, db *sql.DB) {
	fmt.Println("Processing Azure Logs...")
	for _, table := range logEvents.Tables {
		for _, row := range table.Rows {
			userInfoCell, ok := row[2].(string)
			if !ok {
				continue
			}
			var userInfo UserInfo
			err := json.Unmarshal([]byte(userInfoCell), &userInfo)
			if err != nil {
				fmt.Printf("Error unmarshaling user info: %v\n", err)
				continue
			}

			objectRefCell, ok := row[3].(string)
			if !ok {
				continue
			}
			var objectRef objectRef
			err = json.Unmarshal([]byte(objectRefCell), &objectRef)
			if err != nil {
				fmt.Printf("Error unmarshaling object ref: %v\n", err)
				continue
			}
			entityName, entityType := getEntityNameAndType(userInfo.Username)
			apiGroup := getAPIGroup(objectRef.ApiGroup, objectRef.ApiVersion)
			resourceType := getResourceType(objectRef.Resource, objectRef.Subresource)
			verb := row[1].(string)
			permissionScope := getPermissionScope(objectRef.Namespace, objectRef.Name)
			lastUsedTime := getLastUsedTime(row[0].(string))
			lastUsedResource := getLastUsedResource(objectRef.Namespace, resourceType, objectRef.Name)

			updateDatabase(db, entityName, entityType, apiGroup, resourceType, verb, permissionScope, lastUsedTime, lastUsedResource)
		}
	}
	fmt.Println("Azure Logs processed successfully!")
}

/*

- check user in the log - if user doesn't exist in the table, add permissions for user based on group permissions. See if can map Azure users
- cluster admin creds from azure = masterclient (in system:masters and system:authenticated)

- AAD object ID for cluster admin group is in permissions (also check clusterAdmin and clusterUser)
*/
