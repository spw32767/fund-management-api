package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

const (
	benchmarkAffiliationURL = "https://api.elsevier.com/content/search/affiliation"
	benchmarkSubjectDefault = "COMP"
	benchmarkKKUAfID        = "60017165"
	benchmarkKKUScienceAfID = "60280609"
	benchmarkFacetMaxYears  = 30
	benchmarkCountLockName  = "scopus_benchmark_harvest_lock"
)

// ScopusBenchmarkService harvests whole-university and whole-country Scopus
// publications (Computer Science) into isolated scopus_benchmark_* tables so the
// faculty output can be compared without touching scopus_documents.
type ScopusBenchmarkService struct {
	db     *gorm.DB
	client *http.Client
}

// FacultyEmploymentCoverage reports how many active faculty users with a Scopus
// ID can apply the optional employment-date refinement instead of AF-ID fallback.
type FacultyEmploymentCoverage struct {
	Ready                  bool  `json:"ready"`
	EmploymentDateComplete bool  `json:"employment_date_complete"`
	FacultyWithScopusID    int   `json:"faculty_with_scopus_id"`
	EmploymentDateSet      int   `json:"employment_date_set"`
	EmploymentDateMissing  int   `json:"employment_date_missing"`
	BenchmarkYearsMissing  []int `json:"benchmark_years_missing"`
}

