package services

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

var insightColumns = []string{
	"docs", "oa_pct", "intl_pct", "avg_cite", "q1", "q2", "q3", "q4", "unclassified", "article", "conference", "other",
}

func TestBenchmarkInsightQueryUsesDocumentLevelRules(t *testing.T) {
	query := benchmarkInsightQuery(true)
	required := []string{
		"scopus_benchmark_document_scopes AS bds",
		"bds.pub_year = ?",
		"faculty_da.is_faculty = 1",
		"scopus_benchmark_affiliations AS intl_a",
		"LOWER(TRIM(intl_a.country)) <> 'thailand'",
		"scopus_source_metrics AS m",
		"MAX(latest_m.metric_year)",
		"conference paper",
	}
	for _, fragment := range required {
		if !strings.Contains(query, fragment) {
			t.Fatalf("insight query is missing %q", fragment)
		}
	}
	if strings.Contains(benchmarkInsightQuery(false), "faculty_da.is_faculty = 1") {
		t.Fatal("non-faculty levels must not apply the faculty subset")
	}
}

func TestUnavailableBenchmarkInsightSerializesCompactly(t *testing.T) {
	got, err := json.Marshal(BenchmarkInsightLevel{Available: false})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"available":false}` {
		t.Fatalf("unexpected unavailable payload: %s", got)
	}
}

func TestBenchmarkInsightsForYearBuildsCoverageAndUnavailableLevel(t *testing.T) {
	steps := []*queryStep{
		{
			kind: kindQuery, pattern: regexp.MustCompile(`(?s)SELECT.*bds\.scope_id = \? AND bds\.pub_year = \?.*faculty_da\.is_faculty = 1`),
			args: []driver.Value{int64(1), int64(2026)}, columns: insightColumns,
			rows: [][]driver.Value{{int64(59), float64(53), float64(53), float64(0.83), int64(27), int64(8), int64(2), int64(13), int64(9), int64(37), int64(12), int64(10)}},
		},
		{
			kind: kindQuery, pattern: regexp.MustCompile(`(?s)SELECT.*scopus_benchmark_document_scopes.*bds\.scope_id = \? AND bds\.pub_year = \?`),
			args: []driver.Value{int64(1), int64(2026)}, columns: insightColumns,
			rows: [][]driver.Value{{int64(221), float64(57), float64(39), float64(1.25), int64(90), int64(31), int64(18), int64(47), int64(35), int64(140), int64(51), int64(30)}},
		},
		{
			kind: kindQuery, pattern: regexp.MustCompile(`(?s)SELECT.*scopus_benchmark_document_scopes.*bds\.scope_id = \? AND bds\.pub_year = \?`),
			args: []driver.Value{int64(2), int64(2026)}, columns: insightColumns,
			rows: [][]driver.Value{{int64(0), float64(0), float64(0), float64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0)}},
		},
	}
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()

	got, err := NewScopusBenchmarkService(db, nil).BenchmarkInsightsForYear(context.Background(), 2026)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Levels["faculty"].Available || got.Levels["faculty"].Docs != 59 {
		t.Fatalf("unexpected faculty insight: %+v", got.Levels["faculty"])
	}
	if got.Levels["thailand"].Available {
		t.Fatalf("empty Thailand level must be unavailable: %+v", got.Levels["thailand"])
	}
	if got.Coverage.Classified != 236 || got.Coverage.Total != 280 {
		t.Fatalf("unexpected coverage: %+v", got.Coverage)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}

func TestBenchmarkTopJournalsUsesKKUScopeAndLimit(t *testing.T) {
	steps := []*queryStep{{
		kind:    kindQuery,
		pattern: regexp.MustCompile(`(?s)WHERE bds\.scope_id = 1.*GROUP BY TRIM\(d\.publication_name\).*LIMIT \?`),
		args:    []driver.Value{int64(8)},
		columns: []string{"name", "docs", "avg_cite"},
		rows:    [][]driver.Value{{"  Journal A  ", int64(12), float64(4.5)}},
	}}
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()

	got, err := NewScopusBenchmarkService(db, nil).BenchmarkTopJournals(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Journal A" || got[0].Docs != 12 {
		t.Fatalf("unexpected journals: %+v", got)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}
