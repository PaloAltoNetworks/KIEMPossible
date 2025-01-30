package log_parsing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/cheggaaa/pb"
	v1 "k8s.io/api/core/v1"
)

// Get logs from Azure for last 7 days using query
func ExtractAWSLogs(sess *session.Session, clusterName string) ([]*cloudwatchlogs.FilteredLogEvent, error) {
	cwl := cloudwatchlogs.New(sess)
	logGroupName := fmt.Sprintf("/aws/eks/%s/cluster", clusterName)
	now := time.Now()
	start := now.AddDate(0, 0, -7)
	startTime := start.UnixMilli()
	endTime := now.UnixMilli()
	fmt.Printf("Ingesting AWS Logs from %+v to now...\n", start)

	var wg sync.WaitGroup
	logEventsChan := make(chan []*cloudwatchlogs.FilteredLogEvent)
	errorChan := make(chan error)

	bar := pb.StartNew(0)

	for start := startTime; start < endTime; start += 12 * 60 * 60 * 1000 {
		wg.Add(1)
		go func(start int64) {
			defer wg.Done()
			var nextToken *string
			localLogEvents := []*cloudwatchlogs.FilteredLogEvent{}
			for {
				filterLogEventsOutput, err := cwl.FilterLogEvents(&cloudwatchlogs.FilterLogEventsInput{
					StartTime:           aws.Int64(start),
					EndTime:             aws.Int64(min(start+12*60*60*1000, endTime)),
					LogGroupName:        aws.String(logGroupName),
					LogStreamNamePrefix: aws.String("kube-apiserver-audit-"),
					NextToken:           nextToken,
					FilterPattern:       aws.String(`{ $.stage = "ResponseComplete" && $.responseStatus.code = 200 }`),
				})
				if err != nil {
					errorChan <- err
					return
				}
				localLogEvents = append(localLogEvents, filterLogEventsOutput.Events...)
				bar.Add(len(filterLogEventsOutput.Events))
				if filterLogEventsOutput.NextToken == nil {
					break
				}
				nextToken = filterLogEventsOutput.NextToken
			}
			logEventsChan <- localLogEvents
		}(start)
	}

	go func() {
		wg.Wait()
		close(logEventsChan)
		close(errorChan)
	}()

	var logEvents []*cloudwatchlogs.FilteredLogEvent

	for {
		select {
		case localEvents, ok := <-logEventsChan:
			if !ok {
				bar.Finish()
				return logEvents, nil
			}
			logEvents = append(logEvents, localEvents...)
		case err := <-errorChan:
			bar.Finish()
			return nil, fmt.Errorf("error occurred during log extraction: %v", err)
		}
	}
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
	Annotations              struct {
		Reason string `json:"authorization.k8s.io/reason"`
	} `json:"annotations"`
}

// Normalize log data and update DB in batches
func HandleAWSLogs(logEvents []*cloudwatchlogs.FilteredLogEvent, db *sql.DB, sess *session.Session, clusterName string, namespaces *v1.NamespaceList) {
	fmt.Println("Processing AWS Logs...")
	bar := pb.StartNew(len(logEvents))
	userGroups := make(map[string][]string)
	var updateDataList []UpdateData
	for _, event := range logEvents {
		var auditLogEvent AuditLogEvent
		err := json.Unmarshal([]byte(*event.Message), &auditLogEvent)
		if err != nil {
			fmt.Printf("Error parsing log event: %v\n", err)
			continue
		}
		// If permissions given through access policy, use accessPolicy flow
		if strings.HasPrefix(auditLogEvent.Annotations.Reason, "EKS Access Policy") {
			handleEKSAccessPolicy(auditLogEvent.User.Username, auditLogEvent.Annotations.Reason, clusterName, sess, db, namespaces)
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
		bar.Increment()
	}
	bar.Finish()
	fmt.Println("AWS Logs processed successfully!")
	batchUpdateDatabase(db, updateDataList)
}
