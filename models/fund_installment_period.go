package models

import "time"

// FundInstallmentPeriod represents fund_installment_periods records.
type FundInstallmentPeriod struct {
	InstallmentPeriodID int        `gorm:"column:installment_period_id;primaryKey" json:"installment_period_id"`
	FundLevel           *string    `gorm:"column:fund_level" json:"fund_level,omitempty"`
	FundKeyword         *string    `gorm:"column:fund_keyword" json:"fund_keyword,omitempty"`
	FundParentKeyword   *string    `gorm:"column:fund_parent_keyword" json:"fund_parent_keyword,omitempty"`
	YearID              int        `gorm:"column:year_id" json:"year_id"`
	InstallmentNumber   int        `gorm:"column:installment_number" json:"installment_number"`
	CutoffDate          time.Time  `gorm:"column:cutoff_date" json:"cutoff_date"`
	Name                *string    `gorm:"column:name" json:"name,omitempty"`
	Status              *string    `gorm:"column:status" json:"status,omitempty"`
	Remark              *string    `gorm:"column:remark" json:"remark,omitempty"`
	CreatedAt           time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt           *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
}

// TableName implements gorm's tablename interface.
func (FundInstallmentPeriod) TableName() string {
	return "fund_installment_periods"
}
