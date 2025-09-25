package controllers

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type publicationSummaryRequest struct {
	Placeholders  map[string]interface{} `json:"placeholders"`
	DocumentLines []string               `json:"documentLines"`
}

func resolveTemplatePath() (string, error) {
	candidates := make([]string, 0, 3)

	if explicit := strings.TrimSpace(os.Getenv("PUBLICATION_TEMPLATE_PATH")); explicit != "" {
		candidates = append(candidates, explicit)
	}

	wd, _ := os.Getwd()
	if wd != "" {
		candidates = append(candidates,
			filepath.Join(wd, "frontend_project_fund", "public", "templates", "publication_reward_template.docx"),
			filepath.Join(wd, "แบบฟอร์มสมัครรับ เงินรางวัลตีพิมพ์ - 2568.docx"),
		)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", errors.New("template not found")
}

func sanitizePlaceholderValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func buildReplacementMap(placeholders map[string]interface{}, documentLines []string) map[string]string {
	replacements := make(map[string]string, len(placeholders)+1)

	for key, value := range placeholders {
		if key == "document_line" {
			continue
		}
		replacements["{{"+key+"}}"] = sanitizePlaceholderValue(value)
	}

	if len(documentLines) == 0 {
		documentLines = []string{"☑ ไม่พบรายการเอกสารแนบ"}
	}

	replacements["{{document_line}}"] = strings.Join(documentLines, "\n")
	return replacements
}

func replacePlaceholdersInDocx(templatePath string, replacements map[string]string) ([]byte, error) {
	reader, err := zip.OpenReader(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open template: %w", err)
	}
	defer reader.Close()

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	defer zipWriter.Close()

	replacer := strings.NewReplacer(func() []string {
		pairs := make([]string, 0, len(replacements)*2)
		for key, value := range replacements {
			pairs = append(pairs, key, value)
		}
		return pairs
	}()...)

	for _, file := range reader.File {
		src, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to read entry %s: %w", file.Name, err)
		}

		w, err := zipWriter.CreateHeader(&file.FileHeader)
		if err != nil {
			src.Close()
			return nil, fmt.Errorf("failed to create entry %s: %w", file.Name, err)
		}

		if strings.HasPrefix(file.Name, "word/") {
			data, err := io.ReadAll(src)
			src.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read entry %s: %w", file.Name, err)
			}
			if _, err := w.Write([]byte(replacer.Replace(string(data)))); err != nil {
				return nil, fmt.Errorf("failed to write entry %s: %w", file.Name, err)
			}
			continue
		}

		if _, err := io.Copy(w, src); err != nil {
			src.Close()
			return nil, fmt.Errorf("failed to copy entry %s: %w", file.Name, err)
		}
		src.Close()
	}

	return buffer.Bytes(), nil
}

func resolveLibreOfficeBinary() (string, error) {
	candidates := []string{"soffice", "libreoffice"}
	if runtime.GOOS == "windows" {
		candidates = []string{
			`C:\\Program Files\\LibreOffice\\program\\soffice.exe`,
			`C:\\Program Files (x86)\\LibreOffice\\program\\soffice.exe`,
			"soffice.exe",
		}
	}

	for _, candidate := range candidates {
		if strings.Contains(candidate, "/") || strings.Contains(candidate, "\\") {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}

	return "", errors.New("LibreOffice CLI not found")
}

func convertDocxToPDF(docxBytes []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "publication-summary-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	docxPath := filepath.Join(tmpDir, fmt.Sprintf("summary-%d.docx", time.Now().UnixNano()))
	if err := os.WriteFile(docxPath, docxBytes, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write temp docx: %w", err)
	}

	libreOffice, err := resolveLibreOfficeBinary()
	if err != nil {
		return nil, fmt.Errorf("LIBREOFFICE_NOT_INSTALLED: %w", err)
	}

	cmd := exec.Command(libreOffice, "--headless", "--convert-to", "pdf", "--outdir", tmpDir, docxPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("DOCX_TO_PDF_FAILED: %w (output: %s)", err, string(output))
	}

	pdfPath := strings.TrimSuffix(docxPath, filepath.Ext(docxPath)) + ".pdf"
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read generated pdf: %w", err)
	}

	return pdfBytes, nil
}

func GeneratePublicationSummary(c *gin.Context) {
	var request publicationSummaryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_PAYLOAD",
			"details": "placeholders object is required",
		})
		return
	}

	if len(request.Placeholders) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_PAYLOAD",
			"details": "placeholders object is required",
		})
		return
	}

	templatePath, err := resolveTemplatePath()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TEMPLATE_NOT_AVAILABLE"})
		return
	}

	replacements := buildReplacementMap(request.Placeholders, request.DocumentLines)
	docxBytes, err := replacePlaceholdersInDocx(templatePath, replacements)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "SUMMARY_GENERATION_FAILED",
			"details": err.Error(),
		})
		return
	}

	pdfBytes, err := convertDocxToPDF(docxBytes)
	if err != nil {
		errMsg := err.Error()
		status := http.StatusInternalServerError
		errorCode := "DOCX_TO_PDF_FAILED"
		if strings.HasPrefix(errMsg, "LIBREOFFICE_NOT_INSTALLED") {
			status = http.StatusServiceUnavailable
			errorCode = "LIBREOFFICE_NOT_INSTALLED"
		}
		c.JSON(status, gin.H{
			"error":   errorCode,
			"details": errMsg,
		})
		return
	}

	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}
