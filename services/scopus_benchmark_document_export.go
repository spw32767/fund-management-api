package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fund-management-api/models"
)

// ErrBenchmarkExportEmpty is returned when the requested scope/range has no stored
// benchmark documents, so the caller can surface "no documents" instead of handing
// the user an empty file that looks like a complete export (§10.3).
var ErrBenchmarkExportEmpty = errors.New("no benchmark documents for this scope and year range")

// benchmarkDocumentExportHeaders is the Documents schema — the SAME 36 column names
// and order as the search page's EXPORT_COLUMNS (handoff §10.2). This is a
// document-level export only: no per-user columns, one row per document.
var benchmarkDocumentExportHeaders = []string{
	"ลำดับ", "scopus_id", "scopus_link", "title", "authors", "abstract", "aggregation_type", "source_id", "publication_name",
	"afid", "name", "city", "country", "affiliation_url", "affiliations_json", "issn", "eissn", "isbn", "volume", "issue",
	"page_range", "article_number", "cover_date", "doi", "citedby_count", "authkeywords", "fund_sponsor", "cite_score_status",
	"cite_score_rank", "cite_score_percentile", "journal_tier_bucket", "cite_score_quartile", "publication_year", "eid",
	"scopus_url", "doi_url",
}

// BenchmarkDocumentExportLevels maps the two exportable UI levels to benchmark scope
// levels (§10.3: KKU → university, Thailand → country). Faculty is intentionally not
// exportable here.
var BenchmarkDocumentExportLevels = map[string]string{
	"university": "university",
	"country":    "country",
}

// BenchmarkExportCompleteness reports whether the exported document set is the full
// harvested cohort the count snapshots imply, so the UI can warn when a level/year is
// still being harvested rather than presenting a short file as complete (§10.3, R4).
// The check is PER YEAR (missingBenchmarkYears) — a year that over-harvested can never
// mask another year that is short.
type BenchmarkExportCompleteness struct {
	ExportedRows  int
	ExpectedDocs  int
	MissingYears  []int
	ActiveHarvest bool
	Incomplete    bool
}

type benchmarkExportRow struct {
	ID                  int64      `gorm:"column:id"`
	ScopusID            *string    `gorm:"column:scopus_id"`
	ScopusLink          *string    `gorm:"column:scopus_link"`
	Title               *string    `gorm:"column:title"`
	Abstract            *string    `gorm:"column:abstract"`
	AggregationType     *string    `gorm:"column:aggregation_type"`
	SourceID            *string    `gorm:"column:source_id"`
	PublicationName     *string    `gorm:"column:publication_name"`
	ISSN                *string    `gorm:"column:issn"`
	EISSN               *string    `gorm:"column:eissn"`
	ISBN                *string    `gorm:"column:isbn"`
	Volume              *string    `gorm:"column:volume"`
	Issue               *string    `gorm:"column:issue"`
	PageRange           *string    `gorm:"column:page_range"`
	ArticleNumber       *string    `gorm:"column:article_number"`
	CoverDate           *time.Time `gorm:"column:cover_date"`
	DOI                 *string    `gorm:"column:doi"`
	CitedByCount        *int       `gorm:"column:citedby_count"`
	AuthKeywords        []byte     `gorm:"column:authkeywords"`
	FundSponsor         *string    `gorm:"column:fund_sponsor"`
	PubYear             *int       `gorm:"column:pub_year"`
	EID                 string     `gorm:"column:eid"`
	CiteScoreStatus     *string    `gorm:"column:cs_status"`
	CiteScoreRank       *int       `gorm:"column:cs_rank"`
	CiteScorePercentile *float64   `gorm:"column:cs_pct"`
	CiteScoreQuartile   *string    `gorm:"column:cs_quartile"`
	Authors             *string    `gorm:"column:authors"`
	// MembershipYear is bds.pub_year — the year used by the WHERE filter, so the
	// per-year harvested count is derived from the exact rows in the file (R4.1).
	MembershipYear int `gorm:"column:membership_year"`
}

