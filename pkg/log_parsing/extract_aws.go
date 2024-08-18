package log_parsing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

func ExtractAWSLogs(sess *session.Session, clusterName string) ([]*cloudwatchlogs.FilteredLogEvent, error) {
	cwl := cloudwatchlogs.New(sess)
	logGroupName := fmt.Sprintf("/aws/eks/%s/cluster", clusterName)
	now := time.Now()
	start := now.AddDate(0, 0, -15)
	startTime := start.UnixMilli()
	endTime := now.UnixMilli()

	var logEvents []*cloudwatchlogs.FilteredLogEvent
	var nextToken *string

	for {
		filterLogEventsOutput, err := cwl.FilterLogEvents(&cloudwatchlogs.FilterLogEventsInput{
			StartTime:           aws.Int64(startTime),
			EndTime:             aws.Int64(endTime),
			LogGroupName:        aws.String(logGroupName),
			LogStreamNamePrefix: aws.String("kube-apiserver-audit-"),
			NextToken:           nextToken,
		})
		if err != nil {
			return nil, err
		}
		logEvents = append(logEvents, filterLogEventsOutput.Events...)

		if filterLogEventsOutput.NextToken == nil {
			break
		}
		nextToken = filterLogEventsOutput.NextToken
	}
	return logEvents, nil
}

type AuditLogEvent struct {
	Verb string `json:"verb"`
	User struct {
		Username string `json:"username"`
	} `json:"user"`
	ObjectRef struct {
		Resource       string `json:"resource"`
		Namespace      string `json:"namespace"`
		Name           string `json:"name"`
		UID            string `json:"uid"`
		APIGroup       string `json:"apiGroup"`
		APIVersion     string `json:"apiVersion"`
		ResourceSource struct {
			Resource string `json:"resource"`
		} `json:"resourceSource"`
	} `json:"objectRef"`
	RequestReceivedTimestamp string `json:"requestReceivedTimestamp"`
}

func HandleAWSLogs(logEvents []*cloudwatchlogs.FilteredLogEvent, db *sql.DB) {
	for _, event := range logEvents {
		var auditLogEvent AuditLogEvent
		err := json.Unmarshal([]byte(*event.Message), &auditLogEvent)
		if err != nil {
			fmt.Printf("Error parsing log event: %v\n", err)
			continue
		}

		entityName, entityType := getEntityNameAndType(auditLogEvent.User.Username)
		apiGroup := getAPIGroup(auditLogEvent.ObjectRef.APIGroup, auditLogEvent.ObjectRef.APIVersion)
		resourceType := auditLogEvent.ObjectRef.Resource
		verb := auditLogEvent.Verb
		permissionScope := getPermissionScope(auditLogEvent.ObjectRef.Namespace, auditLogEvent.ObjectRef.Resource, auditLogEvent.ObjectRef.Name)
		lastUsedTime := getLastUsedTime(auditLogEvent.RequestReceivedTimestamp)
		lastUsedResource := getLastUsedResource(auditLogEvent.ObjectRef.Namespace, auditLogEvent.ObjectRef.Resource, auditLogEvent.ObjectRef.Name)
		// Get * from permission
		// If a match is found for this group
		// updateDatabase(entityName, entityType, apiGroup, resourceType, verb, permissionScope, lastUsedTime, lastUsedResource)
		fmt.Println(entityName, entityType, apiGroup, resourceType, verb, permissionScope, lastUsedTime, lastUsedResource)

	}
}

func getEntityNameAndType(username string) (string, string) {
	if strings.HasPrefix(username, "system:serviceaccount:") {
		return strings.TrimPrefix(username, "system:serviceaccount:"), "ServiceAccount"
	}
	return username, "User"
}

func getAPIGroup(apiGroup, apiVersion string) string {
	if apiGroup == "" {
		return apiVersion
	}
	return apiGroup + "/" + apiVersion
}

func getPermissionScope(namespace, resource, name string) string {
	if namespace != "" && name != "" {
		return namespace + "/" + resource + "/" + name
	} else if name != "" {
		return resource + "/" + name
	} else if namespace != "" {
		return namespace
	} else {
		return "cluster-wide"
	}
}

func getLastUsedTime(timestamp string) int64 {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return 0
	}
	return t.UnixNano() / int64(time.Millisecond)
}

func getLastUsedResource(namespace, resource, name string) string {
	if namespace != "" && name != "" {
		return namespace + "/" + resource + "/" + name
	} else if name != "" {
		return resource + "/" + name
	} else if namespace != "" {
		return namespace + "/" + resource
	} else {
		return resource
	}
}

func updateDatabase(db *sql.DB, entityName, entityType, apiGroup, resourceType, verb, permissionScope string, lastUsedTime int64, lastUsedResource string) error {
	// Implement db update logic
	return nil
}

// entity_name, entity_type, api_group, resource_type, verb, permission_scope
