package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

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

	// Query for unused permissions with risk reasons
	query := `
		SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'Wide secret access permissions' AS risk_reason, last_used_time
		FROM permission 
		WHERE resource_type = 'secrets' AND verb IN('get', 'list') AND permission_scope = 'cluster-wide' 
		GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time

		UNION ALL

		SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'nodes/proxy access permissions' AS risk_reason, last_used_time
		FROM permission 
		WHERE resource_type = 'nodes/proxy' AND verb IN ('create', 'get') AND permission_scope = 'cluster-wide' 
		GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time
		HAVING COUNT(DISTINCT verb) = 2

		UNION ALL

		SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'serviceaccount token creation permissions' AS risk_reason, last_used_time
		FROM permission 
		WHERE resource_type = 'serviceaccounts/token' AND verb = 'create' AND permission_scope = 'cluster-wide' 
		GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time

		UNION ALL

		SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'Escalate, bind or impersonate permissions' AS risk_reason, last_used_time
		FROM permission 
		WHERE verb IN('escalate', 'bind', 'impersonate') AND permission_scope = 'cluster-wide' 
		GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time

		UNION ALL

		SELECT a.entity_name, a.entity_type, a.permission_source, a.permission_source_type, a.permission_binding, a.permission_binding_type, 'CSR and certificate issuing permissions' AS risk_reason, a.last_used_time
		FROM (
			SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time
			FROM permission 
			WHERE resource_type = 'certificatesigningrequests' AND verb = 'create' AND permission_scope = 'cluster-wide' 
			GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time
		) AS a 
		INNER JOIN (
			SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time
			FROM permission 
			WHERE resource_type = 'certificatesigningrequests/approval' AND verb IN ('patch', 'update') 
			GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time
		) AS b 
		ON a.entity_name = b.entity_name AND a.entity_type = b.entity_type 
		AND a.permission_source = b.permission_source AND a.permission_source_type = b.permission_source_type 
		AND a.permission_binding = b.permission_binding AND a.permission_binding_type = b.permission_binding_type

		UNION ALL

		SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'Workload creation permissions' AS risk_reason, last_used_time
		FROM permission 
		WHERE resource_type IN ('pods', 'deployments', 'statefulsets', 'replicasets', 'daemonsets', 'jobs', 'cronjobs') 
		AND verb = 'create' AND permission_scope IN('cluster-wide', 'kube-system') 
		GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time

		UNION ALL

		SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'PersistentVolume creation permissions' AS risk_reason, last_used_time
		FROM permission 
		WHERE resource_type = 'persistentvolumes' AND verb = 'create' AND permission_scope = 'cluster-wide' 
		GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time

		UNION ALL

		SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'Admission webhook management permissions' AS risk_reason, last_used_time
		FROM permission 
		WHERE resource_type IN ('validatingwebhookconfigurations', 'mutatingwebhookconfigurations') 
		AND verb IN ('create', 'delete', 'patch', 'update') AND permission_scope = 'cluster-wide' 
		GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time

		ORDER BY entity_name, entity_type, risk_reason
	`

	// Execute the query
	rows, err := DB.Query(query)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return
	}
	defer rows.Close()

	// Process each row
	for rows.Next() {
		var (
			entityName            string
			entityType            string
			permissionSource      string
			permissionSourceType  string
			permissionBinding     string
			permissionBindingType string
			riskReason            string
			lastUsedTime          sql.NullTime
		)

		err := rows.Scan(&entityName, &entityType, &permissionSource, &permissionSourceType, &permissionBinding, &permissionBindingType, &riskReason, &lastUsedTime)
		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}

		riskReason = strings.ToUpper(riskReason)

		if !lastUsedTime.Valid {
			fmt.Printf("[WARN] %s %s had %s unused in the observed period (%s %s bound by %s %s)\n",
				entityType, entityName, riskReason, permissionSource, permissionSourceType, permissionBinding, permissionBindingType)
		} else {
			unusedDuration := time.Since(lastUsedTime.Time)
			days := int(unusedDuration.Hours() / 24)
			hours := int(unusedDuration.Hours())

			if days >= 1 {
				fmt.Printf("[WARN] %s %s had %s unused for at least %d days (%s %s bound by %s %s)\n",
					entityType, entityName, riskReason, days, permissionSource, permissionSourceType, permissionBinding, permissionBindingType)
			} else {
				fmt.Printf("[WARN] %s %s had %s unused for at least %d hours (%s %s bound by %s %s)\n",
					entityType, entityName, riskReason, hours, permissionSource, permissionSourceType, permissionBinding, permissionBindingType)
			}
		}
	}

	if err = rows.Err(); err != nil {
		fmt.Printf("Error iterating over rows: %v\n", err)
	}

	fmt.Println("\nNOTICE: Only unused permissions observed in the ingestion timeframe which are in the scope of KIEMPOSSIBLE_UNUSED_ENTITY_DAYS are shown. Explore the database for more information.")
}
