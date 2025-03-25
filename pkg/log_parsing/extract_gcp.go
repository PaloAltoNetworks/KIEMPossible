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

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Get logs from AWS for last 7 days
func ExtractGCPLogs(creds *google.Credentials, clusterName, projectID, region string) (string, error) {
	client, err := logadmin.NewClient(context.Background(), projectID, option.WithCredentials(creds))
	if err != nil {
		return "", err
	}
	defer client.Close()

	endTime := time.Now()

	// Default to 7 days, allow override via environment variable
	days := 7
	if envDays := os.Getenv("KIEMPOSSIBLE_LOG_DAYS"); envDays != "" {
		if parsed, err := strconv.Atoi(envDays); err == nil && parsed > 0 {
			days = parsed
		}
	}

	startTime := endTime.Add(-time.Duration(days) * 24 * time.Hour)
	fmt.Printf("Ingesting GCP Logs from %+v to %+v...\n", startTime, endTime)

	// Create a temporary file for logs
	tempFile, err := os.CreateTemp("", "gcp_logs_*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer tempFile.Close()
	writer := bufio.NewWriter(tempFile)

	// Concurrency control
	// Default to 4 concurrent requests
	maxConcurrency := 4

	// Allow override
	if envMax := os.Getenv("KIEMPOSSIBLE_LOG_CONCURRENCY"); envMax != "" {
		if parsed, err := strconv.Atoi(envMax); err == nil && parsed > 0 {
			maxConcurrency = parsed
		}
	}

	// Page size control
	pageSize := int32(1000000)
	if envPageSize := os.Getenv("KIEMPOSSIBLE_GCP_PAGE_SIZE"); envPageSize != "" {
		if parsed, err := strconv.Atoi(envPageSize); err == nil && parsed > 0 {
			pageSize = int32(parsed)
		}
	}

	semaphore := make(chan struct{}, maxConcurrency) // Dynamic concurrency limit
	var wg sync.WaitGroup
	errorChan := make(chan error)
	GlobalProgressBar.Start("cluster log chunks ingested from GCP")
	defer GlobalProgressBar.Stop()
	done := make(chan struct{}) // Channel to signal completion

	for start := startTime; start.Before(endTime); start = start.Add(6 * time.Hour) {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(start time.Time) {
			defer wg.Done()
			end := start.Add(6 * time.Hour)
			if end.After(endTime) {
				end = endTime
			}

			filter := fmt.Sprintf(`
						log_name="projects/%s/logs/cloudaudit.googleapis.com%%2Factivity" AND
						resource.type="k8s_cluster" AND
						resource.labels.cluster_name="%s" AND
						resource.labels.project_id="%s" AND
						resource.labels.location="%s" AND
						protoPayload.status.code=0 AND
						operation.last=true AND
						timestamp>="%s" AND
						timestamp<"%s"
				`, projectID, clusterName, projectID, region, start.Format(time.RFC3339), end.Format(time.RFC3339))

			// Retry with exponential backoff for rate limit
			maxRetries := 6
			baseDelay := time.Duration(2)
			var lastErr error

			for retry := 0; retry < maxRetries; retry++ {
				iter := client.Entries(context.Background(), logadmin.Filter(filter), logadmin.PageSize(pageSize))

				for {
					entry, err := iter.Next()
					if err == iterator.Done {
						break
					}
					if err != nil {
						if strings.Contains(err.Error(), "RATE_LIMIT_EXCEEDED") {
							lastErr = err
							// Exponential backoff
							delay := baseDelay * time.Duration(1<<retry)
							time.Sleep(delay)
							break
						}
						errorChan <- err
						fmt.Println(err.Error())
						<-semaphore
						return
					}

					err = writeGCPLogEntryToTempFile(writer, entry)
					if err != nil {
						fmt.Println(err.Error())
						errorChan <- err
						<-semaphore
						return
					}
					GlobalProgressBar.Add(1)
				}

				// Didn't hit rate limit, break
				if lastErr == nil {
					break
				}
				lastErr = nil
			}

			// If we still have an error, report it
			if lastErr != nil {
				errorChan <- fmt.Errorf("max retries exceeded for rate limit: %v", lastErr)
				fmt.Println(lastErr.Error())
			}

			<-semaphore
		}(start)
	}

	go func() {
		wg.Wait()
		close(errorChan)
		close(done)
	}()

	for {
		select {
		case err, ok := <-errorChan:
			if ok && err != nil { // Only return if a non-nil error is received
				GlobalProgressBar.Stop()
				println()
				return "", fmt.Errorf("error occurred during log extraction: %v", err)
			}
		case <-done:
			GlobalProgressBar.Stop()
			println()
			writer.Flush()
			return tempFile.Name(), nil
		}
	}
}

func writeGCPLogEntryToTempFile(writer *bufio.Writer, entry *logging.Entry) error {
	fileMutex.Lock() // Ensure only one goroutine writes at a time
	defer fileMutex.Unlock()
	if entry.HTTPRequest != nil && entry.HTTPRequest.Request != nil {
		entry.HTTPRequest.Request.GetBody = nil
	}

	logLine, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = writer.WriteString(string(logLine) + "\n")
	if err != nil {
		return err
	}
	return writer.Flush()
}

// Normalize log data and update DB in batches
func HandleGCPLogs(tempFilePath string, db *sql.DB) {
	fmt.Println("Processing GCP Logs and attempting to update database...")

	file, err := os.Open(tempFilePath)
	if err != nil {
		fmt.Printf("Error opening temp file: %v\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var updateDataList []UpdateData
	GlobalProgressBar.Start("cluster events processed")

	for scanner.Scan() {
		line := scanner.Text()
		var entry logging.Entry
		err := json.Unmarshal([]byte(line), &entry)
		if err != nil {
			fmt.Printf("Error parsing log entry: %v\n", err)
			continue
		}

		name, apiGroup, apiVersion, resourceType, verb, namespace, resourceName, err := getvalues(&entry)
		if err != nil {
			fmt.Printf("Error extracting fields from log entry: %v\n", err)
			continue
		}

		entityName, entityType := getEntityNameAndType(name)
		finalApiGroup := getAPIGroup(apiGroup, apiVersion)
		permissionScope := getPermissionScope(namespace, resourceName)

		lastUsedTime := entry.Timestamp.Format("2006-01-02 15:04:05")

		lastUsedResource := getLastUsedResource(namespace, resourceType, resourceName)

		updateDataList = append(updateDataList, UpdateData{
			EntityName:       entityName,
			EntityType:       entityType,
			APIGroup:         finalApiGroup,
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

func getvalues(entry *logging.Entry) (string, string, string, string, string, string, string, error) {
	payload, ok := entry.Payload.(map[string]interface{})
	if !ok {
		return "", "", "", "", "", "", "", fmt.Errorf("payload is not a map[string]interface{}")
	}

	authnInfo, ok := payload["authentication_info"].(map[string]interface{})
	if !ok {
		return "", "", "", "", "", "", "", fmt.Errorf("authentication_info not found or invalid")
	}

	principalEmail, ok := authnInfo["principal_email"].(string)
	if !ok {
		return "", "", "", "", "", "", "", fmt.Errorf("principal_email not found or invalid")
	}

	authzInfoSlice, ok := payload["authorization_info"].([]interface{})
	if !ok || len(authzInfoSlice) == 0 {
		return principalEmail, "", "", "", "", "", "", nil
	}

	authzInfo, ok := authzInfoSlice[0].(map[string]interface{})
	if !ok {
		return principalEmail, "", "", "", "", "", "", fmt.Errorf("authorization_info[0] is not a map[string]interface{}")
	}

	permission, ok := authzInfo["permission"].(string)
	if !ok {
		return principalEmail, "", "", "", "", "", "", fmt.Errorf("permission not found")
	}

	resource, ok := authzInfo["resource"].(string)
	if !ok {
		return principalEmail, "", "", "", "", "", "", fmt.Errorf("resource not found")
	}

	parts := strings.Split(permission, ".")
	if len(parts) < 4 {
		return principalEmail, "", "", "", "", "", "", fmt.Errorf("invalid permission format")
	}

	verb := parts[len(parts)-1]

	// Split the resource path into components
	resourceParts := strings.Split(resource, "/")

	// Initialize variables with default values
	apiGroup := ""
	apiVersion := ""
	namespace := ""
	resourceType := ""
	resourceName := ""

	// Handle different resource path patterns
	switch {
	case len(resourceParts) >= 6 && resourceParts[2] == "namespaces":
		// Pattern: {apiGroup}/{apiVersion}/namespaces/{namespace}/{resourceType}/{resourceName}
		apiGroup = resourceParts[0]
		apiVersion = resourceParts[1]
		namespace = resourceParts[3]
		resourceType = resourceParts[4]
		resourceName = resourceParts[5]
	case len(resourceParts) == 5 && resourceParts[2] == "namespaces":
		// Pattern: {apiGroup}/{apiVersion}/namespaces/{namespace}/{resourceType}
		apiGroup = resourceParts[0]
		apiVersion = resourceParts[1]
		namespace = resourceParts[3]
		resourceType = resourceParts[4]
	case len(resourceParts) == 4:
		// Pattern: {apiGroup}/{apiVersion}/{resourceType}/{resourceName}
		apiGroup = resourceParts[0]
		apiVersion = resourceParts[1]
		resourceType = resourceParts[2]
		resourceName = resourceParts[3]
	case len(resourceParts) == 3:
		// Pattern: {apiGroup}/{apiVersion}/{resourceType}
		apiGroup = resourceParts[0]
		apiVersion = resourceParts[1]
		resourceType = resourceParts[2]
	}

	return principalEmail, apiGroup, apiVersion, resourceType, verb, namespace, resourceName, nil
}
