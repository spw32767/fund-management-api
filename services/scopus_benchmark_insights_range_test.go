package services

import (
	"strings"
	"testing"
)

// makeLevel builds an available level with the given intl coverage and a ready
// per-metric readiness block, so aggregation tests can focus on one dimension.
func makeLevel(docs, intlKnown, intlPositive int) BenchmarkInsightLevel {
	lvl := BenchmarkInsightLevel{
		Available: true,
		Docs:      docs,
		Intl:      BenchmarkCoverageCounts{Known: intlKnown, Positive: intlPositive, Unknown: docs - intlKnown},
		OA:        BenchmarkCoverageCounts{Known: docs, Positive: 0, Unknown: 0},
		Citations: computeCitationSummary(docs, docs, ptrInt64(0)),
	}
	lvl.Readiness = BenchmarkLevelReadiness{
		ComparisonReady: true,
		ObservedDocs:    docs,
		Reasons:         []string{},
		Metrics: map[string]BenchmarkMetricReadiness{
			"count":     {Ready: true, Reasons: []string{}},
			"quality":   {Ready: true, Reasons: []string{}},
			"intl":      {Ready: intlKnown == docs, Reasons: []string{}},
			"oa":        {Ready: true, Reasons: []string{}},
			"citations": {Ready: true, Reasons: []string{}},
		},
	}
	return lvl
}

func ptrInt64(v int64) *int64 { return &v }

// The international rate over a range must be Σpositive / Σknown, NEVER the mean of
// per-year percentages (§3.2 / §9). 1/2 and 9/18 → 10/20 = 50%.
func TestAggregateRangeLevelIntlIsPooledNotAveraged(t *testing.T) {
	byYear := map[int]BenchmarkInsightLevel{
		2025: makeLevel(2, 2, 1),   // 50%
		2026: makeLevel(18, 18, 9), // 50%
	}
	agg := aggregateRangeLevel([]int{2025, 2026}, byYear)
	if agg.Intl.Positive != 10 || agg.Intl.Known != 20 {
		t.Fatalf("pooled intl = %d/%d, want 10/20", agg.Intl.Positive, agg.Intl.Known)
	}
	if agg.IntlPct != 50.0 {
		t.Fatalf("intl pct = %v, want 50.0", agg.IntlPct)
	}
}

// A second fixture with unequal denominators catches the averaging bug: 1/2 (50%)
// and 9/10 (90%) pool to 10/12 = 83.33%, not the 70% a per-year mean would give.
func TestAggregateRangeLevelIntlUnequalDenominators(t *testing.T) {
	byYear := map[int]BenchmarkInsightLevel{
		2025: makeLevel(2, 2, 1),   // 50%
		2026: makeLevel(10, 10, 9), // 90%
	}
	agg := aggregateRangeLevel([]int{2025, 2026}, byYear)
	if agg.Intl.Positive != 10 || agg.Intl.Known != 12 {
		t.Fatalf("pooled intl = %d/%d, want 10/12", agg.Intl.Positive, agg.Intl.Known)
	}
	if agg.IntlPct != 83.33 {
		t.Fatalf("intl pct = %v, want 83.33", agg.IntlPct)
	}
}

// T1–Q2 numerator/denominator must pool across years without double counting T1
// into Q1, and highTierShare must be computable from the aggregate quartile.
func TestAggregateRangeLevelQuartilePooled(t *testing.T) {
	a := makeLevel(10, 10, 5)
	a.Quartile = BenchmarkQuartileBreakdown{T1: 1, Q1: 2, Q2: 1, Q3: 1, Q4: 0}
	b := makeLevel(10, 10, 5)
	b.Quartile = BenchmarkQuartileBreakdown{T1: 1, Q1: 1, Q2: 2, Q3: 0, Q4: 1}
	agg := aggregateRangeLevel([]int{2025, 2026}, map[int]BenchmarkInsightLevel{2025: a, 2026: b})
	q := agg.Quartile
	if q.T1 != 2 || q.Q1 != 3 || q.Q2 != 3 || q.Q3 != 1 || q.Q4 != 1 {
		t.Fatalf("pooled quartile = %+v", q)
	}
	// classified = 2+3+3+1+1 = 10; T1–Q2 numerator = 2+3+3 = 8.
	classified := q.T1 + q.Q1 + q.Q2 + q.Q3 + q.Q4
	if classified != 10 {
		t.Fatalf("classified = %d, want 10", classified)
	}
}

