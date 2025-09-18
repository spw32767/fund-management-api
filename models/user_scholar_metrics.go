package models

import "time"

type UserScholarMetrics struct {
	UserID       int       `json:"user_id" gorm:"column:user_id;primaryKey"`
	HIndex       *int      `json:"hindex" gorm:"column:hindex"`
	HIndex5Y     *int      `json:"hindex5y" gorm:"column:hindex5y"`
	I10Index     *int      `json:"i10index" gorm:"column:i10index"`
	I10Index5Y   *int      `json:"i10index5y" gorm:"column:i10index5y"`
	CitedByTotal *int      `json:"citedby_total" gorm:"column:citedby_total"`
	CitedBy5Y    *int      `json:"citedby_5y" gorm:"column:citedby_5y"`
	CitesPerYear *string   `json:"cites_per_year" gorm:"column:cites_per_year"` // JSON string
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (UserScholarMetrics) TableName() string { return "user_scholar_metrics" }
