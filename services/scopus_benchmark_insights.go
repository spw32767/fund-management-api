package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"fund-management-api/models"
)

// BenchmarkQuartileBreakdown groups benchmark documents by their journal tier,
// using the same rule as the Scopus dashboard: conference proceedings sit outside
// the tiers, a CiteScore percentile of 90–100 is T1, otherwise the CiteScore
// quartile applies. T1 is carved out of Q1 so a paper is never counted in both.
type BenchmarkQuartileBreakdown struct {
	T1 int `json:"t1"`
	Q1 int `json:"q1"`
	Q2 int `json:"q2"`
	Q3 int `json:"q3"`
	Q4 int `json:"q4"`
	// Unclassified is the legacy field (docs − classified) kept for compatibility.
	// The precise split below separates eligible journals without a CiteScore metric,
	// non-journal documents (books/conference proceedings), and unresolved types so
	// journal-metadata coverage can be read honestly (§5 F / §9 B).
	Unclassified        int `json:"unclassified"`
	UnclassifiedJournal int `json:"unclassified_journal"`
	ExcludedNonJournal  int `json:"excluded_non_journal"`
	Unresolved          int `json:"unresolved"`
}

// BenchmarkCoverageCounts reports how much of a boolean metric is actually known,
// so an unknown value is never silently treated as a negative (§8/§9 B).
type BenchmarkCoverageCounts struct {
	Known    int `json:"known"`
	Positive int `json:"positive"`
	Unknown  int `json:"unknown"`
}

// BenchmarkDocumentTypes keeps the dashboard categories deliberately broad so
// uncommon Scopus subtypes remain visible without making the chart unreadable.
type BenchmarkDocumentTypes struct {
	Article    int `json:"article"`
	Conference int `json:"conference"`
	Other      int `json:"other"`
}

// BenchmarkCitationSummary reports the cumulative citations of the documents
// published in the selected year for one level. It is a running total as of the
// last data update, NOT citations that occurred within the year, and the three
// levels overlap so their totals must never be summed. See handoff §5 E2 / §9 D.
type BenchmarkCitationSummary struct {
	Total             *int64   `json:"total"`
	Average           *float64 `json:"average"`
	KnownDocs         int      `json:"known_docs"`
	CohortDocs        int      `json:"cohort_docs"`
	UnknownDocs       int      `json:"unknown_docs"`
	CoverageStatus    string   `json:"coverage_status"`    // complete | partial | none
	DenominatorPolicy string   `json:"denominator_policy"` // known_citation_docs
	// UpdatedAt/UpdateRange are nil because no stored timestamp provably reflects a
	// citation refresh (row-updated timestamps are not a citation refresh — §9 B/D).
	UpdatedAt       *string `json:"updated_at"`
	UpdateRange     *string `json:"update_range"`
	FreshnessStatus string  `json:"freshness_status"` // unknown
}

// BenchmarkMetricReadiness is per-metric readiness (count / quality / intl / oa /
// citations). A metric is ready to compare only when the harvest is complete AND
// that metric's own metadata is complete — so an unknown OA or an unclassified
// journal blocks the quality/OA gap without hiding metrics that are fine (§9 B/C).
type BenchmarkMetricReadiness struct {
	Ready   bool     `json:"ready"`
	Reasons []string `json:"reasons"`
}

// BenchmarkLevelReadiness states whether comparative narrative/gap statements may
// use this level, and why not when they cannot. Readiness is per level AND per
// metric — never a single boolean that hides one metric behind another (§9 B/C).
type BenchmarkLevelReadiness struct {
	ComparisonReady  bool                                `json:"comparison_ready"` // base/harvest readiness (== metric "count")
	ActiveRun        bool                                `json:"active_run"`
	SnapshotMismatch bool                                `json:"snapshot_mismatch"`
	ExpectedDocs     *int                                `json:"expected_docs"`
	ObservedDocs     int                                 `json:"observed_docs"`
	Reasons          []string                            `json:"reasons"`
	Metrics          map[string]BenchmarkMetricReadiness `json:"metrics"`
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
	OA        BenchmarkCoverageCounts    `json:"oa"`
	Intl      BenchmarkCoverageCounts    `json:"intl"`
	Citations BenchmarkCitationSummary   `json:"citations"`
	Readiness BenchmarkLevelReadiness    `json:"readiness"`
}

