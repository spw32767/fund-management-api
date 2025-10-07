package controllers

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PublicationRewardPreviewSubmissionRequest represents the payload for generating a preview from a stored submission.
type PublicationRewardPreviewSubmissionRequest struct {
	SubmissionID int `json:"submission_id" binding:"required"`
}

// PublicationRewardPreviewFormPayload represents the form-based preview payload.
type PublicationRewardPreviewFormPayload struct {
	FormData    PublicationRewardPreviewFormData     `json:"formData"`
	Applicant   PublicationRewardPreviewApplicant    `json:"applicant"`
	Coauthors   []PublicationRewardPreviewCoauthor   `json:"coauthors"`
	Attachments []PublicationRewardPreviewAttachment `json:"attachments"`
	External    []PublicationRewardPreviewExternal   `json:"external_fundings"`
}

type PublicationRewardPreviewFormData struct {
	AuthorStatus          string `json:"author_status"`
	ArticleTitle          string `json:"article_title"`
	JournalName           string `json:"journal_name"`
	JournalIssue          string `json:"journal_issue"`
	JournalPages          string `json:"journal_pages"`
	JournalMonth          string `json:"journal_month"`
	JournalYear           string `json:"journal_year"`
	JournalQuartile       string `json:"journal_quartile"`
	PublicationReward     string `json:"publication_reward"`
	RevisionFee           string `json:"revision_fee"`
	PublicationFee        string `json:"publication_fee"`
	ExternalFundingAmount string `json:"external_funding_amount"`
	TotalAmount           string `json:"total_amount"`
	AuthorNameList        string `json:"author_name_list"`
	Signature             string `json:"signature"`
	PublicationDate       string `json:"publication_date"`
	Doi                   string `json:"doi"`
	VolumeIssue           string `json:"volume_issue"`
	PageNumbers           string `json:"page_numbers"`
	JournalURL            string `json:"journal_url"`
	ArticleOnlineDB       string `json:"article_online_db"`
	ArticleOnlineDate     string `json:"article_online_date"`
}

type PublicationRewardPreviewApplicant struct {
	PrefixName       string `json:"prefix_name"`
	FirstName        string `json:"user_fname"`
	LastName         string `json:"user_lname"`
	PositionName     string `json:"position_name"`
	DateOfEmployment string `json:"date_of_employment"`
}

type PublicationRewardPreviewCoauthor struct {
	Order  int    `json:"order"`
	UserID int    `json:"user_id"`
	First  string `json:"user_fname"`
	Last   string `json:"user_lname"`
}

type PublicationRewardPreviewAttachment struct {
	Filename         string `json:"filename"`
	DocumentTypeID   *int   `json:"document_type_id"`
	DocumentTypeName string `json:"document_type_name"`
	DisplayOrder     int    `json:"display_order"`
}

type PublicationRewardPreviewExternal struct {
	FundName string `json:"fund_name"`
	Amount   string `json:"amount"`
}

// PreviewPublicationReward generates a Publication Reward preview PDF from a DOCX template.
func PreviewPublicationReward(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		handlePublicationRewardPreviewForm(c)
		return
	}

	handlePublicationRewardPreviewSubmission(c)
}

func handlePublicationRewardPreviewSubmission(c *gin.Context) {
	var req PublicationRewardPreviewSubmissionRequest
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

	documents, err := fetchSubmissionDocuments(config.DB, req.SubmissionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load submission documents"})
		return
	}

	var headUser *models.User
	if submission.HeadApprovedBy != nil {
		var head models.User
		if err := config.DB.First(&head, *submission.HeadApprovedBy).Error; err == nil {
			headUser = &head
		}
	}

	replacements, err := buildSubmissionPreviewReplacements(&submission, &detail, sysConfig, documents, headUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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

func handlePublicationRewardPreviewForm(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(64 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse form data"})
		return
	}
	defer c.Request.MultipartForm.RemoveAll()

	form := c.Request.MultipartForm
	rawPayload := form.Value["data"]
	if len(rawPayload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing form payload"})
		return
	}

	var payload PublicationRewardPreviewFormPayload
	decoder := json.NewDecoder(strings.NewReader(rawPayload[0]))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid form payload"})
		return
	}

	sysConfig, err := fetchLatestSystemConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load system configuration"})
		return
	}

	attachments := form.File["attachments"]

	requesterID := 0
	if id, ok := getUserIDFromContext(c); ok {
		requesterID = int(id)
	}

	replacements, err := buildFormPreviewReplacements(&payload, sysConfig, attachments, requesterID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pdfData, err := generatePublicationRewardPDF(replacements)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	merged, err := mergePreviewPDFWithAttachments(pdfData, attachments)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "inline; filename=publication_reward_preview.pdf")
	c.Data(http.StatusOK, "application/pdf", merged)
}

