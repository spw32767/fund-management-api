package services

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"fund-management-api/config"

	"gorm.io/gorm"
)

// AuthorHIndexPoint is one point on the Hirsch h-graph: a document ranked by citations.
type AuthorHIndexPoint struct {
	Rank      int     `json:"rank"`
	Citations int     `json:"citations"`
	Title     *string `json:"title,omitempty"`
	Year      *int    `json:"year,omitempty"`
	EID       string  `json:"eid"`
}

// AuthorHIndexGraph is the computed h-graph for an author over an optional publication-year range.
type AuthorHIndexGraph struct {
	ScopusAuthorID  string              `json:"scopus_author_id"`
	HIndex          int                 `json:"h_index"`
	DocumentCount   int                 `json:"document_count"`
	CitationTotal   int                 `json:"citation_total"`
	YearFrom         *int                `json:"year_from,omitempty"`
	YearTo           *int                `json:"year_to,omitempty"`
	AvailableYearMin *int                `json:"available_year_min,omitempty"`
	AvailableYearMax *int                `json:"available_year_max,omitempty"`
	AvailableYears   []int               `json:"available_years"`
	Points           []AuthorHIndexPoint `json:"points"`
}

// AuthorHGraphService computes the classic Hirsch h-graph from already-ingested Scopus documents.
// This reproduces the graph shown on scopus.com (documents ranked by citations, h-index = the
// point where the curve crosses y=x) without any additional API calls. Citation counts reflect
// the freshness of scopus_documents.citedby_count at ingest time.
type AuthorHGraphService struct {
	db *gorm.DB
}

// NewAuthorHGraphService constructs an AuthorHGraphService.
func NewAuthorHGraphService(db *gorm.DB) *AuthorHGraphService {
	if db == nil {
		db = config.DB
	}
	return &AuthorHGraphService{db: db}
}

// GetGraph builds the h-graph for the given Scopus author ID. yearFrom/yearTo are inclusive
// publication-year bounds; pass nil to leave a bound open.
func (s *AuthorHGraphService) GetGraph(ctx context.Context, scopusAuthorID string, yearFrom, yearTo *int) (*AuthorHIndexGraph, error) {
	scopusAuthorID = strings.TrimSpace(scopusAuthorID)
	if scopusAuthorID == "" {
		return nil, errors.New("scopus author id is required")
	}

	type docRow struct {
		Citations int     `gorm:"column:citations"`
		Year      *int    `gorm:"column:year"`
		Title     *string `gorm:"column:title"`
		EID       string  `gorm:"column:eid"` // GORM แปลง EID เป็นชื่อคอลัมน์ผิด ต้องระบุเอง
	}

	// Full document set for this author (for the available-year range), unfiltered.
	baseQuery := s.db.WithContext(ctx).
		Table("scopus_documents AS d").
		Select("COALESCE(d.citedby_count, 0) AS citations, YEAR(d.cover_date) AS year, d.title AS title, d.eid AS eid").
		Joins("JOIN scopus_document_authors da ON da.document_id = d.id").
		Joins("JOIN scopus_authors a ON a.id = da.author_id").
		Where("a.scopus_author_id = ?", scopusAuthorID)

	var allDocs []docRow
	if err := baseQuery.Find(&allDocs).Error; err != nil {
		return nil, err
	}

	graph := &AuthorHIndexGraph{
		ScopusAuthorID: scopusAuthorID,
		YearFrom:       yearFrom,
		YearTo:         yearTo,
		AvailableYears: []int{},
		Points:         []AuthorHIndexPoint{},
	}

	// Distinct publication years that actually have documents, plus the min/max range.
	// Ignores the filter so the UI can populate its year selectors from real data.
	yearSeen := make(map[int]struct{})
	for _, d := range allDocs {
		if d.Year == nil || *d.Year == 0 {
			continue
		}
		y := *d.Year
		if _, ok := yearSeen[y]; !ok {
			yearSeen[y] = struct{}{}
			graph.AvailableYears = append(graph.AvailableYears, y)
		}
		if graph.AvailableYearMin == nil || y < *graph.AvailableYearMin {
			yv := y
			graph.AvailableYearMin = &yv
		}
		if graph.AvailableYearMax == nil || y > *graph.AvailableYearMax {
			yv := y
			graph.AvailableYearMax = &yv
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(graph.AvailableYears)))

	// Apply the publication-year filter.
	filtered := make([]docRow, 0, len(allDocs))
	for _, d := range allDocs {
		if yearFrom != nil {
			if d.Year == nil || *d.Year < *yearFrom {
				continue
			}
		}
		if yearTo != nil {
			if d.Year == nil || *d.Year > *yearTo {
				continue
			}
		}
		filtered = append(filtered, d)
	}

	// Rank by citations descending (ties broken by year desc for stable, sensible ordering).
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Citations != filtered[j].Citations {
			return filtered[i].Citations > filtered[j].Citations
		}
		yi, yj := 0, 0
		if filtered[i].Year != nil {
			yi = *filtered[i].Year
		}
		if filtered[j].Year != nil {
			yj = *filtered[j].Year
		}
		return yi > yj
	})

	hIndex := 0
	for i, d := range filtered {
		rank := i + 1
		if d.Citations >= rank {
			hIndex = rank
		}
		graph.CitationTotal += d.Citations
		graph.Points = append(graph.Points, AuthorHIndexPoint{
			Rank:      rank,
			Citations: d.Citations,
			Title:     d.Title,
			Year:      d.Year,
			EID:       d.EID,
		})
	}

	graph.HIndex = hIndex
	graph.DocumentCount = len(filtered)
	return graph, nil
}

