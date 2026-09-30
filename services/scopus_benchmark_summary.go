package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"fund-management-api/models"
	"gorm.io/gorm"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var summaryKKU = map[string]bool{"60017165": true, "60280609": true, "60026046": true, "60277695": true, "109899034": true}
var summaryCOC = map[string]bool{"60017165": true, "60280609": true}

type BenchmarkSummaryFilter struct {
	YearFrom     int      `json:"year_from"`
	YearTo       int      `json:"year_to"`
	Types        []string `json:"types"`
	Category     string   `json:"category"`
	Confidence   []string `json:"confidence"`
	QuartileMode string   `json:"quartile_mode"`
}

func ParseBenchmarkSummaryFilter(q url.Values, now time.Time) (BenchmarkSummaryFilter, error) {
	f := BenchmarkSummaryFilter{YearFrom: now.Year() - 1, YearTo: now.Year(), Types: []string{"Journal"}, Category: "classified", Confidence: []string{"High", "Medium", "unknown"}, QuartileMode: "t1"}
	for key, dst := range map[string]*int{"year_from": &f.YearFrom, "year_to": &f.YearTo} {
		if q.Has(key) {
			n, e := strconv.Atoi(q.Get(key))
			if e != nil {
				return f, fmt.Errorf("invalid %s", key)
			}
			*dst = n
		}
	}
	if f.YearFrom < 1900 || f.YearTo > now.Year()+1 || f.YearFrom > f.YearTo || f.YearTo-f.YearFrom >= 60 {
		return f, fmt.Errorf("invalid year range (maximum 60 years)")
	}
	if q.Has("types") {
		f.Types = summaryCSV(q.Get("types"))
		if len(f.Types) == 0 {
			return f, fmt.Errorf("choose at least one type")
		}
	}
	if v := q.Get("category"); v != "" {
		f.Category = v
	}
	if f.Category != "all" && f.Category != "classified" && f.Category != "unknown" {
		n, e := strconv.ParseUint(f.Category, 10, 64)
		if e != nil || n == 0 {
			return f, fmt.Errorf("invalid category")
		}
	}
	if q.Has("confidence") {
		f.Confidence = summaryCSV(q.Get("confidence"))
		if len(f.Confidence) == 0 {
			return f, fmt.Errorf("choose at least one confidence")
		}
	}
	for _, v := range f.Confidence {
		if !summaryContains([]string{"High", "Medium", "Low", "Preface", "unknown"}, v) {
			return f, fmt.Errorf("invalid confidence")
		}
	}
	if v := q.Get("quartile_mode"); v != "" {
		f.QuartileMode = v
	}
	if f.QuartileMode != "t1" && f.QuartileMode != "q" {
		return f, fmt.Errorf("invalid quartile mode")
	}
	for _, v := range f.Types {
		if len(v) > 80 {
			return f, fmt.Errorf("invalid type")
		}
	}
	return f, nil
}
func summaryCSV(v string) []string {
	out := []string{}
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" && !summaryContains(out, s) {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
func summaryContains(a []string, v string) bool {
	for _, s := range a {
		if s == v {
			return true
		}
	}
	return false
}

// Narrow the complete drilldown cohort before the controller paginates it.
// Additional filters never broaden the table cell/year/role that was selected.
func FilterSummaryDocumentList(documents []SummaryDocument, search string, category *uint64, quartile string) []SummaryDocument {
	terms := strings.Fields(strings.ToLower(search))
	matches := make([]SummaryDocument, 0, len(documents))
	for _, d := range documents {
		if category != nil && d.CategoryID != *category || quartile != "" && d.Quartile != quartile {
			continue
		}
		if len(terms) > 0 {
			parts := []string{d.Title, d.EID, d.DOI, d.PublicationName}
			for _, a := range d.Authors {
				parts = append(parts, a.Name, a.ScopusAuthorID)
			}
			haystack := strings.ToLower(strings.Join(parts, " "))
			matched := true
			for _, term := range terms {
				if !strings.Contains(haystack, term) {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
		}
		matches = append(matches, d)
	}
	return matches
}

type SummaryCategory struct {
	ID           uint64 `gorm:"column:category_id" json:"id"`
	Name         string `json:"name"`
	Code         string `json:"code"`
	DisplayOrder int    `json:"display_order"`
}
type SummaryRoleCounts struct {
	Total         int `json:"total"`
	First         int `json:"first"`
	Corresponding int `json:"corresponding"`
	Lead          int `json:"lead"`
	Co            int `json:"co"`
	Unknown       int `json:"unknown"`
}
type SummaryFaculty struct {
	UserID   int    `json:"user_id"`
	Name     string `json:"name"`
	ScopusID string `json:"scopus_id"`
	Linkable bool   `json:"linkable"`
	SummaryRoleCounts
	FirstPct         *float64 `json:"first_pct"`
	CorrespondingPct *float64 `json:"corresponding_pct"`
	LeadPct          *float64 `json:"lead_pct"`
	CoPct            *float64 `json:"co_pct"`
	UnknownPct       *float64 `json:"unknown_pct"`
}
type SummaryAuthor struct {
	AuthorID             uint     `json:"author_id"`
	ScopusAuthorID       string   `json:"scopus_author_id"`
	Name                 string   `json:"name"`
	Seq                  int      `json:"seq"`
	Afids                []string `json:"afids"`
	AffiliationsComplete bool     `json:"affiliations_complete"`
	RoleStatus           string   `json:"role_status"`
	First                *bool    `json:"first"`
	Corresponding        *bool    `json:"corresponding"`
	EligibleUserIDs      []int    `json:"eligible_user_ids"`
}
type SummaryDocument struct {
	ID                   uint              `json:"id"`
	EID                  string            `gorm:"column:eid" json:"eid"`
	Title                string            `json:"title"`
	Year                 int               `json:"year"`
	Type                 string            `json:"type"`
	CategoryID           uint64            `json:"category_id"`
	CategoryName         string            `json:"category_name"`
	Confidence           string            `json:"confidence"`
	SourceID             string            `json:"-"`
	PublicationName      string            `json:"publication_name"`
	DOI                  string            `json:"doi"`
	ScopusLink           string            `json:"scopus_link"`
	Afids                []string          `gorm:"-" json:"afids"`
	AffiliationsComplete bool              `json:"affiliations_complete"`
	Authors              []SummaryAuthor   `gorm:"-" json:"authors"`
	KKU                  bool              `json:"kku"`
	COC                  bool              `json:"coc"`
	Quartile             string            `json:"quartile"`
	MetricYear           *int              `json:"metric_year"`
	MetricFallback       bool              `json:"metric_fallback"`
	FacultyRoles         SummaryRoleCounts `gorm:"-" json:"faculty_roles"`
}
type SummaryCountRow struct {
	Year       int      `json:"year,omitempty"`
	CategoryID uint64   `json:"category_id,omitempty"`
	Label      string   `json:"label"`
	Quartile   string   `json:"quartile,omitempty"`
	Thailand   *int     `json:"thailand"`
	KKU        *int     `json:"kku"`
	COC        *int     `json:"coc"`
	KKUPct     *float64 `json:"kku_pct"`
	COCPct     *float64 `json:"coc_pct"`
}
type SummaryYearState struct {
	Year        int        `json:"year"`
	Status      string     `json:"status"`
	Observed    int        `json:"observed"`
	Expected    *int       `json:"expected"`
	LastHarvest *time.Time `json:"last_harvest"`
	SubjectArea string     `json:"subject_area"`
	ExtraQuery  *string    `json:"extra_query"`
}
type SummaryCoverage struct {
	BaseDocuments                int `json:"base_documents"`
	Classified                   int `json:"classified"`
	Selected                     int `json:"selected"`
	AffiliationIncomplete        int `json:"affiliation_incomplete"`
	FacultyAffiliationIncomplete int `json:"faculty_affiliation_incomplete"`
	RoleUnknownPairs             int `json:"role_unknown_pairs"`
	MetricFallback               int `json:"metric_fallback"`
	MissingQuartile              int `json:"missing_quartile"`
	FacultyWithoutID             int `json:"faculty_without_id"`
}
type BenchmarkSummaryReport struct {
	Filters      BenchmarkSummaryFilter `json:"applied_filters"`
	GeneratedAt  time.Time              `json:"generated_at"`
	Revision     string                 `json:"revision"`
	Years        []SummaryYearState     `json:"year_states"`
	Yearly       []SummaryCountRow      `json:"yearly"`
	Categories   []SummaryCountRow      `json:"categories"`
	Quartiles    []SummaryCountRow      `json:"quartiles"`
	Total        SummaryCountRow        `json:"total"`
	FacultyRoles SummaryRoleCounts      `json:"faculty_roles"`
	Coverage     SummaryCoverage        `json:"coverage"`
	Faculty      []SummaryFaculty       `json:"faculty,omitempty"`
	Documents    []SummaryDocument      `json:"-"`
}
type summaryInput struct {
	Documents  []SummaryDocument
	Faculty    []SummaryFaculty
	Categories []SummaryCategory
	Metrics    []models.ScopusSourceMetric
	Years      []SummaryYearState
}

// All streams inside a response use one REPEATABLE READ snapshot. No network
// calls, writes or hidden backfill run on this path.
func (s *ScopusBenchmarkService) BenchmarkSummary(ctx context.Context, f BenchmarkSummaryFilter) (*BenchmarkSummaryReport, error) {
	var input summaryInput
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return loadSummaryInput(tx, f, &input) }, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return aggregateBenchmarkSummary(input, f), nil
}
func loadSummaryInput(tx *gorm.DB, f BenchmarkSummaryFilter, in *summaryInput) error {
	var scope models.ScopusBenchmarkScope
	found := tx.Where("code='country_thailand' AND level='country' AND LOWER(affil_country)='thailand'").Limit(1).Find(&scope)
	if found.Error != nil {
		return found.Error
	}
	if err := tx.Table("paper_categories").Order("display_order, category_id").Scan(&in.Categories).Error; err != nil {
		return err
	}
	if err := tx.Table("users").Select("user_id,TRIM(CONCAT(COALESCE(user_fname,''),' ',COALESCE(user_lname,''))) AS name,COALESCE(scopus_id,'') AS scopus_id").Where("delete_at IS NULL AND is_test=0 AND role_id IN (1,4,5)").Order("user_id").Scan(&in.Faculty).Error; err != nil {
		return err
	}
	if scope.ID == 0 {
		for y := f.YearFrom; y <= f.YearTo; y++ {
			in.Years = append(in.Years, SummaryYearState{Year: y, Status: "missing"})
		}
		return nil
	}
	base := `SELECT DISTINCT d.id FROM scopus_benchmark_documents d JOIN scopus_benchmark_document_scopes m ON m.document_id=d.id WHERE m.scope_id=? AND m.pub_year BETWEEN ? AND ?`
	args := []interface{}{scope.ID, f.YearFrom, f.YearTo}
	query := tx.Table("scopus_benchmark_documents d").Select(`d.id,d.eid,COALESCE(d.title,'') AS title,m.pub_year AS year,COALESCE(d.aggregation_type,'unknown') AS type,COALESCE(d.category,0) AS category_id,COALESCE(d.classification_confidence,'unknown') AS confidence,COALESCE(d.source_id,'') AS source_id,COALESCE(d.publication_name,'') AS publication_name,COALESCE(d.doi,'') AS doi,COALESCE(d.scopus_link,'') AS scopus_link,d.affiliations_complete`).Joins("JOIN scopus_benchmark_document_scopes m ON m.document_id=d.id AND m.scope_id=?", scope.ID).Where("d.id IN ("+base+")", args...).Order("d.eid")
	if err := query.Scan(&in.Documents).Error; err != nil {
		return err
	}
	var authors []struct {
		DocumentID           uint
		AuthorID             uint
		ScopusAuthorID       string
		Name                 string
		Seq                  int
		AffiliationsComplete bool
		LegacyAfid           string
		RoleStatus           string
		First                *bool
		Corresponding        *bool
	}
	if err := tx.Table("scopus_benchmark_document_authors l").Select(`l.document_id,l.author_id,a.scopus_author_id,COALESCE(a.full_name,'') AS name,COALESCE(l.author_seq,0) AS seq,l.affiliations_complete,COALESCE(af.afid,'') AS legacy_afid,COALESCE(c.author_role_status,'pending') AS role_status,r.is_first_author AS first,r.is_corresponding_author AS corresponding`).Joins("JOIN scopus_benchmark_authors a ON a.id=l.author_id").Joins("LEFT JOIN scopus_benchmark_affiliations af ON af.id=l.affiliation_id").Joins("JOIN scopus_benchmark_documents d ON d.id=l.document_id").Joins("LEFT JOIN scopus_documents c ON c.eid=d.eid").Joins("LEFT JOIN scopus_authors ca ON ca.scopus_author_id=a.scopus_author_id").Joins("LEFT JOIN scopus_document_authors r ON r.document_id=c.id AND r.author_id=ca.id").Where("l.document_id IN ("+base+")", args...).Order("l.document_id,l.author_seq,a.scopus_author_id").Scan(&authors).Error; err != nil {
		return err
	}
	var da []models.BenchmarkDocumentAffiliation
	var aa []models.BenchmarkAuthorAffiliation
	if err := tx.Where("document_id IN ("+base+")", args...).Order("document_id,afid").Find(&da).Error; err != nil {
		return err
	}
	if err := tx.Where("document_id IN ("+base+")", args...).Order("document_id,author_id,afid").Find(&aa).Error; err != nil {
		return err
	}
	docByID := map[uint]*SummaryDocument{}
	for i := range in.Documents {
		d := &in.Documents[i]
		d.Afids = []string{}
		d.Authors = []SummaryAuthor{}
		docByID[d.ID] = d
	}
	afByAuthor := map[[2]uint][]string{}
	for _, a := range aa {
		k := [2]uint{a.DocumentID, a.AuthorID}
		afByAuthor[k] = append(afByAuthor[k], a.Afid)
	}
	for _, a := range da {
		if d := docByID[a.DocumentID]; d != nil {
			d.Afids = append(d.Afids, a.Afid)
		}
	}
	for _, a := range authors {
		d := docByID[a.DocumentID]
		if d == nil {
			continue
		}
		afs := afByAuthor[[2]uint{a.DocumentID, a.AuthorID}]
		if afs == nil {
			afs = []string{}
		}
		if !a.AffiliationsComplete && a.LegacyAfid != "" && !summaryContains(afs, a.LegacyAfid) {
			afs = append(afs, a.LegacyAfid)
		}
		sort.Strings(afs)
		for _, id := range afs {
			if !summaryContains(d.Afids, id) {
				d.Afids = append(d.Afids, id)
			}
		}
		d.Authors = append(d.Authors, SummaryAuthor{AuthorID: a.AuthorID, ScopusAuthorID: a.ScopusAuthorID, Name: a.Name, Seq: a.Seq, Afids: afs, AffiliationsComplete: a.AffiliationsComplete, RoleStatus: a.RoleStatus, First: a.First, Corresponding: a.Corresponding, EligibleUserIDs: []int{}})
	}
	for i := range in.Documents {
		sort.Strings(in.Documents[i].Afids)
	}
	if err := tx.Where("doc_type='all' AND LOWER(cite_score_status)='complete' AND metric_year<=? AND source_id IN (SELECT source_id FROM scopus_benchmark_documents WHERE id IN ("+base+"))", append([]interface{}{f.YearTo}, args...)...).Order("source_id,metric_year,source_metric_id").Find(&in.Metrics).Error; err != nil {
		return err
	}
	var runs []models.ScopusBenchmarkHarvestRun
	var snaps []models.ScopusBenchmarkCountSnapshot
	if err := tx.Where("scope_id=? AND run_type='harvest'", scope.ID).Order("id DESC").Find(&runs).Error; err != nil {
		return err
	}
	if err := tx.Where("scope_id=? AND pub_year BETWEEN ? AND ?", scope.ID, f.YearFrom, f.YearTo).Order("captured_at DESC,id DESC").Find(&snaps).Error; err != nil {
		return err
	}
	for y := f.YearFrom; y <= f.YearTo; y++ {
		state := SummaryYearState{Year: y, Status: "missing", SubjectArea: scope.SubjectArea, ExtraQuery: scope.ExtraQuery}
		for _, d := range in.Documents {
			if d.Year == y {
				state.Observed++
			}
		}
		for _, snap := range snaps {
			if snap.PubYear != nil && *snap.PubYear == y {
				n := snap.TotalResults
				state.Expected = &n
				break
			}
		}
		if state.Observed > 0 {
			state.Status = "partial"
		}
		for _, run := range runs {
			if run.YearFrom != nil && y < *run.YearFrom || run.YearTo != nil && y > *run.YearTo {
				continue
			}
			if run.Status == "running" || run.Status == "cancelling" {
				state.Status = "harvesting"
				break
			}
			if run.Status == "success" {
				state.LastHarvest = run.FinishedAt
				state.Status = "available"
				break
			}
		}
		// Count snapshots describe expected harvest size only; they never supply report totals.
		if state.Status == "available" && state.Expected != nil && state.Observed < *state.Expected {
			state.Status = "partial"
		}
		in.Years = append(in.Years, state)
	}
	return nil
}
func summaryRole(a SummaryAuthor) (first, corr, known bool) {
	known = (a.RoleStatus == "complete" || a.RoleStatus == "no_correspondence") && a.First != nil && a.Corresponding != nil
	if known {
		first = *a.First
		corr = *a.Corresponding
	}
	return
}
func summaryBucket(d SummaryDocument, metrics []models.ScopusSourceMetric, mode string) (string, *int, bool) {
	if !strings.EqualFold(d.Type, "Journal") {
		return "not_applicable", nil, false
	}
	var best *models.ScopusSourceMetric
	for i := range metrics {
		m := &metrics[i]
		if m.SourceID == d.SourceID && m.DocType == "all" && m.MetricYear <= d.Year && m.CiteScoreStatus != nil && strings.EqualFold(*m.CiteScoreStatus, "complete") && (best == nil || m.MetricYear > best.MetricYear) {
			best = m
		}
	}
	if best == nil {
		return "missing", nil, false
	}
	year := best.MetricYear
	if mode == "t1" && best.CiteScorePercentile != nil && *best.CiteScorePercentile >= 90 && *best.CiteScorePercentile <= 100 {
		return "T1", &year, year < d.Year
	}
	if best.CiteScoreQuartile != nil {
		q := strings.ToUpper(strings.TrimSpace(*best.CiteScoreQuartile))
		if summaryContains([]string{"Q1", "Q2", "Q3", "Q4"}, q) {
			return q, &year, year < d.Year
		}
	}
	return "missing", &year, year < d.Year
}
func summaryPct(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) * 100 / float64(d)
	return &v
}
func summaryRow(label string, docs []SummaryDocument, available bool) SummaryCountRow {
	r := SummaryCountRow{Label: label}
	if !available {
		return r
	}
	t, k, c := len(docs), 0, 0
	for _, d := range docs {
		if d.KKU {
			k++
		}
		if d.COC {
			c++
		}
	}
	r.Thailand = &t
	r.KKU = &k
	r.COC = &c
	r.KKUPct = summaryPct(k, t)
	r.COCPct = summaryPct(c, k)
	return r
}
func summaryPasses(d SummaryDocument, f BenchmarkSummaryFilter) bool {
	return (summaryContains(f.Types, "all") || summaryContains(f.Types, d.Type)) && summaryContains(f.Confidence, d.Confidence) && (f.Category == "all" || f.Category == "classified" && d.CategoryID != 0 || f.Category == "unknown" && d.CategoryID == 0 || f.Category == strconv.FormatUint(d.CategoryID, 10))
}
func aggregateBenchmarkSummary(in summaryInput, f BenchmarkSummaryFilter) *BenchmarkSummaryReport {
	r := &BenchmarkSummaryReport{Filters: f, GeneratedAt: time.Now().UTC(), Years: in.Years, Yearly: []SummaryCountRow{}, Categories: []SummaryCountRow{}, Quartiles: []SummaryCountRow{}, Documents: []SummaryDocument{}, Faculty: append([]SummaryFaculty{}, in.Faculty...)}
	r.Coverage.BaseDocuments = len(in.Documents)
	cats := map[uint64]string{0: "ไม่มี Category"}
	for _, c := range in.Categories {
		cats[c.ID] = c.Name
	}
	facultyIDs := map[string][]int{}
	byUser := map[int]*SummaryFaculty{}
	for i := range r.Faculty {
		u := &r.Faculty[i]
		u.ScopusID = normalizeScopusID(u.ScopusID)
		u.Linkable = u.ScopusID != "" && u.ScopusID != "-"
		if !u.Linkable {
			r.Coverage.FacultyWithoutID++
		} else {
			facultyIDs[u.ScopusID] = append(facultyIDs[u.ScopusID], u.UserID)
		}
		byUser[u.UserID] = u
	}
	metricMap := map[string][]models.ScopusSourceMetric{}
	for _, m := range in.Metrics {
		metricMap[m.SourceID] = append(metricMap[m.SourceID], m)
	}
	for _, doc := range in.Documents {
		d := doc
		d.Authors = append([]SummaryAuthor{}, doc.Authors...)
		if d.CategoryID != 0 {
			r.Coverage.Classified++
		}
		if !summaryPasses(d, f) {
			continue
		}
		r.Coverage.Selected++
		d.CategoryName = cats[d.CategoryID]
		if d.CategoryName == "" {
			d.CategoryName = fmt.Sprintf("Category #%d", d.CategoryID)
		}
		d.Quartile, d.MetricYear, d.MetricFallback = summaryBucket(d, metricMap[d.SourceID], f.QuartileMode)
		if d.MetricFallback {
			r.Coverage.MetricFallback++
		}
		if d.Quartile == "missing" {
			r.Coverage.MissingQuartile++
		}
		if !d.AffiliationsComplete {
			r.Coverage.AffiliationIncomplete++
		}
		for _, af := range d.Afids {
			if summaryKKU[af] {
				d.KKU = true
			}
		}
		seenUsers := map[int]bool{}
		hasFirst, hasCorr, hasUnknown := false, false, false
		for i := range d.Authors {
			a := &d.Authors[i]
			ids := facultyIDs[normalizeScopusID(a.ScopusAuthorID)]
			if len(ids) == 0 {
				continue
			}
			if !a.AffiliationsComplete {
				r.Coverage.FacultyAffiliationIncomplete++
			}
			eligible := false
			for _, af := range a.Afids {
				if summaryCOC[af] {
					eligible = true
				}
			}
			if !eligible || !d.KKU {
				continue
			}
			d.COC = true
			a.EligibleUserIDs = ids
			first, corr, known := summaryRole(*a)
			hasFirst = hasFirst || first
			hasCorr = hasCorr || corr
			hasUnknown = hasUnknown || !known
			for _, id := range ids {
				if seenUsers[id] {
					continue
				}
				seenUsers[id] = true
				u := byUser[id]
				u.Total++
				if first {
					u.First++
				}
				if corr {
					u.Corresponding++
				}
				if first || corr {
					u.Lead++
				} else if known {
					u.Co++
				} else {
					u.Unknown++
					r.Coverage.RoleUnknownPairs++
				}
			}
		}
		if d.COC {
			d.FacultyRoles.Total = 1
			r.FacultyRoles.Total++
			if hasFirst {
				d.FacultyRoles.First = 1
				r.FacultyRoles.First++
			}
			if hasCorr {
				d.FacultyRoles.Corresponding = 1
				r.FacultyRoles.Corresponding++
			}
			if hasFirst || hasCorr {
				d.FacultyRoles.Lead = 1
				r.FacultyRoles.Lead++
			} else if hasUnknown {
				d.FacultyRoles.Unknown = 1
				r.FacultyRoles.Unknown++
			} else {
				d.FacultyRoles.Co = 1
				r.FacultyRoles.Co++
			}
		}
		r.Documents = append(r.Documents, d)
	}
	anyAvailable := false
	for _, state := range r.Years {
		available := state.Status != "missing"
		anyAvailable = anyAvailable || available
		yearDocs := []SummaryDocument{}
		for _, d := range r.Documents {
			if d.Year == state.Year {
				yearDocs = append(yearDocs, d)
			}
		}
		row := summaryRow(strconv.Itoa(state.Year), yearDocs, available)
		row.Year = state.Year
		r.Yearly = append(r.Yearly, row)
		for _, cat := range in.Categories {
			if f.Category == "unknown" || (f.Category != "all" && f.Category != "classified" && f.Category != strconv.FormatUint(cat.ID, 10)) {
				continue
			}
			docs := []SummaryDocument{}
			for _, d := range yearDocs {
				if d.CategoryID == cat.ID {
					docs = append(docs, d)
				}
			}
			row := summaryRow(cat.Name, docs, available)
			row.Year = state.Year
			row.CategoryID = cat.ID
			r.Categories = append(r.Categories, row)
		}
		if f.Category == "all" || f.Category == "unknown" {
			docs := []SummaryDocument{}
			for _, d := range yearDocs {
				if d.CategoryID == 0 {
					docs = append(docs, d)
				}
			}
			row := summaryRow(cats[0], docs, available)
			row.Year = state.Year
			r.Categories = append(r.Categories, row)
		}
	}
	r.Total = summaryRow("รวมจากปีที่มีข้อมูล", r.Documents, anyAvailable)
	buckets := []string{"Q1", "Q2", "Q3", "Q4", "missing", "not_applicable"}
	if f.QuartileMode == "t1" {
		buckets = append([]string{"T1"}, buckets...)
	}
	ordered := append([]SummaryCategory{}, in.Categories...)
	if f.Category == "all" || f.Category == "unknown" {
		ordered = append(ordered, SummaryCategory{Name: cats[0]})
	}
	for _, cat := range ordered {
		if f.Category == "unknown" && cat.ID != 0 || f.Category != "all" && f.Category != "classified" && f.Category != "unknown" && f.Category != strconv.FormatUint(cat.ID, 10) {
			continue
		}
		for _, bucket := range buckets {
			docs := []SummaryDocument{}
			for _, d := range r.Documents {
				if d.CategoryID == cat.ID && d.Quartile == bucket {
					docs = append(docs, d)
				}
			}
			row := summaryRow(cat.Name, docs, anyAvailable)
			row.CategoryID = cat.ID
			row.Quartile = bucket
			r.Quartiles = append(r.Quartiles, row)
		}
	}
	for i := range r.Faculty {
		u := &r.Faculty[i]
		u.FirstPct = summaryPct(u.First, u.Total)
		u.CorrespondingPct = summaryPct(u.Corresponding, u.Total)
		u.LeadPct = summaryPct(u.Lead, u.Total)
		u.CoPct = summaryPct(u.Co, u.Total)
		u.UnknownPct = summaryPct(u.Unknown, u.Total)
	}
	// Revision binds all visible values, drilldown metadata and filters. GeneratedAt
	// is intentionally excluded; changes to users, XML roles, metrics or affiliations
	// therefore invalidate export even without a benchmark updated_at change.
	b, _ := json.Marshal(struct {
		Filter   BenchmarkSummaryFilter
		Years    []SummaryYearState
		Docs     []SummaryDocument
		Faculty  []SummaryFaculty
		Cats     []SummaryCategory
		Coverage SummaryCoverage
	}{f, r.Years, r.Documents, r.Faculty, in.Categories, r.Coverage})
	h := sha256.Sum256(b)
	r.Revision = hex.EncodeToString(h[:])
	return r
}

func (s *ScopusBenchmarkService) BenchmarkSummaryOptions(ctx context.Context) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cats []SummaryCategory
		var years []int
		var types []string
		if err := tx.Table("paper_categories").Order("display_order,category_id").Scan(&cats).Error; err != nil {
			return err
		}
		if err := tx.Table("scopus_benchmark_document_scopes m").Distinct("m.pub_year").Joins("JOIN scopus_benchmark_scopes s ON s.id=m.scope_id").Where("s.code='country_thailand' AND m.pub_year IS NOT NULL").Order("m.pub_year DESC").Pluck("m.pub_year", &years).Error; err != nil {
			return err
		}
		if err := tx.Table("scopus_benchmark_documents d").Distinct("COALESCE(d.aggregation_type,'unknown')").Order("COALESCE(d.aggregation_type,'unknown')").Pluck("COALESCE(d.aggregation_type,'unknown')", &types).Error; err != nil {
			return err
		}
		if !summaryContains(types, "Journal") {
			types = append(types, "Journal")
		}
		out["categories"] = cats
		out["years"] = years
		out["types"] = types
		out["confidence"] = []string{"High", "Medium", "Low", "unknown"}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}
