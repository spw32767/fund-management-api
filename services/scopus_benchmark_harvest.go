package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"fund-management-api/models"

	"gorm.io/gorm"
)

const (
	benchmarkHarvestPageSize     = 25 // view=COMPLETE is capped at 25 per request
	benchmarkHarvestView         = "COMPLETE"
	benchmarkPageDelay           = 150 * time.Millisecond
	benchmarkRateLimitBackoff    = 8 * time.Second
	benchmarkMaxRateLimitRetries = 3
	benchmarkRunFinalizeTimeout  = 10 * time.Second
)

// ErrScopusBenchmarkHarvestRunning indicates a harvest run is already in progress.
var ErrScopusBenchmarkHarvestRunning = errors.New("scopus benchmark harvest already running")

// errBenchmarkCancelled is returned internally when a run's status was flipped away
// from "running" (e.g. cancelled by an admin) so the harvest loop stops gracefully.
var errBenchmarkCancelled = errors.New("scopus benchmark harvest cancelled")

// ScopusBenchmarkHarvestSummary reports the result of a harvest run.
type ScopusBenchmarkHarvestSummary struct {
	TotalResultsReported int `json:"total_results_reported"`
	PagesFetched         int `json:"pages_fetched"`
	DocumentsUpserted    int `json:"documents_upserted"`
	RequestsMade         int `json:"requests_made"`
	FacultyLinks         int `json:"faculty_links"`
}

// GetActiveRun returns the currently running harvest/count run, if any.
func (s *ScopusBenchmarkService) GetActiveRun(ctx context.Context) (*models.ScopusBenchmarkHarvestRun, error) {
	var run models.ScopusBenchmarkHarvestRun
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

// HarvestScope fetches all Computer Science documents for a scope (optionally
// bounded to [yearFrom, yearTo]) via cursor pagination and upserts them into the
// isolated scopus_benchmark_* tables. faculty authorship is tagged inline.
func (s *ScopusBenchmarkService) HarvestScope(ctx context.Context, scope *models.ScopusBenchmarkScope, yearFrom, yearTo *int) (*ScopusBenchmarkHarvestSummary, error) {
	if scope == nil {
		return nil, errors.New("scope is nil")
	}

	releaseLock, err := s.acquireRunLock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := releaseLock(); err != nil {
			log.Printf("failed to release scopus benchmark lock: %v", err)
		}
	}()

	apiKey, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return nil, err
	}

	facultySet, err := s.loadFacultyScopusIDs(ctx)
	if err != nil {
		return nil, err
	}

	summary := &ScopusBenchmarkHarvestSummary{}
	run := &models.ScopusBenchmarkHarvestRun{
		ScopeID:   &scope.ID,
		RunType:   "harvest",
		YearFrom:  yearFrom,
		YearTo:    yearTo,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, err
	}

	started := time.Now()
	var runErr error
	cancelled := false
	defer func() {
		status := "success"
		var errMsg *string
		if cancelled {
			status = "cancelled"
			msg := "cancelled by user"
			errMsg = &msg
		} else if runErr != nil {
			status = "failed"
			msg := runErr.Error()
			errMsg = &msg
		}
		updates := map[string]interface{}{
			"status":                 status,
			"finished_at":            time.Now(),
			"duration_seconds":       time.Since(started).Seconds(),
			"total_results_reported": summary.TotalResultsReported,
			"pages_fetched":          summary.PagesFetched,
			"documents_upserted":     summary.DocumentsUpserted,
			"requests_made":          summary.RequestsMade,
			"error_message":          errMsg,
		}
		finalizeCtx, cancel := context.WithTimeout(context.Background(), benchmarkRunFinalizeTimeout)
		defer cancel()
		if err := s.db.WithContext(finalizeCtx).Model(run).Updates(updates).Error; err != nil {
			log.Printf("failed to update scopus benchmark run %d: %v", run.ID, err)
		}
	}()

	years := yearRange(yearFrom, yearTo)
	if len(years) == 0 {
		// no year slicing — single query over all years
		if err := s.harvestQuery(ctx, apiKey, scope, nil, facultySet, run, summary); err != nil {
			if errors.Is(err, errBenchmarkCancelled) {
				cancelled = true
				return summary, nil
			}
			runErr = err
			return summary, err
		}
	} else {
		for _, y := range years {
			year := y
			if err := s.harvestQuery(ctx, apiKey, scope, &year, facultySet, run, summary); err != nil {
				if errors.Is(err, errBenchmarkCancelled) {
					cancelled = true
					return summary, nil
				}
				runErr = err
				return summary, err
			}
		}
	}

	return summary, nil
}

