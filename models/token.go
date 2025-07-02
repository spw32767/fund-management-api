package models

import (
	"time"
)

// UserToken for refresh tokens and other token types
type UserToken struct {
	TokenID    int       `gorm:"primaryKey;column:token_id" json:"token_id"`
	UserID     int       `gorm:"column:user_id" json:"user_id"`
	TokenType  string    `gorm:"column:token_type" json:"token_type"`
	Token      string    `gorm:"column:token" json:"token"`
	ExpiresAt  time.Time `gorm:"column:expires_at" json:"expires_at"`
	IsRevoked  bool      `gorm:"column:is_revoked" json:"is_revoked"`
	DeviceInfo string    `gorm:"column:device_info" json:"device_info,omitempty"`
	IPAddress  string    `gorm:"column:ip_address" json:"ip_address,omitempty"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Relations
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// UserSession for active login sessions
type UserSession struct {
	SessionID      int        `gorm:"primaryKey;column:session_id" json:"session_id"`
	UserID         int        `gorm:"column:user_id" json:"user_id"`
	AccessTokenJTI string     `gorm:"column:access_token_jti" json:"access_token_jti"`
	RefreshToken   string     `gorm:"column:refresh_token" json:"refresh_token"`
	DeviceName     string     `gorm:"column:device_name" json:"device_name,omitempty"`
	DeviceType     string     `gorm:"column:device_type" json:"device_type,omitempty"`
	IPAddress      string     `gorm:"column:ip_address" json:"ip_address,omitempty"`
	UserAgent      string     `gorm:"column:user_agent" json:"user_agent,omitempty"`
	LastActivity   *time.Time `gorm:"column:last_activity" json:"last_activity"`
	ExpiresAt      time.Time  `gorm:"column:expires_at" json:"expires_at"`
	IsActive       bool       `gorm:"column:is_active" json:"is_active"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at"`

	// Relations
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName overrides
func (UserToken) TableName() string {
	return "user_tokens"
}

func (UserSession) TableName() string {
	return "user_sessions"
}