// facultyHGraphDocRow is one document in the faculty-level document set.
type facultyHGraphDocRow struct {
	Citations int     `gorm:"column:citations"`
	Year      *int    `gorm:"column:year"`
	Title     *string `gorm:"column:title"`
	EID       string  `gorm:"column:eid"` // GORM แปลง EID เป็นชื่อคอลัมน์ผิด ต้องระบุเอง
}

// facultyKKUExists returns a correlated EXISTS subquery (mirroring
// ScopusPublicationService.kkuDocumentAffiliationFilter) that is true when the document aliased
// by outerDocAlias has at least one author who is an in-faculty teacher (users.scopus_id, not
// deleted) affiliated with KKU on that document. Used as Where("EXISTS (?)", ...) against an
// outer "scopus_documents AS <outerDocAlias>" — selecting from scopus_documents directly means a
// paper shared by ≥2 KKU teachers is matched once. outerDocAlias is an internal constant, never
// user input.
func (s *AuthorHGraphService) facultyKKUExists(outerDocAlias string) *gorm.DB {
	return s.db.Table("scopus_document_authors AS sda_kku").
		Select("1").
		Joins("INNER JOIN scopus_authors AS sa_kku ON sa_kku.id = sda_kku.author_id").
		Joins("INNER JOIN users AS u_kku ON TRIM(u_kku.Scopus_id) = sa_kku.scopus_author_id").
		Joins("INNER JOIN scopus_affiliations AS aff_kku ON aff_kku.id = sda_kku.affiliation_id").
		Where("sda_kku.document_id = "+outerDocAlias+".id").
		Where("u_kku.delete_at IS NULL").
		Where("u_kku.Scopus_id IS NOT NULL AND TRIM(u_kku.Scopus_id) <> ''").
		Where("LOWER(TRIM(COALESCE(aff_kku.name, ''))) IN ?", kkuAffiliationNames)
}

