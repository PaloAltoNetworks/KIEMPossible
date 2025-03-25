package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/Golansami125/kiempossible/pkg/auth_handling"
	"github.com/Golansami125/kiempossible/pkg/log_parsing"
)

// Handling log collection and processing from different cloud providers
// EKS, AKS, GKE, and local supported for now

func Collect() {
	// Authenticate and get cluster information

	credentialsPath, clusterInfo, cloudProvider := auth_handling.Authenticator()
	clusterName := clusterInfo.ClusterName
	workspaceID := clusterInfo.WorkspaceID
	subscriptionID := clusterInfo.Sub
	resourceGroup := clusterInfo.RG
	projectID := clusterInfo.ProjectID
	region := clusterInfo.Region
	logFile := credentialsPath.LogFile

	// Platform specific handling - cluster resource collection, logs extraction and processing and DB updates
	if cloudProvider == "aws" {
		client, err := auth_handling.AwsAuth(credentialsPath)
		if err != nil {
			fmt.Printf("Failed to establish AWS client: %+v\n", err)
		}
		namespaces := KubeCollect(clusterName, "EKS", client, nil, "", "", nil, "", "", credentialsPath)
		log_parsing.InitSession(client)
		logEventsFile, err := log_parsing.ExtractAWSLogs(log_parsing.GetSession(), clusterName)
		if err != nil {
			fmt.Printf("Failed to extract AWS logs: %+v\n", err)
		} else {
			DB, err := auth_handling.DBConnect()
			if err != nil {
				fmt.Println("Error in DB Connection", err)
			}
			defer DB.Close()
			log_parsing.HandleAWSLogs(logEventsFile, DB, client, clusterName, namespaces)
		}

	} else if cloudProvider == "azure" {
		cred, err := auth_handling.AzureAuth(credentialsPath)
		if err != nil {
			fmt.Printf("Failed to establish Azure client: %+v\n", err)
		}
		KubeCollect(clusterName, "AKS", nil, cred, subscriptionID, resourceGroup, nil, "", "", credentialsPath)
		logEventsFile, err := log_parsing.ExtractAzureLogs(cred, clusterName, workspaceID)
		if err != nil {
			fmt.Printf("Failed to extract Azure logs: %+v\n", err)
		} else {
			DB, err := auth_handling.DBConnect()
			if err != nil {
				fmt.Println("Error in DB Connection", err)
			}
			defer DB.Close()
			log_parsing.HandleAzureLogs(logEventsFile, DB)
		}

	} else if cloudProvider == "gcp" {

		cred, cred_path, err := auth_handling.GCPAuth(credentialsPath)
		if err != nil {
			fmt.Printf("Failed to establish GCP client: %+v\n", err)
		}
		KubeCollect(clusterName, "GKE", nil, nil, "", "", cred, region, projectID, cred_path)
		logEventsFile, err := log_parsing.ExtractGCPLogs(cred, clusterName, projectID, region)
		if err != nil {
			fmt.Printf("Failed to extract GCP logs: %+v\n", err)
		} else {
			DB, err := auth_handling.DBConnect()
			if err != nil {
				fmt.Println("Error in DB Connection", err)
			}
			defer DB.Close()
			log_parsing.HandleGCPLogs(logEventsFile, DB)
		}

	} else if cloudProvider == "local" {
		KubeCollect("", "LOCAL", nil, nil, "", "", nil, "", "", credentialsPath)
		logEventsFile, err := log_parsing.ExtractLocalLogs(logFile)
		if err != nil {
			fmt.Printf("Failed to extract Local logs: %+v\n", err)
		} else {
			DB, err := auth_handling.DBConnect()
			if err != nil {
				fmt.Println("Error in DB Connection", err)
			}
			defer DB.Close()
			log_parsing.HandleLocalLogs(logEventsFile, DB)
		}
	}
}

func Advise() {
	fmt.Println("Analyzing results...")
	DB, err := auth_handling.DBConnect()
	if err != nil {
		fmt.Println("Error in DB Connection", err)
		return
	}
	defer DB.Close()

	filterDays := 5
	if envDays := os.Getenv("KIEMPOSSIBLE_UNUSED_ENTITY_DAYS"); envDays != "" {
		if parsed, err := strconv.Atoi(envDays); err == nil && parsed > 0 {
			filterDays = parsed
		}
	}

	query := `
		SELECT entity_type, entity_name, COUNT(*) as unused_count
		FROM permission
		WHERE TIMESTAMPDIFF(HOUR, last_used_time, NOW()) > ?*24
		GROUP BY entity_type, entity_name
		ORDER BY unused_count DESC
	`

	rows, err := DB.Query(query, filterDays)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var entityType, entityName string
		var unusedCount int
		err := rows.Scan(&entityType, &entityName, &unusedCount)
		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}

		fmt.Printf("%s %s has %d permissions which are unused for at least %d days\n",
			entityType, entityName, unusedCount, filterDays)
	}

	if err = rows.Err(); err != nil {
		fmt.Printf("Error iterating over rows: %v\n", err)
	}

	fmt.Println("NOTICE: Only unused permissions observed in the ingestion timerame which are in the scope of KIEMPOSSIBLE_UNUSED_ENTITY_DAYS are shown. Explore the database for more information.")
}
