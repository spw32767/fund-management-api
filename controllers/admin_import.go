package controllers

import (
	"archive/zip"
	"encoding/xml"
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
	"gorm.io/gorm/clause"
)

// AdminImportUsers handles Excel imports for user records using the provided template.
func AdminImportUsers(c *gin.Context) {
	roleID, ok := c.Get("roleID")
	if !ok || roleID.(int) != 3 {
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
	requiredCols := []string{"user_fname", "user_lname", "email", "role_id", "position_id"}
	for _, col := range requiredCols {
		if _, exists := headers[col]; !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("คอลัมน์ %s หายไปจากไฟล์", col)})
			return
		}
	}

	now := time.Now()
	hashedPassword, err := utils.HashPassword("changeme123")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถเตรียมข้อมูลรหัสผ่านได้"})
		return
	}

	tx := config.DB.Begin()
	imported := 0

	for rowIdx := 1; rowIdx < len(rows); rowIdx++ {
		row := rows[rowIdx]
		rowData := readRow(headers, row)

		email := strings.TrimSpace(rowData["email"])
		if email == "" {
			continue
		}

		roleVal, err := strconv.Atoi(strings.TrimSpace(rowData["role_id"]))
		if err != nil || roleVal <= 0 {
			continue
		}
		positionVal, err := strconv.Atoi(strings.TrimSpace(rowData["position_id"]))
		if err != nil || positionVal <= 0 {
			continue
		}

		user := models.User{
			UserFname:        strings.TrimSpace(rowData["user_fname"]),
			UserLname:        strings.TrimSpace(rowData["user_lname"]),
			Gender:           strings.TrimSpace(rowData["gender"]),
			Email:            email,
			RoleID:           roleVal,
			PositionID:       positionVal,
			Password:         hashedPassword,
			Prefix:           optionalString(rowData["prefix"]),
			ManagePosition:   optionalString(rowData["manage_position"]),
			PositionTitle:    optionalString(rowData["position_title"]),
			PositionEn:       optionalString(rowData["position_en"]),
			PrefixPositionEn: optionalString(rowData["prefix_position_en"]),
			NameEn:           optionalString(rowData["name_en"]),
			SuffixEn:         optionalString(rowData["suffix_en"]),
			Tel:              optionalString(rowData["tel"]),
			TelFormat:        optionalString(rowData["tel_format"]),
			TelEng:           optionalString(rowData["tel_eng"]),
			ManagePositionEn: optionalString(rowData["manage_position_en"]),
			LabName:          optionalString(rowData["lab_name"]),
			Room:             optionalString(rowData["room"]),
			CPWebID:          optionalString(rowData["cp_web_id"]),
			ScopusID:         optionalString(rowData["scopus_id"]),
			AccountStatus:    optionalString(rowData["is_active"]),
		}

		if dob := strings.TrimSpace(rowData["date_of_employment"]); dob != "" {
			if t := parseDate(dob); t != nil {
				user.DateOfEmployment = t
			}
		}

		// Assign timestamps for new rows
		user.CreateAt = &now
		user.UpdateAt = &now

		assignments := clause.Assignments(map[string]interface{}{
			"user_fname":         user.UserFname,
			"user_lname":         user.UserLname,
			"gender":             user.Gender,
			"role_id":            user.RoleID,
			"position_id":        user.PositionID,
			"date_of_employment": user.DateOfEmployment,
			"prefix":             user.Prefix,
			"manage_position":    user.ManagePosition,
			"position":           user.PositionTitle,
			"position_en":        user.PositionEn,
			"prefix_position_en": user.PrefixPositionEn,
			"Name_en":            user.NameEn,
			"suffix_en":          user.SuffixEn,
			"TEL":                user.Tel,
			"TELformat":          user.TelFormat,
			"TEL_ENG":            user.TelEng,
			"manage_position_en": user.ManagePositionEn,
			"LAB_Name":           user.LabName,
			"Room":               user.Room,
			"CP_WEB_ID":          user.CPWebID,
			"Scopus_id":          user.ScopusID,
			"Is_active":          user.AccountStatus,
			"update_at":          now,
		})

		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "email"}},
			DoUpdates: assignments,
		}).Create(&user).Error; err != nil {
			tx.Rollback()
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
		"success":      true,
		"message":      fmt.Sprintf("นำเข้าผู้ใช้สำเร็จ %d รายการ (อัปเดตหากมีอยู่แล้ว)", imported),
		"count":        imported,
		"file":         safeName,
		"content_type": ct,
	})
}

// AdminImportLegacySubmissions stores uploaded legacy submission spreadsheets for offline processing.
func AdminImportLegacySubmissions(c *gin.Context) {
	roleID, ok := c.Get("roleID")
	if !ok || roleID.(int) != 3 {
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
