package services

import (
	"bytes"
	"fund-management-api/models"
	"github.com/xuri/excelize/v2"
	"net/url"
	"testing"
	"time"
)

func summaryBool(v bool) *bool        { return &v }
func summaryString(v string) *string  { return &v }
func summaryFloat(v float64) *float64 { return &v }
func summaryFixture() summaryInput {
	return summaryInput{
		Categories: []SummaryCategory{{ID: 1, Name: "AI", Code: "AI_ALGORITHMS"}},
		Faculty:    []SummaryFaculty{{UserID: 1, Name: "First Corr", ScopusID: "A"}, {UserID: 2, Name: "Co", ScopusID: "B"}, {UserID: 3, Name: "No ID"}, {UserID: 4, Name: "Zero", ScopusID: "Z"}},
		Years:      []SummaryYearState{{Year: 2025, Status: "missing"}, {Year: 2026, Status: "available", Observed: 4}},
		Documents: []SummaryDocument{
			{EID: "one", Year: 2026, Type: "Journal", CategoryID: 1, Confidence: "High", SourceID: "j", Afids: []string{"60277695", "60017165"}, Authors: []SummaryAuthor{
				{ScopusAuthorID: "A", Afids: []string{"other", "60017165"}, RoleStatus: "complete", First: summaryBool(true), Corresponding: summaryBool(true)},
				{ScopusAuthorID: "B", Afids: []string{"60280609"}, RoleStatus: "complete", First: summaryBool(false), Corresponding: summaryBool(false)},
			}},
			{EID: "two", Year: 2026, Type: "Journal", CategoryID: 1, Confidence: "Medium", Afids: []string{"60026046", "60280609"}, Authors: []SummaryAuthor{{ScopusAuthorID: "B", Afids: []string{"60280609"}, RoleStatus: "no_correspondence", First: summaryBool(false), Corresponding: summaryBool(false)}}},
			{EID: "other-author-kku", Year: 2026, Type: "Journal", CategoryID: 1, Confidence: "High", Afids: []string{"109899034", "60017165"}, Authors: []SummaryAuthor{{ScopusAuthorID: "A", Afids: []string{"other"}}, {ScopusAuthorID: "outside", Afids: []string{"60017165"}}}},
			{EID: "country", Year: 2026, Type: "Journal", CategoryID: 1, Confidence: "High", Afids: []string{"not-kku"}},
		},
		Metrics: []models.ScopusSourceMetric{{SourceID: "j", DocType: "all", MetricYear: 2025, CiteScoreStatus: summaryString("Complete"), CiteScoreQuartile: summaryString("Q1"), CiteScorePercentile: summaryFloat(95)}},
	}
}
func summaryFilter() BenchmarkSummaryFilter {
	f, _ := ParseBenchmarkSummaryFilter(url.Values{}, time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC))
	return f
}
func TestSummaryDefaultExcludesPreface(t *testing.T) {
	f := summaryFilter()
	if summaryContains(f.Confidence, "Preface") || !summaryContains(f.Confidence, "unknown") {
		t.Fatalf("unexpected default confidence: %v", f.Confidence)
	}
	in := summaryFixture()
	in.Documents[0].Confidence = "Preface"
	if r := aggregateBenchmarkSummary(in, f); *r.Total.Thailand != 3 {
		t.Fatalf("Preface included in default report: %+v", r.Total)
	}
}
func TestSummaryNestedCohortAndRoleUnits(t *testing.T) {
	r := aggregateBenchmarkSummary(summaryFixture(), summaryFilter())
	if *r.Total.Thailand != 4 || *r.Total.KKU != 3 || *r.Total.COC != 2 {
		t.Fatalf("nested counts: %+v", r.Total)
	}
	if r.Yearly[0].Thailand != nil || *r.Yearly[1].Thailand != 4 {
		t.Fatal("missing year treated as zero")
	}
	if r.FacultyRoles != (SummaryRoleCounts{Total: 2, First: 1, Corresponding: 1, Lead: 1, Co: 1}) {
		t.Fatalf("paper roles %+v", r.FacultyRoles)
	}
	if r.Faculty[0].Total != 1 || r.Faculty[0].Lead != 1 || r.Faculty[1].Total != 2 || r.Faculty[1].Co != 2 {
		t.Fatal("faculty-paper pair or role overlap incorrect")
	}
	if r.Faculty[2].Linkable || !r.Faculty[3].Linkable || r.Faculty[3].Total != 0 {
		t.Fatal("roster must distinguish missing ID from zero")
	}
}
func TestSummaryUnknownRolesAndMultipleCorresponding(t *testing.T) {
	in := summaryFixture()
	in.Documents = in.Documents[:1]
	in.Documents[0].Authors[0].First = summaryBool(false)
	in.Documents[0].Authors[0].Corresponding = summaryBool(true)
	in.Documents[0].Authors[1].Corresponding = summaryBool(true)
	r := aggregateBenchmarkSummary(in, summaryFilter())
	if r.Faculty[0].Corresponding != 1 || r.Faculty[1].Corresponding != 1 || r.FacultyRoles.Corresponding != 1 {
		t.Fatal("multiple corresponding must count one paper but two pairs")
	}
	for _, status := range []string{"pending", "needs_review", "fetch_error", ""} {
		in := summaryFixture()
		in.Documents = in.Documents[1:2]
		in.Documents[0].Authors[0].RoleStatus = status
		r := aggregateBenchmarkSummary(in, summaryFilter())
		if r.FacultyRoles.Unknown != 1 || r.FacultyRoles.Co != 0 {
			t.Fatalf("%s inferred co", status)
		}
	}
	in = summaryFixture()
	in.Documents = in.Documents[1:2]
	in.Documents[0].Authors[0].First = nil
	r = aggregateBenchmarkSummary(in, summaryFilter())
	if r.FacultyRoles.Unknown != 1 {
		t.Fatal("nil flag inferred co")
	}
}
func TestSummaryFiltersAndQuartile(t *testing.T) {
	in := summaryFixture()
	f := summaryFilter()
	r := aggregateBenchmarkSummary(in, f)
	if r.Documents[0].Quartile != "T1" || r.Documents[0].MetricYear == nil || *r.Documents[0].MetricYear != 2025 || !r.Documents[0].MetricFallback {
		t.Fatal("previous complete metric/T1")
	}
	f.QuartileMode = "q"
	q := aggregateBenchmarkSummary(in, f)
	if q.Documents[0].Quartile != "Q1" || *q.Total.Thailand != *r.Total.Thailand {
		t.Fatal("toggle changes cohort")
	}
	in.Metrics = append(in.Metrics, models.ScopusSourceMetric{SourceID: "j", DocType: "all", MetricYear: 2026, CiteScoreStatus: summaryString("InProgress"), CiteScoreQuartile: summaryString("Q4")}, models.ScopusSourceMetric{SourceID: "j", DocType: "all", MetricYear: 2027, CiteScoreStatus: summaryString("Complete"), CiteScoreQuartile: summaryString("Q4")})
	q = aggregateBenchmarkSummary(in, f)
	if q.Documents[0].Quartile != "Q1" {
		t.Fatal("in progress or future metric used")
	}
	in.Metrics = append(in.Metrics, models.ScopusSourceMetric{SourceID: "j", DocType: "all", MetricYear: 2026, CiteScoreStatus: summaryString("Complete"), CiteScoreQuartile: summaryString("Q2")})
	q = aggregateBenchmarkSummary(in, f)
	if q.Documents[0].Quartile != "Q2" || q.Documents[0].MetricFallback {
		t.Fatal("same year precedence")
	}
	in.Documents[0].Type = "Conference Proceeding"
	in.Documents[1].CategoryID = 0
	in.Documents[2].Confidence = "Low"
	in.Documents[3].Confidence = "unknown"
	q = aggregateBenchmarkSummary(in, summaryFilter())
	if *q.Total.Thailand != 1 {
		t.Fatal("default filtering")
	}
	f.Types = []string{"all"}
	f.Category = "all"
	f.Confidence = []string{"High", "Medium", "Low", "unknown"}
	q = aggregateBenchmarkSummary(in, f)
	if *q.Total.Thailand != 4 || q.Documents[0].Quartile != "not_applicable" {
		t.Fatal("non journal/missing category/low filters")
	}
	if summaryPct(0, 0) != nil {
		t.Fatal("zero denominator")
	}
}
func TestSummaryAllFiveAffiliationsAndTwoFacultyAffiliations(t *testing.T) {
	for af := range summaryKKU {
		in := summaryFixture()
		in.Documents = in.Documents[:1]
		in.Documents[0].Afids = []string{af}
		in.Documents[0].Authors = nil
		r := aggregateBenchmarkSummary(in, summaryFilter())
		if *r.Total.KKU != 1 || *r.Total.COC != 0 {
			t.Fatal(af)
		}
	}
	for af := range summaryCOC {
		in := summaryFixture()
		in.Documents = in.Documents[:1]
		in.Documents[0].Afids = []string{af}
		in.Documents[0].Authors = in.Documents[0].Authors[:1]
		in.Documents[0].Authors[0].Afids = []string{"other", af}
		r := aggregateBenchmarkSummary(in, summaryFilter())
		if *r.Total.COC != 1 {
			t.Fatal(af)
		}
	}
}
func TestSummaryValidation(t *testing.T) {
	for _, q := range []url.Values{{"year_from": {"2027"}, "year_to": {"2026"}}, {"confidence": {"NeedsReview"}}, {"types": {""}}, {"category": {"-1"}}, {"quartile_mode": {"future"}}, {"year_from": {"abc"}}} {
		if _, e := ParseBenchmarkSummaryFilter(q, time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)); e == nil {
			t.Fatalf("accepted %+v", q)
		}
	}
}
func TestSummaryRevisionAndExcelNumericParity(t *testing.T) {
	in := summaryFixture()
	r := aggregateBenchmarkSummary(in, summaryFilter())
	same := aggregateBenchmarkSummary(in, summaryFilter())
	if r.Revision != same.Revision {
		t.Fatal("time invalidates revision")
	}
	in.Documents[0].Authors[0].Corresponding = summaryBool(false)
	changed := aggregateBenchmarkSummary(in, summaryFilter())
	if r.Revision == changed.Revision {
		t.Fatal("role update does not change revision")
	}
	for _, view := range []string{"overview", "faculty"} {
		b, e := BuildBenchmarkSummaryExcel(r, view)
		if e != nil {
			t.Fatal(e)
		}
		f, e := excelize.OpenReader(bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		sheet, col := "รายปี", "D4"
		expected := "4"
		if view == "faculty" {
			sheet, col, expected = "บทบาทอาจารย์", "D2", "1"
		}
		v, e := f.GetCellValue(sheet, col)
		if e != nil || v != expected {
			t.Fatalf("%s %s %q %v", view, col, v, e)
		}
		typ, _ := f.GetCellType(sheet, col)
		if typ == excelize.CellTypeInlineString || typ == excelize.CellTypeSharedString {
			t.Fatal("count is not numeric")
		}
		rev, _ := f.GetCellValue("คำอธิบาย", "B4")
		if rev != r.Revision {
			t.Fatal("revision not exported")
		}
		if view == "faculty" {
			noID, _ := f.GetCellValue("บทบาทอาจารย์", "D4")
			if noID != "-" {
				t.Fatal("unlinked faculty exported as a real zero")
			}
		}
		f.Close()
	}
}
func TestSummaryPreservesIngestClassification(t *testing.T) {
	id := uint64(7)
	old := models.ScopusBenchmarkDocument{Category: &id, ClassificationConfidence: summaryString("Medium"), ClassificationModel: summaryString("model"), ClassificationTaxonomyVersion: summaryString("version")}
	newDoc := models.ScopusBenchmarkDocument{}
	preserveBenchmarkClassification(&newDoc, &old)
	if newDoc.Category != old.Category || newDoc.ClassificationConfidence != old.ClassificationConfidence || newDoc.ClassificationModel != old.ClassificationModel || newDoc.ClassificationTaxonomyVersion != old.ClassificationTaxonomyVersion {
		t.Fatal("ingest erased classification")
	}
}
