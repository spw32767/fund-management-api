package controllers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func classifyPaperPayload(ctx context.Context, paperID any, title, abstract, content string, authKeywords any, model string) (int, json.RawMessage, []models.PaperCategory, error) {
	var categories []models.PaperCategory
	if err := config.DB.Where("is_active = ?", true).Order("display_order ASC, category_id ASC").Find(&categories).Error; err != nil {
		body, _ := json.Marshal(gin.H{"error": "failed to load paper categories"})
		return http.StatusInternalServerError, body, nil, nil
	}
	if len(categories) == 0 {
		return http.StatusConflict, json.RawMessage(`{"detail":"no active paper categories are configured"}`), categories, nil
	}
	taxonomy := make([]gin.H, 0, len(categories))
	for _, category := range categories {
		taxonomy = append(taxonomy, gin.H{"code": category.Code, "name": category.Name, "description": category.Description})
	}
	requestPayload := gin.H{
		"paper_id": paperID, "title": title, "abstract": abstract, "content": content,
		"categories": taxonomy, "taxonomy_version": paperTaxonomyVersion(categories),
	}
	if authKeywords != nil {
		requestPayload["authkeywords"] = authKeywords
	}
	if strings.TrimSpace(model) != "" {
		requestPayload["model"] = strings.TrimSpace(model)
	}
	payload, _ := json.Marshal(requestPayload)
	status, body, err := services.NewPaperAIClient().Classify(ctx, payload)
	return status, body, categories, err
}

const maxPaperPDFBytes = 25 * 1024 * 1024

func isValidClassificationConfidence(value string) bool {
	switch value {
	case "High", "Medium", "Low", "Preface":
		return true
	default:
		return false
	}
}

func beginPaperAIJob(c *gin.Context, jobType string) string {
	userIDValue, ok := c.Get("userID")
	userID, valid := userIDValue.(int)
	if !ok || !valid || userID <= 0 {
		return ""
	}
	now := time.Now()
	job := models.PaperAIJob{
		JobID: uuid.NewString(), UserID: userID, JobType: jobType,
		Status: "processing", StartedAt: &now,
	}
	if err := config.DB.Create(&job).Error; err != nil {
		return ""
	}
	return job.JobID
}

func finishPaperAIJob(jobID string, status int, body json.RawMessage, callErr error) {
	if jobID == "" {
		return
	}
	now := time.Now()
	updates := map[string]any{"completed_at": now}
	if callErr != nil || status < 200 || status >= 300 {
		updates["status"] = "failed"
		message := "AI service request failed"
		if callErr != nil {
			message = callErr.Error()
		} else if len(body) > 0 {
			message = string(body)
		}
		if len(message) > 4000 {
			message = message[:4000]
		}
		updates["error_message"] = message
	} else {
		updates["status"] = "completed"
		stored := body
		var compact map[string]any
		if json.Unmarshal(stored, &compact) == nil {
			if _, hasText := compact["text"]; hasText {
				delete(compact, "text")
				compact["text_omitted_from_job_log"] = true
			}
			stored, _ = json.Marshal(compact)
		}
		if len(stored) > 1024*1024 {
			stored = json.RawMessage(`{"result_omitted_from_job_log":true}`)
		}
		result := string(stored)
		updates["result_json"] = result
	}
	_ = config.DB.Model(&models.PaperAIJob{}).Where("job_id = ?", jobID).Updates(updates).Error
}

func GetPaperAIJob(c *gin.Context) {
	userID, _ := c.Get("userID")
	var job models.PaperAIJob
	if err := config.DB.Where("job_id = ? AND user_id = ?", c.Param("id"), userID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "paper AI job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func ListPaperCategories(c *gin.Context) {
	var categories []models.PaperCategory
	if err := config.DB.Where("is_active = ?", true).Order("display_order ASC, category_id ASC").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load paper categories"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"categories": categories, "taxonomy_version": paperTaxonomyVersion(categories)})
}

func paperTaxonomyVersion(categories []models.PaperCategory) string {
	latest := time.Time{}
	for _, category := range categories {
		if category.UpdatedAt.After(latest) {
			latest = category.UpdatedAt
		}
	}
	if latest.IsZero() {
		return "empty"
	}
	return latest.UTC().Format("20060102T150405Z")
}

func relayPaperAIResponse(c *gin.Context, status int, body json.RawMessage, err error) {
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "paper AI service unavailable", "detail": err.Error()})
		return
	}
	c.Data(status, "application/json; charset=utf-8", body)
}

