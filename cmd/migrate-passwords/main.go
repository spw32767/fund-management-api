// cmd/migrate-passwords/main.go
// Enhanced migration script with better error handling and logging
package main

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"log"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	log.Println("🔐 Starting password migration...")

	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize database
	config.InitDB()

	// Check database connection
	if err := checkDatabaseConnection(); err != nil {
		log.Fatal("❌ Database connection failed:", err)
	}

	// Hash existing passwords
	if err := hashExistingPasswords(); err != nil {
		log.Printf("⚠️ Warning: Failed to hash some existing passwords: %v", err)
	}

	// Add sample users if needed
	if err := addSampleUsers(); err != nil {
		log.Printf("⚠️ Warning: Failed to add some sample users: %v", err)
	}

	// Show final status
	showFinalStatus()

	log.Println("✅ Password migration completed!")
}

func checkDatabaseConnection() error {
	log.Println("🔗 Checking database connection...")

	// Test connection
	sqlDB, err := config.DB.DB()
	if err != nil {
		return err
	}

	if err := sqlDB.Ping(); err != nil {
		return err
	}

	// Check if users table exists
	var tableExists bool
	if err := config.DB.Raw("SELECT COUNT(*) > 0 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'users'").Scan(&tableExists).Error; err != nil {
		return err
	}

	if !tableExists {
		log.Println("❌ Users table does not exist!")
		return err
	}

	log.Println("✅ Database connection OK")
	return nil
}

func hashExistingPasswords() error {
	log.Println("📝 Checking existing user passwords...")

	// Get all users
	var users []models.User
	if err := config.DB.Find(&users).Error; err != nil {
		return err
	}

	if len(users) == 0 {
		log.Println("📝 No existing users found in database")
		return nil
	}

	log.Printf("📝 Found %d existing users", len(users))

	successCount := 0
	skipCount := 0
	errorCount := 0

	for _, user := range users {
		// Skip if already hashed (bcrypt hashes start with $2)
		if strings.HasPrefix(user.Password, "$2") {
			log.Printf("✓ User %s (ID: %d) already has hashed password, skipping", user.Email, user.UserID)
			skipCount++
			continue
		}

		// Hash password
		hashedPassword, err := utils.HashPassword(user.Password)
		if err != nil {
			log.Printf("❌ Failed to hash password for user %s (ID: %d): %v", user.Email, user.UserID, err)
			errorCount++
			continue
		}

		// Update in database
		now := time.Now()
		if err := config.DB.Model(&user).Updates(map[string]interface{}{
			"password":  hashedPassword,
			"update_at": &now,
		}).Error; err != nil {
			log.Printf("❌ Failed to update password for user %s (ID: %d): %v", user.Email, user.UserID, err)
			errorCount++
			continue
		}

		log.Printf("✅ Successfully updated password for user %s (ID: %d)", user.Email, user.UserID)
		successCount++
	}

	log.Printf("📊 Password update summary: %d updated, %d skipped, %d errors", successCount, skipCount, errorCount)
	return nil
}

