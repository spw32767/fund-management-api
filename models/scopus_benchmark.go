package models

import "time"

// ScopusBenchmarkScope registers a harvest target (whole university / whole country)
// used for comparing Computer Science output against the faculty.
type ScopusBenchmarkScope struct {
	ID           uint64    `gorm:"primaryKey;column:id" json:"id"`
	Code         string    `gorm:"column:code" json:"code"`
	Label        string    `gorm:"column:label" json:"label"`
	Level        string    `gorm:"column:level" json:"level"`
	AfID         *string   `gorm:"column:af_id" json:"af_id,omitempty"`
	AffilCountry *string   `gorm:"column:affil_country" json:"affil_country,omitempty"`
	SubjectArea  string    `gorm:"column:subject_area" json:"subject_area"`
	ExtraQuery   *string   `gorm:"column:extra_query" json:"extra_query,omitempty"`
	Active       bool      `gorm:"column:active" json:"active"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (ScopusBenchmarkScope) TableName() string { return "scopus_benchmark_scopes" }

// ScopusBenchmarkDocument mirrors scopus_documents but lives in an isolated table
// so university/country harvests never leak into the faculty-facing views.
type ScopusBenchmarkDocument struct {
	ID                 uint       `gorm:"primaryKey;column:id" json:"id"`
	EID                string     `gorm:"column:eid;uniqueIndex" json:"eid"`
	ScopusID           *string    `gorm:"column:scopus_id" json:"scopus_id,omitempty"`
	ScopusLink         *string    `gorm:"column:scopus_link" json:"scopus_link,omitempty"`
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
	PubYear            *int       `gorm:"column:pub_year" json:"pub_year,omitempty"`
	RawJSON            []byte     `gorm:"column:raw_json" json:"raw_json,omitempty"`
	FirstSeenAt        *time.Time `gorm:"column:first_seen_at" json:"first_seen_at,omitempty"`
	LastSeenAt         *time.Time `gorm:"column:last_seen_at" json:"last_seen_at,omitempty"`
	CreatedAt          time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (ScopusBenchmarkDocument) TableName() string { return "scopus_benchmark_documents" }

// ScopusBenchmarkAuthor mirrors scopus_authors for the benchmark dataset.
type ScopusBenchmarkAuthor struct {
	ID             uint      `gorm:"primaryKey;column:id" json:"id"`
	ScopusAuthorID string    `gorm:"column:scopus_author_id;uniqueIndex" json:"scopus_author_id"`
	FullName       *string   `gorm:"column:full_name" json:"full_name,omitempty"`
	GivenName      *string   `gorm:"column:given_name" json:"given_name,omitempty"`
	Surname        *string   `gorm:"column:surname" json:"surname,omitempty"`
	Initials       *string   `gorm:"column:initials" json:"initials,omitempty"`
	AuthorURL      *string   `gorm:"column:author_url" json:"author_url,omitempty"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (ScopusBenchmarkAuthor) TableName() string { return "scopus_benchmark_authors" }

// ScopusBenchmarkDocumentAuthor links a benchmark document to an author, flagging
// authors that belong to our faculty (users.scopus_id) so the faculty subset can be derived.
type ScopusBenchmarkDocumentAuthor struct {
	ID         uint `gorm:"primaryKey;column:id" json:"id"`
	DocumentID uint `gorm:"column:document_id" json:"document_id"`
	AuthorID   uint `gorm:"column:author_id" json:"author_id"`
	AuthorSeq  *int `gorm:"column:author_seq" json:"author_seq,omitempty"`
	IsFaculty  bool `gorm:"column:is_faculty" json:"is_faculty"`
}

func (ScopusBenchmarkDocumentAuthor) TableName() string {
	return "scopus_benchmark_document_authors"
}

// ScopusBenchmarkDocumentScope records that a document belongs to a harvest scope.
type ScopusBenchmarkDocumentScope struct {
	ID         uint      `gorm:"primaryKey;column:id" json:"id"`
	DocumentID uint      `gorm:"column:document_id" json:"document_id"`
	ScopeID    uint64    `gorm:"column:scope_id" json:"scope_id"`
	PubYear    *int      `gorm:"column:pub_year" json:"pub_year,omitempty"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (ScopusBenchmarkDocumentScope) TableName() string {
	return "scopus_benchmark_document_scopes"
}

// ScopusBenchmarkHarvestRun tracks a count or harvest run for a scope.
type ScopusBenchmarkHarvestRun struct {
	ID                   uint64     `gorm:"primaryKey;column:id" json:"id"`
	ScopeID              *uint64    `gorm:"column:scope_id" json:"scope_id,omitempty"`
	RunType              string     `gorm:"column:run_type" json:"run_type"`
	YearFrom             *int       `gorm:"column:year_from" json:"year_from,omitempty"`
	YearTo               *int       `gorm:"column:year_to" json:"year_to,omitempty"`
	Status               string     `gorm:"column:status" json:"status"`
	TotalResultsReported *int       `gorm:"column:total_results_reported" json:"total_results_reported,omitempty"`
	PagesFetched         int        `gorm:"column:pages_fetched" json:"pages_fetched"`
	DocumentsUpserted    int        `gorm:"column:documents_upserted" json:"documents_upserted"`
	RequestsMade         int        `gorm:"column:requests_made" json:"requests_made"`
	CursorState          *string    `gorm:"column:cursor_state" json:"cursor_state,omitempty"`
	ErrorMessage         *string    `gorm:"column:error_message" json:"error_message,omitempty"`
	StartedAt            time.Time  `gorm:"column:started_at;autoCreateTime" json:"started_at"`
	FinishedAt           *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	DurationSeconds      *float64   `gorm:"column:duration_seconds" json:"duration_seconds,omitempty"`
	CreatedAt            time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (ScopusBenchmarkHarvestRun) TableName() string { return "scopus_benchmark_harvest_runs" }

// ScopusBenchmarkCountSnapshot stores a point-in-time document count for a scope/year.
type ScopusBenchmarkCountSnapshot struct {
	ID           uint64    `gorm:"primaryKey;column:id" json:"id"`
	ScopeID      uint64    `gorm:"column:scope_id" json:"scope_id"`
	SubjectArea  string    `gorm:"column:subject_area" json:"subject_area"`
	PubYear      *int      `gorm:"column:pub_year" json:"pub_year,omitempty"`
	TotalResults int       `gorm:"column:total_results" json:"total_results"`
	CapturedAt   time.Time `gorm:"column:captured_at" json:"captured_at"`
}

func (ScopusBenchmarkCountSnapshot) TableName() string { return "scopus_benchmark_count_snapshots" }
