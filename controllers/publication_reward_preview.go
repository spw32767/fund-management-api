package controllers

import (
	"archive/zip"
	"database/sql"
	"errors"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PublicationRewardPreviewRequest represents the payload for generating a preview PDF.
type PublicationRewardPreviewRequest struct {
	SubmissionID int `json:"submission_id" binding:"required"`
}

// PreviewPublicationReward generates a Publication Reward preview PDF from a DOCX template.
func PreviewPublicationReward(c *gin.Context) {
	var req PublicationRewardPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	var submission models.Submission
	if err := config.DB.
		Preload("User").
		Preload("User.Position").
		Where("submission_id = ? AND submission_type = ?", req.SubmissionID, "publication_reward").
		First(&submission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load submission"})
		return
	}

	if submission.User == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "submission missing applicant"})
		return
	}

	var detail models.PublicationRewardDetail
	if err := config.DB.
		Where("submission_id = ? AND (delete_at IS NULL OR delete_at = '0000-00-00 00:00:00')", req.SubmissionID).
		First(&detail).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "publication reward detail not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load publication reward detail"})
		return
	}

	sysConfig, err := fetchLatestSystemConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load system configuration"})
		return
	}

	documents, err := fetchSubmissionDocuments(req.SubmissionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load submission documents"})
		return
	}

	replacements := map[string]string{
		"{{date_th}}":            utils.FormatThaiDate(submission.CreatedAt),
		"{{applicant_name}}":     buildApplicantName(submission.User),
		"{{date_of_employment}}": utils.FormatThaiDatePtr(submission.User.DateOfEmployment),
		"{{position}}":           strings.TrimSpace(submission.User.Position.PositionName),
		"{{installment}}":        formatNullableInt(sysConfig.Installment),
		"{{total_amount}}":       formatAmount(detail.TotalAmount),
		"{{total_amount_text}}":  utils.BahtText(detail.TotalAmount),
		"{{author_name_list}}":   strings.TrimSpace(detail.AuthorNameList),
		"{{paper_title}}":        strings.TrimSpace(detail.PaperTitle),
		"{{journal_name}}":       strings.TrimSpace(detail.JournalName),
		"{{publication_date}}":   utils.FormatThaiDate(detail.PublicationDate),
		"{{volume_issue}}":       strings.TrimSpace(detail.VolumeIssue),
		"{{page_numbers}}":       strings.TrimSpace(detail.PageNumbers),
		"{{author_role}}":        buildAuthorRole(detail.AuthorType),
		"{{quartile_line}}":      buildQuartileLine(detail.Quartile),
		"{{document_line}}":      buildDocumentLine(documents),
		"{{kku_report_year}}":    formatNullableString(sysConfig.KkuReportYear),
		"{{signature}}":          strings.TrimSpace(detail.Signature),
	}

	pdfData, err := generatePublicationRewardPDF(replacements)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "inline; filename=publication_reward_preview.pdf")
	c.Data(http.StatusOK, "application/pdf", pdfData)
}

func fetchLatestSystemConfig() (*systemConfigSnapshot, error) {
	var row systemConfigSnapshot
	if err := config.DB.Table("system_config").
		Select("installment, kku_report_year").
		Order("config_id DESC").
		Limit(1).
		Scan(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &systemConfigSnapshot{}, nil
		}
		return nil, err
	}
	return &row, nil
}

func fetchSubmissionDocuments(submissionID int) ([]models.SubmissionDocument, error) {
	var documents []models.SubmissionDocument
	if err := config.DB.
		Preload("DocumentType").
		Where("submission_id = ?", submissionID).
		Order("display_order ASC, document_id ASC").
		Find(&documents).Error; err != nil {
		return nil, err
	}
	return documents, nil
}

type systemConfigSnapshot struct {
	Installment   sql.NullInt64
	KkuReportYear sql.NullString
}

func buildApplicantName(user *models.User) string {
	if user == nil {
		return ""
	}
	fname := strings.TrimSpace(user.UserFname)
	lname := strings.TrimSpace(user.UserLname)
	return strings.TrimSpace(strings.Join([]string{fname, lname}, " "))
}

func formatNullableInt(value sql.NullInt64) string {
	if !value.Valid {
		return ""
	}
	return strconv.FormatInt(value.Int64, 10)
}

func formatNullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

func formatAmount(amount float64) string {
	formatted := fmt.Sprintf("%.2f", amount)
	parts := strings.Split(formatted, ".")
	integerPart := parts[0]
	decimalPart := ""
	if len(parts) > 1 {
		decimalPart = parts[1]
	}

	negative := false
	if strings.HasPrefix(integerPart, "-") {
		negative = true
		integerPart = integerPart[1:]
	}

	var builder strings.Builder
	for i, r := range integerPart {
		if i != 0 && (len(integerPart)-i)%3 == 0 {
			builder.WriteByte(',')
		}
		builder.WriteRune(r)
	}

	result := builder.String()
	if negative {
		result = "-" + result
	}
	if decimalPart != "" {
		result += "." + decimalPart
	}
	return result
}