// GetFacultyGraph builds the faculty-wide Hirsch h-graph. Unlike the per-author graph (which
// matches Scopus by using an author's whole global output), the faculty view counts only work
// affiliated with KKU — the same rule as the research-search page — so it reuses
// kkuAffiliationNames. A document is included once (deduped) when it has at least one author
// who is a teacher in this system AND was affiliated with KKU on that document; a paper
// co-authored by several KKU teachers is therefore counted a single time. yearFrom/yearTo are
// inclusive publication-year bounds; pass nil to leave a bound open. The returned shape matches
// AuthorHIndexGraph so the frontend can reuse the same graph renderer.
func (s *AuthorHGraphService) GetFacultyGraph(ctx context.Context, yearFrom, yearTo *int) (*AuthorHIndexGraph, error) {
	var allDocs []facultyHGraphDocRow
	if err := s.db.WithContext(ctx).
		Table("scopus_documents AS d").
		Select("COALESCE(d.citedby_count, 0) AS citations, YEAR(d.cover_date) AS year, d.title AS title, d.eid AS eid").
		Where("EXISTS (?)", s.facultyKKUExists("d")).
		Find(&allDocs).Error; err != nil {
		return nil, err
	}

	graph := &AuthorHIndexGraph{
		YearFrom:       yearFrom,
		YearTo:         yearTo,
		AvailableYears: []int{},
		Points:         []AuthorHIndexPoint{},
	}

	// Distinct publication years that actually have documents, plus the min/max range — ignores
	// the filter so the UI can populate its year selectors from real data (same as GetGraph).
	yearSeen := make(map[int]struct{})
	for _, d := range allDocs {
		if d.Year == nil || *d.Year == 0 {
			continue
		}
		y := *d.Year
		if _, ok := yearSeen[y]; !ok {
			yearSeen[y] = struct{}{}
			graph.AvailableYears = append(graph.AvailableYears, y)
		}
		if graph.AvailableYearMin == nil || y < *graph.AvailableYearMin {
			yv := y
			graph.AvailableYearMin = &yv
		}
		if graph.AvailableYearMax == nil || y > *graph.AvailableYearMax {
			yv := y
			graph.AvailableYearMax = &yv
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(graph.AvailableYears)))

	// Apply the publication-year filter.
	filtered := make([]facultyHGraphDocRow, 0, len(allDocs))
	for _, d := range allDocs {
		if yearFrom != nil {
			if d.Year == nil || *d.Year < *yearFrom {
				continue
			}
		}
		if yearTo != nil {
			if d.Year == nil || *d.Year > *yearTo {
				continue
			}
		}
		filtered = append(filtered, d)
	}

	// Rank by citations descending (ties broken by year desc) — same ordering as GetGraph.
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Citations != filtered[j].Citations {
			return filtered[i].Citations > filtered[j].Citations
		}
		yi, yj := 0, 0
		if filtered[i].Year != nil {
			yi = *filtered[i].Year
		}
		if filtered[j].Year != nil {
			yj = *filtered[j].Year
		}
		return yi > yj
	})

	hIndex := 0
	for i, d := range filtered {
		rank := i + 1
		if d.Citations >= rank {
			hIndex = rank
		}
		graph.CitationTotal += d.Citations
		graph.Points = append(graph.Points, AuthorHIndexPoint{
			Rank:      rank,
			Citations: d.Citations,
			Title:     d.Title,
			Year:      d.Year,
			EID:       d.EID,
		})
	}

	graph.HIndex = hIndex
	graph.DocumentCount = len(filtered)
	return graph, nil
}

// FacultyExportRow is one document in the faculty h-index XLSX export (Data sheet).
type FacultyExportRow struct {
	ScopusID            string
	Title               string
	Authors             string // author full names in author_seq order, joined by "; "
	Abstract            string
	AggregationType     string
	CitedByCount        int
	AuthKeywords        string // decoded from the stored JSON array, joined by "; "
	FundSponsor         string
	CiteScoreStatus     string
	CiteScoreRank       *int
	CiteScorePercentile *float64
	JournalTierBucket   string // T1 / Q1..Q4 / N/A — same tiering as the dashboard
	CiteScoreQuartile   string
	PublicationYearCE   *int
	PublicationYearBE   *int
	EID                 string
	ScopusURL           string
	DOIURL              string
	Order4Index         int // rank in citation-desc order (the ordering that defines h-index)
}

// FacultyYearCount is one row of the per-year breakdown on the Summary sheet.
type FacultyYearCount struct {
	YearCE    int
	Count     int
	Citations int
}

