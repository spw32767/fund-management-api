package controllers

import (
	"errors"
	"strings"
	"sync"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

var (
	fileUploadMetadataOnce               sync.Once
	fileUploadMetadataSupported          bool
	submissionDocumentOriginalNameOnce   sync.Once
	submissionDocumentOriginalNameExists bool
)

// createFileUploadRecord persists a FileUpload while accounting for databases
// that predate the optional metadata column. We check for column availability
// once per process and omit the field on inserts when the schema does not
// expose it, matching the behaviour of earlier revisions of the service.
func createFileUploadRecord(db *gorm.DB, fileUpload *models.FileUpload) error {
	if db == nil {
		db = config.DB
	}

	if !fileUploadSupportsMetadata(db) {
		return db.Omit("Metadata").Create(fileUpload).Error
	}

	if err := db.Create(fileUpload).Error; err != nil {
		// Be tolerant of legacy schemas that might not include the metadata column.
		if strings.Contains(strings.ToLower(err.Error()), "metadata") {
			markFileUploadMetadataUnsupported()
			return db.Omit("Metadata").Create(fileUpload).Error
		}
		return err
	}

	return nil
}

func saveFileUploadRecord(db *gorm.DB, fileUpload *models.FileUpload) error {
	if db == nil {
		db = config.DB
	}

	if !fileUploadSupportsMetadata(db) {
		return db.Omit("Metadata").Save(fileUpload).Error
	}

	if err := db.Save(fileUpload).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "metadata") {
			markFileUploadMetadataUnsupported()
			return db.Omit("Metadata").Save(fileUpload).Error
		}
		return err
	}

	return nil
}

func createSubmissionDocumentRecord(db *gorm.DB, document *models.SubmissionDocument) error {
	if !submissionDocumentSupportsOriginalName(db) {
		return db.Omit("OriginalName").Create(document).Error
	}
	return db.Create(document).Error
}

func saveSubmissionDocumentRecord(db *gorm.DB, document *models.SubmissionDocument) error {
	if !submissionDocumentSupportsOriginalName(db) {
		return db.Omit("OriginalName").Save(document).Error
	}
	return db.Save(document).Error
}

func fileUploadSupportsMetadata(db *gorm.DB) bool {
	fileUploadMetadataOnce.Do(func() {
		fileUploadMetadataSupported = db.Migrator().HasColumn(&models.FileUpload{}, "metadata")
	})
	return fileUploadMetadataSupported
}

// markFileUploadMetadataUnsupported resets the detection cache so future calls
// will avoid using the metadata column when talking to legacy databases.
func markFileUploadMetadataUnsupported() {
	fileUploadMetadataSupported = false
	fileUploadMetadataOnce = sync.Once{}
}

func submissionDocumentSupportsOriginalName(db *gorm.DB) bool {
	submissionDocumentOriginalNameOnce.Do(func() {
		submissionDocumentOriginalNameExists = db.Migrator().HasColumn(&models.SubmissionDocument{}, "original_name")
	})
	return submissionDocumentOriginalNameExists
}

func resolveDocumentTypeByCode(db *gorm.DB, code string) (*models.DocumentType, error) {
	if db == nil {
		db = config.DB
	}

	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return nil, errors.New("document type code is required")
	}

	var documentType models.DocumentType
	if err := db.Where("code = ? AND (delete_at IS NULL OR delete_at = '0000-00-00 00:00:00')", trimmed).
		First(&documentType).Error; err != nil {
		return nil, err
	}

	return &documentType, nil
}

type submissionDocumentWithTypeOrder struct {
	DocumentID    int  `gorm:"column:document_id"`
	DisplayOrder  int  `gorm:"column:display_order"`
	DocumentOrder *int `gorm:"column:document_order"`
}

func resequenceSubmissionDocumentsByDocumentType(db *gorm.DB, submissionID int) error {
	if db == nil {
		db = config.DB
	}

	var documents []submissionDocumentWithTypeOrder
	if err := db.Model(&models.SubmissionDocument{}).
		Joins("LEFT JOIN document_types dt ON dt.document_type_id = submission_documents.document_type_id").
		Select("submission_documents.document_id, submission_documents.display_order, dt.document_order").
		Where("submission_documents.submission_id = ?", submissionID).
		Order("CASE WHEN dt.document_order IS NULL THEN 1 ELSE 0 END").
		Order("dt.document_order ASC").
		Order("submission_documents.display_order ASC").
		Order("submission_documents.document_id ASC").
		Find(&documents).Error; err != nil {
		return err
	}

	nextOrder := 1
	for _, doc := range documents {
		if doc.DisplayOrder == nextOrder {
			nextOrder++
			continue
		}

		if err := db.Model(&models.SubmissionDocument{}).
			Where("document_id = ?", doc.DocumentID).
			Update("display_order", nextOrder).Error; err != nil {
			return err
		}
		nextOrder++
	}

	return nil
}

// enrichSubmissionDocumentsWithFileMetadata copies frequently used file fields onto the
// submission document itself so that API consumers always have a reliable filename to display
// even if the nested File relation is trimmed or omitted by the serializer.
func enrichSubmissionDocumentsWithFileMetadata(documents []models.SubmissionDocument) {
	for i := range documents {
		file := documents[i].File
		if file.FileID == 0 {
			continue
		}

		if documents[i].FileID != file.FileID {
			documents[i].FileID = file.FileID
		}
		if documents[i].OriginalName == "" {
			documents[i].OriginalName = file.OriginalName
		}
	}
}
