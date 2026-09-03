package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// BenchmarkQuartileBreakdown groups benchmark documents by their latest
// available CiteScore quartile.
type BenchmarkQuartileBreakdown struct {
	Q1           int `json:"q1"`
	Q2           int `json:"q2"`
	Q3           int `json:"q3"`
	Q4           int `json:"q4"`
	Unclassified int `json:"unclassified"`
}

// BenchmarkDocumentTypes keeps the dashboard categories deliberately broad so
// uncommon Scopus subtypes remain visible without making the chart unreadable.
type BenchmarkDocumentTypes struct {
	Article    int `json:"article"`
	Conference int `json:"conference"`
	Other      int `json:"other"`
}

// BenchmarkInsightLevel is the document-level summary for one comparison
// level. When no harvested documents exist, only Available is serialized.
type BenchmarkInsightLevel struct {
	Available bool                       `json:"available"`
	Docs      int                        `json:"docs"`
	OAPct     float64                    `json:"oa_pct"`
	IntlPct   float64                    `json:"intl_pct"`
	AvgCite   float64                    `json:"avg_cite"`
	Quartile  BenchmarkQuartileBreakdown `json:"quartile"`
	DocTypes  BenchmarkDocumentTypes     `json:"doctypes"`
}

// MarshalJSON keeps unavailable levels compact while ensuring zero-valued
// metrics remain present for an available level.
func (level BenchmarkInsightLevel) MarshalJSON() ([]byte, error) {
	if !level.Available {
		return json.Marshal(struct {
			Available bool `json:"available"`
		}{Available: false})
	}
	type alias BenchmarkInsightLevel
	return json.Marshal(alias(level))
}

// BenchmarkInsights is the complete deep-dive payload for a selected year.
type BenchmarkInsights struct {
	Year     int                              `json:"year"`
	Levels   map[string]BenchmarkInsightLevel `json:"levels"`
	Coverage struct {
		Classified int `json:"classified"`
		Total      int `json:"total"`
	} `json:"quartile_coverage"`
}

// BenchmarkTopJournal is one KKU publication venue ordered by document count.
type BenchmarkTopJournal struct {
	Name    string  `json:"name"`
	Docs    int     `json:"docs"`
	AvgCite float64 `json:"avg_cite"`
}

type benchmarkInsightRow struct {
	Docs         int     `gorm:"column:docs"`
	OAPct        float64 `gorm:"column:oa_pct"`
	IntlPct      float64 `gorm:"column:intl_pct"`
	AvgCite      float64 `gorm:"column:avg_cite"`
	Q1           int     `gorm:"column:q1"`
	Q2           int     `gorm:"column:q2"`
	Q3           int     `gorm:"column:q3"`
	Q4           int     `gorm:"column:q4"`
	Unclassified int     `gorm:"column:unclassified"`
	Article      int     `gorm:"column:article"`
	Conference   int     `gorm:"column:conference"`
	Other        int     `gorm:"column:other"`
}

func benchmarkInsightQuery(facultyOnly bool) string {
	facultyFilter := ""
	if facultyOnly {
		facultyFilter = `
		  AND EXISTS (
			SELECT 1
			FROM scopus_benchmark_document_authors AS faculty_da
			WHERE faculty_da.document_id = d.id AND faculty_da.is_faculty = 1
		  )`
	}

	return fmt.Sprintf(`
		SELECT
			COUNT(*) AS docs,
			COALESCE(ROUND(100.0 * SUM(CASE WHEN d.openaccess_flag = 1 THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 2), 0) AS oa_pct,
			COALESCE(ROUND(100.0 * SUM(CASE WHEN EXISTS (
				SELECT 1
				FROM scopus_benchmark_document_authors AS intl_da
				JOIN scopus_benchmark_affiliations AS intl_a ON intl_a.id = intl_da.affiliation_id
				WHERE intl_da.document_id = d.id
				  AND intl_a.country IS NOT NULL
				  AND TRIM(intl_a.country) <> ''
				  AND LOWER(TRIM(intl_a.country)) <> 'thailand'
			) THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 2), 0) AS intl_pct,
			COALESCE(ROUND(AVG(d.citedby_count), 2), 0) AS avg_cite,
			SUM(CASE WHEN UPPER(TRIM(m.cite_score_quartile)) = 'Q1' THEN 1 ELSE 0 END) AS q1,
			SUM(CASE WHEN UPPER(TRIM(m.cite_score_quartile)) = 'Q2' THEN 1 ELSE 0 END) AS q2,
			SUM(CASE WHEN UPPER(TRIM(m.cite_score_quartile)) = 'Q3' THEN 1 ELSE 0 END) AS q3,
			SUM(CASE WHEN UPPER(TRIM(m.cite_score_quartile)) = 'Q4' THEN 1 ELSE 0 END) AS q4,
			SUM(CASE WHEN UPPER(TRIM(COALESCE(m.cite_score_quartile, ''))) NOT IN ('Q1', 'Q2', 'Q3', 'Q4') THEN 1 ELSE 0 END) AS unclassified,
			SUM(CASE WHEN LOWER(TRIM(COALESCE(d.subtype_description, ''))) = 'article' THEN 1 ELSE 0 END) AS article,
			SUM(CASE WHEN LOWER(TRIM(COALESCE(d.subtype_description, ''))) = 'conference paper' THEN 1 ELSE 0 END) AS conference,
			SUM(CASE WHEN LOWER(TRIM(COALESCE(d.subtype_description, ''))) NOT IN ('article', 'conference paper') THEN 1 ELSE 0 END) AS other
		FROM scopus_benchmark_document_scopes AS bds
		JOIN scopus_benchmark_documents AS d ON d.id = bds.document_id
		LEFT JOIN scopus_source_metrics AS m
		  ON m.source_id = d.source_id
		 AND m.doc_type = 'all'
		 AND m.metric_year = (
			SELECT MAX(latest_m.metric_year)
			FROM scopus_source_metrics AS latest_m
			WHERE latest_m.source_id = d.source_id AND latest_m.doc_type = 'all'
		 )
		WHERE bds.scope_id = ? AND bds.pub_year = ?%s`, facultyFilter)
}

