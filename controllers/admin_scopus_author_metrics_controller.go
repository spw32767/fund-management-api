package controllers

import (
	"context"
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
	"github.com/xuri/excelize/v2"
)

const authorMetricsRunTimeout = 1 * time.Hour

// POST /api/v1/admin/scopus/author-metrics/refresh?user_ids=1,2&limit=10
// ดึง h-index ของอาจารย์ทุกคน (อิง users.scopus_id) แล้วเก็บ snapshot รายวัน แบบ async
func AdminRefreshAuthorMetrics(c *gin.Context) {
	var userIDs []uint
	if csv := strings.TrimSpace(c.Query("user_ids")); csv != "" {
		for _, p := range strings.Split(csv, ",") {
			if id64, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64); err == nil && id64 > 0 {
				userIDs = append(userIDs, uint(id64))
			}
		}
	}

	limit := 0
	if limStr := strings.TrimSpace(c.Query("limit")); limStr != "" {
		if lim, err := strconv.Atoi(limStr); err == nil && lim > 0 {
			limit = lim
		}
	}

	svc := services.NewAuthorMetricsService(nil, nil)
	activeRun, err := svc.GetActiveRun(c.Request.Context())
	if err != nil {
		InternalError(c, "scopus", err)
		return
	}
	if activeRun != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "scopus author metrics job already running",
			"data":    activeRun,
		})
		return
	}

	input := &services.ScopusAuthorMetricsAllInput{UserIDs: userIDs, Limit: limit, TriggerSource: "admin_ui"}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), authorMetricsRunTimeout)
		defer cancel()

		if _, err := svc.RunForAll(ctx, input); err != nil {
			if errors.Is(err, services.ErrAuthorMetricsAlreadyRunning) {
				log.Printf("scopus author metrics skipped: job already running")
				return
			}
			log.Printf("scopus author metrics job failed: %v", err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"summary": gin.H{"status": "running", "message": "author metrics refresh started"},
	})
}

