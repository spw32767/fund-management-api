package controllers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const facultyInsightContractVersion = "faculty-insights-v1"

var insightInternationalStates = []string{"yes", "no", "unknown"}
var insightRoleStates = []string{"first", "corresponding", "coauthor", "unknown"}
var insightCountryKeyPattern = regexp.MustCompile(`^[a-z][a-z '-]{0,95}$`)

// This projection deliberately excludes raw_json, abstracts and any benchmark/ThaiJO data.
type facultyInsightDocument struct {
	ID                     uint                          `json:"document_id"`
	EID                    string                        `json:"eid"`
	ScopusID               *string                       `json:"scopus_id"`
	Title                  *string                       `json:"title"`
	DOI                    *string                       `json:"doi"`
	ScopusLink             *string                       `json:"scopus_link"`
	PublicationName        *string                       `json:"publication_name"`
	AggregationType        *string                       `json:"aggregation_type"`
	YearCE                 *int                          `gorm:"column:year_ce" json:"year_ce"`
	YearBE                 *int                          `gorm:"-" json:"year_be"`
	Citations              int                           `gorm:"column:citations" json:"citations"`
	OpenAccessFlag         int                           `json:"openaccess_flag"`
	OpenAccess             int                           `json:"openaccess"`
	MetricYear             *int                          `json:"metric_year"`
	Quartile               *string                       `json:"quartile"`
	CiteScorePercentile    *float64                      `json:"cite_score_percentile"`
	CiteScoreStatus        *string                       `json:"cite_score_status"`
	AuthorRoleStatus       *string                       `json:"author_role_status"`
	AuthorRoleCheckedAt    *time.Time                    `json:"author_role_checked_at"`
	UpdatedAt              time.Time                     `json:"updated_at"`
	InternationalStatus    string                        `gorm:"-" json:"international_status"`
	FacultyRole            string                        `gorm:"-" json:"faculty_role"`
	EligibleAuthors        []facultyInsightAuthor        `gorm:"-" json:"eligible_authors"`
	Countries              []facultyInsightCountry       `gorm:"-" json:"countries"`
	CountryMetadata        *models.ScopusDocumentInsight `gorm:"-" json:"country_metadata"`
	CountryEvidenceCurrent bool                          `gorm:"-" json:"country_evidence_current"`
}

type facultyInsightAuthor struct {
	DocumentID     uint    `json:"-"`
	LinkID         uint    `json:"link_id"`
	AuthorID       uint    `json:"author_id"`
	ScopusAuthorID string  `json:"scopus_author_id"`
	FullName       *string `json:"full_name"`
	AuthorSeq      int     `json:"author_seq"`
	AffiliationID  uint    `json:"affiliation_id"`
	First          *bool   `gorm:"column:is_first_author" json:"is_first_author"`
	Corresponding  *bool   `gorm:"column:is_corresponding_author" json:"is_corresponding_author"`
}

type facultyInsightCountry struct {
	Key        string `json:"country_key"`
	Name       string `json:"country_name"`
	Provenance string `json:"provenance"`
}

type facultyInsightSnapshot struct {
	Documents []facultyInsightDocument
	Revision  string
}

// Production expressions are the existing dashboard expressions. The SQLite
// equivalent lets isolated fixtures exercise the same filters without a server.
func facultyInsightYearExpr(db *gorm.DB) string {
	if db.Dialector.Name() == "sqlite" {
		return "COALESCE(CAST(strftime('%Y', sd.cover_date) AS INTEGER), CAST(substr(sd.cover_display_date, -4) AS INTEGER))"
	}
	return scopusPublicationYearExpr()
}

func facultyInsightBaseQuery(db *gorm.DB, filters scopusDashboardFilters) *gorm.DB {
	year := facultyInsightYearExpr(db)
	q := db.Table("scopus_documents AS sd").Joins("LEFT JOIN scopus_source_metrics AS metrics ON metrics.source_id = sd.source_id AND metrics.doc_type = 'all' AND metrics.metric_year = " + scopusMetricYearForPublicationExpr(year))
	return applyScopusDashboardFiltersWithYear(q, filters, true, year)
}

func facultyInsightMetadataCurrent(m *models.ScopusDocumentInsight) bool {
	return m != nil && m.NormalizerVersion == services.ScopusInsightNormalizerVersion && (m.Status == "complete" || m.Status == "incomplete")
}

