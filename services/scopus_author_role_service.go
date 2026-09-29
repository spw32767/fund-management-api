package services

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

const (
	authorRoleComplete         = "complete"
	authorRoleNoCorrespondence = "no_correspondence"
	authorRoleNeedsReview      = "needs_review"
	authorRoleFetchError       = "fetch_error"
	authorRoleRunLockName      = "scopus_author_role_backfill_lock"
	maxAuthorRoleXMLBytes      = 8 << 20
)

// ScopusAuthorRoleSummary counts documents, rather than faculty-author pairs.
type ScopusAuthorRoleSummary struct {
	Eligible         int `json:"eligible"`
	Selected         int `json:"selected"`
	SkippedExisting  int `json:"skipped_existing"`
	Fetched          int `json:"fetched"`
	Complete         int `json:"complete"`
	NoCorrespondence int `json:"no_correspondence"`
	NeedsReview      int `json:"needs_review"`
	Failed           int `json:"failed"`
}

type ScopusAuthorRoleService struct {
	db     *gorm.DB
	client *http.Client
}

var ErrScopusAuthorRoleRunActive = errors.New("scopus author role backfill already running")

func NewScopusAuthorRoleService(db *gorm.DB, client *http.Client) *ScopusAuthorRoleService {
	if db == nil {
		db = config.DB
	}
	if client == nil {
		client = &http.Client{Timeout: 35 * time.Second}
	}
	return &ScopusAuthorRoleService{db: db, client: client}
}

// facultyDocumentQuery uses only the main Scopus tables. A document is selected
// once even if several registered faculty members wrote it. Affiliation and
// employment date are intentionally not filters here.
func facultyDocumentQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&models.ScopusDocument{}).Where(`EXISTS (
		SELECT 1 FROM scopus_document_authors AS da
		JOIN scopus_authors AS a ON a.id = da.author_id
		JOIN users AS u ON TRIM(u.Scopus_id) = a.scopus_author_id
		WHERE da.document_id = scopus_documents.id
		  AND u.Scopus_id IS NOT NULL AND TRIM(u.Scopus_id) <> ''
	)`)
}

// Backfill fetches one XML response per selected document. limit=0 means all
// selected documents. Ordinary runs skip checked documents; reviewOnly selects
// only ambiguous records; refresh re-fetches all eligible documents.
func (s *ScopusAuthorRoleService) Backfill(ctx context.Context, limit int, refresh, reviewOnly bool) (*ScopusAuthorRoleSummary, error) {
	return s.BackfillWithProgress(ctx, limit, refresh, reviewOnly, nil)
}

