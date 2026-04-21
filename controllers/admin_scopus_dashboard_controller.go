package controllers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	scopusDashboardOptionsCacheTTL = 6 * time.Hour
	scopusDashboardSummaryCacheTTL = 10 * time.Minute
	scopusAffiliationNameKKU       = "khon kaen university"
	scopusAffiliationNameSciKKU    = "faculty of science, khon kaen university"
)

type scopusDashboardCacheItem struct {
	ExpiresAt time.Time
	Payload   interface{}
}

var scopusDashboardCache = struct {
	mu    sync.RWMutex
	items map[string]scopusDashboardCacheItem
}{
	items: map[string]scopusDashboardCacheItem{},
}

type scopusDashboardFilters struct {
	Scope            string
	YearStartCE      *int
	YearEndCE        *int
	AggregationTypes []string
	QualityBuckets   []string
	OpenAccessMode   string
	CitationMin      *int
	CitationMax      *int
	Title            string
	DOI              string
	EID              string
	ScopusID         string
	Journal          string
	Author           string
	Affiliation      string
	Keyword          string
}

type scopusSummaryRow struct {
	ID                  uint
	CitedByCount        int      `gorm:"column:cited_by_count"`
	OpenAccessFlag      int      `gorm:"column:openaccess_flag"`
	OpenAccess          int      `gorm:"column:openaccess"`
	Quartile            string   `gorm:"column:quartile"`
	CiteScorePercentile *float64 `gorm:"column:cite_score_percentile"`
	PublicationYearCE   *int     `gorm:"column:publication_year_ce"`
	PublicationMonthCE  *int     `gorm:"column:publication_month_ce"`
	AggregationType     string   `gorm:"column:aggregation_type"`
	PublicationName     string   `gorm:"column:publication_name"`
	FundSponsor         string   `gorm:"column:fund_sponsor"`
}

type scopusPersonSummaryDocRow struct {
	UserID              int      `gorm:"column:user_id"`
	UserName            string   `gorm:"column:user_name"`
	UserEmail           string   `gorm:"column:user_email"`
	UserScopusID        string   `gorm:"column:user_scopus_id"`
	DocumentID          uint     `gorm:"column:document_id"`
	PublicationYearCE   *int     `gorm:"column:publication_year_ce"`
	CitedByCount        int      `gorm:"column:cited_by_count"`
	Quartile            string   `gorm:"column:quartile"`
	CiteScorePercentile *float64 `gorm:"column:cite_score_percentile"`
	AggregationType     string   `gorm:"column:aggregation_type"`
}

type scopusInternalCollaborationPairRow struct {
	UserAID         int    `gorm:"column:user_a_id"`
	UserA           string `gorm:"column:user_a"`
	UserBID         int    `gorm:"column:user_b_id"`
	UserB           string `gorm:"column:user_b"`
	SharedDocuments int    `gorm:"column:shared_documents"`
}

type scopusAggregationTypeRow struct {
	Label string `gorm:"column:label"`
	Total int64  `gorm:"column:total"`
}

type scopusYearOptionRow struct {
	YearCE int   `gorm:"column:year_ce"`
	Total  int64 `gorm:"column:total"`
}

func readScopusDashboardCache(key string) (interface{}, bool) {
	now := time.Now()

	scopusDashboardCache.mu.RLock()
	item, ok := scopusDashboardCache.items[key]
	scopusDashboardCache.mu.RUnlock()
	if !ok || now.After(item.ExpiresAt) {
		if ok {
			scopusDashboardCache.mu.Lock()
			delete(scopusDashboardCache.items, key)
			scopusDashboardCache.mu.Unlock()
		}
		return nil, false
	}

	return item.Payload, true
}

func writeScopusDashboardCache(key string, payload interface{}, ttl time.Duration) {
	scopusDashboardCache.mu.Lock()
	scopusDashboardCache.items[key] = scopusDashboardCacheItem{
		ExpiresAt: time.Now().Add(ttl),
		Payload:   payload,
	}
	scopusDashboardCache.mu.Unlock()
}

func normalizeCommaValues(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	seen := map[string]struct{}{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func parseYearBEToCE(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return nil
	}
	if n >= 2400 {
		n -= 543
	}
	if n <= 0 {
		return nil
	}
	return &n
}

func parseOptionalNonNegativeInt(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return nil
	}
	return &n
}

func parseScopusDashboardFilters(c *gin.Context) scopusDashboardFilters {
	scope := strings.ToLower(strings.TrimSpace(c.DefaultQuery("scope", "faculty")))
	if scope != "individual" {
		scope = "faculty"
	}

	qualityRaw := normalizeCommaValues(c.Query("quality_buckets"))
	qualityAllowed := map[string]struct{}{
		"Q1": {}, "Q2": {}, "Q3": {}, "Q4": {}, "N/A": {}, "T1": {},
	}
	quality := make([]string, 0, len(qualityRaw))
	for _, value := range qualityRaw {
		normalized := strings.ToUpper(strings.TrimSpace(value))
		if _, ok := qualityAllowed[normalized]; ok {
			quality = append(quality, normalized)
		}
	}

	openAccess := strings.ToLower(strings.TrimSpace(c.DefaultQuery("open_access_mode", "all")))
	if openAccess != "oa" && openAccess != "non_oa" {
		openAccess = "all"
	}

	filters := scopusDashboardFilters{
		Scope:            scope,
		YearStartCE:      parseYearBEToCE(c.Query("year_start_be")),
		YearEndCE:        parseYearBEToCE(c.Query("year_end_be")),
		AggregationTypes: normalizeCommaValues(c.Query("aggregation_types")),
		QualityBuckets:   quality,
		OpenAccessMode:   openAccess,
		CitationMin:      parseOptionalNonNegativeInt(c.Query("citation_min")),
		CitationMax:      parseOptionalNonNegativeInt(c.Query("citation_max")),
		Title:            strings.TrimSpace(c.Query("search_title")),
		DOI:              strings.TrimSpace(c.Query("search_doi")),
		EID:              strings.TrimSpace(c.Query("search_eid")),
		ScopusID:         strings.TrimSpace(c.Query("search_scopus_id")),
		Journal:          strings.TrimSpace(c.Query("search_journal")),
		Author:           strings.TrimSpace(c.Query("search_author")),
		Affiliation:      strings.TrimSpace(c.Query("search_affiliation")),
		Keyword:          strings.TrimSpace(c.Query("search_keyword")),
	}

	if filters.YearStartCE != nil && filters.YearEndCE != nil && *filters.YearStartCE > *filters.YearEndCE {
		filters.YearStartCE, filters.YearEndCE = filters.YearEndCE, filters.YearStartCE
	}

	return filters
}

