package services

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// The category summary follows the supplied workbook: yearly counts at A:D, followed by
// category/year and category/quartile tables stacked at F:K. Values always
// come from the report snapshot, never the historical example workbook.
func writeBenchmarkCategorySummarySheet(f *excelize.File, r *BenchmarkSummaryReport) error {
	const sheet = "สรุป Category และ Quartile"
	index, err := f.NewSheet(sheet)
	if err != nil {
		return err
	}
	makeStyle := func(style excelize.Style) int {
		if err != nil {
			return 0
		}
		var id int
		id, err = f.NewStyle(&style)
		return id
	}
	borders := []excelize.Border{{Type: "left", Color: "CBD5E1", Style: 1}, {Type: "right", Color: "CBD5E1", Style: 1}, {Type: "top", Color: "CBD5E1", Style: 1}, {Type: "bottom", Color: "CBD5E1", Style: 1}}
	body := makeStyle(excelize.Style{Font: &excelize.Font{Family: "Calibri", Size: 11}, Border: borders, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	number := makeStyle(excelize.Style{NumFmt: 3, Border: borders, Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"}})
	percentFormat := "0.0%"
	percent := makeStyle(excelize.Style{CustomNumFmt: &percentFormat, Border: borders, Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"}})
	header := makeStyle(excelize.Style{Font: &excelize.Font{Bold: true, Color: "17365D", Size: 11}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"D9EAF7"}}, Border: borders, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	title := makeStyle(excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF", Size: 12}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"315A80"}}, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	totalText := makeStyle(excelize.Style{Font: &excelize.Font{Bold: true, Size: 11}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EAF2F8"}}, Border: borders, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	totalNumber := makeStyle(excelize.Style{NumFmt: 3, Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EAF2F8"}}, Border: borders, Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"}})
	totalPercent := makeStyle(excelize.Style{CustomNumFmt: &percentFormat, Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EAF2F8"}}, Border: borders, Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"}})
	check := func(e error) {
		if err == nil {
			err = e
		}
	}
	cell := func(col, row int) string { name, _ := excelize.CoordinatesToCellName(col, row); return name }
	writeRow := func(col, row int, values []interface{}, style int) {
		check(f.SetSheetRow(sheet, cell(col, row), &values))
		check(f.SetCellStyle(sheet, cell(col, row), cell(col+len(values)-1, row), style))
		check(f.SetRowHeight(sheet, row, 32))
	}
	section := func(col, row, width int, label string) {
		writeRow(col, row, []interface{}{label}, title)
		check(f.MergeCell(sheet, cell(col, row), cell(col+width-1, row)))
		check(f.SetCellStyle(sheet, cell(col, row), cell(col+width-1, row), title))
	}
	count := func(n *int) interface{} {
		if n == nil {
			return "—"
		}
		return *n
	}
	pct := func(n *float64) interface{} {
		if n == nil {
			return "—"
		}
		return *n / 100
	}
	counts := func(row SummaryCountRow, label string, line int, total bool) {
		textStyle, numberStyle, pctStyle := body, number, percent
		if total {
			textStyle, numberStyle, pctStyle = totalText, totalNumber, totalPercent
		}
		writeRow(6, line, []interface{}{label, count(row.Thailand), count(row.KKU), pct(row.KKUPct), count(row.COC), pct(row.COCPct)}, textStyle)
		for _, col := range []int{7, 8, 10} {
			check(f.SetCellStyle(sheet, cell(col, line), cell(col, line), numberStyle))
		}
		for _, col := range []int{9, 11} {
			check(f.SetCellStyle(sheet, cell(col, line), cell(col, line), pctStyle))
		}
	}
	section(1, 1, 4, "จำนวนเอกสารตามปี")
	writeRow(1, 2, []interface{}{"ปี", "Thailand", "KKU", "COC"}, header)
	for i, y := range r.Yearly {
		writeRow(1, i+3, []interface{}{y.Year, count(y.Thailand), count(y.KKU), count(y.COC)}, number)
		check(f.SetCellStyle(sheet, cell(1, i+3), cell(1, i+3), body))
	}
	line := len(r.Yearly) + 3
	writeRow(1, line, []interface{}{"รวม", count(r.Total.Thailand), count(r.Total.KKU), count(r.Total.COC)}, totalNumber)
	check(f.SetCellStyle(sheet, cell(1, line), cell(1, line), totalText))
	categoryLabel := map[string]string{"all": "ทั้งหมด", "classified": "มี Category", "unknown": "ยังไม่จัด Category"}[r.Filters.Category]
	if categoryLabel == "" {
		categoryLabel = r.Filters.Category
		for _, row := range r.Categories {
			if fmt.Sprint(row.CategoryID) == r.Filters.Category {
				categoryLabel = row.Label
				break
			}
		}
	}
	confidence := strings.ReplaceAll(strings.Join(r.Filters.Confidence, ", "), "unknown", "ไม่ระบุ")
	for i, text := range []string{
		fmt.Sprintf("ช่วงปี ค.ศ. %d–%d", r.Filters.YearFrom, r.Filters.YearTo),
		"ประเภทผลงาน: " + strings.Join(r.Filters.Types, ", "),
		"Category: " + categoryLabel,
		"Confidence: " + confidence,
		"Quartile: " + map[string]string{"t1": "แยก T1", "q": "Q1–Q4"}[r.Filters.QuartileMode],
		"% KKU = KKU / Thailand · %COC = COC / KKU",
		"— หมายถึงไม่มีข้อมูล หรือไม่มีตัวหารสำหรับร้อยละ",
	} {
		row := line + i + 2
		writeRow(1, row, []interface{}{text}, body)
		check(f.MergeCell(sheet, cell(1, row), cell(4, row)))
	}
	headers := []interface{}{"Category", "Thailand", "KKU", "% KKU", "COC", "%COC"}
	line = 1
	for _, year := range r.Yearly {
		section(6, line, 6, fmt.Sprintf("Category  ปี %d", year.Year))
		writeRow(6, line+1, headers, header)
		line += 2
		rows := []SummaryCountRow{}
		for _, row := range r.Categories {
			if row.Year == year.Year {
				rows = append(rows, row)
			}
		}
		// The reference lists the largest Thailand category first in each year.
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Thailand == nil {
				return false
			}
			if rows[j].Thailand == nil {
				return true
			}
			return *rows[i].Thailand > *rows[j].Thailand
		})
		for _, row := range rows {
			counts(row, row.Label, line, false)
			line++
		}
		counts(year, "รวม", line, true)
		line += 2
	}
	groups := map[uint64][]SummaryCountRow{}
	order := []uint64{}
	for _, row := range r.Quartiles {
		if _, exists := groups[row.CategoryID]; !exists {
			order = append(order, row.CategoryID)
		}
		groups[row.CategoryID] = append(groups[row.CategoryID], row)
	}
	// Preserve the reference's sequence; additional categories follow report order.
	reference := []string{"Applied AI, GeoAI and Agentic Applications", "Networks, Security, and Distributed Systems", "Quantum Information Science", "AI Algorithms and Intelligent Systems", "Educational Technology, Learning Sciences, and Digital Library Systems", "Theoretical Computer Science", "Computer Engineering, IoT, and Embedded Systems"}
	rank := func(id uint64) int {
		for i, name := range reference {
			if groups[id][0].Label == name {
				return i
			}
		}
		return len(reference)
	}
	sort.SliceStable(order, func(i, j int) bool { return rank(order[i]) < rank(order[j]) })
	for _, id := range order {
		rows := groups[id]
		total := sumBenchmarkCategorySummaryRows(rows)
		section(6, line, 6, fmt.Sprintf("%s (%v)", rows[0].Label, count(total.Thailand)))
		line++
		writeRow(6, line, []interface{}{fmt.Sprintf("ปี %d–%d · %s", r.Filters.YearFrom, r.Filters.YearTo, map[string]string{"t1": "แยก T1", "q": "Q1–Q4"}[r.Filters.QuartileMode])}, body)
		check(f.MergeCell(sheet, cell(6, line), cell(11, line)))
		line++
		writeRow(6, line, []interface{}{"Quartile", "Thailand", "KKU", "% KKU", "COC", "%COC"}, header)
		line++
		for _, row := range rows {
			label := row.Quartile
			if label == "missing" {
				label = "ไม่มีข้อมูล Quartile"
			}
			if label == "not_applicable" {
				label = "ไม่ถูกนำมาจัดอันดับ"
			}
			counts(row, label, line, false)
			line++
		}
		counts(total, "รวม", line, true)
		line += 2
	}
	for _, col := range []string{"A", "B", "C", "D", "G", "H", "I", "J", "K"} {
		check(f.SetColWidth(sheet, col, col, 15))
	}
	check(f.SetColWidth(sheet, "E", "E", 4))
	check(f.SetColWidth(sheet, "F", "F", 62))
	check(f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft"}))
	f.SetActiveSheet(index)
	return err
}

func sumBenchmarkCategorySummaryRows(rows []SummaryCountRow) SummaryCountRow {
	sum := func(get func(SummaryCountRow) *int) *int {
		var result *int
		for _, row := range rows {
			if n := get(row); n != nil {
				if result == nil {
					result = new(int)
				}
				*result += *n
			}
		}
		return result
	}
	r := SummaryCountRow{Thailand: sum(func(row SummaryCountRow) *int { return row.Thailand }), KKU: sum(func(row SummaryCountRow) *int { return row.KKU }), COC: sum(func(row SummaryCountRow) *int { return row.COC })}
	if r.Thailand != nil && r.KKU != nil {
		r.KKUPct = summaryPct(*r.KKU, *r.Thailand)
	}
	if r.KKU != nil && r.COC != nil {
		r.COCPct = summaryPct(*r.COC, *r.KKU)
	}
	return r
}
