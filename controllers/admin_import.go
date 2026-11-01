package controllers

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// AdminImportUsers handles Excel imports for user records using the provided template.
func AdminImportUsers(c *gin.Context) {
	adminImportUsers(c, false)
}

// AdminPreviewImportUsers always inspects without creating or updating accounts.
// A separate route also makes older API deployments reject preview requests safely.
func AdminPreviewImportUsers(c *gin.Context) {
	adminImportUsers(c, true)
}

func adminImportUsers(c *gin.Context, previewOnly bool) {

	if !requestHasPermission(c, "users.manage") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไฟล์นำเข้าจำเป็นต้องระบุ"})
		return
	}
	defer file.Close()

	ct, ok := canonicalMime(header.Header.Get("Content-Type"), header.Filename, allowedImportTemplateMimeTypes, importTemplateExtensionToMime)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ประเภทไฟล์ไม่รองรับ กรุณาใช้ .xlsx หรือ .xls"})
		return
	}
	if header.Size > 20*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไฟล์มีขนาดใหญ่เกิน 20MB"})
		return
	}

	uploadDir := "uploads/import_runs/users"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถสร้างโฟลเดอร์อัปโหลดได้"})
		return
	}

	safeName := utils.GenerateUniqueFilename(uploadDir, header.Filename)
	dstPath := filepath.Join(uploadDir, safeName)
	if err := c.SaveUploadedFile(header, dstPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกไฟล์นำเข้าได้"})
		return
	}

	rows, err := readXLSXRows(dstPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "อ่านข้อมูลจากไฟล์ไม่สำเร็จ"})
		return
	}

	if len(rows) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไฟล์ไม่มีข้อมูลสำหรับนำเข้า"})
		return
	}

	headers := normalizeHeaders(rows[0])
	requiredCols := []string{"user_fname", "user_lname", "email", "role_id"}
	for _, col := range requiredCols {
		if _, exists := headers[col]; !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("คอลัมน์ %s หายไปจากไฟล์", col)})
			return
		}
	}

	now := time.Now()
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถเริ่มนำเข้าผู้ใช้ได้"})
		return
	}
	defer tx.Rollback()

	// Inspect the whole file before inserting anything, including inactive/deleted accounts.
	seenEmails := make(map[string]int)
	blockedEmails := make(map[string]string)
	for rowIdx := 1; rowIdx < len(rows); rowIdx++ {
		data := readRow(headers, rows[rowIdx])
		email := strings.ToLower(strings.TrimSpace(data["email"]))
		if email == "" {
			continue
		}
		role, roleErr := strconv.Atoi(strings.TrimSpace(data["role_id"]))
		if !utils.ValidateEmail(email) || strings.TrimSpace(data["user_fname"]) == "" || strings.TrimSpace(data["user_lname"]) == "" || roleErr != nil || role <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("ข้อมูลผู้ใช้แถว %d ไม่ถูกต้อง กรุณาตรวจชื่อ นามสกุล อีเมล และ role_id", rowIdx+1)})
			return
		}
		if _, exists := seenEmails[email]; exists {
			if blockedEmails[email] == "" {
				blockedEmails[email] = "ซ้ำในไฟล์"
			}
			continue
		}
		seenEmails[email] = rowIdx + 1
		var count int64
		if err := tx.Model(&models.User{}).Where("LOWER(TRIM(email)) = ?", email).Count(&count).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบผู้ใช้ซ้ำได้ ยกเลิกการนำเข้า"})
			return
		}
		if count > 0 {
			blockedEmails[email] = "มีผู้ใช้อยู่แล้วในระบบ"
		}
	}
	duplicates := make([]gin.H, 0)
	for rowIdx := 1; rowIdx < len(rows); rowIdx++ {
		data := readRow(headers, rows[rowIdx])
		email := strings.ToLower(strings.TrimSpace(data["email"]))
		if reason := blockedEmails[email]; reason != "" {
			duplicates = append(duplicates, gin.H{"row": rowIdx + 1, "name": strings.TrimSpace(data["user_fname"] + " " + data["user_lname"]), "email": email, "reason": reason})
		}
	}
	newCount := len(seenEmails) - len(blockedEmails)
	if previewOnly || c.PostForm("preview") == "true" {
		c.JSON(http.StatusOK, gin.H{"success": true, "preview": true, "duplicates": duplicates, "new_count": newCount})
		return
	}
	if len(duplicates) > 0 && c.PostForm("skip_duplicates") != "true" {
		c.JSON(http.StatusConflict, gin.H{"error": "พบผู้ใช้ซ้ำ กรุณาตรวจสอบรายชื่อแล้วเลือกเพิ่มเฉพาะผู้ใช้ที่ไม่ซ้ำ หรือยกเลิกการเพิ่ม", "code": "USER_ALREADY_EXISTS", "duplicates": duplicates, "new_count": newCount})
		return
	}
	imported := 0

	for rowIdx := 1; rowIdx < len(rows); rowIdx++ {
		row := rows[rowIdx]
		rowData := readRow(headers, row)

		email := strings.ToLower(strings.TrimSpace(rowData["email"]))
		if email == "" || blockedEmails[email] != "" {
			continue
		}

		roleVal, err := strconv.Atoi(strings.TrimSpace(rowData["role_id"]))
		if err != nil || roleVal <= 0 {
			continue
		}
		accountStatus := optionalString(rowData["is_active"])
		if accountStatus == nil {
			active := "A"
			accountStatus = &active
		}
		user := models.User{
			UserFname:         strings.TrimSpace(rowData["user_fname"]),
			UserLname:         strings.TrimSpace(rowData["user_lname"]),
			Gender:            strings.TrimSpace(rowData["gender"]),
			Email:             email,
			EmailNotification: optionalString(rowData["email_notification"]),
			ScholarAuthorID:   optionalString(rowData["scholar_author_id"]),
			RoleID:            roleVal,
			Prefix:            optionalString(rowData["prefix"]),
			ManagePosition:    optionalString(rowData["manage_position"]),
			PositionTitle:     optionalString(rowData["position_title"]),
			PositionEn:        optionalString(rowData["position_en"]),
			PrefixPositionEn:  optionalString(rowData["prefix_position_en"]),
			NameEn:            optionalString(rowData["name_en"]),
			SuffixEn:          optionalString(rowData["suffix_en"]),
			Tel:               optionalString(rowData["tel"]),
			TelFormat:         optionalString(rowData["tel_format"]),
			TelEng:            optionalString(rowData["tel_eng"]),
			ManagePositionEn:  optionalString(rowData["manage_position_en"]),
			LabName:           optionalString(rowData["lab_name"]),
			Room:              optionalString(rowData["room"]),
			CPWebID:           optionalString(rowData["cp_web_id"]),
			ScopusID:          optionalString(rowData["scopus_id"]),
			AccountStatus:     accountStatus,
		}

		if dob := strings.TrimSpace(rowData["date_of_employment"]); dob != "" {
			if t := parseDate(dob); t != nil {
				user.DateOfEmployment = t
			}
		}

		// Assign timestamps for new rows
		user.CreateAt = &now
		user.UpdateAt = &now

		// Plain INSERT: a concurrent import must fail on the unique email constraint,
		// never update an existing account. Roll back every earlier insert as well.
		if err := tx.Create(&user).Error; err != nil {
			tx.Rollback()
			var mysqlErr *mysqlDriver.MySQLError
			if errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &mysqlErr) && mysqlErr.Number == 1062) {
				c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("มีผู้ใช้อยู่แล้วหรือข้อมูลซ้ำ (แถว %d อีเมล %s) ยกเลิกการนำเข้าทั้งไฟล์ ไม่มีการบันทึกหรือเขียนทับข้อมูล", rowIdx+1, email), "code": "USER_ALREADY_EXISTS"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "เกิดข้อผิดพลาดระหว่างนำเข้าผู้ใช้"})
			return
		}
		imported++
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึกข้อมูลไม่สำเร็จ"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       fmt.Sprintf("นำเข้าผู้ใช้ใหม่สำเร็จ %d รายการ", imported),
		"count":         imported,
		"skipped_count": len(duplicates),
		"file":          safeName,
		"content_type":  ct,
	})
}