type benchmarkAffiliationRow struct {
	DocumentID     int64   `gorm:"column:document_id"`
	Afid           string  `gorm:"column:afid"`
	Name           *string `gorm:"column:name"`
	City           *string `gorm:"column:city"`
	Country        *string `gorm:"column:country"`
	AffiliationURL *string `gorm:"column:affiliation_url"`
}

type benchmarkAffiliationJSON struct {
	Afid           string `json:"afid"`
	Name           string `json:"name"`
	City           string `json:"city"`
	Country        string `json:"country"`
	AffiliationURL string `json:"affiliation_url"`
}

// ExportBenchmarkDocumentsCSV builds the full Documents CSV for one benchmark level
// over an inclusive [yearFrom, yearTo] window. It is read-only (no harvest/refresh)
// and returns exactly one row per benchmark document — the author/affiliation joins
// are done as a correlated subquery and a separate grouped query so they can never
// multiply the document rows (§10.4). The whole file is built in memory and returned
// only on full success, so a mid-build failure never yields a partial "successful"
// download (§10.4). Returns ErrBenchmarkExportEmpty when nothing matches.
func (s *ScopusBenchmarkService) ExportBenchmarkDocumentsCSV(ctx context.Context, level string, yearFrom, yearTo int) ([]byte, BenchmarkExportCompleteness, error) {
	var completeness BenchmarkExportCompleteness
	scopeLevel, ok := BenchmarkDocumentExportLevels[level]
	if !ok {
		return nil, completeness, fmt.Errorf("unsupported export level %q", level)
	}
	if yearFrom > yearTo {
		yearFrom, yearTo = yearTo, yearFrom
	}

	var scope models.ScopusBenchmarkScope
	if err := s.db.WithContext(ctx).Where("level = ?", scopeLevel).First(&scope).Error; err != nil {
		return nil, completeness, fmt.Errorf("resolve %s benchmark scope: %w", scopeLevel, err)
	}

	var rows []benchmarkExportRow
	// One row per document: bds has a single membership row per (document, scope); the
	// metrics LEFT JOIN binds a SINGLE latest metric row id (never the whole history,
	// which would duplicate the document); authors are a correlated GROUP_CONCAT.
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			d.id, d.scopus_id, d.scopus_link, d.title, d.abstract, d.aggregation_type, d.source_id, d.publication_name,
			d.issn, d.eissn, d.isbn, d.volume, d.issue, d.page_range, d.article_number, d.cover_date, d.doi,
			d.citedby_count, d.authkeywords, d.fund_sponsor, d.pub_year, d.eid,
			m.cite_score_status AS cs_status, m.cite_score_rank AS cs_rank,
			m.cite_score_percentile AS cs_pct, m.cite_score_quartile AS cs_quartile,
			bds.pub_year AS membership_year,
			(SELECT GROUP_CONCAT(COALESCE(a.full_name, a.surname, a.scopus_author_id) ORDER BY da.author_seq SEPARATOR '; ')
				FROM scopus_benchmark_document_authors AS da
				JOIN scopus_benchmark_authors AS a ON a.id = da.author_id
				WHERE da.document_id = d.id) AS authors
		FROM scopus_benchmark_document_scopes AS bds
		JOIN scopus_benchmark_documents AS d ON d.id = bds.document_id
		LEFT JOIN scopus_source_metrics AS m ON m.source_metric_id = (
			SELECT im.source_metric_id FROM scopus_source_metrics AS im
			WHERE im.source_id = d.source_id AND im.doc_type = 'all'
			ORDER BY im.metric_year DESC, im.source_metric_id DESC
			LIMIT 1)
		WHERE bds.scope_id = ? AND bds.pub_year BETWEEN ? AND ?
		ORDER BY d.pub_year DESC, d.id ASC`, scope.ID, yearFrom, yearTo).
		Scan(&rows).Error; err != nil {
		return nil, completeness, fmt.Errorf("load benchmark documents: %w", err)
	}
	if len(rows) == 0 {
		return nil, completeness, ErrBenchmarkExportEmpty
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	affByDoc, err := s.loadBenchmarkExportAffiliations(ctx, ids)
	if err != nil {
		return nil, completeness, err
	}

	// Harvested-per-year is counted from the EXACT rows that will be written to the
	// file (by the membership year used in the WHERE), so a harvest that finishes
	// mid-request can never make a short file read as complete (R4.1).
	harvestedByYear := make(map[int]int, yearTo-yearFrom+1)
	for _, r := range rows {
		harvestedByYear[r.MembershipYear]++
	}
	completeness, err = s.benchmarkExportCompleteness(ctx, scope.ID, yearFrom, yearTo, harvestedByYear, len(rows))
	if err != nil {
		return nil, completeness, err
	}

	var buf bytes.Buffer
	writeBenchmarkExportBOMAndHeader(&buf)
	writeBenchmarkExportCSVRows(&buf, rows, affByDoc, 1)
	return buf.Bytes(), completeness, nil
}

// writeBenchmarkExportBOMAndHeader writes the UTF-8 BOM (so Thai opens in Excel) and
// the 36-column header row. Only the first page of an export carries these.
func writeBenchmarkExportBOMAndHeader(buf *bytes.Buffer) {
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	buf.WriteString(joinCSV(benchmarkDocumentExportHeaders))
	buf.WriteString("\r\n")
}

// writeBenchmarkExportCSVRows appends one CSV line per document, numbered from
// startNumber so the "ลำดับ" column stays continuous across pages. Shared by the full
// and paged exports so the 36-column format is byte-identical either way.
func writeBenchmarkExportCSVRows(buf *bytes.Buffer, rows []benchmarkExportRow, affByDoc map[int64][]benchmarkAffiliationJSON, startNumber int) {
	for i, r := range rows {
		affs := affByDoc[r.ID]
		// The single afid/name/city/country/affiliation_url columns aggregate ALL of the
		// document's affiliations joined with " | " (dedup, stable order), exactly like
		// the search page's Documents export (joinNonEmptyValues) — never just the first,
		// so filtering the country column still reveals foreign collaboration (R2).
		affJSON := "[]"
		if len(affs) > 0 {
			if encoded, err := json.Marshal(affs); err == nil {
				affJSON = string(encoded)
			}
		}
		record := []string{
			strconv.Itoa(startNumber + i),
			deref(r.ScopusID),
			deref(r.ScopusLink),
			deref(r.Title),
			deref(r.Authors),
			deref(r.Abstract),
			deref(r.AggregationType),
			deref(r.SourceID),
			deref(r.PublicationName),
			joinAffField(affs, func(a benchmarkAffiliationJSON) string { return a.Afid }),
			joinAffField(affs, func(a benchmarkAffiliationJSON) string { return a.Name }),
			joinAffField(affs, func(a benchmarkAffiliationJSON) string { return a.City }),
			joinAffField(affs, func(a benchmarkAffiliationJSON) string { return a.Country }),
			joinAffField(affs, func(a benchmarkAffiliationJSON) string { return a.AffiliationURL }),
			affJSON,
			deref(r.ISSN),
			deref(r.EISSN),
			deref(r.ISBN),
			deref(r.Volume),
			deref(r.Issue),
			deref(r.PageRange),
			deref(r.ArticleNumber),
			formatBenchmarkDate(r.CoverDate),
			deref(r.DOI),
			formatIntPtr(r.CitedByCount),
			benchmarkAuthKeywords(r.AuthKeywords),
			deref(r.FundSponsor),
			deref(r.CiteScoreStatus),
			formatIntPtr(r.CiteScoreRank),
			formatFloatPtr(r.CiteScorePercentile),
			journalTierBucket(r.CiteScorePercentile),
			strings.ToUpper(deref(r.CiteScoreQuartile)),
			formatIntPtr(r.PubYear),
			r.EID,
			deref(r.ScopusLink), // scopus_url: benchmark stores only scopus_link (mapping note)
			deref(r.DOI),        // doi_url: benchmark has no separate doi_url column
		}
		buf.WriteString(joinCSV(record))
		buf.WriteString("\r\n")
	}
}

// BenchmarkExportPage is one keyset page of the document export. The FE requests pages
// in order and concatenates their bodies into one CSV file, so no single HTTP response
// is large enough to hit the production proxy's size ceiling (net::ERR_FAILED on big
// Thailand exports). Only the first page carries the BOM+header, the total row count
// and the completeness metadata.
type BenchmarkExportPage struct {
	Body         []byte
	RowsInPage   int
	NextYear     *int
	NextID       *int64
	TotalRows    int
	Completeness BenchmarkExportCompleteness
	FirstPage    bool
}

// ExportBenchmarkDocumentsPage returns one keyset page of the export, ordered by
// pub_year DESC, id ASC. afterYear/afterID is the cursor (nil on the first page).
// rowOffset is the number of rows already emitted by earlier pages, so the "ลำดับ"
// column stays continuous. Read-only; one row per document (same joins as the full
// export). Keyset (not OFFSET) avoids re-scanning skipped rows on a slow DB.
func (s *ScopusBenchmarkService) ExportBenchmarkDocumentsPage(ctx context.Context, level string, yearFrom, yearTo, limit int, afterYear *int, afterID *int64, rowOffset int) (BenchmarkExportPage, error) {
	var page BenchmarkExportPage
	page.FirstPage = afterYear == nil && afterID == nil
	scopeLevel, ok := BenchmarkDocumentExportLevels[level]
	if !ok {
		return page, fmt.Errorf("unsupported export level %q", level)
	}
	if yearFrom > yearTo {
		yearFrom, yearTo = yearTo, yearFrom
	}
	if limit <= 0 {
		limit = 2000
	}

	var scope models.ScopusBenchmarkScope
	if err := s.db.WithContext(ctx).Where("level = ?", scopeLevel).First(&scope).Error; err != nil {
		return page, fmt.Errorf("resolve %s benchmark scope: %w", scopeLevel, err)
	}

	cursor := ""
	args := []interface{}{scope.ID, yearFrom, yearTo}
	if !page.FirstPage {
		// Rows AFTER (afterYear, afterID) in pub_year DESC, id ASC order.
		cursor = "\n\t\t\tAND (d.pub_year < ? OR (d.pub_year = ? AND d.id > ?))"
		args = append(args, *afterYear, *afterYear, *afterID)
	}
	args = append(args, limit)

	var rows []benchmarkExportRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			d.id, d.scopus_id, d.scopus_link, d.title, d.abstract, d.aggregation_type, d.source_id, d.publication_name,
			d.issn, d.eissn, d.isbn, d.volume, d.issue, d.page_range, d.article_number, d.cover_date, d.doi,
			d.citedby_count, d.authkeywords, d.fund_sponsor, d.pub_year, d.eid,
			m.cite_score_status AS cs_status, m.cite_score_rank AS cs_rank,
			m.cite_score_percentile AS cs_pct, m.cite_score_quartile AS cs_quartile,
			bds.pub_year AS membership_year,
			(SELECT GROUP_CONCAT(COALESCE(a.full_name, a.surname, a.scopus_author_id) ORDER BY da.author_seq SEPARATOR '; ')
				FROM scopus_benchmark_document_authors AS da
				JOIN scopus_benchmark_authors AS a ON a.id = da.author_id
				WHERE da.document_id = d.id) AS authors
		FROM scopus_benchmark_document_scopes AS bds
		JOIN scopus_benchmark_documents AS d ON d.id = bds.document_id
		LEFT JOIN scopus_source_metrics AS m ON m.source_metric_id = (
			SELECT im.source_metric_id FROM scopus_source_metrics AS im
			WHERE im.source_id = d.source_id AND im.doc_type = 'all'
			ORDER BY im.metric_year DESC, im.source_metric_id DESC
			LIMIT 1)
		WHERE bds.scope_id = ? AND bds.pub_year BETWEEN ? AND ?`+cursor+`
		ORDER BY d.pub_year DESC, d.id ASC
		LIMIT ?`, args...).
		Scan(&rows).Error; err != nil {
		return page, fmt.Errorf("load benchmark documents page: %w", err)
	}

	if page.FirstPage && len(rows) == 0 {
		return page, ErrBenchmarkExportEmpty
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	affByDoc, err := s.loadBenchmarkExportAffiliations(ctx, ids)
	if err != nil {
		return page, err
	}

	var buf bytes.Buffer
	if page.FirstPage {
		writeBenchmarkExportBOMAndHeader(&buf)
	}
	writeBenchmarkExportCSVRows(&buf, rows, affByDoc, rowOffset+1)
	page.Body = buf.Bytes()
	page.RowsInPage = len(rows)

	// A full page means there may be more — hand back the keyset cursor of the last row.
	if len(rows) == limit {
		last := rows[len(rows)-1]
		year := 0
		if last.PubYear != nil {
			year = *last.PubYear
		}
		id := last.ID
		page.NextYear = &year
		page.NextID = &id
	}

	// Total row count + completeness are computed once, on the first page.
	if page.FirstPage {
		var total int64
		if err := s.db.WithContext(ctx).Raw(`
			SELECT COUNT(*) FROM scopus_benchmark_document_scopes AS bds
			WHERE bds.scope_id = ? AND bds.pub_year BETWEEN ? AND ?`, scope.ID, yearFrom, yearTo).
			Scan(&total).Error; err != nil {
			return page, fmt.Errorf("count benchmark documents: %w", err)
		}
		page.TotalRows = int(total)

		type yearCount struct {
			PubYear int `gorm:"column:pub_year"`
			Total   int `gorm:"column:total"`
		}
		var harvestedRows []yearCount
		if err := s.db.WithContext(ctx).Raw(`
			SELECT bds.pub_year AS pub_year, COUNT(DISTINCT bds.document_id) AS total
			FROM scopus_benchmark_document_scopes AS bds
			WHERE bds.scope_id = ? AND bds.pub_year BETWEEN ? AND ?
			GROUP BY bds.pub_year`, scope.ID, yearFrom, yearTo).
			Scan(&harvestedRows).Error; err != nil {
			return page, fmt.Errorf("load export harvested coverage: %w", err)
		}
		harvestedByYear := make(map[int]int, len(harvestedRows))
		for _, r := range harvestedRows {
			harvestedByYear[r.PubYear] = r.Total
		}
		page.Completeness, err = s.benchmarkExportCompleteness(ctx, scope.ID, yearFrom, yearTo, harvestedByYear, int(total))
		if err != nil {
			return page, err
		}
	}

	return page, nil
}

