package models

// AF-ID is stored directly: unnamed affiliations in author payloads are still
// evidence, even when there is no corresponding affiliation catalogue row.
type BenchmarkDocumentAffiliation struct {
	DocumentID uint   `gorm:"primaryKey;column:document_id"`
	Afid       string `gorm:"primaryKey;column:afid"`
	Provenance string `gorm:"column:provenance"`
}

func (BenchmarkDocumentAffiliation) TableName() string {
	return "scopus_benchmark_document_affiliations"
}

type BenchmarkAuthorAffiliation struct {
	DocumentID uint   `gorm:"primaryKey;column:document_id"`
	AuthorID   uint   `gorm:"primaryKey;column:author_id"`
	Afid       string `gorm:"primaryKey;column:afid"`
	Provenance string `gorm:"column:provenance"`
}

func (BenchmarkAuthorAffiliation) TableName() string { return "scopus_benchmark_author_affiliations" }
