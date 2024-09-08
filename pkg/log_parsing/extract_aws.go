package log_parsing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/cheggaaa/pb"
)

func ExtractAWSLogs(sess *session.Session, clusterName string) ([]*cloudwatchlogs.FilteredLogEvent, error) {
	cwl := cloudwatchlogs.New(sess)
	logGroupName := fmt.Sprintf("/aws/eks/%s/cluster", clusterName)
	now := time.Now()
	start := now.AddDate(0, 0, -10)
	startTime := start.UnixMilli()
	endTime := now.UnixMilli()

	var logEvents []*cloudwatchlogs.FilteredLogEvent
	var nextToken *string
	fmt.Println("Ingesting AWS Logs from %v to %v...", startTime, now)
	bar := pb.StartNew(0)

	for {
		filterLogEventsOutput, err := cwl.FilterLogEvents(&cloudwatchlogs.FilterLogEventsInput{
			StartTime:           aws.Int64(startTime),
			EndTime:             aws.Int64(endTime),
			LogGroupName:        aws.String(logGroupName),
			LogStreamNamePrefix: aws.String("kube-apiserver-audit-"),
			NextToken:           nextToken,
			FilterPattern:       aws.String(`{ $.stage = "ResponseComplete" && $.responseStatus.code = 200 }`),
		})
		if err != nil {
			return nil, err
		}
		logEvents = append(logEvents, filterLogEventsOutput.Events...)
		bar.Add(len(filterLogEventsOutput.Events))

		if filterLogEventsOutput.NextToken == nil {
			break
		}
		nextToken = filterLogEventsOutput.NextToken
	}
	bar.Finish()
	return logEvents, nil
}

type AuditLogEvent struct {
	Verb string `json:"verb"`
	User struct {
		Username string   `json:"username"`
		Groups   []string `json:"groups"`
	} `json:"user"`
	ObjectRef struct {
		Resource    string `json:"resource"`
		Subresource string `json:"subresource"`
		Namespace   string `json:"namespace"`
		Name        string `json:"name"`
		UID         string `json:"uid"`
		APIGroup    string `json:"apiGroup"`
		APIVersion  string `json:"apiVersion"`
	} `json:"objectRef"`
	RequestReceivedTimestamp string `json:"requestReceivedTimestamp"`
}

func HandleAWSLogs(logEvents []*cloudwatchlogs.FilteredLogEvent, db *sql.DB) {
	fmt.Println("Processing AWS Logs...")
	bar := pb.StartNew(0)
	userGroups := make(map[string][]string)
	for _, event := range logEvents {
		var auditLogEvent AuditLogEvent
		err := json.Unmarshal([]byte(*event.Message), &auditLogEvent)
		if err != nil {
			fmt.Printf("Error parsing log event: %v\n", err)
			continue
		}

		entityName, entityType := getEntityNameAndType(auditLogEvent.User.Username)
		entityGroups := auditLogEvent.User.Groups
		if _, exists := userGroups[entityName]; !exists {
			userGroups[entityName] = entityGroups
			handleGroupInheritance(db, entityName, entityGroups)
		}
		apiGroup := getAPIGroup(auditLogEvent.ObjectRef.APIGroup, auditLogEvent.ObjectRef.APIVersion)
		resourceType := getResourceType(auditLogEvent.ObjectRef.Resource, auditLogEvent.ObjectRef.Subresource)
		verb := auditLogEvent.Verb
		permissionScope := getPermissionScope(auditLogEvent.ObjectRef.Namespace, auditLogEvent.ObjectRef.Name)
		lastUsedTime := getLastUsedTime(auditLogEvent.RequestReceivedTimestamp)
		lastUsedResource := getLastUsedResource(auditLogEvent.ObjectRef.Namespace, resourceType, auditLogEvent.ObjectRef.Name)
		updateDatabase(db, entityName, entityType, apiGroup, resourceType, verb, permissionScope, lastUsedTime, lastUsedResource)

		bar.Increment()
	}
	bar.Finish()
	fmt.Println("AWS Logs processed successfully!")
}
