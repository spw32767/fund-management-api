package services

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

const (
	thaiJOAuthorSearchURL  = "https://www.tci-thaijo.org/api/v1/authors/search"
	thaiJOAdvancedSearchURL = "https://www.tci-thaijo.org/api/v1/advanced-search"
	thaiJOJournalSearchURL = "https://www.tci-thaijo.org/api/v1/journals/search"

	thaiJOMaxRequestsPerMinute = 10
	thaiJOPacedInterval        = 6500 * time.Millisecond
	thaiJORateLimitLockName    = "thaijo_api_rate_limit_lock"
	thaiJOJobFinalizeTimeout   = 10 * time.Second
)

type ThaiJOIngestResult struct {
	DocumentsFetched int `json:"documents_fetched"`
	DocumentsCreated int `json:"documents_created"`
	DocumentsUpdated int `json:"documents_updated"`
	DocumentsFailed  int `json:"documents_failed"`

	AuthorsCreated int `json:"authors_created"`
	AuthorsUpdated int `json:"authors_updated"`

	JournalsCreated int `json:"journals_created"`
	JournalsUpdated int `json:"journals_updated"`

	DocumentAuthorsInserted int `json:"document_authors_inserted"`
	DocumentAuthorsUpdated  int `json:"document_authors_updated"`
	RejectedHits            int `json:"rejected_hits"`
}

type ThaiJOIngestUserInput struct {
	UserID       uint
	PreferredID  *string
	ForceNameTH  *string
}

type ThaiJOIngestService struct {
	db     *gorm.DB
	client *http.Client
}

