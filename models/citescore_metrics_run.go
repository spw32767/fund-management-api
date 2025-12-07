package models

import "time"

type CiteScoreMetricsRun struct {
	ID               uint64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	RunType          string     `json:"run_type" gorm:"column:run_type;type:varchar(32);not null"`
	Status           string     `json:"status" gorm:"column:status;type:varchar(32);not null;default:'running'"`
	ErrorMessage     *string    `json:"error_message,omitempty" gorm:"column:error_message;type:text"`
	SourcesScanned   int        `json:"sources_scanned" gorm:"column:sources_scanned;not null;default:0"`
	SourcesRefreshed int        `json:"sources_refreshed" gorm:"column:sources_refreshed;not null;default:0"`
	Skipped          int        `json:"skipped" gorm:"column:skipped;not null;default:0"`
	Errors           int        `json:"errors" gorm:"column:errors;not null;default:0"`
	JournalsScanned  int        `json:"journals_scanned" gorm:"column:journals_scanned;not null;default:0"`
	MetricsFetched   int        `json:"metrics_fetched" gorm:"column:metrics_fetched;not null;default:0"`
	SkippedExisting  int        `json:"skipped_existing" gorm:"column:skipped_existing;not null;default:0"`
	StartedAt        time.Time  `json:"started_at" gorm:"column:started_at;autoCreateTime"`
	FinishedAt       *time.Time `json:"finished_at,omitempty" gorm:"column:finished_at"`
	DurationSeconds  *float64   `json:"duration_seconds,omitempty" gorm:"column:duration_seconds"`
	CreatedAt        time.Time  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt        time.Time  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (CiteScoreMetricsRun) TableName() string { return "citescore_metrics_runs" }
