package controllers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

const benchmarkHarvestTimeout = 6 * time.Hour

// POST /api/v1/admin/scopus/benchmark/affiliation/lookup
func AdminBenchmarkResolveAffiliation(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing affiliation name"})
		return
	}

	svc := services.NewScopusBenchmarkService(nil, nil)
	hits, err := svc.ResolveAffiliation(c.Request.Context(), body.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": hits})
}

// GET /api/v1/admin/scopus/benchmark/scopes
func AdminListBenchmarkScopes(c *gin.Context) {
	var scopes []models.ScopusBenchmarkScope
	if err := config.DB.Order("id ASC").Find(&scopes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": scopes})
}

// PUT /api/v1/admin/scopus/benchmark/scopes/:id
func AdminUpdateBenchmarkScope(c *gin.Context) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid scope id"})
		return
	}

	var body struct {
		Label        *string `json:"label"`
		AfID         *string `json:"af_id"`
		AffilCountry *string `json:"affil_country"`
		SubjectArea  *string `json:"subject_area"`
		ExtraQuery   *string `json:"extra_query"`
		Active       *bool   `json:"active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	var scope models.ScopusBenchmarkScope
	if err := config.DB.First(&scope, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "scope not found"})
		return
	}

	updates := map[string]interface{}{}
	if body.Label != nil {
		updates["label"] = strings.TrimSpace(*body.Label)
	}
	if body.AfID != nil {
		updates["af_id"] = strings.TrimSpace(*body.AfID)
	}
	if body.AffilCountry != nil {
		updates["affil_country"] = strings.TrimSpace(*body.AffilCountry)
	}
	if body.SubjectArea != nil {
		updates["subject_area"] = strings.TrimSpace(*body.SubjectArea)
	}
	if body.ExtraQuery != nil {
		updates["extra_query"] = strings.TrimSpace(*body.ExtraQuery)
	}
	if body.Active != nil {
		updates["active"] = *body.Active
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "no fields to update"})
		return
	}

	if err := config.DB.Model(&scope).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	config.DB.First(&scope, id)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": scope})
}

// POST /api/v1/admin/scopus/benchmark/counts/refresh?years_back=10
// Counts CS totals for every active scope (all-years + per-year) and stores snapshots.
func AdminRefreshBenchmarkCounts(c *gin.Context) {
	yearsBack := 0
	if v := strings.TrimSpace(c.Query("years_back")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			yearsBack = n
		}
	}

	var scopes []models.ScopusBenchmarkScope
	if err := config.DB.Where("active = 1").Order("id ASC").Find(&scopes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	svc := services.NewScopusBenchmarkService(nil, nil)
	ctx := c.Request.Context()

	results := make([]gin.H, 0, len(scopes))
	for i := range scopes {
		scope := &scopes[i]
		entry := gin.H{"scope_id": scope.ID, "code": scope.Code, "label": scope.Label}

		total, err := svc.CountScope(ctx, scope, nil)
		if err != nil {
			entry["error"] = err.Error()
			results = append(results, entry)
			continue
		}
		entry["total"] = total

		if yearsBack > 0 {
			currentYear := time.Now().Year()
			byYear := make([]gin.H, 0, yearsBack)
			for y := currentYear; y > currentYear-yearsBack; y-- {
				year := y
				n, err := svc.CountScope(ctx, scope, &year)
				if err != nil {
					byYear = append(byYear, gin.H{"year": year, "error": err.Error()})
					continue
				}
				byYear = append(byYear, gin.H{"year": year, "count": n})
			}
			entry["by_year"] = byYear
		}
		results = append(results, entry)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": results})
}

// POST /api/v1/admin/scopus/benchmark/harvest
// body: { "scope_id": 1, "years_back": 10 } or { "code": "university_kku", "year_from": 2015, "year_to": 2025 }
func AdminHarvestBenchmarkScope(c *gin.Context) {
	var body struct {
		ScopeID   uint64 `json:"scope_id"`
		Code      string `json:"code"`
		YearsBack int    `json:"years_back"`
		YearFrom  int    `json:"year_from"`
		YearTo    int    `json:"year_to"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	var scope models.ScopusBenchmarkScope
	q := config.DB
	switch {
	case body.ScopeID > 0:
		q = q.Where("id = ?", body.ScopeID)
	case strings.TrimSpace(body.Code) != "":
		q = q.Where("code = ?", strings.TrimSpace(body.Code))
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "scope_id or code is required"})
		return
	}
	if err := q.First(&scope).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "scope not found"})
		return
	}

	var yearFrom, yearTo *int
	if body.YearsBack > 0 {
		currentYear := time.Now().Year()
		from := currentYear - body.YearsBack + 1
		yearFrom, yearTo = &from, &currentYear
	} else if body.YearFrom > 0 || body.YearTo > 0 {
		if body.YearFrom > 0 {
			yf := body.YearFrom
			yearFrom = &yf
		}
		if body.YearTo > 0 {
			yt := body.YearTo
			yearTo = &yt
		}
	}

	svc := services.NewScopusBenchmarkService(nil, nil)
	activeRun, err := svc.GetActiveRun(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if activeRun != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "scopus benchmark harvest already running",
			"data":    activeRun,
		})
		return
	}

	scopeCopy := scope
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), benchmarkHarvestTimeout)
		defer cancel()
		if _, err := svc.HarvestScope(ctx, &scopeCopy, yearFrom, yearTo); err != nil {
			if errors.Is(err, services.ErrScopusBenchmarkHarvestRunning) {
				log.Printf("scopus benchmark harvest skipped: already running")
				return
			}
			log.Printf("scopus benchmark harvest failed for scope %s: %v", scopeCopy.Code, err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"summary": gin.H{"status": "running", "message": "benchmark harvest started", "scope": scope.Code},
	})
}

