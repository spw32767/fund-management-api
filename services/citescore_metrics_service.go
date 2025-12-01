package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const citeScoreBaseURL = "https://api.elsevier.com/content/serial/title/issn"

// CiteScoreMetricsService fetches and stores CiteScore metrics for journals.
type CiteScoreMetricsService struct {
	db     *gorm.DB
	client *http.Client
}

// CiteScoreBackfillSummary reports the result of a backfill run over existing Scopus documents.
type CiteScoreBackfillSummary struct {
	JournalsScanned int `json:"journals_scanned"`
	MetricsFetched  int `json:"metrics_fetched"`
	SkippedExisting int `json:"skipped_existing"`
	Errors          int `json:"errors"`
}

// NewCiteScoreMetricsService constructs a CiteScoreMetricsService.
func NewCiteScoreMetricsService(db *gorm.DB, client *http.Client) *CiteScoreMetricsService {
	if db == nil {
		db = config.DB
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &CiteScoreMetricsService{db: db, client: client}
}

// EnsureJournalMetrics fetches CiteScore metrics for a journal if they are not already stored for the given year.
func (s *CiteScoreMetricsService) EnsureJournalMetrics(ctx context.Context, issn, sourceID string, metricYear int) error {
	issn = strings.TrimSpace(issn)
	sourceID = strings.TrimSpace(sourceID)
	if issn == "" && sourceID == "" {
		return nil
	}

	if metricYear > 0 {
		exists, err := s.metricExists(ctx, issn, sourceID, metricYear)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}

	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return err
	}

	entry, err := s.fetchMetrics(ctx, apiKey, issn, sourceID)
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}

	return s.persistMetrics(ctx, entry)
}

// BackfillMissingMetrics scans existing Scopus documents and fetches CiteScore metrics for journals
// that do not yet have stored metrics. It returns a summary of the backfill run.
func (s *CiteScoreMetricsService) BackfillMissingMetrics(ctx context.Context) (*CiteScoreBackfillSummary, error) {
	if s == nil {
		return nil, errors.New("citescore metrics service is nil")
	}

	var targets []struct {
		SourceID  *string
		ISSN      *string
		CoverDate *time.Time
	}

	query := s.db.WithContext(ctx).Model(&models.ScopusDocument{}).
		Select("source_id", "issn", "MAX(cover_date) AS cover_date").
		Where("(source_id IS NOT NULL AND source_id <> '') OR (issn IS NOT NULL AND issn <> '')").
		Group("source_id, issn")

	if err := query.Find(&targets).Error; err != nil {
		return nil, err
	}

	summary := &CiteScoreBackfillSummary{}
	seen := make(map[string]struct{})

	for _, target := range targets {
		issn := ""
		if target.ISSN != nil {
			issn = *target.ISSN
		}
		sourceID := ""
		if target.SourceID != nil {
			sourceID = *target.SourceID
		}

		issn = strings.TrimSpace(issn)
		sourceID = strings.TrimSpace(sourceID)
		if issn == "" && sourceID == "" {
			continue
		}

		key := issn + "|" + sourceID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		summary.JournalsScanned++

		metricYear := 0
		if target.CoverDate != nil {
			metricYear = target.CoverDate.Year()
		}

		exists, err := s.metricExistsAny(ctx, issn, sourceID)
		if err != nil {
			return nil, err
		}
		if exists {
			summary.SkippedExisting++
			continue
		}

		if err := s.EnsureJournalMetrics(ctx, issn, sourceID, metricYear); err != nil {
			summary.Errors++
			log.Printf("citescore backfill: failed for issn %s source %s: %v", issn, sourceID, err)
			continue
		}

		summary.MetricsFetched++
	}

	return summary, nil
}