func facultyInsightInternational(m *models.ScopusDocumentInsight) string {
	if !facultyInsightMetadataCurrent(m) || m.International == nil {
		return "unknown"
	}
	if *m.International {
		return "yes"
	}
	if m.CountriesComplete {
		return "no"
	}
	return "unknown"
}

// A current successful check is necessary even for First. Unknown First or
// Corresponding flags block lower roles, but cannot defeat a trustworthy First.
// Conflicting duplicate links for one author are conservatively unknown.
func facultyInsightRole(status *string, authors []facultyInsightAuthor) string {
	if status == nil || (*status != "complete" && *status != "no_correspondence") || len(authors) == 0 {
		return "unknown"
	}
	type flags struct{ first, corresponding *bool }
	byAuthor := map[uint]flags{}
	for _, a := range authors {
		old, exists := byAuthor[a.AuthorID]
		if !exists {
			byAuthor[a.AuthorID] = flags{a.First, a.Corresponding}
			continue
		}
		if old.first == nil || a.First == nil || *old.first != *a.First {
			old.first = nil
		}
		if old.corresponding == nil || a.Corresponding == nil || *old.corresponding != *a.Corresponding {
			old.corresponding = nil
		}
		byAuthor[a.AuthorID] = old
	}
	unknown, corresponding := false, false
	for _, a := range byAuthor {
		if a.first != nil && *a.first {
			return "first"
		}
		if a.first == nil || a.corresponding == nil || (*status == "no_correspondence" && *a.corresponding) {
			unknown = true
		}
		if a.corresponding != nil && *a.corresponding {
			corresponding = true
		}
	}
	if unknown {
		return "unknown"
	}
	if corresponding {
		return "corresponding"
	}
	return "coauthor"
}

