package log_parsing

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Create logging client and query - irrelevant because of logging api rate limit. Need to verify, but probably do with pub/sub.
// TBD
func ExtractGCPLogs(creds *google.Credentials, clusterName, projectID, region string) ([]*logging.Entry, error) {
	client, err := logadmin.NewClient(context.Background(), projectID, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to create logging client: %v", err)
	}
	defer client.Close()
	start := time.Now().AddDate(0, 0, -10).Format(time.RFC3339)
	filter := fmt.Sprintf(` log_name="projects/%s/logs/cloudaudit.googleapis.com%%2Factivity" AND
							resource.type="k8s_cluster" AND
                            resource.labels.cluster_name="%s" AND
							resource.labels.project_id="%s" AND
							resource.labels.location="%s" AND
							timestamp>"%s"`, projectID, clusterName, projectID, region, start)

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
