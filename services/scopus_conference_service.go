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
)

const (
	// Scopus Abstract Retrieval API — returns full bibrecord including conference event details.
	conferenceAbstractBaseURL   = "https://api.elsevier.com/content/abstract/scopus_id/"
	scopusConferenceAggType     = "Conference Proceeding"
	scopusConferenceLockName    = "scopus_conference_fetch_job_lock"
	conferenceRunFinalizeTimeout = 10 * time.Second
)

// ErrScopusConferenceFetchAlreadyRunning indicates a manual conference fetch run is already in progress.
var ErrScopusConferenceFetchAlreadyRunning = errors.New("scopus conference fetch already running")

// ScopusConferenceFetchSummary reports the result of a conference-info fetch run.
type ScopusConferenceFetchSummary struct {
	DocumentsScanned int `json:"documents_scanned"`
	DocumentsFetched int `json:"documents_fetched"`
	SkippedExisting  int `json:"skipped_existing"`
	DocumentsFailed  int `json:"documents_failed"`
}

// ScopusConferenceService fetches conference event details from the Scopus
// Abstract Retrieval API and stores them on scopus_documents.
type ScopusConferenceService struct {
	db     *gorm.DB
	client *http.Client
}

// NewScopusConferenceService constructs a ScopusConferenceService.
func NewScopusConferenceService(db *gorm.DB, client *http.Client) *ScopusConferenceService {
	if db == nil {
		db = config.DB
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &ScopusConferenceService{db: db, client: client}
}

// GetActiveRun returns the currently running manual conference fetch run, if any.
func (s *ScopusConferenceService) GetActiveRun(ctx context.Context) (*models.ScopusConferenceFetchRun, error) {
	if s == nil {
		return nil, errors.New("scopus conference service is nil")
	}

	var run models.ScopusConferenceFetchRun
	err := s.db.WithContext(ctx).
		Where("status = ?", "running").
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

// EnsureConferenceInfo fetches and stores conference details for a single document
// if it is a conference proceeding that has not been fetched yet. Best-effort: used
// inline during ingest, so callers should log (not fail) on error.
func (s *ScopusConferenceService) EnsureConferenceInfo(ctx context.Context, doc *models.ScopusDocument) error {
	if doc == nil {
		return nil
	}
	if !isConferenceDocument(doc) {
		return nil
	}
	if doc.ConferenceInfoFetchedAt != nil {
		return nil
	}

	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return err
	}
	return s.fetchAndPersist(ctx, apiKey, doc.ID, scopusNumericID(doc))
}

// BackfillMissing fetches conference details for conference documents that have not
// been fetched yet (conference_info_fetched_at IS NULL).
func (s *ScopusConferenceService) BackfillMissing(ctx context.Context) (*ScopusConferenceFetchSummary, error) {
	return s.run(ctx, "backfill", false)
}

// RefreshExisting re-fetches conference details for all conference documents,
// overwriting any previously stored values.
func (s *ScopusConferenceService) RefreshExisting(ctx context.Context) (*ScopusConferenceFetchSummary, error) {
	return s.run(ctx, "refresh", true)
}

// run performs a manual fetch run over conference documents. When forceRefetch is
// false, documents that already have conference_info_fetched_at are skipped.
func (s *ScopusConferenceService) run(ctx context.Context, runType string, forceRefetch bool) (*ScopusConferenceFetchSummary, error) {
	if s == nil {
		return nil, errors.New("scopus conference service is nil")
	}

	releaseLock, err := s.acquireRunLock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := releaseLock(); err != nil {
			log.Printf("failed to release scopus conference fetch lock after %s: %v", runType, err)
		}
	}()

	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return nil, err
	}

	summary := &ScopusConferenceFetchSummary{}
	run := &models.ScopusConferenceFetchRun{RunType: runType, Status: "running", StartedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, err
	}

	started := time.Now()
	var runErr error
	defer func() {
		status := "success"
		if runErr != nil {
			status = "failed"
		}

		updates := map[string]interface{}{
			"status":            status,
			"finished_at":       time.Now(),
			"duration_seconds":  time.Since(started).Seconds(),
			"documents_scanned": summary.DocumentsScanned,
			"documents_fetched": summary.DocumentsFetched,
			"skipped_existing":  summary.SkippedExisting,
			"documents_failed":  summary.DocumentsFailed,
		}
		if runErr != nil {
			errMsg := runErr.Error()
			updates["error_message"] = &errMsg
		}

		finalizeCtx, cancel := context.WithTimeout(context.Background(), conferenceRunFinalizeTimeout)
		defer cancel()

		if err := s.db.WithContext(finalizeCtx).Model(run).Updates(updates).Error; err != nil {
			log.Printf("failed to update scopus conference fetch run %d: %v", run.ID, err)
		}
	}()

	query := s.db.WithContext(ctx).Model(&models.ScopusDocument{}).
		Where("aggregation_type = ?", scopusConferenceAggType)
	if !forceRefetch {
		query = query.Where("conference_info_fetched_at IS NULL")
	}

	var docs []models.ScopusDocument
	if err := query.Order("id ASC").Find(&docs).Error; err != nil {
		runErr = err
		return nil, err
	}

	for i := range docs {
		doc := &docs[i]
		summary.DocumentsScanned++

		numericID := scopusNumericID(doc)
		if numericID == "" {
			summary.DocumentsFailed++
			log.Printf("scopus conference fetch: doc %d missing scopus id", doc.ID)
			continue
		}

		if err := s.fetchAndPersist(ctx, apiKey, doc.ID, numericID); err != nil {
			summary.DocumentsFailed++
			log.Printf("scopus conference fetch: failed for doc %d (%s): %v", doc.ID, numericID, err)
			continue
		}
		summary.DocumentsFetched++
	}

	return summary, nil
}

