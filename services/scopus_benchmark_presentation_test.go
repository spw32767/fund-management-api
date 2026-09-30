package services

import (
	"bytes"
	"github.com/xuri/excelize/v2"
	"net/url"
	"testing"
	"time"
)

func TestPresentationStagesUseOneNestedCohortAndBaselineRetention(t *testing.T) {
	in := summaryFixture()
	extra := in.Documents[0]
	extra.EID = "book"
	extra.Type = "Book"
	in.Documents = append(in.Documents, extra)
	extra.EID = "uncategorized"
	extra.Type = "Journal"
	extra.CategoryID = 0
	in.Documents = append(in.Documents, extra)
	extra.EID = "low"
	extra.CategoryID = 1
	extra.Confidence = "Low"
	in.Documents = append(in.Documents, extra)
	// Duplicate EID and out-of-range membership cannot inflate any step.
	in.Documents = append(in.Documents, in.Documents[0])
	extra.EID = "outside-years"
	extra.Year = 2024
	in.Documents = append(in.Documents, extra)
	f := summaryFilter()
	f.ReportView = "presentation"
	r := aggregateBenchmarkSummary(in, f)
	expected := [][3]int{{7, 6, 5}, {6, 5, 4}, {5, 4, 3}, {4, 3, 2}}
	for i, step := range r.Presentation.Steps {
		if *step.Thailand != expected[i][0] || *step.KKU != expected[i][1] || *step.COC != expected[i][2] {
			t.Fatalf("step %d: %+v", i, step)
		}
		if *step.COC > *step.KKU || *step.KKU > *step.Thailand {
			t.Fatal("cohort is not nested")
		}
	}
	if *r.Presentation.Steps[3].ThailandRetained != 4.0/7*100 {
		t.Fatal("retained percentage is not from baseline")
	}
	last := r.Presentation.Steps[3]
	if *last.Thailand != *r.Total.Thailand || *last.KKU != *r.Total.KKU || *last.COC != *r.Total.COC {
		t.Fatal("last step differs from report")
	}
	if *r.Presentation.Categories[0].Thailand != 4 {
		t.Fatal("category not combined")
	}
	sum := 0
	for _, row := range r.Presentation.Quartiles {
		sum += *row.Thailand
	}
	if sum != 4 {
		t.Fatal("quartile total differs")
	}
	// Every clickable stage's filter must reproduce its counts in the existing drilldown.
	for _, step := range r.Presentation.Steps {
		stage := aggregateBenchmarkSummary(in, step.Filters)
		if *stage.Total.Thailand != *step.Thailand || *stage.Total.KKU != *step.KKU || *stage.Total.COC != *step.COC {
			t.Fatalf("stage drilldown differs: %s", step.Key)
		}
	}
	// Changes excluded by applied filters must still invalidate the presentation export.
	before := r.Revision
	extra.EID = "another-book"
	extra.Year = 2026
	extra.Type = "Book"
	in.Documents = append(in.Documents, extra)
	if aggregateBenchmarkSummary(in, f).Revision == before {
		t.Fatal("baseline changes do not invalidate export")
	}
}

func TestPresentationFiltersMissingYearsAndExcel(t *testing.T) {
	f := summaryFilter()
	f.ReportView = "presentation"
	empty := summaryFixture()
	empty.Documents = nil
	empty.Years = []SummaryYearState{{Year: 2025, Status: "missing"}, {Year: 2026, Status: "missing"}}
	r := aggregateBenchmarkSummary(empty, f)
	if r.Presentation.Steps[0].Thailand != nil || r.Presentation.Steps[0].ThailandRetained != nil {
		t.Fatal("missing is not zero")
	}
	in := summaryFixture()
	in.Documents = nil
	r = aggregateBenchmarkSummary(in, f)
	if *r.Presentation.Steps[0].Thailand != 0 || r.Presentation.Steps[0].ThailandRetained != nil {
		t.Fatal("zero denominator")
	}
	in = summaryFixture()
	f.Types = []string{"Book"}
	r = aggregateBenchmarkSummary(in, f)
	if *r.Total.Thailand != 0 || *r.Presentation.Steps[0].Thailand != 4 {
		t.Fatal("types alter before-filter count")
	}
	f = summaryFilter()
	f.ReportView = "presentation"
	r = aggregateBenchmarkSummary(in, f)
	blob, err := BuildBenchmarkSummaryExcel(r, "presentation")
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for _, check := range []struct{ sheet, cell, want string }{{"ผลกระทบการกรอง", "B5", "4"}, {"ผลกระทบการกรอง", "E5", "1"}, {"Category รวมช่วงปี", "B2", "4"}, {"Category รวมช่วงปี", "F2", "0.5"}, {"Quartile รวมช่วงปี", "B2", "1"}} {
		value, e := book.GetCellValue(check.sheet, check.cell, excelize.Options{RawCellValue: true})
		if e != nil || value != check.want {
			t.Fatalf("%s %s=%s want %s: %v", check.sheet, check.cell, value, check.want, e)
		}
	}
	q := f
	q.QuartileMode = "q"
	qr := aggregateBenchmarkSummary(in, q)
	if *qr.Total.Thailand != *r.Total.Thailand || qr.Presentation.Quartiles[0].Quartile != "Q1" {
		t.Fatal("T1 toggle changes cohort")
	}
	if r.Presentation.Quartiles[0].Quartile != "T1" {
		t.Fatal("missing T1 row")
	}
	_, err = ParseBenchmarkSummaryFilter(url.Values{"report_view": {"bad"}}, time.Now())
	if err == nil {
		t.Fatal("invalid report_view accepted")
	}
	plain := aggregateBenchmarkSummary(in, summaryFilter())
	if plain.Presentation != nil {
		t.Fatal("hidden presentation calculated")
	}
}