// FacultySummary is the aggregate block written to the Summary sheet.
type FacultySummary struct {
	HIndex          int
	DocumentCount   int
	CitationTotal   int
	AvgCitation     float64
	YearMinCE       *int
	YearMaxCE       *int
	T1              int
	Q1              int
	Q2              int
	Q3              int
	Q4              int
	QuartileNA      int
	JournalCount    int
	ConferenceCount int
	BookCount       int
	ByYear          []FacultyYearCount // newest year first
}

// facultyTierBucket derives the quality tier for a document, matching the research dashboard:
// conference proceedings sit outside the tiers; a CiteScore percentile of 90–100 is T1; otherwise
// the CiteScore quartile (Q1..Q4) applies, falling back to N/A.
func facultyTierBucket(aggType string, percentile *float64, quartile string) string {
	if strings.EqualFold(strings.TrimSpace(aggType), "conference proceeding") {
		return "N/A"
	}
	if percentile != nil && *percentile >= 90 && *percentile <= 100 {
		return "T1"
	}
	switch strings.ToUpper(strings.TrimSpace(quartile)) {
	case "Q1", "Q2", "Q3", "Q4":
		return strings.ToUpper(strings.TrimSpace(quartile))
	default:
		return "N/A"
	}
}

// GetFacultyExport returns the deduped, KKU-only faculty document set (ranked for h-index) plus a
// summary block, for the XLSX export. It applies the same KKU-affiliation dedupe rule as
// GetFacultyGraph and joins CiteScore metrics using the dashboard's metric-year resolution.
func (s *AuthorHGraphService) GetFacultyExport(ctx context.Context, yearFrom, yearTo *int) ([]FacultyExportRow, *FacultySummary, error) {
	type scanRow struct {
		DocumentID          uint     `gorm:"column:document_id"`
		ScopusID            string   `gorm:"column:scopus_id"`
		Title               string   `gorm:"column:title"`
		Abstract            string   `gorm:"column:abstract"`
		AggregationType     string   `gorm:"column:aggregation_type"`
		CitedByCount        int      `gorm:"column:citedby_count"`
		AuthKeywords        []byte   `gorm:"column:authkeywords"`
		FundSponsor         string   `gorm:"column:fund_sponsor"`
		CiteScoreStatus     *string  `gorm:"column:cite_score_status"`
		CiteScoreRank       *int     `gorm:"column:cite_score_rank"`
		CiteScorePercentile *float64 `gorm:"column:cite_score_percentile"`
		CiteScoreQuartile   *string  `gorm:"column:cite_score_quartile"`
		YearCE              *int     `gorm:"column:year_ce"`
		EID                 string   `gorm:"column:eid"`
		ScopusLink          string   `gorm:"column:scopus_link"`
		DOI                 string   `gorm:"column:doi"`
	}

	yearExpr := yearExpression(s.db)
	metricYearExpr := metricYearForDocumentExpression(s.db)

	query := s.db.WithContext(ctx).
		Table("scopus_documents AS sd").
		Select("sd.id AS document_id, COALESCE(sd.scopus_id,'') AS scopus_id, COALESCE(sd.title,'') AS title, " +
			"COALESCE(sd.abstract,'') AS abstract, COALESCE(sd.aggregation_type,'') AS aggregation_type, " +
			"COALESCE(sd.citedby_count,0) AS citedby_count, sd.authkeywords AS authkeywords, " +
			"COALESCE(sd.fund_sponsor,'') AS fund_sponsor, " +
			"metrics.cite_score_status AS cite_score_status, metrics.cite_score_rank AS cite_score_rank, " +
			"metrics.cite_score_percentile AS cite_score_percentile, metrics.cite_score_quartile AS cite_score_quartile, " +
			yearExpr + " AS year_ce, COALESCE(sd.eid,'') AS eid, COALESCE(sd.scopus_link,'') AS scopus_link, COALESCE(sd.doi,'') AS doi").
		Joins("LEFT JOIN scopus_source_metrics AS metrics ON metrics.source_id = sd.source_id AND metrics.doc_type = 'all' AND metrics.metric_year = " + metricYearExpr).
		Where("EXISTS (?)", s.facultyKKUExists("sd"))
	if yearFrom != nil {
		query = query.Where(yearExpr+" >= ?", *yearFrom)
	}
	if yearTo != nil {
		query = query.Where(yearExpr+" <= ?", *yearTo)
	}

	var scanned []scanRow
	if err := query.Find(&scanned).Error; err != nil {
		return nil, nil, err
	}

	// Authors per document (author_seq order), deduped within a document, resolved in Go to avoid
	// GROUP_CONCAT truncation on papers with many authors.
	authorsByDoc := make(map[uint][]string)
	if len(scanned) > 0 {
		docIDs := make([]uint, 0, len(scanned))
		for _, r := range scanned {
			docIDs = append(docIDs, r.DocumentID)
		}
		type authorScan struct {
			DocumentID uint    `gorm:"column:document_id"`
			FullName   *string `gorm:"column:full_name"`
		}
		var arows []authorScan
		if err := s.db.WithContext(ctx).
			Table("scopus_document_authors AS sda").
			Select("sda.document_id AS document_id, sa.full_name AS full_name").
			Joins("JOIN scopus_authors AS sa ON sa.id = sda.author_id").
			Where("sda.document_id IN ?", docIDs).
			Order("sda.document_id ASC, sda.author_seq ASC").
			Find(&arows).Error; err != nil {
			return nil, nil, err
		}
		seen := make(map[uint]map[string]struct{})
		for _, a := range arows {
			if a.FullName == nil {
				continue
			}
			name := strings.TrimSpace(*a.FullName)
			if name == "" {
				continue
			}
			if seen[a.DocumentID] == nil {
				seen[a.DocumentID] = make(map[string]struct{})
			}
			if _, dup := seen[a.DocumentID][name]; dup {
				continue
			}
			seen[a.DocumentID][name] = struct{}{}
			authorsByDoc[a.DocumentID] = append(authorsByDoc[a.DocumentID], name)
		}
	}

	// Rank by citations desc (ties by year desc) — identical ordering to the h-graph, so
	// Order4Index lines up with the graph's document ranks.
	sort.SliceStable(scanned, func(i, j int) bool {
		if scanned[i].CitedByCount != scanned[j].CitedByCount {
			return scanned[i].CitedByCount > scanned[j].CitedByCount
		}
		yi, yj := 0, 0
		if scanned[i].YearCE != nil {
			yi = *scanned[i].YearCE
		}
		if scanned[j].YearCE != nil {
			yj = *scanned[j].YearCE
		}
		return yi > yj
	})

	rows := make([]FacultyExportRow, 0, len(scanned))
	summary := &FacultySummary{DocumentCount: len(scanned)}
	yearAgg := make(map[int]*FacultyYearCount)
	hIndex := 0
	for i, r := range scanned {
		rank := i + 1
		if r.CitedByCount >= rank {
			hIndex = rank
		}

		keywords := ""
		if len(r.AuthKeywords) > 0 {
			var arr []string
			if err := json.Unmarshal(r.AuthKeywords, &arr); err == nil {
				keywords = strings.Join(arr, "; ")
			} else {
				keywords = strings.TrimSpace(string(r.AuthKeywords))
			}
		}

		quartile := ""
		if r.CiteScoreQuartile != nil {
			quartile = strings.ToUpper(strings.TrimSpace(*r.CiteScoreQuartile))
		}
		tier := facultyTierBucket(r.AggregationType, r.CiteScorePercentile, quartile)

		var yBE *int
		if r.YearCE != nil {
			v := *r.YearCE + 543
			yBE = &v
		}

		scopusURL := strings.TrimSpace(r.ScopusLink)
		if scopusURL == "" && r.EID != "" {
			scopusURL = "https://www.scopus.com/record/display.uri?eid=" + r.EID + "&origin=resultslist"
		}
		doiURL := ""
		if doi := strings.TrimSpace(r.DOI); doi != "" {
			doiURL = "https://doi.org/" + doi
		}
		status := ""
		if r.CiteScoreStatus != nil {
			status = strings.TrimSpace(*r.CiteScoreStatus)
		}

		rows = append(rows, FacultyExportRow{
			ScopusID:            r.ScopusID,
			Title:               r.Title,
			Authors:             strings.Join(authorsByDoc[r.DocumentID], "; "),
			Abstract:            r.Abstract,
			AggregationType:     r.AggregationType,
			CitedByCount:        r.CitedByCount,
			AuthKeywords:        keywords,
			FundSponsor:         r.FundSponsor,
			CiteScoreStatus:     status,
			CiteScoreRank:       r.CiteScoreRank,
			CiteScorePercentile: r.CiteScorePercentile,
			JournalTierBucket:   tier,
			CiteScoreQuartile:   quartile,
			PublicationYearCE:   r.YearCE,
			PublicationYearBE:   yBE,
			EID:                 r.EID,
			ScopusURL:           scopusURL,
			DOIURL:              doiURL,
			Order4Index:         rank,
		})

		summary.CitationTotal += r.CitedByCount
		if r.YearCE != nil && *r.YearCE > 0 {
			y := *r.YearCE
			if summary.YearMinCE == nil || y < *summary.YearMinCE {
				yy := y
				summary.YearMinCE = &yy
			}
			if summary.YearMaxCE == nil || y > *summary.YearMaxCE {
				yy := y
				summary.YearMaxCE = &yy
			}
			yc := yearAgg[y]
			if yc == nil {
				yc = &FacultyYearCount{YearCE: y}
				yearAgg[y] = yc
			}
			yc.Count++
			yc.Citations += r.CitedByCount
		}
		switch tier {
		case "T1":
			summary.T1++
		case "Q1":
			summary.Q1++
		case "Q2":
			summary.Q2++
		case "Q3":
			summary.Q3++
		case "Q4":
			summary.Q4++
		default:
			summary.QuartileNA++
		}
		switch strings.ToLower(strings.TrimSpace(r.AggregationType)) {
		case "journal":
			summary.JournalCount++
		case "conference proceeding":
			summary.ConferenceCount++
		case "book", "book series":
			summary.BookCount++
		}
	}

	summary.HIndex = hIndex
	if summary.DocumentCount > 0 {
		summary.AvgCitation = math.Round(float64(summary.CitationTotal)/float64(summary.DocumentCount)*100) / 100
	}
	for _, yc := range yearAgg {
		summary.ByYear = append(summary.ByYear, *yc)
	}
	sort.SliceStable(summary.ByYear, func(i, j int) bool { return summary.ByYear[i].YearCE > summary.ByYear[j].YearCE })

	return rows, summary, nil
}