// GET /api/v1/admin/scopus/author-metrics/runs?page=1&per_page=20
func AdminListAuthorMetricsRuns(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	query := config.DB.Model(&models.ScopusAuthorMetricsRun{})

	var total int64
	if err := query.Count(&total).Error; err != nil {
		InternalError(c, "scopus", err)
		return
	}

	var runs []models.ScopusAuthorMetricsRun
	offset := (page - 1) * perPage
	if err := query.Order("started_at DESC").Offset(offset).Limit(perPage).Find(&runs).Error; err != nil {
		InternalError(c, "scopus", err)
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

// GET /api/v1/admin/scopus/author-metrics/faculty-hgraph?year_from=2011&year_to=2026
// สร้าง Hirsch h-graph ระดับคณะ (นับเฉพาะผลงานที่สังกัด KKU, dedupe ต่อ document) จาก scopus_documents
func AdminGetScopusFacultyHIndexGraph(c *gin.Context) {
	var yearFrom, yearTo *int
	if v := strings.TrimSpace(c.Query("year_from")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			yearFrom = &y
		}
	}
	if v := strings.TrimSpace(c.Query("year_to")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			yearTo = &y
		}
	}

	svc := services.NewAuthorHGraphService(nil)
	graph, err := svc.GetFacultyGraph(c.Request.Context(), yearFrom, yearTo)
	if err != nil {
		InternalError(c, "scopus", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": graph})
}

// GET /api/v1/admin/scopus/author-metrics/faculty-export?year_from=2011&year_to=2026
// ส่งออกไฟล์ Excel (.xlsx) ของผลงานระดับคณะ (เฉพาะ KKU, dedupe) — ชีต Data (รายบทความ) + ชีต Summary
func AdminExportScopusFacultyHIndex(c *gin.Context) {
	var yearFrom, yearTo *int
	if v := strings.TrimSpace(c.Query("year_from")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			yearFrom = &y
		}
	}
	if v := strings.TrimSpace(c.Query("year_to")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			yearTo = &y
		}
	}

	svc := services.NewAuthorHGraphService(nil)
	rows, summary, err := svc.GetFacultyExport(c.Request.Context(), yearFrom, yearTo)
	if err != nil {
		InternalError(c, "scopus", err)
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	// ---- Data sheet: one row per document, sorted by citations desc (Order4Index order) ----
	const dataSheet = "Data"
	const dataLastCol = "S" // 19 columns A..S
	f.SetSheetName("Sheet1", dataSheet)
	// citedby_count ถูกย้ายมาไว้ท้ายสุดก่อน Order4Index ตามที่ผู้ใช้ขอ
	headers := []interface{}{
		"scopus_id", "title", "authors", "abstract", "aggregation_type",
		"authkeywords", "fund_sponsor", "cite_score_status", "cite_score_rank",
		"cite_score_percentile", "journal_tier_bucket", "cite_score_quartile",
		"publication_year", "publication_year_be", "eid", "scopus_url", "doi_url",
		"citedby_count", "Order4Index",
	}
	_ = f.SetSheetRow(dataSheet, "A1", &headers)

	intOrBlank := func(p *int) interface{} {
		if p == nil {
			return ""
		}
		return *p
	}
	floatOrBlank := func(p *float64) interface{} {
		if p == nil {
			return ""
		}
		return *p
	}
	for i, r := range rows {
		record := []interface{}{
			r.ScopusID, r.Title, r.Authors, r.Abstract, r.AggregationType,
			r.AuthKeywords, r.FundSponsor, r.CiteScoreStatus, intOrBlank(r.CiteScoreRank),
			floatOrBlank(r.CiteScorePercentile), r.JournalTierBucket, r.CiteScoreQuartile,
			intOrBlank(r.PublicationYearCE), intOrBlank(r.PublicationYearBE), r.EID, r.ScopusURL, r.DOIURL,
			r.CitedByCount, r.Order4Index,
		}
		if err := f.SetSheetRow(dataSheet, fmt.Sprintf("A%d", i+2), &record); err != nil {
			InternalError(c, "scopus", err)
			return
		}
	}

	// Bold header + AutoFilter so ผู้ใช้เรียง/กรองใน Excel เองได้ (ค่าเริ่มต้นเรียงตาม citedby มาก→น้อยอยู่แล้ว)
	if len(rows) > 0 {
		if hStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"F1F5F9"}, Pattern: 1}}); err == nil {
			_ = f.SetCellStyle(dataSheet, "A1", dataLastCol+"1", hStyle)
		}
		_ = f.AutoFilter(dataSheet, fmt.Sprintf("A1:%s%d", dataLastCol, len(rows)+1), []excelize.AutoFilterOptions{})
	}
	// Highlight the h-core rows (Order4Index ≤ h-index) — บทความที่ประกอบกันเป็น h-index
	if summary.HIndex > 0 {
		if hlStyle, err := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"FEF9C3"}, Pattern: 1}}); err == nil {
			last := summary.HIndex + 1
			if last > len(rows)+1 {
				last = len(rows) + 1
			}
			_ = f.SetCellStyle(dataSheet, "A2", fmt.Sprintf("%s%d", dataLastCol, last), hlStyle)
		}
	}

	// ---- Summary sheet: key metrics + tier/type/year breakdowns (with totals) ----
	const sumSheet = "Summary"
	if _, err := f.NewSheet(sumSheet); err != nil {
		InternalError(c, "scopus", err)
		return
	}
	yearRangeCE, yearRangeBE := "-", "-"
	if summary.YearMinCE != nil && summary.YearMaxCE != nil {
		yearRangeCE = fmt.Sprintf("%d–%d", *summary.YearMinCE, *summary.YearMaxCE)
		yearRangeBE = fmt.Sprintf("%d–%d", *summary.YearMinCE+543, *summary.YearMaxCE+543)
	}
	filterLabel := "ทั้งหมด"
	if yearFrom != nil || yearTo != nil {
		fromCE, toCE := "ต้น", "ล่าสุด"
		fromBE, toBE := "ต้น", "ล่าสุด"
		if yearFrom != nil {
			fromCE = strconv.Itoa(*yearFrom)
			fromBE = strconv.Itoa(*yearFrom + 543)
		}
		if yearTo != nil {
			toCE = strconv.Itoa(*yearTo)
			toBE = strconv.Itoa(*yearTo + 543)
		}
		filterLabel = fmt.Sprintf("%s–%s (ค.ศ.) / %s–%s (พ.ศ.)", fromCE, toCE, fromBE, toBE)
	}

	tierTotal := summary.T1 + summary.Q1 + summary.Q2 + summary.Q3 + summary.Q4 + summary.QuartileNA
	otherType := summary.DocumentCount - summary.JournalCount - summary.ConferenceCount - summary.BookCount
	if otherType < 0 {
		otherType = 0
	}

	sumRows := [][]interface{}{
		{"สรุป h-index ระดับคณะ (Scopus)"},
		{"หมายเหตุ", "นับเฉพาะผลงานที่สังกัด KKU และนับบทความที่อาจารย์ร่วมมือกันหลายคนเพียงครั้งเดียว (dedupe)"},
		{"ช่วงปีที่กรอง", filterLabel},
		{"ออกรายงานเมื่อ", time.Now().Format("2006-01-02 15:04")},
		{},
		{"ตัวชี้วัด", "ค่า"},
		{"h-index", summary.HIndex},
		{"จำนวนเอกสาร (ไม่ซ้ำ)", summary.DocumentCount},
		{"การอ้างอิงรวม", summary.CitationTotal},
		{"การอ้างอิงเฉลี่ย/เอกสาร", summary.AvgCitation},
		{"ช่วงปีที่มีผลงาน (ค.ศ.)", yearRangeCE},
		{"ช่วงปีที่มีผลงาน (พ.ศ.)", yearRangeBE},
		{},
		{"คุณภาพวารสาร (tier)", "จำนวน"},
		{"T1", summary.T1},
		{"Q1", summary.Q1},
		{"Q2", summary.Q2},
		{"Q3", summary.Q3},
		{"Q4", summary.Q4},
		{"N/A", summary.QuartileNA},
		{"รวม", tierTotal},
		{},
		{"ประเภทผลงาน", "จำนวน"},
		{"Journal", summary.JournalCount},
		{"Conference", summary.ConferenceCount},
		{"Book/Book Series", summary.BookCount},
		{"อื่น ๆ", otherType},
		{"รวม", summary.DocumentCount},
		{},
		{"ผลงานรายปี (พ.ศ.)", "จำนวนเอกสาร", "การอ้างอิงรวม"},
	}
	yearCountTotal, yearCitesTotal := 0, 0
	for _, yc := range summary.ByYear {
		sumRows = append(sumRows, []interface{}{yc.YearCE + 543, yc.Count, yc.Citations})
		yearCountTotal += yc.Count
		yearCitesTotal += yc.Citations
	}
	sumRows = append(sumRows, []interface{}{"รวม", yearCountTotal, yearCitesTotal})

	for i, r := range sumRows {
		rr := r
		if err := f.SetSheetRow(sumSheet, fmt.Sprintf("A%d", i+1), &rr); err != nil {
			InternalError(c, "scopus", err)
			return
		}
	}

	// Bold the title, section headers, and total rows.
	boldLabels := map[string]bool{
		"สรุป h-index ระดับคณะ (Scopus)": true, "ตัวชี้วัด": true,
		"คุณภาพวารสาร (tier)": true, "ประเภทผลงาน": true, "ผลงานรายปี (พ.ศ.)": true, "รวม": true,
	}
	if bStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err == nil {
		for i, r := range sumRows {
			if len(r) > 0 {
				if label, ok := r[0].(string); ok && boldLabels[label] {
					_ = f.SetCellStyle(sumSheet, fmt.Sprintf("A%d", i+1), fmt.Sprintf("C%d", i+1), bStyle)
				}
			}
		}
	}

	if idx, err := f.GetSheetIndex(dataSheet); err == nil {
		f.SetActiveSheet(idx)
	}

	filename := fmt.Sprintf("scopus-hindex-faculty-%s.xlsx", time.Now().Format("2006-01-02"))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := f.Write(c.Writer); err != nil {
		log.Printf("faculty h-index export write failed: %v", err)
	}
}
