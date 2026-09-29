package services

import (
	"encoding/json"
	"fmt"
	"github.com/xuri/excelize/v2"
)

func BuildBenchmarkSummaryExcel(r *BenchmarkSummaryReport, view string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetName("Sheet1", "คำอธิบาย")
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"315A80"}}})
	if err != nil {
		return nil, err
	}
	pctStyle, err := f.NewStyle(&excelize.Style{NumFmt: 10})
	if err != nil {
		return nil, err
	}
	write := func(sheet string, rows [][]interface{}, percentColumns []int) error {
		if _, e := f.NewSheet(sheet); e != nil {
			return e
		}
		for i, row := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if e := f.SetSheetRow(sheet, cell, &row); e != nil {
				return e
			}
		}
		if len(rows) > 0 {
			end, _ := excelize.CoordinatesToCellName(len(rows[0]), 1)
			if e := f.SetCellStyle(sheet, "A1", end, style); e != nil {
				return e
			}
			if e := f.SetColWidth(sheet, "A", end[:len(end)-1], 18); e != nil {
				return e
			}
		}
		_ = f.SetColWidth(sheet, "A", "A", 52)
		for _, col := range percentColumns {
			start, _ := excelize.CoordinatesToCellName(col, 2)
			end, _ := excelize.CoordinatesToCellName(col, len(rows))
			if len(rows) > 1 {
				if e := f.SetCellStyle(sheet, start, end, pctStyle); e != nil {
					return e
				}
			}
		}
		return f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	}
	countValue := func(n *int) interface{} {
		if n == nil {
			return "-"
		}
		return *n
	}
	pctValue := func(n *float64) interface{} {
		if n == nil {
			return "-"
		}
		return *n / 100
	}
	countRows := func(data []SummaryCountRow, group string) [][]interface{} {
		rows := [][]interface{}{{group, "ปี", "Quartile", "Thailand", "KKU", "% KKU", "COC", "% COC"}}
		for _, row := range data {
			var year interface{} = ""
			if row.Year != 0 {
				year = row.Year
			}
			rows = append(rows, []interface{}{row.Label, year, row.Quartile, countValue(row.Thailand), countValue(row.KKU), pctValue(row.KKUPct), countValue(row.COC), pctValue(row.COCPct)})
		}
		return rows
	}
	if view == "faculty" {
		rows := [][]interface{}{{"อาจารย์", "Scopus ID", "เชื่อมได้", "ทั้งหมด", "First", "Corresponding", "First หรือ Corresponding", "Co-author", "ยังระบุไม่ได้", "% First", "% Corresponding", "% First หรือ Corresponding", "% Co-author", "% ยังระบุไม่ได้"}}
		for _, u := range r.Faculty {
			roleCells := []interface{}{u.Total, u.First, u.Corresponding, u.Lead, u.Co, u.Unknown}
			if !u.Linkable {
				for i := range roleCells {
					roleCells[i] = "-"
				}
			}
			row := append([]interface{}{u.Name, u.ScopusID, u.Linkable}, roleCells...)
			row = append(row, pctValue(u.FirstPct), pctValue(u.CorrespondingPct), pctValue(u.LeadPct), pctValue(u.CoPct), pctValue(u.UnknownPct))
			rows = append(rows, row)
		}
		if err = write("บทบาทอาจารย์", rows, []int{10, 11, 12, 13, 14}); err != nil {
			return nil, err
		}
	} else {
		rows := append([]SummaryCountRow{}, r.Yearly...)
		rows = append(rows, r.Total)
		if err = write("รายปี", countRows(rows, "รายการ"), []int{6, 8}); err != nil {
			return nil, err
		}
		if err = write("Category รายปี", countRows(r.Categories, "Category"), []int{6, 8}); err != nil {
			return nil, err
		}
		if err = write("Quartile ราย Category", countRows(r.Quartiles, "Category"), []int{6, 8}); err != nil {
			return nil, err
		}
		c := r.FacultyRoles
		rowsRole := [][]interface{}{{"ทั้งหมด", "First", "Corresponding", "First หรือ Corresponding", "Co-author เท่านั้น", "ยังสรุปไม่ได้"}, {c.Total, c.First, c.Corresponding, c.Lead, c.Co, c.Unknown}}
		if err = write("บทบาทระดับคณะ", rowsRole, nil); err != nil {
			return nil, err
		}
	}
	filters, _ := json.Marshal(r.Filters)
	coverage, _ := json.Marshal(r.Coverage)
	notes := [][]interface{}{
		{"รายการ", "คำอธิบาย"}, {"มุมมอง", view}, {"เวลาสร้าง", r.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")}, {"Revision", r.Revision}, {"ตัวกรอง", string(filters)},
		{"ฐานข้อมูล", "Thailand: benchmark country_thailand scope และ membership ปีรายงาน นับ EID ไม่ซ้ำ"},
		{"KKU AF-ID", "60017165, 60280609, 60026046, 60277695, 109899034"},
		{"COC", "ผู้เขียนตรงกับทะเบียน users role 1/4/5 ไม่ถูกลบ ไม่ใช่บัญชีทดสอบ และผู้เขียนนั้นมี AF-ID 60017165 หรือ 60280609 ไม่กรองวันเริ่มงาน"},
		{"เปอร์เซ็นต์", "%KKU=KKU/Thailand; %COC=COC/KKU; ตัวหาร 0 แสดงขีด; ยอดรวมไม่เฉลี่ยเปอร์เซ็นต์"},
		{"Quartile", "Complete ปีตีพิมพ์ หรือ Complete ล่าสุดก่อนปีตีพิมพ์; doc_type=all; T1 percentile 90–100; non-Journal ไม่ใช้ Quartile"},
		{"บทบาท", "อ่าน XML จากตารางหลักด้วย EID และ Author ID; first/corresponding ซ้อนกันได้; co เมื่อสอง flags false บน complete/no_correspondence; อื่นๆ ยังระบุไม่ได้"},
		{"ข้อจำกัด", "บทบาทไม่ใช่คะแนนปริมาณงาน; ไม่แยก co-first/co-corresponding; no_correspondence ใช้กติกาที่ตกลงไว้ มิใช่หลักฐานว่าไม่มี corresponding"},
		{"หน่วย", "ภาพรวมนับผลงานไม่ซ้ำ; รายบุคคลนับ user_id/EID ยอดรวมรายบุคคลอาจมากกว่าภาพรวม"},
		{"ความครบถ้วน", string(coverage)},
		{"ข้อจำกัดข้อมูล", "แสดงจากข้อมูลที่มีใน DB ที่เชื่อมอยู่; ปี missing ไม่ใช่ 0 และข้อมูล dev ไม่แสดงสถานะ production"},
	}
	for _, y := range r.Years {
		notes = append(notes, []interface{}{fmt.Sprintf("สถานะปี %d", y.Year), fmt.Sprintf("%s; observed=%d; subject=%s", y.Status, y.Observed, y.SubjectArea)})
	}
	for i, row := range notes {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err = f.SetSheetRow("คำอธิบาย", cell, &row); err != nil {
			return nil, err
		}
	}
	_ = f.SetColWidth("คำอธิบาย", "A", "A", 28)
	_ = f.SetColWidth("คำอธิบาย", "B", "B", 110)
	_ = f.SetCellStyle("คำอธิบาย", "A1", "B1", style)
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
