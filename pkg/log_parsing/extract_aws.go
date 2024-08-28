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
	fmt.Println("Extracting AWS CloudWatch Logs...")
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
	fmt.Println("Processing AWS Logs...")
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
		permissionScope := getPermissionScope(auditLogEvent.ObjectRef.Namespace, auditLogEvent.ObjectRef.Name)
		lastUsedTime := getLastUsedTime(auditLogEvent.RequestReceivedTimestamp)
		lastUsedResource := getLastUsedResource(auditLogEvent.ObjectRef.Namespace, auditLogEvent.ObjectRef.Resource, auditLogEvent.ObjectRef.Name)
		updateDatabase(db, entityName, entityType, apiGroup, resourceType, verb, permissionScope, lastUsedTime, lastUsedResource)
	}
	fmt.Println("AWS Logs processed successfully!")
}

/*

- check user.groups in the log - if user doesn't exist in the table, add permissions for user based on group permissions. See if can map AWS users
{
    "kind": "Event",
    "apiVersion": "audit.k8s.io/v1",
    "level": "Request",
    "auditID": "fb5dba68-6262-4718-b45e-d293b03e434d",
    "stage": "ResponseComplete",
    "requestURI": "/api/v1/namespaces",
    "verb": "list",
    "user": {
        "username": "arn:aws:sts::211125685544:assumed-role/sso_admin/gomyers-paloaltonetworks.com",
        "uid": "aws-iam-authenticator:211125685544:AROATCKASJUUBCXRRKSFT",
        "groups": [
            "system:authenticated"
        ],
        "extra": {
            "accessKeyId": [
                "ASIATCKASJUUD7MPDOZN"
            ],
            "arn": [
                "arn:aws:sts::211125685544:assumed-role/sso_admin/gomyers@paloaltonetworks.com"
            ],
            "canonicalArn": [
                "arn:aws:iam::211125685544:role/sso_admin"
            ],
            "principalId": [
                "AROATCKASJUUBCXRRKSFT"
            ],
            "sessionName": [
                "gomyers@paloaltonetworks.com"
            ]
        }
    },
    "sourceIPs": [
        "130.41.219.137"
    ],
    "userAgent": "ClusterLoGo_darwin_amd64/v0.0.0 (darwin/amd64) kubernetes/$Format",
    "objectRef": {
        "resource": "namespaces",
        "apiVersion": "v1"
    },
    "responseStatus": {
        "metadata": {},
        "code": 200
    },
    "requestReceivedTimestamp": "2024-08-24T14:27:07.461683Z",
    "stageTimestamp": "2024-08-24T14:27:07.491536Z",
    "annotations": {
        "authorization.k8s.io/decision": "allow",
        "authorization.k8s.io/reason": "EKS Access Policy: allowed by ClusterRoleBinding \"arn:aws:iam::211125685544:role/sso_admin+arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy\" of ClusterRole \"arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy\" to User \"AROATCKASJUUBCXRRKSFT\""
    }
}

*/
