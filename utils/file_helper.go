// utils/file_helper.go
package utils

import (
	"fmt"
	"fund-management-api/models"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// CreateUserFolderIfNotExists สร้างโฟลเดอร์ user ถ้ายังไม่มี
func CreateUserFolderIfNotExists(user models.User, uploadPath string) (string, error) {
	// สร้างชื่อโฟลเดอร์: user_{id}_{firstname}_{lastname}
	folderName := fmt.Sprintf("user_%d_%s_%s",
		user.UserID,
		SanitizeForFilename(user.UserFname),
		SanitizeForFilename(user.UserLname))

	userFolderPath := filepath.Join(uploadPath, "users", folderName)

	// สร้างโฟลเดอร์หลัก
	if err := os.MkdirAll(userFolderPath, 0755); err != nil {
		return "", err
	}

	// สร้างโฟลเดอร์ย่อย
	subFolders := []string{"temp", "submissions", "profile"}
	for _, subFolder := range subFolders {
		subPath := filepath.Join(userFolderPath, subFolder)
		if err := os.MkdirAll(subPath, 0755); err != nil {
			return "", err
		}
	}

	return userFolderPath, nil
}

// CreateSubmissionFolder สร้างโฟลเดอร์ submission
func CreateSubmissionFolder(userFolderPath string, submissionType string, submissionID int, createdDate time.Time) (string, error) {
	// สร้างชื่อโฟลเดอร์: {type}{id}_{YYYY-MM-DD}
	dateStr := createdDate.Format("2006-01-02")
	var folderName string

	// กำหนดชื่อตามประเภท
	switch submissionType {
	case "fund_application":
		folderName = fmt.Sprintf("fund%d_%s", submissionID, dateStr)
	case "publication_reward":
		folderName = fmt.Sprintf("pub%d_%s", submissionID, dateStr)
	default:
		folderName = fmt.Sprintf("sub%d_%s", submissionID, dateStr)
	}

	submissionPath := filepath.Join(userFolderPath, "submissions", folderName)

	// สร้างโฟลเดอร์
	if err := os.MkdirAll(submissionPath, 0755); err != nil {
		return "", err
	}

	return submissionPath, nil
}

// SanitizeForFilename ทำความสะอาดชื่อไฟล์
func SanitizeForFilename(filename string) string {
	// แทนที่อักขระพิเศษที่ไม่ปลอดภัย
	reg := regexp.MustCompile(`[<>:"/\\|?*]`)
	filename = reg.ReplaceAllString(filename, "_")

	// แทนที่ช่องว่างที่ต่อเนื่องด้วย underscore
	reg = regexp.MustCompile(`\s+`)
	filename = reg.ReplaceAllString(filename, "_")

	// ลบอักขระที่ไม่ปลอดภัยสำหรับ filesystem
	var result strings.Builder
	for _, r := range filename {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			result.WriteRune(r)
		} else if unicode.IsSpace(r) {
			result.WriteRune('_')
		}
	}

	cleanName := strings.TrimSpace(result.String())

	// จำกัดความยาว
	if len(cleanName) > 100 {
		ext := filepath.Ext(cleanName)
		nameWithoutExt := cleanName[:len(cleanName)-len(ext)]
		cleanName = nameWithoutExt[:100-len(ext)] + ext
	}

	return cleanName
}

// GenerateUniqueFilename สร้างชื่อไฟล์ที่ไม่ซ้ำในโฟลเดอร์
func GenerateUniqueFilename(dirPath, originalFilename string) string {
	filename := SanitizeForFilename(originalFilename)

	// ถ้าไฟล์ไม่ซ้ำ ใช้ชื่อเดิม
	if !FileExistsInDirectory(dirPath, filename) {
		return filename
	}

	// ถ้าไฟล์ซ้ำ เพิ่มหมายเลข
	ext := filepath.Ext(filename)
	nameWithoutExt := filename[:len(filename)-len(ext)]

	counter := 1
	for {
		newFilename := fmt.Sprintf("%s_%d%s", nameWithoutExt, counter, ext)
		if !FileExistsInDirectory(dirPath, newFilename) {
			return newFilename
		}
		counter++
	}
}

// FileExistsInDirectory ตรวจสอบว่าไฟล์มีอยู่ในโฟลเดอร์หรือไม่
func FileExistsInDirectory(dirPath, filename string) bool {
	fullPath := filepath.Join(dirPath, filename)
	_, err := os.Stat(fullPath)
	return !os.IsNotExist(err)
}

// EnsureDirectoryExists สร้างโฟลเดอร์ถ้ายังไม่มี
func EnsureDirectoryExists(dirPath string) error {
	return os.MkdirAll(dirPath, 0755)
}

// MoveFileToSubmissionFolder ย้ายไฟล์จาก temp ไปยัง submission folder
func MoveFileToSubmissionFolder(currentPath, newPath string) error {
	// สร้างโฟลเดอร์ปลายทางถ้ายังไม่มี
	if err := EnsureDirectoryExists(filepath.Dir(newPath)); err != nil {
		return err
	}

	// ย้ายไฟล์
	return os.Rename(currentPath, newPath)
}

// GetUserFolderName สร้างชื่อโฟลเดอร์ user (ไม่สร้างจริง)
func GetUserFolderName(user models.User) string {
	return fmt.Sprintf("user_%d_%s_%s",
		user.UserID,
		SanitizeForFilename(user.UserFname),
		SanitizeForFilename(user.UserLname))
}

// GetFileSize คำนวณขนาดไฟล์ในหน่วย MB
func GetFileSize(filePath string) (float64, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}

	// แปลงเป็น MB
	sizeInMB := float64(fileInfo.Size()) / (1024 * 1024)
	return sizeInMB, nil
}

// CleanupTempFiles ลบไฟล์ temp ที่เก่าเกินกำหนด
func CleanupTempFiles(uploadPath string, olderThanDays int) (int, error) {
	deletedCount := 0
	usersPath := filepath.Join(uploadPath, "users")

	err := filepath.Walk(usersPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // ข้ามไฟล์ที่มีปัญหา
		}

		// ลบไฟล์ใน temp folder ที่เก่ากว่าที่กำหนด
		if strings.Contains(path, "/temp/") && !info.IsDir() {
			if time.Since(info.ModTime()) > time.Duration(olderThanDays)*24*time.Hour {
				if os.Remove(path) == nil {
					deletedCount++
				}
			}
		}
		return nil
	})

	return deletedCount, err
}
