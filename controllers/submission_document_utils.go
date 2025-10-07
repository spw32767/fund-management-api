package controllers

import "fund-management-api/models"

// enrichSubmissionDocumentsWithFileMetadata copies frequently used file fields onto the
// submission document itself so that API consumers always have a reliable filename to display
// even if the nested File relation is trimmed or omitted by the serializer.
func enrichSubmissionDocumentsWithFileMetadata(documents []models.SubmissionDocument) {
	for i := range documents {
		file := documents[i].File
		if file.FileID == 0 {
			continue
		}

		if documents[i].OriginalName == "" {
			documents[i].OriginalName = file.OriginalName
		}
		if documents[i].OriginalFilename == "" {
			documents[i].OriginalFilename = file.OriginalName
		}
		if documents[i].FileName == "" {
			documents[i].FileName = file.OriginalName
		}
		if documents[i].FilePath == "" {
			documents[i].FilePath = file.StoredPath
		}
	}
}
