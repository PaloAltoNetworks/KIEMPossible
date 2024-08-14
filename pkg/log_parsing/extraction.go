package log_parsing

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func ExtractAWSLogs(sess *session.Session, clusterName string) (*string, *string, error) {
	cwl := cloudwatchlogs.New(sess)

	logGroupNamePattern := aws.String(clusterName)

	logGroupsOutput, err := cwl.DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePattern: logGroupNamePattern,
	})
	if err != nil {
		return nil, nil, err
	}

	if len(logGroupsOutput.LogGroups) > 0 {
		for _, logGroup := range logGroupsOutput.LogGroups {
			logGroupName := logGroup.LogGroupName

			logStreamsOutput, err := cwl.DescribeLogStreams(&cloudwatchlogs.DescribeLogStreamsInput{
				LogGroupName: logGroupName,
			})
			if err != nil {
				return nil, nil, err
			}

			for _, logStream := range logStreamsOutput.LogStreams {

				if logStream.LogStreamName != nil && strings.HasPrefix(*logStream.LogStreamName, "kube-apiserver-audit-") {
					logEvents, err := getAWSLogEvents(cwl, logGroupName, logStream.LogStreamName)
					if err != nil {
						return nil, nil, err
					} else if len(logEvents) == 0 {
						fmt.Printf("No Logs Events")
						return logGroupName, logStream.LogStreamName, nil
					} else if len(logEvents) > 0 {
						// Logic to handle logs
						return logGroupName, logStream.LogStreamName, nil
					}
				}
			}
		}
	}

	return nil, nil, nil
}

func getAWSLogEvents(cwl *cloudwatchlogs.CloudWatchLogs, logGroupName *string, logStreamName *string) ([]*cloudwatchlogs.OutputLogEvent, error) {
	var logEvents []*cloudwatchlogs.OutputLogEvent
	nextToken := aws.String("")

	for {
		logEventsOutput, err := cwl.GetLogEvents(&cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  logGroupName,
			LogStreamName: logStreamName,
			NextToken:     nextToken,
		})
		if err != nil {
			return nil, err
		}

		logEvents = append(logEvents, logEventsOutput.Events...)

		if logEventsOutput.NextForwardToken == nil {
			break
		}
		nextToken = logEventsOutput.NextForwardToken
	}

	return logEvents, nil
}

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

func ExtractGCPLogs(creds *google.Credentials, clusterName, projectID, region string) ([]*logging.Entry, error) {
	client, err := logadmin.NewClient(context.Background(), projectID, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to create logging client: %v", err)
	}
	defer client.Close()
	filter := fmt.Sprintf(`logName="cloudaudit.googleapis.com/activity" ANDs
							resource.type="k8s_cluster" AND
                            resource.labels.cluster_name="%s" AND
							resource.labels.project_id="%s" AND
							resource.labels.location="%s" AND
                            protoPayload.@type="type.googleapis.com/k8s.io.audit.v1.Event"`, clusterName, projectID, region)

	var logEvents []*logging.Entry
	iter := client.Entries(context.Background(), logadmin.Filter(filter))

	for {
		entry, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve logs: %v", err)
		}
		logEvents = append(logEvents, entry)
	}

	return logEvents, nil
}
