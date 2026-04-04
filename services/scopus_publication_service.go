package services

import (
	"encoding/json"
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
	Subtype             *string    `json:"subtype,omitempty"`
	SubtypeDescription  *string    `json:"subtype_description,omitempty"`
	PublicationName     *string    `json:"publication_name,omitempty"`
	AffiliationAFID     *string    `json:"affiliation_afid,omitempty"`
	AffiliationName     *string    `json:"affiliation_name,omitempty"`
	AffiliationCity     *string    `json:"affiliation_city,omitempty"`
	AffiliationCountry  *string    `json:"affiliation_country,omitempty"`
	AffiliationURL      *string    `json:"affiliation_url,omitempty"`
	AffiliationsJSON    *string    `json:"affiliations_json,omitempty"`
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

// ScopusPublicationByUser represents a publication associated with a specific user.
type ScopusPublicationByUser struct {
	UserID              uint       `json:"user_id"`
	UserName            string     `json:"user_name"`
	UserEmail           string     `json:"user_email"`
	UserScopusID        *string    `json:"user_scopus_id,omitempty"`
	DocumentID          uint       `json:"document_id"`
	Title               string     `json:"title"`
	PublicationName     *string    `json:"publication_name,omitempty"`
	AffiliationAFID     *string    `json:"affiliation_afid,omitempty"`
	AffiliationName     *string    `json:"affiliation_name,omitempty"`
	AffiliationCity     *string    `json:"affiliation_city,omitempty"`
	AffiliationCountry  *string    `json:"affiliation_country,omitempty"`
	AffiliationURL      *string    `json:"affiliation_url,omitempty"`
	AffiliationsJSON    *string    `json:"affiliations_json,omitempty"`
	SourceID            *string    `json:"source_id,omitempty"`
	PublicationYear     *int       `json:"publication_year,omitempty"`
	CoverDate           *time.Time `json:"cover_date,omitempty"`
	CitedBy             *int       `json:"cited_by,omitempty"`
	DOI                 *string    `json:"doi,omitempty"`
	EID                 string     `json:"eid"`
	ScopusID            *string    `json:"scopus_id,omitempty"`
	ScopusURL           *string    `json:"scopus_url,omitempty"`
	CiteScorePercentile *float64   `json:"cite_score_percentile,omitempty"`
	CiteScoreQuartile   *string    `json:"cite_score_quartile,omitempty"`
	CiteScoreStatus     *string    `json:"cite_score_status,omitempty"`
	CiteScoreRank       *int       `json:"cite_score_rank,omitempty"`
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
	Subtype             *string
	SubtypeDescription  *string `gorm:"column:subtype_description"`
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

type scopusPublicationByUserRow struct {
	UserID              uint
	UserName            string  `gorm:"column:user_name"`
	UserEmail           *string `gorm:"column:user_email"`
	UserScopusID        *string `gorm:"column:user_scopus_id"`
	DocumentID          uint    `gorm:"column:document_id"`
	Title               *string
	PublicationName     *string
	SourceID            *string
	CoverDate           *time.Time
	CoverDisplayDate    *string `gorm:"column:cover_display_date"`
	CitedByCount        *int    `gorm:"column:citedby_count"`
	DOI                 *string
	EID                 string
	ScopusID            *string `gorm:"column:scopus_id"`
	ScopusLink          *string `gorm:"column:scopus_link"`
	CiteScorePercentile *float64
	CiteScoreQuartile   *string
	CiteScoreStatus     *string
	CiteScoreRank       *int
}

type scopusDocumentAffiliationRow struct {
	DocumentID     uint    `gorm:"column:document_id"`
	AffiliationID  *uint   `gorm:"column:affiliation_id"`
	Afid           *string `gorm:"column:afid"`
	Name           *string `gorm:"column:name"`
	City           *string `gorm:"column:city"`
	Country        *string `gorm:"column:country"`
	AffiliationURL *string `gorm:"column:affiliation_url"`
}

type scopusAffiliationAggregate struct {
	AFID    *string
	Name    *string
	City    *string
	Country *string
	URL     *string
	JSON    *string
}

type scopusAffiliationEntry struct {
	AFID           string `json:"afid"`
	Name           string `json:"name"`
	City           string `json:"city"`
	Country        string `json:"country"`
	AffiliationURL string `json:"affiliation_url"`
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
	metricYearExpr := metricYearForDocumentExpression(s.db)
	base := s.db.Table("scopus_documents AS sd").
		Select("sd.id, sd.title, sd.abstract, sd.aggregation_type, sd.subtype, sd.subtype_description, sd.publication_name, sd.source_id, sd.cover_date, sd.citedby_count, sd.doi, sd.eid, sd.scopus_id, sd.scopus_link, sd.issn, sd.eissn, sd.isbn, sd.volume, sd.issue, sd.page_range, sd.article_number, sd.authkeywords, sd.fund_sponsor, metrics.cite_score_percentile, metrics.cite_score_quartile, metrics.cite_score_status, metrics.cite_score_rank").
		Joins("INNER JOIN (?) AS doc_ids ON doc_ids.doc_id = sd.id", docIDs.Session(&gorm.Session{NewDB: true})).
		Joins("LEFT JOIN scopus_source_metrics AS metrics ON metrics.source_id = sd.source_id AND metrics.doc_type = 'all' AND metrics.metric_year = " + metricYearExpr)

	var rows []scopusPublicationRow
	if err := base.Session(&gorm.Session{}).Order(orderClause).Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, meta, err
	}

	affiliationByDocument, err := s.loadDocumentAffiliationAggregates(collectDocumentIDs(rows))
	if err != nil {
		return nil, 0, meta, err
	}

	return mapScopusRows(rows, affiliationByDocument), total, meta, nil
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

	metricYearExpr := metricYearForDocumentExpression(s.db)
	base := s.db.Table("scopus_documents AS sd").
		Select("sd.id, sd.title, sd.abstract, sd.aggregation_type, sd.subtype, sd.subtype_description, sd.publication_name, sd.source_id, sd.cover_date, sd.citedby_count, sd.doi, sd.eid, sd.scopus_id, sd.scopus_link, sd.issn, sd.eissn, sd.isbn, sd.volume, sd.issue, sd.page_range, sd.article_number, sd.authkeywords, sd.fund_sponsor, metrics.cite_score_percentile, metrics.cite_score_quartile, metrics.cite_score_status, metrics.cite_score_rank").
		Joins("LEFT JOIN scopus_source_metrics AS metrics ON metrics.source_id = sd.source_id AND metrics.doc_type = 'all' AND metrics.metric_year = " + metricYearExpr)

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

	affiliationByDocument, err := s.loadDocumentAffiliationAggregates(collectDocumentIDs(rows))
	if err != nil {
		return nil, 0, err
	}

	return mapScopusRows(rows, affiliationByDocument), total, nil
}

// ListByUserOwnership returns paginated Scopus publications mapped to users in this system.
func (s *ScopusPublicationService) ListByUserOwnership(limit, offset int, sortField, sortDirection, search string) ([]ScopusPublicationByUser, int64, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	pairQuery := s.db.Table("users AS u").
		Select("u.user_id, sd.id AS document_id").
		Joins("INNER JOIN scopus_authors sa ON sa.scopus_author_id = u.Scopus_id").
		Joins("INNER JOIN scopus_document_authors sda ON sda.author_id = sa.id").
		Joins("INNER JOIN scopus_documents sd ON sd.id = sda.document_id").
		Where("u.Scopus_id IS NOT NULL AND TRIM(u.Scopus_id) <> ''")

	if search = strings.TrimSpace(search); search != "" {
		like := fmt.Sprintf("%%%s%%", search)
		pairQuery = pairQuery.Where(
			"u.user_fname LIKE ? OR u.user_lname LIKE ? OR u.email LIKE ? OR u.Scopus_id LIKE ? OR sd.title LIKE ? OR sd.doi LIKE ? OR sd.eid LIKE ? OR sd.publication_name LIKE ?",
			like, like, like, like, like, like, like, like,
		)
	}

	pairQuery = pairQuery.Group("u.user_id, sd.id")

	var total int64
	if err := s.db.Table("(?) AS user_doc_pairs", pairQuery.Session(&gorm.Session{NewDB: true})).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []ScopusPublicationByUser{}, 0, nil
	}

	metricYearExpr := metricYearForDocumentExpression(s.db)
	base := s.db.Table("(?) AS pairs", pairQuery.Session(&gorm.Session{NewDB: true})).
		Select("pairs.user_id, TRIM(CONCAT(COALESCE(u.user_fname,''), ' ', COALESCE(u.user_lname,''))) AS user_name, u.email AS user_email, u.Scopus_id AS user_scopus_id, sd.id AS document_id, sd.title, sd.publication_name, sd.source_id, sd.cover_date, sd.cover_display_date, sd.citedby_count, sd.doi, sd.eid, sd.scopus_id, sd.scopus_link, metrics.cite_score_percentile, metrics.cite_score_quartile, metrics.cite_score_status, metrics.cite_score_rank").
		Joins("INNER JOIN users u ON u.user_id = pairs.user_id").
		Joins("INNER JOIN scopus_documents sd ON sd.id = pairs.document_id").
		Joins("LEFT JOIN scopus_source_metrics AS metrics ON metrics.source_id = sd.source_id AND metrics.doc_type = 'all' AND metrics.metric_year = " + metricYearExpr)

	orderClause := orderForScopusByUser(sortField, sortDirection)
	var rows []scopusPublicationByUserRow
	if err := base.Order(orderClause).Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	affiliationByDocument, err := s.loadDocumentAffiliationAggregates(collectDocumentIDsByUser(rows))
	if err != nil {
		return nil, 0, err
	}

	return mapScopusRowsByUser(rows, affiliationByDocument), total, nil
}

