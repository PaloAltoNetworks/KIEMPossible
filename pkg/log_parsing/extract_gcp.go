package log_parsing

import (
	"context"
	"fmt"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

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