func (s *CiteScoreMetricsService) metricExists(ctx context.Context, issn, sourceID string, metricYear int) (bool, error) {
	query := s.db.WithContext(ctx).Model(&models.ScopusSourceMetric{})
	if sourceID != "" {
		query = query.Where("source_id = ?", sourceID)
	} else {
		query = query.Where("issn = ?", issn)
	}
	if metricYear > 0 {
		query = query.Where("metric_year = ?", metricYear)
	}

	var count int64
	if err := query.Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *CiteScoreMetricsService) metricExistsAny(ctx context.Context, issn, sourceID string) (bool, error) {
	query := s.db.WithContext(ctx).Model(&models.ScopusSourceMetric{})
	if sourceID != "" {
		query = query.Where("source_id = ?", sourceID)
	} else {
		query = query.Where("issn = ?", issn)
	}

	var count int64
	if err := query.Limit(1).Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

func (s *CiteScoreMetricsService) fetchMetrics(ctx context.Context, apiKey, issn, sourceID string) (*citeScoreEntry, error) {
	target := strings.TrimSpace(issn)
	if target == "" {
		target = strings.TrimSpace(sourceID)
	}
	if target == "" {
		return nil, nil
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/%s", citeScoreBaseURL, url.PathEscape(target)))
	if err != nil {
		return nil, err
	}
	q := reqURL.Query()
	q.Set("view", "CITESCORE")
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(scopusAPIKeyField, apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("citescore api error: status %d", resp.StatusCode)
	}

	var payload citeScoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode citescore response: %w", err)
	}
	if len(payload.SerialMetadataResponse.Entry) == 0 {
		return nil, nil
	}

	return &payload.SerialMetadataResponse.Entry[0], nil
}

func (s *CiteScoreMetricsService) persistMetrics(ctx context.Context, entry *citeScoreEntry) error {
	if entry == nil {
		return nil
	}

	sjrByYear := entry.SJRList.toYearMap()
	snipByYear := entry.SNIPList.toYearMap()
	currentMetric := parseFloatPointer(entry.CiteScoreYearInfoList.CurrentMetric)
	currentMetricYear := parseIntPointer(entry.CiteScoreYearInfoList.CurrentMetricYear)
	tracker := parseFloatPointer(entry.CiteScoreYearInfoList.Tracker)
	trackerYear := parseIntPointer(entry.CiteScoreYearInfoList.TrackerYear)
	fetchedAt := time.Now().UTC()

	for _, info := range entry.CiteScoreYearInfoList.YearInfo {
		metricYear := parseIntSafe(info.Year)
		if metricYear == 0 {
			continue
		}
		for _, csInfo := range info.CiteScoreInformationList.Items {
			docType := strings.TrimSpace(csInfo.DocType)
			if docType == "" {
				docType = "all"
			}

			percentile, rank := bestPercentileAndRank(csInfo.CiteScoreSubjectRank)
			metric := &models.ScopusSourceMetric{
				SourceID:                   strings.TrimSpace(entry.SourceID),
				ISSN:                       optionalStringValue(entry.ISSN),
				EISSN:                      optionalStringValue(entry.EISSN),
				MetricYear:                 metricYear,
				DocType:                    docType,
				CiteScore:                  parseFloatPointer(csInfo.CiteScore),
				CiteScoreStatus:            optionalStringValue(info.Status),
				CiteScoreScholarlyOutput:   parseIntPointer(csInfo.ScholarlyOutput),
				CiteScoreCitationCount:     parseIntPointer(csInfo.CitationCount),
				CiteScorePercentCited:      parseFloatPointer(csInfo.PercentCited),
				CiteScoreRank:              rank,
				CiteScorePercentile:        percentile,
				CiteScoreQuartile:          percentileToQuartile(percentile),
				CiteScoreCurrentMetric:     currentMetric,
				CiteScoreCurrentMetricYear: currentMetricYear,
				CiteScoreTracker:           tracker,
				CiteScoreTrackerYear:       trackerYear,
				SJR:                        sjrByYear[metricYear],
				SNIP:                       snipByYear[metricYear],
				LastFetchedAt:              &fetchedAt,
			}

			if metric.SourceID == "" && metric.ISSN != nil {
				metric.SourceID = *metric.ISSN
			}
			if strings.TrimSpace(metric.SourceID) == "" {
				continue
			}

			if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "source_id"}, {Name: "metric_year"}, {Name: "doc_type"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"issn", "eissn", "cite_score", "cite_score_status", "cite_score_scholarly_output",
					"cite_score_citation_count", "cite_score_percent_cited", "cite_score_rank", "cite_score_percentile",
					"cite_score_quartile", "cite_score_current_metric", "cite_score_current_metric_year", "cite_score_tracker",
					"cite_score_tracker_year", "sjr", "snip", "last_fetched_at",
				}),
			}).Create(metric).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

// Data structures for parsing CiteScore API responses.
type citeScoreResponse struct {
	SerialMetadataResponse struct {
		Entry []citeScoreEntry `json:"entry"`
	} `json:"serial-metadata-response"`
}

type citeScoreEntry struct {
	SourceID              string                `json:"source-id"`
	ISSN                  string                `json:"prism:issn"`
	EISSN                 string                `json:"prism:eIssn"`
	CiteScoreYearInfoList citeScoreYearInfoList `json:"citeScoreYearInfoList"`
	SNIPList              citeScoreMetricList   `json:"SNIPList"`
	SJRList               citeScoreMetricList   `json:"SJRList"`
}

type citeScoreMetricList struct {
	SNIP []citeScoreYearValue `json:"SNIP"`
	SJR  []citeScoreYearValue `json:"SJR"`
}

func (l citeScoreMetricList) toYearMap() map[int]*float64 {
	result := make(map[int]*float64)
	for _, v := range append(l.SNIP, l.SJR...) {
		year := parseIntSafe(v.Year)
		if year == 0 {
			continue
		}
		if val := parseFloatPointer(v.Value); val != nil {
			result[year] = val
		}
	}
	return result
}

type citeScoreYearValue struct {
	Year  string `json:"@year"`
	Value string `json:"$"`
}

type citeScoreYearInfoList struct {
	CurrentMetric     string             `json:"citeScoreCurrentMetric"`
	CurrentMetricYear string             `json:"citeScoreCurrentMetricYear"`
	Tracker           string             `json:"citeScoreTracker"`
	TrackerYear       string             `json:"citeScoreTrackerYear"`
	YearInfo          citeScoreYearInfos `json:"citeScoreYearInfo"`
}

