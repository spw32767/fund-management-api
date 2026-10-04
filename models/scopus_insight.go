package models

import "time"

// Separate metadata avoids unrelated ScopusDocument.Save calls resetting derived state.
type ScopusDocumentInsight struct {
	DocumentID           uint      `gorm:"primaryKey;autoIncrement:false" json:"document_id"`
	International        *bool     `gorm:"column:international_collaboration" json:"international_collaboration"`
	AffiliationsComplete bool      `json:"affiliations_complete"`
	CountriesComplete    bool      `json:"countries_complete"`
	Status               string    `json:"status"`
	ReasonsJSON          string    `json:"reasons_json"`
	DiagnosticsJSON      string    `json:"diagnostics_json"`
	Provenance           string    `json:"provenance"`
	NormalizerVersion    string    `json:"normalizer_version"`
	PayloadHash          string    `json:"payload_hash"`
	CatalogueHash        string    `json:"catalogue_hash"`
	CheckedAt            time.Time `json:"checked_at"`
}

func (ScopusDocumentInsight) TableName() string { return "scopus_document_insights" }

type ScopusDocumentAffiliation struct {
	DocumentID     uint   `gorm:"primaryKey;autoIncrement:false"`
	Afid           string `gorm:"primaryKey;size:32"`
	Provenance     string
	PayloadCountry string
}

func (ScopusDocumentAffiliation) TableName() string { return "scopus_document_affiliations" }

type ScopusDocumentCountry struct {
	DocumentID  uint   `gorm:"primaryKey;autoIncrement:false"`
	CountryKey  string `gorm:"primaryKey;size:96"`
	CountryName string
	Provenance  string
}

func (ScopusDocumentCountry) TableName() string { return "scopus_document_countries" }