func buildFormPreviewReplacements(payload *PublicationRewardPreviewFormPayload, sysConfig *systemConfigSnapshot, attachments []*multipart.FileHeader, requesterID int) (map[string]string, error) {
	if payload == nil {
		return nil, fmt.Errorf("invalid form payload")
	}

	if sysConfig == nil {
		sysConfig = &systemConfigSnapshot{}
	}

	totalAmount := parseFormFloat(payload.FormData.TotalAmount)
	publicationDate := resolveFormPublicationDate(&payload.FormData)
	publicationYearText := derivePublicationYear(publicationDate, payload.FormData.JournalYear)

	employmentDate := formatThaiDateFromString(payload.Applicant.DateOfEmployment)
	if employmentDate == "" {
		employmentDate = lookupEmploymentDateFromUserID(requesterID)
	}

	replacements := map[string]string{
		"{{date_th}}":            utils.FormatThaiDate(time.Now()),
		"{{applicant_name}}":     buildPreviewApplicantName(payload.Applicant),
		"{{date_of_employment}}": employmentDate,
		"{{position}}":           strings.TrimSpace(payload.Applicant.PositionName),
		"{{installment}}":        formatNullableInt(sysConfig.Installment),
		"{{total_amount}}":       formatAmount(totalAmount),
		"{{total_amount_text}}":  utils.BahtText(totalAmount),
		"{{author_name_list}}":   strings.TrimSpace(payload.FormData.AuthorNameList),
		"{{paper_title}}":        strings.TrimSpace(payload.FormData.ArticleTitle),
		"{{journal_name}}":       strings.TrimSpace(payload.FormData.JournalName),
		"{{publication_year}}":   publicationYearText,
		"{{volume_issue}}":       strings.TrimSpace(payload.FormData.JournalIssue),
		"{{page_number}}":        strings.TrimSpace(payload.FormData.JournalPages),
		"{{author_role}}":        buildAuthorRole(payload.FormData.AuthorStatus),
		"{{quartile_line}}":      buildQuartileLine(payload.FormData.JournalQuartile),
		"{{document_line}}":      buildPreviewDocumentLine(payload.Attachments, attachments),
		"{{kku_report_year}}":    formatNullableString(sysConfig.KkuReportYear),
		"{{signature}}":          strings.TrimSpace(payload.FormData.Signature),
		"{{head_comment}}":       "",
		"{{head_signature}}":     "",
		"{{head_name}}":          "",
		"{{head_approved_date}}": "",
	}

	return replacements, nil
}

func parseFormFloat(raw string) float64 {
	cleaned := strings.ReplaceAll(strings.TrimSpace(raw), ",", "")
	if cleaned == "" {
		return 0
	}
	value, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0
	}
	return value
}

func resolveFormPublicationDate(data *PublicationRewardPreviewFormData) *time.Time {
	if data == nil {
		return nil
	}

	raw := strings.TrimSpace(data.PublicationDate)
	if raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			return &t
		}
	}

	year := strings.TrimSpace(data.JournalYear)
	if year == "" {
		return nil
	}

	month := strings.TrimSpace(data.JournalMonth)
	monthValue, err := strconv.Atoi(month)
	if err != nil || monthValue < 1 || monthValue > 12 {
		monthValue = 1
	}

	dateStr := fmt.Sprintf("%s-%02d-01", year, monthValue)
	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		return &t
	}

	return nil
}

func formatThaiDateFromString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if t, err := time.Parse("2006-01-02", trimmed); err == nil {
		return utils.FormatThaiDate(t)
	}

	return ""
}

func resolveApplicantEmploymentDate(user *models.User) string {
	if user == nil {
		return ""
	}

	if formatted := utils.FormatThaiDatePtr(user.DateOfEmployment); formatted != "" {
		return formatted
	}

	if user.UserID == 0 {
		return ""
	}

	type employmentRow struct {
		Date sql.NullTime `gorm:"column:date_of_employment"`
	}

	var row employmentRow
	if err := config.DB.Table("users").
		Select("date_of_employment").
		Where("user_id = ?", user.UserID).
		Scan(&row).Error; err != nil {
		return ""
	}

	if row.Date.Valid {
		return utils.FormatThaiDate(row.Date.Time)
	}

	return ""
}

