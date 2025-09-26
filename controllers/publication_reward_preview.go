package controllers

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// convertDocxToPDF runs LibreOffice headless and ensures we get a PDF path or an error with details.
func convertDocxToPDF(loBinary, docxPath, outDir string) (string, error) {
	cmd := exec.Command(loBinary,
		"--headless",
		"--nologo",
		"--nodefault",
		"--nolockcheck",
		"--norestore",
		"--convert-to", "pdf",
		"--outdir", outDir,
		docxPath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("libreoffice failed: %v; stderr: %s", err, strings.TrimSpace(stderr.String()))
	}

	// Expect same basename but .pdf in outDir
	base := strings.TrimSuffix(filepath.Base(docxPath), filepath.Ext(docxPath)) + ".pdf"
	pdfPath := filepath.Join(outDir, base)
	if _, err := os.Stat(pdfPath); err != nil {
		return "", fmt.Errorf("pdf not created: %v; stdout: %s; stderr: %s",
			err, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}
	return pdfPath, nil
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
		"{{publication_year}}":   formatThaiYear(detail.PublicationDate),
		"{{volume_issue}}":       strings.TrimSpace(detail.VolumeIssue),
		"{{page_number}}":        strings.TrimSpace(detail.PageNumbers),
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

	replacements, err := buildFormPreviewReplacements(&payload, sysConfig, attachments)
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

func buildFormPreviewReplacements(payload *PublicationRewardPreviewFormPayload, sysConfig *systemConfigSnapshot, attachments []*multipart.FileHeader) (map[string]string, error) {
	if payload == nil {
		return nil, fmt.Errorf("invalid form payload")
	}

	if sysConfig == nil {
		sysConfig = &systemConfigSnapshot{}
	}

	totalAmount := parseFormFloat(payload.FormData.TotalAmount)
	publicationDate := resolveFormPublicationDate(&payload.FormData)
	publicationYearText := derivePublicationYear(publicationDate, payload.FormData.JournalYear)

	replacements := map[string]string{
		"{{date_th}}":            utils.FormatThaiDate(time.Now()),
		"{{applicant_name}}":     buildPreviewApplicantName(payload.Applicant),
		"{{date_of_employment}}": formatThaiDateFromString(payload.Applicant.DateOfEmployment),
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
			lines = append(lines, "☑ "+name+" — จำนวน 1 ฉบับ")
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
		lines = append(lines, "☑ "+name+" — จำนวน 1 ฉบับ")
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

// --- helper: replace inside <w:t> and keep styles; supports \n as <w:br/> ---
func replacePlaceholdersInWText(xmlBytes []byte, repl map[string]string) []byte {
	dec := xml.NewDecoder(bytes.NewReader(xmlBytes))
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)

	inWText := false

	// Small helper: write <w:br/>
	writeBr := func() {
		_ = enc.EncodeToken(xml.StartElement{Name: xml.Name{Space: "w", Local: "br"}})
		_ = enc.EncodeToken(xml.EndElement{Name: xml.Name{Space: "w", Local: "br"}})
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// If we can’t parse, don’t risk corrupting the part
			return xmlBytes
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" && (t.Name.Space == "w" || t.Name.Space == "") {
				inWText = true
			}
			if err := enc.EncodeToken(t); err != nil {
				return xmlBytes
			}
		case xml.EndElement:
			if t.Name.Local == "t" && (t.Name.Space == "w" || t.Name.Space == "") {
				inWText = false
			}
			if err := enc.EncodeToken(t); err != nil {
				return xmlBytes
			}
		case xml.CharData:
			if !inWText {
				if err := enc.EncodeToken(t); err != nil {
					return xmlBytes
				}
				continue
			}
			// Only text inside <w:t>
			s := string(t)
			// exact placeholder replacements
			for ph, v := range repl {
				if strings.Contains(s, ph) {
					s = strings.ReplaceAll(s, ph, v)
				}
			}
			// Handle explicit newlines as <w:br/> (Word-friendly)
			if strings.Contains(s, "\n") {
				lines := strings.Split(s, "\n")
				// Write first line as text
				if err := enc.EncodeToken(xml.CharData([]byte(lines[0]))); err != nil {
					return xmlBytes
				}
				// For each subsequent line: <w:br/><w:t>line</w:t> is not allowed here,
				// but inside the same <w:t> we can insert <w:br/> tokens between text nodes.
				for i := 1; i < len(lines); i++ {
					writeBr()
					if err := enc.EncodeToken(xml.CharData([]byte(lines[i]))); err != nil {
						return xmlBytes
					}
				}
				continue
			}
			if err := enc.EncodeToken(xml.CharData([]byte(s))); err != nil {
				return xmlBytes
			}
		default:
			if err := enc.EncodeToken(t); err != nil {
				return xmlBytes
			}
		}
	}
	if err := enc.Flush(); err != nil {
		return xmlBytes
	}
	return out.Bytes()
}

func generatePublicationRewardPDF(replacements map[string]string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "publication-preview-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	docxPath := filepath.Join(tmpDir, "publication_reward_preview.docx")

	// Write DOCX with replacements
	if err := fillDocxTemplate("templates/publication_reward_template.docx", docxPath, replacements); err != nil {
		return nil, fmt.Errorf("failed to fill template: %v", err)
	}

	// Which binary to use (allow override via env LIBREOFFICE)
	lo := os.Getenv("LIBREOFFICE")
	if lo == "" {
		lo = "soffice"
	}

	// Convert DOCX → PDF
	pdfPath, err := convertDocxToPDF(lo, docxPath, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("DOCX to PDF conversion failed: %v", err)
	}

	// Read back PDF bytes
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read generated pdf: %v", err)
	}
	return pdfBytes, nil
}

