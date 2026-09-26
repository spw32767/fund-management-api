package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type classificationInput struct {
	ID              uint64     `gorm:"column:id"`
	Title           *string    `gorm:"column:title"`
	Abstract        *string    `gorm:"column:abstract"`
	AuthKeywords    []byte     `gorm:"column:authkeywords"`
	Category        *uint64    `gorm:"column:category"`
	Confidence      *string    `gorm:"column:classification_confidence"`
	Model           *string    `gorm:"column:classification_model"`
	TaxonomyVersion *string    `gorm:"column:classification_taxonomy_version"`
	ClassifiedAt    *time.Time `gorm:"column:classified_at"`
}

func classificationTable(source string) (string, bool) {
	switch source {
	case "benchmark":
		return "scopus_benchmark_documents", true
	case "faculty":
		return "scopus_documents", true
	default:
		return "", false
	}
}

func classificationScope(c *gin.Context) (string, string, *int, error) {
	source := c.Query("source")
	table, ok := classificationTable(source)
	if !ok {
		return "", "", nil, errors.New("source must be benchmark or faculty")
	}
	scope := c.DefaultQuery("scope", "unprocessed")
	if scope != "unprocessed" && scope != "all" {
		return "", "", nil, errors.New("scope must be unprocessed or all")
	}
	var year *int
	if raw := strings.TrimSpace(c.Query("year")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1900 || n > time.Now().Year()+1 {
			return "", "", nil, errors.New("invalid publication year")
		}
		year = &n
	}
	return table, scope, year, nil
}

func scopedClassificationQuery(db *gorm.DB, table, scope string, year *int) *gorm.DB {
	q := db.Table(table)
	if scope == "unprocessed" {
		q = q.Where("classified_at IS NULL")
	}
	if year != nil {
		if table == "scopus_benchmark_documents" {
			q = q.Where("COALESCE(pub_year, YEAR(cover_date)) = ?", *year)
		} else {
			q = q.Where("YEAR(cover_date) = ?", *year)
		}
	}
	return q
}

func classificationHash(d classificationInput) string {
	values, _ := json.Marshal([]any{d.Title, d.Abstract, string(d.AuthKeywords)})
	sum := sha256.Sum256(values)
	return hex.EncodeToString(sum[:])
}

func classificationCategories() ([]models.PaperCategory, string, error) {
	var categories []models.PaperCategory
	err := config.DB.Where("is_active = ?", true).Order("display_order ASC, category_id ASC").Find(&categories).Error
	if err == nil && len(categories) == 0 {
		err = errors.New("no active paper categories")
	}
	return categories, paperTaxonomyVersion(categories), err
}

func ListPaperClassificationYears(c *gin.Context) {
	table, ok := classificationTable(c.Query("source"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source must be benchmark or faculty"})
		return
	}
	yearExpr := "YEAR(cover_date)"
	if table == "scopus_benchmark_documents" {
		yearExpr = "COALESCE(pub_year, YEAR(cover_date))"
	}
	var years []struct {
		Year  int   `gorm:"column:year" json:"year"`
		Count int64 `gorm:"column:count" json:"count"`
	}
	if err := config.DB.Table(table).
		Select(yearExpr+" AS year, COUNT(*) AS count").
		Where(yearExpr+" BETWEEN ? AND ?", 1900, time.Now().Year()+1).
		Group(yearExpr).Order("year DESC").Scan(&years).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list publication years"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"source": c.Query("source"), "years": years})
}

func PreviewPaperClassification(c *gin.Context) {
	table, scope, year, err := classificationScope(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var count int64
	if err := scopedClassificationQuery(config.DB, table, scope, year).Count(&count).Error; err != nil {
		c.JSON(500, gin.H{"error": "failed to count documents"})
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 100000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
		return
	}
	const pageSize = 20
	var documents []struct {
		ID    uint64  `gorm:"column:id" json:"id"`
		Title *string `gorm:"column:title" json:"title"`
		DOI   *string `gorm:"column:doi" json:"doi"`
	}
	if err := scopedClassificationQuery(config.DB, table, scope, year).
		Select("id, title, doi").
		Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&documents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list documents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"source": c.Query("source"), "scope": scope, "year": year, "count": count, "page": page, "page_size": pageSize, "documents": documents})
}

