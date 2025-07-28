package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"encoding/json"

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
	fmt.Println("\n\033[31mPreparing output report...\033[0m")
	currentDate := time.Now().Format("20060102")
	filename := fmt.Sprintf("kiempossible_report_%s.json", currentDate)
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating report file: %v\n", err)
		return
	}
	defer file.Close()

	DB, err := auth_handling.DBConnect()
	if err != nil {
		fmt.Println("Error in DB Connection", err)
		return
	}
	defer DB.Close()

	report := make(map[string]interface{})

	// Section 1: Entities with Risky Permissions
	riskyPermissions := []map[string]interface{}{}
	query := `
		SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'Wide secret access permissions' AS risk_reason, last_used_time
		FROM permission 
		WHERE resource_type = 'secrets' AND verb IN('get', 'list') 
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
		WHERE resource_type = 'serviceaccounts/token' AND verb = 'create'
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
		AND verb = 'create'
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

	rows, err := DB.Query(query)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			entityName            string
			entityType            string
			permissionSource      string
			permissionSourceType  string
			permissionBinding     string
			permissionBindingType string
			riskReason            string
			lastUsedTimeRaw       sql.RawBytes
		)

		err := rows.Scan(&entityName, &entityType, &permissionSource, &permissionSourceType, &permissionBinding, &permissionBindingType, &riskReason, &lastUsedTimeRaw)
		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}

		riskReason = strings.ToUpper(riskReason)
		lastUsed := ""
		if len(lastUsedTimeRaw) == 0 {
			lastUsed = "UNUSED in the observed period"
		} else {
			lastUsedTimeStr := string(lastUsedTimeRaw)
			t, err := time.Parse("2006-01-02 15:04:05", lastUsedTimeStr)
			if err != nil {
				lastUsed = "Parse error: " + lastUsedTimeStr
			} else {
				unusedDuration := time.Since(t)
				days := int(unusedDuration.Hours() / 24)
				hours := int(unusedDuration.Hours())
				if days >= 1 {
					lastUsed = fmt.Sprintf("UNUSED for at least %d days", days)
				} else {
					lastUsed = fmt.Sprintf("UNUSED for at least %d hours", hours)
				}
			}
		}
		row := map[string]interface{}{
			"entity_name":                       entityName,
			"entity_type":                       entityType,
			"permission_source":                 permissionSource,
			"permission_source_type":            permissionSourceType,
			"permission_binding":                permissionBinding,
			"permission_binding_type":           permissionBindingType,
			"risk_reason":                       riskReason,
			"last_used_time_or_unused_duration": lastUsed,
		}
		riskyPermissions = append(riskyPermissions, row)
	}
	if err = rows.Err(); err != nil {
		fmt.Printf("Error iterating over rows: %v\n", err)
	}
	report["risky_permissions"] = riskyPermissions

	// Section 2: Workloads using Service Accounts with Risky Permissions
	var count int
	err = DB.QueryRow("SELECT COUNT(*) FROM rufus.workload_identities").Scan(&count)
	if err != nil {
		fmt.Printf("Error checking workload_identities table: %v\n", err)
		return
	}
	workloads := []map[string]interface{}{}
	if count != 0 {
		workloadQuery := `
		WITH risky_permissions AS (
			SELECT DISTINCT entity_name, risk_reason
			FROM (
				SELECT entity_name, 'Wide secret access permissions' AS risk_reason
				FROM permission 
				WHERE resource_type = 'secrets' AND verb IN('get', 'list') AND permission_scope = 'cluster-wide' 
				GROUP BY entity_name

				UNION ALL

				SELECT entity_name, 'nodes/proxy access permissions' AS risk_reason
				FROM permission 
				WHERE resource_type = 'nodes/proxy' AND verb IN ('create', 'get') AND permission_scope = 'cluster-wide' 
				GROUP BY entity_name
				HAVING COUNT(DISTINCT verb) = 2

				UNION ALL

				SELECT entity_name, 'serviceaccount token creation permissions' AS risk_reason
				FROM permission 
				WHERE resource_type = 'serviceaccounts/token' AND verb = 'create'
				GROUP BY entity_name

				UNION ALL

				SELECT entity_name, 'Escalate, bind or impersonate permissions' AS risk_reason
				FROM permission 
				WHERE verb IN('escalate', 'bind', 'impersonate') AND permission_scope = 'cluster-wide' 
				GROUP BY entity_name

				UNION ALL

				SELECT a.entity_name, 'CSR and certificate issuing permissions' AS risk_reason
				FROM (
					SELECT entity_name
					FROM permission 
					WHERE resource_type = 'certificatesigningrequests' AND verb = 'create' AND permission_scope = 'cluster-wide' 
					GROUP BY entity_name
				) AS a 
				INNER JOIN (
					SELECT entity_name
					FROM permission 
					WHERE resource_type = 'certificatesigningrequests/approval' AND verb IN ('patch', 'update') 
					GROUP BY entity_name
				) AS b 
				ON a.entity_name = b.entity_name

				UNION ALL

				SELECT entity_name, 'Workload creation permissions' AS risk_reason
				FROM permission 
				WHERE resource_type IN ('pods', 'deployments', 'statefulsets', 'replicasets', 'daemonsets', 'jobs', 'cronjobs') 
				AND verb = 'create' AND permission_scope IN('cluster-wide', 'kube-system') 
				GROUP BY entity_name

				UNION ALL

				SELECT entity_name, 'PersistentVolume creation permissions' AS risk_reason
				FROM permission 
				WHERE resource_type = 'persistentvolumes' AND verb = 'create' AND permission_scope = 'cluster-wide' 
				GROUP BY entity_name

				UNION ALL

				SELECT entity_name, 'Admission webhook management permissions' AS risk_reason
				FROM permission 
				WHERE resource_type IN ('validatingwebhookconfigurations', 'mutatingwebhookconfigurations') 
				AND verb IN ('create', 'delete', 'patch', 'update') AND permission_scope = 'cluster-wide' 
				GROUP BY entity_name
			) AS all_risks
		)
		SELECT w.workload_type, w.workload_name, w.service_account_name, rp.risk_reason
		FROM rufus.workload_identities w
		INNER JOIN risky_permissions rp ON w.service_account_name = rp.entity_name
		ORDER BY w.workload_type, w.workload_name
		`
		workloadRows, err := DB.Query(workloadQuery)
		if err != nil {
			fmt.Printf("Error querying workload database: %v\n", err)
			return
		}
		defer workloadRows.Close()
		for workloadRows.Next() {
			var (
				workloadType       string
				workloadName       string
				serviceAccountName string
				riskReason         string
			)
			err := workloadRows.Scan(&workloadType, &workloadName, &serviceAccountName, &riskReason)
			if err != nil {
				fmt.Printf("Error scanning workload row: %v\n", err)
				continue
			}
			row := map[string]interface{}{
				"workload_type":        workloadType,
				"workload_name":        workloadName,
				"service_account_name": serviceAccountName,
				"risk_reason":          strings.ToUpper(riskReason),
			}
			workloads = append(workloads, row)
		}
		if err = workloadRows.Err(); err != nil {
			fmt.Printf("Error iterating over workload rows: %v\n", err)
		}
	}
	report["workloads_with_risky_permissions"] = workloads

	// Section 3: Roles where all permissions are unused
	unusedRoles := []map[string]interface{}{}
	rolesQuery := `
		SELECT 
			permission_source AS rbac_object,
			permission_source_type AS rbac_type,
			COUNT(*) AS unused_permission_count
		FROM permission
		WHERE (last_used_time IS NULL OR last_used_time < (NOW() - INTERVAL 7 DAY))
		  AND permission_source_type IN ('Role', 'ClusterRole')
		  AND permission_source NOT IN (
			  SELECT permission_source
			  FROM permission
			  WHERE last_used_time >= (NOW() - INTERVAL 7 DAY)
				AND permission_source_type IN ('Role', 'ClusterRole')
		  )
		GROUP BY permission_source, permission_source_type
		ORDER BY unused_permission_count DESC
	`
	rolesRows, err := DB.Query(rolesQuery)
	if err != nil {
		fmt.Printf("Error querying unused roles: %v\n", err)
		return
	}
	defer rolesRows.Close()
	for rolesRows.Next() {
		var rbacObject, rbacType string
		var unusedCount int
		err := rolesRows.Scan(&rbacObject, &rbacType, &unusedCount)
		if err != nil {
			fmt.Printf("Error scanning unused roles row: %v\n", err)
			continue
		}
		row := map[string]interface{}{
			"role_name":               rbacObject,
			"role_type":               rbacType,
			"unused_permission_count": unusedCount,
		}
		unusedRoles = append(unusedRoles, row)
	}
	if err = rolesRows.Err(); err != nil {
		fmt.Printf("Error iterating over unused roles rows: %v\n", err)
	}
	report["unused_roles"] = unusedRoles

	// Section 4: Bindings where all permissions are unused
	unusedBindings := []map[string]interface{}{}
	bindingsQuery := `
		SELECT 
			permission_binding AS rbac_object,
			permission_binding_type AS rbac_type,
			COUNT(*) AS unused_permission_count
		FROM permission
		WHERE (last_used_time IS NULL OR last_used_time < (NOW() - INTERVAL 7 DAY))
		  AND permission_binding_type IN ('RoleBinding', 'ClusterRoleBinding')
		  AND permission_binding NOT IN (
			  SELECT permission_binding
			  FROM permission
			  WHERE last_used_time >= (NOW() - INTERVAL 7 DAY)
				AND permission_binding_type IN ('RoleBinding', 'ClusterRoleBinding')
		  )
		GROUP BY permission_binding, permission_binding_type
		ORDER BY unused_permission_count DESC
	`
	bindingsRows, err := DB.Query(bindingsQuery)
	if err != nil {
		fmt.Printf("Error querying unused bindings: %v\n", err)
		return
	}
	defer bindingsRows.Close()
	for bindingsRows.Next() {
		var rbacObject, rbacType string
		var unusedCount int
		err := bindingsRows.Scan(&rbacObject, &rbacType, &unusedCount)
		if err != nil {
			fmt.Printf("Error scanning unused bindings row: %v\n", err)
			continue
		}
		row := map[string]interface{}{
			"binding_name":            rbacObject,
			"binding_type":            rbacType,
			"unused_permission_count": unusedCount,
		}
		unusedBindings = append(unusedBindings, row)
	}
	if err = bindingsRows.Err(); err != nil {
		fmt.Printf("Error iterating over unused bindings rows: %v\n", err)
	}
	report["unused_bindings"] = unusedBindings

	// Write JSON to file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(report)
	if err != nil {
		fmt.Printf("Error writing JSON report: %v\n", err)
		return
	}

	// Print notice to screen
	fmt.Println("\n\033[31mNOTICE: Unused permissions observed in the ingestion timeframe are shown with a last used time. Unused Permissions not observed are shown without. Explore the database for more information.\033[0m")
}