func addSampleUsers() error {
	log.Println("👥 Adding sample users...")

	sampleUsers := []struct {
		Email      string
		Password   string
		FirstName  string
		LastName   string
		Gender     string
		RoleID     int
		PositionID int
	}{
		{
			Email:      "admin@cpkku.ac.th",
			Password:   "Admin123!",
			FirstName:  "ผู้ดูแล",
			LastName:   "ระบบ",
			Gender:     "male",
			RoleID:     3, // admin
			PositionID: 3, // พนักงานธุรการ
		},
		{
			Email:      "teacher@cpkku.ac.th",
			Password:   "Teacher123!",
			FirstName:  "สมชาย",
			LastName:   "ใจดี",
			Gender:     "male",
			RoleID:     1, // teacher
			PositionID: 1, // อาจารย์
		},
		{
			Email:      "teacher2@cpkku.ac.th",
			Password:   "Teacher123!",
			FirstName:  "สมหญิง",
			LastName:   "รักการศึกษา",
			Gender:     "female",
			RoleID:     1, // teacher
			PositionID: 2, // รองศาสตราจารย์
		},
		{
			Email:      "staff@cpkku.ac.th",
			Password:   "Staff123!",
			FirstName:  "สุดา",
			LastName:   "ช่วยเหลือ",
			Gender:     "female",
			RoleID:     2, // staff
			PositionID: 3, // พนักงานธุรการ
		},
	}

	successCount := 0
	skipCount := 0
	errorCount := 0

	for _, userData := range sampleUsers {
		// Check if user exists
		var existingUser models.User
		if err := config.DB.Where("email = ?", userData.Email).First(&existingUser).Error; err == nil {
			log.Printf("✓ User %s already exists (ID: %d), skipping", userData.Email, existingUser.UserID)
			skipCount++
			continue
		}

		// Hash password
		hashedPassword, err := utils.HashPassword(userData.Password)
		if err != nil {
			log.Printf("❌ Failed to hash password for %s: %v", userData.Email, err)
			errorCount++
			continue
		}

		// Create user (without specifying user_id, let AUTO_INCREMENT handle it)
		now := time.Now()
		user := models.User{
			UserFname:  userData.FirstName,
			UserLname:  userData.LastName,
			Gender:     userData.Gender,
			Email:      userData.Email,
			Password:   hashedPassword,
			RoleID:     userData.RoleID,
			PositionID: userData.PositionID,
			CreateAt:   &now,
			UpdateAt:   &now,
		}

		if err := config.DB.Create(&user).Error; err != nil {
			log.Printf("❌ Failed to create user %s: %v", userData.Email, err)
			if strings.Contains(err.Error(), "user_id") {
				log.Printf("💡 Hint: Make sure user_id column is set to AUTO_INCREMENT in database")
				log.Printf("💡 Run: ALTER TABLE users MODIFY COLUMN user_id INT AUTO_INCREMENT;")
			}
			errorCount++
			continue
		}

		log.Printf("✅ Created user %s (ID: %d) - %s %s with role ID %d",
			userData.Email, user.UserID, userData.FirstName, userData.LastName, userData.RoleID)
		successCount++
	}

	log.Printf("📊 User creation summary: %d created, %d skipped, %d errors", successCount, skipCount, errorCount)
	return nil
}

func showFinalStatus() {
	log.Println("\n📋 Final Database Status:")
	log.Println("==========================")

	// Count users by role
	var totalUsers int64
	var adminCount int64
	var teacherCount int64
	var staffCount int64

	config.DB.Model(&models.User{}).Where("delete_at IS NULL").Count(&totalUsers)
	config.DB.Model(&models.User{}).Where("role_id = ? AND delete_at IS NULL", 3).Count(&adminCount)
	config.DB.Model(&models.User{}).Where("role_id = ? AND delete_at IS NULL", 1).Count(&teacherCount)
	config.DB.Model(&models.User{}).Where("role_id = ? AND delete_at IS NULL", 2).Count(&staffCount)

	log.Printf("👥 Total users: %d", totalUsers)
	log.Printf("   - Admins: %d", adminCount)
	log.Printf("   - Teachers: %d", teacherCount)
	log.Printf("   - Staff: %d", staffCount)

	// Show sample credentials if we have them
	if totalUsers > 0 {
		log.Println("\n🔑 Sample Login Credentials:")
		log.Println("----------------------------")

		// Check which sample users exist
		sampleEmails := []string{"admin@cpkku.ac.th", "teacher@cpkku.ac.th", "teacher2@cpkku.ac.th", "staff@cpkku.ac.th"}
		passwords := map[string]string{
			"admin@cpkku.ac.th":    "Admin123!",
			"teacher@cpkku.ac.th":  "Teacher123!",
			"teacher2@cpkku.ac.th": "Teacher123!",
			"staff@cpkku.ac.th":    "Staff123!",
		}

		for _, email := range sampleEmails {
			var user models.User
			if err := config.DB.Where("email = ? AND delete_at IS NULL", email).First(&user).Error; err == nil {
				log.Printf("📧 %s | 🔐 %s", email, passwords[email])
			}
		}
	}

	log.Println("==========================")
}