// All reads, including the eligibility evidence, occur in one read-only repeatable
// snapshot. Relations are batched to avoid parameter limits and query-per-document.
func loadFacultyInsightSnapshot(db *gorm.DB, filters scopusDashboardFilters) (facultyInsightSnapshot, error) {
	snapshot := facultyInsightSnapshot{Documents: []facultyInsightDocument{}}
	year := facultyInsightYearExpr(db)
	projection := "sd.id, sd.eid, sd.scopus_id, sd.title, sd.doi, sd.scopus_link, sd.publication_name, sd.aggregation_type, " + year + " AS year_ce, COALESCE(sd.citedby_count,0) AS citations, COALESCE(sd.openaccess_flag,0) AS open_access_flag, COALESCE(sd.openaccess,0) AS open_access, metrics.metric_year, metrics.cite_score_quartile AS quartile, metrics.cite_score_percentile, metrics.cite_score_status, sd.author_role_status, sd.author_role_checked_at, sd.updated_at"
	var rows []facultyInsightDocument
	if err := facultyInsightBaseQuery(db, filters).Select(projection).Order("sd.id ASC").Scan(&rows).Error; err != nil {
		return snapshot, err
	}
	// EXISTS avoids user/link fan-out; dedupe defensively against duplicate metric
	// rows too. Conflicting rows are an error instead of an arbitrary classification.
	for _, row := range rows {
		if n := len(snapshot.Documents); n > 0 && snapshot.Documents[n-1].ID == row.ID {
			a, _ := json.Marshal(snapshot.Documents[n-1])
			b, _ := json.Marshal(row)
			if string(a) != string(b) {
				return snapshot, errors.New("ambiguous source metrics")
			}
			continue
		}
		snapshot.Documents = append(snapshot.Documents, row)
	}
	ids := make([]uint, len(snapshot.Documents))
	index := make(map[uint]int, len(ids))
	for i := range snapshot.Documents {
		ids[i] = snapshot.Documents[i].ID
		index[ids[i]] = i
	}
	// Include stored country rows (even stale ones) in the revision. Only current
	// rows are exposed/aggregated; a dirty or obsolete metadata row cannot leak them.
	storedCountries := []models.ScopusDocumentCountry{}
	for start := 0; start < len(ids); start += 400 {
		end := start + 400
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		var metadata []models.ScopusDocumentInsight
		if err := db.Where("document_id IN ?", batch).Order("document_id").Find(&metadata).Error; err != nil {
			return snapshot, err
		}
		for i := range metadata {
			m := metadata[i]
			snapshot.Documents[index[m.DocumentID]].CountryMetadata = &m
		}
		var countries []models.ScopusDocumentCountry
		if err := db.Where("document_id IN ?", batch).Order("document_id, country_key").Find(&countries).Error; err != nil {
			return snapshot, err
		}
		storedCountries = append(storedCountries, countries...)
		for _, country := range countries {
			d := &snapshot.Documents[index[country.DocumentID]]
			if facultyInsightMetadataCurrent(d.CountryMetadata) {
				d.Countries = append(d.Countries, facultyInsightCountry{country.CountryKey, country.CountryName, country.Provenance})
			}
		}
		var authors []facultyInsightAuthor
		q := db.Table("scopus_document_authors AS sda").Select("DISTINCT sda.document_id, sda.id AS link_id, sa.id AS author_id, sa.scopus_author_id, sa.full_name, sda.author_seq, sda.affiliation_id, sda.is_first_author, sda.is_corresponding_author").Joins("JOIN scopus_authors sa ON sa.id = sda.author_id").Joins("JOIN users u ON TRIM(u.scopus_id) = sa.scopus_author_id").Joins("JOIN scopus_affiliations aff ON aff.id = sda.affiliation_id").Where("sda.document_id IN ? AND u.delete_at IS NULL AND u.is_test = 0 AND u.scopus_id IS NOT NULL AND TRIM(u.scopus_id) <> '' AND LOWER(TRIM(COALESCE(aff.name, ''))) IN (?, ?)", batch, scopusAffiliationNameKKU, scopusAffiliationNameSciKKU).Order("sda.document_id, sa.id, sda.id")
		if err := q.Scan(&authors).Error; err != nil {
			return snapshot, err
		}
		for _, a := range authors {
			d := &snapshot.Documents[index[a.DocumentID]]
			d.EligibleAuthors = append(d.EligibleAuthors, a)
		}
	}
	for i := range snapshot.Documents {
		d := &snapshot.Documents[i]
		if d.YearCE != nil && *d.YearCE <= 0 {
			d.YearCE = nil
		}
		if d.YearCE != nil {
			be := *d.YearCE + 543
			d.YearBE = &be
		}
		if d.Countries == nil {
			d.Countries = []facultyInsightCountry{}
		}
		if d.EligibleAuthors == nil {
			d.EligibleAuthors = []facultyInsightAuthor{}
		}
		d.CountryEvidenceCurrent = facultyInsightMetadataCurrent(d.CountryMetadata)
		d.InternationalStatus = facultyInsightInternational(d.CountryMetadata)
		d.FacultyRole = facultyInsightRole(d.AuthorRoleStatus, d.EligibleAuthors)
	}
	filters.Scope = "faculty"
	filters.AggregationTypes = insightSortedUnique(filters.AggregationTypes)
	filters.QualityBuckets = insightSortedUnique(filters.QualityBuckets)
	encoded, err := json.Marshal(struct {
		Version         string
		Filters         scopusDashboardFilters
		Documents       []facultyInsightDocument
		StoredCountries []models.ScopusDocumentCountry
	}{facultyInsightContractVersion, filters, snapshot.Documents, storedCountries})
	if err != nil {
		return snapshot, err
	}
	hash := sha256.Sum256(encoded)
	snapshot.Revision = hex.EncodeToString(hash[:])
	return snapshot, nil
}

func insightSortedUnique(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	result := []string{}
	for _, v := range out {
		if len(result) == 0 || result[len(result)-1] != v {
			result = append(result, v)
		}
	}
	return result
}

type facultyInsightPartner struct {
	CountryKey           string   `json:"country_key"`
	CountryName          string   `json:"country_name"`
	Documents            int      `json:"documents"`
	PercentInternational *float64 `json:"percent_international"`
}

type facultyInsightAggregate struct {
	Total                int                       `json:"total"`
	International        map[string]int            `json:"international"`
	InternationalPercent map[string]*float64       `json:"international_percent"`
	Roles                map[string]int            `json:"roles"`
	RolePercent          map[string]*float64       `json:"role_percent"`
	CountryRole          map[string]map[string]int `json:"country_role"`
	Partners             []facultyInsightPartner   `json:"partners"`
}

func insightPercent(n, denominator int) *float64 {
	if denominator == 0 {
		return nil
	}
	p := 100 * float64(n) / float64(denominator)
	return &p
}