func (f scopusDashboardFilters) cacheKey() string {
	aggregationTypes := append([]string(nil), f.AggregationTypes...)
	qualityBuckets := append([]string(nil), f.QualityBuckets...)
	sort.Strings(aggregationTypes)
	sort.Strings(qualityBuckets)

	chunks := []string{
		"scope=" + f.Scope,
		"open_access_mode=" + f.OpenAccessMode,
		"year_start=" + intPtrToString(f.YearStartCE),
		"year_end=" + intPtrToString(f.YearEndCE),
		"citation_min=" + intPtrToString(f.CitationMin),
		"citation_max=" + intPtrToString(f.CitationMax),
		"aggregation_types=" + strings.Join(aggregationTypes, "|"),
		"quality=" + strings.Join(qualityBuckets, "|"),
		"title=" + strings.ToLower(f.Title),
		"doi=" + strings.ToLower(f.DOI),
		"eid=" + strings.ToLower(f.EID),
		"scopus_id=" + strings.ToLower(f.ScopusID),
		"journal=" + strings.ToLower(f.Journal),
		"author=" + strings.ToLower(f.Author),
		"affiliation=" + strings.ToLower(f.Affiliation),
		"keyword=" + strings.ToLower(f.Keyword),
	}
	return strings.Join(chunks, "&")
}

