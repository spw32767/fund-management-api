package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

const (
	scopusBaseURL     = "https://api.elsevier.com/content/search/scopus"
	scopusPageSize    = 25
	scopusAPIKeyField = "X-ELS-APIKey"
)

var scopusAPIKeyLegacyFields = []string{"api_key"}

// ScopusIngestResult captures summary data for a single author ingest run.
type ScopusIngestResult struct {
	DocumentsFetched        int `json:"documents_fetched"`
	DocumentsCreated        int `json:"documents_created"`
	DocumentsUpdated        int `json:"documents_updated"`
	AuthorsCreated          int `json:"authors_created"`
	AuthorsUpdated          int `json:"authors_updated"`
	AffiliationsCreated     int `json:"affiliations_created"`
	AffiliationsUpdated     int `json:"affiliations_updated"`
	DocumentAuthorsInserted int `json:"document_authors_inserted"`
	DocumentAuthorsUpdated  int `json:"document_authors_updated"`
	DocumentsFailed         int `json:"documents_failed"`
}

// ScopusIngestService fetches and stores publications from the Scopus API.
type ScopusIngestService struct {
	db      *gorm.DB
	client  *http.Client
	metrics *CiteScoreMetricsService
}

// NewScopusIngestService constructs a ScopusIngestService.
func NewScopusIngestService(db *gorm.DB, client *http.Client) *ScopusIngestService {
	if db == nil {
		db = config.DB
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &ScopusIngestService{
		db:      db,
		client:  client,
		metrics: NewCiteScoreMetricsService(db, client),
	}
}

// RunForAuthor fetches all publications for the provided Scopus author ID and upserts them.
func (s *ScopusIngestService) RunForAuthor(ctx context.Context, scopusAuthorID string) (*ScopusIngestResult, error) {
	scopusAuthorID = strings.TrimSpace(scopusAuthorID)
	if scopusAuthorID == "" {
		return nil, errors.New("scopus author id is required")
	}

	apiKey, err := s.getAPIKey(ctx)
	if err != nil {
		return nil, err
	}

	return s.ingestAuthor(ctx, scopusAuthorID, apiKey)
}

// ingestAuthor performs the ingest using the provided API key.
func (s *ScopusIngestService) ingestAuthor(ctx context.Context, scopusAuthorID, apiKey string) (*ScopusIngestResult, error) {
	result := &ScopusIngestResult{}

	job, err := s.startImportJob(ctx, scopusAuthorID)
	if err != nil {
		return nil, err
	}

	start := 0
	totalResults := -1
	metricsSeen := make(map[string]struct{})
	var ingestErr error

	defer func() {
		status := "success"
		var errMsg *string
		if ingestErr != nil {
			status = "failed"
			msg := ingestErr.Error()
			errMsg = &msg
		}

		updates := map[string]interface{}{
			"status":      status,
			"finished_at": time.Now(),
			"total_results": func() *int {
				if totalResults >= 0 {
					return &totalResults
				}
				return nil
			}(),
			"error_message": errMsg,
		}

		if err := s.db.WithContext(ctx).Model(job).Updates(updates).Error; err != nil {
			log.Printf("failed to update scopus import job %d: %v", job.ID, err)
		}
	}()

	for {
		resp, err := s.fetchPage(ctx, apiKey, scopusAuthorID, start, job.ID)
		if err != nil {
			ingestErr = err
			break
		}

		if totalResults < 0 {
			totalResults = resp.TotalResults
		}

		if len(resp.Entries) == 0 {
			break
		}

		for _, rawEntry := range resp.Entries {
			result.DocumentsFetched++
			doc, created, err := s.processEntry(ctx, rawEntry, result)
			if err != nil {
				result.DocumentsFailed++
				log.Printf("scopus ingest: failed to process entry: %v", err)
				continue
			}
			if created && doc != nil {
				s.enqueueMetricFetch(ctx, doc, metricsSeen)
			}
		}

		start += len(resp.Entries)
		if totalResults >= 0 && start >= totalResults {
			break
		}
	}

	if ingestErr != nil {
		return nil, ingestErr
	}

	return result, nil
}

type scopusResponse struct {
	TotalResults int
	Entries      []json.RawMessage
}

func (s *ScopusIngestService) fetchPage(ctx context.Context, apiKey, scopusAuthorID string, start int, jobID uint64) (*scopusResponse, error) {
	reqURL, err := url.Parse(scopusBaseURL)
	if err != nil {
		return nil, err
	}

	query := reqURL.Query()
	query.Set("query", fmt.Sprintf("AU-ID(%s)", scopusAuthorID))
	query.Set("count", strconv.Itoa(scopusPageSize))
	query.Set("start", strconv.Itoa(start))
	query.Set("view", "COMPLETE")
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-ELS-APIKey", apiKey)

	started := time.Now()
	resp, err := s.client.Do(req)
	duration := time.Since(started)

	var payload scopusResponse
	var requestErr error
	var itemsReturned int
	var statusCode int

	if resp != nil {
		statusCode = resp.StatusCode
	}

	if err == nil && resp != nil {
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			requestErr = fmt.Errorf("scopus api error: status %d body %s", resp.StatusCode, string(body))
		} else {
			var decoded struct {
				SearchResults struct {
					TotalResults string            `json:"opensearch:totalResults"`
					Entries      []json.RawMessage `json:"entry"`
				} `json:"search-results"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
				requestErr = fmt.Errorf("decode scopus response: %w", err)
			} else {
				payload.TotalResults = parseIntSafe(decoded.SearchResults.TotalResults)
				payload.Entries = decoded.SearchResults.Entries
				itemsReturned = len(decoded.SearchResults.Entries)
			}
		}
	}

	s.recordAPIRequest(ctx, jobID, req, statusCode, duration, start, scopusPageSize, itemsReturned)

	if err != nil {
		return nil, err
	}
	if requestErr != nil {
		return nil, requestErr
	}

	return &payload, nil
}

func (s *ScopusIngestService) getAPIKey(ctx context.Context) (string, error) {
	return lookupScopusAPIKey(ctx, s.db)
}

func (s *ScopusIngestService) startImportJob(ctx context.Context, scopusAuthorID string) (*models.ScopusAPIImportJob, error) {
	job := &models.ScopusAPIImportJob{
		Service:        "scopus",
		JobType:        "author_documents",
		ScopusAuthorID: &scopusAuthorID,
		QueryString:    fmt.Sprintf("AU-ID(%s)", scopusAuthorID),
		Status:         "running",
		StartedAt:      time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(job).Error; err != nil {
		return nil, err
	}

	return job, nil
}

func (s *ScopusIngestService) recordAPIRequest(ctx context.Context, jobID uint64, req *http.Request, statusCode int, duration time.Duration, pageStart, pageCount, itemsReturned int) {
	if jobID == 0 || req == nil {
		return
	}

	paramsJSON, _ := json.Marshal(req.URL.Query())
	headersJSON, _ := json.Marshal(req.Header)
	responseMs := int(duration / time.Millisecond)

	request := &models.ScopusAPIRequest{
		JobID:          jobID,
		HTTPMethod:     req.Method,
		Endpoint:       req.URL.Path,
		QueryParams:    stringPtr(string(paramsJSON)),
		RequestHeaders: stringPtr(string(headersJSON)),
		ResponseStatus: intPtr(statusCode),
		ResponseTimeMs: intPtr(responseMs),
		PageStart:      intPtr(pageStart),
		PageCount:      intPtr(pageCount),
		ItemsReturned:  intPtr(itemsReturned),
	}

	if err := s.db.WithContext(ctx).Create(request).Error; err != nil {
		log.Printf("failed to record scopus api request for job %d: %v", jobID, err)
	}
}

func stringPtr(value string) *string { return &value }

func intPtr(value int) *int { return &value }

func (s *ScopusIngestService) processEntry(ctx context.Context, raw json.RawMessage, result *ScopusIngestResult) (*models.ScopusDocument, bool, error) {
	entry, err := parseScopusEntry(raw)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(entry.EID) == "" {
		return nil, false, errors.New("scopus entry missing eid")
	}

	var created bool
	var persisted models.ScopusDocument

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		docModel := buildScopusDocument(entry)
		docModel.RawJSON = cloneJSON(raw)

		var doc models.ScopusDocument
		if err := tx.Where("eid = ?", docModel.EID).First(&doc).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(docModel).Error; err != nil {
				return err
			}
			result.DocumentsCreated++
			doc = *docModel
			created = true
		} else {
			docModel.ID = doc.ID
			if err := tx.Save(docModel).Error; err != nil {
				return err
			}
			result.DocumentsUpdated++
			doc = *docModel
		}

		persisted = doc

		affiliationMap, err := s.upsertAffiliations(tx, entry, result)
		if err != nil {
			return err
		}

		return s.upsertAuthorsAndLinks(tx, entry, doc.ID, affiliationMap, result)
	})

	if err != nil {
		return nil, false, err
	}

	return &persisted, created, nil
}

func (s *ScopusIngestService) enqueueMetricFetch(ctx context.Context, doc *models.ScopusDocument, seen map[string]struct{}) {
	if doc == nil {
		return
	}
	if s.metrics == nil {
		s.metrics = NewCiteScoreMetricsService(s.db, s.client)
	}

	issn := ""
	if doc.ISSN != nil {
		issn = *doc.ISSN
	}
	sourceID := ""
	if doc.SourceID != nil {
		sourceID = *doc.SourceID
	}

	key := strings.TrimSpace(issn) + "|" + strings.TrimSpace(sourceID)
	if key == "|" {
		return
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}

	metricYear := 0
	if doc.CoverDate != nil {
		metricYear = doc.CoverDate.Year()
	}

	if err := s.metrics.EnsureJournalMetrics(ctx, issn, sourceID, metricYear); err != nil {
		log.Printf("scopus ingest: failed to fetch CiteScore metrics for issn %s source %s: %v", issn, sourceID, err)
	}
}

func (s *ScopusIngestService) upsertAffiliations(tx *gorm.DB, entry *scopusEntry, result *ScopusIngestResult) (map[string]uint, error) {
	affMap := make(map[string]uint)
	for _, aff := range entry.Affiliation {
		afid := strings.TrimSpace(aff.Afid)
		if afid == "" {
			continue
		}
		model := &models.ScopusAffiliation{
			Afid:           afid,
			Name:           optionalString(aff.AffilName),
			City:           optionalString(aff.City),
			Country:        optionalString(aff.Country),
			AffiliationURL: optionalString(aff.URL),
		}

		var existing models.ScopusAffiliation
		if err := tx.Where("afid = ?", afid).First(&existing).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			if err := tx.Create(model).Error; err != nil {
				return nil, err
			}
			affMap[afid] = model.ID
			result.AffiliationsCreated++
		} else {
			model.ID = existing.ID
			if err := tx.Save(model).Error; err != nil {
				return nil, err
			}
			affMap[afid] = existing.ID
			result.AffiliationsUpdated++
		}
	}
	return affMap, nil
}

func (s *ScopusIngestService) upsertAuthorsAndLinks(tx *gorm.DB, entry *scopusEntry, documentID uint, affiliationMap map[string]uint, result *ScopusIngestResult) error {
	for idx, author := range entry.Author {
		authID := strings.TrimSpace(author.AuthID)
		if authID == "" {
			continue
		}

		model := &models.ScopusAuthor{
			ScopusAuthorID: authID,
			FullName:       optionalString(author.AuthName),
			GivenName:      optionalString(author.GivenName),
			Surname:        optionalString(author.Surname),
			Initials:       optionalString(author.Initials),
			ORCID:          optionalString(author.ORCID),
			AuthorURL:      optionalString(author.AuthorURL),
		}

		var existing models.ScopusAuthor
		if err := tx.Where("scopus_author_id = ?", authID).First(&existing).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(model).Error; err != nil {
				return err
			}
			existing = *model
			result.AuthorsCreated++
		} else {
			model.ID = existing.ID
			if model.ORCID == nil && existing.ORCID != nil {
				model.ORCID = existing.ORCID
			}
			if err := tx.Save(model).Error; err != nil {
				return err
			}
			existing = *model
			result.AuthorsUpdated++
		}

		docAuthor := &models.ScopusDocumentAuthor{
			DocumentID: documentID,
			AuthorID:   existing.ID,
			AuthorSeq:  idx + 1,
		}

		if firstAfid := author.Affiliations.First(); firstAfid != "" {
			if affID, ok := affiliationMap[firstAfid]; ok {
				docAuthor.AffiliationID = &affID
			}
		}

		var existingLink models.ScopusDocumentAuthor
		if err := tx.Where("document_id = ? AND author_id = ?", documentID, existing.ID).
			First(&existingLink).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(docAuthor).Error; err != nil {
				return err
			}
			result.DocumentAuthorsInserted++
		} else {
			docAuthor.ID = existingLink.ID
			if err := tx.Save(docAuthor).Error; err != nil {
				return err
			}
			result.DocumentAuthorsUpdated++
		}
	}
	return nil
}

type scopusEntry struct {
	Raw              json.RawMessage    `json:"-"`
	EID              string             `json:"eid"`
	Identifier       string             `json:"dc:identifier"`
	PrismURL         string             `json:"prism:url"`
	Title            string             `json:"dc:title"`
	Description      string             `json:"dc:description"`
	AggregationType  string             `json:"prism:aggregationType"`
	Subtype          string             `json:"subtype"`
	SubtypeDesc      string             `json:"subtypeDescription"`
	SourceID         string             `json:"source-id"`
	PublicationName  string             `json:"prism:publicationName"`
	ISSN             string             `json:"prism:issn"`
	EISSNRaw         json.RawMessage    `json:"prism:eIssn"`
	ISBNRaw          json.RawMessage    `json:"prism:isbn"`
	Volume           string             `json:"prism:volume"`
	Issue            string             `json:"prism:issueIdentifier"`
	PageRange        string             `json:"prism:pageRange"`
	ArticleNumber    string             `json:"article-number"`
	CoverDate        string             `json:"prism:coverDate"`
	CoverDisplayDate string             `json:"prism:coverDisplayDate"`
	DOI              string             `json:"prism:doi"`
	PII              string             `json:"pii"`
	CitedByCount     string             `json:"citedby-count"`
	OpenAccess       string             `json:"openaccess"`
	OpenAccessFlag   *bool              `json:"openaccessFlag"`
	AuthKeywords     string             `json:"authkeywords"`
	FundAcr          string             `json:"fund-acr"`
	FundSponsor      string             `json:"fund-sponsor"`
	Links            scopusLinks        `json:"link"`
	Affiliation      scopusAffiliations `json:"affiliation"`
	Author           scopusAuthors      `json:"author"`
}

func parseScopusEntry(raw json.RawMessage) (*scopusEntry, error) {
	var entry scopusEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, fmt.Errorf("parse scopus entry: %w", err)
	}
	entry.Raw = raw
	return &entry, nil
}

type scopusLinks []scopusLink

func (l scopusLinks) FirstByRef(ref string) *string {
	for _, link := range l {
		if strings.EqualFold(strings.TrimSpace(link.Ref), strings.TrimSpace(ref)) {
			href := strings.TrimSpace(link.Href)
			if href != "" {
				return &href
			}
		}
	}
	return nil
}

type scopusLink struct {
	Ref  string `json:"@ref"`
	Href string `json:"@href"`
}

type scopusAffiliations []scopusAffiliation

func (a *scopusAffiliations) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '[' {
		var arr []scopusAffiliation
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*a = arr
		return nil
	}
	var single scopusAffiliation
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*a = []scopusAffiliation{single}
	return nil
}

type scopusAffiliation struct {
	Afid      string `json:"afid"`
	AffilName string `json:"affilname"`
	City      string `json:"affiliation-city"`
	Country   string `json:"affiliation-country"`
	URL       string `json:"affiliation-url"`
}

type scopusAuthors []scopusAuthor

func (a *scopusAuthors) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '[' {
		var arr []scopusAuthor
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*a = arr
		return nil
	}
	var single scopusAuthor
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*a = []scopusAuthor{single}
	return nil
}

type scopusAuthor struct {
	AuthID       string            `json:"authid"`
	AuthName     string            `json:"authname"`
	GivenName    string            `json:"given-name"`
	Surname      string            `json:"surname"`
	Initials     string            `json:"initials"`
	AuthorURL    string            `json:"author-url"`
	ORCID        string            `json:"orcid"`
	Affiliations scopusStringSlice `json:"afid"`
}

func (a scopusAuthors) FirstAuthorID() string {
	if len(a) == 0 {
		return ""
	}
	return a[0].AuthID
}

type scopusStringSlice []string

func (s *scopusStringSlice) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '[' {
		var arr []string
		if err := json.Unmarshal(data, &arr); err != nil {
			// sometimes the array can contain objects with `$` keys
			var rawArr []json.RawMessage
			if err2 := json.Unmarshal(data, &rawArr); err2 != nil {
				return err
			}
			var values []string
			for _, raw := range rawArr {
				if str := extractStringFromRaw(raw); str != nil {
					values = append(values, *str)
				}
			}
			*s = values
			return nil
		}
		*s = arr
		return nil
	}

	if data[0] == '{' {
		if str := extractStringFromRaw(data); str != nil {
			*s = []string{*str}
			return nil
		}
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*s = []string{single}
	return nil
}

func (s scopusStringSlice) First() string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

func buildScopusDocument(entry *scopusEntry) *models.ScopusDocument {
	doc := &models.ScopusDocument{
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

func extractScopusLink(entry *scopusEntry) *string {
	if entry == nil {
		return nil
	}
	if link := entry.Links.FirstByRef("scopus"); link != nil {
		return link
	}
	if link := entry.Links.FirstByRef("scopus-citedby"); link != nil {
		return link
	}
	if link := entry.Links.FirstByRef("self"); link != nil {
		return link
	}
	if trimmed := strings.TrimSpace(entry.PrismURL); trimmed != "" {
		return &trimmed
	}
	return nil
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func parseIntSafe(val string) int {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return parsed
}

func parseIntPointer(val string) *int {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseUint8Pointer(val string) *uint8 {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return nil
	}
	if parsed < 0 {
		return nil
	}
	conv := uint8(parsed)
	return &conv
}

func parseScopusDate(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	layouts := []string{"2006-01-02", "2006-01", "2006"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			switch layout {
			case "2006-01":
				t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
			case "2006":
				t = time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
			default:
				t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			}
			return &t
		}
	}
	return nil
}

func buildKeywordsJSON(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "|")
	var keywords []string
	for _, part := range parts {
		word := strings.TrimSpace(part)
		if word != "" {
			keywords = append(keywords, word)
		}
	}
	if len(keywords) == 0 {
		return nil
	}
	data, err := json.Marshal(keywords)
	if err != nil {
		return nil
	}
	return data
}

func extractStringFromRaw(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			str = strings.TrimSpace(str)
			if str == "" {
				return nil
			}
			return &str
		}
	}
	if raw[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil
		}
		for _, item := range arr {
			if val := extractStringFromRaw(item); val != nil {
				return val
			}
		}
		return nil
	}
	if raw[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil
		}
		if val, ok := obj["$"]; ok {
			return extractStringFromRaw(val)
		}
		if val, ok := obj["value"]; ok {
			return extractStringFromRaw(val)
		}
	}
	return nil
}

func cloneJSON(raw json.RawMessage) []byte {
	if raw == nil {
		return nil
	}
	buf := make([]byte, len(raw))
	copy(buf, raw)
	return buf
}
