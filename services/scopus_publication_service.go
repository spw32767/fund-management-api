package services

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

// ScopusPublication represents a normalized publication row returned to clients.
type ScopusPublication struct {
	ID                  uint       `json:"id"`
	Title               string     `json:"title"`
	Abstract            *string    `json:"abstract,omitempty"`
	AggregationType     *string    `json:"aggregation_type,omitempty"`
	PublicationName     *string    `json:"publication_name,omitempty"`
	Venue               *string    `json:"venue,omitempty"`
	SourceID            *string    `json:"source_id,omitempty"`
	PublicationYear     *int       `json:"publication_year,omitempty"`
	CoverDate           *time.Time `json:"cover_date,omitempty"`
	CitedBy             *int       `json:"cited_by,omitempty"`
	CiteScorePercentile *float64   `json:"cite_score_percentile,omitempty"`
	CiteScoreQuartile   *string    `json:"cite_score_quartile,omitempty"`
	CiteScoreStatus     *string    `json:"cite_score_status,omitempty"`
	CiteScoreRank       *int       `json:"cite_score_rank,omitempty"`
	ISSN                *string    `json:"issn,omitempty"`
	EISSN               *string    `json:"eissn,omitempty"`
	ISBN                *string    `json:"isbn,omitempty"`
	Volume              *string    `json:"volume,omitempty"`
	Issue               *string    `json:"issue,omitempty"`
	PageRange           *string    `json:"page_range,omitempty"`
	ArticleNumber       *string    `json:"article_number,omitempty"`
	AuthKeywords        *string    `json:"authkeywords,omitempty"`
	FundSponsor         *string    `json:"fund_sponsor,omitempty"`
	DOI                 *string    `json:"doi,omitempty"`
	URL                 *string    `json:"url,omitempty"`
	EID                 string     `json:"eid"`
	ScopusID            *string    `json:"scopus_id,omitempty"`
	ScopusURL           *string    `json:"scopus_url,omitempty"`
	Source              string     `json:"source"`
}

// ScopusPublicationTrendPoint represents per-year document/citation aggregates.
type ScopusPublicationTrendPoint struct {
	Year      int `json:"year"`
	Documents int `json:"documents"`
	Citations int `json:"citations"`
}

// ScopusPublicationStats captures summary + trend information for a user.
type ScopusPublicationStats struct {
	TotalDocuments int                           `json:"total_documents"`
	TotalCitations int                           `json:"total_citations"`
	Trend          []ScopusPublicationTrendPoint `json:"trend"`
}

// ScopusPublicationMeta captures metadata about the user's Scopus linkage.
type ScopusPublicationMeta struct {
	HasScopusID bool `json:"has_scopus_id"`
	HasAuthor   bool `json:"has_author_record"`
}

// ScopusPublicationService provides read helpers for Scopus documents.
type ScopusPublicationService struct {
	db *gorm.DB
}

type scopusPublicationRow struct {
	ID                  uint
	Title               *string
	Abstract            *string
	AggregationType     *string
	PublicationName     *string
	SourceID            *string
	ISSN                *string
	EISSN               *string
	ISBN                *string
	Volume              *string
	Issue               *string
	PageRange           *string
	ArticleNumber       *string
	CoverDate           *time.Time
	CitedByCount        *int `gorm:"column:citedby_count"`
	DOI                 *string
	EID                 string
	ScopusID            *string  `gorm:"column:scopus_id"`
	ScopusLink          *string  `gorm:"column:scopus_link"`
	CiteScorePercentile *float64 `gorm:"column:cite_score_percentile"`
	CiteScoreQuartile   *string  `gorm:"column:cite_score_quartile"`
	CiteScoreStatus     *string  `gorm:"column:cite_score_status"`
	CiteScoreRank       *int     `gorm:"column:cite_score_rank"`
	AuthKeywords        []byte   `gorm:"column:authkeywords"`
	FundSponsor         *string  `gorm:"column:fund_sponsor"`
}

// NewScopusPublicationService instantiates the service.
func NewScopusPublicationService(db *gorm.DB) *ScopusPublicationService {
	if db == nil {
		db = config.DB
	}
	return &ScopusPublicationService{db: db}
}