// BackfillWithProgress publishes counts after selection and after each document.
// The callback must return promptly; an admin job uses it to persist progress.
func (s *ScopusAuthorRoleService) BackfillWithProgress(ctx context.Context, limit int, refresh, reviewOnly bool, progress func(ScopusAuthorRoleSummary)) (*ScopusAuthorRoleSummary, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("scopus author role service has no database")
	}
	if limit < 0 {
		return nil, errors.New("limit must be non-negative")
	}
	if refresh && reviewOnly {
		return nil, errors.New("refresh and review-only cannot be combined")
	}
	lockCtx := persistentContext(ctx)
	sqlDB, err := s.db.DB()
	if err != nil {
		return nil, err
	}
	// MySQL named locks belong to a connection. Keep the same connection until
	// release so a pool rotation cannot silently drop the run guard.
	conn, err := sqlDB.Conn(lockCtx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var lockAcquired int
	if err := conn.QueryRowContext(lockCtx, "SELECT GET_LOCK(?, 0)", authorRoleRunLockName).Scan(&lockAcquired); err != nil {
		return nil, err
	}
	if lockAcquired != 1 {
		return nil, ErrScopusAuthorRoleRunActive
	}
	defer func() {
		var released sql.NullInt64
		if err := conn.QueryRowContext(lockCtx, "SELECT RELEASE_LOCK(?)", authorRoleRunLockName).Scan(&released); err != nil {
			log.Printf("scopus author roles: release lock failed: %v", err)
		}
	}()
	key, err := lookupScopusAPIKey(ctx, s.db)
	if err != nil {
		return nil, err
	}
	summary := &ScopusAuthorRoleSummary{}
	var eligible int64
	if err := facultyDocumentQuery(s.db.WithContext(ctx)).Count(&eligible).Error; err != nil {
		return nil, err
	}
	summary.Eligible = int(eligible)
	query := facultyDocumentQuery(s.db.WithContext(ctx))
	if reviewOnly {
		query = query.Where("author_role_status = ?", authorRoleNeedsReview)
	} else if !refresh {
		query = query.Where("author_role_checked_at IS NULL")
	}
	var pending int64
	if err := query.Count(&pending).Error; err != nil {
		return nil, err
	}
	if !refresh {
		summary.SkippedExisting = int(eligible - pending)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var docs []models.ScopusDocument
	if err := query.Select("id, eid, scopus_id").Order("id ASC").Find(&docs).Error; err != nil {
		return nil, err
	}
	summary.Selected = len(docs)
	notify := func() {
		if progress != nil {
			progress(*summary)
		}
	}
	notify()
	for i := range docs {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		doc := &docs[i]
		numericID := scopusNumericID(doc)
		if numericID == "" {
			summary.Failed++
			_ = s.recordFetchError(ctx, doc.ID)
			log.Printf("scopus author roles: document %d has no numeric Scopus ID", doc.ID)
			notify()
			continue
		}
		body, statusCode, err := s.fetchXML(ctx, key, numericID)
		if err != nil {
			summary.Failed++
			_ = s.recordFetchError(ctx, doc.ID)
			log.Printf("scopus author roles: document %d fetch failed: %v", doc.ID, err)
			notify()
			if statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
				return summary, fmt.Errorf("stop author role backfill after HTTP %d; rerun later", statusCode)
			}
			continue
		}
		summary.Fetched++
		status, err := s.persistXML(ctx, doc.ID, body)
		if err != nil {
			summary.Failed++
			log.Printf("scopus author roles: document %d processing failed: %v", doc.ID, err)
			notify()
			continue
		}
		switch status {
		case authorRoleComplete:
			summary.Complete++
		case authorRoleNoCorrespondence:
			summary.NoCorrespondence++
		case authorRoleNeedsReview:
			summary.NeedsReview++
		}
		notify()
		// A modest pace avoids turning an initial backfill into a burst of API calls.
		if i+1 < len(docs) {
			select {
			case <-ctx.Done():
				return summary, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	return summary, nil
}

func (s *ScopusAuthorRoleService) fetchXML(ctx context.Context, key, numericID string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, conferenceAbstractBaseURL+numericID+"?view=FULL", nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/xml")
	req.Header.Set(scopusAPIKeyField, key)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAuthorRoleXMLBytes+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(body) > maxAuthorRoleXMLBytes {
		return nil, resp.StatusCode, errors.New("XML response exceeds size limit")
	}
	return body, resp.StatusCode, nil
}

func (s *ScopusAuthorRoleService) recordFetchError(ctx context.Context, documentID uint) error {
	return s.db.WithContext(ctx).Model(&models.ScopusDocument{}).
		Where("id = ? AND author_role_checked_at IS NULL", documentID).
		Update("author_role_status", authorRoleFetchError).Error
}

type roleLink struct {
	LinkID    uint   `gorm:"column:link_id"`
	AuthorID  string `gorm:"column:author_id"`
	GivenName string `gorm:"column:given_name"`
	Surname   string `gorm:"column:surname"`
}

func (s *ScopusAuthorRoleService) persistXML(ctx context.Context, documentID uint, body []byte) (string, error) {
	parsed, parseErr := parseAuthorRoleXML(body)
	if parseErr != nil {
		log.Printf("scopus author roles: document %d XML parse needs review: %v", documentID, parseErr)
	}
	var links []roleLink
	if err := s.db.WithContext(ctx).Table("scopus_document_authors AS da").
		Select("da.id AS link_id, a.scopus_author_id AS author_id, COALESCE(a.given_name, '') AS given_name, COALESCE(a.surname, '') AS surname").
		Joins("JOIN scopus_authors AS a ON a.id = da.author_id").
		Where("da.document_id = ?", documentID).Find(&links).Error; err != nil {
		return "", err
	}
	roles, resolved := resolveXMLRoles(parsed, links)
	var staleLinkIDs []uint
	if parseErr == nil && !resolved {
		// Search re-ingest historically upserted links without removing authors
		// that disappeared. Prune only when saved Search and current XML agree
		// on the complete roster; otherwise keep every row for manual review.
		var doc models.ScopusDocument
		if err := s.db.WithContext(ctx).Select("raw_json").First(&doc, documentID).Error; err != nil {
			return "", err
		}
		if searchEntry, err := parseScopusEntry(doc.RawJSON); err == nil {
			if current, stale, ok := verifiedStaleRoleLinks(parsed, searchEntry, links); ok {
				if candidate, candidateOK := resolveXMLRoles(parsed, current); candidateOK {
					var registered int64
					if err := s.db.WithContext(ctx).Table("scopus_document_authors AS da").
						Joins("JOIN scopus_authors AS a ON a.id = da.author_id").
						Joins("JOIN users AS u ON TRIM(u.Scopus_id) = a.scopus_author_id").
						Where("da.id IN ?", stale).Count(&registered).Error; err != nil {
						return "", err
					}
					if registered == 0 {
						roles, resolved, staleLinkIDs = candidate, true, stale
					}
				}
			}
		}
	}
	if parseErr == nil && !resolved {
		log.Printf("scopus author roles: document %d author/correspondence mapping needs review", documentID)
	}
	status := authorRoleNeedsReview
	if parseErr == nil && resolved {
		status = authorRoleComplete
		if len(parsed.Corresponding) == 0 {
			status = authorRoleNoCorrespondence
		}
	}
	now := time.Now()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(staleLinkIDs) > 0 {
			if err := tx.Where("document_id = ? AND id IN ?", documentID, staleLinkIDs).
				Delete(&models.ScopusDocumentAuthor{}).Error; err != nil {
				return err
			}
		}
		initial := interface{}(false)
		if status == authorRoleNeedsReview {
			initial = nil
		}
		if err := tx.Model(&models.ScopusDocumentAuthor{}).Where("document_id = ?", documentID).
			Updates(map[string]interface{}{"is_first_author": initial, "is_corresponding_author": initial}).Error; err != nil {
			return err
		}
		if status != authorRoleNeedsReview {
			for linkID, role := range roles {
				updates := map[string]interface{}{}
				if role.First {
					updates["is_first_author"] = true
				}
				if role.Corresponding {
					updates["is_corresponding_author"] = true
				}
				if len(updates) == 0 {
					continue
				}
				if err := tx.Model(&models.ScopusDocumentAuthor{}).Where("id = ?", linkID).Updates(updates).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&models.ScopusDocument{}).Where("id = ?", documentID).
			Updates(map[string]interface{}{"author_role_status": status, "author_role_checked_at": now}).Error
	})
	if err == nil && len(staleLinkIDs) > 0 {
		log.Printf("scopus author roles: document %d removed %d obsolete author links confirmed by Search and XML", documentID, len(staleLinkIDs))
	}
	return status, err
}

// verifiedStaleRoleLinks is deliberately conservative: it will not delete a
// link unless both the saved Search author list and current XML have identical
// nonempty Scopus Author ID sets and both exclude that local link.
func verifiedStaleRoleLinks(parsed parsedAuthorRoles, search *scopusEntry, links []roleLink) ([]roleLink, []uint, bool) {
	if search == nil || len(parsed.Authors) == 0 {
		return nil, nil, false
	}
	searchIDs := make(map[string]struct{}, len(search.Author))
	for _, author := range search.Author {
		if author.AuthID == "" {
			return nil, nil, false
		}
		searchIDs[author.AuthID] = struct{}{}
	}
	xmlIDs := make(map[string]struct{}, len(parsed.Authors))
	for _, author := range parsed.Authors {
		if author.AuthorID == "" {
			return nil, nil, false
		}
		xmlIDs[author.AuthorID] = struct{}{}
	}
	if len(searchIDs) == 0 || len(searchIDs) != len(xmlIDs) {
		return nil, nil, false
	}
	for id := range searchIDs {
		if _, ok := xmlIDs[id]; !ok {
			return nil, nil, false
		}
	}
	current := make([]roleLink, 0, len(links))
	stale := make([]uint, 0)
	for _, link := range links {
		if _, ok := searchIDs[link.AuthorID]; ok {
			current = append(current, link)
		} else {
			stale = append(stale, link.LinkID)
		}
	}
	return current, stale, len(stale) > 0
}

// A small XML tree keeps matching confined to item/bibrecord/head. Full XML
// contains reference-list authors, which must never be classified as authors
// of the document itself.
type roleXMLNode struct {
	XMLName  xml.Name      `xml:""`
	Attrs    []xml.Attr    `xml:",any,attr"`
	Children []roleXMLNode `xml:",any"`
	Text     string        `xml:",chardata"`
}

func (n roleXMLNode) child(name string) *roleXMLNode {
	for i := range n.Children {
		if n.Children[i].XMLName.Local == name {
			return &n.Children[i]
		}
	}
	return nil
}

func (n roleXMLNode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return strings.TrimSpace(a.Value)
		}
	}
	return ""
}

