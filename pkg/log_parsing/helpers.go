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