// isCancelRequested reports whether the run's status has been moved away from
// "running" (e.g. an admin cancelled it), so the harvest loop can stop.
func (s *ScopusBenchmarkService) isCancelRequested(ctx context.Context, runID uint64) bool {
	var status string
	if err := s.db.WithContext(ctx).Model(&models.ScopusBenchmarkHarvestRun{}).
		Where("id = ?", runID).Pluck("status", &status).Error; err != nil {
		return false
	}
	return status != "" && status != "running"
}

// harvestQuery runs the cursor loop for one scope query (single year or all years).
func (s *ScopusBenchmarkService) harvestQuery(ctx context.Context, apiKey string, scope *models.ScopusBenchmarkScope, year *int, facultySet map[string]bool, run *models.ScopusBenchmarkHarvestRun, summary *ScopusBenchmarkHarvestSummary) error {
	query, err := buildScopeQuery(scope, year)
	if err != nil {
		return err
	}

	cursor := "*"
	for {
		if s.isCancelRequested(ctx, run.ID) {
			return errBenchmarkCancelled
		}
		total, entries, nextCursor, err := s.searchPageWithRetry(ctx, apiKey, query, cursor)
		if err != nil {
			return err
		}
		summary.RequestsMade++
		if total > summary.TotalResultsReported {
			summary.TotalResultsReported = total
		}
		if len(entries) == 0 {
			break
		}
		summary.PagesFetched++

		for _, raw := range entries {
			if err := s.upsertBenchmarkEntry(ctx, raw, scope.ID, facultySet, summary); err != nil {
				log.Printf("scopus benchmark: failed to upsert entry: %v", err)
				continue
			}
		}

		// persist cursor for observability / resume
		s.db.WithContext(ctx).Model(run).Updates(map[string]interface{}{
			"cursor_state":       nextCursor,
			"pages_fetched":      summary.PagesFetched,
			"documents_upserted": summary.DocumentsUpserted,
			"requests_made":      summary.RequestsMade,
		})

		if nextCursor == "" || nextCursor == cursor {
			break
		}
		cursor = nextCursor
		time.Sleep(benchmarkPageDelay)
	}
	return nil
}

// searchPageWithRetry wraps searchPage with 429 backoff.
func (s *ScopusBenchmarkService) searchPageWithRetry(ctx context.Context, apiKey, query, cursor string) (int, []json.RawMessage, string, error) {
	var lastErr error
	for attempt := 0; attempt <= benchmarkMaxRateLimitRetries; attempt++ {
		total, entries, next, err := s.searchPage(ctx, apiKey, query, cursor, benchmarkHarvestPageSize, benchmarkHarvestView)
		if err == nil {
			return total, entries, next, nil
		}
		if !errors.Is(err, errScopusRateLimited) {
			return 0, nil, "", err
		}
		lastErr = err
		log.Printf("scopus benchmark: rate limited, backing off (attempt %d)", attempt+1)
		select {
		case <-ctx.Done():
			return 0, nil, "", ctx.Err()
		case <-time.After(benchmarkRateLimitBackoff):
		}
	}
	return 0, nil, "", lastErr
}

