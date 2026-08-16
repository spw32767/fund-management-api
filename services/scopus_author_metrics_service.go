package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const authorMetricsBaseURL = "https://api.elsevier.com/content/author/author_id/"

const authorMetricsRunLockName = "scopus_author_metrics_job_lock"

// ErrAuthorMetricsAlreadyRunning is returned when a batch run is already in progress.
var ErrAuthorMetricsAlreadyRunning = errors.New("scopus author metrics job already running")

// ScopusAuthorMetricsAllInput controls the behaviour when running for many users.
type ScopusAuthorMetricsAllInput struct {
	UserIDs       []uint
	Limit         int
	TriggerSource string
}

// ScopusAuthorMetricsSummary summarises a batch run over multiple users.
type ScopusAuthorMetricsSummary struct {
	UsersProcessed  int `json:"users_processed"`
	UsersWithErrors int `json:"users_with_errors"`
	MetricsUpserted int `json:"metrics_upserted"`
	NotFound        int `json:"not_found"`
	APICalls        int `json:"api_calls"`
}

const scopusAuthorMetricsRunFinalizeTimeout = 10 * time.Second

// AuthorMetricsService fetches and stores author-level metrics from the Scopus Author API.
type AuthorMetricsService struct {
	db     *gorm.DB
	client *http.Client
}

// NewAuthorMetricsService constructs an AuthorMetricsService.
func NewAuthorMetricsService(db *gorm.DB, client *http.Client) *AuthorMetricsService {
	if db == nil {
		db = config.DB
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &AuthorMetricsService{db: db, client: client}
}

// RunForAuthor fetches metrics for a single Scopus author ID and upserts a daily snapshot.
// userID may be nil when the author is not linked to a user in the system.
func (s *AuthorMetricsService) RunForAuthor(ctx context.Context, userID *int, scopusAuthorID string) (*models.ScopusAuthorMetric, error) {
	scopusAuthorID = strings.TrimSpace(scopusAuthorID)
	if scopusAuthorID == "" {
		return nil, errors.New("scopus author id is required")
	}

	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return nil, err
	}

	metric, err := s.fetchAuthorMetric(ctx, apiKey, scopusAuthorID, nil, userID)
	if err != nil {
		return nil, err
	}
	if metric == nil {
		return nil, nil
	}

	metric.UserID = userID
	if err := s.upsert(ctx, metric); err != nil {
		return nil, err
	}
	return metric, nil
}

