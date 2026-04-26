package models

import "time"

type ThaiJOAPIRequest struct {
	ID             uint64    `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	JobID          *uint64   `json:"job_id,omitempty" gorm:"column:job_id"`
	HTTPMethod     string    `json:"http_method" gorm:"column:http_method;type:varchar(8);not null"`
	Endpoint       string    `json:"endpoint" gorm:"column:endpoint;type:text;not null"`
	QueryParams    *string   `json:"query_params,omitempty" gorm:"column:query_params;type:json"`
	RequestHeaders *string   `json:"request_headers,omitempty" gorm:"column:request_headers;type:json"`
	RequestBody    *string   `json:"request_body,omitempty" gorm:"column:request_body;type:json"`
	ResponseStatus *int      `json:"response_status,omitempty" gorm:"column:response_status"`
	ResponseTimeMs *int      `json:"response_time_ms,omitempty" gorm:"column:response_time_ms"`
	ItemsReturned  *int      `json:"items_returned,omitempty" gorm:"column:items_returned"`
	ResponseBody   *string   `json:"response_body,omitempty" gorm:"column:response_body;type:json"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at;autoCreateTime"`
}

func (ThaiJOAPIRequest) TableName() string { return "thaijo_api_requests" }