// AdminImportLegacySubmissions stores uploaded legacy submission spreadsheets for offline processing.
func AdminImportLegacySubmissions(c *gin.Context) {

	if !requestHasPermission(c, "academic.import.manage") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไฟล์นำเข้าจำเป็นต้องระบุ"})
		return
	}
	defer file.Close()

	ct, ok := canonicalMime(header.Header.Get("Content-Type"), header.Filename, allowedImportTemplateMimeTypes, importTemplateExtensionToMime)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ประเภทไฟล์ไม่รองรับ กรุณาใช้ .xlsx หรือ .xls"})
		return
	}
	if header.Size > 20*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไฟล์มีขนาดใหญ่เกิน 20MB"})
		return
	}

	uploadDir := "uploads/import_runs/legacy_submissions"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถสร้างโฟลเดอร์อัปโหลดได้"})
		return
	}

	safeName := utils.GenerateUniqueFilename(uploadDir, header.Filename)
	dstPath := filepath.Join(uploadDir, safeName)

	if err := c.SaveUploadedFile(header, dstPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกไฟล์นำเข้าได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "บันทึกไฟล์ประวัติทุนย้อนหลังเรียบร้อยแล้ว สามารถนำไปประมวลผลต่อได้",
		"file":         safeName,
		"content_type": ct,
	})
}

