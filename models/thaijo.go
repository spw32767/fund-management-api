package models

import "time"

type ThaiJODocument struct {
	ID             uint64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	ThaiJOArticleID string     `json:"thaijo_article_id" gorm:"column:thaijo_article_id;type:varchar(128);uniqueIndex"`
	ArticleURL     *string    `json:"article_url,omitempty" gorm:"column:article_url;type:text"`
	JournalID      *int       `json:"journal_id,omitempty" gorm:"column:journal_id"`
	JournalPath    *string    `json:"journal_path,omitempty" gorm:"column:journal_path;type:varchar(128)"`
	JournalURL     *string    `json:"journal_url,omitempty" gorm:"column:journal_url;type:text"`
	TitleEN        *string    `json:"title_en,omitempty" gorm:"column:title_en;type:text"`
	TitleTH        *string    `json:"title_th,omitempty" gorm:"column:title_th;type:text"`
	AbstractEN     *string    `json:"abstract_en,omitempty" gorm:"column:abstract_en;type:longtext"`
	AbstractTH     *string    `json:"abstract_th,omitempty" gorm:"column:abstract_th;type:longtext"`
	Year           *int       `json:"year,omitempty" gorm:"column:year"`
	DatePublished  *time.Time `json:"date_published,omitempty" gorm:"column:date_published"`
	DOI            *string    `json:"doi,omitempty" gorm:"column:doi;type:varchar(255)"`
	PDFURL         *string    `json:"pdf_url,omitempty" gorm:"column:pdf_url;type:text"`
	PublicationID  *int       `json:"publication_id,omitempty" gorm:"column:publication_id"`
	SubmissionID   *int       `json:"submission_id,omitempty" gorm:"column:submission_id"`
	LanguagesJSON  []byte     `json:"languages_json,omitempty" gorm:"column:languages_json"`
	KeywordsJSON   []byte     `json:"keywords_json,omitempty" gorm:"column:keywords_json"`
	RawJSON        []byte     `json:"raw_json,omitempty" gorm:"column:raw_json"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (ThaiJODocument) TableName() string { return "thaijo_documents" }

type ThaiJOAuthor struct {
	ID             uint64    `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	ThaiJOAuthorID string    `json:"thaijo_author_id" gorm:"column:thaijo_author_id;type:varchar(128);uniqueIndex"`
	IdentityKey    *string   `json:"identity_key,omitempty" gorm:"column:identity_key;type:varchar(255)"`
	ORCID          *string   `json:"orcid,omitempty" gorm:"column:orcid;type:varchar(255)"`
	FullNameEN     *string   `json:"full_name_en,omitempty" gorm:"column:full_name_en;type:text"`
	FullNameTH     *string   `json:"full_name_th,omitempty" gorm:"column:full_name_th;type:text"`
	GivenNameEN    *string   `json:"given_name_en,omitempty" gorm:"column:given_name_en;type:text"`
	GivenNameTH    *string   `json:"given_name_th,omitempty" gorm:"column:given_name_th;type:text"`
	FamilyNameEN   *string   `json:"family_name_en,omitempty" gorm:"column:family_name_en;type:text"`
	FamilyNameTH   *string   `json:"family_name_th,omitempty" gorm:"column:family_name_th;type:text"`
	Country        *string   `json:"country,omitempty" gorm:"column:country;type:varchar(16)"`
	ArticleCount   *int      `json:"article_count,omitempty" gorm:"column:article_count"`
	ArticleIDsJSON []byte    `json:"article_ids_json,omitempty" gorm:"column:article_ids_json"`
	AffiliationsJSON []byte  `json:"affiliations_json,omitempty" gorm:"column:affiliations_json"`
	JournalPathsJSON []byte  `json:"journal_paths_json,omitempty" gorm:"column:journal_paths_json"`
	JournalsJSON   []byte    `json:"journals_json,omitempty" gorm:"column:journals_json"`
	YearsJSON      []byte    `json:"years_json,omitempty" gorm:"column:years_json"`
	RawJSON        []byte    `json:"raw_json,omitempty" gorm:"column:raw_json"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (ThaiJOAuthor) TableName() string { return "thaijo_authors" }

type ThaiJOJournal struct {
	ID             uint64    `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	ThaiJOJournalKey string   `json:"thaijo_journal_key" gorm:"column:thaijo_journal_key;type:varchar(128);uniqueIndex"`
	JournalID      *int      `json:"journal_id,omitempty" gorm:"column:journal_id"`
	Path           *string   `json:"path,omitempty" gorm:"column:path;type:varchar(128)"`
	Acronym        *string   `json:"acronym,omitempty" gorm:"column:acronym;type:varchar(128)"`
	Category       *string   `json:"category,omitempty" gorm:"column:category;type:varchar(128)"`
	JournalURL     *string   `json:"journal_url,omitempty" gorm:"column:journal_url;type:text"`
	NameEN         *string   `json:"name_en,omitempty" gorm:"column:name_en;type:text"`
	NameTH         *string   `json:"name_th,omitempty" gorm:"column:name_th;type:text"`
	OnlineISSN     *string   `json:"online_issn,omitempty" gorm:"column:online_issn;type:varchar(64)"`
	PrintISSN      *string   `json:"print_issn,omitempty" gorm:"column:print_issn;type:varchar(64)"`
	Tier           *int      `json:"tier,omitempty" gorm:"column:tier"`
	TierPeriod     *string   `json:"tier_period,omitempty" gorm:"column:tier_period;type:varchar(64)"`
	Enabled        *bool     `json:"enabled,omitempty" gorm:"column:enabled"`
	RawJSON        []byte    `json:"raw_json,omitempty" gorm:"column:raw_json"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (ThaiJOJournal) TableName() string { return "thaijo_journals" }

type ThaiJODocumentAuthor struct {
	ID         uint64    `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	DocumentID uint64    `json:"document_id" gorm:"column:document_id"`
	AuthorID   *uint64   `json:"author_id,omitempty" gorm:"column:author_id"`
	AuthorSeq  *int      `json:"author_seq,omitempty" gorm:"column:author_seq"`
	NameEN     *string   `json:"name_en,omitempty" gorm:"column:name_en;type:text"`
	NameTH     *string   `json:"name_th,omitempty" gorm:"column:name_th;type:text"`
	CreatedAt  time.Time `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (ThaiJODocumentAuthor) TableName() string { return "thaijo_document_authors" }

type ThaiJORejectedHit struct {
	ID             uint64    `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	JobID          *uint64   `json:"job_id,omitempty" gorm:"column:job_id"`
	UserID         *uint     `json:"user_id,omitempty" gorm:"column:user_id"`
	ThaiJOArticleID *string   `json:"thaijo_article_id,omitempty" gorm:"column:thaijo_article_id;type:varchar(128)"`
	Reason         string    `json:"reason" gorm:"column:reason;type:varchar(64);not null"`
	AuthorMatch    bool      `json:"author_match" gorm:"column:author_match;not null"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at;autoCreateTime"`
}

func (ThaiJORejectedHit) TableName() string { return "thaijo_rejected_hits" }