func lookupEmploymentDateFromUserID(userID int) string {
	if userID <= 0 {
		return ""
	}

	var user models.User
	if err := config.DB.
		Select("user_id", "date_of_employment").
		Where("user_id = ?", userID).
		First(&user).Error; err != nil {
		return ""
	}

	if formatted := utils.FormatThaiDatePtr(user.DateOfEmployment); formatted != "" {
		return formatted
	}

	return ""
}

func buildPreviewApplicantName(app PublicationRewardPreviewApplicant) string {
	parts := []string{
		strings.TrimSpace(app.PrefixName),
		strings.TrimSpace(app.FirstName),
		strings.TrimSpace(app.LastName),
	}
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, " ")
}

func mergePreviewPDFWithAttachments(base []byte, files []*multipart.FileHeader) ([]byte, error) {
	if len(files) == 0 {
		return base, nil
	}

	tmpDir, err := os.MkdirTemp("", "publication-preview-merge-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	basePath := filepath.Join(tmpDir, "base.pdf")
	if err := os.WriteFile(basePath, base, 0600); err != nil {
		return nil, fmt.Errorf("failed to write base pdf: %w", err)
	}

	inputFiles := []string{basePath}

	for idx, header := range files {
		src, err := header.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open attachment %s: %w", header.Filename, err)
		}
		data, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read attachment %s: %w", header.Filename, err)
		}
		if len(bytes.TrimSpace(data)) == 0 {
			continue
		}
		if !bytes.HasPrefix(data, []byte("%PDF")) {
			return nil, fmt.Errorf("attachment %s is not a PDF file", header.Filename)
		}

		destPath := filepath.Join(tmpDir, fmt.Sprintf("attachment-%d.pdf", idx+1))
		if err := os.WriteFile(destPath, data, 0600); err != nil {
			return nil, fmt.Errorf("failed to write attachment %s: %w", header.Filename, err)
		}
		inputFiles = append(inputFiles, destPath)
	}

	if len(inputFiles) == 1 {
		return base, nil
	}

	outputPath := filepath.Join(tmpDir, "merged.pdf")
	if err := mergePDFs(inputFiles, outputPath); err != nil {
		return nil, err
	}

	merged, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read merged pdf: %w", err)
	}

	return merged, nil
}

func mergePDFs(inputs []string, outputPath string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no pdf files provided for merging")
	}

	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve output path: %w", err)
	}

	absInputs := make([]string, 0, len(inputs))
	for _, input := range inputs {
		absInput, err := filepath.Abs(input)
		if err != nil {
			return fmt.Errorf("failed to resolve input path %s: %w", input, err)
		}
		absInputs = append(absInputs, absInput)
	}

	var attempts []string

	if nodeBinary, err := resolveNodeBinary(); err == nil {
		if err := mergePDFsWithNode(nodeBinary, absInputs, absOutput); err == nil {
			return nil
		} else {
			attempts = append(attempts, fmt.Sprintf("node (%v)", err))
		}
	} else {
		attempts = append(attempts, fmt.Sprintf("node (%v)", err))
	}

	if gsBinary, err := exec.LookPath("gs"); err == nil {
		if err := mergePDFsWithGhostscript(gsBinary, absInputs, absOutput); err == nil {
			return nil
		} else {
			attempts = append(attempts, fmt.Sprintf("gs (%v)", err))
		}
	} else {
		attempts = append(attempts, fmt.Sprintf("gs (%v)", err))
	}

	if uniteBinary, err := exec.LookPath("pdfunite"); err == nil {
		if err := mergePDFsWithPdfunite(uniteBinary, absInputs, absOutput); err == nil {
			return nil
		} else {
			attempts = append(attempts, fmt.Sprintf("pdfunite (%v)", err))
		}
	} else {
		attempts = append(attempts, fmt.Sprintf("pdfunite (%v)", err))
	}

	if len(attempts) == 0 {
		return fmt.Errorf("failed to merge pdf files: no merge strategy available")
	}

	return fmt.Errorf("failed to merge pdf files: %s", strings.Join(attempts, "; "))
}

