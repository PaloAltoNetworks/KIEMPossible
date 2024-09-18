package log_parsing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
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

	endTime := time.Now()
	startTime := endTime.Add(-7 * 24 * time.Hour)

	fmt.Printf("Ingesting Azure Logs from %+v to %+v...\n", startTime, endTime)
	bar := pb.StartNew(0)

	var wg sync.WaitGroup
	logEventsChan := make(chan azquery.LogsClientQueryWorkspaceResponse)
	errorChan := make(chan error)

	for start := startTime; start.Before(endTime); start = start.Add(1 * time.Hour) {
		wg.Add(1)
		go func(start time.Time) {
			defer wg.Done()
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
				errorChan <- err
				return
			}
			if resp.Error != nil {
				errorChan <- resp.Error
				return
			}

			logEventsChan <- resp
			for _, table := range resp.Tables {
				bar.Add(len(table.Rows))
			}
		}(start)
	}

	go func() {
		wg.Wait()
		close(logEventsChan)
		close(errorChan)
	}()

	var logEvents azquery.LogsClientQueryWorkspaceResponse

	for {
		select {
		case resp, ok := <-logEventsChan:
			if !ok {
				bar.Finish()
				return logEvents, nil
			}
			logEvents.Tables = append(logEvents.Tables, resp.Tables...)
		case err := <-errorChan:
			bar.Finish()
			return azquery.LogsClientQueryWorkspaceResponse{}, err
		}
	}
}

type AzureUserInfo struct {
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
	totalRows := 0
	for _, table := range logEvents.Tables {
		totalRows += len(table.Rows)
	}
	bar := pb.StartNew(totalRows)

	userGroups := make(map[string][]string)
	var updateDataList []UpdateData

	for _, table := range logEvents.Tables {
		for _, row := range table.Rows {
			bar.Increment()
			AzureUserInfoCell, ok := row[2].(string)
			if !ok {
				continue
			}
			var AzureUserInfo AzureUserInfo
			err := json.Unmarshal([]byte(AzureUserInfoCell), &AzureUserInfo)
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
			entityName, entityType := getEntityNameAndType(AzureUserInfo.Username)
			entityGroups := AzureUserInfo.Groups
			if _, exists := userGroups[entityName]; !exists {
				userGroups[entityName] = entityGroups
				handleGroupInheritance(db, entityName, entityGroups)
			}
			apiGroup := getAPIGroup(objectRef.ApiGroup, objectRef.ApiVersion)
			resourceType := getResourceType(objectRef.Resource, objectRef.Subresource)
			verb := row[1].(string)
			permissionScope := getPermissionScope(objectRef.Namespace, objectRef.Name)
			lastUsedTime := getLastUsedTime(row[0].(string))
			lastUsedResource := getLastUsedResource(objectRef.Namespace, resourceType, objectRef.Name)

			updateDataList = append(updateDataList, UpdateData{
				EntityName:       entityName,
				EntityType:       entityType,
				APIGroup:         apiGroup,
				ResourceType:     resourceType,
				Verb:             verb,
				PermissionScope:  permissionScope,
				LastUsedTime:     lastUsedTime,
				LastUsedResource: lastUsedResource,
			})
		}
	}
	bar.Finish()
	fmt.Println("Azure Logs processed successfully!")
	batchUpdateDatabase(db, updateDataList)
}
