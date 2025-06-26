package log_parsing

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
)

// Get logs from Azure for last 7 days
func ExtractAzureLogs(cred *azidentity.ClientSecretCredential, clusterName string, workspaceID string) (string, error) {
	client, err := azquery.NewLogsClient(cred, nil)
	if err != nil {
		return "", err
	}

	endTime := time.Now()

	// Default to 7 days, allow override via environment variable
	days := 7
	if envDays := os.Getenv("KIEMPOSSIBLE_LOG_DAYS"); envDays != "" {
		if parsed, err := strconv.Atoi(envDays); err == nil && parsed > 0 {
			days = parsed
		}
	}

	startTime := endTime.Add(-time.Duration(days) * 24 * time.Hour)

	fmt.Printf("Ingesting Azure Logs from %+v to %+v...\n", startTime, endTime)

	// Create a temporary file for logs
	tempFile, err := os.CreateTemp("", "azure_logs_*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer tempFile.Close()
	writer := bufio.NewWriter(tempFile)

	// Concurrency control
	// Calculate optimal concurrency based on CPU cores (4<=x<=16)
	numCPU := runtime.NumCPU()
	maxConcurrency := numCPU * 2
	if maxConcurrency < 4 {
		maxConcurrency = 4
	}
	if maxConcurrency > 16 {
		maxConcurrency = 16
	}

	// Allow override
	if envMax := os.Getenv("KIEMPOSSIBLE_LOG_CONCURRENCY"); envMax != "" {
		if parsed, err := strconv.Atoi(envMax); err == nil && parsed > 0 {
			maxConcurrency = parsed
		}
	}

	semaphore := make(chan struct{}, maxConcurrency) // Dynamic concurrency limit
	var wg sync.WaitGroup
	errorChan := make(chan error)
	GlobalProgressBar.Start("cluster log chunks ingested from Azure")
	defer GlobalProgressBar.Stop()

	for start := startTime; start.Before(endTime); start = start.Add(1 * time.Hour) {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(start time.Time) {
			defer wg.Done()
			end := start.Add(1 * time.Hour)
			if end.After(endTime) {
				end = endTime
			}

			query := fmt.Sprintf(`
                AKSAudit
                | where ResponseStatus.code >= 100 and ResponseStatus.code =< 299 and Stage == 'ResponseComplete'
                | where TimeGenerated >= datetime(%v)
                | where TimeGenerated < datetime(%v)
                | project TimeGenerated, Verb, User, ObjectRef
            `, start.Format(time.RFC3339), end.Format(time.RFC3339))

			resp, err := client.QueryWorkspace(context.Background(), workspaceID, azquery.Body{
				Query: to.Ptr(query),
			}, nil)
			if err != nil {
				errorChan <- err
				<-semaphore
				return
			}
			if resp.Error != nil {
				errorChan <- resp.Error
				<-semaphore
				return
			}

			err = writeLogsToTempFile(writer, resp)
			if err != nil {
				errorChan <- err
				<-semaphore
				return
			}
			<-semaphore
		}(start)
	}

	go func() {
		wg.Wait()
		close(errorChan)
	}()

	for {
		select {
		case err := <-errorChan:
			GlobalProgressBar.Stop()
			println()
			return "", fmt.Errorf("error occurred during log extraction: %v", err)
		default:
			GlobalProgressBar.Stop()
			println()
			writer.Flush()
			return tempFile.Name(), nil
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

// Normalize log data and update DB in batches
func HandleAzureLogs(tempFilePath string, db *sql.DB) {
	fmt.Println("Processing Azure Logs and attempting to update database...")

	file, err := os.Open(tempFilePath)
	if err != nil {
		fmt.Printf("Error opening temp file: %v\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	userGroups := make(map[string][]string)
	var updateDataList []UpdateData
	GlobalProgressBar.Start("cluster events processed")

	for scanner.Scan() {
		line := scanner.Text()
		var row []interface{}
		err := json.Unmarshal([]byte(line), &row)
		if err != nil {
			fmt.Printf("Error unmarshaling row: %v\n", err)
			continue
		}

		if len(row) > 3 {
			AzureUserInfoCell, ok := row[2].(string)
			if !ok {
				continue
			}
			var AzureUserInfo AzureUserInfo
			err = json.Unmarshal([]byte(AzureUserInfoCell), &AzureUserInfo)
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
			verb, ok := row[1].(string)
			if !ok {
				continue
			}
			permissionScope := getPermissionScope(objectRef.Namespace, objectRef.Name)
			lastUsedTime, ok := row[0].(string)
			if !ok {
				continue
			}

			parts := strings.Split(strings.Replace(lastUsedTime, "T", " ", 1), ".")
			formattedLastUsedTime := parts[0]
			lastUsedResource := getLastUsedResource(objectRef.Namespace, resourceType, objectRef.Name)

			updateDataList = append(updateDataList, UpdateData{
				EntityName:       entityName,
				EntityType:       entityType,
				APIGroup:         apiGroup,
				ResourceType:     resourceType,
				Verb:             verb,
				PermissionScope:  permissionScope,
				LastUsedTime:     formattedLastUsedTime,
				LastUsedResource: lastUsedResource,
			})

			GlobalProgressBar.Add(1)
			// Periodic memory cleanup
			if len(updateDataList) > 5000 {
				batchUpdateDatabase(db, updateDataList)
				updateDataList = nil
				runtime.GC()
				debug.FreeOSMemory()
			}
		}
	}
	GlobalProgressBar.Stop()
	println()
	batchUpdateDatabase(db, updateDataList)

	// Cleanup temp file
	fmt.Println("Logs processed, cleaning up temp log file...")
	os.Remove(tempFilePath)
}
