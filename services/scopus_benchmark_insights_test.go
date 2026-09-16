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
	"docs", "oa_pct", "intl_pct", "avg_cite", "t1", "q1", "q2", "q3", "q4", "article", "conference", "other",
}

func TestBenchmarkInsightQueryUsesVerifiedFacultyCohort(t *testing.T) {
	faculty := benchmarkInsightQuery(true)
	// The faculty cohort must use the verified-faculty EXISTS selection (matching
	// the official count), not the is_faculty flag (handoff §9 B).
	required := []string{
		"scopus_benchmark_document_scopes AS bds",
		"bds.pub_year = ?",
		"FROM scopus_documents AS sd",
		"JOIN users AS u ON TRIM(u.scopus_id) = sa.scopus_author_id",
		"TRIM(aff.afid) IN (?, ?)",
		"scopus_source_metrics AS m",
		"m.cite_score_percentile >= 90 AND m.cite_score_percentile <= 100",
	}
	for _, fragment := range required {
		if !strings.Contains(faculty, fragment) {
			t.Fatalf("faculty insight query is missing %q", fragment)
		}
	}
	if strings.Contains(faculty, "faculty_da.is_faculty = 1") {
		t.Fatal("faculty cohort must no longer rely on the is_faculty flag")
	}

	scope := benchmarkInsightQuery(false)
	if strings.Contains(scope, "FROM scopus_documents AS sd") {
		t.Fatal("non-faculty levels must not apply the verified-faculty subset")
	}
}

func TestBenchmarkInsightQueryTiersJournalsOnlyAndReportsCoverage(t *testing.T) {
	q := benchmarkInsightQuery(false)
	// Tiers must be restricted to journals; books/book series/conference proceedings
	// are excluded, not tiered (R6/§9 B).
	if !strings.Contains(q, "= 'journal'\n\t\t\t\tAND m.cite_score_percentile >= 90") &&
		!strings.Contains(q, "'journal'") {
		t.Fatal("tier CASE must require aggregation_type = 'journal'")
	}
	if strings.Contains(q, "<> 'conference proceeding'\n\t\t\t\tAND m.cite_score_percentile >= 90") {
		t.Fatal("tiers must no longer accept any non-conference type")
	}
	for _, fragment := range []string{
		"AS oa_known", "AS oa_positive", "AS intl_known", "AS intl_positive",
		"AS journal_docs", "AS excluded_non_journal", "AS unresolved_type",
	} {
		if !strings.Contains(q, fragment) {
			t.Fatalf("insight query missing coverage column %q", fragment)
		}
	}
}

func TestBenchmarkCitationQueryIsDistinctCohortSum(t *testing.T) {
	scope := benchmarkCitationQuery(false)
	required := []string{
		"SUM(d.citedby_count) AS total",
		"d.citedby_count IS NOT NULL THEN 1 ELSE 0 END) AS known_docs",
		"COUNT(*) AS cohort_docs",
		"bds.pub_year = ?",
	}
	for _, fragment := range required {
		if !strings.Contains(scope, fragment) {
			t.Fatalf("citation query is missing %q", fragment)
		}
	}
	// Citations must be summed over the plain document cohort, never joined to the
	// source-metrics table (which can duplicate a document — §9 D).
	if strings.Contains(scope, "scopus_source_metrics") {
		t.Fatal("citation query must not join source metrics (would duplicate docs)")
	}
	if !strings.Contains(benchmarkCitationQuery(true), "TRIM(aff.afid) IN (?, ?)") {
		t.Fatal("faculty citation cohort must use the verified-faculty selection")
	}
}

func TestInsightArgsOrderMatchesQueryPlaceholders(t *testing.T) {
	if got := insightArgs(7, 2025, false); len(got) != 2 || got[0] != uint64(7) || got[1] != 2025 {
		t.Fatalf("scope args = %#v", got)
	}
	// The WHERE clause (scope_id, pub_year) comes first in the SQL; the verified-
	// faculty EXISTS clause is appended AFTER it, so its two AF-IDs bind LAST.
	got := insightArgs(7, 2025, true)
	want := []interface{}{uint64(7), 2025, benchmarkKKUAfID, benchmarkKKUScienceAfID}
	if len(got) != 4 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("faculty args = %#v, want %#v", got, want)
	}
}