// ListByUser returns paginated Scopus publications for the given user.
func (s *ScopusPublicationService) ListByUser(userID uint, limit, offset int, sortField, sortDirection, search string) ([]ScopusPublication, int64, ScopusPublicationMeta, error) {
	meta := ScopusPublicationMeta{}

	if limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	var user models.User
	if err := s.db.Select("Scopus_id").Where("user_id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []ScopusPublication{}, 0, meta, nil
		}
		return nil, 0, meta, err
	}

	scopusID := strings.TrimSpace(stringValue(user.ScopusID))
	if scopusID == "" {
		return []ScopusPublication{}, 0, meta, nil
	}
	meta.HasScopusID = true

	var docIDs *gorm.DB
	var author models.ScopusAuthor
	if err := s.db.Select("id").Where("scopus_author_id = ?", scopusID).First(&author).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Fallback: allow documents that carry the same scopus_id even if we don't
			// have an author linkage yet. This ensures the admin view can still surface
			// imported documents.
			docIDs = s.db.Table("scopus_documents AS sd").
				Select("MIN(sd.id) AS doc_id").
				Where("sd.scopus_id = ?", scopusID).
				Group("sd.eid")
		} else {
			return nil, 0, meta, err
		}
	} else {
		meta.HasAuthor = true
		docIDs = s.db.Table("scopus_documents AS sd").
			Select("MIN(sd.id) AS doc_id").
			Joins("INNER JOIN scopus_document_authors sda ON sda.document_id = sd.id").
			Where("sda.author_id = ?", author.ID).
			Group("sd.eid")
	}

	if search = strings.TrimSpace(search); search != "" {
		like := fmt.Sprintf("%%%s%%", search)
		docIDs = docIDs.Where("sd.title LIKE ?", like)
	}

	countQuery := s.db.Table("(?) AS doc_ids", docIDs.Session(&gorm.Session{NewDB: true}))
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, meta, err
	}
	if total == 0 {
		return []ScopusPublication{}, 0, meta, nil
	}

	orderClause := orderForScopus(sortField, sortDirection)
	metricSubquery := latestCiteScoreMetricsSubquery(s.db)
	base := s.db.Table("scopus_documents AS sd").
		Select("sd.id, sd.title, sd.abstract, sd.aggregation_type, sd.publication_name, sd.source_id, sd.cover_date, sd.citedby_count, sd.doi, sd.eid, sd.scopus_id, sd.scopus_link, sd.issn, sd.eissn, sd.isbn, sd.volume, sd.issue, sd.page_range, sd.article_number, sd.authkeywords, sd.fund_sponsor, metrics.cite_score_percentile, metrics.cite_score_quartile, metrics.cite_score_status, metrics.cite_score_rank").
		Joins("INNER JOIN (?) AS doc_ids ON doc_ids.doc_id = sd.id", docIDs.Session(&gorm.Session{NewDB: true})).
		Joins("LEFT JOIN (?) AS metrics ON metrics.source_id = sd.source_id", metricSubquery)

	var rows []scopusPublicationRow
	if err := base.Session(&gorm.Session{}).Order(orderClause).Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, meta, err
	}

	return mapScopusRows(rows), total, meta, nil
}

// ListAll returns paginated Scopus publications across all users.
func (s *ScopusPublicationService) ListAll(limit, offset int, sortField, sortDirection, search string) ([]ScopusPublication, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	metricSubquery := latestCiteScoreMetricsSubquery(s.db)
	base := s.db.Table("scopus_documents AS sd").
		Select("sd.id, sd.title, sd.abstract, sd.aggregation_type, sd.publication_name, sd.source_id, sd.cover_date, sd.citedby_count, sd.doi, sd.eid, sd.scopus_id, sd.scopus_link, sd.issn, sd.eissn, sd.isbn, sd.volume, sd.issue, sd.page_range, sd.article_number, sd.authkeywords, sd.fund_sponsor, metrics.cite_score_percentile, metrics.cite_score_quartile, metrics.cite_score_status, metrics.cite_score_rank").
		Joins("LEFT JOIN (?) AS metrics ON metrics.source_id = sd.source_id", metricSubquery)

	if search = strings.TrimSpace(search); search != "" {
		like := fmt.Sprintf("%%%s%%", search)
		base = base.Where(
			"sd.title LIKE ? OR sd.doi LIKE ? OR sd.eid LIKE ? OR sd.scopus_id LIKE ? OR sd.publication_name LIKE ?",
			like, like, like, like, like,
		)
	}

	// Preserve the table metadata so GORM can count correctly.
	countQuery := base.Session(&gorm.Session{})
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []ScopusPublication{}, 0, nil
	}

	orderClause := orderForScopus(sortField, sortDirection)

	var rows []scopusPublicationRow
	if err := base.Session(&gorm.Session{}).Order(orderClause).Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	return mapScopusRows(rows), total, nil
}