// AuthorSummaryRow is one row of the "all teachers h-index" export.
type AuthorSummaryRow struct {
	Rank                int     `json:"rank"`
	UserID              int     `json:"user_id"`
	Name                string  `json:"name"`
	ScopusAuthorID      string  `json:"scopus_author_id"`
	HIndex              int     `json:"h_index"`          // computed from ingested documents (matches the graph)
	DocumentCount       int     `json:"document_count"`   // documents in our system
	CitationTotal       int     `json:"citation_total"`   // sum of citedby_count
	YearMin             *int    `json:"year_min,omitempty"`
	YearMax             *int    `json:"year_max,omitempty"`
	ScopusHIndex        *int    `json:"scopus_h_index,omitempty"`         // official value from the Author API snapshot
	ScopusCitedByCount  *int    `json:"scopus_cited_by_count,omitempty"`
	ScopusCoauthorCount *int    `json:"scopus_coauthor_count,omitempty"`
	ScopusSnapshotDate  *string `json:"scopus_snapshot_date,omitempty"`
}

// GetAllSummary computes an h-index summary for every teacher that has a Scopus ID, joining the
// official Author API snapshot (scopus_author_metrics) when present. Rows are ranked by the
// computed h-index (the same number shown on the graph) descending.
func (s *AuthorHGraphService) GetAllSummary(ctx context.Context) ([]AuthorSummaryRow, error) {
	type teacher struct {
		UserID   int
		Name     string
		ScopusID string
	}
	var teachers []teacher
	if err := s.db.WithContext(ctx).Table("users").
		Select("user_id, TRIM(CONCAT(COALESCE(user_fname,''),' ',COALESCE(user_lname,''))) AS name, scopus_id AS scopus_id").
		Where("scopus_id IS NOT NULL AND scopus_id <> '' AND delete_at IS NULL").
		Order("user_id ASC").Find(&teachers).Error; err != nil {
		return nil, err
	}
	if len(teachers) == 0 {
		return []AuthorSummaryRow{}, nil
	}

	ids := make([]string, 0, len(teachers))
	for _, t := range teachers {
		ids = append(ids, t.ScopusID)
	}

	// All documents (citations + year) for those authors in one query.
	type docRow struct {
		ScopusAuthorID string
		Citations      int
		Year           *int
	}
	var docs []docRow
	if err := s.db.WithContext(ctx).
		Table("scopus_documents AS d").
		Select("a.scopus_author_id AS scopus_author_id, COALESCE(d.citedby_count, 0) AS citations, YEAR(d.cover_date) AS year").
		Joins("JOIN scopus_document_authors da ON da.document_id = d.id").
		Joins("JOIN scopus_authors a ON a.id = da.author_id").
		Where("a.scopus_author_id IN ?", ids).
		Find(&docs).Error; err != nil {
		return nil, err
	}
	byAuthor := make(map[string][]docRow)
	for _, d := range docs {
		byAuthor[d.ScopusAuthorID] = append(byAuthor[d.ScopusAuthorID], d)
	}

	// Latest Author API snapshot per author (best-effort; absence is fine).
	type metricRow struct {
		ScopusAuthorID string
		HIndex         *int
		CitedByCount   *int
		CoauthorCount  *int
		SnapshotDate   *time.Time
	}
	var metrics []metricRow
	_ = s.db.WithContext(ctx).Raw(`
		SELECT m.scopus_author_id, m.h_index, m.cited_by_count, m.coauthor_count, m.snapshot_date
		FROM scopus_author_metrics m
		JOIN (
			SELECT scopus_author_id, MAX(snapshot_date) AS md
			FROM scopus_author_metrics GROUP BY scopus_author_id
		) x ON x.scopus_author_id = m.scopus_author_id AND x.md = m.snapshot_date
	`).Scan(&metrics).Error
	metricByAuthor := make(map[string]metricRow)
	for _, m := range metrics {
		metricByAuthor[m.ScopusAuthorID] = m
	}

	rows := make([]AuthorSummaryRow, 0, len(teachers))
	for _, t := range teachers {
		ds := byAuthor[t.ScopusID]
		cites := make([]int, 0, len(ds))
		citationTotal := 0
		var ymin, ymax *int
		for _, d := range ds {
			cites = append(cites, d.Citations)
			citationTotal += d.Citations
			if d.Year != nil && *d.Year > 0 {
				if ymin == nil || *d.Year < *ymin {
					y := *d.Year
					ymin = &y
				}
				if ymax == nil || *d.Year > *ymax {
					y := *d.Year
					ymax = &y
				}
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(cites)))
		h := 0
		for i, c := range cites {
			if c >= i+1 {
				h = i + 1
			} else {
				break
			}
		}

		row := AuthorSummaryRow{
			UserID:         t.UserID,
			Name:           t.Name,
			ScopusAuthorID: t.ScopusID,
			HIndex:         h,
			DocumentCount:  len(ds),
			CitationTotal:  citationTotal,
			YearMin:        ymin,
			YearMax:        ymax,
		}
		if m, ok := metricByAuthor[t.ScopusID]; ok {
			row.ScopusHIndex = m.HIndex
			row.ScopusCitedByCount = m.CitedByCount
			row.ScopusCoauthorCount = m.CoauthorCount
			if m.SnapshotDate != nil {
				d := m.SnapshotDate.Format("2006-01-02")
				row.ScopusSnapshotDate = &d
			}
		}
		rows = append(rows, row)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].HIndex != rows[j].HIndex {
			return rows[i].HIndex > rows[j].HIndex
		}
		return rows[i].CitationTotal > rows[j].CitationTotal
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows, nil
}
