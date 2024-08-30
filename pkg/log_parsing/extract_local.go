package log_parsing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

func ExtractLocalLogs(logFile string) ([]auditv1.Event, error) {
	file, err := os.Open(logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %v", err)
	}

	var events []auditv1.Event
	for _, line := range parseLines(data) {
		event := auditv1.Event{}
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

func parseLines(data []byte) [][]byte {
	var lines [][]byte
	var line []byte
	for _, b := range data {
		if b == '\n' {
			lines = append(lines, line)
			line = nil
			continue
		}
		line = append(line, b)
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	return lines
}

func HandleLocalLogs(logEvents []auditv1.Event, db *sql.DB) {
	for _, event := range logEvents {
		fmt.Printf("Event: %v\n", event)
	}
	// Implement logic to handle the log events in the database
	// ...
}