// fetchAndPersist calls the Abstract Retrieval API for one document and stores the
// parsed conference fields. On a successful HTTP response it always stamps
// conference_info_fetched_at so future backfills skip the document even when the
// record has no conference event block.
func (s *ScopusConferenceService) fetchAndPersist(ctx context.Context, apiKey string, documentID uint, scopusNumericID string) error {
	if strings.TrimSpace(scopusNumericID) == "" {
		return errors.New("missing scopus numeric id")
	}

	rawConferenceInfo, parsed, err := s.fetchConferenceInfo(ctx, apiKey, scopusNumericID)
	if err != nil {
		return err
	}

	now := time.Now()
	updates := map[string]interface{}{
		"conference_info_fetched_at": now,
	}
	if len(rawConferenceInfo) > 0 {
		updates["conference_info_json"] = string(rawConferenceInfo)
	}
	if parsed != nil {
		updates["conference_name"] = optionalString(parsed.Name)
		updates["conference_venue"] = optionalString(parsed.Venue)
		updates["conference_city"] = optionalString(parsed.City)
		updates["conference_country"] = optionalString(parsed.Country)
		updates["conference_location"] = optionalString(parsed.Location)
	}

	return s.db.WithContext(ctx).Model(&models.ScopusDocument{}).
		Where("id = ?", documentID).
		Updates(updates).Error
}

type conferenceParseResult struct {
	Name     string
	Venue    string
	City     string
	Country  string
	Location string
}