// upsertBenchmarkEntry stores one search entry + its authors + scope membership.
func (s *ScopusBenchmarkService) upsertBenchmarkEntry(ctx context.Context, raw json.RawMessage, scopeID uint64, facultySet map[string]bool, summary *ScopusBenchmarkHarvestSummary) error {
	entry, err := parseScopusEntry(raw)
	if err != nil {
		return err
	}
	if strings.TrimSpace(entry.EID) == "" {
		return errors.New("entry missing eid")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		docModel := buildBenchmarkDocument(entry)
		docModel.RawJSON = cloneJSON(raw)
		docModel.LastSeenAt = &now

		var existing models.ScopusBenchmarkDocument
		found := tx.Where("eid = ?", docModel.EID).Limit(1).Find(&existing)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 0 {
			docModel.FirstSeenAt = &now
			if err := tx.Create(docModel).Error; err != nil {
				return err
			}
			existing = *docModel
		} else {
			docModel.ID = existing.ID
			docModel.FirstSeenAt = existing.FirstSeenAt
			if err := tx.Save(docModel).Error; err != nil {
				return err
			}
			existing = *docModel
		}
		summary.DocumentsUpserted++

		if err := s.upsertBenchmarkAuthors(tx, entry, existing.ID, facultySet, summary); err != nil {
			return err
		}

		// scope membership
		membership := &models.ScopusBenchmarkDocumentScope{
			DocumentID: existing.ID,
			ScopeID:    scopeID,
			PubYear:    existing.PubYear,
		}
		var existingMember models.ScopusBenchmarkDocumentScope
		mfound := tx.Where("document_id = ? AND scope_id = ?", existing.ID, scopeID).Limit(1).Find(&existingMember)
		if mfound.Error != nil {
			return mfound.Error
		}
		if mfound.RowsAffected == 0 {
			if err := tx.Create(membership).Error; err != nil {
				return err
			}
		} else if existingMember.PubYear == nil && existing.PubYear != nil {
			tx.Model(&existingMember).Update("pub_year", existing.PubYear)
		}
		return nil
	})
}

