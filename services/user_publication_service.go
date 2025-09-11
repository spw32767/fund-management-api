package services

import (
	"errors"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

type PublicationService struct {
	db *gorm.DB
}

func NewPublicationService(db *gorm.DB) *PublicationService {
	if db == nil {
		db = config.DB
	}
	return &PublicationService{db: db}
}

func (s *PublicationService) ListByUser(userID uint, year *int, limit, offset int) ([]models.UserPublication, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var pubs []models.UserPublication
	q := s.db.Model(&models.UserPublication{}).
		Where("user_id = ? AND deleted_at IS NULL", userID)

	if year != nil {
		q = q.Where("publication_year = ?", *year)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := q.
		Order("publication_year DESC, publication_date DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&pubs).Error; err != nil {
		return nil, 0, err
	}

	return pubs, total, nil
}

// Upsert by (user_id, doi) first; fallback to (user_id, fingerprint).
func (s *PublicationService) Upsert(pub *models.UserPublication) (bool, models.UserPublication, error) {
	var empty models.UserPublication
	if pub == nil {
		return false, empty, errors.New("publication is nil")
	}

	// Derive year from date if needed
	if pub.PublicationYear == nil && pub.PublicationDate != nil {
		yy := uint16(pub.PublicationDate.Year())
		pub.PublicationYear = &yy
	}
	// Normalize DOI spacing
	if pub.DOI != nil {
		d := strings.TrimSpace(*pub.DOI)
		pub.DOI = &d
	}

	var existing models.UserPublication
	var found bool

	// Prefer DOI
	if pub.DOI != nil && *pub.DOI != "" {
		if err := s.db.Where("user_id = ? AND doi = ? AND deleted_at IS NULL", pub.UserID, *pub.DOI).
			First(&existing).Error; err == nil {
			found = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, empty, err
		}
	}
	// Fallback fingerprint
	if !found && pub.Fingerprint != nil && *pub.Fingerprint != "" {
		if err := s.db.Where("user_id = ? AND fingerprint = ? AND deleted_at IS NULL", pub.UserID, *pub.Fingerprint).
			First(&existing).Error; err == nil {
			found = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, empty, err
		}
	}

	if found {
		updates := map[string]interface{}{
			"title":            pub.Title,
			"authors":          pub.Authors,
			"journal":          pub.Journal,
			"publication_type": pub.PublicationType,
			"publication_date": pub.PublicationDate,
			"publication_year": pub.PublicationYear,
			"doi":              pub.DOI,
			"url":              pub.URL,
			"source":           pub.Source,
			"external_ids":     pub.ExternalIDs,
			"fingerprint":      pub.Fingerprint,
			"is_verified":      pub.IsVerified,
			"updated_at":       time.Now(),
		}
		if err := s.db.Model(&existing).Updates(updates).Error; err != nil {
			return false, empty, err
		}
		return false, existing, nil
	}

	if err := s.db.Create(pub).Error; err != nil {
		return false, empty, err
	}
	return true, *pub, nil
}

func (s *PublicationService) SoftDelete(id uint, userID uint) error {
	return s.db.Where("id = ? AND user_id = ?", id, userID).
		Delete(&models.UserPublication{}).Error
}

func (s *PublicationService) Restore(id uint, userID uint) error {
	return s.db.Unscoped().
		Model(&models.UserPublication{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("deleted_at", nil).Error
}
