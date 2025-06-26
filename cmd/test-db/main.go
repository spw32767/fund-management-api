package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Get database credentials
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbDatabase := os.Getenv("DB_DATABASE")
	dbUsername := os.Getenv("DB_USERNAME")
	dbPassword := os.Getenv("DB_PASSWORD")

	// Show connection info
	fmt.Println("=== Database Connection Test ===")
	fmt.Printf("Host: %s\n", dbHost)
	fmt.Printf("Port: %s\n", dbPort)
	fmt.Printf("Database: %s\n", dbDatabase)
	fmt.Printf("Username: %s\n", dbUsername)
	fmt.Printf("Password: %s\n", dbPassword)
	fmt.Println("================================")

	// Create DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbUsername,
		dbPassword,
		dbHost,
		dbPort,
		dbDatabase,
	)

	// Try to connect
	fmt.Println("\nConnecting to database...")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("❌ Failed to connect to database:", err)
	}

	fmt.Println("✅ Successfully connected to database!")

	// Test query
	var count int64
	db.Table("users").Count(&count)
	fmt.Printf("\nNumber of users in database: %d\n", count)

	// Check tables
	var tables []string
	db.Raw("SHOW TABLES").Scan(&tables)
	fmt.Printf("\nTables in database (%d total):\n", len(tables))
	for i, table := range tables {
		fmt.Printf("%d. %s\n", i+1, table)
	}
}

// This program connects to a MySQL database using GORM and prints connection details,
// performs a simple query to count users, and lists all tables in the database.
