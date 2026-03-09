package models

import "time"

type AuthIdentity struct {
	IdentityID      int        `gorm:"primaryKey;column:identity_id" json:"identity_id"`
	UserID          int        `gorm:"column:user_id" json:"user_id"`
	Provider        string     `gorm:"column:provider" json:"provider"`
	ProviderSubject *string    `gorm:"column:provider_subject" json:"provider_subject,omitempty"`
	EmailAtProvider *string    `gorm:"column:email_at_provider" json:"email_at_provider,omitempty"`
	RawClaims       []byte     `gorm:"column:raw_claims;type:json" json:"raw_claims,omitempty"`
	LastLoginAt     *time.Time `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
	CreateAt        *time.Time `gorm:"column:create_at" json:"create_at,omitempty"`
	UpdateAt        *time.Time `gorm:"column:update_at" json:"update_at,omitempty"`
	DeleteAt        *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

func (AuthIdentity) TableName() string {
	return "auth_identities"
}
