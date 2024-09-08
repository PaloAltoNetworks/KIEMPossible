package log_parsing

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

func getEntityNameAndType(username string) (string, string) {
	if strings.HasPrefix(username, "system:serviceaccount:") {
		return strings.TrimPrefix(username, "system:serviceaccount:"), "ServiceAccount"
	}
	return username, "User"
}

func getAPIGroup(apiGroup, apiVersion string) string {
	if apiGroup == "" {
		return apiVersion
	}
	return apiGroup + "/" + apiVersion
}

func getResourceType(Resource, Subresource string) string {
	resourceType := Resource
	if Subresource != "" {
		resourceType = fmt.Sprintf("%s/%s", Resource, Subresource)
	}
	return resourceType
}

func getPermissionScope(namespace, name string) string {
	if namespace != "" && name != "" {
		return namespace + "/" + name
	} else if name != "" {
		return "cluster-wide" + "/" + name
	} else if namespace != "" {
		return namespace
	} else {
		return "cluster-wide"
	}
}

func getLastUsedTime(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func getLastUsedResource(namespace, resource, name string) string {
	if namespace != "" && name != "" {
		return namespace + "/" + resource + "/" + name
	} else if name != "" {
		return resource + "/" + name
	} else if namespace != "" {
		return namespace + "/" + resource
	} else {
		return resource
	}
}

func updateDatabase(db *sql.DB, entityName, entityType, apiGroup, resourceType, verb, permissionScope, lastUsedTime, lastUsedResource string) {
	query := `
			UPDATE permission
    		SET last_used_time = ?, last_used_resource = ?
    		WHERE entity_name = ? AND entity_type = ? AND api_group = ? AND resource_type = ? AND verb = ? 
        		AND (last_used_time < ? OR last_used_time IS NULL)
        		AND (
            		permission_scope = ? OR
            		(permission_scope like SUBSTRING_INDEX(?, '/', 1) AND ? LIKE '%/%')
        	)
    `

	stmt, err := db.Prepare(query)
	if err != nil {
		fmt.Printf("Error preparing statement: %v\n", err)
		fmt.Printf("Query: %s\n", query)
		os.Exit(1)
	}
	defer stmt.Close()

	_, err = stmt.Exec(lastUsedTime, lastUsedResource, entityName, entityType, apiGroup, resourceType, verb, lastUsedTime, permissionScope, permissionScope, permissionScope)
	if err != nil {
		fmt.Printf("Error updating database: %v\n", err)
		os.Exit(1)
	}
}

type PermissionRow struct {
	entity_name             string
	entity_type             string
	api_group               string
	resource_type           string
	verb                    string
	permission_scope        string
	permission_source       string
	permission_source_type  string
	permission_binding      string
	permission_binding_type string
	last_used_time          sql.NullTime
	last_used_resource      sql.NullString
}

func insertInheritedPermissionRow(db *sql.DB, row PermissionRow) error {
	query := `
        INSERT IGNORE INTO permission (
            entity_name, entity_type, api_group, resource_type, verb, permission_scope,
            permission_source, permission_source_type, permission_binding, permission_binding_type,
            last_used_time, last_used_resource
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `

	_, err := db.Exec(query,
		row.entity_name, row.entity_type, row.api_group, row.resource_type, row.verb, row.permission_scope,
		row.permission_source, row.permission_source_type, row.permission_binding, row.permission_binding_type,
		row.last_used_time, row.last_used_resource,
	)

	return err
}

func handleGroupInheritance(db *sql.DB, username string, groups []string) {
	var rowData []PermissionRow
	for _, group := range groups {
		rows, err := db.Query(`
                SELECT entity_name, entity_type, api_group, resource_type, verb, permission_scope,
                       permission_source, permission_source_type, permission_binding, permission_binding_type,
                       last_used_time, last_used_resource
                FROM permission
                WHERE entity_name = ?
            `, group)
		if err != nil {
			fmt.Printf("Error querying database: %v\n", err)
			continue
		}
		defer rows.Close()

		for rows.Next() {
			var row PermissionRow
			err := rows.Scan(
				&row.entity_name,
				&row.entity_type,
				&row.api_group,
				&row.resource_type,
				&row.verb,
				&row.permission_scope,
				&row.permission_source,
				&row.permission_source_type,
				&row.permission_binding,
				&row.permission_binding_type,
				&row.last_used_time,
				&row.last_used_resource,
			)
			if err != nil {
				fmt.Printf("Error scanning row: %v\n", err)
				continue
			}

			row.entity_name = username
			if strings.HasPrefix(username, "system:serviceaccount:") {
				row.entity_type = "ServiceAccount"
			} else {
				row.entity_type = "User"
			}
			row.permission_source = group
			row.permission_source_type = "Group"

			rowData = append(rowData, row)
		}
	}

	for _, row := range rowData {
		err := insertInheritedPermissionRow(db, row)
		if err != nil {
			fmt.Printf("Error inserting inherited permission row: %v\n", err)
		}
	}
}