func NewThaiJOIngestService(db *gorm.DB, client *http.Client) *ThaiJOIngestService {
	if db == nil {
		db = config.DB
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &ThaiJOIngestService{db: db, client: client}
}

func (s *ThaiJOIngestService) RunForUser(ctx context.Context, input *ThaiJOIngestUserInput) (*ThaiJOIngestResult, error) {
	if input == nil || input.UserID == 0 {
		return nil, errors.New("user id is required")
	}

	lecturerNameTH, preferredAuthorID, err := s.resolveThaiJOIdentity(ctx, input)
	if err != nil {
		return nil, err
	}

	query := lecturerNameTH
	job, err := s.startImportJob(ctx, input.UserID, preferredAuthorID, lecturerNameTH, query)
	if err != nil {
		return nil, err
	}

	result := &ThaiJOIngestResult{}
	var ingestErr error
	authorSelectionReason := ""
	defer func() {
		status := "success"
		var errMsg *string
		if ingestErr != nil {
			status = "failed"
			msg := ingestErr.Error()
			errMsg = &msg
		}

		authorSelectionReasonPtr := optionalString(authorSelectionReason)

		finalizeCtx, cancel := context.WithTimeout(context.Background(), thaiJOJobFinalizeTimeout)
		defer cancel()

		if err := s.db.WithContext(finalizeCtx).Model(job).Updates(map[string]interface{}{
			"status":                  status,
			"error_message":           errMsg,
			"author_selection_reason": authorSelectionReasonPtr,
			"finished_at":             time.Now(),
		}).Error; err != nil {
			log.Printf("failed to finalize thaijo import job %d: %v", job.ID, err)
		}
	}()

	authorResp, err := s.authorSearch(ctx, job.ID, lecturerNameTH)
	if err != nil {
		ingestErr = err
		return nil, err
	}

	selectedAuthor, articleIDSet, authorUsable, authorSelectionConfident, authorSelectionReason := selectThaiJOAuthor(authorResp, lecturerNameTH, preferredAuthorID)
	authorSelectionReason = authorSelectionReason
	if selectedAuthor != nil {
		if err := s.upsertSelectedAuthor(ctx, selectedAuthor, result); err != nil {
			log.Printf("thaijo ingest: failed to upsert selected author for user %d: %v", input.UserID, err)
		}
		if preferredAuthorID == nil {
			if authorSelectionConfident {
				updated, fillErr := s.autoFillUserThaiJOAuthorID(ctx, input.UserID, selectedAuthor.ID)
				if fillErr != nil {
					log.Printf("thaijo ingest: auto-fill user %d thaijo_author_id failed: %v", input.UserID, fillErr)
				} else if updated {
					log.Printf("thaijo ingest: auto-filled user %d thaijo_author_id = %s", input.UserID, strings.TrimSpace(selectedAuthor.ID))
				}
			} else {
				log.Printf("thaijo ingest: skip auto-fill user %d due to ambiguous author selection (%s)", input.UserID, authorSelectionReason)
			}
		}
	}

	advancedResp, err := s.advancedSearch(ctx, job.ID, lecturerNameTH)
	if err != nil {
		ingestErr = err
		return nil, err
	}

	filtered := make([]thaiJOAdvancedSearchHit, 0, len(advancedResp.Hits))
	for _, hit := range advancedResp.Hits {
		result.DocumentsFetched++
		articleID := strings.TrimSpace(hit.ID)
		if articleID == "" {
			result.DocumentsFailed++
			s.recordRejectedHit(ctx, job.ID, input.UserID, nil, "missing_article_id", false)
			result.RejectedHits++
			continue
		}

		if authorUsable {
			if _, ok := articleIDSet[articleID]; !ok {
				s.recordRejectedHit(ctx, job.ID, input.UserID, &articleID, "author_article_id_mismatch", false)
				result.RejectedHits++
				continue
			}
			filtered = append(filtered, hit)
			continue
		}

		if !hit.hasExactThaiAuthor(lecturerNameTH) {
			s.recordRejectedHit(ctx, job.ID, input.UserID, &articleID, "fallback_author_name_mismatch", false)
			result.RejectedHits++
			continue
		}
		filtered = append(filtered, hit)
	}

	journalCache := map[string]*thaiJOJournalSearchHit{}
	for _, hit := range filtered {
		journalName := preferredJournalName(hit)
		if journalName == "" {
			continue
		}
		if _, ok := journalCache[journalName]; ok {
			continue
		}

		journalResp, jErr := s.journalSearch(ctx, job.ID, journalName)
		if jErr != nil {
			log.Printf("thaijo ingest: journal search failed for %q: %v", journalName, jErr)
			journalCache[journalName] = nil
			continue
		}
		best := selectThaiJOJournal(journalResp, journalName)
		journalCache[journalName] = best
		if best != nil {
			if _, created, upsertErr := s.upsertJournal(ctx, best); upsertErr != nil {
				log.Printf("thaijo ingest: upsert journal failed for %q: %v", journalName, upsertErr)
			} else if created {
				result.JournalsCreated++
			} else {
				result.JournalsUpdated++
			}
		}
	}

	for _, hit := range filtered {
		if err := s.persistArticle(ctx, hit, selectedAuthor, result); err != nil {
			result.DocumentsFailed++
			log.Printf("thaijo ingest: failed to persist article %s: %v", strings.TrimSpace(hit.ID), err)
		}
	}

	total := len(filtered)
	if err := s.db.WithContext(ctx).Model(job).Update("total_results", total).Error; err != nil {
		log.Printf("failed to update thaijo import job total %d: %v", job.ID, err)
	}

	return result, nil
}

func (s *ThaiJOIngestService) resolveThaiJOIdentity(ctx context.Context, input *ThaiJOIngestUserInput) (string, *string, error) {
	if input.ForceNameTH != nil && strings.TrimSpace(*input.ForceNameTH) != "" {
		name := strings.TrimSpace(*input.ForceNameTH)
		if input.PreferredID != nil {
			pref := strings.TrimSpace(*input.PreferredID)
			if pref != "" {
				return name, &pref, nil
			}
		}
		return name, nil, nil
	}

	var row struct {
		UserFname       *string
		UserLname       *string
		ThaiJOAuthorID  *string `gorm:"column:thaijo_author_id"`
	}

	if err := s.db.WithContext(ctx).
		Table("users").
		Select("user_fname, user_lname, thaijo_author_id").
		Where("user_id = ?", input.UserID).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, errors.New("user not found")
		}
		return "", nil, err
	}

	name := strings.TrimSpace(strings.TrimSpace(derefString(row.UserFname)) + " " + strings.TrimSpace(derefString(row.UserLname)))
	if name == "" {
		return "", nil, errors.New("thai lecturer name is required")
	}

	if input.PreferredID != nil && strings.TrimSpace(*input.PreferredID) != "" {
		pref := strings.TrimSpace(*input.PreferredID)
		return name, &pref, nil
	}

	if row.ThaiJOAuthorID != nil && strings.TrimSpace(*row.ThaiJOAuthorID) != "" {
		pref := strings.TrimSpace(*row.ThaiJOAuthorID)
		return name, &pref, nil
	}

	return name, nil, nil
}

