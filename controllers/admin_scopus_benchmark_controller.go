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

// benchmarkYearBounds resolves the [from, to] year window from query params.
// Range (year_from/year_to) takes precedence; otherwise years_back (defaulting to
// defaultYearsBack) is used relative to the current year.
func benchmarkYearBounds(c *gin.Context, defaultYearsBack int) (int, int) {
	yf, _ := strconv.Atoi(strings.TrimSpace(c.Query("year_from")))
	yt, _ := strconv.Atoi(strings.TrimSpace(c.Query("year_to")))
	if yf > 0 || yt > 0 {
		if yt == 0 {
			yt = time.Now().Year()
		}
		if yf == 0 {
			yf = yt
		}
		if yf > yt {
			yf, yt = yt, yf
		}
		return yf, yt
	}
	yb := defaultYearsBack
	if v := strings.TrimSpace(c.Query("years_back")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			yb = n
		}
	}
	to := time.Now().Year()
	return to - yb + 1, to
}

// GET /api/v1/admin/scopus/benchmark/scopes/:id/year-range
// Detects the earliest/latest publication year for a scope via Scopus sorting.
func AdminDetectBenchmarkYearRange(c *gin.Context) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid scope id"})
		return
	}
	var scope models.ScopusBenchmarkScope
	if err := config.DB.First(&scope, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "scope not found"})
		return
	}

	svc := services.NewScopusBenchmarkService(nil, nil)
	first, last, err := svc.DetectYearRange(c.Request.Context(), &scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"first_year": first, "last_year": last},
	})
}

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

// POST /api/v1/admin/scopus/benchmark/counts/refresh?year_from=2015&year_to=2025
// (or ?years_back=10). Counts CS totals for every active scope within the
// selected window and stores per-year snapshots.
func AdminRefreshBenchmarkCounts(c *gin.Context) {
	yearFrom, yearTo := benchmarkYearBounds(c, 10)

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

		total, counts, err := svc.CountScopeRange(ctx, scope, yearFrom, yearTo)
		if err != nil {
			entry["error"] = err.Error()
			results = append(results, entry)
			continue
		}
		entry["total"] = total

		byYear := make([]gin.H, 0, yearTo-yearFrom+1)
		for y := yearTo; y >= yearFrom; y-- {
			byYear = append(byYear, gin.H{"year": y, "count": counts[y]})
		}
		entry["by_year"] = byYear
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

// POST /api/v1/admin/scopus/benchmark/runs/:id/cancel
// Flags a running harvest as cancelled. A live run stops within one page; a stale
// run (whose process died) is simply cleaned up.
func AdminCancelBenchmarkRun(c *gin.Context) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid run id"})
		return
	}

	now := time.Now()
	res := config.DB.Model(&models.ScopusBenchmarkHarvestRun{}).
		Where("id = ? AND status = ?", id, "running").
		Updates(map[string]interface{}{
			"status":        "cancelled",
			"finished_at":   now,
			"error_message": "cancelled by user",
		})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "no running run to cancel"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "cancellation requested"})
}

