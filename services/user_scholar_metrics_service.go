package services

import (
	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

type UserScholarMetricsService struct {
	db *gorm.DB
}

func NewUserScholarMetricsService(db *gorm.DB) *UserScholarMetricsService {
	if db == nil {
		db = config.DB
	}
	return &UserScholarMetricsService{db: db}
}

func (s *UserScholarMetricsService) Upsert(m *models.UserScholarMetrics) error {
	// insert or update by primary key (user_id)
	var existing models.UserScholarMetrics
	err := s.db.First(&existing, "user_id = ?", m.UserID).Error
	if err == nil {
		return s.db.Model(&existing).Updates(map[string]interface{}{
			"hindex":         m.HIndex,
			"hindex5y":       m.HIndex5Y,
			"i10index":       m.I10Index,
			"i10index5y":     m.I10Index5Y,
			"citedby_total":  m.CitedByTotal,
			"citedby_5y":     m.CitedBy5Y,
			"cites_per_year": m.CitesPerYear,
		}).Error
	}
	// not found -> create
	return s.db.Create(m).Error
}

func (s *UserScholarMetricsService) Get(userID uint) (*models.UserScholarMetrics, error) {
	var m models.UserScholarMetrics
	if err := s.db.First(&m, "user_id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &m, nil
}