func (s *ThaiJOIngestService) startImportJob(ctx context.Context, userID uint, preferredAuthorID *string, searchName, query string) (*models.ThaiJOAPIImportJob, error) {
	job := &models.ThaiJOAPIImportJob{
		Service:        "thaijo",
		JobType:        "author_documents",
		UserID:         &userID,
		ThaiJOAuthorID: preferredAuthorID,
		SearchName:     optionalString(searchName),
		QueryString:    query,
		Status:         "running",
		StartedAt:      time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

func (s *ThaiJOIngestService) autoFillUserThaiJOAuthorID(ctx context.Context, userID uint, selectedAuthorID string) (bool, error) {
	selectedAuthorID = strings.TrimSpace(selectedAuthorID)
	if userID == 0 || selectedAuthorID == "" {
		return false, nil
	}

	res := s.db.WithContext(ctx).
		Table("users").
		Where("user_id = ?", userID).
		Where("thaijo_author_id IS NULL OR TRIM(thaijo_author_id) = ''").
		Update("thaijo_author_id", selectedAuthorID)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (s *ThaiJOIngestService) authorSearch(ctx context.Context, jobID uint64, nameTH string) (*thaiJOAuthorSearchResponse, error) {
	reqBody := map[string]interface{}{
		"q":         fmt.Sprintf("\"%s\"", strings.TrimSpace(nameTH)),
		"page":      1,
		"per_page":  20,
		"sort":      "articles_desc",
		"highlight": true,
		"facets":    true,
	}

	respBody, err := s.postRateLimited(ctx, jobID, thaiJOAuthorSearchURL, reqBody)
	if err != nil {
		return nil, err
	}

	var resp thaiJOAuthorSearchResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("decode author search response: %w", err)
	}
	return &resp, nil
}

func (s *ThaiJOIngestService) advancedSearch(ctx context.Context, jobID uint64, nameTH string) (*thaiJOAdvancedSearchResponse, error) {
	reqBody := map[string]interface{}{
		"rows": []map[string]string{{
			"term":  fmt.Sprintf("\"%s\"", strings.TrimSpace(nameTH)),
			"field": "author",
		}},
		"page":              1,
		"per_page":          100,
		"facets":            true,
		"matching_strategy": "all",
	}

	respBody, err := s.postRateLimited(ctx, jobID, thaiJOAdvancedSearchURL, reqBody)
	if err != nil {
		return nil, err
	}

	var resp thaiJOAdvancedSearchResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("decode advanced search response: %w", err)
	}
	return &resp, nil
}

func (s *ThaiJOIngestService) journalSearch(ctx context.Context, jobID uint64, journalName string) (*thaiJOJournalSearchResponse, error) {
	reqBody := map[string]interface{}{
		"q":                 journalName,
		"enabled":           true,
		"page":              1,
		"per_page":          20,
		"sort":              "date_created_desc",
		"matching_strategy": "all",
		"exact":             true,
	}

	respBody, err := s.postRateLimited(ctx, jobID, thaiJOJournalSearchURL, reqBody)
	if err != nil {
		return nil, err
	}

	var resp thaiJOJournalSearchResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("decode journal search response: %w", err)
	}
	return &resp, nil
}

func (s *ThaiJOIngestService) postRateLimited(ctx context.Context, jobID uint64, endpoint string, reqBody map[string]interface{}) ([]byte, error) {
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	requestLogID, err := s.reserveRateLimitSlot(ctx, jobID, endpoint, bodyJSON)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	resp, err := s.client.Do(req)
	responseMs := int(time.Since(started) / time.Millisecond)

	statusCode := 0
	itemsReturned := 0
	var responseBody []byte
	if resp != nil {
		statusCode = resp.StatusCode
		defer resp.Body.Close()
		responseBody, _ = io.ReadAll(resp.Body)
		itemsReturned = detectItemsReturned(responseBody)
	}

	if updateErr := s.finishAPIRequestLog(ctx, requestLogID, statusCode, responseMs, itemsReturned, responseBody); updateErr != nil {
		log.Printf("thaijo ingest: failed to update request log %d: %v", requestLogID, updateErr)
	}

	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		trimmed := string(responseBody)
		if len(trimmed) > 1000 {
			trimmed = trimmed[:1000]
		}
		return nil, fmt.Errorf("thaijo api error: status %d body %s", statusCode, trimmed)
	}

	return responseBody, nil
}

