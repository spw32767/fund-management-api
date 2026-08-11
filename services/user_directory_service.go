package services

import (
	"time"

	"fund-management-api/config"

	"gorm.io/gorm"
)

// PartnerUser is one faculty (users) record exposed to external clients via /api/ext.
// Column tags map the mixed-case DB columns to clean snake_case JSON keys. Nullable fields
// are pointers with omitempty; user_id / names / role_id are always present.
type PartnerUser struct {
	UserID           int        `gorm:"column:user_id" json:"user_id"`
	Prefix           *string    `gorm:"column:prefix" json:"prefix,omitempty"`
	UserFname        string     `gorm:"column:user_fname" json:"user_fname"`
	UserLname        string     `gorm:"column:user_lname" json:"user_lname"`
	Gender           *string    `gorm:"column:gender" json:"gender,omitempty"`
	Email            *string    `gorm:"column:email" json:"email,omitempty"`
	Tel              *string    `gorm:"column:tel" json:"tel,omitempty"`
	TelFormat        *string    `gorm:"column:tel_format" json:"tel_format,omitempty"`
	PositionTitle    *string    `gorm:"column:position_title" json:"position_title,omitempty"`
	PositionEn       *string    `gorm:"column:position_en" json:"position_en,omitempty"`
	PrefixPositionEn *string    `gorm:"column:prefix_position_en" json:"prefix_position_en,omitempty"`
	ManagePosition   *string    `gorm:"column:manage_position" json:"manage_position,omitempty"`
	NameEn           *string    `gorm:"column:name_en" json:"name_en,omitempty"`
	SuffixEn         *string    `gorm:"column:suffix_en" json:"suffix_en,omitempty"`
	ScopusID         *string    `gorm:"column:scopus_id" json:"scopus_id,omitempty"`
	ScholarAuthorID  *string    `gorm:"column:scholar_author_id" json:"scholar_author_id,omitempty"`
	LabName          *string    `gorm:"column:lab_name" json:"lab_name,omitempty"`
	Room             *string    `gorm:"column:room" json:"room,omitempty"`
	CPWebID          *string    `gorm:"column:cp_web_id" json:"cp_web_id,omitempty"`
	RoleID           int        `gorm:"column:role_id" json:"role_id"`
	RoleName         *string    `gorm:"column:role_name" json:"role_name,omitempty"`
	IsActive         *string    `gorm:"column:is_active" json:"is_active,omitempty"`
	UpdatedAt        *time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// partnerUserSelect lists the columns (aliased to the JSON-friendly names above) fetched for
// each user. The mixed-case source columns (TEL, Name_en, Scopus_id, ...) are aliased here so
// the scan mapping stays unambiguous.
const partnerUserSelect = "u.user_id AS user_id, u.prefix AS prefix, u.user_fname AS user_fname, " +
	"u.user_lname AS user_lname, u.gender AS gender, u.email AS email, u.TEL AS tel, " +
	"u.TELformat AS tel_format, u.position AS position_title, u.position_en AS position_en, " +
	"u.prefix_position_en AS prefix_position_en, u.manage_position AS manage_position, " +
	"u.Name_en AS name_en, u.suffix_en AS suffix_en, u.Scopus_id AS scopus_id, " +
	"u.scholar_author_id AS scholar_author_id, u.LAB_Name AS lab_name, u.Room AS room, " +
	"u.CP_WEB_ID AS cp_web_id, u.role_id AS role_id, r.role AS role_name, " +
	"u.Is_active AS is_active, u.update_at AS updated_at"

// UserDirectoryService provides read access to the faculty directory for external clients.
type UserDirectoryService struct {
	db *gorm.DB
}

// NewUserDirectoryService instantiates the service, falling back to the global config.DB.
func NewUserDirectoryService(db *gorm.DB) *UserDirectoryService {
	if db == nil {
		db = config.DB
	}
	return &UserDirectoryService{db: db}
}

// ListForPartner returns paginated faculty records for the external users endpoint. It
// excludes soft-deleted users (delete_at IS NULL). When updatedSince is non-nil, only users
// modified at/after that time are returned (incremental sync). Returns rows + total count.
func (s *UserDirectoryService) ListForPartner(updatedSince *time.Time, limit, offset int) ([]PartnerUser, int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	// Build a fresh base query per finisher to avoid reusing a mutated statement.
	buildBase := func() *gorm.DB {
		q := s.db.Table("users AS u").
			Joins("LEFT JOIN roles AS r ON r.role_id = u.role_id").
			Where("u.delete_at IS NULL")
		if updatedSince != nil {
			q = q.Where("u.update_at >= ?", *updatedSince)
		}
		return q
	}

	var total int64
	if err := buildBase().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []PartnerUser{}, 0, nil
	}

	var rows []PartnerUser
	if err := buildBase().
		Select(partnerUserSelect).
		Order("u.user_id").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}