func (s *ScopusBenchmarkService) upsertBenchmarkAuthors(tx *gorm.DB, entry *scopusEntry, documentID uint, facultySet map[string]bool, summary *ScopusBenchmarkHarvestSummary) error {
	for idx, author := range entry.Author {
		authID := normalizeScopusID(author.AuthID)
		if authID == "" {
			continue
		}

		model := &models.ScopusBenchmarkAuthor{
			ScopusAuthorID: authID,
			FullName:       optionalString(author.AuthName),
			GivenName:      optionalString(author.GivenName),
			Surname:        optionalString(author.Surname),
			Initials:       optionalString(author.Initials),
			AuthorURL:      optionalString(author.AuthorURL),
		}

		var existing models.ScopusBenchmarkAuthor
		afound := tx.Where("scopus_author_id = ?", authID).Limit(1).Find(&existing)
		if afound.Error != nil {
			return afound.Error
		}
		if afound.RowsAffected == 0 {
			if err := tx.Create(model).Error; err != nil {
				return err
			}
			existing = *model
		} else {
			model.ID = existing.ID
			if err := tx.Save(model).Error; err != nil {
				return err
			}
			existing = *model
		}

		isFaculty := facultySet[authID]
		if isFaculty {
			summary.FacultyLinks++
		}
		seq := idx + 1
		link := &models.ScopusBenchmarkDocumentAuthor{
			DocumentID: documentID,
			AuthorID:   existing.ID,
			AuthorSeq:  &seq,
			IsFaculty:  isFaculty,
		}
		var existingLink models.ScopusBenchmarkDocumentAuthor
		lfound := tx.Where("document_id = ? AND author_id = ?", documentID, existing.ID).Limit(1).Find(&existingLink)
		if lfound.Error != nil {
			return lfound.Error
		}
		if lfound.RowsAffected == 0 {
			if err := tx.Create(link).Error; err != nil {
				return err
			}
		} else {
			link.ID = existingLink.ID
			if err := tx.Save(link).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// buildBenchmarkDocument maps a parsed Scopus entry to a benchmark document,
// reusing the same field parsing as the faculty ingest.
func buildBenchmarkDocument(entry *scopusEntry) *models.ScopusBenchmarkDocument {
	doc := &models.ScopusBenchmarkDocument{
		EID:                strings.TrimSpace(entry.EID),
		ScopusID:           optionalString(entry.Identifier),
		ScopusLink:         extractScopusLink(entry),
		Title:              optionalString(entry.Title),
		Abstract:           optionalString(entry.Description),
		AggregationType:    optionalString(entry.AggregationType),
		Subtype:            optionalString(entry.Subtype),
		SubtypeDescription: optionalString(entry.SubtypeDesc),
		SourceID:           optionalString(entry.SourceID),
		PublicationName:    optionalString(entry.PublicationName),
		ISSN:               optionalString(entry.ISSN),
		Volume:             optionalString(entry.Volume),
		Issue:              optionalString(entry.Issue),
		PageRange:          optionalString(entry.PageRange),
		ArticleNumber:      optionalString(entry.ArticleNumber),
		CoverDisplayDate:   optionalString(entry.CoverDisplayDate),
		DOI:                optionalString(entry.DOI),
		PII:                optionalString(entry.PII),
		FundAcr:            optionalString(entry.FundAcr),
		FundSponsor:        optionalString(entry.FundSponsor),
	}

	if eissn := extractStringFromRaw(entry.EISSNRaw); eissn != nil {
		doc.EISSN = optionalString(*eissn)
	}
	if isbn := extractStringFromRaw(entry.ISBNRaw); isbn != nil {
		doc.ISBN = optionalString(*isbn)
	}
	if date := parseScopusDate(entry.CoverDate); date != nil {
		doc.CoverDate = date
		y := date.Year()
		doc.PubYear = &y
	}
	if count := parseIntPointer(entry.CitedByCount); count != nil {
		doc.CitedByCount = count
	}
	if oa := parseUint8Pointer(entry.OpenAccess); oa != nil {
		doc.OpenAccess = oa
	}
	if entry.OpenAccessFlag != nil {
		val := uint8(0)
		if *entry.OpenAccessFlag {
			val = 1
		}
		doc.OpenAccessFlag = &val
	}
	doc.AuthKeywords = buildKeywordsJSON(entry.AuthKeywords)
	return doc
}

// loadFacultyScopusIDs returns a set of normalized scopus author ids for users
// registered in our system (used to flag faculty authorship in the harvest).
func (s *ScopusBenchmarkService) loadFacultyScopusIDs(ctx context.Context) (map[string]bool, error) {
	var ids []string
	if err := s.db.WithContext(ctx).Table("users").
		Where("scopus_id IS NOT NULL AND scopus_id <> ''").
		Pluck("scopus_id", &ids).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		if norm := normalizeScopusID(id); norm != "" {
			set[norm] = true
		}
	}
	return set, nil
}

func (s *ScopusBenchmarkService) acquireRunLock(ctx context.Context) (func() error, error) {
	lockCtx := persistentContext(ctx)
	var ok int
	if err := s.db.WithContext(lockCtx).Raw("SELECT GET_LOCK(?, 0)", benchmarkCountLockName).Scan(&ok).Error; err != nil {
		return nil, err
	}
	if ok != 1 {
		return nil, ErrScopusBenchmarkHarvestRunning
	}
	return func() error {
		var released int
		if err := s.db.WithContext(lockCtx).Raw("SELECT RELEASE_LOCK(?)", benchmarkCountLockName).Scan(&released).Error; err != nil {
			return err
		}
		return nil
	}, nil
}

// yearRange returns the descending list of years to harvest, or nil when unbounded.
func yearRange(from, to *int) []int {
	if from == nil && to == nil {
		return nil
	}
	lo, hi := 0, 0
	switch {
	case from != nil && to != nil:
		lo, hi = *from, *to
	case from != nil:
		lo, hi = *from, *from
	default:
		lo, hi = *to, *to
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	years := make([]int, 0, hi-lo+1)
	for y := hi; y >= lo; y-- {
		years = append(years, y)
	}
	return years
}

// normalizeScopusID strips a "SCOPUS_ID:" prefix and trims, so faculty and
// harvested author ids compare consistently.
func normalizeScopusID(v string) string {
	v = strings.TrimSpace(v)
	if idx := strings.LastIndex(v, ":"); idx >= 0 {
		v = strings.TrimSpace(v[idx+1:])
	}
	return v
}
