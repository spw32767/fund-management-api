package models

import "time"

const (
	ScopusAuthorMetricsRunStatusRunning = "running"
	ScopusAuthorMetricsRunStatusSuccess = "success"
	ScopusAuthorMetricsRunStatusFailed  = "failed"
)

// ScopusAuthorMetricsRun summarises a single batch run of the Scopus author-metrics ingest.
type ScopusAuthorMetricsRun struct {
	ID              uint64     `gorm:"primaryKey;column:id" json:"id"`
	TriggerSource   string     `gorm:"column:trigger_source" json:"trigger_source"`
	Status          string     `gorm:"column:status" json:"status"`
	ErrorMessage    *string    `gorm:"column:error_message" json:"error_message,omitempty"`
	UsersProcessed  int        `gorm:"column:users_processed" json:"users_processed"`
	UsersWithErrors int        `gorm:"column:users_with_errors" json:"users_with_errors"`
	MetricsUpserted int        `gorm:"column:metrics_upserted" json:"metrics_upserted"`
	NotFound        int        `gorm:"column:not_found" json:"not_found"`
	APICalls        int        `gorm:"column:api_calls" json:"api_calls"`
	StartedAt       time.Time  `gorm:"column:started_at" json:"started_at"`
	FinishedAt      *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	DurationSeconds *float64   `gorm:"column:duration_seconds" json:"duration_seconds,omitempty"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

// TableName overrides the table name used by ScopusAuthorMetricsRun.
func (ScopusAuthorMetricsRun) TableName() string {
	return "scopus_author_metrics_runs"
}

// ScopusAuthorMetricRequest logs a single HTTP request to the Scopus Author Retrieval API.
type ScopusAuthorMetricRequest struct {
	ID             uint64    `gorm:"primaryKey;column:id" json:"id"`
	RunID          *uint64   `gorm:"column:run_id" json:"run_id,omitempty"`
	UserID         *int      `gorm:"column:user_id" json:"user_id,omitempty"`
	ScopusAuthorID string    `gorm:"column:scopus_author_id" json:"scopus_author_id"`
	HTTPMethod     string    `gorm:"column:http_method" json:"http_method"`
	Endpoint       string    `gorm:"column:endpoint" json:"endpoint"`
	ResponseStatus *int      `gorm:"column:response_status" json:"response_status,omitempty"`
	ResponseTimeMs *int      `gorm:"column:response_time_ms" json:"response_time_ms,omitempty"`
	HIndex         *int      `gorm:"column:h_index" json:"h_index,omitempty"`
	ErrorMessage   *string   `gorm:"column:error_message" json:"error_message,omitempty"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName overrides the table name used by ScopusAuthorMetricRequest.
func (ScopusAuthorMetricRequest) TableName() string {
	return "scopus_author_metric_requests"
}
