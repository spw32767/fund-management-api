// controllers/admin.go
package controllers

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ListUserFolders สำหรับ admin ดู user folders ทั้งหมด
func ListUserFolders(c *gin.Context) {
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	usersPath := filepath.Join(uploadPath, "users")

	// ตรวจสอบว่าโฟลเดอร์ users มีอยู่หรือไม่
	if _, err := os.Stat(usersPath); os.IsNotExist(err) {
		c.JSON(http.StatusOK, gin.H{
			"folders": []gin.H{},
			"total":   0,
			"message": "No user folders found",
		})
		return
	}

	folders, err := os.ReadDir(usersPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot read users directory"})
		return
	}

	var userFolders []gin.H
	for _, folder := range folders {
		if folder.IsDir() {
			info, _ := folder.Info()

			// นับจำนวนไฟล์ในโฟลเดอร์
			folderPath := filepath.Join(usersPath, folder.Name())
			fileCount := countFilesInDirectory(folderPath)

			// คำนวณขนาดโฟลเดอร์
			folderSize := calculateDirectorySize(folderPath)

			userFolders = append(userFolders, gin.H{
				"name":        folder.Name(),
				"modified_at": info.ModTime(),
				"path":        filepath.Join("users", folder.Name()),
				"file_count":  fileCount,
				"size_mb":     fmt.Sprintf("%.2f", float64(folderSize)/(1024*1024)),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"folders": userFolders,
		"total":   len(userFolders),
	})
}

// ListUserFiles สำหรับ admin ดูไฟล์ของ user คนใดคนหนึ่ง
func ListUserFiles(c *gin.Context) {
	userID := c.Param("id")

	// ดึงข้อมูล user
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	// สร้าง user folder path
	userFolderName := utils.GetUserFolderName(user)
	userFolderPath := filepath.Join(uploadPath, "users", userFolderName)

	// ตรวจสอบว่าโฟลเดอร์มีอยู่หรือไม่
	if _, err := os.Stat(userFolderPath); os.IsNotExist(err) {
		c.JSON(http.StatusOK, gin.H{
			"user":    user,
			"files":   []gin.H{},
			"total":   0,
			"message": "User folder not found",
		})
		return
	}

	// ดึงไฟล์ทั้งหมดของ user จากฐานข้อมูล
	var files []models.FileUpload
	if err := config.DB.Where("uploaded_by = ? AND delete_at IS NULL", userID).
		Order("uploaded_at DESC").Find(&files).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user files"})
		return
	}

	// จัดกลุ่มไฟล์ตาม folder
	fileGroups := make(map[string][]gin.H)

	for _, file := range files {
		var folder string
		if file.IsInTempFolder() {
			folder = "temp"
		} else if file.IsInSubmissionFolder() {
			folder = file.GetSubmissionFolder()
			if folder == "" {
				folder = "submissions"
			}
		} else {
			folder = "other"
		}

		fileInfo := gin.H{
			"file_id":       file.FileID,
			"original_name": file.OriginalName,
			"stored_path":   file.GetRelativePath(),
			"file_size":     file.GetFormattedFileSize(),
			"mime_type":     file.MimeType,
			"uploaded_at":   file.UploadedAt,
			"exists":        file.FileExists(),
		}

		fileGroups[folder] = append(fileGroups[folder], fileInfo)
	}

	c.JSON(http.StatusOK, gin.H{
		"user":        user,
		"file_groups": fileGroups,
		"total":       len(files),
	})
}

// GetFileStats สำหรับดูสถิติการใช้งานไฟล์
func GetFileStats(c *gin.Context) {
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	// สถิติจากฐานข้อมูล
	var totalFiles int64
	var totalSize int64
	var activeUsers int64

	config.DB.Model(&models.FileUpload{}).Where("delete_at IS NULL").Count(&totalFiles)
	config.DB.Model(&models.FileUpload{}).Where("delete_at IS NULL").Select("SUM(file_size)").Scan(&totalSize)
	config.DB.Model(&models.FileUpload{}).Where("delete_at IS NULL").
		Distinct("uploaded_by").Count(&activeUsers)

	// สถิติตามประเภทไฟล์
	var fileTypeStats []struct {
		MimeType  string `json:"mime_type"`
		Count     int64  `json:"count"`
		TotalSize int64  `json:"total_size"`
	}

	config.DB.Model(&models.FileUpload{}).
		Where("delete_at IS NULL").
		Select("mime_type, COUNT(*) as count, SUM(file_size) as total_size").
		Group("mime_type").
		Order("count DESC").
		Limit(10).
		Find(&fileTypeStats)

	// สถิติการอัปโหลดรายเดือน
	var monthlyStats []struct {
		Month string `json:"month"`
		Count int64  `json:"count"`
	}

	config.DB.Raw(`
		SELECT DATE_FORMAT(uploaded_at, '%Y-%m') as month, COUNT(*) as count 
		FROM file_uploads 
		WHERE delete_at IS NULL AND uploaded_at >= DATE_SUB(NOW(), INTERVAL 12 MONTH)
		GROUP BY DATE_FORMAT(uploaded_at, '%Y-%m')
		ORDER BY month DESC
	`).Scan(&monthlyStats)

	// สถิติพื้นที่ดิสก์จริง
	totalDiskSize := calculateDirectorySize(uploadPath)

	c.JSON(http.StatusOK, gin.H{
		"total_files":     totalFiles,
		"total_size_mb":   fmt.Sprintf("%.2f", float64(totalSize)/(1024*1024)),
		"total_disk_mb":   fmt.Sprintf("%.2f", float64(totalDiskSize)/(1024*1024)),
		"active_users":    activeUsers,
		"file_type_stats": fileTypeStats,
		"monthly_stats":   monthlyStats,
		"upload_path":     uploadPath,
	})
}

// CleanupTempFiles ลบไฟล์ temp เก่า
func CleanupTempFiles(c *gin.Context) {
	// รับพารามิเตอร์จำนวนวันที่เก่า (default 7 วัน)
	daysOld := 7
	if days := c.Query("days"); days != "" {
		if d, err := strconv.Atoi(days); err == nil && d > 0 {
			daysOld = d
		}
	}

	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	deletedCount, err := utils.CleanupTempFiles(uploadPath, daysOld)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cleanup failed: " + err.Error()})
		return
	}

	// อัพเดทฐานข้อมูล - ลบ record ของไฟล์ที่ถูกลบ
	var orphanedFiles []models.FileUpload
	config.DB.Where("stored_path LIKE ? AND delete_at IS NULL", "%/temp/%").Find(&orphanedFiles)

	orphanedCount := 0
	for _, file := range orphanedFiles {
		if !file.FileExists() {
			now := time.Now()
			file.DeleteAt = &now
			config.DB.Save(&file)
			orphanedCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            "Cleanup completed successfully",
		"deleted_files":      deletedCount,
		"cleaned_db_records": orphanedCount,
		"days_old":           daysOld,
	})
}

// BackupUserData สำหรับ backup ข้อมูลของ user
func BackupUserData(c *gin.Context) {
	userID := c.Param("id")

	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	// สร้างโฟลเดอร์ backup
	backupPath := filepath.Join(uploadPath, "system", "backups")
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create backup directory"})
		return
	}

	// สร้างชื่อไฟล์ backup
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	backupFileName := fmt.Sprintf("user_%d_%s.zip", user.UserID, timestamp)

	// ในการใช้งานจริง จะต้องใช้ library สำหรับสร้าง ZIP
	// เช่น archive/zip package

	c.JSON(http.StatusOK, gin.H{
		"message":     "Backup process initiated",
		"backup_file": backupFileName,
		"backup_path": filepath.Join(backupPath, backupFileName),
		"user":        user,
	})
}

// ===== Helper Functions =====

// countFilesInDirectory นับจำนวนไฟล์ในโฟลเดอร์
func countFilesInDirectory(dirPath string) int {
	count := 0
	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}

// calculateDirectorySize คำนวณขนาดโฟลเดอร์
func calculateDirectorySize(dirPath string) int64 {
	var size int64
	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