// benchmarkExportCompleteness compares, per year, the latest count snapshot total for
// the scope against the documents ACTUALLY IN THE EXPORTED FILE (harvestedByYear, from
// the exported rows — not a re-query), and reports the short years plus whether a
// harvest is currently writing this scope. Exported rows are still returned in full —
// the flag only tells the UI to warn (§10.3, R4/R4.1).
func (s *ScopusBenchmarkService) benchmarkExportCompleteness(ctx context.Context, scopeID uint64, yearFrom, yearTo int, harvestedByYear map[int]int, exportedRows int) (BenchmarkExportCompleteness, error) {
	type yearCount struct {
		PubYear int `gorm:"column:pub_year"`
		Total   int `gorm:"column:total"`
	}
	var snapshotRows []yearCount
	if err := s.db.WithContext(ctx).Raw(`
		SELECT s.pub_year AS pub_year, s.total_results AS total
		FROM scopus_benchmark_count_snapshots AS s
		JOIN (
			SELECT pub_year, MAX(id) AS max_id
			FROM scopus_benchmark_count_snapshots
			WHERE scope_id = ? AND pub_year BETWEEN ? AND ?
			GROUP BY pub_year
		) AS latest ON latest.max_id = s.id
		WHERE s.scope_id = ?`, scopeID, yearFrom, yearTo, scopeID).
		Scan(&snapshotRows).Error; err != nil {
		return BenchmarkExportCompleteness{ExportedRows: exportedRows, MissingYears: []int{}}, fmt.Errorf("load export snapshot coverage: %w", err)
	}
	expected := make(map[int]int, len(snapshotRows))
	for _, r := range snapshotRows {
		expected[r.PubYear] = r.Total
	}

	activeRun, err := s.GetActiveRun(ctx)
	if err != nil {
		return BenchmarkExportCompleteness{ExportedRows: exportedRows, MissingYears: []int{}}, err
	}
	activeHarvest := activeRun != nil && activeRun.ScopeID != nil && *activeRun.ScopeID == scopeID

	return deriveExportCompleteness(yearFrom, yearTo, expected, harvestedByYear, activeHarvest, exportedRows), nil
}

