package services

import (
	"strings"
	"testing"
)

func TestBenchmarkDocumentExportHeadersMatchSchema(t *testing.T) {
	if len(benchmarkDocumentExportHeaders) != 36 {
		t.Fatalf("expected 36 Documents columns, got %d", len(benchmarkDocumentExportHeaders))
	}
	// Spot-check the exact order/name at a few positions against the search page schema.
	want := map[int]string{0: "ลำดับ", 1: "scopus_id", 4: "authors", 9: "afid", 14: "affiliations_json", 24: "citedby_count", 30: "journal_tier_bucket", 32: "publication_year", 35: "doi_url"}
	for idx, name := range want {
		if benchmarkDocumentExportHeaders[idx] != name {
			t.Fatalf("column %d = %q, want %q", idx, benchmarkDocumentExportHeaders[idx], name)
		}
	}
}

func TestBenchmarkCSVFieldNeutralisesFormulaInjection(t *testing.T) {
	cases := map[string]string{
		"=SUM(A1)":  "'=SUM(A1)",
		"+1+1":      "'+1+1",
		"-2+3":      "'-2+3",
		"@cmd":      "'@cmd",
		"Normal":    "Normal",
		"บทความไทย": "บทความไทย",
	}
	for in, want := range cases {
		if got := benchmarkCSVField(in); got != want {
			t.Fatalf("benchmarkCSVField(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBenchmarkCSVFieldEscapesSpecials(t *testing.T) {
	if got := benchmarkCSVField(`a,b`); got != `"a,b"` {
		t.Fatalf("comma escape = %q", got)
	}
	if got := benchmarkCSVField("line1\nline2"); got != "\"line1\nline2\"" {
		t.Fatalf("newline escape = %q", got)
	}
	if got := benchmarkCSVField(`he said "hi"`); got != `"he said ""hi"""` {
		t.Fatalf("quote escape = %q", got)
	}
	// A formula-triggering value that also needs quoting: guard first, then quote.
	if got := benchmarkCSVField("=1,2"); got != `"'=1,2"` {
		t.Fatalf("guard+quote = %q", got)
	}
}

func TestJournalTierBucketBoundaries(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		in   *float64
		want string
	}{
		{nil, ""}, {f(0), ""}, {f(-5), ""}, {f(24.9), "Q4"}, {f(25), "Q3"},
		{f(50), "Q2"}, {f(74.9), "Q2"}, {f(75), "Q1"}, {f(89.9), "Q1"}, {f(90), "T1"}, {f(100), "T1"},
	}
	for _, c := range cases {
		if got := journalTierBucket(c.in); got != c.want {
			t.Fatalf("journalTierBucket(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBenchmarkAuthKeywordsShapes(t *testing.T) {
	cases := map[string]string{
		`["ai","ml"]`:                  "ai; ml",
		`[{"$":"ai"},{"$":"ml"}]`:      "ai; ml",
		`[{"keyword":"nlp"}]`:          "nlp",
		`{"author-keyword":["a","b"]}`: "a; b",
		`ai; ml`:                       "ai; ml",
		``:                             "",
		`[garbage`:                     "", // structured but unparseable → blank, not a JSON dump
	}
	for in, want := range cases {
		if got := benchmarkAuthKeywords([]byte(in)); got != want {
			t.Fatalf("benchmarkAuthKeywords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinAffFieldJoinsAllAffiliations(t *testing.T) {
	// R2: the single country column must carry EVERY affiliation's country joined with
	// " | " (not just the first), so a Thailand+Japan paper still shows Japan.
	affs := []benchmarkAffiliationJSON{
		{Afid: "60001", Name: "KKU", City: "Khon Kaen", Country: "Thailand", AffiliationURL: "u1"},
		{Afid: "60999", Name: "U-Tokyo", City: "Tokyo", Country: "Japan", AffiliationURL: ""},
	}
	if got := joinAffField(affs, func(a benchmarkAffiliationJSON) string { return a.Country }); got != "Thailand | Japan" {
		t.Fatalf("country join = %q", got)
	}
	if got := joinAffField(affs, func(a benchmarkAffiliationJSON) string { return a.Afid }); got != "60001 | 60999" {
		t.Fatalf("afid join = %q", got)
	}
	// Empty values are skipped, not joined as blanks.
	if got := joinAffField(affs, func(a benchmarkAffiliationJSON) string { return a.AffiliationURL }); got != "u1" {
		t.Fatalf("url join must skip empties = %q", got)
	}
	if got := joinAffField(nil, func(a benchmarkAffiliationJSON) string { return a.Afid }); got != "" {
		t.Fatalf("no affiliations = %q", got)
	}
}

// R4: export completeness is decided PER YEAR — a year that over-harvests can never
// offset another year that is short (missingBenchmarkYears, reused by the export).
func TestExportCompletenessIsPerYearNotSum(t *testing.T) {
	// 60 harvested vs 100 snapshot for a single year → that year is short.
	if got := missingBenchmarkYears(2025, 2025, map[int]int{2025: 100}, map[int]int{2025: 60}); len(got) != 1 || got[0] != 2025 {
		t.Fatalf("60/100 must flag 2025: %v", got)
	}
	// Complete: harvested == snapshot → not flagged.
	if got := missingBenchmarkYears(2025, 2025, map[int]int{2025: 100}, map[int]int{2025: 100}); len(got) != 0 {
		t.Fatalf("100/100 must be complete: %v", got)
	}
	// Offsetting: 120/100 (over) + 80/100 (short) sums to 200/200 but BOTH years mismatch.
	got := missingBenchmarkYears(2024, 2025, map[int]int{2024: 100, 2025: 100}, map[int]int{2024: 120, 2025: 80})
	if len(got) != 2 {
		t.Fatalf("offsetting years must both be flagged, not netted: %v", got)
	}
	// Missing snapshot for a year in range → flagged.
	if got := missingBenchmarkYears(2024, 2025, map[int]int{2025: 10}, map[int]int{2025: 10}); len(got) != 1 || got[0] != 2024 {
		t.Fatalf("year with no snapshot must be flagged: %v", got)
	}
	// A real zero year (snapshot 0, harvested 0) is complete, not missing.
	if got := missingBenchmarkYears(2025, 2025, map[int]int{2025: 0}, map[int]int{}); len(got) != 0 {
		t.Fatalf("zero/zero must be complete: %v", got)
	}
}

func TestJoinCSVEscapesEachField(t *testing.T) {
	line := joinCSV([]string{"1", "a,b", "=x", "ปกติ"})
	if line != `1,"a,b",'=x,ปกติ` {
		t.Fatalf("joinCSV = %q", line)
	}
	if strings.Count(line, ",") < 3 {
		t.Fatalf("expected at least 3 separators, got %q", line)
	}
}
