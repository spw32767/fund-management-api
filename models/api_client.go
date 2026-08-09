package models

import "time"

// APIClient is an external system authorized to call the /api/ext APIs.
type APIClient struct {
	ID           uint64    `gorm:"primaryKey;column:id" json:"id"`
	Name         string    `gorm:"column:name" json:"name"`
	Description  *string   `gorm:"column:description" json:"description,omitempty"`
	ContactEmail *string   `gorm:"column:contact_email" json:"contact_email,omitempty"`
	Status       string    `gorm:"column:status" json:"status"` // active | disabled
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (APIClient) TableName() string { return "api_clients" }

// APIScope is a catalog entry describing a permission that can be granted to a client.
type APIScope struct {
	ID          uint64    `gorm:"primaryKey;column:id" json:"id"`
	Code        string    `gorm:"column:code" json:"code"`
	Description *string   `gorm:"column:description" json:"description,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
}

func (APIScope) TableName() string { return "api_scopes" }

// APIClientScope grants a single scope to a single client.
type APIClientScope struct {
	ID        uint64    `gorm:"primaryKey;column:id" json:"id"`
	ClientID  uint64    `gorm:"column:client_id" json:"client_id"`
	ScopeID   uint64    `gorm:"column:scope_id" json:"scope_id"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (APIClientScope) TableName() string { return "api_client_scopes" }

// APIKey is a credential belonging to a client. Only the SHA-256 hash of the raw key is
// stored; the plaintext key is shown once at creation and never persisted.
type APIKey struct {
	ID         uint64     `gorm:"primaryKey;column:id" json:"id"`
	ClientID   uint64     `gorm:"column:client_id" json:"client_id"`
	Label      *string    `gorm:"column:label" json:"label,omitempty"`
	KeyPrefix  string     `gorm:"column:key_prefix" json:"key_prefix"`
	KeyHash    string     `gorm:"column:key_hash" json:"-"`
	Status     string     `gorm:"column:status" json:"status"` // active | revoked
	ExpiresAt  *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	LastUsedAt *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (APIKey) TableName() string { return "api_keys" }

// APIRequestLog is one row per external API request (audit trail). The key value is never
// recorded here.
type APIRequestLog struct {
	ID               uint64    `gorm:"primaryKey;column:id" json:"id"`
	ClientID         *uint64   `gorm:"column:client_id" json:"client_id,omitempty"`
	APIKeyID         *uint64   `gorm:"column:api_key_id" json:"api_key_id,omitempty"`
	Method           string    `gorm:"column:method" json:"method"`
	Endpoint         string    `gorm:"column:endpoint" json:"endpoint"`
	RequestedUserIDs *string   `gorm:"column:requested_user_ids" json:"requested_user_ids,omitempty"`
	YearFrom         *uint16   `gorm:"column:year_from" json:"year_from,omitempty"`
	YearTo           *uint16   `gorm:"column:year_to" json:"year_to,omitempty"`
	HTTPStatus       uint16    `gorm:"column:http_status" json:"http_status"`
	IP               *string   `gorm:"column:ip" json:"ip,omitempty"`
	LatencyMs        *uint32   `gorm:"column:latency_ms" json:"latency_ms,omitempty"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
}

func (APIRequestLog) TableName() string { return "api_request_logs" }