func (n roleXMLNode) childText(name string) string {
	if child := n.child(name); child != nil {
		return strings.TrimSpace(child.Text)
	}
	return ""
}

type xmlRolePerson struct {
	AuthorID string
	Seq      int
	Given    string
	Surname  string
	Indexed  string
}

type parsedAuthorRoles struct {
	Authors       []xmlRolePerson
	Corresponding []xmlRolePerson
	CreatorID     string
}

func xmlPerson(n roleXMLNode) xmlRolePerson {
	seq, _ := strconv.Atoi(n.attr("seq"))
	return xmlRolePerson{
		AuthorID: n.attr("auid"), Seq: seq,
		Given: n.childText("given-name"), Surname: n.childText("surname"),
		Indexed: n.childText("indexed-name"),
	}
}

func parseAuthorRoleXML(body []byte) (parsedAuthorRoles, error) {
	var root roleXMLNode
	if err := xml.Unmarshal(body, &root); err != nil {
		return parsedAuthorRoles{}, err
	}
	if root.XMLName.Local != "abstracts-retrieval-response" {
		return parsedAuthorRoles{}, errors.New("unexpected Abstract Retrieval root")
	}
	core := root.child("coredata")
	item := root.child("item")
	if item == nil || item.child("bibrecord") == nil || item.child("bibrecord").child("head") == nil {
		return parsedAuthorRoles{}, errors.New("Abstract Retrieval XML has no bibrecord head")
	}
	head := item.child("bibrecord").child("head")
	result := parsedAuthorRoles{}
	if core != nil {
		if creator := core.child("creator"); creator != nil {
			if author := creator.child("author"); author != nil {
				result.CreatorID = author.attr("auid")
			}
		}
	}
	seen := make(map[string]struct{})
	for _, group := range head.Children {
		if group.XMLName.Local != "author-group" {
			continue
		}
		for _, author := range group.Children {
			if author.XMLName.Local != "author" {
				continue
			}
			person := xmlPerson(author)
			identity := person.AuthorID
			if identity == "" {
				identity = fmt.Sprintf("%d:%s:%s", person.Seq, normalizedName(person.Given, person.Surname), normalizeRoleName(person.Indexed))
			}
			if _, ok := seen[identity]; ok {
				continue
			}
			seen[identity] = struct{}{}
			result.Authors = append(result.Authors, person)
		}
	}
	for _, child := range head.Children {
		if child.XMLName.Local != "correspondence" {
			continue
		}
		// Some records contain only an affiliation, while others contain
		// several named people in one correspondence block.
		for _, person := range child.Children {
			if person.XMLName.Local == "person" {
				result.Corresponding = append(result.Corresponding, xmlPerson(person))
			}
		}
	}
	if len(result.Authors) == 0 {
		return result, errors.New("bibrecord head has no authors")
	}
	return result, nil
}