func (s *ThaiJOIngestService) reserveRateLimitSlot(ctx context.Context, jobID uint64, endpoint string, requestBody []byte) (uint64, error) {
	lockCtx := persistentContext(ctx)

	for {
		acquired, release, err := s.acquireThaiJORateLimitLock(lockCtx)
		if err != nil {
			return 0, err
		}
		if !acquired {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		// Pace requests globally to avoid burst traffic.
		var latestCreatedAt time.Time
		if err := s.db.WithContext(lockCtx).
			Table("thaijo_api_requests").
			Select("MAX(created_at)").
			Scan(&latestCreatedAt).Error; err != nil {
			_ = release()
			return 0, err
		}
		if !latestCreatedAt.IsZero() {
			nextAllowed := latestCreatedAt.Add(thaiJOPacedInterval)
			if waitFor := time.Until(nextAllowed); waitFor > 0 {
				_ = release()
				select {
				case <-ctx.Done():
					return 0, ctx.Err()
				case <-time.After(waitFor):
				}
				continue
			}
		}

		var reqCount int64
		windowStart := time.Now().Add(-time.Minute)
		if err := s.db.WithContext(lockCtx).
			Table("thaijo_api_requests").
			Where("created_at >= ?", windowStart).
			Count(&reqCount).Error; err != nil {
			_ = release()
			return 0, err
		}

		if reqCount >= thaiJOMaxRequestsPerMinute {
			var oldest time.Time
			err := s.db.WithContext(lockCtx).
				Table("thaijo_api_requests").
				Select("MIN(created_at)").
				Where("created_at >= ?", windowStart).
				Scan(&oldest).Error
			_ = release()
			if err != nil {
				return 0, err
			}

			waitFor := 2 * time.Second
			if !oldest.IsZero() {
				nextAllowed := oldest.Add(time.Minute).Add(100 * time.Millisecond)
				if duration := time.Until(nextAllowed); duration > 0 {
					waitFor = duration
				}
			}
			if waitFor > 30*time.Second {
				waitFor = 30 * time.Second
			}
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(waitFor):
			}
			continue
		}

		requestBodyStr := string(requestBody)
		method := http.MethodPost
		parsedURL, _ := url.Parse(endpoint)
		queryJSON, _ := json.Marshal(parsedURL.Query())
		headersJSON, _ := json.Marshal(map[string]string{"Accept": "application/json", "Content-Type": "application/json"})
		request := &models.ThaiJOAPIRequest{
			JobID:          &jobID,
			HTTPMethod:     method,
			Endpoint:       parsedURL.Path,
			QueryParams:    stringPtr(string(queryJSON)),
			RequestHeaders: stringPtr(string(headersJSON)),
			RequestBody:    &requestBodyStr,
			CreatedAt:      time.Now(),
		}
		if err := s.db.WithContext(lockCtx).Create(request).Error; err != nil {
			_ = release()
			return 0, err
		}

		_ = release()
		return request.ID, nil
	}
}

func (s *ThaiJOIngestService) finishAPIRequestLog(ctx context.Context, requestID uint64, statusCode, responseMs, itemsReturned int, responseBody []byte) error {
	if requestID == 0 {
		return nil
	}
	responseBodyStr := toJSONStringValue(responseBody)
	updates := map[string]interface{}{
		"response_status":  statusCode,
		"response_time_ms": responseMs,
		"items_returned":   itemsReturned,
		"response_body":    &responseBodyStr,
	}
	return s.db.WithContext(ctx).Model(&models.ThaiJOAPIRequest{}).Where("id = ?", requestID).Updates(updates).Error
}

