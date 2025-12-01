package models

import "time"

// ScopusSourceMetric stores journal-level metrics such as CiteScore, SJR, and SNIP.
type ScopusSourceMetric struct {
	ID                         uint       `gorm:"primaryKey;column:source_metric_id" json:"source_metric_id"`
	SourceID                   string     `gorm:"column:source_id" json:"source_id"`
	ISSN                       *string    `gorm:"column:issn" json:"issn,omitempty"`
	EISSN                      *string    `gorm:"column:eissn" json:"eissn,omitempty"`
	MetricYear                 int        `gorm:"column:metric_year" json:"metric_year"`
	DocType                    string     `gorm:"column:doc_type" json:"doc_type"`
	CiteScore                  *float64   `gorm:"column:cite_score" json:"cite_score,omitempty"`
	CiteScoreStatus            *string    `gorm:"column:cite_score_status" json:"cite_score_status,omitempty"`
	CiteScoreScholarlyOutput   *int       `gorm:"column:cite_score_scholarly_output" json:"cite_score_scholarly_output,omitempty"`
	CiteScoreCitationCount     *int       `gorm:"column:cite_score_citation_count" json:"cite_score_citation_count,omitempty"`
	CiteScorePercentCited      *float64   `gorm:"column:cite_score_percent_cited" json:"cite_score_percent_cited,omitempty"`
	CiteScoreRank              *int       `gorm:"column:cite_score_rank" json:"cite_score_rank,omitempty"`
	CiteScorePercentile        *float64   `gorm:"column:cite_score_percentile" json:"cite_score_percentile,omitempty"`
	CiteScoreQuartile          *string    `gorm:"column:cite_score_quartile" json:"cite_score_quartile,omitempty"`
	CiteScoreCurrentMetric     *float64   `gorm:"column:cite_score_current_metric" json:"cite_score_current_metric,omitempty"`
	CiteScoreCurrentMetricYear *int       `gorm:"column:cite_score_current_metric_year" json:"cite_score_current_metric_year,omitempty"`
	CiteScoreTracker           *float64   `gorm:"column:cite_score_tracker" json:"cite_score_tracker,omitempty"`
	CiteScoreTrackerYear       *int       `gorm:"column:cite_score_tracker_year" json:"cite_score_tracker_year,omitempty"`
	SJR                        *float64   `gorm:"column:sjr" json:"sjr,omitempty"`
	SNIP                       *float64   `gorm:"column:snip" json:"snip,omitempty"`
	PublicationCount           *int       `gorm:"column:publication_count" json:"publication_count,omitempty"`
	CiteCountSce               *int       `gorm:"column:cite_count_sce" json:"cite_count_sce,omitempty"`
	ZeroCitesSce               *float64   `gorm:"column:zero_cites_sce" json:"zero_cites_sce,omitempty"`
	RevPercent                 *float64   `gorm:"column:rev_percent" json:"rev_percent,omitempty"`
	CreatedAt                  time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt                  time.Time  `gorm:"column:updated_at" json:"updated_at"`
	LastFetchedAt              *time.Time `gorm:"column:last_fetched_at" json:"last_fetched_at,omitempty"`
}

// TableName overrides the table name used by ScopusSourceMetric to `scopus_source_metrics`.
func (ScopusSourceMetric) TableName() string {
	return "scopus_source_metrics"
}
