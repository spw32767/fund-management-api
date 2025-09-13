package services

import (
	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

type InnovationService struct {
	db *gorm.DB
}

func NewInnovationService(db *gorm.DB) *InnovationService {
	if db == nil {
		db = config.DB
	}
	return &InnovationService{db: db}
}

func (s *InnovationService) ListByUser(userID uint, limit, offset int) ([]models.Innovation, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var items []models.Innovation
	q := s.db.Model(&models.Innovation{}).Where("user_id = ?", userID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := q.Order("registered_date DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
