package models

import "time"

type MouNotificationSetting struct {
	ID                    int       `gorm:"primaryKey;column:id;default:1" json:"id"`
	NotifyCoordinator     bool      `gorm:"column:notify_coordinator;default:true" json:"notify_coordinator"`
	NotifyFacultyResponsible bool   `gorm:"column:notify_faculty_responsible;default:false" json:"notify_faculty_responsible"`
	NotifyExternal        bool      `gorm:"column:notify_external;default:false" json:"notify_external"`
	IncludeMouCode        bool      `gorm:"column:include_mou_code;default:true" json:"include_mou_code"`
	IncludeTitle          bool      `gorm:"column:include_title;default:true" json:"include_title"`
	IncludePartner        bool      `gorm:"column:include_partner;default:true" json:"include_partner"`
	IncludeDates          bool      `gorm:"column:include_dates;default:true" json:"include_dates"`
	IncludeLevel          bool      `gorm:"column:include_level;default:false" json:"include_level"`
	IncludeStatus         bool      `gorm:"column:include_status;default:true" json:"include_status"`
	UpdatedBy             *int      `gorm:"column:updated_by" json:"updated_by,omitempty"`
	CreatedAt             time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`

	Updater *User `gorm:"foreignKey:UpdatedBy" json:"updater,omitempty"`
}

func (MouNotificationSetting) TableName() string { return "mou_notification_settings" }

type UpdateMouNotificationSettingRequest struct {
	NotifyCoordinator       *bool  `json:"notify_coordinator"`
	NotifyFacultyResponsible *bool `json:"notify_faculty_responsible"`
	NotifyExternal          *bool  `json:"notify_external"`
	IncludeMouCode          *bool  `json:"include_mou_code"`
	IncludeTitle            *bool  `json:"include_title"`
	IncludePartner          *bool  `json:"include_partner"`
	IncludeDates            *bool  `json:"include_dates"`
	IncludeLevel            *bool  `json:"include_level"`
	IncludeStatus           *bool  `json:"include_status"`
}

type NotificationRecipient struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Type        string `json:"type"`        // "coordinator", "faculty", "external"
	FacultyName string `json:"faculty_name,omitempty"`
	OrgName     string `json:"org_name,omitempty"`
	MouTitle    string `json:"mou_title,omitempty"`
}