func ListPaperClassificationRuns(c *gin.Context) {
	var runs []models.PaperClassificationRun
	if err := config.DB.Order("created_at DESC").Limit(30).Find(&runs).Error; err != nil {
		c.JSON(500, gin.H{"error": "failed to list runs"})
		return
	}
	c.JSON(200, gin.H{"runs": runs})
}

func GetPaperClassificationRun(c *gin.Context) {
	var run models.PaperClassificationRun
	if err := config.DB.First(&run, "run_id = ?", c.Param("id")).Error; err != nil {
		c.JSON(404, gin.H{"error": "run not found"})
		return
	}
	c.JSON(200, run)
}

func ListPaperClassificationItems(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	status := c.Query("status")
	q := config.DB.Where("run_id = ?", c.Param("id"))
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Model(&models.PaperClassificationRunItem{}).Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"error": "failed to count items"})
		return
	}
	var items []models.PaperClassificationRunItem
	if err := q.Order("id ASC").Offset((page - 1) * 50).Limit(50).Find(&items).Error; err != nil {
		c.JSON(500, gin.H{"error": "failed to list items"})
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total, "page": page})
}

func StartPaperClassificationRun(c *gin.Context) {
	var request struct {
		Source string `json:"source"`
		Scope  string `json:"scope"`
		Year   *int   `json:"year"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	query := c.Request.URL.Query()
	query.Set("source", request.Source)
	query.Set("scope", request.Scope)
	if request.Year != nil {
		query.Set("year", strconv.Itoa(*request.Year))
	}
	c.Request.URL.RawQuery = query.Encode()
	table, scope, year, err := classificationScope(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	_, version, err := classificationCategories()
	if err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	userValue, ok := c.Get("userID")
	userID, valid := userValue.(int)
	if !ok || !valid {
		c.JSON(401, gin.H{"error": "authentication required"})
		return
	}
	now := time.Now()
	run := models.PaperClassificationRun{RunID: uuid.NewString(), Source: request.Source, PublicationYear: year, Scope: scope, TaxonomyVersion: version, Status: "running", UserID: userID, CreatedAt: now, StartedAt: &now}
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		var control struct {
			ActiveRunID *string `gorm:"column:active_run_id"`
		}
		if e := tx.Table("paper_classification_control").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = 1").Take(&control).Error; e != nil {
			return e
		}
		if control.ActiveRunID != nil {
			return errors.New("another classification run is active")
		}
		var documents []classificationInput
		if e := scopedClassificationQuery(tx, table, scope, year).Select("id,title,abstract,authkeywords").Order("id ASC").Find(&documents).Error; e != nil {
			return e
		}
		run.Total = len(documents)
		if e := tx.Create(&run).Error; e != nil {
			return e
		}
		for start := 0; start < len(documents); start += 200 {
			end := start + 200
			if end > len(documents) {
				end = len(documents)
			}
			items := make([]models.PaperClassificationRunItem, 0, end-start)
			for _, d := range documents[start:end] {
				items = append(items, models.PaperClassificationRunItem{RunID: run.RunID, DocumentID: d.ID, TitleSnapshot: d.Title, InputHash: classificationHash(d), Status: "pending"})
			}
			if e := tx.Create(&items).Error; e != nil {
				return e
			}
		}
		return tx.Table("paper_classification_control").Where("id = 1").Update("active_run_id", run.RunID).Error
	})
	if err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	go processPaperClassificationRun(run.RunID)
	c.JSON(202, run)
}

func StopPaperClassificationRun(c *gin.Context) {
	result := config.DB.Model(&models.PaperClassificationRun{}).Where("run_id = ? AND status = ?", c.Param("id"), "running").Update("stop_requested", true)
	if result.Error != nil {
		c.JSON(500, gin.H{"error": "failed to stop run"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(409, gin.H{"error": "run is not active"})
		return
	}
	c.JSON(202, gin.H{"status": "stopping"})
}

func ResumePaperClassificationRun(c *gin.Context)      { resumePaperClassification(c, false) }
func RetryFailedPaperClassificationRun(c *gin.Context) { resumePaperClassification(c, true) }

func resumePaperClassification(c *gin.Context, retry bool) {
	id := c.Param("id")
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		var control struct {
			ActiveRunID *string `gorm:"column:active_run_id"`
		}
		if e := tx.Table("paper_classification_control").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = 1").Take(&control).Error; e != nil {
			return e
		}
		if control.ActiveRunID != nil {
			return errors.New("another classification run is active")
		}
		var run models.PaperClassificationRun
		if e := tx.First(&run, "run_id = ?", id).Error; e != nil {
			return e
		}
		if retry {
			if run.Status != "completed" && run.Status != "stopped" && run.Status != "interrupted" {
				return errors.New("run cannot be retried yet")
			}
			if e := tx.Model(&models.PaperClassificationRunItem{}).Where("run_id = ? AND status = ?", id, "failed").Updates(map[string]any{"status": "pending", "error_message": nil}).Error; e != nil {
				return e
			}
		} else if run.Status != "stopped" && run.Status != "interrupted" {
			return errors.New("run cannot be resumed")
		}
		if e := tx.Model(&run).Updates(map[string]any{"status": "running", "stop_requested": false, "completed_at": nil}).Error; e != nil {
			return e
		}
		return tx.Table("paper_classification_control").Where("id = 1").Update("active_run_id", id).Error
	})
	if err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	go processPaperClassificationRun(id)
	c.JSON(202, gin.H{"run_id": id, "status": "running"})
}

// RecoverPaperClassificationRuns is called after DB initialization. Interrupted work
// remains in the queue, and an operator can resume it from the admin page.
func RecoverPaperClassificationRuns() {
	if !config.DB.Migrator().HasTable("paper_classification_runs") {
		return
	}
	_ = config.DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&models.PaperClassificationRun{}).Where("status = ?", "running").Update("status", "interrupted").Error; e != nil {
			return e
		}
		if e := tx.Model(&models.PaperClassificationRunItem{}).Where("status = ?", "processing").Update("status", "pending").Error; e != nil {
			return e
		}
		return tx.Table("paper_classification_control").Where("id = 1").Update("active_run_id", nil).Error
	})
}

func processPaperClassificationRun(id string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("classification run %s panic: %v", id, recovered)
		}
		var run models.PaperClassificationRun
		_ = config.DB.First(&run, "run_id = ?", id).Error
		var pending int64
		_ = config.DB.Model(&models.PaperClassificationRunItem{}).Where("run_id = ? AND status IN ?", id, []string{"pending", "processing"}).Count(&pending).Error
		status := "completed"
		if run.StopRequested {
			status = "stopped"
		}
		if pending > 0 && !run.StopRequested {
			status = "interrupted"
		}
		now := time.Now()
		_ = config.DB.Model(&run).Updates(map[string]any{"status": status, "completed_at": now}).Error
		_ = config.DB.Table("paper_classification_control").Where("id = 1 AND active_run_id = ?", id).Update("active_run_id", nil).Error
	}()
	consecutiveFailures := 0
	for {
		var run models.PaperClassificationRun
		if err := config.DB.First(&run, "run_id = ?", id).Error; err != nil || run.StopRequested {
			return
		}
		var item models.PaperClassificationRunItem
		if err := config.DB.Where("run_id = ? AND status = ?", id, "pending").Order("id ASC").First(&item).Error; err != nil {
			return
		}
		if err := classifyRunItem(run, item); err != nil {
			message := err.Error()
			if len(message) > 4000 {
				message = message[:4000]
			}
			_ = config.DB.Model(&item).Updates(map[string]any{"status": "failed", "error_message": message, "completed_at": time.Now()}).Error
			consecutiveFailures++
			if consecutiveFailures >= 3 || strings.Contains(message, "taxonomy changed") {
				_ = config.DB.Model(&models.PaperClassificationRun{}).Where("run_id = ?", id).Update("stop_requested", true).Error
			}
		} else {
			consecutiveFailures = 0
		}
		var counts []struct {
			Status string
			Count  int
		}
		_ = config.DB.Model(&models.PaperClassificationRunItem{}).Select("status, COUNT(*) AS count").Where("run_id = ?", id).Group("status").Scan(&counts).Error
		update := map[string]any{"completed": 0, "preface": 0, "failed": 0}
		for _, count := range counts {
			if count.Status == "completed" || count.Status == "preface" {
				update["completed"] = update["completed"].(int) + count.Count
			}
			if count.Status == "preface" {
				update["preface"] = count.Count
			}
			if count.Status == "failed" {
				update["failed"] = count.Count
			}
		}
		_ = config.DB.Model(&models.PaperClassificationRun{}).Where("run_id = ?", id).Updates(update).Error
	}
}

func classifyRunItem(run models.PaperClassificationRun, item models.PaperClassificationRunItem) error {
	table, ok := classificationTable(run.Source)
	if !ok {
		return errors.New("invalid run source")
	}
	var d classificationInput
	if err := config.DB.Table(table).Where("id = ?", item.DocumentID).Take(&d).Error; err != nil {
		return err
	}
	if classificationHash(d) != item.InputHash {
		return errors.New("document changed after run was queued")
	}
	_ = config.DB.Model(&item).Updates(map[string]any{"status": "processing", "started_at": time.Now()}).Error
	categories, version, err := classificationCategories()
	if err != nil {
		return err
	}
	if version != run.TaxonomyVersion {
		return errors.New("taxonomy changed; start a new run")
	}
	confidence := "Preface"
	model := "qwen3-4b"
	taxonomyVersion := version
	var categoryID any
	var response json.RawMessage
	if d.Title != nil && strings.TrimSpace(*d.Title) != "" {
		taxonomy := make([]gin.H, 0, len(categories))
		for _, cat := range categories {
			taxonomy = append(taxonomy, gin.H{"code": cat.Code, "name": cat.Name, "description": cat.Description})
		}
		abstract := ""
		if d.Abstract != nil {
			abstract = *d.Abstract
		}
		payload, _ := json.Marshal(gin.H{"paper_id": fmt.Sprintf("%s:%d", run.Source, d.ID), "title": *d.Title, "abstract": abstract, "authkeywords": string(d.AuthKeywords), "categories": taxonomy, "taxonomy_version": version, "model": model, "verification_mode": "fewshot_candidate", "confidence_policy": "model_reported"})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		status, body, callErr := services.NewPaperAIClient().Classify(ctx, payload)
		if callErr != nil {
			return callErr
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("classification API returned %d: %.300s", status, string(body))
		}
		response = body
		var result struct {
			PrimaryCategoryCode *string `json:"primary_category_code"`
			Confidence          string  `json:"confidence"`
			Model               string  `json:"model"`
			TaxonomyVersion     string  `json:"taxonomy_version"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return err
		}
		confidence = result.Confidence
		if result.Model != "" {
			model = result.Model
		}
		if result.TaxonomyVersion != "" {
			taxonomyVersion = result.TaxonomyVersion
		}
		if result.PrimaryCategoryCode != nil {
			for _, cat := range categories {
				if cat.Code == *result.PrimaryCategoryCode {
					categoryID = cat.CategoryID
					break
				}
			}
		}
		if !isValidClassificationConfidence(confidence) || (confidence == "Preface" && categoryID != nil) || (confidence != "Preface" && categoryID == nil) || taxonomyVersion != version {
			return errors.New("classification response does not match active taxonomy")
		}
	} else {
		response = []byte(`{"reason":"missing title","confidence":"Preface"}`)
	}
	return config.DB.Transaction(func(tx *gorm.DB) error {
		var current classificationInput
		if err := tx.Table(table).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", d.ID).Take(&current).Error; err != nil {
			return err
		}
		if classificationHash(current) != item.InputHash {
			return errors.New("document changed during classification")
		}
		prior, _ := json.Marshal(gin.H{"category": current.Category, "confidence": current.Confidence, "model": current.Model, "taxonomy_version": current.TaxonomyVersion, "classified_at": current.ClassifiedAt})
		now := time.Now()
		if err := tx.Table(table).Where("id = ?", d.ID).Updates(map[string]any{"category": categoryID, "classification_confidence": confidence, "classification_model": model, "classification_taxonomy_version": taxonomyVersion, "classified_at": now}).Error; err != nil {
			return err
		}
		itemStatus := "completed"
		if confidence == "Preface" {
			itemStatus = "preface"
		}
		return tx.Model(&item).Updates(map[string]any{"status": itemStatus, "prior_json": string(prior), "result_json": string(response), "error_message": nil, "completed_at": now}).Error
	})
}