func (s *ThaiJOIngestService) acquireThaiJORateLimitLock(ctx context.Context) (bool, func() error, error) {
	var ok int
	if err := s.db.WithContext(ctx).Raw("SELECT GET_LOCK(?, 0)", thaiJORateLimitLockName).Scan(&ok).Error; err != nil {
		return false, nil, err
	}
	if ok != 1 {
		return false, nil, nil
	}
	return true, func() error {
		var released int
		if err := s.db.WithContext(ctx).Raw("SELECT RELEASE_LOCK(?)", thaiJORateLimitLockName).Scan(&released).Error; err != nil {
			return err
		}
		if released != 1 {
			return fmt.Errorf("release lock %q returned %d", thaiJORateLimitLockName, released)
		}
		return nil
	}, nil
}

func (s *ThaiJOIngestService) upsertSelectedAuthor(ctx context.Context, hit *thaiJOAuthorSearchHit, result *ThaiJOIngestResult) error {
	if hit == nil || strings.TrimSpace(hit.ID) == "" {
		return nil
	}
	raw, _ := json.Marshal(hit)
	articleIDs, _ := json.Marshal(hit.ArticleIDs)
	affiliations, _ := json.Marshal(hit.Affiliations)
	journalPaths, _ := json.Marshal(hit.JournalPaths)
	journals, _ := json.Marshal(hit.Journals)
	years, _ := json.Marshal(hit.Years)

	model := &models.ThaiJOAuthor{
		ThaiJOAuthorID:   strings.TrimSpace(hit.ID),
		IdentityKey:      optionalString(hit.IdentityKey),
		ORCID:            optionalString(hit.ORCID),
		FullNameEN:       optionalString(hit.FullNames.EN),
		FullNameTH:       optionalString(hit.FullNames.TH),
		GivenNameEN:      optionalString(hit.GivenNames.EN),
		GivenNameTH:      optionalString(hit.GivenNames.TH),
		FamilyNameEN:     optionalString(hit.FamilyNames.EN),
		FamilyNameTH:     optionalString(hit.FamilyNames.TH),
		Country:          optionalString(hit.Country),
		ArticleCount:     intPointer(hit.ArticleCount),
		ArticleIDsJSON:   articleIDs,
		AffiliationsJSON: affiliations,
		JournalPathsJSON: journalPaths,
		JournalsJSON:     journals,
		YearsJSON:        years,
		RawJSON:          raw,
	}

	var existing models.ThaiJOAuthor
	if err := s.db.WithContext(ctx).Where("thaijo_author_id = ?", model.ThaiJOAuthorID).First(&existing).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := s.db.WithContext(ctx).Create(model).Error; err != nil {
			return err
		}
		result.AuthorsCreated++
		return nil
	}
	model.ID = existing.ID
	if err := s.db.WithContext(ctx).Save(model).Error; err != nil {
		return err
	}
	result.AuthorsUpdated++
	return nil
}

func (s *ThaiJOIngestService) upsertJournal(ctx context.Context, hit *thaiJOJournalSearchHit) (*models.ThaiJOJournal, bool, error) {
	if hit == nil {
		return nil, false, nil
	}
	key := strings.TrimSpace(hit.ID)
	if key == "" {
		if hit.JournalID != 0 {
			key = fmt.Sprintf("journal-%d", hit.JournalID)
		} else {
			return nil, false, nil
		}
	}

	raw, _ := json.Marshal(hit)
	model := &models.ThaiJOJournal{
		ThaiJOJournalKey: key,
		JournalID:        intPointer(hit.JournalID),
		Path:             optionalString(hit.Path),
		Acronym:          optionalString(hit.Acronym),
		Category:         optionalString(hit.Category),
		JournalURL:       optionalString(hit.JournalURL),
		NameEN:           optionalString(hit.Names.EN),
		NameTH:           optionalString(hit.Names.TH),
		OnlineISSN:       optionalString(hit.OnlineISSN),
		PrintISSN:        optionalString(hit.PrintISSN),
		Tier:             intPointerNullable(hit.Tier),
		TierPeriod:       optionalString(hit.TierPeriod),
		Enabled:          boolPointer(hit.Enabled),
		RawJSON:          raw,
	}

	var existing models.ThaiJOJournal
	if err := s.db.WithContext(ctx).Where("thaijo_journal_key = ?", key).First(&existing).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
		if err := s.db.WithContext(ctx).Create(model).Error; err != nil {
			return nil, false, err
		}
		return model, true, nil
	}
	model.ID = existing.ID
	if err := s.db.WithContext(ctx).Save(model).Error; err != nil {
		return nil, false, err
	}
	return model, false, nil
}

