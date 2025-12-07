package models

import "time"

type ScopusBatchImportRun struct {
	ID                  uint64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Status              string     `json:"status" gorm:"column:status;type:varchar(32);not null;default:'running'"`
	ErrorMessage        *string    `json:"error_message,omitempty" gorm:"column:error_message;type:text"`
	RequestedUserIDs    *string    `json:"requested_user_ids,omitempty" gorm:"column:requested_user_ids;type:text"`
	Limit               *int       `json:"limit,omitempty" gorm:"column:limit"`
	UsersProcessed      int        `json:"users_processed" gorm:"column:users_processed;not null;default:0"`
	UsersWithErrors     int        `json:"users_with_errors" gorm:"column:users_with_errors;not null;default:0"`
	DocumentsFetched    int        `json:"documents_fetched" gorm:"column:documents_fetched;not null;default:0"`
	DocumentsCreated    int        `json:"documents_created" gorm:"column:documents_created;not null;default:0"`
	DocumentsUpdated    int        `json:"documents_updated" gorm:"column:documents_updated;not null;default:0"`
	DocumentsFailed     int        `json:"documents_failed" gorm:"column:documents_failed;not null;default:0"`
	AuthorsCreated      int        `json:"authors_created" gorm:"column:authors_created;not null;default:0"`
	AuthorsUpdated      int        `json:"authors_updated" gorm:"column:authors_updated;not null;default:0"`
	AffiliationsCreated int        `json:"affiliations_created" gorm:"column:affiliations_created;not null;default:0"`
	AffiliationsUpdated int        `json:"affiliations_updated" gorm:"column:affiliations_updated;not null;default:0"`
	LinksInserted       int        `json:"links_inserted" gorm:"column:links_inserted;not null;default:0"`
	LinksUpdated        int        `json:"links_updated" gorm:"column:links_updated;not null;default:0"`
	StartedAt           time.Time  `json:"started_at" gorm:"column:started_at;autoCreateTime"`
	FinishedAt          *time.Time `json:"finished_at,omitempty" gorm:"column:finished_at"`
	DurationSeconds     *float64   `json:"duration_seconds,omitempty" gorm:"column:duration_seconds"`
	CreatedAt           time.Time  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt           time.Time  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (ScopusBatchImportRun) TableName() string { return "scopus_batch_import_runs" }
