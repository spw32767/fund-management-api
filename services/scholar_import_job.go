package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

var (
	ErrScholarImportAlreadyRunning = errors.New("scholar import already running")
)

type ScholarImportSummary struct {
	UsersProcessed      int `json:"users"`
	UsersWithErrors     int `json:"users_with_errors"`
	PublicationsFetched int `json:"fetched"`
	PublicationsCreated int `json:"created"`
	PublicationsUpdated int `json:"updated"`
	PublicationsFailed  int `json:"failed"`
}

type ScholarImportUserSummary struct {
	PublicationsFetched int `json:"fetched"`
	PublicationsCreated int `json:"created"`
	PublicationsUpdated int `json:"updated"`
	PublicationsFailed  int `json:"failed"`
}

type ScholarImportUserInput struct {
	UserID   uint
	AuthorID string
	DryRun   bool
}

type ScholarImportAllInput struct {
	UserIDs       []uint
	Limit         int
	TriggerSource string
	LockName      string
	DryRun        bool
	RecordRun     bool
}

type ScholarImportJobService struct {
	db         *gorm.DB
	pubs       *PublicationService
	metrics    *UserScholarMetricsService
	runService *ScholarImportRunService
}

func NewScholarImportJobService(db *gorm.DB) *ScholarImportJobService {
	if db == nil {
		db = config.DB
	}
	return &ScholarImportJobService{
		db:         db,
		pubs:       NewPublicationService(db),
		metrics:    NewUserScholarMetricsService(db),
		runService: NewScholarImportRunService(db),
	}
}

func (s *ScholarImportJobService) RunForUser(ctx context.Context, input *ScholarImportUserInput) (*ScholarImportUserSummary, error) {
	if input == nil {
		return nil, errors.New("input is nil")
	}
	if input.UserID == 0 {
		return nil, errors.New("user_id is required")
	}
	if strings.TrimSpace(input.AuthorID) == "" {
		return nil, errors.New("author_id is required")
	}

	res, err := s.processUser(ctx, input.UserID, input.AuthorID, input.DryRun)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *ScholarImportJobService) RunForAll(ctx context.Context, input *ScholarImportAllInput) (*ScholarImportSummary, error) {
	if input == nil {
		return nil, errors.New("input is nil")
	}
	recordRun := input.RecordRun
	summary := &ScholarImportSummary{}

	release, err := s.acquireLock(ctx, input.LockName)
	if err != nil {
		return nil, err
	}
	if release != nil {
		defer func() {
			if relErr := release(); relErr != nil {
				log.Printf("failed to release scholar import lock: %v", relErr)
			}
		}()
	}

	var run *models.ScholarImportRun
	if recordRun {
		run, err = s.runService.Start(input.TriggerSource)
		if err != nil {
			return nil, err
		}
	}

	var finalErr error
	if run != nil {
		defer func() {
			if finalErr != nil {
				if err := s.runService.MarkFailure(run.ID, summary, finalErr); err != nil {
					log.Printf("failed to mark scholar import run failure: %v", err)
				}
			} else {
				if err := s.runService.MarkSuccess(run.ID, summary); err != nil {
					log.Printf("failed to mark scholar import run success: %v", err)
				}
			}
		}()
	}

	type userRow struct {
		UserID          uint
		ScholarAuthorID string
	}

	query := s.db.WithContext(ctx).Table("users").
		Select("user_id, scholar_author_id").
		Where("scholar_author_id IS NOT NULL AND scholar_author_id <> ''")

	if len(input.UserIDs) > 0 {
		query = query.Where("user_id IN ?", input.UserIDs)
	}
	if input.Limit > 0 {
		query = query.Limit(input.Limit)
	}

	var users []userRow
	if err := query.Order("user_id ASC").Find(&users).Error; err != nil {
		finalErr = err
		return nil, err
	}

	for _, u := range users {
		res, err := s.processUser(ctx, u.UserID, u.ScholarAuthorID, input.DryRun)
		if err != nil {
			summary.UsersWithErrors++
			log.Printf("scholar import failed for user %d: %v", u.UserID, err)
			continue
		}
		summary.UsersProcessed++
		summary.PublicationsFetched += res.PublicationsFetched
		summary.PublicationsCreated += res.PublicationsCreated
		summary.PublicationsUpdated += res.PublicationsUpdated
		summary.PublicationsFailed += res.PublicationsFailed
	}

	return summary, nil
}

