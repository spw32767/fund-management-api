package models

import "time"

// ScopusAuthorMetric stores author-level metrics (h-index, citation/document counts)
// fetched from the Scopus Author Retrieval API. Rows are stored as daily snapshots so
// the current value is the latest row and historical values can be plotted over time.
type ScopusAuthorMetric struct {
	ID             uint      `gorm:"primaryKey;column:id" json:"id"`
	UserID         *int      `gorm:"column:user_id" json:"user_id,omitempty"`
	ScopusAuthorID string    `gorm:"column:scopus_author_id" json:"scopus_author_id"`
	HIndex         *int      `gorm:"column:h_index" json:"h_index,omitempty"`
	DocumentCount  *int      `gorm:"column:document_count" json:"document_count,omitempty"`
	CitedByCount   *int      `gorm:"column:cited_by_count" json:"cited_by_count,omitempty"`
	CitationCount  *int      `gorm:"column:citation_count" json:"citation_count,omitempty"`
	CoauthorCount  *int      `gorm:"column:coauthor_count" json:"coauthor_count,omitempty"`
	SnapshotDate   time.Time `gorm:"column:snapshot_date" json:"snapshot_date"`
	RawJSON        []byte    `gorm:"column:raw_json" json:"-"`
	FetchedAt      time.Time `gorm:"column:fetched_at" json:"fetched_at"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// TableName overrides the table name used by ScopusAuthorMetric to `scopus_author_metrics`.
func (ScopusAuthorMetric) TableName() string {
	return "scopus_author_metrics"
}
