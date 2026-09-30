package services

// These aggregates share the report's snapshot, cohort rules and applied filters.
// The browser receives counts, never the complete pre-filter document roster.
type SummaryFilterStep struct {
	SummaryCountRow
	Key              string                 `json:"key"`
	ThailandRetained *float64               `json:"thailand_retained"`
	KKURetained      *float64               `json:"kku_retained"`
	COCRetained      *float64               `json:"coc_retained"`
	Filters          BenchmarkSummaryFilter `json:"filters"`
}

type SummaryPresentation struct {
	Steps      []SummaryFilterStep `json:"steps"`
	Categories []SummaryCountRow   `json:"categories"`
	Quartiles  []SummaryCountRow   `json:"quartiles"`
}

func uniqueSummaryDocuments(docs []SummaryDocument, f BenchmarkSummaryFilter) []SummaryDocument {
	seen := map[string]bool{}
	out := make([]SummaryDocument, 0, len(docs))
	for _, d := range docs {
		if d.Year < f.YearFrom || d.Year > f.YearTo {
			continue
		}
		if d.EID != "" {
			if seen[d.EID] {
				continue
			}
			seen[d.EID] = true
		}
		out = append(out, d)
	}
	return out
}

func summaryCohortMembership(d SummaryDocument, facultyIDs map[string][]int) (kku, coc bool) {
	for _, af := range d.Afids {
		kku = kku || summaryKKU[af]
	}
	if !kku {
		return
	}
	for _, a := range d.Authors {
		if len(facultyIDs[normalizeScopusID(a.ScopusAuthorID)]) == 0 {
			continue
		}
		for _, af := range a.Afids {
			coc = coc || summaryCOC[af]
		}
	}
	return
}

func retainedCount(n, base *int) *float64 {
	if n == nil || base == nil {
		return nil
	}
	return summaryPct(*n, *base)
}

func buildSummaryPresentation(base []SummaryDocument, r *BenchmarkSummaryReport, cats []SummaryCategory, available bool) *SummaryPresentation {
	p := &SummaryPresentation{Steps: []SummaryFilterStep{}, Categories: []SummaryCountRow{}, Quartiles: []SummaryCountRow{}}
	allConfidence := []string{"High", "Medium", "Low", "Preface", "unknown"}
	f := r.Filters
	f.ReportView = "" // Drilldowns read the stage cohort without calculating another presentation.
	f.Types, f.Category, f.Confidence = []string{"all"}, "all", allConfidence
	stageFilters := []BenchmarkSummaryFilter{f}
	f.Types = r.Filters.Types
	stageFilters = append(stageFilters, f)
	f.Category = r.Filters.Category
	stageFilters = append(stageFilters, f)
	f.Confidence = r.Filters.Confidence
	stageFilters = append(stageFilters, f)
	labels := []string{"ก่อนกรอง", "หลังกรองประเภทผลงาน", "หลังกรอง Category", "หลังกรอง Confidence"}
	keys := []string{"base", "types", "category", "confidence"}
	baseline := summaryRow(labels[0], base, available)
	for i, filter := range stageFilters {
		docs := []SummaryDocument{}
		for _, d := range base {
			if summaryPasses(d, filter) {
				docs = append(docs, d)
			}
		}
		row := summaryRow(labels[i], docs, available)
		p.Steps = append(p.Steps, SummaryFilterStep{SummaryCountRow: row, Key: keys[i], Filters: filter, ThailandRetained: retainedCount(row.Thailand, baseline.Thailand), KKURetained: retainedCount(row.KKU, baseline.KKU), COCRetained: retainedCount(row.COC, baseline.COC)})
	}
	ordered := append([]SummaryCategory{}, cats...)
	if r.Filters.Category == "all" || r.Filters.Category == "unknown" {
		ordered = append(ordered, SummaryCategory{Name: "ไม่มี Category"})
	}
	for _, cat := range ordered {
		if !summaryPassesCategory(SummaryDocument{CategoryID: cat.ID}, r.Filters.Category) {
			continue
		}
		docs := []SummaryDocument{}
		for _, d := range r.Documents {
			if d.CategoryID == cat.ID {
				docs = append(docs, d)
			}
		}
		row := summaryRow(cat.Name, docs, available)
		row.CategoryID = cat.ID
		p.Categories = append(p.Categories, row)
	}
	buckets := []string{"Q1", "Q2", "Q3", "Q4", "missing", "not_applicable"}
	if r.Filters.QuartileMode == "t1" {
		buckets = append([]string{"T1"}, buckets...)
	}
	for _, bucket := range buckets {
		docs := []SummaryDocument{}
		for _, d := range r.Documents {
			if d.Quartile == bucket {
				docs = append(docs, d)
			}
		}
		row := summaryRow(bucket, docs, available)
		row.Quartile = bucket
		p.Quartiles = append(p.Quartiles, row)
	}
	return p
}