func mergePDFsWithNode(nodeBinary string, inputs []string, outputPath string) error {
	scriptPath := filepath.Join("scripts", "merge_pdf.js")
	absScriptPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to resolve merge script path: %w", err)
	}
	if _, err := os.Stat(absScriptPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("merge script not found at %s", absScriptPath)
		}
		return fmt.Errorf("failed to access merge script: %w", err)
	}

	args := append([]string{absScriptPath, outputPath}, inputs...)

	repoDir := filepath.Dir(filepath.Dir(absScriptPath))
	nodeModulesPath := filepath.Join(repoDir, "..", "frontend_project_fund", "node_modules")
	absNodeModules, err := filepath.Abs(nodeModulesPath)
	if err != nil {
		return fmt.Errorf("failed to resolve node_modules path: %w", err)
	}
	if _, err := os.Stat(absNodeModules); err != nil {
		return fmt.Errorf("pdf-lib dependency not found: %w", err)
	}

	cmd := exec.Command(nodeBinary, args...)
	env := append([]string{}, os.Environ()...)
	env = append(env, fmt.Sprintf("NODE_PATH=%s", absNodeModules))
	cmd.Env = env

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}

	return nil
}

func mergePDFsWithGhostscript(gsBinary string, inputs []string, outputPath string) error {
	args := []string{"-q", "-dNOPAUSE", "-dBATCH", "-sDEVICE=pdfwrite", fmt.Sprintf("-sOutputFile=%s", outputPath)}
	args = append(args, inputs...)

	cmd := exec.Command(gsBinary, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}

	return nil
}

func mergePDFsWithPdfunite(uniteBinary string, inputs []string, outputPath string) error {
	args := append([]string{}, inputs...)
	args = append(args, outputPath)

	cmd := exec.Command(uniteBinary, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}

	return nil
}

func resolveNodeBinary() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PUBLICATION_PREVIEW_NODE_BINARY")); override != "" {
		if filepath.IsAbs(override) {
			if _, err := os.Stat(override); err == nil {
				return override, nil
			} else {
				return "", fmt.Errorf("configured node binary %s is not accessible: %w", override, err)
			}
		}
		if resolved, err := exec.LookPath(override); err == nil {
			return resolved, nil
		}
	}

	candidates := []string{"node", "nodejs"}
	var errs []string
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		} else {
			errs = append(errs, fmt.Sprintf("%s: %v", candidate, err))
		}
	}

	if len(errs) > 0 {
		return "", fmt.Errorf("node binary not found: %s", strings.Join(errs, "; "))
	}

	return "", fmt.Errorf("node binary not found")
}

func buildPreviewDocumentLine(meta []PublicationRewardPreviewAttachment, attachments []*multipart.FileHeader) string {
	if len(meta) > 0 {
		metaCopy := append([]PublicationRewardPreviewAttachment(nil), meta...)
		sort.SliceStable(metaCopy, func(i, j int) bool {
			if metaCopy[i].DisplayOrder == metaCopy[j].DisplayOrder {
				return i < j
			}
			return metaCopy[i].DisplayOrder < metaCopy[j].DisplayOrder
		})

		lines := make([]string, 0, len(metaCopy))
		for _, entry := range metaCopy {
			name := strings.TrimSpace(entry.DocumentTypeName)
			if name == "" {
				name = strings.TrimSpace(entry.Filename)
			}
			if name == "" {
				continue
			}
			lines = append(lines, buildDocumentQuantityLine(name))
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n")
		}
	}

	if len(attachments) == 0 {
		return ""
	}

	lines := make([]string, 0, len(attachments))
	for _, header := range attachments {
		name := strings.TrimSpace(header.Filename)
		if name == "" {
			continue
		}
		lines = append(lines, buildDocumentQuantityLine(name))
	}
	return strings.Join(lines, "\n")
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

func fetchSubmissionDocuments(db *gorm.DB, submissionID int) ([]models.SubmissionDocument, error) {
	if db == nil {
		db = config.DB
	}

	var documents []models.SubmissionDocument
	if err := db.
		Joins("LEFT JOIN document_types dt ON dt.document_type_id = submission_documents.document_type_id").
		Select("submission_documents.*, dt.document_type_name").
		Preload("DocumentType").
		Preload("File").
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
			name = strings.TrimSpace(doc.Description)
		}
		if name == "" {
			name = strings.TrimSpace(doc.File.OriginalName)
		}
		if name == "" {
			continue
		}
		lines = append(lines, buildDocumentQuantityLine(name))
	}

	return strings.Join(lines, "\n")
}

