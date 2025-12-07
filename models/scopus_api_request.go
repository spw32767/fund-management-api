package models

import "time"

type ScopusAPIRequest struct {
	ID             uint64    `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	JobID          uint64    `json:"job_id" gorm:"column:job_id;not null"`
	HTTPMethod     string    `json:"http_method" gorm:"column:http_method;type:varchar(8);not null"`
	Endpoint       string    `json:"endpoint" gorm:"column:endpoint;type:text;not null"`
	QueryParams    *string   `json:"query_params,omitempty" gorm:"column:query_params;type:json"`
	RequestHeaders *string   `json:"request_headers,omitempty" gorm:"column:request_headers;type:json"`
	ResponseStatus *int      `json:"response_status,omitempty" gorm:"column:response_status"`
	ResponseTimeMs *int      `json:"response_time_ms,omitempty" gorm:"column:response_time_ms"`
	PageStart      *int      `json:"page_start,omitempty" gorm:"column:page_start"`
	PageCount      *int      `json:"page_count,omitempty" gorm:"column:page_count"`
	ItemsReturned  *int      `json:"items_returned,omitempty" gorm:"column:items_returned"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at;autoCreateTime"`
}

func (ScopusAPIRequest) TableName() string { return "scopus_api_requests" }
