package models

import "time"

// ScopusDocument represents a document fetched from the Scopus API.
type ScopusDocument struct {
	ID                 uint       `gorm:"primaryKey;column:id" json:"id"`
	EID                string     `gorm:"column:eid;uniqueIndex" json:"eid"`
	ScopusID           *string    `gorm:"column:scopus_id" json:"scopus_id,omitempty"`
	Title              *string    `gorm:"column:title" json:"title,omitempty"`
	Abstract           *string    `gorm:"column:abstract" json:"abstract,omitempty"`
	AggregationType    *string    `gorm:"column:aggregation_type" json:"aggregation_type,omitempty"`
	Subtype            *string    `gorm:"column:subtype" json:"subtype,omitempty"`
	SubtypeDescription *string    `gorm:"column:subtype_description" json:"subtype_description,omitempty"`
	SourceID           *string    `gorm:"column:source_id" json:"source_id,omitempty"`
	PublicationName    *string    `gorm:"column:publication_name" json:"publication_name,omitempty"`
	ISSN               *string    `gorm:"column:issn" json:"issn,omitempty"`
	EISSN              *string    `gorm:"column:eissn" json:"eissn,omitempty"`
	ISBN               *string    `gorm:"column:isbn" json:"isbn,omitempty"`
	Volume             *string    `gorm:"column:volume" json:"volume,omitempty"`
	Issue              *string    `gorm:"column:issue" json:"issue,omitempty"`
	PageRange          *string    `gorm:"column:page_range" json:"page_range,omitempty"`
	ArticleNumber      *string    `gorm:"column:article_number" json:"article_number,omitempty"`
	CoverDate          *time.Time `gorm:"column:cover_date" json:"cover_date,omitempty"`
	CoverDisplayDate   *string    `gorm:"column:cover_display_date" json:"cover_display_date,omitempty"`
	DOI                *string    `gorm:"column:doi" json:"doi,omitempty"`
	PII                *string    `gorm:"column:pii" json:"pii,omitempty"`
	CitedByCount       *int       `gorm:"column:citedby_count" json:"citedby_count,omitempty"`
	OpenAccess         *uint8     `gorm:"column:openaccess" json:"openaccess,omitempty"`
	OpenAccessFlag     *uint8     `gorm:"column:openaccess_flag" json:"openaccess_flag,omitempty"`
	AuthKeywords       []byte     `gorm:"column:authkeywords" json:"authkeywords,omitempty"`
	FundAcr            *string    `gorm:"column:fund_acr" json:"fund_acr,omitempty"`
	FundSponsor        *string    `gorm:"column:fund_sponsor" json:"fund_sponsor,omitempty"`
	RawJSON            []byte     `gorm:"column:raw_json" json:"raw_json,omitempty"`
}

// TableName overrides the table name used by ScopusDocument to `scopus_documents`.
func (ScopusDocument) TableName() string {
	return "scopus_documents"
}

// ScopusAuthor represents an author entry from Scopus.
type ScopusAuthor struct {
	ID             uint    `gorm:"primaryKey;column:id" json:"id"`
	ScopusAuthorID string  `gorm:"column:scopus_author_id;uniqueIndex" json:"scopus_author_id"`
	FullName       *string `gorm:"column:full_name" json:"full_name,omitempty"`
	GivenName      *string `gorm:"column:given_name" json:"given_name,omitempty"`
	Surname        *string `gorm:"column:surname" json:"surname,omitempty"`
	Initials       *string `gorm:"column:initials" json:"initials,omitempty"`
	ORCID          *string `gorm:"column:orcid" json:"orcid,omitempty"`
	AuthorURL      *string `gorm:"column:author_url" json:"author_url,omitempty"`
}

// TableName overrides the table name used by ScopusAuthor to `scopus_authors`.
func (ScopusAuthor) TableName() string {
	return "scopus_authors"
}

// ScopusAffiliation represents an affiliation associated with a Scopus document.
type ScopusAffiliation struct {
	ID             uint    `gorm:"primaryKey;column:id" json:"id"`
	Afid           string  `gorm:"column:afid;uniqueIndex" json:"afid"`
	Name           *string `gorm:"column:name" json:"name,omitempty"`
	City           *string `gorm:"column:city" json:"city,omitempty"`
	Country        *string `gorm:"column:country" json:"country,omitempty"`
	AffiliationURL *string `gorm:"column:affiliation_url" json:"affiliation_url,omitempty"`
}

// TableName overrides the table name used by ScopusAffiliation to `scopus_affiliations`.
func (ScopusAffiliation) TableName() string {
	return "scopus_affiliations"
}

// ScopusDocumentAuthor represents the link between a Scopus document and an author.
type ScopusDocumentAuthor struct {
	ID            uint  `gorm:"primaryKey;column:id" json:"id"`
	DocumentID    uint  `gorm:"column:document_id" json:"document_id"`
	AuthorID      uint  `gorm:"column:author_id" json:"author_id"`
	AuthorSeq     int   `gorm:"column:author_seq" json:"author_seq"`
	AffiliationID *uint `gorm:"column:affiliation_id" json:"affiliation_id,omitempty"`
}

// TableName overrides the table name used by ScopusDocumentAuthor to `scopus_document_authors`.
func (ScopusDocumentAuthor) TableName() string {
	return "scopus_document_authors"
}

// ScopusConfig represents configuration rows stored in the scopus_config table.
type ScopusConfig struct {
	ID    uint    `gorm:"primaryKey;column:id" json:"id"`
	Key   string  `gorm:"column:key" json:"key"`
	Value *string `gorm:"column:value" json:"value,omitempty"`
}

// TableName overrides the table name used by ScopusConfig to `scopus_config`.
func (ScopusConfig) TableName() string {
	return "scopus_config"
}