func lookupLibreOfficeBinary() (string, error) {
	if path, err := exec.LookPath("soffice"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("libreoffice"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("libreoffice (soffice) binary not found in PATH")
}

// replacePlaceholdersRobust replaces {{...}} even when Word splits tokens across runs.
// It merges the placeholder span into the first run (preserving its style) and clears the rest.
func replacePlaceholdersRobust(xmlBytes []byte, repl map[string]string) []byte {
	dec := xml.NewDecoder(bytes.NewReader(xmlBytes))
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)

	type runText struct {
		startEl xml.StartElement // <w:t ...>
		text    string           // text inside this w:t (raw)
		endEl   xml.EndElement   // </w:t>
		// We also keep any tokens between runs so we can re-emit them
		preTokens  []xml.Token
		postTokens []xml.Token
	}

	// We will buffer tokens so we can look back across adjacent <w:t> runs.
	var bufTokens []xml.Token
	flush := func() {
		for _, tk := range bufTokens {
			_ = enc.EncodeToken(tk)
		}
		bufTokens = bufTokens[:0]
	}

	// Helpers to know we’re in <w:t>
	inWText := false

	// Sliding window of recent runs to detect split placeholders
	var runs []runText

	// Regex for a full placeholder
	phRe := regexp.MustCompile(`\{\{[a-zA-Z0-9_]+\}\}`)

	// Emit runs back out (used after we maybe modify one)
	emitRuns := func(rs []runText) {
		for _, r := range rs {
			// preTokens (usually </w:r> <w:r> etc.)
			for _, tk := range r.preTokens {
				_ = enc.EncodeToken(tk)
			}
			// <w:t>
			_ = enc.EncodeToken(r.startEl)
			// text
			if r.text != "" {
				_ = enc.EncodeToken(xml.CharData([]byte(r.text)))
			}
			// </w:t>
			_ = enc.EncodeToken(r.endEl)
			// postTokens
			for _, tk := range r.postTokens {
				_ = enc.EncodeToken(tk)
			}
		}
	}

	// Try to join consecutive runs’ text and replace placeholders across them.
	tryReplaceAcrossRuns := func() bool {
		if len(runs) == 0 {
			return false
		}
		// Join all run texts
		joined := ""
		for _, r := range runs {
			joined += r.text
		}
		// For each placeholder we know
		changed := false
		for ph, v := range repl {
			if !strings.Contains(joined, ph) {
				continue
			}
			changed = true
			joined = strings.ReplaceAll(joined, ph, v)
		}
		if !changed {
			return false
		}
		// If changed, we’ll put the entire joined text back into the FIRST run,
		// and blank the others (to keep structure/styles of the first run).
		runs[0].text = joined
		for i := 1; i < len(runs); i++ {
			runs[i].text = ""
		}
		return true
	}

	// Stream parse
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// On parse failure, fall back to original bytes
			return xmlBytes
		}

		switch t := tok.(type) {
		case xml.StartElement:
			// Record tokens so we can rebuild if needed
			if inWText {
				// inside <w:t>, just pass through
				bufTokens = append(bufTokens, t)
				continue
			}
			if t.Name.Local == "t" && (t.Name.Space == "w" || t.Name.Space == "") {
				inWText = true
				// start a new run
				runs = append(runs, runText{startEl: t})
				continue
			}
			// if we’re between runs, we may be crossing run boundaries (</w:r><w:r> etc.)
			// capture these boundary tokens so we can re-emit around runs later
			if len(runs) > 0 && !inWText {
				runs[len(runs)-1].postTokens = append(runs[len(runs)-1].postTokens, t)
			} else {
				bufTokens = append(bufTokens, t)
			}

		case xml.EndElement:
			if inWText && t.Name.Local == "t" && (t.Name.Space == "w" || t.Name.Space == "") {
				// close current run’s text
				inWText = false
				runs[len(runs)-1].endEl = t

				// If the current run text contains a full placeholder already, we may replace now,
				// but the real win is to consider multi-run spans below after we see what’s next.
				// If the text itself contains any "{{" or "}}", we hold off until we see boundaries.

				// Look-ahead logic: when we hit a boundary that is NOT part of adjacent w:t,
				// or when the buffered boundary clearly ends a placeholder, we try replacement.
				// Here we don’t know yet—so just continue and the next tokens decide.
				continue
			}

			if len(runs) > 0 && !inWText {
				// We’re between runs; record boundary token
				runs[len(runs)-1].postTokens = append(runs[len(runs)-1].postTokens, t)

				// Heuristic: if we are closing a paragraph or a run container, try replacement now.
				name := t.Name.Local
				if name == "r" || name == "p" || name == "tr" {
					// Try replace across collected runs
					_ = tryReplaceAcrossRuns()
					// Emit everything so far
					flush()
					emitRuns(runs)
					runs = runs[:0]
				}
			} else {
				bufTokens = append(bufTokens, t)
			}

		case xml.CharData:
			if inWText {
				// accumulate text for the current run
				txt := string([]byte(t))
				// We only replace single-run placeholders here if present and not cross-run
				// (multi-run case handled by tryReplaceAcrossRuns).
				if phRe.MatchString(txt) {
					for ph, v := range repl {
						txt = strings.ReplaceAll(txt, ph, v)
					}
				}
				runs[len(runs)-1].text += txt
			} else {
				bufTokens = append(bufTokens, t)
			}

		default:
			if len(runs) > 0 && !inWText {
				runs[len(runs)-1].postTokens = append(runs[len(runs)-1].postTokens, t)
			} else {
				bufTokens = append(bufTokens, t)
			}
		}
	}

	// Final flush: replace across any leftover runs and emit
	if len(runs) > 0 {
		_ = tryReplaceAcrossRuns()
		flush()
		emitRuns(runs)
		runs = runs[:0]
	} else {
		flush()
	}

	_ = enc.Flush()
	return out.Bytes()
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

		// Only touch Wordprocessing XML parts; leave rels/[Content_Types].xml alone.
		if strings.HasSuffix(strings.ToLower(file.Name), ".xml") && strings.HasPrefix(file.Name, "word/") {
			data = replacePlaceholdersRobust(data, replacements)
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