func normalizeHeaders(row []string) map[string]int {
	headers := make(map[string]int)
	for idx, h := range row {
		key := strings.TrimSpace(strings.ToLower(h))
		if key != "" {
			headers[key] = idx
		}
	}
	return headers
}

func readRow(headers map[string]int, row []string) map[string]string {
	values := make(map[string]string)
	for key, idx := range headers {
		if idx < len(row) {
			values[key] = row[idx]
		}
	}
	return values
}

func optionalString(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func parseDate(val string) *time.Time {
	layouts := []string{"2006-01-02", "02/01/2006", "01/02/2006", "2/1/2006"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, val); err == nil {
			return &t
		}
	}
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return &t
	}
	return nil
}

// readXLSXRows extracts all rows from the first worksheet of an XLSX file without third-party dependencies.
func readXLSXRows(path string) ([][]string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var sheetXML, sharedXML io.ReadCloser
	for _, f := range r.File {
		switch f.Name {
		case "xl/worksheets/sheet1.xml":
			sheetXML, _ = f.Open()
		case "xl/sharedStrings.xml":
			sharedXML, _ = f.Open()
		}
	}

	if sheetXML == nil {
		return nil, fmt.Errorf("worksheet not found")
	}
	defer sheetXML.Close()
	defer func() {
		if sharedXML != nil {
			sharedXML.Close()
		}
	}()

	sharedStrings, _ := parseSharedStrings(sharedXML)
	return parseSheet(sheetXML, sharedStrings)
}

func parseSharedStrings(r io.Reader) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	type t struct {
		XMLName xml.Name `xml:"sst"`
		Items   []struct {
			T string `xml:"t"`
		} `xml:"si"`
	}
	var data t
	if err := xml.NewDecoder(r).Decode(&data); err != nil {
		return nil, err
	}
	strs := make([]string, 0, len(data.Items))
	for _, item := range data.Items {
		strs = append(strs, item.T)
	}
	return strs, nil
}

func parseSheet(r io.Reader, shared []string) ([][]string, error) {
	decoder := xml.NewDecoder(r)
	rows := [][]string{}
	var currentRow []string
	var lastCol int

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch se := tok.(type) {
		case xml.StartElement:
			if se.Name.Local == "row" {
				currentRow = []string{}
				lastCol = 0
			}
			if se.Name.Local == "c" {
				var cell struct {
					R  string `xml:"r,attr"`
					T  string `xml:"t,attr"`
					V  string `xml:"v"`
					IS struct {
						T string `xml:"t"`
					} `xml:"is"`
				}
				if err := decoder.DecodeElement(&cell, &se); err != nil {
					return nil, err
				}

				colIdx := columnIndex(cell.R)
				for len(currentRow) < colIdx-1 {
					currentRow = append(currentRow, "")
				}

				value := cell.V
				if cell.T == "s" { // shared string
					if idx, err := strconv.Atoi(strings.TrimSpace(cell.V)); err == nil && idx < len(shared) {
						value = shared[idx]
					}
				} else if cell.T == "inlineStr" {
					value = cell.IS.T
				}

				if len(currentRow) < colIdx {
					currentRow = append(currentRow, value)
				} else {
					currentRow[colIdx-1] = value
				}
				lastCol = colIdx
			}
		case xml.EndElement:
			if se.Name.Local == "row" {
				// Ensure row length aligns
				if len(currentRow) < lastCol {
					for len(currentRow) < lastCol {
						currentRow = append(currentRow, "")
					}
				}
				rows = append(rows, currentRow)
			}
		}
	}

	return rows, nil
}

func columnIndex(cellRef string) int {
	colPart := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, cellRef)

	col := 0
	for i := 0; i < len(colPart); i++ {
		col = col*26 + int(strings.ToUpper(string(colPart[i]))[0]-'A') + 1
	}
	return col
}