func mapScopusRows(rows []scopusPublicationRow) []ScopusPublication {
	publications := make([]ScopusPublication, 0, len(rows))
	for _, row := range rows {
		publication := ScopusPublication{
			ID:                  row.ID,
			Title:               strings.TrimSpace(stringOrEmpty(row.Title)),
			Abstract:            normalizeNullable(row.Abstract),
			AggregationType:     normalizeNullable(row.AggregationType),
			Venue:               row.PublicationName,
			SourceID:            normalizeNullable(row.SourceID),
			Source:              "scopus",
			CitedBy:             row.CitedByCount,
			CiteScorePercentile: row.CiteScorePercentile,
			CiteScoreQuartile:   row.CiteScoreQuartile,
			CiteScoreStatus:     row.CiteScoreStatus,
			CiteScoreRank:       row.CiteScoreRank,
			ISSN:                normalizeNullable(row.ISSN),
			EISSN:               normalizeNullable(row.EISSN),
			ISBN:                normalizeNullable(row.ISBN),
			Volume:              normalizeNullable(row.Volume),
			Issue:               normalizeNullable(row.Issue),
			PageRange:           normalizeNullable(row.PageRange),
			ArticleNumber:       normalizeNullable(row.ArticleNumber),
			FundSponsor:         normalizeNullable(row.FundSponsor),
			DOI:                 normalizeNullable(row.DOI),
			EID:                 row.EID,
			ScopusID:            row.ScopusID,
		}
		publication.CoverDate = row.CoverDate
		publication.PublicationName = row.PublicationName

		if len(row.AuthKeywords) > 0 {
			keywords := strings.TrimSpace(string(row.AuthKeywords))
			if keywords != "" {
				publication.AuthKeywords = &keywords
			}
		}

		if link := normalizeNullable(row.ScopusLink); link != nil {
			publication.ScopusURL = link
			if publication.URL == nil {
				publication.URL = link
			}
		}

		if row.CoverDate != nil {
			year := row.CoverDate.Year()
			if year > 0 {
				publication.PublicationYear = &year
			}
		}

		if publication.DOI != nil && *publication.DOI != "" {
			doiURL := fmt.Sprintf("https://doi.org/%s", strings.TrimSpace(*publication.DOI))
			publication.URL = &doiURL
		}

		if publication.ScopusURL == nil {
			if link := buildScopusURL(row.EID); link != nil {
				publication.ScopusURL = link
				if publication.URL == nil {
					publication.URL = link
				}
			}
		}

		publications = append(publications, publication)
	}

	return publications
}

func latestCiteScoreMetricsSubquery(db *gorm.DB) *gorm.DB {
	return db.Table("scopus_source_metrics AS ssm").
		Select("ssm.source_id, ssm.cite_score_percentile, ssm.cite_score_quartile, ssm.cite_score_status, ssm.cite_score_rank").
		Where("ssm.doc_type = ?", "all").
		Where(
			"ssm.metric_year = (SELECT MAX(metric_year) FROM scopus_source_metrics WHERE source_id = ssm.source_id AND doc_type = ssm.doc_type)",
		)
}

func yearExpression(db *gorm.DB) string {
	switch db.Dialector.Name() {
	case "sqlite":
		return "COALESCE(CAST(strftime('%Y', sd.cover_date) AS INTEGER), CAST(substr(sd.cover_display_date, -4) AS INTEGER))"
	default:
		return "COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED))"
	}
}

func scopusDocumentGroupExpression(db *gorm.DB) string {
	// We only want to collapse rows that truly share the same Scopus EID. When
	// an EID is missing we treat each row as unique by falling back to the
	// document's numeric ID so distinct publications don't get merged.
	switch db.Dialector.Name() {
	case "sqlite":
		return "COALESCE(NULLIF(sd.eid, ''), CAST(sd.id AS TEXT))"
	default:
		return "COALESCE(NULLIF(sd.eid, ''), CAST(sd.id AS CHAR))"
	}
}

