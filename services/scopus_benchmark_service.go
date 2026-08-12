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
	benchmarkCountLockName  = "scopus_benchmark_harvest_lock"
)

// ScopusBenchmarkService harvests whole-university and whole-country Scopus
// publications (Computer Science) into isolated scopus_benchmark_* tables so the
// faculty output can be compared without touching scopus_documents.
type ScopusBenchmarkService struct {
	db     *gorm.DB
	client *http.Client
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

// CountScope returns the Scopus totalResults for a scope (optionally a single year)
// using a lightweight count=1 search, and records a count snapshot.
func (s *ScopusBenchmarkService) CountScope(ctx context.Context, scope *models.ScopusBenchmarkScope, year *int) (int, error) {
	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return 0, err
	}
	var query string
	if strings.EqualFold(strings.TrimSpace(scope.Level), "faculty") {
		query, err = s.buildFacultyQuery(ctx, scope, year)
	} else {
		query, err = buildScopeQuery(scope, year)
	}
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
