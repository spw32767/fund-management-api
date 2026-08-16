package services

import (
	"context"
	"errors"
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
