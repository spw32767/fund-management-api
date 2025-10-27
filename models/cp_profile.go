package models

import "time"

type CpProfile struct {
	ID         int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID     *int      `gorm:"column:user_id" json:"user_id,omitempty"`
	NameTh     string    `gorm:"column:name_th" json:"name_th"`
	NameEn     string    `gorm:"column:name_en" json:"name_en"`
	Position   string    `gorm:"column:position" json:"position"`
	Email      string    `gorm:"column:email" json:"email"`
	PhotoURL   string    `gorm:"column:photo_url;size:500" json:"photo_url"`
	ProfileURL string    `gorm:"column:profile_url;size:500;uniqueIndex:uq_cp_profile_url" json:"profile_url"`
	Info       string    `gorm:"column:info" json:"info"`
	Education  string    `gorm:"column:education" json:"education"`
	CreatedAt  time.Time `gorm:"column:create_at;autoCreateTime" json:"create_at"`
	UpdatedAt  time.Time `gorm:"column:update_at;autoUpdateTime" json:"update_at"`
}

func (CpProfile) TableName() string { return "cp_profile" }