type resolvedRole struct {
	First         bool
	Corresponding bool
}

func resolveXMLRoles(parsed parsedAuthorRoles, links []roleLink) (map[uint]resolvedRole, bool) {
	if len(parsed.Authors) == 0 || len(links) == 0 {
		return nil, false
	}
	byID := make(map[string]roleLink, len(links))
	for _, link := range links {
		byID[link.AuthorID] = link
	}
	roles := make(map[uint]resolvedRole, len(links))
	personToLink := make(map[int]uint, len(parsed.Authors))
	firstIndex := -1
	for i, person := range parsed.Authors {
		var link roleLink
		var ok bool
		if person.AuthorID != "" {
			link, ok = byID[person.AuthorID]
		}
		if !ok {
			// Scopus occasionally reassigns an Author ID between the saved
			// Search result and a later Abstract XML response. Only an exact,
			// unique name within this document may bridge that change.
			link, ok = uniqueLocalNameMatch(person, links)
		}
		if !ok || link.LinkID == 0 {
			return nil, false
		}
		if _, duplicate := roles[link.LinkID]; duplicate {
			return nil, false
		}
		roles[link.LinkID] = resolvedRole{}
		personToLink[i] = link.LinkID
		if person.Seq == 1 {
			if firstIndex >= 0 {
				return nil, false
			}
			firstIndex = i
		}
	}
	if len(roles) != len(links) || firstIndex < 0 {
		return nil, false
	}
	if parsed.CreatorID != "" && parsed.Authors[firstIndex].AuthorID != "" && parsed.CreatorID != parsed.Authors[firstIndex].AuthorID {
		return nil, false
	}
	firstLinkID := personToLink[firstIndex]
	roles[firstLinkID] = resolvedRole{First: true}
	for _, correspondent := range parsed.Corresponding {
		index := -1
		for i, person := range parsed.Authors {
			if correspondent.AuthorID != "" && correspondent.AuthorID == person.AuthorID {
				index = i
				break
			}
			if correspondent.AuthorID == "" && sameRolePerson(correspondent, person) {
				if index >= 0 {
					return nil, false
				}
				index = i
			}
		}
		if index < 0 {
			return nil, false
		}
		linkID := personToLink[index]
		role := roles[linkID]
		role.Corresponding = true
		roles[linkID] = role
	}
	return roles, true
}

func uniqueLocalNameMatch(person xmlRolePerson, links []roleLink) (roleLink, bool) {
	name := normalizedName(person.Given, person.Surname)
	if name == "" {
		return roleLink{}, false
	}
	var found roleLink
	for _, link := range links {
		if name == normalizedName(link.GivenName, link.Surname) {
			if found.LinkID != 0 {
				return roleLink{}, false
			}
			found = link
		}
	}
	return found, found.LinkID != 0
}

func sameRolePerson(a, b xmlRolePerson) bool {
	if normalizedName(a.Given, a.Surname) != "" && normalizedName(a.Given, a.Surname) == normalizedName(b.Given, b.Surname) {
		return true
	}
	return a.Indexed != "" && b.Indexed != "" && normalizeRoleName(a.Indexed) == normalizeRoleName(b.Indexed)
}

func normalizedName(given, surname string) string {
	if strings.TrimSpace(given) == "" || strings.TrimSpace(surname) == "" {
		return ""
	}
	return normalizeRoleName(given) + "|" + normalizeRoleName(surname)
}

func normalizeRoleName(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
