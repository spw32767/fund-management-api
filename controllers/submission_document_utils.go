package controllers

import (
	"sync"

	"fund-management-api/models"

	"gorm.io/gorm"
)

var (
	fileUploadMetadataOnce      sync.Once
	fileUploadMetadataSupported bool
)

// createFileUploadRecord persists a FileUpload while accounting for databases
// that predate the optional metadata column. We check for column availability
// once per process and omit the field on inserts when the schema does not
// expose it, matching the behaviour of earlier revisions of the service.
func createFileUploadRecord(db *gorm.DB, fileUpload *models.FileUpload) error {
	if !fileUploadSupportsMetadata(db) {
		return db.Omit("Metadata").Create(fileUpload).Error
	}
	return db.Create(fileUpload).Error
}

func fileUploadSupportsMetadata(db *gorm.DB) bool {
	fileUploadMetadataOnce.Do(func() {
		fileUploadMetadataSupported = db.Migrator().HasColumn(&models.FileUpload{}, "metadata")
	})
	return fileUploadMetadataSupported
}

// enrichSubmissionDocumentsWithFileMetadata copies frequently used file fields onto the
// submission document itself so that API consumers always have a reliable filename to display
// even if the nested File relation is trimmed or omitted by the serializer.
// func enrichSubmissionDocumentsWithFileMetadata(documents []models.SubmissionDocument) {
// for i := range documents {
// file := documents[i].File
// if file.FileID == 0 {
// continue
// }
// // หากข้อมูล file_id ใน submission_documents ไม่ตรงกับไฟล์ที่ preload มา
// // (เช่น กรณี query ที่ select ข้อมูลไม่ครบจนทำให้ FileID ถูก set ผิด)
// // ให้ใช้ค่าจาก relation ของไฟล์ซึ่งเป็นแหล่งความจริงแทน
// if documents[i].FileID != file.FileID {
// documents[i].FileID = file.FileID
// }
// if documents[i].OriginalName == "" {
// documents[i].OriginalName = file.OriginalName
// }
// if documents[i].OriginalFilename == "" {
// documents[i].OriginalFilename = file.OriginalName
// }
// if documents[i].FileName == "" {
// documents[i].FileName = file.OriginalName
// }
// if documents[i].FilePath == "" {
// documents[i].FilePath = file.StoredPath
// }
// }
// }