func ExtractPaper(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "PDF file is required"})
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > maxPaperPDFBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "PDF must be between 1 byte and 25 MB"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open uploaded PDF"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPaperPDFBytes+1))
	if err != nil || len(data) > maxPaperPDFBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "failed to read PDF or file is too large"})
		return
	}
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "uploaded file is not a PDF"})
		return
	}
	jobID := beginPaperAIJob(c, "extract")
	status, body, callErr := services.NewPaperAIClient().Extract(c.Request.Context(), fileHeader.Filename, data)
	finishPaperAIJob(jobID, status, body, callErr)
	if jobID != "" {
		c.Header("X-Paper-AI-Job-ID", jobID)
	}
	relayPaperAIResponse(c, status, body, callErr)
}

func SummarizePaper(c *gin.Context) {
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, 600_001))
	if err != nil || len(payload) > 600_000 || !json.Valid(payload) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or oversized JSON body"})
		return
	}
	jobID := beginPaperAIJob(c, "summarize")
	status, body, callErr := services.NewPaperAIClient().Summarize(c.Request.Context(), payload)
	finishPaperAIJob(jobID, status, body, callErr)
	if jobID != "" {
		c.Header("X-Paper-AI-Job-ID", jobID)
	}
	relayPaperAIResponse(c, status, body, callErr)
}

func ClassifyPaper(c *gin.Context) {
	var request struct {
		PaperID      any    `json:"paper_id"`
		Title        string `json:"title"`
		Abstract     string `json:"abstract"`
		Content      string `json:"content"`
		AuthKeywords any    `json:"authkeywords"`
		Model        string `json:"model"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title and valid JSON are required"})
		return
	}
	jobID := beginPaperAIJob(c, "classify")
	status, body, categories, err := classifyPaperPayload(c.Request.Context(), request.PaperID, request.Title, request.Abstract, request.Content, request.AuthKeywords, request.Model)
	finishPaperAIJob(jobID, status, body, err)
	if jobID != "" {
		c.Header("X-Paper-AI-Job-ID", jobID)
	}
	if err != nil {
		relayPaperAIResponse(c, status, body, err)
		return
	}
	if status < 200 || status >= 300 {
		relayPaperAIResponse(c, status, body, err)
		return
	}
	var result map[string]any
	if json.Unmarshal(body, &result) == nil {
		if code, ok := result["primary_category_code"].(string); ok {
			for _, category := range categories {
				if category.Code == code {
					result["paper_category_id"] = category.CategoryID
					result["paper_category_name"] = category.Name
					break
				}
			}
		}
		c.JSON(status, result)
		return
	}
	c.Data(status, "application/json; charset=utf-8", body)
}

func ClassifyBenchmarkPaper(c *gin.Context) {
	var document models.ScopusBenchmarkDocument
	if err := config.DB.First(&document, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "benchmark document not found"})
		return
	}
	title := ""
	abstract := ""
	if document.Title != nil {
		title = strings.TrimSpace(*document.Title)
	}
	if document.Abstract != nil {
		abstract = strings.TrimSpace(*document.Abstract)
	}
	if title == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "benchmark document has no title"})
		return
	}
	jobID := beginPaperAIJob(c, "classify_benchmark")
	var authKeywords any
	if len(document.AuthKeywords) > 0 {
		authKeywords = string(document.AuthKeywords)
	}
	status, body, categories, err := classifyPaperPayload(c.Request.Context(), document.ID, title, abstract, "", authKeywords, "")
	finishPaperAIJob(jobID, status, body, err)
	if jobID != "" {
		c.Header("X-Paper-AI-Job-ID", jobID)
	}
	if err != nil || status < 200 || status >= 300 {
		relayPaperAIResponse(c, status, body, err)
		return
	}
	var result struct {
		PrimaryCategoryCode *string `json:"primary_category_code"`
		Confidence          string  `json:"confidence"`
		Model               string  `json:"model"`
		TaxonomyVersion     string  `json:"taxonomy_version"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "invalid classification response"})
		return
	}
	var categoryID uint64
	if result.PrimaryCategoryCode != nil {
		for _, category := range categories {
			if category.Code == *result.PrimaryCategoryCode {
				categoryID = category.CategoryID
				break
			}
		}
	}
	if !isValidClassificationConfidence(result.Confidence) || (result.Confidence == "Preface" && result.PrimaryCategoryCode != nil) || (result.Confidence != "Preface" && categoryID == 0) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "classification response does not match active taxonomy"})
		return
	}
	now := time.Now()
	var storedCategoryID any
	if categoryID != 0 {
		storedCategoryID = categoryID
	}
	if err := config.DB.Model(&models.ScopusBenchmarkDocument{}).Where("id = ?", document.ID).Updates(map[string]any{
		"category": storedCategoryID, "classification_confidence": result.Confidence,
		"classification_model": result.Model, "classification_taxonomy_version": result.TaxonomyVersion,
		"classified_at": now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save benchmark classification"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"document_id": document.ID, "paper_category_id": storedCategoryID,
		"primary_category_code": result.PrimaryCategoryCode, "confidence": result.Confidence,
		"model": result.Model, "taxonomy_version": result.TaxonomyVersion, "classified_at": now,
	})
}