// GetActiveRun returns the currently-running batch run, if any.
func (s *AuthorMetricsService) GetActiveRun(ctx context.Context) (*models.ScopusAuthorMetricsRun, error) {
	if s == nil {
		return nil, errors.New("author metrics service is nil")
	}
	var run models.ScopusAuthorMetricsRun
	err := s.db.WithContext(ctx).
		Where("status = ?", models.ScopusAuthorMetricsRunStatusRunning).
		Order("started_at DESC").
		First(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

// RunForAll fetches metrics for every user that has a non-empty scopus_id and upserts snapshots.
func (s *AuthorMetricsService) RunForAll(ctx context.Context, input *ScopusAuthorMetricsAllInput) (*ScopusAuthorMetricsSummary, error) {
	if input == nil {
		input = &ScopusAuthorMetricsAllInput{}
	}

	releaseLock, err := s.acquireRunLock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := releaseLock(); err != nil {
			log.Printf("failed to release scopus author metrics lock: %v", err)
		}
	}()

	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return nil, err
	}

	trigger := strings.TrimSpace(input.TriggerSource)
	if trigger == "" {
		trigger = "unknown"
	}
	run := &models.ScopusAuthorMetricsRun{
		TriggerSource: trigger,
		Status:        models.ScopusAuthorMetricsRunStatusRunning,
		StartedAt:     time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, err
	}

	summary := &ScopusAuthorMetricsSummary{}
	startedAt := time.Now()
	var runErr error
	defer func() {
		status := models.ScopusAuthorMetricsRunStatusSuccess
		if runErr != nil {
			status = models.ScopusAuthorMetricsRunStatusFailed
		}
		updates := map[string]interface{}{
			"status":            status,
			"finished_at":       time.Now(),
			"duration_seconds":  time.Since(startedAt).Seconds(),
			"users_processed":   summary.UsersProcessed,
			"users_with_errors": summary.UsersWithErrors,
			"metrics_upserted":  summary.MetricsUpserted,
			"not_found":         summary.NotFound,
			"api_calls":         summary.APICalls,
		}
		if runErr != nil {
			msg := runErr.Error()
			if len(msg) > 1000 {
				msg = msg[:997] + "..."
			}
			updates["error_message"] = msg
		}
		finalizeCtx, cancel := context.WithTimeout(context.Background(), scopusAuthorMetricsRunFinalizeTimeout)
		defer cancel()
		if err := s.db.WithContext(finalizeCtx).Model(run).Updates(updates).Error; err != nil {
			log.Printf("failed to finalize scopus author metrics run %d: %v", run.ID, err)
		}
	}()

	type userRow struct {
		UserID   int
		ScopusID string
	}

	query := s.db.WithContext(ctx).Table("users").
		Select("user_id, scopus_id AS scopus_id").
		Where("scopus_id IS NOT NULL AND scopus_id <> ''").
		Where("delete_at IS NULL")

	if len(input.UserIDs) > 0 {
		query = query.Where("user_id IN ?", input.UserIDs)
	}
	if input.Limit > 0 {
		query = query.Limit(input.Limit)
	}

	var users []userRow
	if err := query.Order("user_id ASC").Find(&users).Error; err != nil {
		runErr = err
		return nil, err
	}

	for _, u := range users {
		userID := u.UserID
		summary.APICalls++
		metric, err := s.fetchAuthorMetric(ctx, apiKey, u.ScopusID, &run.ID, &userID)
		if err != nil {
			summary.UsersWithErrors++
			log.Printf("scopus author metrics failed for user %d (scopus %s): %v", u.UserID, u.ScopusID, err)
			continue
		}
		if metric == nil {
			summary.NotFound++
			log.Printf("scopus author metrics: no entry returned for user %d (scopus %s)", u.UserID, u.ScopusID)
			continue
		}

		metric.UserID = &userID
		if err := s.upsert(ctx, metric); err != nil {
			summary.UsersWithErrors++
			log.Printf("scopus author metrics upsert failed for user %d: %v", u.UserID, err)
			continue
		}
		summary.UsersProcessed++
		summary.MetricsUpserted++
	}

	return summary, nil
}