func aggregateFacultyInsights(docs []facultyInsightDocument) facultyInsightAggregate {
	a := facultyInsightAggregate{Total: len(docs), International: map[string]int{}, InternationalPercent: map[string]*float64{}, Roles: map[string]int{}, RolePercent: map[string]*float64{}, CountryRole: map[string]map[string]int{}, Partners: []facultyInsightPartner{}}
	for _, s := range insightInternationalStates {
		a.International[s] = 0
		a.CountryRole[s] = map[string]int{}
		for _, r := range insightRoleStates {
			a.CountryRole[s][r] = 0
		}
	}
	for _, r := range insightRoleStates {
		a.Roles[r] = 0
	}
	partners := map[string]facultyInsightPartner{}
	for _, d := range docs {
		a.International[d.InternationalStatus]++
		a.Roles[d.FacultyRole]++
		a.CountryRole[d.InternationalStatus][d.FacultyRole]++
		if d.InternationalStatus != "yes" {
			continue
		}
		seen := map[string]bool{}
		for _, c := range d.Countries {
			if c.Key == "thailand" || seen[c.Key] {
				continue
			}
			seen[c.Key] = true
			p := partners[c.Key]
			p.CountryKey = c.Key
			p.CountryName = c.Name
			p.Documents++
			partners[c.Key] = p
		}
	}
	for _, s := range insightInternationalStates {
		a.InternationalPercent[s] = insightPercent(a.International[s], a.Total)
	}
	for _, r := range insightRoleStates {
		a.RolePercent[r] = insightPercent(a.Roles[r], a.Total)
	}
	for _, p := range partners {
		p.PercentInternational = insightPercent(p.Documents, a.International["yes"])
		a.Partners = append(a.Partners, p)
	}
	sort.Slice(a.Partners, func(i, j int) bool {
		if a.Partners[i].Documents != a.Partners[j].Documents {
			return a.Partners[i].Documents > a.Partners[j].Documents
		}
		return a.Partners[i].CountryKey < a.Partners[j].CountryKey
	})
	return a
}

type facultyInsightYear struct {
	Bucket string `json:"bucket"`
	YearCE *int   `json:"year_ce"`
	YearBE *int   `json:"year_be"`
	facultyInsightAggregate
}

func summarizeFacultyInsights(snapshot facultyInsightSnapshot) gin.H {
	years := map[int][]facultyInsightDocument{}
	for _, d := range snapshot.Documents {
		year := 0
		if d.YearCE != nil {
			year = *d.YearCE
		}
		years[year] = append(years[year], d)
	}
	keys := []int{}
	for y := range years {
		if y != 0 {
			keys = append(keys, y)
		}
	}
	sort.Ints(keys)
	byYear := []facultyInsightYear{}
	for _, y := range keys {
		ce, be := y, y+543
		byYear = append(byYear, facultyInsightYear{strconv.Itoa(be), &ce, &be, aggregateFacultyInsights(years[y])})
	}
	byYear = append(byYear, facultyInsightYear{"undated", nil, nil, aggregateFacultyInsights(years[0])})
	return gin.H{"success": true, "contract_version": facultyInsightContractVersion, "revision": snapshot.Revision, "source": "scopus_core", "scope": "faculty", "totals": aggregateFacultyInsights(snapshot.Documents), "by_year": byYear}
}

// The transaction is rolled back after reads; no cache or request-time repair.
func withFacultyInsightSnapshot(c *gin.Context, action func(facultyInsightSnapshot)) {
	c.Header("Cache-Control", "no-store")
	if config.DB == nil {
		facultyInsightsUnavailable(c)
		return
	}
	tx := config.DB.WithContext(c.Request.Context()).Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		facultyInsightsUnavailable(c)
		return
	}
	defer tx.Rollback()
	for _, table := range []string{"scopus_document_insights", "scopus_document_countries", "scopus_document_affiliations", "scopus_country_catalogue_guard"} {
		if !tx.Migrator().HasTable(table) {
			facultyInsightsUnavailable(c)
			return
		}
	}
	filters := parseScopusDashboardFilters(c)
	filters.Scope = "faculty"
	snapshot, err := loadFacultyInsightSnapshot(tx, filters)
	if err != nil {
		facultyInsightsUnavailable(c)
		return
	}
	action(snapshot)
}

func facultyInsightsUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "code": "faculty_insights_unavailable", "error": "Faculty insights are unavailable. Verify migration 050 and the database read path."})
}

func AdminGetScopusFacultyInsights(c *gin.Context) {
	withFacultyInsightSnapshot(c, func(snapshot facultyInsightSnapshot) { c.JSON(http.StatusOK, summarizeFacultyInsights(snapshot)) })
}

type facultyInsightDimensions struct {
	Year                                      *int
	Undated                                   bool
	International, Role, CountryKey, Revision string
	Page, PageSize                            int
}

func parseFacultyInsightDimensions(c *gin.Context) (facultyInsightDimensions, error) {
	d := facultyInsightDimensions{Page: 1, PageSize: 50, International: c.Query("international_status"), Role: c.Query("faculty_role"), CountryKey: c.Query("country_key"), Revision: c.Query("revision")}
	if y := c.Query("year_be"); y != "" {
		if y == "undated" {
			d.Undated = true
		} else {
			n, err := strconv.Atoi(y)
			if err != nil || n <= 543 || n > 10542 {
				return d, errors.New("year_be must be a Buddhist year or undated")
			}
			ce := n - 543
			d.Year = &ce
		}
	}
	for _, pair := range []struct {
		value   string
		allowed []string
		label   string
	}{{d.International, insightInternationalStates, "international_status"}, {d.Role, insightRoleStates, "faculty_role"}} {
		if pair.value == "" {
			continue
		}
		valid := false
		for _, v := range pair.allowed {
			if pair.value == v {
				valid = true
			}
		}
		if !valid {
			return d, errors.New("invalid " + pair.label)
		}
	}
	if d.CountryKey != "" && !insightCountryKeyPattern.MatchString(d.CountryKey) {
		return d, errors.New("country_key must be a canonical lowercase country key")
	}
	if d.Revision != "" {
		b, err := hex.DecodeString(d.Revision)
		if err != nil || len(b) != 32 || strings.ToLower(d.Revision) != d.Revision {
			return d, errors.New("revision must be a lowercase SHA-256 hash")
		}
	}
	for _, field := range []struct {
		key    string
		target *int
		max    int
	}{{"page", &d.Page, 1000000}, {"page_size", &d.PageSize, 200}} {
		if raw, ok := c.GetQuery(field.key); ok {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > field.max {
				return d, errors.New(field.key + " is outside its allowed range")
			}
			*field.target = n
		}
	}
	return d, nil
}

func (dim facultyInsightDimensions) matches(d facultyInsightDocument) bool {
	if dim.Undated && d.YearCE != nil {
		return false
	}
	if dim.Year != nil && (d.YearCE == nil || *dim.Year != *d.YearCE) {
		return false
	}
	if dim.International != "" && dim.International != d.InternationalStatus {
		return false
	}
	if dim.Role != "" && dim.Role != d.FacultyRole {
		return false
	}
	if dim.CountryKey != "" {
		for _, c := range d.Countries {
			if c.Key == dim.CountryKey {
				return true
			}
		}
		return false
	}
	return true
}

func AdminGetScopusFacultyInsightsDrilldown(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	dim, err := parseFacultyInsightDimensions(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "invalid_insight_dimension", "error": err.Error()})
		return
	}
	withFacultyInsightSnapshot(c, func(snapshot facultyInsightSnapshot) {
		if dim.Revision != "" && dim.Revision != snapshot.Revision {
			c.JSON(http.StatusConflict, gin.H{"success": false, "code": "insight_revision_mismatch", "revision": snapshot.Revision, "error": "The filtered population changed. Refresh the summary before continuing."})
			return
		}
		matching := []facultyInsightDocument{}
		for _, d := range snapshot.Documents {
			if dim.matches(d) {
				matching = append(matching, d)
			}
		}
		total := len(matching)
		start := (dim.Page - 1) * dim.PageSize
		end := start + dim.PageSize
		if start > total {
			start = total
		}
		if end > total {
			end = total
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "contract_version": facultyInsightContractVersion, "source": "scopus_core", "scope": "faculty", "revision": snapshot.Revision, "total": total, "page": dim.Page, "page_size": dim.PageSize, "total_pages": (total + dim.PageSize - 1) / dim.PageSize, "sort": "document_id_asc", "documents": matching[start:end]})
	})
}