// NewScopusBenchmarkService constructs a ScopusBenchmarkService.
func NewScopusBenchmarkService(db *gorm.DB, client *http.Client) *ScopusBenchmarkService {
	if db == nil {
		db = config.DB
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &ScopusBenchmarkService{db: db, client: client}
}

// ---------------------------------------------------------------------------
// Affiliation resolution
// ---------------------------------------------------------------------------

// AffiliationHit is one candidate from the Scopus Affiliation Search API.
type AffiliationHit struct {
	AfID          string `json:"af_id"`
	EID           string `json:"eid"`
	Name          string `json:"name"`
	City          string `json:"city"`
	Country       string `json:"country"`
	DocumentCount int    `json:"document_count"`
	ScopusURL     string `json:"scopus_url"`
}

// ResolveAffiliation searches the Affiliation Search API for candidates matching name.
func (s *ScopusBenchmarkService) ResolveAffiliation(ctx context.Context, name string) ([]AffiliationHit, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("affiliation name is required")
	}
	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return nil, err
	}

	reqURL, _ := url.Parse(benchmarkAffiliationURL)
	q := reqURL.Query()
	q.Set("query", fmt.Sprintf("AFFIL(%s)", name))
	q.Set("count", "25")
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(scopusAPIKeyField, apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("affiliation search error: status %d body %s", resp.StatusCode, truncateBody(body))
	}

	var decoded struct {
		SearchResults struct {
			Entries []struct {
				EID            string          `json:"eid"`
				Identifier     string          `json:"dc:identifier"`
				AffiliationURL string          `json:"prism:url"`
				Name           string          `json:"affiliation-name"`
				City           string          `json:"city"`
				Country        string          `json:"country"`
				DocumentCount  string          `json:"document-count"`
				Error          json.RawMessage `json:"error"`
			} `json:"entry"`
		} `json:"search-results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode affiliation response: %w", err)
	}

	hits := make([]AffiliationHit, 0, len(decoded.SearchResults.Entries))
	for _, e := range decoded.SearchResults.Entries {
		if len(e.Error) > 0 {
			continue
		}
		hits = append(hits, AffiliationHit{
			AfID:          stripScopusPrefix(e.Identifier),
			EID:           e.EID,
			Name:          e.Name,
			City:          e.City,
			Country:       e.Country,
			DocumentCount: parseIntSafe(e.DocumentCount),
			ScopusURL:     e.AffiliationURL,
		})
	}
	return hits, nil
}

// ---------------------------------------------------------------------------
// Query building + counting
// ---------------------------------------------------------------------------

// buildScopeQuery assembles the Scopus advanced-search query for a scope and optional year.
func buildScopeQuery(scope *models.ScopusBenchmarkScope, year *int) (string, error) {
	if scope == nil {
		return "", errors.New("scope is nil")
	}

	var parts []string
	switch strings.ToLower(strings.TrimSpace(scope.Level)) {
	case "university":
		if scope.AfID == nil || strings.TrimSpace(*scope.AfID) == "" {
			return "", fmt.Errorf("scope %q has no af_id (resolve the affiliation first)", scope.Code)
		}
		parts = append(parts, fmt.Sprintf("AF-ID(%s)", strings.TrimSpace(*scope.AfID)))
	case "country":
		if scope.AffilCountry == nil || strings.TrimSpace(*scope.AffilCountry) == "" {
			return "", fmt.Errorf("scope %q has no affil_country", scope.Code)
		}
		parts = append(parts, fmt.Sprintf("AFFILCOUNTRY(%s)", strings.TrimSpace(*scope.AffilCountry)))
	default:
		// custom / unknown level: the extra_query IS the base constraint
		if scope.ExtraQuery == nil || strings.TrimSpace(*scope.ExtraQuery) == "" {
			return "", fmt.Errorf("scope %q has unsupported level %q and no extra_query", scope.Code, scope.Level)
		}
		parts = append(parts, "("+strings.TrimSpace(*scope.ExtraQuery)+")")
	}

	subject := strings.TrimSpace(scope.SubjectArea)
	if subject == "" {
		subject = benchmarkSubjectDefault
	}
	parts = append(parts, fmt.Sprintf("SUBJAREA(%s)", subject))

	if year != nil {
		parts = append(parts, fmt.Sprintf("PUBYEAR = %d", *year))
	}

	// university/country may carry an OPTIONAL extra filter on top of their base;
	// custom/unknown levels already used extra_query as their base above.
	lvl := strings.ToLower(strings.TrimSpace(scope.Level))
	if (lvl == "university" || lvl == "country") &&
		scope.ExtraQuery != nil && strings.TrimSpace(*scope.ExtraQuery) != "" {
		parts = append(parts, fmt.Sprintf("(%s)", strings.TrimSpace(*scope.ExtraQuery)))
	}

	return strings.Join(parts, " AND "), nil
}

// buildFacultyQuery builds the "faculty" query: CS papers affiliated with KKU that
// were authored by any of our registered faculty (users.scopus_id). This works with
// a STANDARD-only key (count=1) — it does not need the COMPLETE view.
func (s *ScopusBenchmarkService) buildFacultyQuery(ctx context.Context, scope *models.ScopusBenchmarkScope, year *int) (string, error) {
	var uni models.ScopusBenchmarkScope
	if err := s.db.WithContext(ctx).Where("level = ?", "university").First(&uni).Error; err != nil {
		return "", fmt.Errorf("faculty query needs the university scope: %w", err)
	}
	if uni.AfID == nil || strings.TrimSpace(*uni.AfID) == "" {
		return "", errors.New("set the KKU AF-ID first (step 1)")
	}

	ids, err := s.facultyAuthorIDs(ctx)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", errors.New("no faculty scopus ids in the system")
	}
	au := make([]string, 0, len(ids))
	for _, id := range ids {
		au = append(au, "AU-ID("+id+")")
	}

	subject := strings.TrimSpace(scope.SubjectArea)
	if subject == "" {
		subject = benchmarkSubjectDefault
	}

	parts := []string{
		fmt.Sprintf("AF-ID(%s)", strings.TrimSpace(*uni.AfID)),
		fmt.Sprintf("SUBJAREA(%s)", subject),
	}
	if year != nil {
		parts = append(parts, fmt.Sprintf("PUBYEAR = %d", *year))
	}
	parts = append(parts, "("+strings.Join(au, " OR ")+")")
	return strings.Join(parts, " AND "), nil
}

// facultyAuthorIDs returns the de-duplicated, normalized Scopus author ids of users
// registered in our system.
func (s *ScopusBenchmarkService) facultyAuthorIDs(ctx context.Context) ([]string, error) {
	var raw []string
	if err := s.db.WithContext(ctx).Table("users").
		Where("scopus_id IS NOT NULL AND scopus_id <> ''").
		Pluck("scopus_id", &raw).Error; err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(raw))
	ids := make([]string, 0, len(raw))
	for _, id := range raw {
		norm := normalizeScopusID(id)
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		ids = append(ids, norm)
	}
	return ids, nil
}

// CountScope returns the total for a scope (optionally a single year) and records
// a snapshot. University/country scopes use Scopus Search; the faculty scope is
// derived locally with the stricter verified-faculty rule.
func (s *ScopusBenchmarkService) CountScope(ctx context.Context, scope *models.ScopusBenchmarkScope, year *int) (int, error) {
	if scope == nil {
		return 0, errors.New("scope is nil")
	}
	if strings.EqualFold(strings.TrimSpace(scope.Level), "faculty") {
		return s.countVerifiedFacultyScope(ctx, scope, year)
	}

	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return 0, err
	}
	query, err := buildScopeQuery(scope, year)
	if err != nil {
		return 0, err
	}

	total, _, err := s.searchPage(ctx, apiKey, query, 0, 1, "STANDARD")
	if err != nil {
		return 0, err
	}

	snapshot := &models.ScopusBenchmarkCountSnapshot{
		ScopeID:      scope.ID,
		SubjectArea:  strings.TrimSpace(scope.SubjectArea),
		PubYear:      year,
		TotalResults: total,
		CapturedAt:   time.Now(),
	}
	if snapshot.SubjectArea == "" {
		snapshot.SubjectArea = benchmarkSubjectDefault
	}
	if err := s.db.WithContext(ctx).Create(snapshot).Error; err != nil {
		return total, err
	}
	return total, nil
}

// CountScopeRange counts and stores every year in one selected range. Scopus
// scopes use PUBYEAR facets (one request per at-most-30-year chunk); faculty is
// grouped locally. No snapshot is written unless every chunk validates.
func (s *ScopusBenchmarkService) CountScopeRange(ctx context.Context, scope *models.ScopusBenchmarkScope, yearFrom, yearTo int) (int, map[int]int, error) {
	if scope == nil {
		return 0, nil, errors.New("scope is nil")
	}
	if yearFrom <= 0 || yearTo <= 0 || yearFrom > yearTo {
		return 0, nil, fmt.Errorf("invalid year range %d-%d", yearFrom, yearTo)
	}

	var (
		byYear map[int]int
		err    error
	)
	if strings.EqualFold(strings.TrimSpace(scope.Level), "faculty") {
		byYear, err = s.countVerifiedFacultyRange(ctx, yearFrom, yearTo)
	} else {
		byYear, err = s.countScopusScopeRange(ctx, scope, yearFrom, yearTo)
	}
	if err != nil {
		return 0, nil, err
	}

	total := 0
	snapshots := make([]models.ScopusBenchmarkCountSnapshot, 0, yearTo-yearFrom+1)
	capturedAt := time.Now()
	subject := strings.TrimSpace(scope.SubjectArea)
	if subject == "" {
		subject = benchmarkSubjectDefault
	}
	for year := yearFrom; year <= yearTo; year++ {
		count := byYear[year]
		total += count
		y := year
		snapshots = append(snapshots, models.ScopusBenchmarkCountSnapshot{
			ScopeID:      scope.ID,
			SubjectArea:  subject,
			PubYear:      &y,
			TotalResults: count,
			CapturedAt:   capturedAt,
		})
	}
	if err := s.db.WithContext(ctx).Create(&snapshots).Error; err != nil {
		return 0, nil, fmt.Errorf("store benchmark range snapshots: %w", err)
	}
	return total, byYear, nil
}

func (s *ScopusBenchmarkService) countScopusScopeRange(ctx context.Context, scope *models.ScopusBenchmarkScope, yearFrom, yearTo int) (map[int]int, error) {
	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return nil, err
	}
	baseQuery, err := buildScopeQuery(scope, nil)
	if err != nil {
		return nil, err
	}

	byYear := make(map[int]int, yearTo-yearFrom+1)
	for _, years := range benchmarkYearRanges(yearFrom, yearTo) {
		chunkFrom, chunkTo := years[0], years[1]
		query := fmt.Sprintf("(%s) AND PUBYEAR > %d AND PUBYEAR < %d", baseQuery, chunkFrom-1, chunkTo+1)
		chunkCounts, err := s.searchPubYearFacet(ctx, apiKey, query, chunkFrom, chunkTo)
		if err != nil {
			if errors.Is(err, errScopusFacetNotEntitled) {
				// Facet access needs KKU-network entitlement; fall back to the
				// per-year count path, which works on a STANDARD key anywhere.
				return s.countScopusScopePerYear(ctx, apiKey, scope, yearFrom, yearTo)
			}
			return nil, err
		}
		for year, count := range chunkCounts {
			byYear[year] = count
		}
	}
	return byYear, nil
}

// countScopusScopePerYear counts one Scopus request per year without writing
// snapshots. It is the fallback used when facet access is unavailable (e.g. off
// the KKU network), preserving the pre-optimization behaviour.
func (s *ScopusBenchmarkService) countScopusScopePerYear(ctx context.Context, apiKey string, scope *models.ScopusBenchmarkScope, yearFrom, yearTo int) (map[int]int, error) {
	byYear := make(map[int]int, yearTo-yearFrom+1)
	for year := yearFrom; year <= yearTo; year++ {
		y := year
		query, err := buildScopeQuery(scope, &y)
		if err != nil {
			return nil, err
		}
		total, _, err := s.searchPage(ctx, apiKey, query, 0, 1, "STANDARD")
		if err != nil {
			return nil, err
		}
		byYear[year] = total
	}
	return byYear, nil
}

// isFacetNotEntitled reports whether a non-200 facet response indicates the key
// or network lacks facet entitlement (seen as HTTP 400/403 off the KKU network).
func isFacetNotEntitled(status int, body []byte) bool {
	if status != http.StatusBadRequest && status != http.StatusForbidden {
		return false
	}
	lower := strings.ToLower(string(body))
	if !strings.Contains(lower, "facet") {
		return false
	}
	return strings.Contains(lower, "entitl") ||
		strings.Contains(lower, "not allowed") ||
		strings.Contains(lower, "authoriz")
}

func benchmarkYearRanges(yearFrom, yearTo int) [][2]int {
	ranges := make([][2]int, 0, (yearTo-yearFrom)/benchmarkFacetMaxYears+1)
	for chunkFrom := yearFrom; chunkFrom <= yearTo; chunkFrom += benchmarkFacetMaxYears {
		chunkTo := chunkFrom + benchmarkFacetMaxYears - 1
		if chunkTo > yearTo {
			chunkTo = yearTo
		}
		ranges = append(ranges, [2]int{chunkFrom, chunkTo})
	}
	return ranges
}

func (s *ScopusBenchmarkService) searchPubYearFacet(ctx context.Context, apiKey, query string, yearFrom, yearTo int) (map[int]int, error) {
	reqURL, err := url.Parse(scopusBaseURL)
	if err != nil {
		return nil, err
	}
	q := reqURL.Query()
	q.Set("query", query)
	q.Set("count", "1")
	q.Set("view", "STANDARD")
	q.Set("facets", fmt.Sprintf("pubyear(count=%d)", yearTo-yearFrom+1))
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(scopusAPIKeyField, apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errScopusRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		if isFacetNotEntitled(resp.StatusCode, body) {
			return nil, errScopusFacetNotEntitled
		}
		return nil, fmt.Errorf("scopus pubyear facet error: status %d body %s", resp.StatusCode, truncateBody(body))
	}
	return decodePubYearFacet(body, yearFrom, yearTo)
}

func decodePubYearFacet(body []byte, yearFrom, yearTo int) (map[int]int, error) {
	type category struct {
		Value    string `json:"value"`
		Name     string `json:"name"`
		HitCount string `json:"hitCount"`
	}
	type facet struct {
		Name       string     `json:"name"`
		Categories []category `json:"category"`
	}
	var decoded struct {
		SearchResults struct {
			Total string          `json:"opensearch:totalResults"`
			Facet json.RawMessage `json:"facet"`
		} `json:"search-results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode scopus pubyear facet response: %w", err)
	}
	expectedTotal := parseIntSafe(decoded.SearchResults.Total)
	if expectedTotal == 0 {
		rawFacet := strings.TrimSpace(string(decoded.SearchResults.Facet))
		if rawFacet == "" || rawFacet == "null" {
			return map[int]int{}, nil
		}
	}

	var facets []facet
	if len(decoded.SearchResults.Facet) > 0 && decoded.SearchResults.Facet[0] == '[' {
		if err := json.Unmarshal(decoded.SearchResults.Facet, &facets); err != nil {
			return nil, fmt.Errorf("decode scopus pubyear facets: %w", err)
		}
	} else {
		var single facet
		if err := json.Unmarshal(decoded.SearchResults.Facet, &single); err != nil {
			return nil, fmt.Errorf("decode scopus pubyear facet: %w", err)
		}
		facets = []facet{single}
	}

	counts := make(map[int]int, yearTo-yearFrom+1)
	actualTotal := 0
	found := false
	for _, f := range facets {
		if !strings.EqualFold(strings.TrimSpace(f.Name), "pubyear") {
			continue
		}
		found = true
		for _, item := range f.Categories {
			year := parseIntSafe(item.Value)
			if year == 0 {
				year = parseIntSafe(item.Name)
			}
			if year < yearFrom || year > yearTo {
				continue
			}
			count := parseIntSafe(item.HitCount)
			counts[year] = count
			actualTotal += count
		}
	}
	if !found {
		return nil, errors.New("scopus response has no pubyear facet")
	}
	if actualTotal != expectedTotal {
		return nil, fmt.Errorf("scopus pubyear facet total mismatch: buckets=%d totalResults=%d", actualTotal, expectedTotal)
	}
	return counts, nil
}

