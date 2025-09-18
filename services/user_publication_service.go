package services

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strconv"
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

// --- helpers ----------------------------------------------------------------

func normalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// collapse multiple spaces
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func makeFingerprint(title string, year *uint16) string {
	t := normalizeTitle(title)
	y := "0"
	if year != nil && *year > 0 {
		y = strconv.Itoa(int(*year))
	}
	h := sha1.New()
	h.Write([]byte(t + ":" + y))
	return hex.EncodeToString(h.Sum(nil))
}

// Upsert by (user_id, doi) first; fallback to (user_id, fingerprint).
// Returns (created?, record, error).
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

	// Ensure we have a fingerprint if none provided
	if (pub.Fingerprint == nil || *pub.Fingerprint == "") && pub.Title != "" {
		fp := makeFingerprint(pub.Title, pub.PublicationYear)
		pub.Fingerprint = &fp
	}

	var existing models.UserPublication
	var found bool

	// 1) Prefer DOI
	if pub.DOI != nil && *pub.DOI != "" {
		if err := s.db.Where("user_id = ? AND doi = ? AND deleted_at IS NULL",
			pub.UserID, *pub.DOI).
			First(&existing).Error; err == nil {
			found = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, empty, err
		}
	}

	// 2) Fallback fingerprint
	if !found && pub.Fingerprint != nil && *pub.Fingerprint != "" {
		if err := s.db.Where("user_id = ? AND fingerprint = ? AND deleted_at IS NULL",
			pub.UserID, *pub.Fingerprint).
			First(&existing).Error; err == nil {
			found = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, empty, err
		}
	}

	// 3) Update (match found)
	if found {
		updates := map[string]interface{}{
			"title":            pub.Title,
			"authors":          pub.Authors,
			"journal":          pub.Journal,
			"publication_type": pub.PublicationType,
			"publication_date": pub.PublicationDate,
			"publication_year": pub.PublicationYear,
			"doi":              pub.DOI, // if DOI appears later, save it
			"url":              pub.URL,
			"source":           pub.Source,
			"external_ids":     pub.ExternalIDs,
			"fingerprint":      pub.Fingerprint, // keep current fingerprint
			"is_verified":      pub.IsVerified,
			// Optional new fields (safe even if columns don't exist—remove if not added):
			"cited_by":         pub.CitedBy,
			"cited_by_url":     pub.CitedByURL,
			"citation_history": pub.CitationHistory,
			"updated_at":       time.Now(),
		}

		if err := s.db.Model(&existing).Updates(updates).Error; err != nil {
			return false, empty, err
		}
		// Return the fresh record
		if err := s.db.First(&existing, existing.ID).Error; err != nil {
			return false, empty, err
		}
		return false, existing, nil
	}

	// 4) Create new
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
