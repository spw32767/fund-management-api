// Migration script to hash existing passwords
// cmd/migrate-passwords/main.go
package main

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"log"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize database
	config.InitDB()

	// Get all users
	var users []models.User
	if err := config.DB.Find(&users).Error; err != nil {
		log.Fatal("Failed to fetch users:", err)
	}

	// Update passwords
	for _, user := range users {
		// Skip if already hashed (bcrypt hashes start with $2)
		if strings.HasPrefix(user.Password, "$2") {
			log.Printf("User %s already has hashed password, skipping\n", user.Email)
			continue
		}

		// Hash password
		hashedPassword, err := utils.HashPassword(user.Password)
		if err != nil {
			log.Printf("Failed to hash password for user %s: %v\n", user.Email, err)
			continue
		}

		// Update in database
		if err := config.DB.Model(&user).Update("password", hashedPassword).Error; err != nil {
			log.Printf("Failed to update password for user %s: %v\n", user.Email, err)
			continue
		}

		log.Printf("Successfully updated password for user %s\n", user.Email)
	}

	log.Println("Password migration completed!")
}