func (s *ThaiJOIngestService) persistArticle(ctx context.Context, hit thaiJOAdvancedSearchHit, selectedAuthor *thaiJOAuthorSearchHit, result *ThaiJOIngestResult) error {
	articleID := strings.TrimSpace(hit.ID)
	if articleID == "" {
		return errors.New("missing article id")
	}

	raw, _ := json.Marshal(hit)
	languages, _ := json.Marshal(hit.Languages)
	keywords, _ := json.Marshal(hit.Keywords)

	model := &models.ThaiJODocument{
		ThaiJOArticleID: articleID,
		ArticleURL:      optionalString(hit.ArticleURL),
		JournalID:       intPointer(hit.JournalID),
		JournalPath:     optionalString(hit.JournalPath),
		JournalURL:      optionalString(hit.JournalURL),
		TitleEN:         optionalString(hit.Titles.EN),
		TitleTH:         optionalString(hit.Titles.TH),
		Year:            intPointer(hit.Year),
		DatePublished:   parseThaiJODateTime(hit.DatePublished),
		DOI:             optionalString(hit.DOI),
		PDFURL:          optionalString(hit.PDFURL),
		PublicationID:   intPointer(hit.PublicationID),
		SubmissionID:    intPointer(hit.SubmissionID),
		LanguagesJSON:   languages,
		KeywordsJSON:    keywords,
		RawJSON:         raw,
	}

	var doc models.ThaiJODocument
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("thaijo_article_id = ?", articleID).First(&doc).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(model).Error; err != nil {
				return err
			}
			doc = *model
			created = true
		} else {
			model.ID = doc.ID
			if err := tx.Save(model).Error; err != nil {
				return err
			}
			doc = *model
		}

		var authorID *uint64
		if selectedAuthor != nil {
			var persisted models.ThaiJOAuthor
			if err := tx.Select("id").Where("thaijo_author_id = ?", strings.TrimSpace(selectedAuthor.ID)).First(&persisted).Error; err == nil {
				authorID = &persisted.ID
			}
		}

		var existingCount int64
		if err := tx.Model(&models.ThaiJODocumentAuthor{}).Where("document_id = ?", doc.ID).Count(&existingCount).Error; err != nil {
			return err
		}
		if err := tx.Where("document_id = ?", doc.ID).Delete(&models.ThaiJODocumentAuthor{}).Error; err != nil {
			return err
		}

		pairs := buildThaiJOAuthorPairs(hit)
		for idx, pair := range pairs {
			seq := idx + 1
			link := &models.ThaiJODocumentAuthor{
				DocumentID: doc.ID,
				AuthorID:   nil,
				AuthorSeq:  &seq,
				NameEN:     optionalString(pair.EN),
				NameTH:     optionalString(pair.TH),
			}
			if authorID != nil && pair.matchesSelectedAuthor(selectedAuthor) {
				link.AuthorID = authorID
			}
			if err := tx.Create(link).Error; err != nil {
				return err
			}
		}

		if existingCount == 0 {
			result.DocumentAuthorsInserted += len(pairs)
		} else {
			result.DocumentAuthorsUpdated += len(pairs)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if created {
		result.DocumentsCreated++
	} else {
		result.DocumentsUpdated++
	}
	return nil
}

func (s *ThaiJOIngestService) recordRejectedHit(ctx context.Context, jobID uint64, userID uint, articleID *string, reason string, authorMatch bool) {
	job := jobID
	uid := userID
	row := &models.ThaiJORejectedHit{
		JobID:           &job,
		UserID:          &uid,
		ThaiJOArticleID: articleID,
		Reason:          reason,
		AuthorMatch:     authorMatch,
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		log.Printf("failed to write thaijo_rejected_hits: %v", err)
	}
}

type thaiJOAuthorSearchResponse struct {
	Hits []thaiJOAuthorSearchHit `json:"hits"`
}