// countVerifiedFacultyScope derives the official faculty benchmark from existing
// data only. The KKU benchmark membership supplies the SUBJAREA(COMP) document
// set, while the dashboard tables verify that the matched faculty author had KKU
// AF-ID 60017165 on that document and, when the employment date is available,
// published it on/after that date. AF-ID is the observable fallback while the
// personnel date is unavailable.
// It intentionally does not alter the dashboard query or any core Scopus data.
func (s *ScopusBenchmarkService) countVerifiedFacultyScope(ctx context.Context, scope *models.ScopusBenchmarkScope, year *int) (int, error) {
	if scope == nil {
		return 0, errors.New("scope is nil")
	}
	coverage, err := s.FacultyEmploymentCoverage(ctx)
	if err != nil {
		return 0, err
	}
	if !coverage.Ready {
		return 0, errors.New("verified faculty count unavailable: no active faculty users with Scopus ID")
	}

	var universityScope models.ScopusBenchmarkScope
	if err := s.db.WithContext(ctx).
		Where("code = ? AND level = ?", "university_kku", "university").
		First(&universityScope).Error; err != nil {
		return 0, fmt.Errorf("verified faculty count needs the KKU benchmark scope: %w", err)
	}

	query, args := verifiedFacultyCountQuery(universityScope.ID, year)
	var total int
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("count verified faculty benchmark: %w", err)
	}

	subject := strings.TrimSpace(scope.SubjectArea)
	if subject == "" {
		subject = benchmarkSubjectDefault
	}
	snapshot := &models.ScopusBenchmarkCountSnapshot{
		ScopeID:      scope.ID,
		SubjectArea:  subject,
		PubYear:      year,
		TotalResults: total,
		CapturedAt:   time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(snapshot).Error; err != nil {
		return total, err
	}
	return total, nil
}

