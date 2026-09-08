package services

import (
	"context"
	"database/sql/driver"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

type benchmarkRoundTripFunc func(*http.Request) (*http.Response, error)

func (f benchmarkRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestVerifiedFacultyCountQueryUsesOfficialRules(t *testing.T) {
	query, args := verifiedFacultyCountQuery(7, nil)

	requiredFragments := []string{
		"COUNT(DISTINCT bd.eid)",
		"scopus_benchmark_documents AS bd",
		"scopus_benchmark_document_scopes AS bds",
		"JOIN scopus_documents AS sd",
		"JOIN users AS u ON TRIM(u.scopus_id) = sa.scopus_author_id",
		"JOIN scopus_affiliations AS aff ON aff.id = sda.affiliation_id",
		"u.delete_at IS NULL",
		"u.date_of_employment IS NULL",
		"(sd.cover_date IS NOT NULL AND DATE(sd.cover_date) >= DATE(u.date_of_employment))",
		"TRIM(aff.afid) IN (?, ?)",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(query, fragment) {
			t.Fatalf("verified faculty query is missing %q", fragment)
		}
	}
	if strings.Contains(query, "bds.pub_year = ?") {
		t.Fatal("all-years query must not constrain pub_year")
	}
	if want := []interface{}{uint64(7), benchmarkKKUAfID, benchmarkKKUScienceAfID}; !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestVerifiedFacultyCountQueryAddsYearConstraint(t *testing.T) {
	year := 2025
	query, args := verifiedFacultyCountQuery(3, &year)

	if !strings.Contains(query, "bds.pub_year = ?") {
		t.Fatal("year query must constrain benchmark scope membership year")
	}
	if want := []interface{}{uint64(3), benchmarkKKUAfID, benchmarkKKUScienceAfID, 2025}; !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestIsFacetNotEntitled(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"400 not entitled to facets", http.StatusBadRequest, `{"error":"not entitled to access facets"}`, true},
		{"403 facet not authorized", http.StatusForbidden, "facet access not authorized", true},
		{"400 unrelated bad request", http.StatusBadRequest, "invalid query syntax", false},
		{"429 rate limited", http.StatusTooManyRequests, "not entitled to access facets", false},
		{"200 ok", http.StatusOK, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFacetNotEntitled(tc.status, []byte(tc.body)); got != tc.want {
				t.Fatalf("isFacetNotEntitled(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
			}
		})
	}
}

func TestFacultyEmploymentCoverageReportsDateFallback(t *testing.T) {
	db, state, cleanup := newScriptedGormDB(t, []*queryStep{{
		kind:    kindQuery,
		pattern: regexp.MustCompile(`(?s)SELECT.*COUNT\(\*\).*FROM users`),
		columns: []string{"faculty_with_scopus_id", "employment_date_set", "employment_date_missing"},
		rows:    [][]driver.Value{{int64(41), int64(1), int64(40)}},
	}})
	defer cleanup()

	coverage, err := NewScopusBenchmarkService(db, nil).FacultyEmploymentCoverage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !coverage.Ready {
		t.Fatal("AF-ID fallback keeps the metric available when employment dates are incomplete")
	}
	if coverage.EmploymentDateComplete {
		t.Fatal("incomplete employment dates must be reported")
	}
	if coverage.FacultyWithScopusID != 41 || coverage.EmploymentDateSet != 1 || coverage.EmploymentDateMissing != 40 {
		t.Fatalf("unexpected coverage: %+v", coverage)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}

func TestFacultyEmploymentCoverageReadyWhenComplete(t *testing.T) {
	db, state, cleanup := newScriptedGormDB(t, []*queryStep{{
		kind:    kindQuery,
		pattern: regexp.MustCompile(`(?s)SELECT.*COUNT\(\*\).*FROM users`),
		columns: []string{"faculty_with_scopus_id", "employment_date_set", "employment_date_missing"},
		rows:    [][]driver.Value{{int64(41), int64(41), int64(0)}},
	}})
	defer cleanup()

	coverage, err := NewScopusBenchmarkService(db, nil).FacultyEmploymentCoverage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !coverage.Ready {
		t.Fatalf("complete employment dates must be ready: %+v", coverage)
	}
	if !coverage.EmploymentDateComplete {
		t.Fatalf("complete employment dates must be reported: %+v", coverage)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}

func TestSearchPubYearFacetReturnsValidatedBuckets(t *testing.T) {
	client := &http.Client{Transport: benchmarkRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.Query().Get("facets"); got != "pubyear(count=3)" {
			t.Fatalf("facets = %q", got)
		}
		if got := req.URL.Query().Get("count"); got != "1" {
			t.Fatalf("count = %q", got)
		}
		if got := req.Header.Get(scopusAPIKeyField); got != "test-key" {
			t.Fatalf("API key header = %q", got)
		}
		body := `{"search-results":{"opensearch:totalResults":"766","facet":{"name":"pubyear","attribute":"pubyear","category":[{"value":"2026","name":"2026","hitCount":"214"},{"value":"2025","name":"2025","hitCount":"311"},{"value":"2024","name":"2024","hitCount":"241"}]}}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	counts, err := NewScopusBenchmarkService(nil, client).
		searchPubYearFacet(context.Background(), "test-key", "AF-ID(60017165)", 2024, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[int]int{2024: 241, 2025: 311, 2026: 214}; !reflect.DeepEqual(counts, want) {
		t.Fatalf("counts = %#v, want %#v", counts, want)
	}
}

func TestDecodePubYearFacetRejectsIncompleteBuckets(t *testing.T) {
	body := []byte(`{"search-results":{"opensearch:totalResults":"10","facet":{"name":"pubyear","category":[{"value":"2026","hitCount":"9"}]}}}`)
	_, err := decodePubYearFacet(body, 2026, 2026)
	if err == nil || !strings.Contains(err.Error(), "total mismatch") {
		t.Fatalf("expected total mismatch, got %v", err)
	}
}

func TestDecodePubYearFacetAllowsEmptyResult(t *testing.T) {
	body := []byte(`{"search-results":{"opensearch:totalResults":"0"}}`)
	counts, err := decodePubYearFacet(body, 2027, 2027)
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 0 {
		t.Fatalf("counts = %#v, want empty", counts)
	}
}

func TestSearchPageWithRetryRecoversFromTimeout(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: benchmarkRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, context.DeadlineExceeded
		}
		body := `{"search-results":{"opensearch:totalResults":"1","cursor":{"@next":"done"},"entry":[{"eid":"2-s2.0-1"}]}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	total, entries, next, err := NewScopusBenchmarkService(nil, client).
		searchPageWithRetry(context.Background(), "test-key", "SUBJAREA(COMP)", "*")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || total != 1 || len(entries) != 1 || next != "done" {
		t.Fatalf("calls=%d total=%d entries=%d next=%q", calls, total, len(entries), next)
	}
}

func TestBenchmarkYearRangesUsesAtMostThirtyYears(t *testing.T) {
	got := benchmarkYearRanges(1981, 2026)
	want := [][2]int{{1981, 2010}, {2011, 2026}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranges = %#v, want %#v", got, want)
	}
}

func TestMissingBenchmarkYearsRequiresExactHarvestCoverage(t *testing.T) {
	expected := map[int]int{2024: 241, 2025: 311, 2026: 214}
	harvested := map[int]int{2024: 241, 2025: 300, 2026: 214}

	got := missingBenchmarkYears(2023, 2026, expected, harvested)
	want := []int{2023, 2025}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing years = %#v, want %#v", got, want)
	}
}

func TestMissingBenchmarkYearsAcceptsCompleteHarvest(t *testing.T) {
	expected := map[int]int{2024: 241, 2025: 311, 2026: 214}
	harvested := map[int]int{2024: 241, 2025: 311, 2026: 214}

	if got := missingBenchmarkYears(2024, 2026, expected, harvested); len(got) != 0 {
		t.Fatalf("missing years = %#v, want empty", got)
	}
}