func buildDocumentQuantityLine(name string) string {
	unit := documentUnitForName(name)
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return ""
	}
	return fmt.Sprintf("%s จำนวน 1 %s", cleanName, unit)
}

func documentUnitForName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return "ฉบับ"
	}

	if normalized == strings.ToLower("Full Reprint (บทความตีพิมพ์)") || strings.Contains(normalized, "บทความตีพิมพ์") {
		return "เรื่อง"
	}

	return "ฉบับ"
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

	fontEnv, err := configureLibreOfficeFonts(tmpDir)
	if err != nil {
		return nil, err
	}

	converter, err := lookupLibreOfficeBinary()
	if err != nil {
		return nil, err
	}

	profileDir := filepath.Join(tmpDir, "lo-profile")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to prepare libreoffice profile: %w", err)
	}

	profileURL, err := fileURLFromPath(profileDir)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare libreoffice profile: %w", err)
	}

	profileArg := fmt.Sprintf("-env:UserInstallation=%s", profileURL)
	filterArg := "pdf:writer_pdf_Export:EmbedStandardFonts=true;EmbedFonts=true"

	args := []string{profileArg, "--headless", "--convert-to", filterArg, "--outdir", tmpDir, outputDocx}
	cmd := exec.Command(converter, args...)
	env := append([]string{}, os.Environ()...)
	if len(fontEnv) > 0 {
		env = append(env, fontEnv...)
	}
	cmd.Env = env

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