func mapScopusRows(rows []scopusPublicationRow, affiliationByDocument map[uint]scopusAffiliationAggregate) []ScopusPublication {
	publications := make([]ScopusPublication, 0, len(rows))
	for _, row := range rows {
		publication := ScopusPublication{
			ID:                  row.ID,
			Title:               strings.TrimSpace(stringOrEmpty(row.Title)),
			Abstract:            normalizeNullable(row.Abstract),
			AggregationType:     normalizeNullable(row.AggregationType),
			Subtype:             normalizeNullable(row.Subtype),
			SubtypeDescription:  normalizeNullable(row.SubtypeDescription),
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

		if affiliation, ok := affiliationByDocument[row.ID]; ok {
			publication.AffiliationAFID = affiliation.AFID
			publication.AffiliationName = affiliation.Name
			publication.AffiliationCity = affiliation.City
			publication.AffiliationCountry = affiliation.Country
			publication.AffiliationURL = affiliation.URL
			publication.AffiliationsJSON = affiliation.JSON
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

func mapScopusRowsByUser(rows []scopusPublicationByUserRow, affiliationByDocument map[uint]scopusAffiliationAggregate) []ScopusPublicationByUser {
	items := make([]ScopusPublicationByUser, 0, len(rows))
	for _, row := range rows {
		title := strings.TrimSpace(stringOrEmpty(row.Title))
		pub := ScopusPublicationByUser{
			UserID:              row.UserID,
			UserName:            strings.TrimSpace(row.UserName),
			UserEmail:           strings.TrimSpace(stringOrEmpty(row.UserEmail)),
			UserScopusID:        normalizeNullable(row.UserScopusID),
			DocumentID:          row.DocumentID,
			Title:               title,
			PublicationName:     normalizeNullable(row.PublicationName),
			SourceID:            normalizeNullable(row.SourceID),
			CoverDate:           row.CoverDate,
			CitedBy:             row.CitedByCount,
			DOI:                 normalizeNullable(row.DOI),
			EID:                 row.EID,
			ScopusID:            normalizeNullable(row.ScopusID),
			ScopusURL:           normalizeNullable(row.ScopusLink),
			CiteScorePercentile: row.CiteScorePercentile,
			CiteScoreQuartile:   normalizeNullable(row.CiteScoreQuartile),
			CiteScoreStatus:     normalizeNullable(row.CiteScoreStatus),
			CiteScoreRank:       row.CiteScoreRank,
		}

		if affiliation, ok := affiliationByDocument[row.DocumentID]; ok {
			pub.AffiliationAFID = affiliation.AFID
			pub.AffiliationName = affiliation.Name
			pub.AffiliationCity = affiliation.City
			pub.AffiliationCountry = affiliation.Country
			pub.AffiliationURL = affiliation.URL
			pub.AffiliationsJSON = affiliation.JSON
		}

		if row.CoverDate != nil {
			year := row.CoverDate.Year()
			if year > 0 {
				pub.PublicationYear = &year
			}
		} else if row.CoverDisplayDate != nil {
			if year := parseYearFromDisplayDate(*row.CoverDisplayDate); year > 0 {
				pub.PublicationYear = &year
			}
		}

		if pub.ScopusURL == nil {
			if link := buildScopusURL(row.EID); link != nil {
				pub.ScopusURL = link
			}
		}

		items = append(items, pub)
	}

	return items
}

func collectDocumentIDs(rows []scopusPublicationRow) []uint {
	documentIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		documentIDs = append(documentIDs, row.ID)
	}
	return documentIDs
}

func collectDocumentIDsByUser(rows []scopusPublicationByUserRow) []uint {
	documentIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		documentIDs = append(documentIDs, row.DocumentID)
	}
	return documentIDs
}

func (s *ScopusPublicationService) loadDocumentAffiliationAggregates(documentIDs []uint) (map[uint]scopusAffiliationAggregate, error) {
	aggregates := make(map[uint]scopusAffiliationAggregate)
	uniqueDocumentIDs := uniqueUintValues(documentIDs)
	if len(uniqueDocumentIDs) == 0 {
		return aggregates, nil
	}

	var rows []scopusDocumentAffiliationRow
	err := s.db.Table("scopus_document_authors AS sda").
		Select("sda.document_id, sda.affiliation_id, sa.afid, sa.name, sa.city, sa.country, sa.affiliation_url").
		Joins("LEFT JOIN scopus_affiliations AS sa ON sa.id = sda.affiliation_id").
		Where("sda.document_id IN ?", uniqueDocumentIDs).
		Where("sda.affiliation_id IS NOT NULL").
		Order("sda.document_id ASC").
		Order("sda.author_seq ASC").
		Order("sda.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	type aggregateState struct {
		seen      map[string]struct{}
		afids     []string
		names     []string
		cities    []string
		countries []string
		urls      []string
		entries   []scopusAffiliationEntry
	}

	states := make(map[uint]*aggregateState)
	for _, row := range rows {
		afid := strings.TrimSpace(stringValue(row.Afid))
		name := strings.TrimSpace(stringValue(row.Name))
		city := strings.TrimSpace(stringValue(row.City))
		country := strings.TrimSpace(stringValue(row.Country))
		affiliationURL := strings.TrimSpace(stringValue(row.AffiliationURL))
		if afid == "" && name == "" && city == "" && country == "" && affiliationURL == "" {
			continue
		}

		state, ok := states[row.DocumentID]
		if !ok {
			state = &aggregateState{seen: map[string]struct{}{}}
			states[row.DocumentID] = state
		}

		dedupKey := ""
		if row.AffiliationID != nil {
			dedupKey = fmt.Sprintf("id:%d", *row.AffiliationID)
		} else {
			dedupKey = fmt.Sprintf("afid:%s|name:%s|city:%s|country:%s|url:%s", afid, name, city, country, affiliationURL)
		}
		if _, exists := state.seen[dedupKey]; exists {
			continue
		}
		state.seen[dedupKey] = struct{}{}

		state.afids = append(state.afids, afid)
		state.names = append(state.names, name)
		state.cities = append(state.cities, city)
		state.countries = append(state.countries, country)
		state.urls = append(state.urls, affiliationURL)
		state.entries = append(state.entries, scopusAffiliationEntry{
			AFID:           afid,
			Name:           name,
			City:           city,
			Country:        country,
			AffiliationURL: affiliationURL,
		})
	}

	for documentID, state := range states {
		aggregates[documentID] = scopusAffiliationAggregate{
			AFID:    joinNonEmptyValues(state.afids),
			Name:    joinNonEmptyValues(state.names),
			City:    joinNonEmptyValues(state.cities),
			Country: joinNonEmptyValues(state.countries),
			URL:     joinNonEmptyValues(state.urls),
			JSON:    marshalAffiliationsJSON(state.entries),
		}
	}

	return aggregates, nil
}

func uniqueUintValues(values []uint) []uint {
	if len(values) == 0 {
		return []uint{}
	}
	seen := make(map[uint]struct{}, len(values))
	unique := make([]uint, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func joinNonEmptyValues(values []string) *string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	if len(filtered) == 0 {
		return nil
	}
	joined := strings.Join(filtered, " | ")
	return &joined
}

func marshalAffiliationsJSON(entries []scopusAffiliationEntry) *string {
	if len(entries) == 0 {
		return nil
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		return nil
	}
	value := string(payload)
	return &value
}

// metricYearForDocumentExpression returns the metric year to join per document.
// Rules: use the publication year when that year is Complete; if that year is
// In-Progress, fall back to the latest previous Complete year; otherwise NULL.
func metricYearForDocumentExpression(db *gorm.DB) string {
	publicationYearExpr := yearExpression(db)
	inProgressExistsExpr := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM scopus_source_metrics AS ssm_ip WHERE ssm_ip.source_id = sd.source_id AND ssm_ip.doc_type = 'all' AND ssm_ip.metric_year = %s AND LOWER(ssm_ip.cite_score_status) = 'in-progress')",
		publicationYearExpr,
	)
	sameYearCompleteExpr := fmt.Sprintf(
		"(SELECT ssm_complete.metric_year FROM scopus_source_metrics AS ssm_complete WHERE ssm_complete.source_id = sd.source_id AND ssm_complete.doc_type = 'all' AND ssm_complete.metric_year = %s AND LOWER(ssm_complete.cite_score_status) = 'complete' LIMIT 1)",
		publicationYearExpr,
	)
	previousCompleteExpr := fmt.Sprintf(
		"(SELECT MAX(ssm_prev.metric_year) FROM scopus_source_metrics AS ssm_prev WHERE ssm_prev.source_id = sd.source_id AND ssm_prev.doc_type = 'all' AND ssm_prev.metric_year < %s AND LOWER(ssm_prev.cite_score_status) = 'complete')",
		publicationYearExpr,
	)

	return fmt.Sprintf(
		"COALESCE(%s, CASE WHEN %s THEN %s ELSE NULL END)",
		sameYearCompleteExpr,
		inProgressExistsExpr,
		previousCompleteExpr,
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

func orderForScopusByUser(field, direction string) string {
	dir := strings.ToUpper(direction)
	if dir != "ASC" {
		dir = "DESC"
	}

	switch strings.ToLower(strings.TrimSpace(field)) {
	case "user_name":
		return fmt.Sprintf("user_name %s, pairs.user_id ASC, pairs.document_id ASC", dir)
	case "title":
		return fmt.Sprintf("sd.title %s, user_name ASC", dir)
	case "cited_by":
		return fmt.Sprintf("sd.citedby_count %s, user_name ASC", dir)
	default:
		return fmt.Sprintf("sd.cover_date %s, user_name ASC, pairs.user_id ASC", dir)
	}
}

func parseYearFromDisplayDate(value string) int {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 4 {
		return 0
	}
	for i := len(trimmed) - 4; i >= 0; i-- {
		chunk := trimmed[i : i+4]
		year := 0
		for _, ch := range chunk {
			if ch < '0' || ch > '9' {
				year = 0
				break
			}
			year = year*10 + int(ch-'0')
		}
		if year >= 1900 && year <= 3000 {
			return year
		}
	}
	return 0
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