// GET /api/v1/admin/scopus/benchmark/comparison?years_back=10
// Compares faculty vs university (KKU) vs country (Thailand) CS counts by year,
// all sourced from the latest count snapshots per scope.
func AdminGetBenchmarkComparison(c *gin.Context) {
	yearFrom, yearTo := benchmarkYearBounds(c, 10)

	var faculty, uni, country models.ScopusBenchmarkScope
	config.DB.Where("level = ?", "faculty").First(&faculty)
	config.DB.Where("level = ?", "university").First(&uni)
	config.DB.Where("level = ?", "country").First(&country)

	// latest snapshot per year for a scope, keeping the count AND its captured_at so
	// the UI can distinguish a real zero snapshot from a missing one and show a
	// per-level data date (§4/§9 A). A NULL captured_at is reported as no date.
	type snapMeta struct {
		total      int
		exists     bool
		snapshotAt *time.Time
	}
	latestSnapshotByYear := func(scopeID uint64) map[int]snapMeta {
		type row struct {
			PubYear    *int
			Total      int
			CapturedAt *time.Time
		}
		var rows []row
		// pick the newest snapshot per year using MAX(id) — deterministic even if
		// two snapshots land in the same second (id is a monotonic autoincrement).
		config.DB.Raw(`
			SELECT s.pub_year AS pub_year, s.total_results AS total, s.captured_at AS captured_at
			FROM scopus_benchmark_count_snapshots s
			JOIN (
				SELECT pub_year, MAX(id) AS mx
				FROM scopus_benchmark_count_snapshots
				WHERE scope_id = ? AND pub_year IS NOT NULL
				GROUP BY pub_year
			) latest ON latest.pub_year = s.pub_year AND latest.mx = s.id
			WHERE s.scope_id = ?`, scopeID, scopeID).Scan(&rows)
		out := map[int]snapMeta{}
		for _, r := range rows {
			if r.PubYear != nil {
				out[*r.PubYear] = snapMeta{total: r.Total, exists: true, snapshotAt: r.CapturedAt}
			}
		}
		return out
	}

	facultyByYear := latestSnapshotByYear(faculty.ID)
	uniByYear := latestSnapshotByYear(uni.ID)
	countryByYear := latestSnapshotByYear(country.ID)

	svc := services.NewScopusBenchmarkService(nil, nil)
	facultyCoverage, err := svc.FacultyMetricCoverage(c.Request.Context(), yearFrom, yearTo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	rows := make([]gin.H, 0, yearTo-yearFrom+1)
	missingFacultyYears := make(map[int]struct{}, len(facultyCoverage.BenchmarkYearsMissing))
	for _, year := range facultyCoverage.BenchmarkYearsMissing {
		missingFacultyYears[year] = struct{}{}
	}

	// Additive per-year/per-level snapshot metadata so the report can pick a report
	// year deterministically and label per-level data dates (handoff §4/§9 A). All
	// existing fields above are preserved unchanged for backward compatibility.
	yearMeta := gin.H{}
	snapAt := func(meta snapMeta) interface{} {
		if meta.snapshotAt == nil {
			return nil
		}
		return meta.snapshotAt.UTC().Format(time.RFC3339)
	}
	for y := yearTo; y >= yearFrom; y-- {
		facultySnap := facultyByYear[y]
		uniSnap := uniByYear[y]
		countrySnap := countryByYear[y]

		var facultyTotal interface{}
		_, benchmarkMissing := missingFacultyYears[y]
		facultyUsable := facultyCoverage.Ready && !benchmarkMissing
		if facultyUsable {
			facultyTotal = facultySnap.total
		}
		rows = append(rows, gin.H{
			"year":       y,
			"faculty":    facultyTotal,
			"university": uniSnap.total,
			"country":    countrySnap.total,
		})

		// Faculty status: available only when a snapshot exists AND the verified
		// metric is ready for this year; blocked when a snapshot exists but the
		// metric is not usable (not ready / incomplete KKU docs / active harvest);
		// missing when there is no snapshot at all.
		facultyStatus := "missing"
		facultyReason := "no faculty snapshot for this year"
		if facultySnap.exists {
			if facultyUsable {
				facultyStatus = "available"
				facultyReason = ""
			} else {
				facultyStatus = "blocked"
				facultyReason = "faculty metric not ready or KKU benchmark documents incomplete for this year"
			}
		}
		uniStatus := "missing"
		if uniSnap.exists {
			uniStatus = "available"
		}
		countryStatus := "missing"
		if countrySnap.exists {
			countryStatus = "available"
		}

		yearMeta[strconv.Itoa(y)] = gin.H{
			"faculty":    gin.H{"status": facultyStatus, "snapshot_exists": facultySnap.exists, "snapshot_at": snapAt(facultySnap), "reason": facultyReason},
			"university": gin.H{"status": uniStatus, "snapshot_exists": uniSnap.exists, "snapshot_at": snapAt(uniSnap), "reason": ""},
			"country":    gin.H{"status": countryStatus, "snapshot_exists": countrySnap.exists, "snapshot_at": snapAt(countrySnap), "reason": ""},
		}
	}

	normSubject := func(s string) string {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "" {
			return "COMP"
		}
		return s
	}
	subjectArea := normSubject(uni.SubjectArea)
	facultySubject := normSubject(faculty.SubjectArea)
	countrySubject := normSubject(country.SubjectArea)
	// All three scopes that feed the comparison must be the same subject and it must
	// be COMP; otherwise the report must withhold cross-scope conclusions (§4, R2-3).
	scopeConsistent := subjectArea == "COMP" && facultySubject == "COMP" && countrySubject == "COMP"

	// available_years lists EVERY year each scope has a snapshot for (existence, not
	// readiness — a real zero or a blocked-faculty year still counts), across all
	// stored years rather than just the requested window, so the report can discover
	// and load older years (handoff §4/§9 A, R7).
	allSnapshotYears := func(scopeID uint64) []int {
		var years []int
		config.DB.Raw(`
			SELECT DISTINCT pub_year FROM scopus_benchmark_count_snapshots
			WHERE scope_id = ? AND pub_year IS NOT NULL
			ORDER BY pub_year DESC`, scopeID).Scan(&years)
		if years == nil {
			years = []int{}
		}
		return years
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"years":            rows,
			"faculty_metric":   facultyCoverage,
			"faculty_scope":    faculty,
			"university_scope": uni,
			"country_scope":    country,
			"year_meta":        yearMeta,
			"available_years": gin.H{
				"faculty":    allSnapshotYears(faculty.ID),
				"university": allSnapshotYears(uni.ID),
				"country":    allSnapshotYears(country.ID),
			},
			"report_scope": gin.H{
				"subject_area":            subjectArea,
				"faculty_subject_area":    facultySubject,
				"university_subject_area": subjectArea,
				"country_subject_area":    countrySubject,
				"consistent":              scopeConsistent,
				"faculty_scope_id":        faculty.ID,
				"university_scope_id":     uni.ID,
				"country_scope_id":        country.ID,
			},
		},
	})
}

// GET /api/v1/admin/scopus/benchmark/insights?year=2026
func AdminGetBenchmarkInsights(c *gin.Context) {
	year, err := strconv.Atoi(strings.TrimSpace(c.Query("year")))
	if err != nil || year < 1900 || year > time.Now().Year()+1 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid year"})
		return
	}

	data, err := services.NewScopusBenchmarkService(nil, nil).BenchmarkInsightsForYear(c.Request.Context(), year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// GET /api/v1/admin/scopus/benchmark/top-journals?limit=8
func AdminGetBenchmarkTopJournals(c *gin.Context) {
	limit := 8
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "limit must be between 1 and 50"})
			return
		}
		limit = parsed
	}

	data, err := services.NewScopusBenchmarkService(nil, nil).BenchmarkTopJournals(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
