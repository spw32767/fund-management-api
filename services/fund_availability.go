package services

import (
	"errors"
	"strings"

	"fund-management-api/models"

	"gorm.io/gorm"
)

var (
	ErrFundClosed   = errors.New("fund is closed for applications")
	ErrFundNotFound = errors.New("fund subcategory not found")
)

func IsFundStatusOpen(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "active")
}

func EnsureSubcategoryOpen(db *gorm.DB, subcategoryID int) error {
	if subcategoryID <= 0 {
		return ErrFundNotFound
	}

	var subcategory models.FundSubcategory
	if err := db.Select("subcategory_id", "status", "delete_at").
		Where("subcategory_id = ? AND delete_at IS NULL", subcategoryID).
		First(&subcategory).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrFundNotFound
		}
		return err
	}

	if !IsFundStatusOpen(subcategory.Status) {
		return ErrFundClosed
	}
	return nil
}

func EnsureSubmissionFundOpen(db *gorm.DB, submissionID int) error {
	var submission models.Submission
	if err := db.Select("submission_id", "subcategory_id").
		Where("submission_id = ? AND deleted_at IS NULL", submissionID).
		First(&submission).Error; err != nil {
		return err
	}
	if submission.SubcategoryID == nil {
		return nil
	}
	return EnsureSubcategoryOpen(db, *submission.SubcategoryID)
}