// StatsByUser returns aggregate Scopus publication stats for the given user.
func (s *ScopusPublicationService) StatsByUser(userID uint) (ScopusPublicationStats, ScopusPublicationMeta, error) {
	stats := ScopusPublicationStats{Trend: []ScopusPublicationTrendPoint{}}
	meta := ScopusPublicationMeta{}

	var user models.User
	if err := s.db.Select("Scopus_id").Where("user_id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return stats, meta, nil
		}
		return stats, meta, err
	}

	scopusID := strings.TrimSpace(stringValue(user.ScopusID))
	if scopusID == "" {
		return stats, meta, nil
	}
	meta.HasScopusID = true

	yearExpr := yearExpression(s.db)
	yearCondition := fmt.Sprintf("%s IS NOT NULL AND %s > 0", yearExpr, yearExpr)

	var docIDs *gorm.DB
	var author models.ScopusAuthor
	if err := s.db.Select("id").Where("scopus_author_id = ?", scopusID).First(&author).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Fallback: group documents directly by scopus_id when author linkage is missing
			docIDs = s.db.Table("scopus_documents AS sd").
				Select("MIN(sd.id) AS doc_id, MAX(COALESCE(sd.citedby_count, 0)) AS citations").
				Where("sd.scopus_id = ?", scopusID).
				Where(yearCondition).
				Group(scopusDocumentGroupExpression(s.db))
		} else {
			return stats, meta, err
		}
	} else {
		meta.HasAuthor = true
		docIDs = s.db.Table("scopus_documents AS sd").
			Select("MIN(sd.id) AS doc_id, MAX(COALESCE(sd.citedby_count, 0)) AS citations").
			Joins("INNER JOIN scopus_document_authors sda ON sda.document_id = sd.id").
			Where("sda.author_id = ?", author.ID).
			Where(yearCondition).
			Group(scopusDocumentGroupExpression(s.db))
	}

	var dedupCount int64
	dedupCountQuery := s.db.Raw(
		"SELECT COUNT(*) FROM (?) AS doc_ids",
		docIDs.Session(&gorm.Session{NewDB: true}),
	)
	if err := dedupCountQuery.Scan(&dedupCount).Error; err != nil {
		return stats, meta, err
	}
	if dedupCount == 0 {
		return stats, meta, nil
	}

	type trendRow struct {
		Year      int
		Documents int64
		Citations int64
	}

	var rawCount int64
	if meta.HasAuthor {
		baseCount := s.db.Table("scopus_documents AS sd").
			Select("sd.id").
			Joins("INNER JOIN scopus_document_authors sda ON sda.document_id = sd.id").
			Where("sda.author_id = ?", author.ID).
			Where(yearCondition)
		if err := baseCount.Count(&rawCount).Error; err != nil {
			return stats, meta, err
		}
	} else {
		baseCount := s.db.Table("scopus_documents AS sd").
			Select("sd.id").
			Where("sd.scopus_id = ?", scopusID).
			Where(yearCondition)
		if err := baseCount.Count(&rawCount).Error; err != nil {
			return stats, meta, err
		}
	}

	documentsSubquery := s.db.Table("scopus_documents AS sd").
		Select(fmt.Sprintf("%s AS year, doc_ids.citations AS citations", yearExpr)).
		Joins("INNER JOIN (?) AS doc_ids ON doc_ids.doc_id = sd.id", docIDs.Session(&gorm.Session{NewDB: true})).
		Where(yearCondition)

	var rows []trendRow
	err := s.db.Table("(?) AS doc_rows", documentsSubquery).
		Select("year, COUNT(*) AS documents, COALESCE(SUM(citations), 0) AS citations").
		Group("year").
		Order("year ASC").
		Scan(&rows).Error
	if err != nil {
		return stats, meta, err
	}

	if len(rows) == 0 {
		return stats, meta, nil
	}

	if rawCount > dedupCount {
		log.Printf("scopus stats: deduplicated %d duplicate document rows for user %d", rawCount-dedupCount, userID)
	}

	stats.Trend = make([]ScopusPublicationTrendPoint, 0, len(rows))
	for _, row := range rows {
		point := ScopusPublicationTrendPoint{
			Year:      row.Year,
			Documents: int(row.Documents),
			Citations: int(row.Citations),
		}
		stats.TotalDocuments += point.Documents
		stats.TotalCitations += point.Citations
		stats.Trend = append(stats.Trend, point)
	}

	return stats, meta, nil
}

func orderForScopus(field, direction string) string {
	dir := strings.ToUpper(direction)
	if dir != "ASC" {
		dir = "DESC"
	}
	switch strings.ToLower(field) {
	case "title":
		return fmt.Sprintf("sd.title %s", dir)
	case "cited_by":
		return fmt.Sprintf("sd.citedby_count %s", dir)
	default:
		return fmt.Sprintf("sd.cover_date %s", dir)
	}
}

func buildScopusURL(eid string) *string {
	trimmed := strings.TrimSpace(eid)
	if trimmed == "" {
		return nil
	}
	encoded := url.QueryEscape(trimmed)
	link := fmt.Sprintf("https://www.scopus.com/record/display.uri?eid=%s", encoded)
	return &link
}

func stringOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func normalizeNullable(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
