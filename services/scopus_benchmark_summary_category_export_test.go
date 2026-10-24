package services

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestBenchmarkCategorySummaryExportLayoutAndSnapshotParity(t *testing.T) {
	r := aggregateBenchmarkSummary(summaryFixture(), summaryFilter())
	b, err := BuildBenchmarkSummaryExcel(r, "overview")
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, name := range f.GetSheetList() {
		if name == "Request" || name == "บทบาทระดับคณะ" {
			t.Fatalf("unrelated or unclear sheet remains in overview export: %s", name)
		}
	}
	for address, want := range map[string]string{
		"A1": "จำนวนเอกสารตามปี", "A2": "ปี", "B2": "Thailand", "C2": "KKU", "D2": "COC",
		"A3": "2025", "B3": "—", "B4": "4", "C4": "3", "D4": "2", "A5": "รวม",
		"F1": "Category  ปี 2025", "F2": "Category", "G2": "Thailand", "H2": "KKU", "I2": "% KKU", "J2": "COC", "K2": "%COC",
		"G3": "—", "F4": "รวม", "F6": "Category  ปี 2026", "G8": "4", "H8": "3", "I8": "75.0%", "J8": "2", "K8": "66.7%",
		"F11": "AI (4)", "F12": "ปี 2025–2026 · แยก T1", "F13": "Quartile", "F14": "T1", "G14": "1", "F19": "ไม่มีข้อมูล Quartile", "F20": "ไม่ถูกนำมาจัดอันดับ",
		"F21": "รวม", "G21": "4", "H21": "3", "I21": "75.0%", "J21": "2", "K21": "66.7%",
	} {
		got, e := f.GetCellValue("สรุป Category และ Quartile", address)
		if e != nil || got != want {
			t.Errorf("สรุป Category และ Quartile!%s=%q, want %q (%v)", address, got, want, e)
		}
	}
	for _, address := range []string{"G8", "H8", "J8", "G21", "H21", "J21"} {
		typ, _ := f.GetCellType("สรุป Category และ Quartile", address)
		if typ == excelize.CellTypeInlineString || typ == excelize.CellTypeSharedString {
			t.Errorf("count %s is text", address)
		}
	}
	merges, _ := f.GetMergeCells("สรุป Category และ Quartile")
	if len(merges) < 4 {
		t.Fatal("missing section titles")
	}
	panes, _ := f.GetPanes("สรุป Category และ Quartile")
	if !panes.Freeze || panes.YSplit != 2 {
		t.Fatal("template headers not frozen")
	}
	if f.GetSheetName(f.GetActiveSheetIndex()) != "สรุป Category และ Quartile" {
		t.Fatal("workbook does not open on the category summary")
	}
}

func TestBenchmarkCategorySummaryExportQuartileModeAndUnavailableTotals(t *testing.T) {
	filters := summaryFilter()
	filters.QuartileMode = "q"
	r := aggregateBenchmarkSummary(summaryFixture(), filters)
	b, err := BuildBenchmarkSummaryExcel(r, "overview")
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := f.GetRows("สรุป Category และ Quartile")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if len(row) > 5 && row[5] == "T1" {
			t.Fatal("Q1–Q4 export incorrectly contains T1")
		}
	}
	joined := ""
	for _, row := range rows {
		joined += strings.Join(row, "|") + "\n"
	}
	if !strings.Contains(joined, "Q1–Q4") {
		t.Fatal("quartile mode not stated")
	}
	missing := sumBenchmarkCategorySummaryRows([]SummaryCountRow{{}, {}})
	if missing.Thailand != nil || missing.KKUPct != nil || missing.COCPct != nil {
		t.Fatal("missing data became zero")
	}
	intPtr := func(n int) *int { return &n }
	weighted := sumBenchmarkCategorySummaryRows([]SummaryCountRow{{Thailand: intPtr(10), KKU: intPtr(5), COC: intPtr(1)}, {Thailand: intPtr(90), KKU: intPtr(9), COC: intPtr(9)}})
	if *weighted.Thailand != 100 || *weighted.KKU != 14 || *weighted.COC != 10 || *weighted.KKUPct != 14 {
		t.Fatal("totals are not based on counts")
	}
	zero := sumBenchmarkCategorySummaryRows([]SummaryCountRow{{Thailand: intPtr(0), KKU: intPtr(0), COC: intPtr(0)}})
	if zero.Thailand == nil || *zero.Thailand != 0 || zero.KKUPct != nil || zero.COCPct != nil {
		t.Fatal("zero denominator is not preserved")
	}
}