// A single missing/blocked year must make the range metric not-ready and keep the
// offending year in the reason, while a real zero-snapshot year stays a value.
func TestAggregateRangeLevelReadinessPerYear(t *testing.T) {
	ready := makeLevel(10, 10, 5)
	blocked := makeLevel(0, 0, 0)
	blocked.Available = false
	blocked.Readiness = BenchmarkLevelReadiness{
		ComparisonReady: false,
		ObservedDocs:    0,
		Reasons:         []string{"no KKU documents for this year"},
		Metrics: map[string]BenchmarkMetricReadiness{
			"count":     {Ready: false, Reasons: []string{"no KKU documents for this year"}},
			"quality":   {Ready: false, Reasons: []string{"no KKU documents for this year"}},
			"intl":      {Ready: false, Reasons: []string{"no KKU documents for this year"}},
			"oa":        {Ready: false, Reasons: []string{"no KKU documents for this year"}},
			"citations": {Ready: false, Reasons: []string{"no KKU documents for this year"}},
		},
	}
	agg := aggregateRangeLevel([]int{2025, 2026}, map[int]BenchmarkInsightLevel{2025: ready, 2026: blocked})
	if agg.Readiness.Metrics["count"].Ready {
		t.Fatalf("count must be blocked when a year is missing: %+v", agg.Readiness.Metrics["count"])
	}
	joined := strings.Join(agg.Readiness.Metrics["count"].Reasons, " | ")
	if !strings.Contains(joined, "2026:") {
		t.Fatalf("blocking reason must name the offending year: %q", joined)
	}
	// The available year still contributed its docs — the range is observed, not empty.
	if !agg.Available || agg.Docs != 10 {
		t.Fatalf("range should stay available with 10 observed docs: %+v", agg)
	}
}

// One quality-blocked year blocks only the range quality metric; intl/oa/count that
// are ready in every year stay ready (§2.3 per-metric readiness, aggregated).
func TestAggregateRangeLevelPerMetricIsolation(t *testing.T) {
	good := makeLevel(10, 10, 5)
	qBad := makeLevel(10, 10, 5)
	qBad.Readiness.Metrics["quality"] = BenchmarkMetricReadiness{Ready: false, Reasons: []string{"3 journals have no CiteScore metadata"}}
	agg := aggregateRangeLevel([]int{2025, 2026}, map[int]BenchmarkInsightLevel{2025: good, 2026: qBad})
	if agg.Readiness.Metrics["quality"].Ready {
		t.Fatalf("quality must be blocked: %+v", agg.Readiness.Metrics["quality"])
	}
	if !agg.Readiness.Metrics["intl"].Ready || !agg.Readiness.Metrics["oa"].Ready || !agg.Readiness.Metrics["count"].Ready {
		t.Fatalf("unaffected metrics must stay ready: %+v", agg.Readiness.Metrics)
	}
}

// A range of exactly one year must equal that year's single-level metrics, so the
// range path and the single-year path agree at start == end (§9).
func TestAggregateRangeLevelSingleYearMatches(t *testing.T) {
	lvl := makeLevel(7, 7, 3)
	lvl.Quartile = BenchmarkQuartileBreakdown{T1: 1, Q1: 1, Q2: 1, Q3: 0, Q4: 0}
	agg := aggregateRangeLevel([]int{2026}, map[int]BenchmarkInsightLevel{2026: lvl})
	if agg.Docs != 7 || agg.Intl.Positive != 3 || agg.Intl.Known != 7 {
		t.Fatalf("single-year aggregate mismatch: %+v", agg)
	}
	if agg.IntlPct != round2(100*3.0/7.0) {
		t.Fatalf("single-year intl pct = %v", agg.IntlPct)
	}
	if agg.Quartile.T1 != 1 || agg.Quartile.Q2 != 1 {
		t.Fatalf("single-year quartile mismatch: %+v", agg.Quartile)
	}
}
