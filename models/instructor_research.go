package models

import (
    "time"  
)

// ResearchDocument คือโมเดลรวมที่ส่ง JSON ไปหา Frontend (Read-Only)
// อิงจาก ScopusDocument และ ThaiJODocument
type ResearchDocument struct {
	ID         uint   `json:"id"`
	UserID     uint   `json:"user_id"`
	SourceType string `json:"source_type"` // "scopus" | "thaijo" | "conference"

	// ── Authors ──────────────────────────────────────
	Authors string `json:"authors"`

	// ── Content ──────────────────────────────────────
	Title       string `json:"title"`
	PublishYear int    `json:"publish_year"`
	DOI         string `json:"doi"`
	ArticleURL  string `json:"article_url"`

	// ── Journal-only ─────────────────────────────────
	JournalName string `json:"journal_name,omitempty"`
	Volume      string `json:"volume,omitempty"`
	Issue       string `json:"issue,omitempty"`
	Pages       string `json:"pages,omitempty"`

	// ── Conference-only ──────────────────────────────
	IsConference        bool       `json:"is_conference"`
	ConferenceName      string     `json:"conference_name,omitempty"`
	ConferenceVenue    *string    `json:"conference_venue,omitempty"`
	City                string     `json:"city,omitempty"`
	Country             string     `json:"country,omitempty"`
	ConferenceDateStart *time.Time `json:"conference_date_start,omitempty"`
	ConferenceDateEnd   *time.Time `json:"conference_date_end,omitempty"`
	CoverDisplayDate string `json:"cover_display_date"`
	

	// ── Tier / Ranking ───────────────────────────────
	TierDetails *RankingTierWeight `json:"tier_details,omitempty" gorm:"-"`

	UpdatedAt time.Time `json:"updated_at"`
}