func (s *ScopusBenchmarkService) benchmarkInsightLevel(ctx context.Context, scopeID uint64, year int, facultyOnly bool) (BenchmarkInsightLevel, error) {
	var row benchmarkInsightRow
	if err := s.db.WithContext(ctx).Raw(benchmarkInsightQuery(facultyOnly), scopeID, year).Scan(&row).Error; err != nil {
		return BenchmarkInsightLevel{}, err
	}
	if row.Docs == 0 {
		return BenchmarkInsightLevel{Available: false}, nil
	}
	return BenchmarkInsightLevel{
		Available: true,
		Docs:      row.Docs,
		OAPct:     row.OAPct,
		IntlPct:   row.IntlPct,
		AvgCite:   row.AvgCite,
		Quartile: BenchmarkQuartileBreakdown{
			Q1: row.Q1, Q2: row.Q2, Q3: row.Q3, Q4: row.Q4, Unclassified: row.Unclassified,
		},
		DocTypes: BenchmarkDocumentTypes{
			Article: row.Article, Conference: row.Conference, Other: row.Other,
		},
	}, nil
}

// BenchmarkInsightsForYear reads the harvested benchmark dataset without
// mutating either benchmark or main Scopus dashboard tables.
func (s *ScopusBenchmarkService) BenchmarkInsightsForYear(ctx context.Context, year int) (BenchmarkInsights, error) {
	var result BenchmarkInsights
	result.Year = year
	result.Levels = make(map[string]BenchmarkInsightLevel, 3)

	levels := []struct {
		name        string
		scopeID     uint64
		facultyOnly bool
	}{
		{name: "faculty", scopeID: 1, facultyOnly: true},
		{name: "kku", scopeID: 1},
		{name: "thailand", scopeID: 2},
	}
	for _, definition := range levels {
		level, err := s.benchmarkInsightLevel(ctx, definition.scopeID, year, definition.facultyOnly)
		if err != nil {
			return result, fmt.Errorf("load %s benchmark insights: %w", definition.name, err)
		}
		result.Levels[definition.name] = level
		if level.Available {
			classified := level.Quartile.Q1 + level.Quartile.Q2 + level.Quartile.Q3 + level.Quartile.Q4
			result.Coverage.Classified += classified
			result.Coverage.Total += level.Docs
		}
	}

	return result, nil
}

// BenchmarkTopJournals returns the most frequent KKU publication venues across
// all harvested years. Empty publication names are intentionally excluded.
func (s *ScopusBenchmarkService) BenchmarkTopJournals(ctx context.Context, limit int) ([]BenchmarkTopJournal, error) {
	if limit < 1 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}

	var rows []BenchmarkTopJournal
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			TRIM(d.publication_name) AS name,
			COUNT(*) AS docs,
			COALESCE(ROUND(AVG(d.citedby_count), 2), 0) AS avg_cite
		FROM scopus_benchmark_document_scopes AS bds
		JOIN scopus_benchmark_documents AS d ON d.id = bds.document_id
		WHERE bds.scope_id = 1
		  AND d.publication_name IS NOT NULL
		  AND TRIM(d.publication_name) <> ''
		GROUP BY TRIM(d.publication_name)
		ORDER BY docs DESC, name ASC
		LIMIT ?`, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Name = strings.TrimSpace(rows[i].Name)
	}
	if rows == nil {
		rows = []BenchmarkTopJournal{}
	}
	return rows, nil
}