func (s *AuthorMetricsService) fetchAuthorMetric(ctx context.Context, apiKey, scopusAuthorID string, runID *uint64, userID *int) (*models.ScopusAuthorMetric, error) {
	reqURL := authorMetricsBaseURL + scopusAuthorID + "?view=METRICS"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(scopusAPIKeyField, apiKey)

	started := time.Now()
	resp, err := s.client.Do(req)
	if err != nil {
		s.recordRequest(ctx, runID, userID, scopusAuthorID, req.URL.Path, nil, time.Since(started), nil, err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	duration := time.Since(started)
	status := resp.StatusCode
	if err != nil {
		s.recordRequest(ctx, runID, userID, scopusAuthorID, req.URL.Path, &status, duration, nil, err)
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		s.recordRequest(ctx, runID, userID, scopusAuthorID, req.URL.Path, &status, duration, nil, nil)
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		apiErr := fmt.Errorf("author metrics api error: status %d", resp.StatusCode)
		s.recordRequest(ctx, runID, userID, scopusAuthorID, req.URL.Path, &status, duration, nil, apiErr)
		return nil, apiErr
	}

	entry, err := parseAuthorRetrieval(body)
	if err != nil {
		s.recordRequest(ctx, runID, userID, scopusAuthorID, req.URL.Path, &status, duration, nil, err)
		return nil, err
	}
	if entry == nil {
		s.recordRequest(ctx, runID, userID, scopusAuthorID, req.URL.Path, &status, duration, nil, nil)
		return nil, nil
	}

	now := time.Now()
	metric := &models.ScopusAuthorMetric{
		ScopusAuthorID: scopusAuthorID,
		HIndex:         parseIntPointer(entry.HIndex),
		DocumentCount:  parseIntPointer(entry.Coredata.DocumentCount),
		CitedByCount:   parseIntPointer(entry.Coredata.CitedByCount),
		CitationCount:  parseIntPointer(entry.Coredata.CitationCount),
		CoauthorCount:  parseIntPointer(entry.CoauthorCount),
		SnapshotDate:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		RawJSON:        body,
		FetchedAt:      now,
	}
	s.recordRequest(ctx, runID, userID, scopusAuthorID, req.URL.Path, &status, duration, metric.HIndex, nil)
	return metric, nil
}

// recordRequest logs one Author API request. Failures to log are non-fatal.
func (s *AuthorMetricsService) recordRequest(ctx context.Context, runID *uint64, userID *int, scopusAuthorID, endpoint string, status *int, duration time.Duration, hIndex *int, reqErr error) {
	responseMs := int(duration / time.Millisecond)
	entry := &models.ScopusAuthorMetricRequest{
		RunID:          runID,
		UserID:         userID,
		ScopusAuthorID: scopusAuthorID,
		HTTPMethod:     http.MethodGet,
		Endpoint:       endpoint,
		ResponseStatus: status,
		ResponseTimeMs: &responseMs,
		HIndex:         hIndex,
	}
	if reqErr != nil {
		msg := reqErr.Error()
		if len(msg) > 1000 {
			msg = msg[:997] + "..."
		}
		entry.ErrorMessage = &msg
	}
	if err := s.db.WithContext(ctx).Create(entry).Error; err != nil {
		log.Printf("failed to record scopus author metric request (author %s): %v", scopusAuthorID, err)
	}
}

func (s *AuthorMetricsService) upsert(ctx context.Context, metric *models.ScopusAuthorMetric) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "scopus_author_id"}, {Name: "snapshot_date"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id", "h_index", "document_count", "cited_by_count",
			"citation_count", "coauthor_count", "raw_json", "fetched_at",
		}),
	}).Create(metric).Error
}

func (s *AuthorMetricsService) acquireRunLock(ctx context.Context) (func() error, error) {
	lockCtx := persistentContext(ctx)

	var ok int
	if err := s.db.WithContext(lockCtx).Raw("SELECT GET_LOCK(?, 0)", authorMetricsRunLockName).Scan(&ok).Error; err != nil {
		return nil, err
	}
	if ok != 1 {
		return nil, ErrAuthorMetricsAlreadyRunning
	}

	return func() error {
		return releaseNamedLock(lockCtx, s.db, authorMetricsRunLockName)
	}, nil
}

// authorRetrievalEntry mirrors the subset of the Scopus Author Retrieval response we persist.
type authorRetrievalEntry struct {
	Coredata struct {
		DocumentCount string `json:"document-count"`
		CitedByCount  string `json:"cited-by-count"`
		CitationCount string `json:"citation-count"`
	} `json:"coredata"`
	HIndex        string `json:"h-index"`
	CoauthorCount string `json:"coauthor-count"`
}

// parseAuthorRetrieval decodes the author-retrieval-response, tolerating both the
// array shape (the usual response) and a bare object shape.
func parseAuthorRetrieval(body []byte) (*authorRetrievalEntry, error) {
	var envelope struct {
		Response json.RawMessage `json:"author-retrieval-response"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode author retrieval response: %w", err)
	}
	raw := strings.TrimSpace(string(envelope.Response))
	if raw == "" || raw == "null" {
		return nil, nil
	}

	if raw[0] == '[' {
		var entries []authorRetrievalEntry
		if err := json.Unmarshal(envelope.Response, &entries); err != nil {
			return nil, fmt.Errorf("decode author retrieval array: %w", err)
		}
		if len(entries) == 0 {
			return nil, nil
		}
		return &entries[0], nil
	}

	var entry authorRetrievalEntry
	if err := json.Unmarshal(envelope.Response, &entry); err != nil {
		return nil, fmt.Errorf("decode author retrieval object: %w", err)
	}
	return &entry, nil
}
