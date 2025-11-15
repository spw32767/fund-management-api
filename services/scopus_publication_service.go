package services

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

// ScopusPublication represents a normalized publication row returned to clients.
type ScopusPublication struct {
	ID              uint    `json:"id"`
	Title           string  `json:"title"`
	PublicationName *string `json:"publication_name,omitempty"`
	Venue           *string `json:"venue,omitempty"`
	PublicationYear *int    `json:"publication_year,omitempty"`
	CitedBy         *int    `json:"cited_by,omitempty"`
	DOI             *string `json:"doi,omitempty"`
	URL             *string `json:"url,omitempty"`
	EID             string  `json:"eid"`
	ScopusID        *string `json:"scopus_id,omitempty"`
	ScopusURL       *string `json:"scopus_url,omitempty"`
	Source          string  `json:"source"`
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

	var author models.ScopusAuthor
	if err := s.db.Select("id").Where("scopus_author_id = ?", scopusID).First(&author).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []ScopusPublication{}, 0, meta, nil
		}
		return nil, 0, meta, err
	}
	meta.HasAuthor = true

	base := s.db.Table("scopus_documents AS sd").
		Select("sd.id, sd.title, sd.publication_name, sd.cover_date, sd.citedby_count, sd.doi, sd.eid, sd.scopus_id").
		Joins("INNER JOIN scopus_document_authors sda ON sda.document_id = sd.id").
		Where("sda.author_id = ?", author.ID)

	if search = strings.TrimSpace(search); search != "" {
		like := fmt.Sprintf("%%%s%%", search)
		base = base.Where("sd.title LIKE ?", like)
	}

	orderClause := orderForScopus(sortField, sortDirection)
	countQuery := base.Session(&gorm.Session{})
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, meta, err
	}
	if total == 0 {
		return []ScopusPublication{}, 0, meta, nil
	}

	type scopusPublicationRow struct {
		ID              uint
		Title           *string
		PublicationName *string
		CoverDate       *time.Time
		CitedByCount    *int `gorm:"column:citedby_count"`
		DOI             *string
		EID             string
		ScopusID        *string `gorm:"column:scopus_id"`
	}

	var rows []scopusPublicationRow
	if err := base.Session(&gorm.Session{}).Order(orderClause).Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, meta, err
	}

	publications := make([]ScopusPublication, 0, len(rows))
	for _, row := range rows {
		publication := ScopusPublication{
			ID:       row.ID,
			Title:    strings.TrimSpace(stringOrEmpty(row.Title)),
			Venue:    row.PublicationName,
			Source:   "scopus",
			CitedBy:  row.CitedByCount,
			DOI:      normalizeNullable(row.DOI),
			EID:      row.EID,
			ScopusID: row.ScopusID,
		}
		publication.PublicationName = row.PublicationName

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

	return publications, total, meta, nil
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

	var author models.ScopusAuthor
	if err := s.db.Select("id").Where("scopus_author_id = ?", scopusID).First(&author).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return stats, meta, nil
		}
		return stats, meta, err
	}
	meta.HasAuthor = true

	documentsBase := s.db.Table("scopus_documents AS sd").
		Joins("INNER JOIN scopus_document_authors sda ON sda.document_id = sd.id").
		Where("sda.author_id = ?", author.ID)

	yearExprSD := scopusYearExpression("sd", s.db)
	yearConditionSD := buildYearCondition(yearExprSD)

	var rawCount int64
	if err := documentsBase.Session(&gorm.Session{}).Where(yearConditionSD).Count(&rawCount).Error; err != nil {
		return stats, meta, err
	}
	if rawCount == 0 {
		return stats, meta, nil
	}

	eidGroupingExpr := scopusEIDGroupingExpression("sd", s.db)
	documentsSubquery := documentsBase.Session(&gorm.Session{}).
		Select(fmt.Sprintf("%s AS dedup_eid, MIN(sd.id) AS document_id, MAX(sd.cover_date) AS cover_date, MAX(sd.cover_display_date) AS cover_display_date, MAX(COALESCE(sd.citedby_count, 0)) AS citations", eidGroupingExpr)).
		Group("dedup_eid")

	docRows := s.db.Table("(?) AS doc_rows", documentsSubquery)
	yearExprRows := scopusYearExpression("doc_rows", s.db)
	yearConditionRows := buildYearCondition(yearExprRows)
	docRows = docRows.Where(yearConditionRows)

	var dedupCount int64
	if err := docRows.Session(&gorm.Session{}).Count(&dedupCount).Error; err != nil {
		return stats, meta, err
	}

	type trendRow struct {
		Year      int
		Documents int64
		Citations int64
	}

	var rows []trendRow
	err := docRows.Session(&gorm.Session{}).
		Select(fmt.Sprintf("%s AS year, COUNT(*) AS documents, COALESCE(SUM(citations), 0) AS citations", yearExprRows)).
		Group("year").
		Order("year ASC").
		Scan(&rows).Error
	if err != nil {
		return stats, meta, err
	}

	if rawCount != dedupCount {
		fmt.Printf("[scopus] StatsByUser detected %d duplicate document rows for author_id=%d (user_id=%d)\n", rawCount-dedupCount, author.ID, userID)
	}

	if len(rows) == 0 {
		return stats, meta, nil
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

func buildYearCondition(yearExpr string) string {
	return fmt.Sprintf("%s IS NOT NULL AND %s > 0", yearExpr, yearExpr)
}

func scopusYearExpression(alias string, db *gorm.DB) string {
	if db == nil || db.Dialector == nil {
		return fmt.Sprintf("COALESCE(YEAR(%s.cover_date), CAST(RIGHT(%s.cover_display_date, 4) AS UNSIGNED))", alias, alias)
	}
	switch db.Dialector.Name() {
	case "sqlite":
		return fmt.Sprintf("COALESCE(CAST(strftime('%%Y', %s.cover_date) AS INTEGER), CAST(substr(%s.cover_display_date, -4) AS INTEGER))", alias, alias)
	default:
		return fmt.Sprintf("COALESCE(YEAR(%s.cover_date), CAST(RIGHT(%s.cover_display_date, 4) AS UNSIGNED))", alias, alias)
	}
}

func scopusEIDGroupingExpression(alias string, db *gorm.DB) string {
	if db == nil || db.Dialector == nil {
		return fmt.Sprintf("COALESCE(NULLIF(%s.eid, ''), CONCAT('internal-', %s.id))", alias, alias)
	}
	switch db.Dialector.Name() {
	case "sqlite":
		return fmt.Sprintf("COALESCE(NULLIF(%s.eid, ''), printf('internal-%%d', %s.id))", alias, alias)
	default:
		return fmt.Sprintf("COALESCE(NULLIF(%s.eid, ''), CONCAT('internal-', %s.id))", alias, alias)
	}
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