type citeScoreYearInfos []citeScoreYearInfo

type citeScoreYearInfo struct {
	Year                     string                     `json:"@year"`
	Status                   string                     `json:"@status"`
	CiteScoreInformationList citeScoreInformationHolder `json:"citeScoreInformationList"`
}

type citeScoreInformationHolder struct {
	Items citeScoreInfos `json:"citeScoreInfo"`
}

func (h *citeScoreInformationHolder) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}

	// The API can return citeScoreInformationList in several shapes:
	// 1) An object: { "citeScoreInfo": {...} }
	// 2) An array of the above objects: [ { "citeScoreInfo": [...] }, ... ]
	// 3) An array of citeScoreInfo objects directly. Normalize all to a flat list.
	type wrapper struct {
		Items citeScoreInfos `json:"citeScoreInfo"`
	}

	// Attempt array handling first to catch both wrapper arrays and direct citeScoreInfo arrays.
	if len(data) > 0 && data[0] == '[' {
		var rawItems []json.RawMessage
		if err := json.Unmarshal(data, &rawItems); err != nil {
			return err
		}

		var merged citeScoreInfos
		for _, raw := range rawItems {
			// Try wrapper form.
			var w wrapper
			if err := json.Unmarshal(raw, &w); err == nil && len(w.Items) > 0 {
				merged = append(merged, w.Items...)
				continue
			}

			// Try direct citeScoreInfos array.
			var direct citeScoreInfos
			if err := json.Unmarshal(raw, &direct); err == nil && len(direct) > 0 {
				merged = append(merged, direct...)
				continue
			}

			// Try single citeScoreInfo object.
			var singleInfo citeScoreInfo
			if err := json.Unmarshal(raw, &singleInfo); err == nil {
				merged = append(merged, singleInfo)
			}
		}

		h.Items = merged
		return nil
	}

	// Handle single object wrapper or direct citeScoreInfo object.
	var w wrapper
	if err := json.Unmarshal(data, &w); err == nil && len(w.Items) > 0 {
		h.Items = w.Items
		return nil
	}

	var direct citeScoreInfos
	if err := json.Unmarshal(data, &direct); err == nil && len(direct) > 0 {
		h.Items = direct
		return nil
	}

	var singleInfo citeScoreInfo
	if err := json.Unmarshal(data, &singleInfo); err != nil {
		return err
	}
	h.Items = []citeScoreInfo{singleInfo}
	return nil
}

type citeScoreInfos []citeScoreInfo

type citeScoreInfo struct {
	DocType              string                `json:"docType"`
	ScholarlyOutput      string                `json:"scholarlyOutput"`
	CitationCount        string                `json:"citationCount"`
	CiteScore            string                `json:"citeScore"`
	PercentCited         string                `json:"percentCited"`
	CiteScoreSubjectRank citeScoreSubjectRanks `json:"citeScoreSubjectRank"`
}

type citeScoreSubjectRanks []citeScoreSubjectRank

type citeScoreSubjectRank struct {
	Rank       string `json:"rank"`
	Percentile string `json:"percentile"`
}

// Custom unmarshallers to support array or object responses.
func (l *citeScoreYearInfos) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '[' {
		var arr []citeScoreYearInfo
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*l = arr
		return nil
	}
	var single citeScoreYearInfo
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*l = []citeScoreYearInfo{single}
	return nil
}

func (l *citeScoreInfos) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '[' {
		var arr []citeScoreInfo
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*l = arr
		return nil
	}
	var single citeScoreInfo
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*l = []citeScoreInfo{single}
	return nil
}

func (l *citeScoreSubjectRanks) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '[' {
		var arr []citeScoreSubjectRank
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*l = arr
		return nil
	}
	var single citeScoreSubjectRank
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*l = []citeScoreSubjectRank{single}
	return nil
}

// Helper parsers.
func parseFloatPointer(val string) *float64 {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func percentileToQuartile(percentile *float64) *string {
	if percentile == nil {
		return nil
	}
	p := *percentile
	var quartile string
	switch {
	case p >= 75:
		quartile = "Q1"
	case p >= 50:
		quartile = "Q2"
	case p >= 25:
		quartile = "Q3"
	case p > 0:
		quartile = "Q4"
	default:
		return nil
	}
	return &quartile
}

func bestPercentileAndRank(ranks []citeScoreSubjectRank) (*float64, *int) {
	var bestPercentile *float64
	var bestRank *int
	for _, r := range ranks {
		p := parseFloatPointer(r.Percentile)
		rank := parseIntPointer(r.Rank)
		if p == nil && rank == nil {
			continue
		}
		if bestPercentile == nil {
			bestPercentile = p
			bestRank = rank
			continue
		}
		if p != nil && (bestPercentile == nil || *p > *bestPercentile) {
			bestPercentile = p
			if rank != nil {
				bestRank = rank
			}
			continue
		}
		if p == nil && rank != nil && bestRank == nil {
			bestRank = rank
		}
	}
	return bestPercentile, bestRank
}

func optionalStringValue(val string) *string {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	return &val
}