type thaiJONamePair struct {
	EN string `json:"en_US"`
	TH string `json:"th_TH"`
}

type thaiJOAuthorSearchHit struct {
	ID          string         `json:"id"`
	IdentityKey string         `json:"identity_key"`
	ORCID       string         `json:"orcid"`
	Country     string         `json:"country"`
	ArticleCount int           `json:"article_count"`
	ArticleIDs  []string       `json:"article_ids"`
	Affiliations []string      `json:"affiliations"`
	JournalPaths []string      `json:"journal_paths"`
	Journals    []interface{}  `json:"journals"`
	Years       []interface{}  `json:"years"`
	FullNames   thaiJONamePair `json:"full_names"`
	GivenNames  thaiJONamePair `json:"given_names"`
	FamilyNames thaiJONamePair `json:"family_names"`
}

type thaiJOAdvancedSearchResponse struct {
	Hits []thaiJOAdvancedSearchHit `json:"hits"`
}

type thaiJOAdvancedSearchHit struct {
	ID            string            `json:"id"`
	ArticleURL    string            `json:"article_url"`
	JournalID     int               `json:"journal_id"`
	JournalPath   string            `json:"journal_path"`
	JournalURL    string            `json:"journal_url"`
	JournalName   thaiJONamePair    `json:"journal_name"`
	Titles        thaiJONamePair    `json:"titles"`
	Authors       thaiJOAuthorsPair `json:"authors"`
	Year          int               `json:"year"`
	DatePublished string            `json:"date_published"`
	DOI           string            `json:"doi"`
	PDFURL        string            `json:"pdf_url"`
	PublicationID int               `json:"publication_id"`
	SubmissionID  int               `json:"submission_id"`
	Languages     []string          `json:"languages"`
	Keywords      map[string][]string `json:"keywords"`
}

type thaiJOAuthorsPair struct {
	EN []string `json:"en_US"`
	TH []string `json:"th_TH"`
}

type thaiJOJournalSearchResponse struct {
	Hits []thaiJOJournalSearchHit `json:"hits"`
}

type thaiJOJournalSearchHit struct {
	ID         string         `json:"id"`
	JournalID  int            `json:"journal_id"`
	Path       string         `json:"path"`
	Acronym    string         `json:"acronym"`
	Category   string         `json:"category"`
	JournalURL string         `json:"journal_url"`
	Names      thaiJONamePair `json:"names"`
	OnlineISSN string         `json:"online_issn"`
	PrintISSN  string         `json:"print_issn"`
	Tier       *int           `json:"tier"`
	TierPeriod string         `json:"tier_period"`
	Enabled    bool           `json:"enabled"`
}

func selectThaiJOAuthor(resp *thaiJOAuthorSearchResponse, lecturerNameTH string, preferredAuthorID *string) (*thaiJOAuthorSearchHit, map[string]struct{}, bool, bool, string) {
	if resp == nil || len(resp.Hits) == 0 {
		return nil, nil, false, false, "no_hits"
	}

	preferredProvided := false
	if preferredAuthorID != nil {
		pref := strings.TrimSpace(*preferredAuthorID)
		if pref != "" {
			preferredProvided = true
			for idx := range resp.Hits {
				if strings.EqualFold(strings.TrimSpace(resp.Hits[idx].ID), pref) {
					set := make(map[string]struct{}, len(resp.Hits[idx].ArticleIDs))
					for _, id := range resp.Hits[idx].ArticleIDs {
						id = strings.TrimSpace(id)
						if id != "" {
							set[id] = struct{}{}
						}
					}
					usable := len(set) > 0
					return &resp.Hits[idx], set, usable, true, "preferred_author_id"
				}
			}
		}
	}

	normalizedName := normalizeThaiJOName(lecturerNameTH)
	exactIndexes := make([]int, 0)
	for idx := range resp.Hits {
		hitName := normalizeThaiJOName(resp.Hits[idx].FullNames.TH)
		if hitName == normalizedName {
			exactIndexes = append(exactIndexes, idx)
		}
	}

	chosen := 0
	confident := false
	reason := "fallback_first_hit"
	if len(exactIndexes) == 1 {
		chosen = exactIndexes[0]
		confident = true
		reason = "exact_name_unique"
	} else if len(exactIndexes) > 1 {
		sort.SliceStable(exactIndexes, func(i, j int) bool {
			return resp.Hits[exactIndexes[i]].ArticleCount > resp.Hits[exactIndexes[j]].ArticleCount
		})
		chosen = exactIndexes[0]
		reason = "exact_name_ambiguous"
	} else {
		reason = "no_exact_name"
	}
	if preferredProvided {
		reason = "preferred_author_id_not_found_" + reason
	}

	hit := &resp.Hits[chosen]
	set := make(map[string]struct{}, len(hit.ArticleIDs))
	for _, id := range hit.ArticleIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			set[id] = struct{}{}
		}
	}
	usable := len(set) > 0
	return hit, set, usable, confident, reason
}