// GET /api/v1/admin/scopus/benchmark/runs
func AdminListBenchmarkRuns(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var total int64
	if err := config.DB.Model(&models.ScopusBenchmarkHarvestRun{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var runs []models.ScopusBenchmarkHarvestRun
	offset := (page - 1) * perPage
	if err := config.DB.Order("started_at DESC").Offset(offset).Limit(perPage).Find(&runs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    runs,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     perPage,
			"total_count":  total,
			"total_pages":  int((total + int64(perPage) - 1) / int64(perPage)),
			"has_next":     int64(offset+perPage) < total,
			"has_prev":     page > 1,
		},
	})
}

// GET /api/v1/admin/scopus/benchmark/comparison?years_back=10
// Compares faculty (derived, is_faculty within the university harvest) vs
// university vs country CS counts by year, using the latest count snapshots for
// university/country and the stored harvest for the derived faculty number.
func AdminGetBenchmarkComparison(c *gin.Context) {
	yearsBack := 10
	if v := strings.TrimSpace(c.Query("years_back")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			yearsBack = n
		}
	}
	currentYear := time.Now().Year()
	minYear := currentYear - yearsBack + 1

	var uni, country models.ScopusBenchmarkScope
	config.DB.Where("level = ?", "university").First(&uni)
	config.DB.Where("level = ?", "country").First(&country)

	// latest snapshot per year for a scope
	latestSnapshotByYear := func(scopeID uint64) map[int]int {
		type row struct {
			PubYear *int
			Total   int
		}
		var rows []row
		config.DB.Raw(`
			SELECT s.pub_year AS pub_year, s.total_results AS total
			FROM scopus_benchmark_count_snapshots s
			JOIN (
				SELECT pub_year, MAX(captured_at) AS mx
				FROM scopus_benchmark_count_snapshots
				WHERE scope_id = ? AND pub_year IS NOT NULL
				GROUP BY pub_year
			) latest ON latest.pub_year = s.pub_year AND latest.mx = s.captured_at
			WHERE s.scope_id = ?`, scopeID, scopeID).Scan(&rows)
		out := map[int]int{}
		for _, r := range rows {
			if r.PubYear != nil {
				out[*r.PubYear] = r.Total
			}
		}
		return out
	}

	uniByYear := latestSnapshotByYear(uni.ID)
	countryByYear := latestSnapshotByYear(country.ID)

	// derived faculty CS docs per year (distinct docs with a faculty author in the university scope)
	facultyByYear := map[int]int{}
	{
		type row struct {
			PubYear *int
			Cnt     int
		}
		var rows []row
		config.DB.Raw(`
			SELECT ms.pub_year AS pub_year, COUNT(DISTINCT da.document_id) AS cnt
			FROM scopus_benchmark_document_authors da
			JOIN scopus_benchmark_document_scopes ms ON ms.document_id = da.document_id
			WHERE da.is_faculty = 1 AND ms.scope_id = ?
			GROUP BY ms.pub_year`, uni.ID).Scan(&rows)
		for _, r := range rows {
			if r.PubYear != nil {
				facultyByYear[*r.PubYear] = r.Cnt
			}
		}
	}

	rows := make([]gin.H, 0, yearsBack)
	for y := currentYear; y >= minYear; y-- {
		rows = append(rows, gin.H{
			"year":       y,
			"faculty":    facultyByYear[y],
			"university": uniByYear[y],
			"country":    countryByYear[y],
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"years":            rows,
			"university_scope": uni,
			"country_scope":    country,
		},
	})
}