// MarshalJSON keeps unavailable levels compact while ensuring zero-valued
// metrics remain present for an available level. Citation and readiness metadata
// still ship for an unavailable level so the UI can render an honest empty state.
func (level BenchmarkInsightLevel) MarshalJSON() ([]byte, error) {
	if !level.Available {
		return json.Marshal(struct {
			Available bool                     `json:"available"`
			Citations BenchmarkCitationSummary `json:"citations"`
			Readiness BenchmarkLevelReadiness  `json:"readiness"`
		}{Available: false, Citations: level.Citations, Readiness: level.Readiness})
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
	Scope struct {
		SubjectArea       string `json:"subject_area"`
		FacultyScopeID    uint64 `json:"faculty_scope_id"`
		UniversityScopeID uint64 `json:"university_scope_id"`
		CountryScopeID    uint64 `json:"country_scope_id"`
	} `json:"scope"`
}

// BenchmarkTopJournal is one KKU publication venue ordered by document count.
type BenchmarkTopJournal struct {
	Name    string  `json:"name"`
	Docs    int     `json:"docs"`
	AvgCite float64 `json:"avg_cite"`
}

type benchmarkInsightRow struct {
	Docs               int     `gorm:"column:docs"`
	OAPct              float64 `gorm:"column:oa_pct"`
	IntlPct            float64 `gorm:"column:intl_pct"`
	AvgCite            float64 `gorm:"column:avg_cite"`
	OAKnown            int     `gorm:"column:oa_known"`
	OAPositive         int     `gorm:"column:oa_positive"`
	IntlKnown          int     `gorm:"column:intl_known"`
	IntlPositive       int     `gorm:"column:intl_positive"`
	T1                 int     `gorm:"column:t1"`
	Q1                 int     `gorm:"column:q1"`
	Q2                 int     `gorm:"column:q2"`
	Q3                 int     `gorm:"column:q3"`
	Q4                 int     `gorm:"column:q4"`
	JournalDocs        int     `gorm:"column:journal_docs"`
	ExcludedNonJournal int     `gorm:"column:excluded_non_journal"`
	UnresolvedType     int     `gorm:"column:unresolved_type"`
	Article            int     `gorm:"column:article"`
	Conference         int     `gorm:"column:conference"`
	Other              int     `gorm:"column:other"`
}

type benchmarkCitationRow struct {
	CohortDocs int    `gorm:"column:cohort_docs"`
	KnownDocs  int    `gorm:"column:known_docs"`
	Total      *int64 `gorm:"column:total"`
}

// benchmarkFacultyExistsClause is the verified-faculty membership test, mirroring
// verifiedFacultyCountQuery so the insights faculty cohort matches the official
// count exactly (distinct KKU/COMP benchmark document authored by a registered
// faculty member with KKU AF-ID, respecting the employment-date gate when known).
// It is applied to the KKU (university) benchmark scope, NOT the is_faculty flag.
const benchmarkFacultyExistsClause = `
	  AND EXISTS (
		SELECT 1
		FROM scopus_documents AS sd
		JOIN scopus_document_authors AS sda ON sda.document_id = sd.id
		JOIN scopus_authors AS sa ON sa.id = sda.author_id
		JOIN users AS u ON TRIM(u.scopus_id) = sa.scopus_author_id
		JOIN scopus_affiliations AS aff ON aff.id = sda.affiliation_id
		WHERE sd.eid = d.eid
		  AND u.delete_at IS NULL
		  AND u.scopus_id IS NOT NULL
		  AND TRIM(u.scopus_id) <> ''
		  AND (
			u.date_of_employment IS NULL
			OR (sd.cover_date IS NOT NULL AND DATE(sd.cover_date) >= DATE(u.date_of_employment))
		  )
		  AND TRIM(aff.afid) IN (?, ?)
	  )`

// benchmarkInsightQuery aggregates one level's document metrics for a scope/year.
// When facultyVerified is true it appends the verified-faculty EXISTS clause AFTER
// the WHERE, so the placeholder order is scope_id, pub_year, then the two AF-IDs
// (see insightArgs).
func benchmarkInsightQuery(facultyVerified bool) string {
	facultyFilter := ""
	if facultyVerified {
		facultyFilter = benchmarkFacultyExistsClause
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
			-- OA coverage: openaccess_flag is nullable, so a NULL is genuinely unknown
			-- and is NOT counted as "not OA" (§8/§9 B). known/positive let the UI show
			-- an observed rate over known docs instead of forcing unknown to false.
			SUM(CASE WHEN d.openaccess_flag IS NOT NULL THEN 1 ELSE 0 END) AS oa_known,
			SUM(CASE WHEN d.openaccess_flag = 1 THEN 1 ELSE 0 END) AS oa_positive,
			-- Intl coverage: a doc with at least one recorded affiliation country is
			-- "known"; a doc with no country data at all is unknown (not domestic).
			SUM(CASE WHEN EXISTS (
				SELECT 1 FROM scopus_benchmark_document_authors AS k_da
				JOIN scopus_benchmark_affiliations AS k_a ON k_a.id = k_da.affiliation_id
				WHERE k_da.document_id = d.id AND k_a.country IS NOT NULL AND TRIM(k_a.country) <> ''
			) THEN 1 ELSE 0 END) AS intl_known,
			SUM(CASE WHEN EXISTS (
				SELECT 1 FROM scopus_benchmark_document_authors AS p_da
				JOIN scopus_benchmark_affiliations AS p_a ON p_a.id = p_da.affiliation_id
				WHERE p_da.document_id = d.id AND p_a.country IS NOT NULL AND TRIM(p_a.country) <> ''
				  AND LOWER(TRIM(p_a.country)) <> 'thailand'
			) THEN 1 ELSE 0 END) AS intl_positive,
			-- Journal tiers are computed ONLY for aggregation_type = 'journal'; books,
			-- book series and conference proceedings are excluded, not tiered (§9 B).
			SUM(CASE WHEN LOWER(TRIM(COALESCE(d.aggregation_type, ''))) = 'journal'
				AND m.cite_score_percentile >= 90 AND m.cite_score_percentile <= 100 THEN 1 ELSE 0 END) AS t1,
			SUM(CASE WHEN LOWER(TRIM(COALESCE(d.aggregation_type, ''))) = 'journal'
				AND UPPER(TRIM(m.cite_score_quartile)) = 'Q1'
				AND (m.cite_score_percentile IS NULL OR m.cite_score_percentile < 90) THEN 1 ELSE 0 END) AS q1,
			SUM(CASE WHEN LOWER(TRIM(COALESCE(d.aggregation_type, ''))) = 'journal'
				AND UPPER(TRIM(m.cite_score_quartile)) = 'Q2'
				AND (m.cite_score_percentile IS NULL OR m.cite_score_percentile < 90) THEN 1 ELSE 0 END) AS q2,
			SUM(CASE WHEN LOWER(TRIM(COALESCE(d.aggregation_type, ''))) = 'journal'
				AND UPPER(TRIM(m.cite_score_quartile)) = 'Q3'
				AND (m.cite_score_percentile IS NULL OR m.cite_score_percentile < 90) THEN 1 ELSE 0 END) AS q3,
			SUM(CASE WHEN LOWER(TRIM(COALESCE(d.aggregation_type, ''))) = 'journal'
				AND UPPER(TRIM(m.cite_score_quartile)) = 'Q4'
				AND (m.cite_score_percentile IS NULL OR m.cite_score_percentile < 90) THEN 1 ELSE 0 END) AS q4,
			SUM(CASE WHEN LOWER(TRIM(COALESCE(d.aggregation_type, ''))) = 'journal' THEN 1 ELSE 0 END) AS journal_docs,
			SUM(CASE WHEN d.aggregation_type IS NOT NULL AND TRIM(d.aggregation_type) <> ''
				AND LOWER(TRIM(d.aggregation_type)) <> 'journal' THEN 1 ELSE 0 END) AS excluded_non_journal,
			SUM(CASE WHEN d.aggregation_type IS NULL OR TRIM(d.aggregation_type) = '' THEN 1 ELSE 0 END) AS unresolved_type,
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

// benchmarkCitationQuery counts the cohort of distinct documents published in the
// selected year for a scope and sums their stored citation counts. It deliberately
// avoids the source-metrics join so a source with multiple metric rows can never
// duplicate a document (§9 D). known_docs counts non-NULL citation values, so real
// zeros are kept while genuinely-unknown citations are excluded from the average.
func benchmarkCitationQuery(facultyVerified bool) string {
	facultyFilter := ""
	if facultyVerified {
		facultyFilter = benchmarkFacultyExistsClause
	}
	return fmt.Sprintf(`
		SELECT
			COUNT(*) AS cohort_docs,
			SUM(CASE WHEN d.citedby_count IS NOT NULL THEN 1 ELSE 0 END) AS known_docs,
			SUM(d.citedby_count) AS total
		FROM scopus_benchmark_document_scopes AS bds
		JOIN scopus_benchmark_documents AS d ON d.id = bds.document_id
		WHERE bds.scope_id = ? AND bds.pub_year = ?%s`, facultyFilter)
}

// insightArgs assembles positional args for the two queries above, in the exact
// order the placeholders appear in the SQL: the WHERE clause (`bds.scope_id = ?
// AND bds.pub_year = ?`) comes first, and the verified-faculty EXISTS clause —
// which is appended AFTER the WHERE via %s — supplies its two AF-IDs last. Getting
// this order wrong silently binds the scope/year into the AF-ID filter and yields
// an empty faculty cohort, so it is covered by a bound-argument test.
func insightArgs(scopeID uint64, year int, facultyVerified bool) []interface{} {
	if facultyVerified {
		return []interface{}{scopeID, year, benchmarkKKUAfID, benchmarkKKUScienceAfID}
	}
	return []interface{}{scopeID, year}
}

// computeCitationSummary applies the deterministic citation rules (§8/§9 D) to the
// raw cohort counts. total/average are nil when nothing usable is known; a cohort
// whose known citations are all real zeros yields total=0, average=0.
func computeCitationSummary(cohortDocs, knownDocs int, total *int64) BenchmarkCitationSummary {
	summary := BenchmarkCitationSummary{
		CohortDocs:        cohortDocs,
		KnownDocs:         knownDocs,
		UnknownDocs:       cohortDocs - knownDocs,
		DenominatorPolicy: "known_citation_docs",
		FreshnessStatus:   "unknown",
	}
	switch {
	case cohortDocs == 0:
		summary.CoverageStatus = "none"
	case summary.UnknownDocs == 0:
		summary.CoverageStatus = "complete"
	default:
		summary.CoverageStatus = "partial"
	}
	if knownDocs > 0 {
		sum := int64(0)
		if total != nil {
			sum = *total
		}
		value := sum
		summary.Total = &value
		avg := float64(sum) / float64(knownDocs)
		summary.Average = &avg
	}
	return summary
}

func (s *ScopusBenchmarkService) benchmarkInsightLevel(ctx context.Context, scopeID uint64, year int, facultyVerified bool) (BenchmarkInsightLevel, error) {
	var row benchmarkInsightRow
	if err := s.db.WithContext(ctx).
		Raw(benchmarkInsightQuery(facultyVerified), insightArgs(scopeID, year, facultyVerified)...).
		Scan(&row).Error; err != nil {
		return BenchmarkInsightLevel{}, err
	}

	var citationRow benchmarkCitationRow
	if err := s.db.WithContext(ctx).
		Raw(benchmarkCitationQuery(facultyVerified), insightArgs(scopeID, year, facultyVerified)...).
		Scan(&citationRow).Error; err != nil {
		return BenchmarkInsightLevel{}, err
	}
	citations := computeCitationSummary(citationRow.CohortDocs, citationRow.KnownDocs, citationRow.Total)

	if row.Docs == 0 {
		return BenchmarkInsightLevel{Available: false, Citations: citations}, nil
	}
	return BenchmarkInsightLevel{
		Available: true,
		Docs:      row.Docs,
		OAPct:     row.OAPct,
		IntlPct:   row.IntlPct,
		AvgCite:   row.AvgCite,
		Quartile: BenchmarkQuartileBreakdown{
			T1: row.T1, Q1: row.Q1, Q2: row.Q2, Q3: row.Q3, Q4: row.Q4,
			// Legacy: everything not in a journal tier (kept for compatibility).
			Unclassified: row.Docs - (row.T1 + row.Q1 + row.Q2 + row.Q3 + row.Q4),
			// Eligible journals with no CiteScore metric (journal docs minus tiered).
			UnclassifiedJournal: row.JournalDocs - (row.T1 + row.Q1 + row.Q2 + row.Q3 + row.Q4),
			ExcludedNonJournal:  row.ExcludedNonJournal,
			Unresolved:          row.UnresolvedType,
		},
		DocTypes: BenchmarkDocumentTypes{
			Article: row.Article, Conference: row.Conference, Other: row.Other,
		},
		OA:        BenchmarkCoverageCounts{Known: row.OAKnown, Positive: row.OAPositive, Unknown: row.Docs - row.OAKnown},
		Intl:      BenchmarkCoverageCounts{Known: row.IntlKnown, Positive: row.IntlPositive, Unknown: row.Docs - row.IntlKnown},
		Citations: citations,
	}, nil
}

// insightScopeSet resolves the three benchmark scopes from their level (never from
// hardcoded ids 1/2 — §4/§9). The faculty cohort is derived from the KKU/university
// scope plus the verified-faculty EXISTS clause, so it needs no separate scope row.
type insightScopeSet struct {
	Faculty    models.ScopusBenchmarkScope
	University models.ScopusBenchmarkScope
	Country    models.ScopusBenchmarkScope
}

func (s *ScopusBenchmarkService) resolveInsightScopes(ctx context.Context) (insightScopeSet, error) {
	var set insightScopeSet
	if err := s.db.WithContext(ctx).Where("level = ?", "faculty").First(&set.Faculty).Error; err != nil {
		return set, fmt.Errorf("resolve faculty scope: %w", err)
	}
	if err := s.db.WithContext(ctx).Where("level = ?", "university").First(&set.University).Error; err != nil {
		return set, fmt.Errorf("resolve university scope: %w", err)
	}
	if err := s.db.WithContext(ctx).Where("level = ?", "country").First(&set.Country).Error; err != nil {
		return set, fmt.Errorf("resolve country scope: %w", err)
	}
	return set, nil
}

// levelReadinessInputs carries the cross-level facts needed to decide readiness
// without re-querying per level.
type levelReadinessInputs struct {
	facultyMetricReady   bool
	facultyYearMissing   bool
	kkuHarvestActive     bool
	countryHarvestActive bool
}

// levelMetricMeta is the per-metric metadata computeLevelReadiness needs to decide
// whether each metric's comparison is trustworthy for this level/year.
type levelMetricMeta struct {
	available           bool
	observedDocs        int
	unclassifiedJournal int // eligible journals with no CiteScore metric
	unresolved          int // documents with an unresolved source type
	oaUnknown           int // docs with unknown open-access status
	intlUnknown         int // docs with no affiliation country data
	citationUnknown     int // docs with unknown citation count
}

// computeLevelReadiness decides readiness per metric, not with one boolean (§9 B/C).
// The BASE (harvest) readiness requires the level to be available, have no active
// harvest, and have a count snapshot that matches the harvested docs (a nil snapshot
// means completeness cannot be proven, so it is NOT ready). Each metric then also
// requires its own metadata to be complete. Observed values still render; only the
// comparison/gap/narrative is withheld.
func computeLevelReadiness(level string, meta levelMetricMeta, expectedDocs *int, in levelReadinessInputs) BenchmarkLevelReadiness {
	readiness := BenchmarkLevelReadiness{Reasons: []string{}, ObservedDocs: meta.observedDocs, ExpectedDocs: expectedDocs}
	if expectedDocs != nil && *expectedDocs != meta.observedDocs {
		readiness.SnapshotMismatch = true
	}

	base := []string{}
	switch level {
	case "faculty":
		readiness.ActiveRun = in.kkuHarvestActive
		if !meta.available {
			base = append(base, "no faculty documents for this year")
		}
		if !in.facultyMetricReady {
			base = append(base, "faculty metric not ready (no verified faculty users)")
		}
		if in.facultyYearMissing {
			base = append(base, "KKU benchmark documents incomplete for this year")
		}
		if in.kkuHarvestActive {
			base = append(base, "KKU harvest in progress")
		}
	case "kku":
		readiness.ActiveRun = in.kkuHarvestActive
		if !meta.available {
			base = append(base, "no KKU documents for this year")
		}
		if in.kkuHarvestActive {
			base = append(base, "KKU harvest in progress")
		}
	case "country":
		readiness.ActiveRun = in.countryHarvestActive
		if !meta.available {
			base = append(base, "no Thailand documents for this year")
		}
		if in.countryHarvestActive {
			base = append(base, "Thailand harvest in progress")
		}
	}
	if readiness.SnapshotMismatch {
		base = append(base, fmt.Sprintf("harvest incomplete: count snapshot has %d docs but %d were harvested for this year", *expectedDocs, meta.observedDocs))
	}
	if expectedDocs == nil {
		base = append(base, "no count snapshot to verify completeness")
	}

	baseReady := len(base) == 0
	metric := func(extra []string) BenchmarkMetricReadiness {
		reasons := append(append([]string{}, base...), extra...)
		return BenchmarkMetricReadiness{Ready: len(reasons) == 0, Reasons: reasons}
	}

	quality := []string{}
	if meta.unclassifiedJournal > 0 {
		quality = append(quality, fmt.Sprintf("%d journals have no CiteScore metadata", meta.unclassifiedJournal))
	}
	if meta.unresolved > 0 {
		quality = append(quality, fmt.Sprintf("%d documents have an unresolved source type", meta.unresolved))
	}
	oa := []string{}
	if meta.oaUnknown > 0 {
		oa = append(oa, fmt.Sprintf("%d documents have unknown open-access status", meta.oaUnknown))
	}
	intl := []string{}
	if meta.intlUnknown > 0 {
		intl = append(intl, fmt.Sprintf("%d documents have no affiliation country data", meta.intlUnknown))
	}
	citations := []string{}
	if meta.citationUnknown > 0 {
		citations = append(citations, fmt.Sprintf("%d documents have unknown citation counts", meta.citationUnknown))
	}

	readiness.Metrics = map[string]BenchmarkMetricReadiness{
		"count":     metric(nil),
		"quality":   metric(quality),
		"intl":      metric(intl),
		"oa":        metric(oa),
		"citations": metric(citations),
	}
	readiness.Reasons = base
	readiness.ComparisonReady = baseReady
	return readiness
}

// benchmarkSnapshotTotal returns the latest count-snapshot total for a scope/year,
// or nil when no snapshot exists. Used to detect harvest/snapshot mismatch per level.
func (s *ScopusBenchmarkService) benchmarkSnapshotTotal(ctx context.Context, scopeID uint64, year int) (*int, error) {
	var rows []int
	if err := s.db.WithContext(ctx).Raw(`
		SELECT s.total_results
		FROM scopus_benchmark_count_snapshots AS s
		JOIN (
			SELECT MAX(id) AS max_id
			FROM scopus_benchmark_count_snapshots
			WHERE scope_id = ? AND pub_year = ?
		) AS latest ON latest.max_id = s.id`, scopeID, year).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load snapshot total: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	total := rows[0]
	return &total, nil
}

// BenchmarkInsightsForYear reads the harvested benchmark dataset without
// mutating either benchmark or main Scopus dashboard tables.
func (s *ScopusBenchmarkService) BenchmarkInsightsForYear(ctx context.Context, year int) (BenchmarkInsights, error) {
	var result BenchmarkInsights
	result.Year = year
	result.Levels = make(map[string]BenchmarkInsightLevel, 3)

	scopes, err := s.resolveInsightScopes(ctx)
	if err != nil {
		return result, err
	}
	result.Scope.SubjectArea = benchmarkSubjectDefault
	if trimmed := strings.TrimSpace(scopes.University.SubjectArea); trimmed != "" {
		result.Scope.SubjectArea = trimmed
	}
	result.Scope.FacultyScopeID = scopes.Faculty.ID
	result.Scope.UniversityScopeID = scopes.University.ID
	result.Scope.CountryScopeID = scopes.Country.ID

	// Readiness inputs: verified-faculty coverage for this single year plus the
	// active-harvest guard, reusing the same rules as the KPI count path.
	facultyCoverage, err := s.FacultyMetricCoverage(ctx, year, year)
	if err != nil {
		return result, err
	}
	facultyYearMissing := false
	for _, missing := range facultyCoverage.BenchmarkYearsMissing {
		if missing == year {
			facultyYearMissing = true
			break
		}
	}
	activeRun, err := s.GetActiveRun(ctx)
	if err != nil {
		return result, err
	}
	readinessInputs := levelReadinessInputs{
		facultyMetricReady: facultyCoverage.Ready,
		facultyYearMissing: facultyYearMissing,
	}
	if activeRun != nil {
		if activeRun.ScopeID == nil || *activeRun.ScopeID == scopes.University.ID {
			readinessInputs.kkuHarvestActive = true
		}
		if activeRun.ScopeID != nil && *activeRun.ScopeID == scopes.Country.ID {
			readinessInputs.countryHarvestActive = true
		}
	}

	levels := []struct {
		name            string
		scopeID         uint64
		snapshotScopeID uint64
		facultyVerified bool
	}{
		{name: "faculty", scopeID: scopes.University.ID, snapshotScopeID: scopes.Faculty.ID, facultyVerified: true},
		{name: "kku", scopeID: scopes.University.ID, snapshotScopeID: scopes.University.ID},
		{name: "thailand", scopeID: scopes.Country.ID, snapshotScopeID: scopes.Country.ID},
	}
	for _, definition := range levels {
		level, err := s.benchmarkInsightLevel(ctx, definition.scopeID, year, definition.facultyVerified)
		if err != nil {
			return result, fmt.Errorf("load %s benchmark insights: %w", definition.name, err)
		}
		expectedDocs, err := s.benchmarkSnapshotTotal(ctx, definition.snapshotScopeID, year)
		if err != nil {
			return result, fmt.Errorf("load %s snapshot total: %w", definition.name, err)
		}
		readinessKey := definition.name
		if readinessKey == "thailand" {
			readinessKey = "country"
		}
		level.Readiness = computeLevelReadiness(readinessKey, levelMetricMeta{
			available:           level.Available,
			observedDocs:        level.Docs,
			unclassifiedJournal: level.Quartile.UnclassifiedJournal,
			unresolved:          level.Quartile.Unresolved,
			oaUnknown:           level.OA.Unknown,
			intlUnknown:         level.Intl.Unknown,
			citationUnknown:     level.Citations.UnknownDocs,
		}, expectedDocs, readinessInputs)
		result.Levels[definition.name] = level
		if level.Available {
			classified := level.Quartile.T1 + level.Quartile.Q1 + level.Quartile.Q2 + level.Quartile.Q3 + level.Quartile.Q4
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

	// Unchanged from the original (top-journals is not part of the executive report
	// UI — §5 F — so its scope stays as-is to avoid needless contract churn).
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
