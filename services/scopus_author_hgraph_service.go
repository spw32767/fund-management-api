package services

import (
	"context"
	"errors"
	"sort"
	"strings"

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
		Citations int
		Year      *int
		Title     *string
		EID       string
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