// deriveExportCompleteness is the pure completeness decision: per-year snapshot totals
// (expected) vs the counts actually in the file (harvested), checked YEAR BY YEAR so an
// over-harvested year can never offset a short one, plus the active-harvest guard. It
// takes the harvested counts as data (from the exported rows), so the result never
// depends on when a second query ran (R4.1).
func deriveExportCompleteness(yearFrom, yearTo int, expected, harvested map[int]int, activeHarvest bool, exportedRows int) BenchmarkExportCompleteness {
	out := BenchmarkExportCompleteness{ExportedRows: exportedRows, MissingYears: []int{}, ActiveHarvest: activeHarvest}
	for _, v := range expected {
		out.ExpectedDocs += v
	}
	out.MissingYears = missingBenchmarkYears(yearFrom, yearTo, expected, harvested)
	out.Incomplete = len(out.MissingYears) > 0 || activeHarvest
	return out
}

// loadBenchmarkExportAffiliations returns each document's distinct affiliations in a
// stable order (author_seq then afid). The first entry is the primary affiliation for
// the single afid/name/city/country/affiliation_url columns; the full list becomes
// affiliations_json (§10.2). Document ids are chunked so a very large Thailand export
// never builds a single unbounded IN(...) clause.
func (s *ScopusBenchmarkService) loadBenchmarkExportAffiliations(ctx context.Context, ids []int64) (map[int64][]benchmarkAffiliationJSON, error) {
	out := make(map[int64][]benchmarkAffiliationJSON, len(ids))
	const chunk = 800
	for start := 0; start < len(ids); start += chunk {
		end := start + chunk
		if end > len(ids) {
			end = len(ids)
		}
		var affRows []benchmarkAffiliationRow
		if err := s.db.WithContext(ctx).Raw(`
			SELECT da.document_id AS document_id, aff.afid AS afid, aff.name AS name, aff.city AS city,
			       aff.country AS country, aff.affiliation_url AS affiliation_url, MIN(da.author_seq) AS seq
			FROM scopus_benchmark_document_authors AS da
			JOIN scopus_benchmark_affiliations AS aff ON aff.id = da.affiliation_id
			WHERE da.document_id IN ?
			GROUP BY da.document_id, aff.afid, aff.name, aff.city, aff.country, aff.affiliation_url
			ORDER BY da.document_id, seq ASC, aff.afid ASC`, ids[start:end]).
			Scan(&affRows).Error; err != nil {
			return nil, fmt.Errorf("load benchmark affiliations: %w", err)
		}
		for _, a := range affRows {
			out[a.DocumentID] = append(out[a.DocumentID], benchmarkAffiliationJSON{
				Afid:           a.Afid,
				Name:           deref(a.Name),
				City:           deref(a.City),
				Country:        deref(a.Country),
				AffiliationURL: deref(a.AffiliationURL),
			})
		}
	}
	return out, nil
}