// FacultyEmploymentCoverage reports how much of the metric can also apply the
// employment-date check rather than relying on document-level KKU AF-ID alone.
// It does not modify users or Scopus data.
func (s *ScopusBenchmarkService) FacultyEmploymentCoverage(ctx context.Context) (FacultyEmploymentCoverage, error) {
	var coverage FacultyEmploymentCoverage
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) AS faculty_with_scopus_id,
			COALESCE(SUM(date_of_employment IS NOT NULL), 0) AS employment_date_set,
			COALESCE(SUM(date_of_employment IS NULL), 0) AS employment_date_missing
		FROM users
		WHERE delete_at IS NULL
		  AND scopus_id IS NOT NULL
		  AND TRIM(scopus_id) <> ''`).Scan(&coverage).Error; err != nil {
		return coverage, fmt.Errorf("check faculty employment-date coverage: %w", err)
	}
	coverage.Ready = coverage.FacultyWithScopusID > 0
	coverage.EmploymentDateComplete = coverage.Ready && coverage.EmploymentDateMissing == 0
	return coverage, nil
}

func (s *ScopusBenchmarkService) countVerifiedFacultyRange(ctx context.Context, yearFrom, yearTo int) (map[int]int, error) {
	coverage, err := s.FacultyMetricCoverage(ctx, yearFrom, yearTo)
	if err != nil {
		return nil, err
	}
	if !coverage.Ready {
		return nil, errors.New("verified faculty count unavailable: no active faculty users with Scopus ID")
	}
	if len(coverage.BenchmarkYearsMissing) > 0 {
		return nil, fmt.Errorf(
			"verified faculty count unavailable: KKU benchmark documents are incomplete for years %v; harvest KKU documents for this range first",
			coverage.BenchmarkYearsMissing,
		)
	}

	var universityScope models.ScopusBenchmarkScope
	if err := s.db.WithContext(ctx).
		Where("code = ? AND level = ?", "university_kku", "university").
		First(&universityScope).Error; err != nil {
		return nil, fmt.Errorf("verified faculty count needs the KKU benchmark scope: %w", err)
	}

	type row struct {
		PubYear int `gorm:"column:pub_year"`
		Total   int `gorm:"column:total"`
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(`
		SELECT bds.pub_year AS pub_year, COUNT(DISTINCT bd.eid) AS total
		FROM scopus_benchmark_documents AS bd
		JOIN scopus_benchmark_document_scopes AS bds
		  ON bds.document_id = bd.id
		JOIN scopus_documents AS sd
		  ON sd.eid = bd.eid
		WHERE bds.scope_id = ?
		  AND bds.pub_year BETWEEN ? AND ?
		  AND EXISTS (
			SELECT 1
			FROM scopus_document_authors AS sda
			JOIN scopus_authors AS sa ON sa.id = sda.author_id
			JOIN users AS u ON TRIM(u.scopus_id) = sa.scopus_author_id
			JOIN scopus_affiliations AS aff ON aff.id = sda.affiliation_id
			WHERE sda.document_id = sd.id
			  AND u.delete_at IS NULL
			  AND u.scopus_id IS NOT NULL
			  AND TRIM(u.scopus_id) <> ''
			  AND (
				u.date_of_employment IS NULL
				OR (sd.cover_date IS NOT NULL AND DATE(sd.cover_date) >= DATE(u.date_of_employment))
			  )
			  AND TRIM(aff.afid) IN (?, ?)
		  )
		GROUP BY bds.pub_year`, universityScope.ID, yearFrom, yearTo, benchmarkKKUAfID, benchmarkKKUScienceAfID).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("count verified faculty benchmark range: %w", err)
	}

	byYear := make(map[int]int, yearTo-yearFrom+1)
	for _, item := range rows {
		byYear[item.PubYear] = item.Total
	}
	return byYear, nil
}

// FacultyMetricCoverage combines personnel-date coverage with a per-year check
// that the harvested KKU/COMP EID set exactly matches the latest KKU count.
func (s *ScopusBenchmarkService) FacultyMetricCoverage(ctx context.Context, yearFrom, yearTo int) (FacultyEmploymentCoverage, error) {
	coverage, err := s.FacultyEmploymentCoverage(ctx)
	if err != nil {
		return coverage, err
	}
	missing, err := s.facultyBenchmarkMissingYears(ctx, yearFrom, yearTo)
	if err != nil {
		return coverage, err
	}
	coverage.BenchmarkYearsMissing = missing
	return coverage, nil
}

func (s *ScopusBenchmarkService) facultyBenchmarkMissingYears(ctx context.Context, yearFrom, yearTo int) ([]int, error) {
	var universityScope models.ScopusBenchmarkScope
	if err := s.db.WithContext(ctx).
		Where("code = ? AND level = ?", "university_kku", "university").
		First(&universityScope).Error; err != nil {
		return nil, fmt.Errorf("faculty coverage needs the KKU benchmark scope: %w", err)
	}

	// While a KKU harvest is writing benchmark membership, the per-year
	// cardinality check below is unreliable: the harvested count can transiently
	// equal the snapshot mid-run. Treat every requested year as not-yet-covered
	// until the harvest finishes so faculty numbers are never refreshed or shown
	// from a partially-written membership set. A Thailand-scope harvest does not
	// touch KKU membership, so it does not block the faculty metric.
	activeRun, err := s.GetActiveRun(ctx)
	if err != nil {
		return nil, err
	}
	if activeRun != nil && (activeRun.ScopeID == nil || *activeRun.ScopeID == universityScope.ID) {
		blocked := make([]int, 0, yearTo-yearFrom+1)
		for year := yearFrom; year <= yearTo; year++ {
			blocked = append(blocked, year)
		}
		return blocked, nil
	}

	type yearCount struct {
		PubYear int `gorm:"column:pub_year"`
		Total   int `gorm:"column:total"`
	}
	var snapshotRows []yearCount
	if err := s.db.WithContext(ctx).Raw(`
		SELECT s.pub_year AS pub_year, s.total_results AS total
		FROM scopus_benchmark_count_snapshots AS s
		JOIN (
			SELECT pub_year, MAX(id) AS max_id
			FROM scopus_benchmark_count_snapshots
			WHERE scope_id = ? AND pub_year BETWEEN ? AND ?
			GROUP BY pub_year
		) AS latest ON latest.max_id = s.id
		WHERE s.scope_id = ?`, universityScope.ID, yearFrom, yearTo, universityScope.ID).
		Scan(&snapshotRows).Error; err != nil {
		return nil, fmt.Errorf("load KKU benchmark snapshot coverage: %w", err)
	}

	var harvestedRows []yearCount
	if err := s.db.WithContext(ctx).Raw(`
		SELECT bds.pub_year AS pub_year, COUNT(DISTINCT bds.document_id) AS total
		FROM scopus_benchmark_document_scopes AS bds
		WHERE bds.scope_id = ? AND bds.pub_year BETWEEN ? AND ?
		GROUP BY bds.pub_year`, universityScope.ID, yearFrom, yearTo).
		Scan(&harvestedRows).Error; err != nil {
		return nil, fmt.Errorf("load harvested KKU benchmark coverage: %w", err)
	}

	expected := make(map[int]int, len(snapshotRows))
	for _, item := range snapshotRows {
		expected[item.PubYear] = item.Total
	}
	harvested := make(map[int]int, len(harvestedRows))
	for _, item := range harvestedRows {
		harvested[item.PubYear] = item.Total
	}
	return missingBenchmarkYears(yearFrom, yearTo, expected, harvested), nil
}

func missingBenchmarkYears(yearFrom, yearTo int, expected, harvested map[int]int) []int {
	missing := make([]int, 0)
	for year := yearFrom; year <= yearTo; year++ {
		expectedCount, hasSnapshot := expected[year]
		if !hasSnapshot || harvested[year] != expectedCount {
			missing = append(missing, year)
		}
	}
	return missing
}

func verifiedFacultyCountQuery(universityScopeID uint64, year *int) (string, []interface{}) {
	query := `
		SELECT COUNT(DISTINCT bd.eid)
		FROM scopus_benchmark_documents AS bd
		JOIN scopus_benchmark_document_scopes AS bds
		  ON bds.document_id = bd.id
		JOIN scopus_documents AS sd
		  ON sd.eid = bd.eid
		WHERE bds.scope_id = ?
		  AND EXISTS (
			SELECT 1
			FROM scopus_document_authors AS sda
			JOIN scopus_authors AS sa ON sa.id = sda.author_id
			JOIN users AS u ON TRIM(u.scopus_id) = sa.scopus_author_id
			JOIN scopus_affiliations AS aff ON aff.id = sda.affiliation_id
			WHERE sda.document_id = sd.id
			  AND u.delete_at IS NULL
			  AND u.scopus_id IS NOT NULL
			  AND TRIM(u.scopus_id) <> ''
			  AND (
				u.date_of_employment IS NULL
				OR (sd.cover_date IS NOT NULL AND DATE(sd.cover_date) >= DATE(u.date_of_employment))
			  )
			  AND TRIM(aff.afid) IN (?, ?)
		  )`
	args := []interface{}{universityScopeID, benchmarkKKUAfID, benchmarkKKUScienceAfID}
	if year != nil {
		query += "\n\t\t  AND bds.pub_year = ?"
		args = append(args, *year)
	}
	return query, args
}

// ---------------------------------------------------------------------------
// Search page fetch
// ---------------------------------------------------------------------------

// searchPage performs one Scopus Search request using offset (start) pagination.
// Used by CountScope (count=1, start=0) — no pagination needed there, so it works
// even off-VPN where the cursor entitlement is unavailable. Note: the Scopus API
// caps offset at 5000; harvests use searchPageCursor instead to avoid that cap.
func (s *ScopusBenchmarkService) searchPage(ctx context.Context, apiKey, query string, start, count int, view string) (int, []json.RawMessage, error) {
	reqURL, err := url.Parse(scopusBaseURL)
	if err != nil {
		return 0, nil, err
	}
	q := reqURL.Query()
	q.Set("query", query)
	q.Set("count", strconv.Itoa(count))
	q.Set("start", strconv.Itoa(start))
	q.Set("view", view)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(scopusAPIKeyField, apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		return 0, nil, errScopusRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("scopus search error: status %d body %s", resp.StatusCode, truncateBody(body))
	}

	var decoded struct {
		SearchResults struct {
			TotalResults string            `json:"opensearch:totalResults"`
			Entries      []json.RawMessage `json:"entry"`
		} `json:"search-results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return 0, nil, fmt.Errorf("decode scopus search response: %w", err)
	}

	total := parseIntSafe(decoded.SearchResults.TotalResults)
	return total, decoded.SearchResults.Entries, nil
}

