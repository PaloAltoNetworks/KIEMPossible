package log_parsing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/cheggaaa/pb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

func ExtractLocalLogs(logFile string) ([]auditv1.Event, error) {
	fmt.Printf("Ingesting Local Logs...\n")
	file, err := os.Open(logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %v", err)
	}

	var events []auditv1.Event
	for _, line := range parseLines(data) {
		event := auditv1.Event{}
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

func parseLines(data []byte) [][]byte {
	var lines [][]byte
	var line []byte
	for _, b := range data {
		if b == '\n' {
			lines = append(lines, line)
			line = nil
			continue
		}
		line = append(line, b)
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	return lines
}

func HandleLocalLogs(logEvents []auditv1.Event, db *sql.DB) {
	fmt.Println("Processing Local Logs...")
	bar := pb.StartNew(0)
	userGroups := make(map[string][]string)
	var updateDataList []UpdateData
	for _, event := range logEvents {
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
		bar.Increment()
	}
	bar.Finish()
	fmt.Println("Log File processed successfully!")
	batchUpdateDatabase(db, updateDataList)
}

func getLocalLastUsedTime(eventTime metav1.MicroTime) string {
	parsedTime := eventTime.Time
	formattedTime := parsedTime.Format("2006-01-02 15:04:05")
	return formattedTime
}
