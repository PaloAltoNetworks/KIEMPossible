package log_parsing

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

var fileMutex sync.Mutex
var sessionMutex sync.Mutex
var sessionCond = sync.NewCond(&sessionMutex)
var sessionRef *session.Session

// Functions to normalize data from the logs
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

type UpdateData struct {
	EntityName       string
	EntityType       string
	APIGroup         string
	ResourceType     string
	Verb             string
	PermissionScope  string
	LastUsedTime     string
	LastUsedResource string
}

// Update DB in batches
func batchUpdateDatabase(db *sql.DB, updateDataList []UpdateData) {
	const batchSize = 10000
	totalBatches := (len(updateDataList) + batchSize - 1) / batchSize

	for i := 0; i < totalBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > len(updateDataList) {
			end = len(updateDataList)
		}

		batch := updateDataList[start:end]
		tx, err := db.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			return
		}

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

		stmt, err := tx.Prepare(query)
		if err != nil {
			fmt.Printf("Error preparing statement: %v\n", err)
			tx.Rollback()
			return
		}
		defer stmt.Close()

		for _, data := range batch {
			_, err = stmt.Exec(data.LastUsedTime, data.LastUsedResource, data.EntityName, data.EntityType, data.APIGroup, data.ResourceType, data.Verb, data.LastUsedTime, data.PermissionScope, data.PermissionScope, data.PermissionScope)
			if err != nil {
				fmt.Printf("Error executing batch update: %v\n", err)
				tx.Rollback()
				return
			}
		}

		err = tx.Commit()
		if err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			tx.Rollback()
			return
		}
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

// Handle situations where user not in DB, but group from claim is (taken from log) - add group permissions to user (inheritance)
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

// Update DB with inherited permissions
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

// Writes logs to temp file
func writeLogsToTempFile(writer *bufio.Writer, logEvents interface{}) error {
	fileMutex.Lock() // Ensure only one goroutine writes at a time
	defer fileMutex.Unlock()

	switch events := logEvents.(type) {
	case []*cloudwatchlogs.FilteredLogEvent:
		for _, event := range events {
			data, err := json.Marshal(event)
			if err != nil {
				return fmt.Errorf("failed to serialize log event: %v", err)
			}
			_, err = writer.Write(append(data, '\n'))
			if err != nil {
				return fmt.Errorf("failed to write log event to temp file: %v", err)
			}
		}
	case azquery.LogsClientQueryWorkspaceResponse:
		for _, table := range events.Tables {
			GlobalProgressBar.Add(len(table.Rows))
			for _, row := range table.Rows {
				data, err := json.Marshal(row)
				if err != nil {
					return fmt.Errorf("failed to serialize log event: %v", err)
				}
				_, err = writer.Write(append(data, '\n'))
				if err != nil {
					return fmt.Errorf("failed to write log event to temp file: %v", err)
				}
			}
		}
	default:
		return fmt.Errorf("unsupported log event type")
	}

	return writer.Flush()
}

// Global progress bar
type ProgressBar struct {
	mu        sync.Mutex
	running   bool
	paused    bool
	count     int64
	start     time.Time
	message   string
	restart   time.Time
	totalTime time.Duration
}

var GlobalProgressBar ProgressBar

func (p *ProgressBar) Start(message string, startCount ...int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		p.running = true
		p.paused = false
		p.start = time.Now()
		p.restart = p.start
		p.message = message
		p.totalTime = 0
		if len(startCount) > 0 {
			p.count = startCount[0]
		} else {
			p.count = 0
		}

		go p.run()
	}
}

func (p *ProgressBar) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		p.running = false
		println()
		p.printFinished()
	}
}

func (p *ProgressBar) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running && !p.paused {
		p.paused = true
	}
}

func (p *ProgressBar) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running && p.paused {
		p.paused = false
	}
}

func (p *ProgressBar) Add(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count += int64(n)
}

func (p *ProgressBar) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		p.mu.Lock()
		if !p.running {
			p.mu.Unlock()
			return
		}
		if !p.paused {
			p.printProgress()
		}
		p.mu.Unlock()
	}
}

func (p *ProgressBar) printProgress() {
	elapsed := time.Since(p.start)
	restartElapsed := time.Since(p.restart)
	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60

	restartHours := int(restartElapsed.Hours())
	restartMinutes := int(restartElapsed.Minutes()) % 60
	restartSeconds := int(restartElapsed.Seconds()) % 60

	fmt.Printf("Progress: %d %s(Total Time: %02d:%02d:%02d, Since last pause: %02d:%02d:%02d)\r", p.count, p.message, hours, minutes, seconds, restartHours, restartMinutes, restartSeconds)
}

func (p *ProgressBar) printFinished() {
	elapsed := time.Since(p.start)
	restartElapsed := time.Since(p.restart)
	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60

	restartHours := int(restartElapsed.Hours())
	restartMinutes := int(restartElapsed.Minutes()) % 60
	restartSeconds := int(restartElapsed.Seconds()) % 60

	fmt.Printf("Total stats: %d %s(Total Time: %02d:%02d:%02d, Since last pause: %02d:%02d:%02d)\n", p.count, p.message, hours, minutes, seconds, restartHours, restartMinutes, restartSeconds)
}
