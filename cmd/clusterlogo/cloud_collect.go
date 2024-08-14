package main

import (
	"fmt"

	"github.com/Golansami125/clusterlogo/pkg/auth_handling"
	"github.com/Golansami125/clusterlogo/pkg/log_parsing"
)

func AuthMain() {
	credentialsPath, clusterInfo, cloudProvider := auth_handling.Authenticator()
	if cloudProvider == "aws" {
		client, err := auth_handling.AwsAuth(credentialsPath)
		if err != nil {
			fmt.Printf("Failed to establish AWS client: %+v\n", err)
		}
		clusterName := clusterInfo.ClusterName

		logGroup, logStream, err := log_parsing.ExtractAWSLogs(client, clusterName)
		if err != nil {
			fmt.Printf("Failed to extract AWS logs: %+v\n", err)
		} else if logGroup == nil || logStream == nil {
			fmt.Printf("Failed to extract AWS logs: No appropriate logGroup or logStream found\n")
		} else {
			// Need to change to go through logs and add logic - need to check logevents return types
			fmt.Printf("Log Group:%+v\nLog Stream:%+v\n", *logGroup, *logStream)
		}

	} else if cloudProvider == "azure" {
		cred, err := auth_handling.AzureAuth(credentialsPath)
		if err != nil {
			fmt.Printf("Failed to establish Azure client: %+v\n", err)
		}
		clusterName := clusterInfo.ClusterName
		workspaceID := clusterInfo.WorkspaceID
		logEvents, err := log_parsing.ExtractAzureLogs(cred, clusterName, workspaceID)
		if err != nil {
			fmt.Printf("Failed to extract Azure logs: %+v\n", err)
		} else {
			// Need to test credential acceptence and add logic to extract azure logs - need to check logevents return types
			fmt.Printf("Azure Credentials:%+v\nCluster Name:%+v\nWorkspace ID:%+v\n", cred, clusterName, workspaceID)
			fmt.Printf("Log Events:%+v\n", logEvents)
		}
	} else if cloudProvider == "gcp" {
		cred, err := auth_handling.GCPAuth(credentialsPath)
		if err != nil {
			fmt.Printf("Failed to establish GCP client: %+v\n", err)
		}
		clusterName := clusterInfo.ClusterName
		projectID := clusterInfo.ProjectID
		region := clusterInfo.Region
		logEvents, err := log_parsing.ExtractGCPLogs(cred, clusterName, projectID, region)
		if err != nil {
			fmt.Printf("Failed to extract Azure logs: %+v\n", err)
		} else {
			// Need to test credential acceptence and check the logic fo GCP log extraction
			fmt.Printf("GCP Credentials:%+v\nCluster Name:%+v\n", cred, clusterName)
			fmt.Printf("Log Events:%+v\n", logEvents)
		}
	}
}