func preferredJournalName(hit thaiJOAdvancedSearchHit) string {
	if v := strings.TrimSpace(hit.JournalName.EN); v != "" {
		return v
	}
	return strings.TrimSpace(hit.JournalName.TH)
}

func selectThaiJOJournal(resp *thaiJOJournalSearchResponse, preferredName string) *thaiJOJournalSearchHit {
	if resp == nil || len(resp.Hits) == 0 {
		return nil
	}
	normName := normalizeThaiJOName(preferredName)
	for idx := range resp.Hits {
		hit := resp.Hits[idx]
		if normalizeThaiJOName(hit.Names.EN) == normName || normalizeThaiJOName(hit.Names.TH) == normName {
			return &resp.Hits[idx]
		}
	}
	return &resp.Hits[0]
}

func (h thaiJOAdvancedSearchHit) hasExactThaiAuthor(name string) bool {
	needle := normalizeThaiJOName(name)
	for _, author := range h.Authors.TH {
		if normalizeThaiJOName(author) == needle {
			return true
		}
	}
	return false
}

type thaiJOAuthorPair struct {
	EN string
	TH string
}

func buildThaiJOAuthorPairs(hit thaiJOAdvancedSearchHit) []thaiJOAuthorPair {
	maxLen := len(hit.Authors.EN)
	if len(hit.Authors.TH) > maxLen {
		maxLen = len(hit.Authors.TH)
	}
	if maxLen == 0 {
		return []thaiJOAuthorPair{}
	}

	out := make([]thaiJOAuthorPair, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		pair := thaiJOAuthorPair{}
		if i < len(hit.Authors.EN) {
			pair.EN = strings.TrimSpace(hit.Authors.EN[i])
		}
		if i < len(hit.Authors.TH) {
			pair.TH = strings.TrimSpace(hit.Authors.TH[i])
		}
		if pair.EN == "" && pair.TH == "" {
			continue
		}
		out = append(out, pair)
	}
	return out
}

func (p thaiJOAuthorPair) matchesSelectedAuthor(selected *thaiJOAuthorSearchHit) bool {
	if selected == nil {
		return false
	}
	if p.TH != "" && normalizeThaiJOName(p.TH) == normalizeThaiJOName(selected.FullNames.TH) {
		return true
	}
	if p.EN != "" && normalizeThaiJOName(p.EN) == normalizeThaiJOName(selected.FullNames.EN) {
		return true
	}
	return false
}

func parseThaiJODateTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return &t
		}
	}
	return nil
}

func normalizeThaiJOName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func detectItemsReturned(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	var payload struct {
		Hits json.RawMessage `json:"hits"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}
	if len(payload.Hits) == 0 {
		return 0
	}
	if payload.Hits[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(payload.Hits, &arr); err == nil {
			return len(arr)
		}
	}
	return 0
}

func boolPointer(value bool) *bool { return &value }

func intPointer(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func intPointerNullable(value *int) *int {
	if value == nil {
		return nil
	}
	return value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func thaiJONameFingerprint(value string) string {
	norm := normalizeThaiJOName(value)
	if norm == "" {
		return ""
	}
	h := sha1.Sum([]byte(norm))
	return hex.EncodeToString(h[:])
}

func toJSONStringValue(raw []byte) string {
	if len(raw) == 0 {
		return "null"
	}
	if json.Valid(raw) {
		return string(raw)
	}
	encoded, err := json.Marshal(string(raw))
	if err != nil {
		return "null"
	}
	return string(encoded)
}