func intPtrToString(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func scopusPublicationYearExpr() string {
	return "COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED))"
}

func scopusMetricYearExpr() string {
	publicationYearExpr := scopusPublicationYearExpr()
	sameYearCompleteExpr := fmt.Sprintf(
		"(SELECT ssm_complete.metric_year FROM scopus_source_metrics AS ssm_complete WHERE ssm_complete.source_id = sd.source_id AND ssm_complete.doc_type = 'all' AND ssm_complete.metric_year = %s AND LOWER(ssm_complete.cite_score_status) = 'complete' LIMIT 1)",
		publicationYearExpr,
	)
	previousCompleteExpr := fmt.Sprintf(
		"(SELECT MAX(ssm_prev.metric_year) FROM scopus_source_metrics AS ssm_prev WHERE ssm_prev.source_id = sd.source_id AND ssm_prev.doc_type = 'all' AND ssm_prev.metric_year < %s AND LOWER(ssm_prev.cite_score_status) = 'complete')",
		publicationYearExpr,
	)

	return fmt.Sprintf("COALESCE(%s, %s)", sameYearCompleteExpr, previousCompleteExpr)
}

func applyScopusKKUAffiliationConstraint(query *gorm.DB) *gorm.DB {
	return query.Where(`
		EXISTS (
			SELECT 1
			FROM scopus_document_authors sda
			JOIN scopus_authors sa ON sa.id = sda.author_id
			JOIN users u ON TRIM(u.scopus_id) = sa.scopus_author_id
			JOIN scopus_affiliations aff ON aff.id = sda.affiliation_id
			WHERE sda.document_id = sd.id
			  AND u.delete_at IS NULL
			  AND u.scopus_id IS NOT NULL
			  AND TRIM(u.scopus_id) <> ''
			  AND LOWER(TRIM(COALESCE(aff.name, ''))) IN (?, ?)
		)
	`, scopusAffiliationNameKKU, scopusAffiliationNameSciKKU)
}

func applyScopusDashboardFilters(query *gorm.DB, filters scopusDashboardFilters, includeQuality bool) *gorm.DB {
	q := applyScopusKKUAffiliationConstraint(query)
	pubYearExpr := scopusPublicationYearExpr()

	if filters.YearStartCE != nil {
		q = q.Where(pubYearExpr+" >= ?", *filters.YearStartCE)
	}
	if filters.YearEndCE != nil {
		q = q.Where(pubYearExpr+" <= ?", *filters.YearEndCE)
	}

	if len(filters.AggregationTypes) > 0 {
		q = q.Where("sd.aggregation_type IN ?", filters.AggregationTypes)
	}

	switch filters.OpenAccessMode {
	case "oa":
		q = q.Where("(COALESCE(sd.openaccess_flag, 0) = 1 OR COALESCE(sd.openaccess, 0) = 1)")
	case "non_oa":
		q = q.Where("(COALESCE(sd.openaccess_flag, 0) = 0 AND COALESCE(sd.openaccess, 0) = 0)")
	}

	if filters.CitationMin != nil {
		q = q.Where("COALESCE(sd.citedby_count, 0) >= ?", *filters.CitationMin)
	}
	if filters.CitationMax != nil {
		q = q.Where("COALESCE(sd.citedby_count, 0) <= ?", *filters.CitationMax)
	}

	if filters.Title != "" {
		q = q.Where("sd.title LIKE ?", "%"+filters.Title+"%")
	}
	if filters.DOI != "" {
		q = q.Where("sd.doi = ?", filters.DOI)
	}
	if filters.EID != "" {
		q = q.Where("sd.eid = ?", filters.EID)
	}
	if filters.ScopusID != "" {
		q = q.Where("sd.scopus_id = ?", filters.ScopusID)
	}
	if filters.Journal != "" {
		q = q.Where("sd.publication_name LIKE ?", "%"+filters.Journal+"%")
	}
	if filters.Keyword != "" {
		q = q.Where("sd.authkeywords LIKE ?", "%"+filters.Keyword+"%")
	}
	if filters.Author != "" {
		q = q.Where(`
			EXISTS (
				SELECT 1
				FROM scopus_document_authors sda
				JOIN scopus_authors sa ON sa.id = sda.author_id
				WHERE sda.document_id = sd.id
				  AND (
					sa.full_name LIKE ?
					OR sa.given_name LIKE ?
					OR sa.surname LIKE ?
				  )
			)
		`, "%"+filters.Author+"%", "%"+filters.Author+"%", "%"+filters.Author+"%")
	}
	if filters.Affiliation != "" {
		q = q.Where(`
			EXISTS (
				SELECT 1
				FROM scopus_document_authors sda
				JOIN scopus_affiliations aff ON aff.id = sda.affiliation_id
				WHERE sda.document_id = sd.id
				  AND (
					aff.name LIKE ?
					OR aff.city LIKE ?
					OR aff.country LIKE ?
					OR aff.afid LIKE ?
				  )
			)
		`, "%"+filters.Affiliation+"%", "%"+filters.Affiliation+"%", "%"+filters.Affiliation+"%", "%"+filters.Affiliation+"%")
	}

	if includeQuality && len(filters.QualityBuckets) > 0 {
		includeT1 := false
		quartiles := make([]string, 0, len(filters.QualityBuckets))
		for _, bucket := range filters.QualityBuckets {
			if bucket == "T1" {
				includeT1 = true
				continue
			}
			quartiles = append(quartiles, bucket)
		}

		if includeT1 && len(quartiles) > 0 {
			q = q.Where("((metrics.cite_score_percentile BETWEEN 90 AND 100) OR COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)),''), 'N/A') IN ?)", quartiles)
		} else if includeT1 {
			q = q.Where("metrics.cite_score_percentile BETWEEN 90 AND 100")
		} else {
			q = q.Where("COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)),''), 'N/A') IN ?", quartiles)
		}
	}

	return q
}

// GET /api/v1/admin/scopus/dashboard/filter-options
func AdminGetScopusDashboardFilterOptions(c *gin.Context) {
	cacheKey := "scopus_dashboard_filter_options_v3"
	forceRefresh := strings.TrimSpace(c.Query("refresh")) == "1"

	if !forceRefresh {
		if cached, ok := readScopusDashboardCache(cacheKey); ok {
			if payload, valid := cached.(map[string]interface{}); valid {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data":    payload,
					"meta": gin.H{
						"cached":      true,
						"ttl_seconds": int(scopusDashboardOptionsCacheTTL / time.Second),
					},
				})
				return
			}
		}
	}

	pubYearExpr := scopusPublicationYearExpr()
	metricYearExpr := scopusMetricYearExpr()

	var years []scopusYearOptionRow
	if err := config.DB.Table("scopus_documents AS sd").
		Select(pubYearExpr + " AS year_ce, COUNT(DISTINCT sd.id) AS total").
		Where(pubYearExpr + " IS NOT NULL AND " + pubYearExpr + " > 0").
		Scopes(applyScopusKKUAffiliationConstraint).
		Group("year_ce").
		Order("year_ce DESC").
		Scan(&years).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var aggregationTypes []scopusAggregationTypeRow
	if err := config.DB.Table("scopus_documents AS sd").
		Select("TRIM(sd.aggregation_type) AS label, COUNT(DISTINCT sd.id) AS total").
		Where("sd.aggregation_type IS NOT NULL AND TRIM(sd.aggregation_type) <> ''").
		Scopes(applyScopusKKUAffiliationConstraint).
		Group("TRIM(aggregation_type)").
		Order("total DESC, label ASC").
		Scan(&aggregationTypes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	qualityCounts := map[string]int64{
		"Q1":  0,
		"Q2":  0,
		"Q3":  0,
		"Q4":  0,
		"N/A": 0,
		"T1":  0,
	}

	type qualityCountRow struct {
		Quartile string `gorm:"column:quartile"`
		Total    int64  `gorm:"column:total"`
	}
	var qualityRows []qualityCountRow
	if err := config.DB.Table("scopus_documents AS sd").
		Select("COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') AS quartile, COUNT(DISTINCT sd.id) AS total").
		Joins("LEFT JOIN scopus_source_metrics AS metrics ON metrics.source_id = sd.source_id AND metrics.doc_type = 'all' AND metrics.metric_year = " + metricYearExpr).
		Scopes(applyScopusKKUAffiliationConstraint).
		Group("quartile").
		Scan(&qualityRows).Error; err == nil {
		for _, row := range qualityRows {
			qualityCounts[row.Quartile] = row.Total
		}
	}

	var t1Count int64
	if err := config.DB.Table("scopus_documents AS sd").
		Joins("LEFT JOIN scopus_source_metrics AS metrics ON metrics.source_id = sd.source_id AND metrics.doc_type = 'all' AND metrics.metric_year = " + metricYearExpr).
		Scopes(applyScopusKKUAffiliationConstraint).
		Where("metrics.cite_score_percentile BETWEEN 90 AND 100").
		Distinct("sd.id").
		Count(&t1Count).Error; err == nil {
		qualityCounts["T1"] = t1Count
	}

	yearOptions := make([]map[string]interface{}, 0, len(years))
	minBE := 0
	maxBE := 0
	for index, year := range years {
		yearBE := year.YearCE + 543
		if index == 0 {
			maxBE = yearBE
		}
		minBE = yearBE
		yearOptions = append(yearOptions, map[string]interface{}{
			"value": yearBE,
			"label": strconv.Itoa(yearBE),
			"total": year.Total,
		})
	}

	aggregationOptions := make([]map[string]interface{}, 0, len(aggregationTypes))
	for _, item := range aggregationTypes {
		aggregationOptions = append(aggregationOptions, map[string]interface{}{
			"value": item.Label,
			"label": item.Label,
			"total": item.Total,
		})
	}

	qualityOrder := []string{"T1", "Q1", "Q2", "Q3", "Q4", "N/A"}
	qualityOptions := make([]map[string]interface{}, 0, len(qualityOrder))
	for _, key := range qualityOrder {
		label := key
		if key == "T1" {
			label = "T1"
		}
		qualityOptions = append(qualityOptions, map[string]interface{}{
			"value": key,
			"label": label,
			"total": qualityCounts[key],
		})
	}

	payload := map[string]interface{}{
		"scopes": []map[string]interface{}{
			{"value": "faculty", "label": "ระดับคณะ"},
			{"value": "individual", "label": "รายบุคคล"},
		},
		"year_options": yearOptions,
		"year_range": map[string]interface{}{
			"min_be": minBE,
			"max_be": maxBE,
		},
		"aggregation_types": aggregationOptions,
		"quality_buckets":   qualityOptions,
		"open_access_modes": []map[string]interface{}{
			{"value": "all", "label": "ทั้งหมด"},
			{"value": "oa", "label": "Open Access"},
			{"value": "non_oa", "label": "Non Open Access"},
		},
	}

	writeScopusDashboardCache(cacheKey, payload, scopusDashboardOptionsCacheTTL)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    payload,
		"meta": gin.H{
			"cached":      false,
			"ttl_seconds": int(scopusDashboardOptionsCacheTTL / time.Second),
		},
	})
}

