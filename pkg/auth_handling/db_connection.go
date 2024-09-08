package auth_handling

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

func DBConnect() (*sql.DB, error) {
	dbUser := "mysql"
	dbPass := "mysql"
	dbName := "clusterlogo"

	dsn := fmt.Sprintf("%s:%s@tcp(localhost:3306)/%s", dbUser, dbPass, dbName)

	DB, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Println("Error opening database connection:", err)
		return nil, err
	}

	// Ping the database to verify the connection
	err = DB.Ping()
	if err != nil {
		fmt.Println("Error pinging database:", err)
		return nil, err
	}

	fmt.Println("Connected to MySQL database successfully!")

	return DB, err
}
