package services

import (
	"fund-management-api/config"
	"time"

	"gorm.io/gorm"
)

// UserInnovationResult represents innovation-like records derived from submissions
type UserInnovationResult struct {
	SubmissionID     int        `json:"submission_id"`
	SubmissionNumber string     `json:"submission_number"`
	Title            string     `json:"title"`
	InnovationType   string     `json:"innovation_type"`
	RegisteredDate   *time.Time `json:"registered_date"`
	StatusName       string     `json:"status_name"`
	YearName         *string    `json:"year_name"`
}

type InnovationService struct {
	db *gorm.DB
}

func NewInnovationService(db *gorm.DB) *InnovationService {
	if db == nil {
		db = config.DB
	}
	return &InnovationService{db: db}
}

func (s *InnovationService) ListByUser(userID uint, limit, offset int) ([]UserInnovationResult, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	keywordFilter := "(fs.subcategory_name LIKE ? OR fs.subcategory_name LIKE ?)"
	keywords := []string{"%สิทธิบัตร%", "%อนุสิทธิบัตร%"}

	query := s.db.Table("submissions").
		Joins("LEFT JOIN fund_application_details fad ON fad.submission_id = submissions.submission_id").
		Joins("LEFT JOIN fund_subcategories fs ON fs.subcategory_id = submissions.subcategory_id").
		Joins("LEFT JOIN years y ON y.year_id = submissions.year_id").
		Joins("LEFT JOIN application_status st ON st.application_status_id = submissions.status_id").
		Select(`
submissions.submission_id,
submissions.submission_number,
COALESCE(NULLIF(fad.project_title, ''), fs.subcategory_name, submissions.submission_type) AS title,
COALESCE(fs.subcategory_name, submissions.submission_type) AS innovation_type,
submissions.submitted_at AS registered_date,
COALESCE(st.status_name, '') AS status_name,
y.year AS year_name
`).
		Where("submissions.user_id = ? AND submissions.deleted_at IS NULL", userID).
		Where("submissions.status_id = ?", 2).
		Where(keywordFilter, keywords[0], keywords[1])

	// Default sort by submission (most recent first) with NULL registered dates last (MariaDB compatible)
	query = query.Order("submissions.submitted_at IS NULL, submissions.submitted_at DESC").Order("submissions.created_at DESC")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []UserInnovationResult
	if err := query.Limit(limit).Offset(offset).Scan(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
