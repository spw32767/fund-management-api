package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fund-management-api/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Commit a search page atomically. Batch shared author/affiliation catalogues
// rather than making four database round trips for every author. Classification
// columns are excluded from both INSERT and UPDATE, including concurrent imports.
func (s *ScopusBenchmarkService) upsertBenchmarkPage(ctx context.Context, payloads []json.RawMessage, scopeID uint64, facultySet map[string]bool, summary *ScopusBenchmarkHarvestSummary) ([]string, error) {
	entries := []*scopusEntry{}
	eids := []string{}
	seen := map[string]bool{}
	for _, raw := range payloads {
		e, err := parseScopusEntry(raw)
		if err != nil {
			return nil, err
		}
		eid := strings.TrimSpace(e.EID)
		if eid == "" || seen[eid] {
			continue
		}
		seen[eid] = true
		entries = append(entries, e)
		eids = append(eids, eid)
	}
	if len(entries) == 0 {
		return eids, nil
	}
	facultyLinks := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		docs := []models.ScopusBenchmarkDocument{}
		authors := []models.ScopusBenchmarkAuthor{}
		affs := []models.ScopusBenchmarkAffiliation{}
		authSeen, affSeen := map[string]bool{}, map[string]bool{}
		for _, e := range entries {
			d := buildBenchmarkDocument(e)
			d.FirstSeenAt = &now
			d.LastSeenAt = &now
			docs = append(docs, *d)
			for _, a := range e.Author {
				id := normalizeScopusID(a.AuthID)
				if id == "" || authSeen[id] {
					continue
				}
				authSeen[id] = true
				authors = append(authors, models.ScopusBenchmarkAuthor{ScopusAuthorID: id, FullName: optionalString(a.AuthName), GivenName: optionalString(a.GivenName), Surname: optionalString(a.Surname), Initials: optionalString(a.Initials), AuthorURL: optionalString(a.AuthorURL)})
			}
			for _, a := range e.Affiliation {
				id := strings.TrimSpace(a.Afid)
				if id == "" || affSeen[id] {
					continue
				}
				affSeen[id] = true
				affs = append(affs, models.ScopusBenchmarkAffiliation{Afid: id, Name: optionalString(a.AffilName), City: optionalString(a.City), Country: optionalString(a.Country), AffiliationURL: optionalString(a.URL)})
			}
		}
		updates := []string{"scopus_id", "scopus_link", "title", "abstract", "aggregation_type", "subtype", "subtype_description", "source_id", "publication_name", "issn", "eissn", "isbn", "volume", "issue", "page_range", "article_number", "cover_date", "cover_display_date", "doi", "pii", "citedby_count", "openaccess", "openaccess_flag", "authkeywords", "fund_acr", "fund_sponsor", "pub_year", "last_seen_at", "updated_at"}
		if err := tx.Omit("Category", "ClassificationConfidence", "ClassificationModel", "ClassificationTaxonomyVersion", "ClassifiedAt", "AffiliationsComplete", "RawJSON").Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns(updates)}).CreateInBatches(&docs, 100).Error; err != nil {
			return err
		}
		// MySQL auto-increment values on mixed insert/update batches must never be
		// used as a catalogue mapping: select actual IDs by their natural keys.
		if err := tx.Where("eid IN ?", eids).Find(&docs).Error; err != nil {
			return err
		}
		docByEID := map[string]models.ScopusBenchmarkDocument{}
		for _, d := range docs {
			docByEID[d.EID] = d
		}
		authByID := map[string]uint{}
		affByID := map[string]uint{}
		if len(authors) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"full_name", "given_name", "surname", "initials", "author_url", "updated_at"})}).CreateInBatches(&authors, 500).Error; err != nil {
				return err
			}
			ids := []string{}
			for id := range authSeen {
				ids = append(ids, id)
			}
			if err := tx.Where("scopus_author_id IN ?", ids).Find(&authors).Error; err != nil {
				return err
			}
			for _, a := range authors {
				authByID[a.ScopusAuthorID] = a.ID
			}
		}
		if len(affs) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"name", "city", "country", "affiliation_url", "updated_at"})}).CreateInBatches(&affs, 500).Error; err != nil {
				return err
			}
			ids := []string{}
			for id := range affSeen {
				ids = append(ids, id)
			}
			if err := tx.Where("afid IN ?", ids).Find(&affs).Error; err != nil {
				return err
			}
			for _, a := range affs {
				affByID[a.Afid] = a.ID
			}
		}
		links := []models.ScopusBenchmarkDocumentAuthor{}
		members := []models.ScopusBenchmarkDocumentScope{}
		docAF := []models.BenchmarkDocumentAffiliation{}
		authorAF := []models.BenchmarkAuthorAffiliation{}
		fullDocs, fullRosters := []uint{}, []uint{}
		completePairs, keepPairs := [][]interface{}{}, [][]interface{}{}
		for _, e := range entries {
			d, ok := docByEID[strings.TrimSpace(e.EID)]
			if !ok {
				return fmt.Errorf("batch document mapping missing")
			}
			members = append(members, models.ScopusBenchmarkDocumentScope{DocumentID: d.ID, ScopeID: scopeID, PubYear: d.PubYear})
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(e.Raw, &raw); err != nil {
				return err
			}
			hasDoc := raw["affiliation"] != nil && string(raw["affiliation"]) != "null"
			if hasDoc {
				fullDocs = append(fullDocs, d.ID)
			}
			rosterComplete := len(e.Author) > 0
			if v, ok := raw["author-count"]; ok {
				n, err := strconv.Atoi(strings.Trim(string(v), "\""))
				if err != nil || n != len(e.Author) {
					rosterComplete = false
				}
			}
			afids := map[string]bool{}
			for _, a := range e.Affiliation {
				if id := strings.TrimSpace(a.Afid); id != "" {
					afids[id] = true
				}
			}
			linkSeen := map[uint]bool{}
			for idx, a := range e.Author {
				id := authByID[normalizeScopusID(a.AuthID)]
				if id == 0 {
					rosterComplete = false
					continue
				}
				if linkSeen[id] {
					rosterComplete = false
					continue
				}
				linkSeen[id] = true
				seq := idx + 1
				isFaculty := facultySet[normalizeScopusID(a.AuthID)]
				if isFaculty {
					facultyLinks++
				}
				links = append(links, models.ScopusBenchmarkDocumentAuthor{DocumentID: d.ID, AuthorID: id, AuthorSeq: &seq, AffiliationID: benchmarkAuthorAffiliationID(a, affByID), IsFaculty: isFaculty, AffiliationsComplete: a.Affiliations != nil})
				pair := []interface{}{d.ID, id}
				keepPairs = append(keepPairs, pair)
				if a.Affiliations != nil {
					completePairs = append(completePairs, pair)
				}
				afsSeen := map[string]bool{}
				for _, af := range a.Affiliations {
					af = strings.TrimSpace(af)
					if af == "" || afsSeen[af] {
						continue
					}
					afsSeen[af] = true
					afids[af] = true
					authorAF = append(authorAF, models.BenchmarkAuthorAffiliation{DocumentID: d.ID, AuthorID: id, Afid: af, Provenance: "benchmark_payload"})
				}
			}
			if rosterComplete {
				fullRosters = append(fullRosters, d.ID)
			}
			for af := range afids {
				docAF = append(docAF, models.BenchmarkDocumentAffiliation{DocumentID: d.ID, Afid: af, Provenance: "benchmark_payload"})
			}
		}
		if len(links) > 0 {
			assign := clause.AssignmentColumns([]string{"author_seq", "affiliation_id", "is_faculty"})
			assign = append(assign, clause.Assignment{Column: clause.Column{Name: "affiliations_complete"}, Value: gorm.Expr("GREATEST(affiliations_complete, VALUES(affiliations_complete))")})
			if err := tx.Clauses(clause.OnConflict{DoUpdates: assign}).CreateInBatches(&links, 500).Error; err != nil {
				return err
			}
		}
		if len(fullDocs) > 0 {
			if err := tx.Where("document_id IN ?", fullDocs).Delete(&models.BenchmarkDocumentAffiliation{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.ScopusBenchmarkDocument{}).Where("id IN ?", fullDocs).Update("affiliations_complete", true).Error; err != nil {
				return err
			}
		}
		if len(completePairs) > 0 {
			if err := tx.Where("(document_id,author_id) IN ?", completePairs).Delete(&models.BenchmarkAuthorAffiliation{}).Error; err != nil {
				return err
			}
		}
		if len(fullRosters) > 0 && len(keepPairs) > 0 {
			if err := tx.Where("document_id IN ? AND (document_id,author_id) NOT IN ?", fullRosters, keepPairs).Delete(&models.BenchmarkAuthorAffiliation{}).Error; err != nil {
				return err
			}
			if err := tx.Where("document_id IN ? AND (document_id,author_id) NOT IN ?", fullRosters, keepPairs).Delete(&models.ScopusBenchmarkDocumentAuthor{}).Error; err != nil {
				return err
			}
		}
		if len(docAF) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"provenance"})}).CreateInBatches(&docAF, 500).Error; err != nil {
				return err
			}
		}
		if len(authorAF) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"provenance"})}).CreateInBatches(&authorAF, 500).Error; err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"pub_year"})}).CreateInBatches(&members, 100).Error
	})
	if err != nil {
		return nil, err
	}
	summary.DocumentsUpserted += len(entries)
	summary.FacultyLinks += facultyLinks
	return eids, nil
}