func lookupLibreOfficeBinary() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("LIBREOFFICE_PATH")); explicit != "" {
		if runtime.GOOS == "windows" {
			explicit = strings.Trim(explicit, "\"")
		}

		candidate := explicit
		if !filepath.IsAbs(candidate) {
			absPath, err := filepath.Abs(candidate)
			if err != nil {
				return "", fmt.Errorf("invalid LIBREOFFICE_PATH: %w", err)
			}
			candidate = absPath
		}

		info, err := os.Stat(candidate)
		if err != nil {
			return "", fmt.Errorf("invalid LIBREOFFICE_PATH: %w", err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("LIBREOFFICE_PATH must point to the soffice executable, not a directory")
		}

		return candidate, nil
	}

	if path, err := exec.LookPath("soffice"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("libreoffice"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("libreoffice (soffice) binary not found in PATH; set LIBREOFFICE_PATH to override")
}

func fileURLFromPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	absPath = filepath.ToSlash(absPath)

	if runtime.GOOS == "windows" {
		if strings.HasPrefix(absPath, "//") {
			trimmed := strings.TrimPrefix(absPath, "//")
			parts := strings.SplitN(trimmed, "/", 2)
			host := parts[0]
			var uncPath string
			if len(parts) == 2 {
				uncPath = "/" + parts[1]
			}

			u := &url.URL{
				Scheme: "file",
				Host:   host,
				Path:   uncPath,
			}
			return u.String(), nil
		}

		if !strings.HasPrefix(absPath, "/") {
			absPath = "/" + absPath
		}
	} else if !strings.HasPrefix(absPath, "/") {
		absPath = "/" + absPath
	}

	u := &url.URL{Scheme: "file", Path: absPath}
	return u.String(), nil
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
			content = normalizeDocxPlaceholders(content, replacements)
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

var (
	placeholderRegexCache sync.Map
	proofErrTagPattern    = regexp.MustCompile(`<w:proofErr[^>]*/>`)
)

func normalizeDocxPlaceholders(content string, replacements map[string]string) string {
	if len(replacements) == 0 {
		return content
	}

	content = proofErrTagPattern.ReplaceAllString(content, "")

	keys := make([]string, 0, len(replacements))
	for placeholder := range replacements {
		keys = append(keys, placeholder)
	}
	sort.Strings(keys)

	for _, placeholder := range keys {
		re := placeholderRegexFor(placeholder)
		content = re.ReplaceAllString(content, placeholder)
	}

	return content
}

func placeholderRegexFor(placeholder string) *regexp.Regexp {
	if cached, ok := placeholderRegexCache.Load(placeholder); ok {
		return cached.(*regexp.Regexp)
	}

	key := strings.TrimSpace(placeholder)
	if len(key) < 4 {
		re := regexp.MustCompile(regexp.QuoteMeta(placeholder))
		placeholderRegexCache.Store(placeholder, re)
		return re
	}

	inner := strings.TrimPrefix(key, "{{")
	inner = strings.TrimSuffix(inner, "}}")

	var builder strings.Builder
	gap := `(?:\s|<[^>]+>)*`

	builder.WriteString(`\{\{`)
	builder.WriteString(gap)
	for _, r := range inner {
		builder.WriteString(regexp.QuoteMeta(string(r)))
		builder.WriteString(gap)
	}
	builder.WriteString(`\}\}`)

	re := regexp.MustCompile(builder.String())
	placeholderRegexCache.Store(placeholder, re)
	return re
}

func formatThaiYear(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return strconv.Itoa(t.Year() + 543)
}

func formatThaiYearPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatThaiYear(*t)
}

func derivePublicationYear(date *time.Time, fallback string) string {
	if value := formatThaiYearPtr(date); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func configureLibreOfficeFonts(tmpDir string) ([]string, error) {
	fontDirs := collectFontDirectories()
	if len(fontDirs) == 0 {
		return nil, nil
	}

	fontConfigDir := filepath.Join(tmpDir, "fontconfig")
	if err := os.MkdirAll(fontConfigDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to prepare fontconfig directory: %w", err)
	}

	configPath := filepath.Join(fontConfigDir, "fonts.conf")
	var builder strings.Builder
	builder.WriteString("<?xml version=\"1.0\"?>\n")
	builder.WriteString("<fontconfig>\n")
	for _, dir := range fontDirs {
		builder.WriteString("  <dir>")
		builder.WriteString(xmlEscape(dir))
		builder.WriteString("</dir>\n")
	}

	aliasMap := map[string]string{
		"Cordia New":      "TH Sarabun New",
		"Angsana New":     "TH Sarabun New",
		"AngsanaUPC":      "TH Sarabun New",
		"Sarabun":         "TH Sarabun New",
		"Times New Roman": "DejaVu Serif",
		"Calibri":         "DejaVu Sans",
		"Calibri Light":   "DejaVu Sans",
		"Segoe UI Symbol": "DejaVu Sans",
	}

	aliasKeys := make([]string, 0, len(aliasMap))
	for from := range aliasMap {
		aliasKeys = append(aliasKeys, from)
	}
	sort.Strings(aliasKeys)

	for _, from := range aliasKeys {
		to := aliasMap[from]
		builder.WriteString("  <alias binding=\"strong\">\n")
		builder.WriteString("    <family>")
		builder.WriteString(xmlEscape(from))
		builder.WriteString("</family>\n")
		builder.WriteString("    <accept>\n")
		builder.WriteString("      <family>")
		builder.WriteString(xmlEscape(to))
		builder.WriteString("</family>\n")
		builder.WriteString("    </accept>\n")
		builder.WriteString("  </alias>\n")
	}

	builder.WriteString("</fontconfig>\n")

	if err := os.WriteFile(configPath, []byte(builder.String()), 0600); err != nil {
		return nil, fmt.Errorf("failed to write font configuration: %w", err)
	}

	xdgDataHome := filepath.Join(tmpDir, "xdg-data")
	if err := os.MkdirAll(xdgDataHome, 0700); err != nil {
		return nil, fmt.Errorf("failed to prepare font cache directory: %w", err)
	}

	xdgCacheHome := filepath.Join(tmpDir, "xdg-cache")
	if err := os.MkdirAll(xdgCacheHome, 0700); err != nil {
		return nil, fmt.Errorf("failed to prepare font cache directory: %w", err)
	}

	env := []string{
		fmt.Sprintf("FONTCONFIG_FILE=%s", configPath),
		fmt.Sprintf("FONTCONFIG_PATH=%s", fontConfigDir),
		fmt.Sprintf("XDG_DATA_HOME=%s", xdgDataHome),
		fmt.Sprintf("XDG_CACHE_HOME=%s", xdgCacheHome),
	}

	return env, nil
}

func collectFontDirectories() []string {
	candidates := []string{
		filepath.Join("templates", "fonts"),
		filepath.Join("frontend_project_fund", "public", "font"),
	}

	seen := make(map[string]struct{})
	var result []string

	for _, candidate := range candidates {
		absRoot, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}

		info, err := os.Stat(absRoot)
		if err != nil || !info.IsDir() {
			continue
		}

		_ = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}

			name := d.Name()
			if strings.HasPrefix(name, "._") {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".ttf" && ext != ".otf" {
				return nil
			}

			dir := filepath.Dir(path)
			if _, exists := seen[dir]; exists {
				return nil
			}

			seen[dir] = struct{}{}
			result = append(result, dir)
			return nil
		})
	}

	sort.Strings(result)
	return result
}
