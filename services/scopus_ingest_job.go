package services

import (
	"context"
	"errors"
	"log"
	"strings"

	"fund-management-api/config"

	"gorm.io/gorm"
)

// ScopusIngestUserInput represents the data required to ingest publications for a single user.
type ScopusIngestUserInput struct {
	UserID         uint
	ScopusAuthorID string
}

// ScopusIngestAllInput controls the behaviour when running for many users.
type ScopusIngestAllInput struct {
	UserIDs []uint
	Limit   int
}

// ScopusIngestJobSummary summarises a job run over multiple users.
type ScopusIngestJobSummary struct {
	UsersProcessed      int `json:"users_processed"`
	UsersWithErrors     int `json:"users_with_errors"`
	DocumentsFetched    int `json:"documents_fetched"`
	DocumentsCreated    int `json:"documents_created"`
	DocumentsUpdated    int `json:"documents_updated"`
	DocumentsFailed     int `json:"documents_failed"`
	AuthorsCreated      int `json:"authors_created"`
	AuthorsUpdated      int `json:"authors_updated"`
	AffiliationsCreated int `json:"affiliations_created"`
	AffiliationsUpdated int `json:"affiliations_updated"`
	LinksInserted       int `json:"links_inserted"`
	LinksUpdated        int `json:"links_updated"`
}

// ScopusIngestJobService coordinates ingestion for one or many users.
type ScopusIngestJobService struct {
	db     *gorm.DB
	ingest *ScopusIngestService
}

// NewScopusIngestJobService constructs a ScopusIngestJobService.
func NewScopusIngestJobService(db *gorm.DB) *ScopusIngestJobService {
	if db == nil {
		db = config.DB
	}
	return &ScopusIngestJobService{
		db:     db,
		ingest: NewScopusIngestService(db, nil),
	}
}

// RunForUser executes the ingest for a single user.
func (s *ScopusIngestJobService) RunForUser(ctx context.Context, input *ScopusIngestUserInput) (*ScopusIngestResult, error) {
	if input == nil {
		return nil, errors.New("input is nil")
	}
	if input.UserID == 0 {
		return nil, errors.New("user id is required")
	}
	if strings.TrimSpace(input.ScopusAuthorID) == "" {
		return nil, errors.New("scopus author id is required")
	}
	return s.ingest.RunForAuthor(ctx, input.ScopusAuthorID)
}

// RunForAll executes the ingest for all users matching the provided filter.
func (s *ScopusIngestJobService) RunForAll(ctx context.Context, input *ScopusIngestAllInput) (*ScopusIngestJobSummary, error) {
	if input == nil {
		input = &ScopusIngestAllInput{}
	}

	summary := &ScopusIngestJobSummary{}

	type userRow struct {
		UserID   uint
		ScopusID string
	}

	query := s.db.WithContext(ctx).Table("users").
		Select("user_id, scopus_id AS scopus_id").
		Where("scopus_id IS NOT NULL AND scopus_id <> ''")

	if len(input.UserIDs) > 0 {
		query = query.Where("user_id IN ?", input.UserIDs)
	}
	if input.Limit > 0 {
		query = query.Limit(input.Limit)
	}

	var users []userRow
	if err := query.Order("user_id ASC").Find(&users).Error; err != nil {
		return nil, err
	}

	for _, user := range users {
		res, err := s.ingest.RunForAuthor(ctx, user.ScopusID)
		if err != nil {
			summary.UsersWithErrors++
			log.Printf("scopus ingest failed for user %d: %v", user.UserID, err)
			continue
		}
		summary.UsersProcessed++
		summary.DocumentsFetched += res.DocumentsFetched
		summary.DocumentsCreated += res.DocumentsCreated
		summary.DocumentsUpdated += res.DocumentsUpdated
		summary.DocumentsFailed += res.DocumentsFailed
		summary.AuthorsCreated += res.AuthorsCreated
		summary.AuthorsUpdated += res.AuthorsUpdated
		summary.AffiliationsCreated += res.AffiliationsCreated
		summary.AffiliationsUpdated += res.AffiliationsUpdated
		summary.LinksInserted += res.DocumentAuthorsInserted
		summary.LinksUpdated += res.DocumentAuthorsUpdated
	}

	return summary, nil
}
