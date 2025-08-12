// models/audit_log.go
package models

import "time"

// AuditLog represents the audit_logs table
type AuditLog struct {
	LogID         int       `gorm:"primaryKey;column:log_id;autoIncrement" json:"log_id"`
	UserID        int       `gorm:"column:user_id" json:"user_id"`
	Action        string    `gorm:"column:action;type:enum('create','update','delete','login','logout','view','download','approve','reject','submit','review')" json:"action"`
	EntityType    string    `gorm:"column:entity_type" json:"entity_type"`
	EntityID      *int      `gorm:"column:entity_id" json:"entity_id"`
	EntityNumber  *string   `gorm:"column:entity_number" json:"entity_number"`
	ChangedFields *string   `gorm:"column:changed_fields;type:text" json:"changed_fields"`
	OldValues     *string   `gorm:"column:old_values;type:text" json:"old_values"`
	NewValues     *string   `gorm:"column:new_values;type:text" json:"new_values"`
	Description   *string   `gorm:"column:description;type:text" json:"description"`
	IPAddress     string    `gorm:"column:ip_address" json:"ip_address"`
	UserAgent     *string   `gorm:"column:user_agent" json:"user_agent"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`

	// Relations
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName specifies the table name
func (AuditLog) TableName() string {
	return "audit_logs"
}
