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
		if scope.ExtraQuery == nil || strings.TrimSpace(*scope.ExtraQuery) == "" {
			return "", fmt.Errorf("scope %q has unsupported level %q and no extra_query", scope.Code, scope.Level)
		}
	}

	subject := strings.TrimSpace(scope.SubjectArea)
	if subject == "" {
		subject = benchmarkSubjectDefault
	}
	parts = append(parts, fmt.Sprintf("SUBJAREA(%s)", subject))

	if year != nil {
		parts = append(parts, fmt.Sprintf("PUBYEAR = %d", *year))
	}

	if scope.ExtraQuery != nil && strings.TrimSpace(*scope.ExtraQuery) != "" &&
		!strings.EqualFold(strings.TrimSpace(scope.Level), "custom") {
		parts = append(parts, fmt.Sprintf("(%s)", strings.TrimSpace(*scope.ExtraQuery)))
	}

	return strings.Join(parts, " AND "), nil
}

// CountScope returns the Scopus totalResults for a scope (optionally a single year)
// using a lightweight count=1 search, and records a count snapshot.
func (s *ScopusBenchmarkService) CountScope(ctx context.Context, scope *models.ScopusBenchmarkScope, year *int) (int, error) {
	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return 0, err
	}
	query, err := buildScopeQuery(scope, year)
	if err != nil {
		return 0, err
	}

	total, _, _, err := s.searchPage(ctx, apiKey, query, "", 1, "STANDARD")
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
// Search page fetch (cursor-based, view=COMPLETE for harvest)
// ---------------------------------------------------------------------------

// searchPage performs one Scopus Search request. Pass cursor="" for the first
// page with a "*" cursor, or a previous @next value to continue. Returns total
// results, the entries, and the next cursor ("" when exhausted).
func (s *ScopusBenchmarkService) searchPage(ctx context.Context, apiKey, query, cursor string, count int, view string) (int, []json.RawMessage, string, error) {
	reqURL, err := url.Parse(scopusBaseURL)
	if err != nil {
		return 0, nil, "", err
	}
	q := reqURL.Query()
	q.Set("query", query)
	q.Set("count", strconv.Itoa(count))
	q.Set("view", view)
	if cursor != "" {
		q.Set("cursor", cursor)
	} else {
		q.Set("cursor", "*")
	}
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

func stripScopusPrefix(v string) string {
	v = strings.TrimSpace(v)
	if idx := strings.LastIndex(v, ":"); idx >= 0 {
		return strings.TrimSpace(v[idx+1:])
	}
	return v
}
