package main

import (
	"github.com/xuri/excelize/v2"
	"path/filepath"
	"strings"
	"testing"
)

func workbookFixture(t *testing.T, rows [][]interface{}, rawRows [][]interface{}) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", "classified_scopus-benchmark-doc")
	f.NewSheet("raw_data")
	for sheet, data := range map[string][][]interface{}{"classified_scopus-benchmark-doc": rows, "raw_data": rawRows} {
		for i, row := range data {
			axis, _ := excelize.CoordinatesToCellName(1, i+1)
			if err := f.SetSheetRow(sheet, axis, &row); err != nil {
				t.Fatal(err)
			}
		}
	}
	p := filepath.Join(t.TempDir(), "source.xlsx")
	if err := f.SaveAs(p); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestWorkbookClassificationUsesExplicitEIDAndPreservesReview(t *testing.T) {
	rows := [][]interface{}{{"scopus_id", "publication_year", "Category", "classification_confidence"}, {"SCOPUS_ID:123", 2025, "Theoretical Computer Science", "High"}, {"SCOPUS_ID:456", 2026, "", "Needs Review"}}
	raw := [][]interface{}{{"scopus_id", "eid"}, {"SCOPUS_ID:123", "2-s2.0-123"}, {"SCOPUS_ID:456", "2-s2.0-456"}}
	data, total, review, err := readWorkbook(workbookFixture(t, rows, raw))
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || review != 1 || len(data) != 1 || data[0].EID != "2-s2.0-123" || data[0].Confidence != "High" {
		t.Fatalf("%+v total=%d review=%d", data, total, review)
	}
	for _, tc := range []struct {
		name      string
		rows, raw [][]interface{}
		message   string
	}{
		{"unsupported", [][]interface{}{rows[0], {"SCOPUS_ID:123", 2025, "Theoretical Computer Science", "Needs Review"}}, raw, "unsupported classification"},
		{"ambiguous", rows, append(raw, []interface{}{"SCOPUS_ID:123", "2-s2.0-999"}), "ambiguous EID"},
		{"duplicate", append(rows, rows[1]), raw, "duplicate EID"},
		{"missing", rows, raw[:2], "missing raw_data match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := readWorkbook(workbookFixture(t, tc.rows, tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
