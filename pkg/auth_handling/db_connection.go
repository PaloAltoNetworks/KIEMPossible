package auth_handling

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

// Establish DB connection, default creds below
func DBConnect() (*sql.DB, error) {
	dbUser := "mysql"
	dbPass := "mysql"
	dbName := "rufus"

	dsn := fmt.Sprintf("%s:%s@tcp(localhost:3306)/%s", dbUser, dbPass, dbName)

	DB, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Println("Error opening database connection:", err)
		return nil, err
	}

	err = DB.Ping()
	if err != nil {
		fmt.Println("Error pinging database:", err)
		return nil, err
	}

	return DB, err
}

func ClearDatabase(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM rufus.permission")
	if err != nil {
		return fmt.Errorf("failed to clear table rufus.permission: %v", err)
	}

	_, err = tx.Exec("DELETE FROM rufus.workload_identities")
	if err != nil {
		return fmt.Errorf("failed to clear table rufus.workload_identities: %v", err)
	}

	_, err = tx.Exec("ALTER TABLE rufus.permission AUTO_INCREMENT = 1")
	if err != nil {
		return fmt.Errorf("failed to reset AUTO_INCREMENT: %v", err)
	}

	_, err = tx.Exec("ALTER TABLE rufus.workload_identities AUTO_INCREMENT = 1")
	if err != nil {
		return fmt.Errorf("failed to reset AUTO_INCREMENT: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}
	return nil
}
