package services

import (
	"errors"
	"strings"

	"fund-management-api/config"

	"gorm.io/gorm"
)

type ThaiJOPublication struct {
	ID             uint64  `json:"id"`
	ThaiJOArticleID string  `json:"thaijo_article_id"`
	Title          string  `json:"title"`
	TitleEN        *string `json:"title_en,omitempty"`
	TitleTH        *string `json:"title_th,omitempty"`
	JournalNameEN  *string `json:"journal_name_en,omitempty"`
	JournalNameTH  *string `json:"journal_name_th,omitempty"`
	JournalPath    *string `json:"journal_path,omitempty"`
	Year           *int    `json:"year,omitempty"`
	DOI            *string `json:"doi,omitempty"`
	ArticleURL     *string `json:"article_url,omitempty"`
	PDFURL         *string `json:"pdf_url,omitempty"`
	Tier           *int    `json:"tier,omitempty"`
	TierPeriod     *string `json:"tier_period,omitempty"`
}

type ThaiJOPublicationService struct {
	db *gorm.DB
}

func NewThaiJOPublicationService(db *gorm.DB) *ThaiJOPublicationService {
	if db == nil {
		db = config.DB
	}
	return &ThaiJOPublicationService{db: db}
}

func (s *ThaiJOPublicationService) ListByUser(userID uint, limit, offset int, search string) ([]ThaiJOPublication, int64, error) {
	if userID == 0 {
		return []ThaiJOPublication{}, 0, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	var user struct {
		ThaiJOAuthorID *string `gorm:"column:thaijo_author_id"`
	}
	if err := s.db.Table("users").Select("thaijo_author_id").Where("user_id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []ThaiJOPublication{}, 0, nil
		}
		return nil, 0, err
	}
	authorID := strings.TrimSpace(derefString(user.ThaiJOAuthorID))
	if authorID == "" {
		return []ThaiJOPublication{}, 0, nil
	}

	base := s.db.Table("thaijo_documents td").
		Joins("JOIN thaijo_document_authors tda ON tda.document_id = td.id").
		Joins("JOIN thaijo_authors ta ON ta.id = tda.author_id").
		Joins("LEFT JOIN thaijo_journals tj ON tj.path = td.journal_path").
		Where("ta.thaijo_author_id = ?", authorID)

	if q := strings.TrimSpace(search); q != "" {
		like := "%" + q + "%"
		base = base.Where("td.title_en LIKE ? OR td.title_th LIKE ? OR td.doi LIKE ?", like, like, like)
	}

	var total int64
	if err := base.Distinct("td.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	type row struct {
		ID             uint64
		ThaiJOArticleID string
		TitleEN        *string
		TitleTH        *string
		JournalNameEN  *string
		JournalNameTH  *string
		JournalPath    *string
		Year           *int
		DOI            *string
		ArticleURL     *string
		PDFURL         *string
		Tier           *int
		TierPeriod     *string
	}

	var rows []row
	if err := base.
		Select("td.id, td.thaijo_article_id, td.title_en, td.title_th, tj.name_en AS journal_name_en, tj.name_th AS journal_name_th, td.journal_path, td.year, td.doi, td.article_url, td.pdf_url, tj.tier, tj.tier_period").
		Order("td.year DESC, td.id DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]ThaiJOPublication, 0, len(rows))
	for _, r := range rows {
		title := strings.TrimSpace(derefString(r.TitleTH))
		if title == "" {
			title = strings.TrimSpace(derefString(r.TitleEN))
		}
		out = append(out, ThaiJOPublication{
			ID:             r.ID,
			ThaiJOArticleID: r.ThaiJOArticleID,
			Title:          title,
			TitleEN:        r.TitleEN,
			TitleTH:        r.TitleTH,
			JournalNameEN:  r.JournalNameEN,
			JournalNameTH:  r.JournalNameTH,
			JournalPath:    r.JournalPath,
			Year:           r.Year,
			DOI:            r.DOI,
			ArticleURL:     r.ArticleURL,
			PDFURL:         r.PDFURL,
			Tier:           r.Tier,
			TierPeriod:     r.TierPeriod,
		})
	}

	return out, total, nil
}