// joinAffField joins one affiliation field across every distinct affiliation of a
// document with " | " (skipping empties), matching the search page's Documents export
// so the single-value columns carry the full set, not just the primary affiliation.
func joinAffField(affs []benchmarkAffiliationJSON, pick func(benchmarkAffiliationJSON) string) string {
	parts := make([]string, 0, len(affs))
	for _, a := range affs {
		if v := strings.TrimSpace(pick(a)); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " | ")
}

// ── CSV formatting helpers ───────────────────────────────────────────────────

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func formatIntPtr(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

func formatFloatPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

func formatBenchmarkDate(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.Format("2006-01-02")
}

// journalTierBucket mirrors the search page's resolveJournalTierBucket exactly so the
// exported tier column matches (§10.2): >=90 T1, >=75 Q1, >=50 Q2, >=25 Q3, else Q4;
// blank for a missing/zero percentile.
func journalTierBucket(percentile *float64) string {
	if percentile == nil || *percentile <= 0 {
		return ""
	}
	switch v := *percentile; {
	case v >= 90:
		return "T1"
	case v >= 75:
		return "Q1"
	case v >= 50:
		return "Q2"
	case v >= 25:
		return "Q3"
	default:
		return "Q4"
	}
}

// benchmarkAuthKeywords renders the stored keyword blob as "kw1; kw2" like the search
// export. It accepts the common Scopus shapes (["a","b"], [{"$":"a"}], {"author-keyword":[…]})
// and returns blank for an unparseable structured blob rather than dumping raw JSON.
func benchmarkAuthKeywords(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	if trimmed[0] != '[' && trimmed[0] != '{' {
		return trimmed // already a plain "; "-style string
	}
	var strArr []string
	if json.Unmarshal(raw, &strArr) == nil {
		return joinNonEmpty(strArr)
	}
	var objArr []map[string]interface{}
	if json.Unmarshal(raw, &objArr) == nil {
		out := make([]string, 0, len(objArr))
		for _, o := range objArr {
			out = append(out, firstStringField(o, "$", "keyword", "authkeyword"))
		}
		return joinNonEmpty(out)
	}
	var wrapper map[string]json.RawMessage
	if json.Unmarshal(raw, &wrapper) == nil {
		for _, key := range []string{"author-keyword", "authkeywords", "keywords"} {
			if inner, ok := wrapper[key]; ok {
				return benchmarkAuthKeywords(inner)
			}
		}
	}
	return "" // structured but unrecognised — blank, never a raw JSON dump
}

func firstStringField(o map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := o[key].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func joinNonEmpty(values []string) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return strings.Join(out, "; ")
}

// benchmarkCSVField escapes one field per RFC 4180 AND neutralises spreadsheet formula
// injection: a value starting with =, +, -, @, TAB or CR is prefixed with a single
// quote so a spreadsheet treats it as text, never a live formula (§10.4). IDs stay
// text (no numeric coercion here — auto-format is the opening program's behaviour).
func benchmarkCSVField(value string) string {
	if value != "" {
		switch value[0] {
		case '=', '+', '-', '@', '\t', '\r':
			value = "'" + value
		}
	}
	if strings.ContainsAny(value, ",\"\n\r") {
		value = "\"" + strings.ReplaceAll(value, "\"", "\"\"") + "\""
	}
	return value
}

func joinCSV(fields []string) string {
	escaped := make([]string, len(fields))
	for i, f := range fields {
		escaped[i] = benchmarkCSVField(f)
	}
	return strings.Join(escaped, ",")
}