// GET /api/v1/admin/scopus/dashboard/summary
func AdminGetScopusDashboardSummary(c *gin.Context) {
	filters := parseScopusDashboardFilters(c)
	forceRefresh := strings.TrimSpace(c.Query("refresh")) == "1"
	cacheKey := "scopus_dashboard_summary_v8:" + filters.cacheKey()

	if !forceRefresh {
		if cached, ok := readScopusDashboardCache(cacheKey); ok {
			if payload, valid := cached.(map[string]interface{}); valid {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data":    payload,
					"meta": gin.H{
						"cached":      true,
						"ttl_seconds": int(scopusDashboardSummaryCacheTTL / time.Second),
					},
				})
				return
			}
		}
	}

	metricYearExpr := scopusMetricYearExpr()

	rows := make([]scopusSummaryRow, 0)
	query := config.DB.Table("scopus_documents AS sd").
		Select(`
			sd.id,
			COALESCE(sd.citedby_count, 0) AS cited_by_count,
			COALESCE(sd.openaccess_flag, 0) AS openaccess_flag,
			COALESCE(sd.openaccess, 0) AS openaccess,
			COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') AS quartile,
			metrics.cite_score_percentile,
			` + scopusPublicationYearExpr() + ` AS publication_year_ce,
			MONTH(sd.cover_date) AS publication_month_ce,
			COALESCE(NULLIF(TRIM(sd.aggregation_type), ''), 'N/A') AS aggregation_type,
			COALESCE(NULLIF(TRIM(sd.publication_name), ''), 'N/A') AS publication_name,
			COALESCE(NULLIF(TRIM(sd.fund_sponsor), ''), 'N/A') AS fund_sponsor
		`).
		Joins("LEFT JOIN scopus_source_metrics AS metrics ON metrics.source_id = sd.source_id AND metrics.doc_type = 'all' AND metrics.metric_year = " + metricYearExpr)

	query = applyScopusDashboardFilters(query, filters, true)

	if err := query.Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	quartileCounts := map[string]int{
		"Q1":  0,
		"Q2":  0,
		"Q3":  0,
		"Q4":  0,
		"N/A": 0,
	}
	aggregationCounts := map[string]int{}
	type publicationSourceKey struct {
		AggregationType string
		PublicationName string
	}
	publicationSourceCounts := map[publicationSourceKey]int{}
	fundingSponsorCounts := map[string]int{}
	yearCounts := map[string]int{}
	type historyBucket struct {
		PublicationYearBE int
		UniqueDocuments   int
		T1                int
		Q1                int
		Q2                int
		Q3                int
		Q4                int
		TCI               int
		NA                int
		Journal           int
		Conference        int
		CitedByTotal      int
	}
	historyByYear := map[int]*historyBucket{}
	historyByFiscalYear := map[int]*historyBucket{}
	seenDocumentIDs := map[uint]struct{}{}
	totalDocuments := 0
	totalCitations := 0
	openAccessDocuments := 0
	t1Documents := 0

	for _, row := range rows {
		if _, exists := seenDocumentIDs[row.ID]; exists {
			continue
		}
		seenDocumentIDs[row.ID] = struct{}{}
		totalDocuments++

		totalCitations += row.CitedByCount

		if row.OpenAccessFlag == 1 || row.OpenAccess == 1 {
			openAccessDocuments++
		}

		quartile := strings.ToUpper(strings.TrimSpace(row.Quartile))
		if quartile == "" {
			quartile = "N/A"
		}
		if _, exists := quartileCounts[quartile]; !exists {
			quartile = "N/A"
		}
		quartileCounts[quartile]++

		aggLabel := strings.TrimSpace(row.AggregationType)
		if aggLabel == "" {
			aggLabel = "N/A"
		}
		aggregationCounts[aggLabel]++

		sourceLabel := strings.TrimSpace(row.PublicationName)
		if sourceLabel == "" {
			sourceLabel = "N/A"
		}
		publicationSourceCounts[publicationSourceKey{AggregationType: aggLabel, PublicationName: sourceLabel}]++

		sponsorLabel := strings.TrimSpace(row.FundSponsor)
		if sponsorLabel == "" {
			sponsorLabel = "N/A"
		}
		fundingSponsorCounts[sponsorLabel]++

		if row.PublicationYearCE != nil && *row.PublicationYearCE > 0 {
			yearKey := strconv.Itoa(*row.PublicationYearCE + 543)
			yearCounts[yearKey]++

			yearBE := *row.PublicationYearCE + 543
			fiscalYearBE := yearBE
			if row.PublicationMonthCE != nil && *row.PublicationMonthCE >= 10 {
				fiscalYearBE = yearBE + 1
			}

			bucket, ok := historyByYear[yearBE]
			if !ok {
				bucket = &historyBucket{PublicationYearBE: yearBE}
				historyByYear[yearBE] = bucket
			}

			fiscalBucket, okFiscal := historyByFiscalYear[fiscalYearBE]
			if !okFiscal {
				fiscalBucket = &historyBucket{PublicationYearBE: fiscalYearBE}
				historyByFiscalYear[fiscalYearBE] = fiscalBucket
			}

			bucket.UniqueDocuments++
			bucket.CitedByTotal += row.CitedByCount
			fiscalBucket.UniqueDocuments++
			fiscalBucket.CitedByTotal += row.CitedByCount

			if strings.EqualFold(strings.TrimSpace(row.AggregationType), "Journal") {
				bucket.Journal++
				fiscalBucket.Journal++
			}
			if strings.EqualFold(strings.TrimSpace(row.AggregationType), "Conference Proceeding") {
				bucket.Conference++
				fiscalBucket.Conference++
			}

			isT1 := row.CiteScorePercentile != nil && *row.CiteScorePercentile >= 90 && *row.CiteScorePercentile <= 100
			if isT1 {
				bucket.T1++
				fiscalBucket.T1++
			} else {
				switch quartile {
				case "Q1":
					bucket.Q1++
					fiscalBucket.Q1++
				case "Q2":
					bucket.Q2++
					fiscalBucket.Q2++
				case "Q3":
					bucket.Q3++
					fiscalBucket.Q3++
				case "Q4":
					bucket.Q4++
					fiscalBucket.Q4++
				default:
					bucket.NA++
					fiscalBucket.NA++
				}
			}
		}

		if row.CiteScorePercentile != nil && *row.CiteScorePercentile >= 90 && *row.CiteScorePercentile <= 100 {
			t1Documents++
		}
	}

	tciRows := make([]struct {
		SubmittedYearCE  int `gorm:"column:submitted_year_ce"`
		SubmittedMonthCE int `gorm:"column:submitted_month_ce"`
		Total            int `gorm:"column:total"`
	}, 0)
	tciQuery := config.DB.Table("publication_reward_details AS prd").
		Select("YEAR(s.submitted_at) AS submitted_year_ce, MONTH(s.submitted_at) AS submitted_month_ce, COUNT(DISTINCT s.submission_id) AS total").
		Joins("JOIN submissions AS s ON s.submission_id = prd.submission_id").
		Where("prd.delete_at IS NULL").
		Where("s.deleted_at IS NULL").
		Where("s.submission_type = ?", "publication_reward").
		Where("s.submitted_at IS NOT NULL").
		Where("s.status_id <> ?", 5).
		Where("UPPER(TRIM(prd.quartile)) = ?", "TCI")
	if filters.YearStartCE != nil {
		tciQuery = tciQuery.Where("YEAR(s.submitted_at) >= ?", *filters.YearStartCE)
	}
	if filters.YearEndCE != nil {
		tciQuery = tciQuery.Where("YEAR(s.submitted_at) <= ?", *filters.YearEndCE)
	}
	tciQuery = tciQuery.Group("YEAR(s.submitted_at), MONTH(s.submitted_at)")
	if err := tciQuery.Find(&tciRows).Error; err == nil {
		for _, row := range tciRows {
			if row.SubmittedYearCE <= 0 || row.Total <= 0 {
				continue
			}

			yearBE := row.SubmittedYearCE + 543
			fiscalYearBE := yearBE
			if row.SubmittedMonthCE >= 10 {
				fiscalYearBE = yearBE + 1
			}

			bucket, ok := historyByYear[yearBE]
			if !ok {
				bucket = &historyBucket{PublicationYearBE: yearBE}
				historyByYear[yearBE] = bucket
			}
			bucket.TCI += row.Total

			fiscalBucket, okFiscal := historyByFiscalYear[fiscalYearBE]
			if !okFiscal {
				fiscalBucket = &historyBucket{PublicationYearBE: fiscalYearBE}
				historyByFiscalYear[fiscalYearBE] = fiscalBucket
			}
			fiscalBucket.TCI += row.Total
		}
	}

	avgCitations := 0.0
	if totalDocuments > 0 {
		avgCitations = float64(totalCitations) / float64(totalDocuments)
	}

	var latestScopusPullAt *time.Time
	latestPullRow := struct {
		FinishedAt *time.Time `gorm:"column:finished_at"`
	}{}
	if err := config.DB.Table("scopus_batch_import_runs").
		Select("finished_at").
		Where("status = ? AND finished_at IS NOT NULL", "success").
		Order("finished_at DESC").
		Limit(1).
		Scan(&latestPullRow).Error; err == nil {
		latestScopusPullAt = latestPullRow.FinishedAt
	}

	var totalTeachersInFaculty int64
	if err := config.DB.Table("users").
		Where("delete_at IS NULL").
		Where("role_id IN ?", []int{1, 4, 5}).
		Count(&totalTeachersInFaculty).Error; err != nil {
		totalTeachersInFaculty = 0
	}

	aggRows := make([]map[string]interface{}, 0, len(aggregationCounts))
	for label, total := range aggregationCounts {
		aggRows = append(aggRows, map[string]interface{}{
			"label": label,
			"total": total,
		})
	}
	sort.Slice(aggRows, func(i, j int) bool {
		left := aggRows[i]["total"].(int)
		right := aggRows[j]["total"].(int)
		if left == right {
			return aggRows[i]["label"].(string) < aggRows[j]["label"].(string)
		}
		return left > right
	})

	sourceRows := make([]map[string]interface{}, 0, len(publicationSourceCounts))
	for key, total := range publicationSourceCounts {
		sourceRows = append(sourceRows, map[string]interface{}{
			"aggregation_type":   key.AggregationType,
			"publication_source": key.PublicationName,
			"label":              key.PublicationName,
			"total":              total,
		})
	}
	sort.Slice(sourceRows, func(i, j int) bool {
		leftTotal := sourceRows[i]["total"].(int)
		rightTotal := sourceRows[j]["total"].(int)
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		leftSource := sourceRows[i]["publication_source"].(string)
		rightSource := sourceRows[j]["publication_source"].(string)
		if leftSource != rightSource {
			return leftSource < rightSource
		}
		leftAgg := sourceRows[i]["aggregation_type"].(string)
		rightAgg := sourceRows[j]["aggregation_type"].(string)
		return leftAgg < rightAgg
	})

	sponsorRows := make([]map[string]interface{}, 0, len(fundingSponsorCounts))
	for label, total := range fundingSponsorCounts {
		sponsorRows = append(sponsorRows, map[string]interface{}{
			"label": label,
			"total": total,
		})
	}
	sort.Slice(sponsorRows, func(i, j int) bool {
		left := sponsorRows[i]["total"].(int)
		right := sponsorRows[j]["total"].(int)
		if left == right {
			return sponsorRows[i]["label"].(string) < sponsorRows[j]["label"].(string)
		}
		return left > right
	})
	if len(sponsorRows) > 10 {
		sponsorRows = sponsorRows[:10]
	}

	historyYears := make([]int, 0, len(historyByYear))
	for year := range historyByYear {
		historyYears = append(historyYears, year)
	}
	sort.Ints(historyYears)

	historyRows := make([]map[string]interface{}, 0, len(historyYears))
	for _, year := range historyYears {
		bucket := historyByYear[year]
		historyRows = append(historyRows, map[string]interface{}{
			"publication_year": year,
			"unique_documents": bucket.UniqueDocuments,
			"t1":               bucket.T1,
			"q1":               bucket.Q1,
			"q2":               bucket.Q2,
			"q3":               bucket.Q3,
			"q4":               bucket.Q4,
			"tci":              bucket.TCI,
			"na":               bucket.NA,
			"journal":          bucket.Journal,
			"conference":       bucket.Conference,
			"cited_by_total":   bucket.CitedByTotal,
		})
	}

	fiscalHistoryYears := make([]int, 0, len(historyByFiscalYear))
	for year := range historyByFiscalYear {
		fiscalHistoryYears = append(fiscalHistoryYears, year)
	}
	sort.Ints(fiscalHistoryYears)

	fiscalHistoryRows := make([]map[string]interface{}, 0, len(fiscalHistoryYears))
	for _, year := range fiscalHistoryYears {
		bucket := historyByFiscalYear[year]
		fiscalHistoryRows = append(fiscalHistoryRows, map[string]interface{}{
			"publication_year": year,
			"unique_documents": bucket.UniqueDocuments,
			"t1":               bucket.T1,
			"q1":               bucket.Q1,
			"q2":               bucket.Q2,
			"q3":               bucket.Q3,
			"q4":               bucket.Q4,
			"tci":              bucket.TCI,
			"na":               bucket.NA,
			"journal":          bucket.Journal,
			"conference":       bucket.Conference,
			"cited_by_total":   bucket.CitedByTotal,
		})
	}

	personSummaryRows := make([]map[string]interface{}, 0)
	personYearMatrix := map[string]interface{}{
		"year_start_be": 0,
		"year_end_be":   0,
		"years":         []int{},
		"rows":          []map[string]interface{}{},
	}
	internalCollaborationPairs := make([]map[string]interface{}, 0)
	if filters.Scope == "individual" {
		personDocRows := make([]scopusPersonSummaryDocRow, 0)
		personQuery := config.DB.Table("users AS u").
			Select(`
				u.user_id,
				TRIM(CONCAT(COALESCE(u.user_fname, ''), ' ', COALESCE(u.user_lname, ''))) AS user_name,
				COALESCE(NULLIF(TRIM(u.email), ''), '-') AS user_email,
				COALESCE(NULLIF(TRIM(u.scopus_id), ''), '-') AS user_scopus_id,
				sd.id AS document_id,
				`+scopusPublicationYearExpr()+` AS publication_year_ce,
				COALESCE(sd.citedby_count, 0) AS cited_by_count,
				COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') AS quartile,
				metrics.cite_score_percentile,
				COALESCE(NULLIF(TRIM(sd.aggregation_type), ''), 'N/A') AS aggregation_type
			`).
			Joins("JOIN scopus_authors sa ON TRIM(u.scopus_id) = sa.scopus_author_id").
			Joins("JOIN scopus_document_authors sda ON sda.author_id = sa.id").
			Joins("JOIN scopus_affiliations aff ON aff.id = sda.affiliation_id").
			Joins("JOIN scopus_documents sd ON sd.id = sda.document_id").
			Joins("LEFT JOIN scopus_source_metrics AS metrics ON metrics.source_id = sd.source_id AND metrics.doc_type = 'all' AND metrics.metric_year = "+metricYearExpr).
			Where("u.delete_at IS NULL AND u.scopus_id IS NOT NULL AND TRIM(u.scopus_id) <> ''").
			Where("LOWER(TRIM(COALESCE(aff.name, ''))) IN (?, ?)", scopusAffiliationNameKKU, scopusAffiliationNameSciKKU)

		personQuery = applyScopusDashboardFilters(personQuery, filters, true)

		if err := personQuery.Find(&personDocRows).Error; err == nil {
			type personAggregate struct {
				UserID           int
				UserName         string
				UserEmail        string
				UserScopusID     string
				PublicationRows  int
				UniqueDocuments  int
				CitedByTotal     int
				T1Count          int
				Q1Count          int
				Q2Count          int
				Q3Count          int
				Q4Count          int
				NACount          int
				JournalCount     int
				ConferenceCount  int
				FirstYearCE      int
				LatestYearCE     int
				activeYears      map[int]struct{}
				seenDocumentByID map[uint]struct{}
				yearDocCounts    map[int]int
			}

			aggByUser := map[int]*personAggregate{}
			for _, row := range personDocRows {
				agg, ok := aggByUser[row.UserID]
				if !ok {
					agg = &personAggregate{
						UserID:           row.UserID,
						UserName:         strings.TrimSpace(row.UserName),
						UserEmail:        strings.TrimSpace(row.UserEmail),
						UserScopusID:     strings.TrimSpace(row.UserScopusID),
						activeYears:      map[int]struct{}{},
						seenDocumentByID: map[uint]struct{}{},
						yearDocCounts:    map[int]int{},
					}
					aggByUser[row.UserID] = agg
				}

				agg.PublicationRows++

				if _, exists := agg.seenDocumentByID[row.DocumentID]; exists {
					continue
				}
				agg.seenDocumentByID[row.DocumentID] = struct{}{}
				agg.UniqueDocuments++
				agg.CitedByTotal += row.CitedByCount

				if strings.EqualFold(strings.TrimSpace(row.AggregationType), "Journal") {
					agg.JournalCount++
				}
				if strings.EqualFold(strings.TrimSpace(row.AggregationType), "Conference Proceeding") {
					agg.ConferenceCount++
				}

				quartile := strings.ToUpper(strings.TrimSpace(row.Quartile))
				if quartile == "" {
					quartile = "N/A"
				}
				isT1 := row.CiteScorePercentile != nil && *row.CiteScorePercentile >= 90 && *row.CiteScorePercentile <= 100
				if isT1 {
					agg.T1Count++
				} else {
					switch quartile {
					case "Q1":
						agg.Q1Count++
					case "Q2":
						agg.Q2Count++
					case "Q3":
						agg.Q3Count++
					case "Q4":
						agg.Q4Count++
					default:
						agg.NACount++
					}
				}

				if row.PublicationYearCE != nil && *row.PublicationYearCE > 0 {
					y := *row.PublicationYearCE
					agg.activeYears[y] = struct{}{}
					agg.yearDocCounts[y]++
					if agg.FirstYearCE == 0 || y < agg.FirstYearCE {
						agg.FirstYearCE = y
					}
					if agg.LatestYearCE == 0 || y > agg.LatestYearCE {
						agg.LatestYearCE = y
					}
				}
			}

			type personSortable struct {
				Data          map[string]interface{}
				UniqueDocs    int
				CitedByTotal  int
				UserName      string
				FirstYearCE   int
				LatestYearCE  int
				UserID        int
				UserEmail     string
				UserScopusID  string
				YearDocCounts map[int]int
			}
			sortableRows := make([]personSortable, 0, len(aggByUser))
			allYearsSet := map[int]struct{}{}
			for _, agg := range aggByUser {
				avg := 0.0
				if agg.UniqueDocuments > 0 {
					avg = float64(agg.CitedByTotal) / float64(agg.UniqueDocuments)
				}
				firstYearBE := 0
				latestYearBE := 0
				if agg.FirstYearCE > 0 {
					firstYearBE = agg.FirstYearCE + 543
				}
				if agg.LatestYearCE > 0 {
					latestYearBE = agg.LatestYearCE + 543
				}

				data := map[string]interface{}{
					"user_id":          agg.UserID,
					"user_name":        agg.UserName,
					"user_email":       agg.UserEmail,
					"user_scopus_id":   agg.UserScopusID,
					"publication_rows": agg.PublicationRows,
					"unique_documents": agg.UniqueDocuments,
					"cited_by_total":   agg.CitedByTotal,
					"avg_cited_by":     avg,
					"t1_count":         agg.T1Count,
					"q1_count":         agg.Q1Count,
					"q2_count":         agg.Q2Count,
					"q3_count":         agg.Q3Count,
					"q4_count":         agg.Q4Count,
					"quartile_na":      agg.NACount,
					"journal_count":    agg.JournalCount,
					"conference_count": agg.ConferenceCount,
					"first_year":       firstYearBE,
					"latest_year":      latestYearBE,
					"active_years":     len(agg.activeYears),
				}

				sortableRows = append(sortableRows, personSortable{
					Data:          data,
					UniqueDocs:    agg.UniqueDocuments,
					CitedByTotal:  agg.CitedByTotal,
					UserName:      agg.UserName,
					FirstYearCE:   agg.FirstYearCE,
					LatestYearCE:  agg.LatestYearCE,
					UserID:        agg.UserID,
					UserEmail:     agg.UserEmail,
					UserScopusID:  agg.UserScopusID,
					YearDocCounts: agg.yearDocCounts,
				})

				for y := range agg.activeYears {
					allYearsSet[y] = struct{}{}
				}
			}

			sort.Slice(sortableRows, func(i, j int) bool {
				if sortableRows[i].UniqueDocs == sortableRows[j].UniqueDocs {
					if sortableRows[i].CitedByTotal == sortableRows[j].CitedByTotal {
						return sortableRows[i].UserName < sortableRows[j].UserName
					}
					return sortableRows[i].CitedByTotal > sortableRows[j].CitedByTotal
				}
				return sortableRows[i].UniqueDocs > sortableRows[j].UniqueDocs
			})

			personSummaryRows = make([]map[string]interface{}, 0, len(sortableRows))
			for _, row := range sortableRows {
				personSummaryRows = append(personSummaryRows, row.Data)
			}

			if len(allYearsSet) > 0 {
				yearsCE := make([]int, 0, len(allYearsSet))
				for y := range allYearsSet {
					yearsCE = append(yearsCE, y)
				}
				sort.Ints(yearsCE)

				yearsBE := make([]int, 0, len(yearsCE))
				for _, y := range yearsCE {
					yearsBE = append(yearsBE, y+543)
				}

				type matrixSortable struct {
					UserID        int
					UserName      string
					UserEmail     string
					UserScopus    string
					FirstYearCE   int
					LatestYearCE  int
					YearDocCounts map[int]int
				}
				matrixRows := make([]matrixSortable, 0, len(sortableRows))
				for _, row := range sortableRows {
					matrixRows = append(matrixRows, matrixSortable{
						UserID:        row.UserID,
						UserName:      row.UserName,
						UserEmail:     row.UserEmail,
						UserScopus:    row.UserScopusID,
						FirstYearCE:   row.FirstYearCE,
						LatestYearCE:  row.LatestYearCE,
						YearDocCounts: row.YearDocCounts,
					})
				}

				sort.Slice(matrixRows, func(i, j int) bool {
					if matrixRows[i].FirstYearCE == matrixRows[j].FirstYearCE {
						if matrixRows[i].LatestYearCE == matrixRows[j].LatestYearCE {
							return matrixRows[i].UserName < matrixRows[j].UserName
						}
						return matrixRows[i].LatestYearCE > matrixRows[j].LatestYearCE
					}
					return matrixRows[i].FirstYearCE < matrixRows[j].FirstYearCE
				})

				matrixPayloadRows := make([]map[string]interface{}, 0, len(matrixRows))
				for _, row := range matrixRows {
					yearCounts := map[string]int{}
					for _, y := range yearsCE {
						yearCounts[strconv.Itoa(y+543)] = row.YearDocCounts[y]
					}
					matrixPayloadRows = append(matrixPayloadRows, map[string]interface{}{
						"user_id":        row.UserID,
						"user_name":      row.UserName,
						"user_email":     row.UserEmail,
						"user_scopus_id": row.UserScopus,
						"year_counts":    yearCounts,
					})
				}

				personYearMatrix = map[string]interface{}{
					"year_start_be": yearsBE[0],
					"year_end_be":   yearsBE[len(yearsBE)-1],
					"years":         yearsBE,
					"rows":          matrixPayloadRows,
				}
			}
		}

		pairRows := make([]scopusInternalCollaborationPairRow, 0)
		pairSQL := `
			SELECT
				d1.user_id AS user_a_id,
				TRIM(CONCAT(COALESCE(ua.user_fname, ''), ' ', COALESCE(ua.user_lname, ''))) AS user_a,
				d2.user_id AS user_b_id,
				TRIM(CONCAT(COALESCE(ub.user_fname, ''), ' ', COALESCE(ub.user_lname, ''))) AS user_b,
				COUNT(DISTINCT d1.document_id) AS shared_documents
			FROM (
				SELECT DISTINCT u.user_id, sda.document_id
				FROM users u
				JOIN scopus_authors sa ON TRIM(u.scopus_id) = sa.scopus_author_id
				JOIN scopus_document_authors sda ON sda.author_id = sa.id
				JOIN scopus_affiliations aff ON aff.id = sda.affiliation_id
				WHERE u.delete_at IS NULL
				  AND u.scopus_id IS NOT NULL
				  AND TRIM(u.scopus_id) <> ''
				  AND LOWER(TRIM(COALESCE(aff.name, ''))) IN (?, ?)
			) d1
			JOIN (
				SELECT DISTINCT u.user_id, sda.document_id
				FROM users u
				JOIN scopus_authors sa ON TRIM(u.scopus_id) = sa.scopus_author_id
				JOIN scopus_document_authors sda ON sda.author_id = sa.id
				JOIN scopus_affiliations aff ON aff.id = sda.affiliation_id
				WHERE u.delete_at IS NULL
				  AND u.scopus_id IS NOT NULL
				  AND TRIM(u.scopus_id) <> ''
				  AND LOWER(TRIM(COALESCE(aff.name, ''))) IN (?, ?)
			) d2 ON d1.document_id = d2.document_id AND d1.user_id < d2.user_id
			JOIN users ua ON ua.user_id = d1.user_id
			JOIN users ub ON ub.user_id = d2.user_id
			GROUP BY d1.user_id, user_a, d2.user_id, user_b
			HAVING COUNT(DISTINCT d1.document_id) >= 1
			ORDER BY shared_documents DESC, user_a ASC, user_b ASC
		`

		if err := config.DB.Raw(pairSQL, scopusAffiliationNameKKU, scopusAffiliationNameSciKKU, scopusAffiliationNameKKU, scopusAffiliationNameSciKKU).Scan(&pairRows).Error; err == nil {
			internalCollaborationPairs = make([]map[string]interface{}, 0, len(pairRows))
			for _, row := range pairRows {
				internalCollaborationPairs = append(internalCollaborationPairs, map[string]interface{}{
					"user_a_id":        row.UserAID,
					"user_a":           row.UserA,
					"user_b_id":        row.UserBID,
					"user_b":           row.UserB,
					"shared_documents": row.SharedDocuments,
				})
			}
		}
	}

	qualityRows := make([]map[string]interface{}, 0, 6)
	for _, key := range []string{"T1", "Q1", "Q2", "Q3", "Q4", "N/A"} {
		total := 0
		if key == "T1" {
			total = t1Documents
		} else {
			total = quartileCounts[key]
		}
		qualityRows = append(qualityRows, map[string]interface{}{
			"value": key,
			"total": total,
		})
	}

	payload := map[string]interface{}{
		"kpi": map[string]interface{}{
			"total_documents":            totalDocuments,
			"total_teachers_with_scopus": totalTeachersInFaculty,
			"total_citations":            totalCitations,
			"avg_citations_per_document": avgCitations,
			"open_access_documents":      openAccessDocuments,
			"t1_documents":               t1Documents,
		},
		"quality_breakdown":               qualityRows,
		"aggregation_breakdown":           aggRows,
		"faculty_quartile_history":        historyRows,
		"faculty_quartile_history_fiscal": fiscalHistoryRows,
		"person_summary":                  personSummaryRows,
		"person_year_matrix":              personYearMatrix,
		"internal_collaboration_pairs":    internalCollaborationPairs,
		"top_publication_sources":         sourceRows,
		"funding_sponsor_breakdown":       sponsorRows,
		"year_breakdown_be":               yearCounts,
		"latest_scopus_pull_at":           latestScopusPullAt,
		"applied_filters": map[string]interface{}{
			"scope":              filters.Scope,
			"year_start_be":      ceToBE(filters.YearStartCE),
			"year_end_be":        ceToBE(filters.YearEndCE),
			"aggregation_types":  filters.AggregationTypes,
			"quality_buckets":    filters.QualityBuckets,
			"open_access_mode":   filters.OpenAccessMode,
			"citation_min":       filters.CitationMin,
			"citation_max":       filters.CitationMax,
			"search_title":       filters.Title,
			"search_doi":         filters.DOI,
			"search_eid":         filters.EID,
			"search_scopus_id":   filters.ScopusID,
			"search_journal":     filters.Journal,
			"search_author":      filters.Author,
			"search_affiliation": filters.Affiliation,
			"search_keyword":     filters.Keyword,
		},
	}

	writeScopusDashboardCache(cacheKey, payload, scopusDashboardSummaryCacheTTL)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    payload,
		"meta": gin.H{
			"cached":      false,
			"ttl_seconds": int(scopusDashboardSummaryCacheTTL / time.Second),
		},
	})
}

func ceToBE(value *int) *int {
	if value == nil {
		return nil
	}
	converted := *value + 543
	return &converted
}
