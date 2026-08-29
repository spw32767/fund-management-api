package controllers

import (
	"fund-management-api/models"
	"strings"
	"testing"
)

// The "ทั้งนี้ได้แนบ..." evidence list ({{document_line}}) must list only the user's
// supporting documents, never the files the system auto-generates after submit
// (request-form DOCX/PDF and the merged PDF). This guards the leak reported for
// re-submitted (returned) applications, where the previous generated files still exist
// when the list is built.
func TestBuildDocumentLine_ExcludesGeneratedForms(t *testing.T) {
	docs := []models.SubmissionDocument{
		{DocumentTypeID: 11, DocumentTypeName: "Full Reprint (บทความตีพิมพ์)"},
		{DocumentTypeID: 12, DocumentTypeName: "สำเนาบัญชีธนาคาร"},
		// Auto-generated files: given real names so that WITHOUT the code filter they
		// would appear — the test proves the code-based filter removes them.
		{DocumentTypeName: "แบบฟอร์มคำขอรับเงินรางวัล (DOCX)", DocumentType: models.DocumentType{Code: publicationRewardFormDocumentCode}},
		{DocumentTypeName: "แบบฟอร์มคำขอรับเงินรางวัล (PDF)", DocumentType: models.DocumentType{Code: publicationRewardFormPdfDocumentCode}},
		{DocumentTypeName: "แบบฟอร์มคำร้องรวม (merged pdf)", DocumentType: models.DocumentType{Code: mergedSubmissionDocumentTypeCode}},
	}

	line := buildDocumentLine(docs)

	for _, want := range []string{"Full Reprint", "สำเนาบัญชีธนาคาร"} {
		if !strings.Contains(line, want) {
			t.Errorf("expected user document %q to be listed; got:\n%s", want, line)
		}
	}
	for _, bad := range []string{"แบบฟอร์มคำขอรับเงินรางวัล", "merged pdf", "คำร้องรวม", "Auto Generated"} {
		if strings.Contains(line, bad) {
			t.Errorf("auto-generated file %q must NOT be listed; got:\n%s", bad, line)
		}
	}
}
