package log_parsing

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// Read log entries from the local file
func ExtractLocalLogs(logFile string) (string, error) {
	fmt.Printf("Ingesting Local Logs...\n")
	file, err := os.Open(logFile)
	if err != nil {
		return "", fmt.Errorf("failed to open log file: %v", err)
	}
	defer file.Close()

	tempFile, err := os.CreateTemp("", "local_logs_*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer tempFile.Close()
	writer := bufio.NewWriter(tempFile)

	GlobalProgressBar.Start("cluster log chunks ingested from file")
	defer GlobalProgressBar.Stop()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		event := auditv1.Event{}
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		_, err = writer.Write(append(data, '\n'))
		if err != nil {
			continue
		}
		GlobalProgressBar.Add(1)
	}

	if err := scanner.Err(); err != nil {
		GlobalProgressBar.Stop()
		println()
		return "", fmt.Errorf("error scanning file: %v", err)
	}

	GlobalProgressBar.Stop()
	println()
	writer.Flush()
	return tempFile.Name(), nil
}

// Handle logs, normalize with helper functions and insert to DB
func HandleLocalLogs(tempFilePath string, db *sql.DB) {
	fmt.Println("Processing Local Logs...")

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
		line := scanner.Bytes()
		event := auditv1.Event{}
		if err := json.Unmarshal(line, &event); err != nil {
			fmt.Printf("Error unmarshaling event: %v\n", err)
			continue
		}

		if event.Stage == "ResponseComplete" && event.ResponseStatus != nil && event.ResponseStatus.Code == 200 {
			entityName, entityType := getEntityNameAndType(event.User.Username)
			entityGroups := event.User.Groups
			if _, exists := userGroups[entityName]; !exists {
				userGroups[entityName] = entityGroups
				handleGroupInheritance(db, entityName, entityGroups)
			}
			apiGroup := getAPIGroup(event.ObjectRef.APIGroup, event.ObjectRef.APIVersion)
			resourceType := event.ObjectRef.Resource
			verb := event.Verb
			permissionScope := getPermissionScope(event.ObjectRef.Namespace, event.ObjectRef.Name)
			lastUsedTime := getLocalLastUsedTime(event.RequestReceivedTimestamp)
			lastUsedResource := getLastUsedResource(event.ObjectRef.Namespace, event.ObjectRef.Resource, event.ObjectRef.Name)
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
		GlobalProgressBar.Add(1)
		// Periodic memory cleanup
		if len(updateDataList) > 5000 {
			batchUpdateDatabase(db, updateDataList)
			updateDataList = nil
			runtime.GC()
			debug.FreeOSMemory()
		}
	}

	GlobalProgressBar.Stop()
	println()
	batchUpdateDatabase(db, updateDataList)

	// Cleanup temp file
	fmt.Println("Logs processed, cleaning up temp log file...")
	os.Remove(tempFilePath)
}

func getLocalLastUsedTime(eventTime metav1.MicroTime) string {
	parsedTime := eventTime.Time
	formattedTime := parsedTime.Format("2006-01-02 15:04:05")
	return formattedTime
}