func (s *ScholarImportJobService) processUser(ctx context.Context, userID uint, authorID string, dryRun bool) (*ScholarImportUserSummary, error) {
	pubs, err := FetchScholarOnce(authorID)
	if err != nil {
		return nil, &ScholarScriptError{AuthorID: authorID, Err: err}
	}

	res := &ScholarImportUserSummary{PublicationsFetched: len(pubs)}

	if !dryRun {
		if err := s.updateAuthorMetrics(ctx, userID, authorID); err != nil {
			log.Printf("failed to update scholar metrics for user %d: %v", userID, err)
		}
	}

	for _, sp := range pubs {
		title := sp.Title
		authorsStr := strings.Join(sp.Authors, ", ")
		source := "scholar"

		var journal *string
		if sp.Venue != nil && *sp.Venue != "" {
			journal = sp.Venue
		}

		var yearPtr *uint16
		if sp.Year != nil && *sp.Year > 0 {
			yy := uint16(*sp.Year)
			yearPtr = &yy
		}

		var externalJSON *string
		if sp.ScholarClusterID != nil && *sp.ScholarClusterID != "" {
			js := fmt.Sprintf(`{"scholar_cluster_id":"%s"}`, *sp.ScholarClusterID)
			externalJSON = &js
		}

		var citedBy *uint
		if sp.NumCitations != nil && *sp.NumCitations >= 0 {
			cb := uint(*sp.NumCitations)
			citedBy = &cb
		}

		var citationHistory *string
		if sp.CitesPerYear != nil && len(sp.CitesPerYear) > 0 {
			if b, err := json.Marshal(sp.CitesPerYear); err == nil {
				s := string(b)
				citationHistory = &s
			}
		}

		pub := &models.UserPublication{
			UserID:          userID,
			Title:           title,
			Authors:         &authorsStr,
			Journal:         journal,
			PublicationType: nil,
			PublicationDate: nil,
			PublicationYear: yearPtr,
			DOI:             sp.DOI,
			URL:             sp.URL,
			CitedBy:         citedBy,
			CitedByURL:      sp.CitedByURL,
			Source:          &source,
			ExternalIDs:     externalJSON,
			CitationHistory: citationHistory,
		}

		if dryRun {
			continue
		}

		created, _, e := s.pubs.Upsert(pub)
		if e != nil {
			res.PublicationsFailed++
			log.Printf("failed to upsert publication for user %d: %v", userID, e)
			continue
		}
		if created {
			res.PublicationsCreated++
		} else {
			res.PublicationsUpdated++
		}
	}

	return res, nil
}

func (s *ScholarImportJobService) updateAuthorMetrics(ctx context.Context, userID uint, authorID string) error {
	ai, err := FetchScholarAuthorIndices(authorID)
	if err != nil || ai == nil {
		return err
	}

	var citesPerYear *string
	if len(ai.CitesPerYear) > 0 {
		if b, e := json.Marshal(ai.CitesPerYear); e == nil {
			str := string(b)
			citesPerYear = &str
		}
	}

	metrics := &models.UserScholarMetrics{
		UserID:       int(userID),
		HIndex:       ai.HIndex,
		HIndex5Y:     ai.HIndex5Y,
		I10Index:     ai.I10Index,
		I10Index5Y:   ai.I10Index5Y,
		CitedByTotal: ai.CitedByTotal,
		CitedBy5Y:    ai.CitedBy5Y,
		CitesPerYear: citesPerYear,
	}
	if err := s.metrics.Upsert(metrics); err != nil {
		return err
	}

	updates := map[string]interface{}{
		"scholar_hindex":         ai.HIndex,
		"scholar_hindex5y":       ai.HIndex5Y,
		"scholar_i10index":       ai.I10Index,
		"scholar_i10index5y":     ai.I10Index5Y,
		"scholar_citedby_total":  ai.CitedByTotal,
		"scholar_citedby_5y":     ai.CitedBy5Y,
		"scholar_cites_per_year": citesPerYear,
	}
	return s.db.WithContext(ctx).Table("users").Where("user_id = ?", userID).Updates(updates).Error
}

func (s *ScholarImportJobService) acquireLock(ctx context.Context, lockName string) (func() error, error) {
	if strings.TrimSpace(lockName) == "" {
		return nil, nil
	}

	var ok int
	if err := s.db.WithContext(ctx).Raw("SELECT GET_LOCK(?, 0)", lockName).Scan(&ok).Error; err != nil {
		return nil, err
	}
	if ok != 1 {
		return nil, ErrScholarImportAlreadyRunning
	}

	return func() error {
		var released int
		if err := s.db.WithContext(ctx).Raw("SELECT RELEASE_LOCK(?)", lockName).Scan(&released).Error; err != nil {
			return err
		}
		return nil
	}, nil
}

// ScholarScriptError indicates the Python script failed to return data.
type ScholarScriptError struct {
	AuthorID string
	Err      error
}

func (e *ScholarScriptError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("scholar script error for author %s: %v", e.AuthorID, e.Err)
}

func (e *ScholarScriptError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
