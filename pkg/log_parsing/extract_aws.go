package log_parsing

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Golansami125/kiempossible/pkg/auth_handling"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	v1 "k8s.io/api/core/v1"
)

// Get logs from AWS for last 7 days using query
func ExtractAWSLogs(sess *session.Session, clusterName string) (string, error) {
	logGroupName := fmt.Sprintf("/aws/eks/%s/cluster", clusterName)
	now := time.Now()
	start := now.AddDate(0, 0, -7)
	startTime := start.UnixMilli()
	endTime := now.UnixMilli()
	fmt.Printf("Ingesting AWS Logs from %+v to now...\n", start)

	// Create a temporary file for logs
	tempFile, err := os.CreateTemp("", "aws_logs_*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer tempFile.Close()
	writer := bufio.NewWriter(tempFile)

	// Concurrency control
	semaphore := make(chan struct{}, 12)
	var wg sync.WaitGroup
	errorChan := make(chan error)
	GlobalProgressBar.Start("cluster log chunks ingested from AWS")
	defer GlobalProgressBar.Stop()

	for start := startTime; start < endTime; start += 12 * 60 * 60 * 1000 {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(start int64) {
			defer wg.Done()

			var nextToken *string
			for {
				input := &cloudwatchlogs.FilterLogEventsInput{
					StartTime:           aws.Int64(start),
					EndTime:             aws.Int64(min(start+12*60*60*1000, endTime)),
					LogGroupName:        aws.String(logGroupName),
					LogStreamNamePrefix: aws.String("kube-apiserver-audit-"),
					NextToken:           nextToken,
					FilterPattern:       aws.String(`{ $.stage = "ResponseComplete" && $.responseStatus.code = 200 }`),
				}

				filterLogEventsOutput, err := retryWithBackoff(input)
				if err != nil {
					errorChan <- err
					<-semaphore
					return
				}

				err = writeLogsToTempFile(writer, filterLogEventsOutput.Events)
				if err != nil {
					errorChan <- err
					<-semaphore
					return
				}

				if filterLogEventsOutput.NextToken == nil {
					break
				}
				nextToken = filterLogEventsOutput.NextToken
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

func InitSession(sess *session.Session) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	sessionRef = sess
}

func GetSession() *session.Session {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	for sessionRef == nil {
		sessionCond.Wait()
	}
	return sessionRef
}

func UpdateSession(sess *session.Session) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	sessionRef = sess
	sessionCond.Broadcast()
}

func isThrottlingError(err error) bool {
	return strings.Contains(err.Error(), "ThrottlingException")
}

func isExpiredCredentialsError(err error) bool {
	return strings.Contains(err.Error(), "ExpiredToken") || strings.Contains(err.Error(), "AccessDenied")
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func retryWithBackoff(input *cloudwatchlogs.FilterLogEventsInput) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	var attempt int

	for attempt = 0; attempt < 5; attempt++ {
		cwl := cloudwatchlogs.New(GetSession())
		result, err := cwl.FilterLogEvents(input)
		if err == nil {
			GlobalProgressBar.Add(len(result.Events))
			return result, nil
		}

		if isThrottlingError(err) {
			sleepTime := time.Duration((1<<attempt)*100+rand.Intn(100)) * time.Millisecond
			fmt.Printf("Throttling detected, retrying in %v...\n", sleepTime)
			time.Sleep(sleepTime)
			continue
		}

		if isExpiredCredentialsError(err) {
			sessionMutex.Lock()

			if sessionRef == nil {
				sessionCond.Wait()
				sessionMutex.Unlock()
				attempt = 0
				continue
			}

			GlobalProgressBar.Pause()
			sessionRef = nil
			sessionMutex.Unlock()

			if err := reauthenticate(); err != nil {
				return nil, err
			}

			GlobalProgressBar.Resume()
			attempt = 0
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("max retries exceeded")
}

func reauthenticate() error {
	authMutex := &sync.Mutex{}
	authMutex.Lock()
	defer authMutex.Unlock()

	fmt.Println("AWS credentials expired. Please reauthenticate.")

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("AWS Access Key ID: ")
	accessKey, _ := reader.ReadString('\n')
	accessKey = strings.TrimSpace(accessKey)

	fmt.Print("AWS Secret Access Key: ")
	secretKey, _ := reader.ReadString('\n')
	secretKey = strings.TrimSpace(secretKey)

	fmt.Print("AWS Session Token: ")
	sessionToken, _ := reader.ReadString('\n')
	sessionToken = strings.TrimSpace(sessionToken)

	fmt.Print("AWS Region: ")
	region, _ := reader.ReadString('\n')
	region = strings.TrimSpace(region)

	fmt.Println("Reauthenticating with new credentials...")

	newSess, err := auth_handling.AwsReauth(accessKey, secretKey, sessionToken, region)
	if err != nil {
		return fmt.Errorf("reauthentication failed: %v", err)
	}

	UpdateSession(newSess)
	fmt.Println("Session updated successfully!")

	return nil
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
func HandleAWSLogs(tempFilePath string, db *sql.DB, sess *session.Session, clusterName string, namespaces *v1.NamespaceList) {
	fmt.Println("Processing AWS Logs and attempting to update database...")

	// Get total number of lines in the file
	totalLines := countLines(tempFilePath)
	if totalLines == 0 {
		fmt.Println("No logs to process.")
		return
	}

	tempFile, err := os.Open(tempFilePath)
	if err != nil {
		fmt.Printf("Error opening temp file: %v\n", err)
		return
	}
	defer tempFile.Close()

	scanner := bufio.NewScanner(tempFile)
	userGroups := make(map[string][]string)
	var updateDataList []UpdateData
	GlobalProgressBar.Start("cluster events processed", int64(totalLines))

	for scanner.Scan() {
		var event cloudwatchlogs.FilteredLogEvent
		err := json.Unmarshal(scanner.Bytes(), &event)
		if err != nil {
			fmt.Printf("Error parsing log event: %v\n", err)
			continue
		}

		var auditLogEvent AuditLogEvent
		err = json.Unmarshal([]byte(*event.Message), &auditLogEvent)
		if err != nil {
			fmt.Printf("Error parsing audit log event: %v\n", err)
			continue
		}

		// Handle EKS Access Policy
		if strings.HasPrefix(auditLogEvent.Annotations.Reason, "EKS Access Policy") {
			handleEKSAccessPolicy(auditLogEvent.User.Username, auditLogEvent.Annotations.Reason, clusterName, sess, db, namespaces)
		}

		// Extract identity and permissions
		entityName, entityType := getEntityNameAndType(auditLogEvent.User.Username)
		entityGroups := auditLogEvent.User.Groups
		if _, exists := userGroups[entityName]; !exists {
			userGroups[entityName] = entityGroups
			handleGroupInheritance(db, entityName, entityGroups)
		}

		// Process resource access details
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