// TestBenchmarkInsightLevelBindsFacultyArgsInQueryOrder is a regression test for
// the R1 bug: the faculty aggregate and citation queries put scope_id/pub_year in
// the WHERE (first) and the two AF-IDs in the trailing EXISTS clause, so the bound
// args MUST be [scope, year, afid1, afid2]. The scripted driver matches args in
// order, so the wrong order (afid1, afid2, scope, year) fails here.
func TestBenchmarkInsightLevelBindsFacultyArgsInQueryOrder(t *testing.T) {
	facultyArgs := []driver.Value{int64(1), int64(2025), benchmarkKKUAfID, benchmarkKKUScienceAfID}
	steps := []*queryStep{
		{
			kind:    kindQuery,
			pattern: regexp.MustCompile(`(?s)SELECT.*bds\.scope_id = \? AND bds\.pub_year = \?.*FROM scopus_documents AS sd.*TRIM\(aff\.afid\) IN \(\?, \?\)`),
			args:    facultyArgs,
			columns: insightColumns,
			rows:    [][]driver.Value{{int64(50), float64(60), float64(48), float64(2.5), int64(8), int64(10), int64(6), int64(4), int64(2), int64(40), int64(8), int64(2)}},
		},
		{
			kind:    kindQuery,
			pattern: regexp.MustCompile(`(?s)SUM\(d\.citedby_count\) AS total.*bds\.scope_id = \? AND bds\.pub_year = \?.*TRIM\(aff\.afid\) IN \(\?, \?\)`),
			args:    facultyArgs,
			columns: []string{"cohort_docs", "known_docs", "total"},
			rows:    [][]driver.Value{{int64(50), int64(40), int64(200)}},
		},
	}
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()

	level, err := NewScopusBenchmarkService(db, nil).benchmarkInsightLevel(context.Background(), 1, 2025, true)
	if err != nil {
		t.Fatal(err)
	}
	if !level.Available || level.Docs != 50 {
		t.Fatalf("faculty level should be available with 50 docs: %+v", level)
	}
	if level.Citations.KnownDocs != 40 || level.Citations.Total == nil || *level.Citations.Total != 200 {
		t.Fatalf("faculty citations mis-bound: %+v", level.Citations)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}

func TestComputeCitationSummaryRules(t *testing.T) {
	i64 := func(v int64) *int64 { return &v }

	// No known citation documents: total and average are null, not zero (§8/§9 D).
	none := computeCitationSummary(10, 0, nil)
	if none.Total != nil || none.Average != nil {
		t.Fatalf("known=0 must yield null total/average: %+v", none)
	}
	if none.CoverageStatus != "partial" || none.UnknownDocs != 10 {
		t.Fatalf("10 cohort / 0 known is partial coverage: %+v", none)
	}

	// Empty cohort is "none", still null.
	empty := computeCitationSummary(0, 0, nil)
	if empty.CoverageStatus != "none" || empty.Total != nil {
		t.Fatalf("empty cohort: %+v", empty)
	}

	// Every known citation is a real zero: total and average are 0, not null.
	zeros := computeCitationSummary(5, 5, i64(0))
	if zeros.Total == nil || *zeros.Total != 0 || zeros.Average == nil || *zeros.Average != 0 {
		t.Fatalf("all-zero known docs must be 0/0: %+v", zeros)
	}
	if zeros.CoverageStatus != "complete" {
		t.Fatalf("no unknown docs must be complete: %+v", zeros)
	}

	// Partial coverage: average divides by known docs only, never by cohort docs.
	partial := computeCitationSummary(72, 50, i64(120))
	if partial.CoverageStatus != "partial" || partial.UnknownDocs != 22 {
		t.Fatalf("partial coverage: %+v", partial)
	}
	if partial.Average == nil || *partial.Average != 120.0/50.0 {
		t.Fatalf("average must use known docs as denominator: %+v", partial)
	}
	if partial.Total == nil || *partial.Total != 120 {
		t.Fatalf("total must be the raw sum, not rounded avg*docs: %+v", partial)
	}
	if partial.DenominatorPolicy != "known_citation_docs" || partial.FreshnessStatus != "unknown" {
		t.Fatalf("denominator policy / freshness metadata: %+v", partial)
	}
	if partial.UpdatedAt != nil {
		t.Fatalf("no provable citation-refresh timestamp must stay null: %+v", partial)
	}
}

func TestComputeLevelReadinessPerLevel(t *testing.T) {
	docs := 72
	matched := &docs                                             // expected snapshot equals observed docs → no mismatch
	full := levelMetricMeta{available: true, observedDocs: docs} // all metadata complete

	// Faculty is ready only when metric ready, year complete, no KKU harvest, and the
	// harvested cohort matches the count snapshot.
	ready := computeLevelReadiness("faculty", full, matched, levelReadinessInputs{facultyMetricReady: true})
	if !ready.ComparisonReady || len(ready.Reasons) != 0 || ready.ObservedDocs != 72 {
		t.Fatalf("faculty should be ready: %+v", ready)
	}
	if !ready.Metrics["count"].Ready || !ready.Metrics["quality"].Ready || !ready.Metrics["oa"].Ready || !ready.Metrics["intl"].Ready {
		t.Fatalf("all metrics should be ready when metadata is complete: %+v", ready.Metrics)
	}
	blocked := computeLevelReadiness("faculty", full, matched, levelReadinessInputs{
		facultyMetricReady: false, facultyYearMissing: true, kkuHarvestActive: true,
	})
	if blocked.ComparisonReady || !blocked.ActiveRun || len(blocked.Reasons) == 0 {
		t.Fatalf("faculty should be blocked with reasons: %+v", blocked)
	}

	// A KKU harvest blocks kku but a Thailand harvest must not, and vice versa.
	kku := computeLevelReadiness("kku", full, matched, levelReadinessInputs{kkuHarvestActive: true})
	if kku.ComparisonReady || !kku.ActiveRun {
		t.Fatalf("kku must not be ready during a KKU harvest: %+v", kku)
	}
	country := computeLevelReadiness("country", full, matched, levelReadinessInputs{kkuHarvestActive: true})
	if !country.ComparisonReady || country.ActiveRun {
		t.Fatalf("a KKU harvest must not block Thailand readiness: %+v", country)
	}
	countryBusy := computeLevelReadiness("country", full, matched, levelReadinessInputs{countryHarvestActive: true})
	if countryBusy.ComparisonReady || !countryBusy.ActiveRun {
		t.Fatalf("a Thailand harvest must block Thailand readiness: %+v", countryBusy)
	}
	unavailable := computeLevelReadiness("country", levelMetricMeta{available: false}, nil, levelReadinessInputs{})
	if unavailable.ComparisonReady || unavailable.Metrics["count"].Ready {
		t.Fatalf("an unavailable level cannot be comparison-ready: %+v", unavailable)
	}
}

func TestComputeLevelReadinessSnapshotMismatch(t *testing.T) {
	expected := 74
	// Snapshot says 74 docs but only 72 were harvested → harvest incomplete, so the
	// level is not comparison-ready even though its metric metadata is otherwise fine.
	r := computeLevelReadiness("kku", levelMetricMeta{available: true, observedDocs: 72}, &expected, levelReadinessInputs{})
	if !r.SnapshotMismatch || r.ComparisonReady {
		t.Fatalf("mismatch must block comparison: %+v", r)
	}
	if r.ExpectedDocs == nil || *r.ExpectedDocs != 74 || r.ObservedDocs != 72 {
		t.Fatalf("expected/observed must be reported: %+v", r)
	}
	if !strings.Contains(strings.Join(r.Reasons, " "), "harvest incomplete") {
		t.Fatalf("mismatch reason missing: %+v", r.Reasons)
	}
}

func TestComputeLevelReadinessNilSnapshotBlocksCompleteness(t *testing.T) {
	// A missing count snapshot means completeness cannot be proven — NOT ready (R2-1).
	r := computeLevelReadiness("kku", levelMetricMeta{available: true, observedDocs: 72}, nil, levelReadinessInputs{})
	if r.ComparisonReady || r.Metrics["count"].Ready {
		t.Fatalf("nil snapshot must block completeness: %+v", r)
	}
	if !strings.Contains(strings.Join(r.Reasons, " "), "no count snapshot") {
		t.Fatalf("nil-snapshot reason missing: %+v", r.Reasons)
	}
}

func TestComputeLevelReadinessPerMetricMetadata(t *testing.T) {
	docs := 100
	snap := &docs
	// Harvest complete, but each metric has its own incomplete metadata: only the
	// affected metric is blocked; the others stay ready (R2-1).
	r := computeLevelReadiness("kku", levelMetricMeta{
		available: true, observedDocs: 100,
		unclassifiedJournal: 10, // blocks quality
		oaUnknown:           5,  // blocks oa
		intlUnknown:         0,  // intl ok
		citationUnknown:     3,  // blocks citations
	}, snap, levelReadinessInputs{})
	if !r.ComparisonReady {
		t.Fatalf("base/count should be ready when harvest is complete: %+v", r)
	}
	if r.Metrics["quality"].Ready {
		t.Fatalf("quality must be blocked by unclassified journals: %+v", r.Metrics["quality"])
	}
	if r.Metrics["oa"].Ready {
		t.Fatalf("oa must be blocked by unknown OA: %+v", r.Metrics["oa"])
	}
	if r.Metrics["citations"].Ready {
		t.Fatalf("citations must be blocked by unknown citation counts: %+v", r.Metrics["citations"])
	}
	if !r.Metrics["intl"].Ready || !r.Metrics["count"].Ready {
		t.Fatalf("intl and count must remain ready: %+v", r.Metrics)
	}
}

func TestUnavailableBenchmarkInsightSerializesWithCitationAndReadiness(t *testing.T) {
	level := BenchmarkInsightLevel{
		Available: false,
		Citations: computeCitationSummary(0, 0, nil),
		Readiness: computeLevelReadiness("country", levelMetricMeta{available: false}, nil, levelReadinessInputs{}),
	}
	got, err := json.Marshal(level)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["available"]; !ok {
		t.Fatalf("unavailable level must still carry available: %s", got)
	}
	if _, ok := decoded["citations"]; !ok {
		t.Fatalf("unavailable level must still carry citations metadata: %s", got)
	}
	if _, ok := decoded["docs"]; ok {
		t.Fatalf("unavailable level must not serialize metric fields: %s", got)
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
