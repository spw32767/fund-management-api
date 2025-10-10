package main

import (
	"errors"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	log.Println("🗂  Starting targeted user folder provisioning...")

	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, falling back to environment variables")
	}

	config.InitDB()

	targetUserIDs := []int{} // 👈 Populate with the user_id values you want to provision, e.g. []int{1, 2, 3}

	if len(targetUserIDs) == 0 {
		log.Fatal("targetUserIDs is empty. Please add at least one user_id before running this command.")
	}

	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	if err := os.MkdirAll(filepath.Join(uploadPath, "users"), 0755); err != nil {
		log.Fatalf("failed to prepare base upload directory: %v", err)
	}

	var (
		succeeded int
		failed    []string
	)

	for _, targetUserID := range targetUserIDs {
		log.Printf("➡️  Provisioning user_id=%d", targetUserID)

		var user models.User
		if err := config.DB.First(&user, "user_id = ?", targetUserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("❌ no user found with user_id %d", targetUserID)
				failed = append(failed, formatFailureLabel(targetUserID, "record not found"))
			} else {
				log.Printf("❌ failed to query user_id %d: %v", targetUserID, err)
				failed = append(failed, formatFailureLabel(targetUserID, err.Error()))
			}
			continue
		}

		folderPath := filepath.Join(uploadPath, "users", utils.GetUserFolderName(user))

		alreadyExists := false
		if info, err := os.Stat(folderPath); err == nil && info.IsDir() {
			alreadyExists = true
		}

		folderPath, err := utils.CreateUserFolderIfNotExists(user, uploadPath)
		if err != nil {
			log.Printf("❌ failed to create folder structure for user_id %d: %v", targetUserID, err)
			failed = append(failed, formatFailureLabel(targetUserID, err.Error()))
			continue
		}

		if alreadyExists {
			log.Printf("ℹ️  User folder already existed, ensured structure at %s", folderPath)
		} else {
			log.Printf("✅ User folder created at %s", folderPath)
		}
		succeeded++
	}

	if len(failed) > 0 {
		log.Fatalf("completed with errors. successful: %d, failed: %s", succeeded, strings.Join(failed, ", "))
	}

	log.Printf("🎉 Successfully provisioned %d user(s)", succeeded)
}

func formatFailureLabel(userID int, reason string) string {
	return "user_id=" + strconv.Itoa(userID) + " (" + reason + ")"
}