// searchPageCursor performs one Scopus Search request using cursor pagination.
// Pass cursor="*" for the first page, then the returned @next value to continue.
// Cursor removes the 5000-result offset cap but requires the institutional
// entitlement (i.e. the request must originate from the KKU network / VPN).
// Used by the harvest (view=COMPLETE), which needs VPN anyway.
func (s *ScopusBenchmarkService) searchPageCursor(ctx context.Context, apiKey, query, cursor string, count int, view string) (int, []json.RawMessage, string, error) {
	reqURL, err := url.Parse(scopusBaseURL)
	if err != nil {
		return 0, nil, "", err
	}
	if cursor == "" {
		cursor = "*"
	}
	q := reqURL.Query()
	q.Set("query", query)
	q.Set("count", strconv.Itoa(count))
	q.Set("view", view)
	q.Set("cursor", cursor)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(scopusAPIKeyField, apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		return 0, nil, "", errScopusRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return 0, nil, "", fmt.Errorf("scopus search error: status %d body %s", resp.StatusCode, truncateBody(body))
	}

	var decoded struct {
		SearchResults struct {
			TotalResults string `json:"opensearch:totalResults"`
			Cursor       struct {
				Next string `json:"@next"`
			} `json:"cursor"`
			Entries []json.RawMessage `json:"entry"`
		} `json:"search-results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return 0, nil, "", fmt.Errorf("decode scopus search response: %w", err)
	}

	total := parseIntSafe(decoded.SearchResults.TotalResults)
	return total, decoded.SearchResults.Entries, decoded.SearchResults.Cursor.Next, nil
}

var errScopusRateLimited = errors.New("scopus api rate limited (429)")

// errScopusFacetNotEntitled means the API key/network is not entitled to use
// Scopus search facets (seen off the KKU network as HTTP 400/403). The caller
// falls back to the per-year count path, which needs no facet entitlement.
var errScopusFacetNotEntitled = errors.New("scopus api not entitled to access facets")

// DetectYearRange returns the earliest and latest publication year available for a
// scope, using coverDate sorting (two lightweight count=1 requests).
func (s *ScopusBenchmarkService) DetectYearRange(ctx context.Context, scope *models.ScopusBenchmarkScope) (int, int, error) {
	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return 0, 0, err
	}
	query, err := buildScopeQuery(scope, nil)
	if err != nil {
		return 0, 0, err
	}

	first, err := s.fetchExtremeYear(ctx, apiKey, query, "+coverDate")
	if err != nil {
		return 0, 0, err
	}
	last, err := s.fetchExtremeYear(ctx, apiKey, query, "-coverDate")
	if err != nil {
		return 0, 0, err
	}
	if last == 0 {
		last = time.Now().Year()
	}
	return first, last, nil
}

// fetchExtremeYear fetches the single document at the head of a sort order and
// returns its publication year. sort="coverDate" gives the oldest, "-coverDate"
// the newest.
func (s *ScopusBenchmarkService) fetchExtremeYear(ctx context.Context, apiKey, query, sort string) (int, error) {
	reqURL, err := url.Parse(scopusBaseURL)
	if err != nil {
		return 0, err
	}
	q := reqURL.Query()
	q.Set("query", query)
	q.Set("count", "1")
	q.Set("view", "STANDARD")
	q.Set("sort", sort)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(scopusAPIKeyField, apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("scopus sort search error: status %d body %s", resp.StatusCode, truncateBody(body))
	}

	var decoded struct {
		SearchResults struct {
			Entries []json.RawMessage `json:"entry"`
		} `json:"search-results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return 0, err
	}
	if len(decoded.SearchResults.Entries) == 0 {
		return 0, nil
	}
	entry, err := parseScopusEntry(decoded.SearchResults.Entries[0])
	if err != nil {
		return 0, err
	}
	if d := parseScopusDate(entry.CoverDate); d != nil {
		return d.Year(), nil
	}
	return 0, nil
}

func stripScopusPrefix(v string) string {
	v = strings.TrimSpace(v)
	if idx := strings.LastIndex(v, ":"); idx >= 0 {
		return strings.TrimSpace(v[idx+1:])
	}
	return v
}