// fetchConferenceInfo performs the Abstract Retrieval API call and extracts the
// conferenceinfo node. Returns the raw conferenceinfo JSON (as stored) and the
// parsed fields. Both may be nil when the document has no conference block.
func (s *ScopusConferenceService) fetchConferenceInfo(ctx context.Context, apiKey, scopusNumericID string) (json.RawMessage, *conferenceParseResult, error) {
	reqURL := conferenceAbstractBaseURL + scopusNumericID + "?view=FULL"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(scopusAPIKeyField, apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("abstract api error: status %d body %s", resp.StatusCode, truncateBody(body))
	}

	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, nil, fmt.Errorf("decode abstract response: %w", err)
	}

	confInfo, ok := digMap(root,
		"abstracts-retrieval-response", "item", "bibrecord", "head", "source", "additional-srcinfo", "conferenceinfo")
	if !ok || confInfo == nil {
		// Not all "Conference Proceeding" documents carry a conference event block.
		return nil, nil, nil
	}

	rawConferenceInfo, err := json.Marshal(confInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal conferenceinfo: %w", err)
	}

	return rawConferenceInfo, parseConferenceInfo(confInfo), nil
}

// parseConferenceInfo extracts the conference fields from a conferenceinfo node.
func parseConferenceInfo(confInfo map[string]any) *conferenceParseResult {
	confevent, ok := childMap(confInfo["confevent"])
	if !ok {
		return &conferenceParseResult{}
	}

	res := &conferenceParseResult{Name: getStr(confevent, "confname")}

	if loc, ok := childMap(confevent["conflocation"]); ok {
		res.Venue = getStr(loc, "venue")
		res.City = getStr(loc, "city")
		res.Country = getStr(loc, "@country")
	}

	parts := make([]string, 0, 3)
	for _, p := range []string{res.Venue, res.City, res.Country} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	res.Location = strings.Join(parts, ", ")

	return res
}

func (s *ScopusConferenceService) acquireRunLock(ctx context.Context) (func() error, error) {
	lockCtx := persistentContext(ctx)

	var ok int
	if err := s.db.WithContext(lockCtx).Raw("SELECT GET_LOCK(?, 0)", scopusConferenceLockName).Scan(&ok).Error; err != nil {
		return nil, err
	}
	if ok != 1 {
		return nil, ErrScopusConferenceFetchAlreadyRunning
	}

	return func() error {
		var released int
		if err := s.db.WithContext(lockCtx).Raw("SELECT RELEASE_LOCK(?)", scopusConferenceLockName).Scan(&released).Error; err != nil {
			return err
		}
		if released != 1 {
			return fmt.Errorf("release lock %q returned %d", scopusConferenceLockName, released)
		}
		return nil
	}, nil
}

// --- helpers ---

func isConferenceDocument(doc *models.ScopusDocument) bool {
	if doc == nil || doc.AggregationType == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(*doc.AggregationType), scopusConferenceAggType)
}

// scopusNumericID derives the numeric Scopus ID used by the Abstract Retrieval API,
// preferring scopus_id ("SCOPUS_ID:105007550068") and falling back to eid ("2-s2.0-105007550068").
func scopusNumericID(doc *models.ScopusDocument) string {
	if doc == nil {
		return ""
	}
	if doc.ScopusID != nil {
		v := strings.TrimSpace(*doc.ScopusID)
		if idx := strings.LastIndex(v, ":"); idx >= 0 {
			v = v[idx+1:]
		}
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	eid := strings.TrimSpace(doc.EID)
	if idx := strings.LastIndex(eid, "-"); idx >= 0 {
		return strings.TrimSpace(eid[idx+1:])
	}
	return eid
}

// digMap walks nested maps/arrays following the given keys. When a level is an
// array it descends into the first element.
func digMap(root map[string]any, keys ...string) (map[string]any, bool) {
	var cur any = root
	for _, k := range keys {
		m, ok := childMap(cur)
		if !ok {
			return nil, false
		}
		cur = m[k]
		if cur == nil {
			return nil, false
		}
	}
	return childMap(cur)
}

// childMap coerces a value into a map, taking the first element when it is an array.
func childMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case []any:
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				return m, true
			}
		}
	}
	return nil, false
}

// getStr reads a string value from a map, supporting the Scopus "{"$":"value"}" wrapper.
func getStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch t := m[key].(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]any:
		if v, ok := t["$"].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func truncateBody(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max])
	}
	return string(b)
}
