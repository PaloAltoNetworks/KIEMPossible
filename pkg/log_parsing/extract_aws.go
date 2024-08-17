package log_parsing

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
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
