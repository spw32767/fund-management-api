package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() {
	var err error

	// Get database credentials from environment variables
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbDatabase := os.Getenv("DB_DATABASE")
	dbUsername := os.Getenv("DB_USERNAME")
	dbPassword := os.Getenv("DB_PASSWORD")

	// Create DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbUsername,
		dbPassword,
		dbHost,
		dbPort,
		dbDatabase,
	)

	// Configure GORM
	environment := strings.ToLower(os.Getenv("ENVIRONMENT"))
	debugSQL := strings.ToLower(os.Getenv("DEBUG_SQL"))

	// In production, suppress SQL logs unless explicitly re-enabled via DEBUG_SQL=true.
	// Switch the level back to logger.Info to print SQL statements again.
	logLevel := logger.Info
	if environment == "production" && debugSQL != "true" {
		logLevel = logger.Warn
	}

	config := &gorm.Config{
		Logger: logger.New(
			log.New(LogWriter, "\r\n", log.LstdFlags),
			logger.Config{LogLevel: logLevel},
		),
	}

	// Connect to database
	DB, err = gorm.Open(mysql.Open(dsn), config)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Configure the connection pool so idle connections are recycled before
	// MySQL/MariaDB drops them (prevents "connection was aborted" errors when a
	// dead pooled connection gets reused).
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatal("Failed to get database instance:", err)
	}
	sqlDB.SetConnMaxLifetime(time.Minute * 3) // recycle before MySQL wait_timeout
	sqlDB.SetConnMaxIdleTime(time.Minute * 1)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	log.Println("Database connected successfully")
}