func buildAuthorRole(authorType string) string {
	switch strings.ToLower(strings.TrimSpace(authorType)) {
	case "first_author":
		return "เป็นผู้ประพันธ์ชื่อแรก (first author)"
	case "corresponding_author":
		return "เป็นผู้ประพันธ์บรรณกิจ (corresponding author)"
	default:
		return ""
	}
}

func buildQuartileLine(quartile string) string {
	switch strings.ToUpper(strings.TrimSpace(quartile)) {
	case "T5":
		return "บทความตีพิมพ์ในวารสารระดับนานาชาติ ควอไทล์ 1 (ลำดับ 5% แรก) ที่สามารถสืบค้นได้ในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS"
	case "T10":
		return "บทความตีพิมพ์ในวารสารระดับนานาชาติ ควอไทล์ 1 (ลำดับ 10% แรก) ที่สามารถสืบค้นได้ในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS"
	case "Q1":
		return "บทความตีพิมพ์ในวารสารระดับนานาชาติ ควอไทล์ 1 ที่สามารถสืบค้นได้ในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS"
	case "Q2":
		return "บทความตีพิมพ์ในวารสารระดับนานาชาติ ควอไทล์ 2 ที่สามารถสืบค้นได้ในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS"
	case "Q3":
		return "บทความตีพิมพ์ในวารสารระดับนานาชาติ ควอไทล์ 3 ที่สามารถสืบค้นได้ในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS"
	case "Q4":
		return "บทความตีพิมพ์ในวารสารระดับนานาชาติ ควอไทล์ 4 ที่สามารถสืบค้นได้ในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS"
	case "TCI":
		return "บทความตีพิมพ์ในวารสารระดับนานาชาติ อยู่ในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS หรือวารสารที่อยู่ในฐานข้อมูล TCI"
	default:
		return ""
	}
}

func buildDocumentLine(documents []models.SubmissionDocument) string {
	if len(documents) == 0 {
		return ""
	}

	lines := make([]string, 0, len(documents))
	for _, doc := range documents {
		name := strings.TrimSpace(doc.DocumentTypeName)
		if name == "" {
			name = strings.TrimSpace(doc.DocumentType.DocumentTypeName)
		}
		if name == "" {
			continue
		}
		lines = append(lines, "☑ "+name+" — จำนวน 1 ฉบับ")
	}

	return strings.Join(lines, "\n")
}

func generatePublicationRewardPDF(replacements map[string]string) ([]byte, error) {
	templatePath := filepath.Join("templates", "publication_reward_template.docx")
	if _, err := os.Stat(templatePath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("template file not found")
		}
		return nil, fmt.Errorf("failed to access template: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "publication-preview-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outputDocx := filepath.Join(tmpDir, "publication_reward_preview.docx")
	if err := fillDocxTemplate(templatePath, outputDocx, replacements); err != nil {
		return nil, err
	}

	cmd := exec.Command("libreoffice", "--headless", "--convert-to", "pdf", "--outdir", tmpDir, outputDocx)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to convert to pdf: %v", strings.TrimSpace(string(output)))
	}

	outputPDF := filepath.Join(tmpDir, "publication_reward_preview.pdf")
	data, err := os.ReadFile(outputPDF)
	if err != nil {
		return nil, fmt.Errorf("failed to read generated pdf: %w", err)
	}

	return data, nil
}

func fillDocxTemplate(templatePath, outputPath string, replacements map[string]string) error {
	reader, err := zip.OpenReader(templatePath)
	if err != nil {
		return fmt.Errorf("failed to open template: %w", err)
	}
	defer reader.Close()

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output docx: %w", err)
	}
	defer outFile.Close()

	writer := zip.NewWriter(outFile)
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			writer.Close()
			return fmt.Errorf("failed to read template entry: %w", err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			writer.Close()
			return fmt.Errorf("failed to read template entry: %w", err)
		}

		if strings.HasSuffix(strings.ToLower(file.Name), ".xml") {
			content := string(data)
			for placeholder, value := range replacements {
				content = strings.ReplaceAll(content, placeholder, formatDocxValue(value))
			}
			data = []byte(content)
		}

		header := file.FileHeader
		writerEntry, err := writer.CreateHeader(&header)
		if err != nil {
			writer.Close()
			return fmt.Errorf("failed to write docx entry: %w", err)
		}

		if _, err := writerEntry.Write(data); err != nil {
			writer.Close()
			return fmt.Errorf("failed to write docx entry: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to finalize docx: %w", err)
	}
	return nil
}

func formatDocxValue(value string) string {
	if value == "" {
		return ""
	}

	parts := strings.Split(value, "\n")
	for i, part := range parts {
		parts[i] = xmlEscape(part)
	}

	if len(parts) == 1 {
		return parts[0]
	}

	return strings.Join(parts, "</w:t><w:br/><w:t xml:space=\"preserve\">")
}

var xmlReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"\"", "&quot;",
	"'", "&apos;",
)

func xmlEscape(value string) string {
	return xmlReplacer.Replace(value)
}