func MatchPaper(c *gin.Context) {
	var request struct {
		DOI   string `json:"doi"`
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || (strings.TrimSpace(request.DOI) == "" && strings.TrimSpace(request.Title) == "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "doi or title is required"})
		return
	}
	normalizedDOI := normalizePaperDOI(request.DOI)
	normalizedTitle := normalizePaperTitle(request.Title)
	if len(normalizedTitle) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is too long"})
		return
	}
	titleToken := longestPaperTitleToken(normalizedTitle)
	titlePattern := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(titleToken)
	type candidate struct {
		Source     string  `json:"source"`
		ID         uint64  `json:"id"`
		Title      string  `json:"title"`
		DOI        string  `json:"doi"`
		MatchType  string  `json:"match_type"`
		MatchScore float64 `json:"match_score"`
	}
	candidates := make([]candidate, 0)
	if normalizedDOI != "" {
		doiVariants := []string{
			normalizedDOI,
			"https://doi.org/" + normalizedDOI,
			"http://doi.org/" + normalizedDOI,
			"https://dx.doi.org/" + normalizedDOI,
			"http://dx.doi.org/" + normalizedDOI,
			"doi:" + normalizedDOI,
		}
		var benchmark []struct {
			ID         uint64
			Title, DOI string
		}
		config.DB.Table("scopus_benchmark_documents").Select("id, COALESCE(title, '') AS title, COALESCE(doi, '') AS doi").
			Where("LOWER(TRIM(doi)) IN ?", doiVariants).Limit(10).Scan(&benchmark)
		for _, item := range benchmark {
			candidates = append(candidates, candidate{"scopus_benchmark_documents", item.ID, item.Title, item.DOI, "doi_exact", 1})
		}
		var rewards []struct {
			ID         uint64
			Title, DOI string
		}
		config.DB.Table("publication_reward_details").Select("detail_id AS id, paper_title AS title, doi").
			Where("LOWER(TRIM(doi)) IN ?", doiVariants).Limit(10).Scan(&rewards)
		for _, item := range rewards {
			candidates = append(candidates, candidate{"publication_reward_details", item.ID, item.Title, item.DOI, "doi_exact", 1})
		}
	}
	if len(candidates) == 0 && normalizedTitle != "" && titleToken != "" {
		var benchmark []struct {
			ID         uint64
			Title, DOI string
		}
		config.DB.Table("scopus_benchmark_documents").Select("id, COALESCE(title, '') AS title, COALESCE(doi, '') AS doi").
			Where("LOWER(title) LIKE ?", "%"+titlePattern+"%").Limit(100).Scan(&benchmark)
		for _, item := range benchmark {
			score := paperTitleSimilarity(normalizedTitle, normalizePaperTitle(item.Title))
			if score >= 0.82 {
				matchType := "title_similar"
				if score == 1 {
					matchType = "title_exact"
				}
				candidates = append(candidates, candidate{"scopus_benchmark_documents", item.ID, item.Title, item.DOI, matchType, score})
			}
		}
		var rewards []struct {
			ID         uint64
			Title, DOI string
		}
		config.DB.Table("publication_reward_details").Select("detail_id AS id, paper_title AS title, doi").
			Where("LOWER(paper_title) LIKE ?", "%"+titlePattern+"%").Limit(100).Scan(&rewards)
		for _, item := range rewards {
			score := paperTitleSimilarity(normalizedTitle, normalizePaperTitle(item.Title))
			if score >= 0.82 {
				matchType := "title_similar"
				if score == 1 {
					matchType = "title_exact"
				}
				candidates = append(candidates, candidate{"publication_reward_details", item.ID, item.Title, item.DOI, matchType, score})
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"normalized_doi": normalizedDOI, "candidates": candidates, "requires_confirmation": len(candidates) > 0})
}

func normalizePaperTitle(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

func longestPaperTitleToken(value string) string {
	longest := ""
	for _, token := range strings.Fields(value) {
		if len([]rune(token)) > len([]rune(longest)) {
			longest = token
		}
	}
	return longest
}

func paperTitleSimilarity(left, right string) float64 {
	if left == "" || right == "" {
		return 0
	}
	if left == right {
		return 1
	}
	leftSet := make(map[string]struct{})
	rightSet := make(map[string]struct{})
	for _, token := range strings.Fields(left) {
		leftSet[token] = struct{}{}
	}
	for _, token := range strings.Fields(right) {
		rightSet[token] = struct{}{}
	}
	intersection := 0
	for token := range leftSet {
		if _, ok := rightSet[token]; ok {
			intersection++
		}
	}
	union := len(leftSet) + len(rightSet) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func normalizePaperDOI(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"https://doi.org/", "http://doi.org/", "doi:"} {
		value = strings.TrimPrefix(value, prefix)
	}
	return strings.TrimSpace(value)
}
