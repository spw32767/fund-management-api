package models

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	"gorm.io/gorm"
)

type UserPublication struct {
	ID              uint       `json:"id"                gorm:"primaryKey;autoIncrement"`
	UserID          uint       `json:"user_id"           gorm:"not null;index:idx_user_year"`
	Title           string     `json:"title"             gorm:"type:varchar(500);not null"`
	Authors         *string    `json:"authors,omitempty" gorm:"type:text"`
	Journal         *string    `json:"journal,omitempty" gorm:"type:varchar(255)"`
	PublicationType *string    `json:"publication_type,omitempty" gorm:"type:enum('journal','conference','book','thesis','other')"`
	PublicationDate *time.Time `json:"publication_date,omitempty" gorm:"type:date"`
	PublicationYear *uint16    `json:"publication_year,omitempty" gorm:"index:idx_user_year"`
	DOI             *string    `json:"doi,omitempty"     gorm:"type:varchar(255);uniqueIndex:uniq_doi"`
	URL             *string    `json:"url,omitempty"     gorm:"type:varchar(512)"`
	Source          *string    `json:"source,omitempty"  gorm:"type:enum('scholar','openalex','orcid','crossref')"`
	ExternalIDs     *string    `json:"external_ids,omitempty" gorm:"type:longtext"`
	Fingerprint     *string    `json:"fingerprint,omitempty"  gorm:"type:varchar(64);uniqueIndex:uniq_fingerprint"`
	IsVerified      bool       `json:"is_verified"       gorm:"type:tinyint(1);not null;default:0"`

	CreatedAt time.Time      `json:"created_at"  gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `json:"updated_at"  gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"deleted_at"  gorm:"column:deleted_at;index"`
}

func (UserPublication) TableName() string { return "publication" }

// Auto-generate fingerprint if missing: sha1(lower(title) + ":" + year)
func (p *UserPublication) BeforeCreate(tx *gorm.DB) error {
	if (p.Fingerprint == nil || *p.Fingerprint == "") && p.Title != "" && p.PublicationYear != nil {
		fp := makeFingerprint(p.Title, int(*p.PublicationYear))
		p.Fingerprint = &fp
	}
	return nil
}
func (p *UserPublication) BeforeSave(tx *gorm.DB) error {
	if (p.Fingerprint == nil || *p.Fingerprint == "") && p.Title != "" && p.PublicationYear != nil {
		fp := makeFingerprint(p.Title, int(*p.PublicationYear))
		p.Fingerprint = &fp
	}
	return nil
}

func makeFingerprint(title string, year int) string {
	base := strings.TrimSpace(strings.ToLower(title)) + ":" + itoa(year)
	sum := sha1.Sum([]byte(base))
	return hex.EncodeToString(sum[:])
}
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	sign := ""
	if i < 0 {
		sign = "-"
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if sign != "" {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
