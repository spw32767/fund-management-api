package services

import (
	"context"
	"encoding/json"
	"fmt"
	"fund-management-api/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	"strconv"
	"strings"
)

func preserveBenchmarkClassification(dst, old *models.ScopusBenchmarkDocument) {
	dst.Category = old.Category
	dst.ClassificationConfidence = old.ClassificationConfidence
	dst.ClassificationModel = old.ClassificationModel
	dst.ClassificationTaxonomyVersion = old.ClassificationTaxonomyVersion
	dst.ClassifiedAt = old.ClassifiedAt
}

// Only explicit, untruncated metadata can establish completeness. Missing keys
// remain unknown; a legacy first affiliation provides positive evidence only.
func replaceBenchmarkAffiliationMetadata(tx *gorm.DB, documentID uint, entry *scopusEntry, provenance string, reconcile bool) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(entry.Raw, &raw); err != nil {
		return err
	}
	_, hasDoc := raw["affiliation"]
	hasDoc = hasDoc && string(raw["affiliation"]) != "null"
	if hasDoc && reconcile {
		if err := tx.Where("document_id = ?", documentID).Delete(&models.BenchmarkDocumentAffiliation{}).Error; err != nil {
			return err
		}
	}
	docIDs := map[string]bool{}
	for _, a := range entry.Affiliation {
		if id := strings.TrimSpace(a.Afid); id != "" {
			docIDs[id] = true
		}
	}
	var links []struct {
		AuthorID             uint
		ScopusAuthorID       string
		AffiliationsComplete bool
	}
	if err := tx.Table("scopus_benchmark_document_authors l").Select("l.author_id,a.scopus_author_id,l.affiliations_complete").Joins("JOIN scopus_benchmark_authors a ON a.id=l.author_id").Where("l.document_id=?", documentID).Scan(&links).Error; err != nil {
		return err
	}
	byID := map[string]uint{}
	for _, l := range links {
		byID[normalizeScopusID(l.ScopusAuthorID)] = l.AuthorID
	}
	rosterComplete := len(entry.Author) > 0
	if v, ok := raw["author-count"]; ok {
		n, err := strconv.Atoi(strings.Trim(string(v), "\""))
		if err != nil || n != len(entry.Author) {
			rosterComplete = false
		}
	}
	keep := []uint{}
	completeIDs := []uint{}
	authorRows := []models.BenchmarkAuthorAffiliation{}
	for _, a := range entry.Author {
		id := byID[normalizeScopusID(a.AuthID)]
		if id == 0 {
			rosterComplete = false
			continue
		}
		keep = append(keep, id)
		if a.Affiliations != nil {
			completeIDs = append(completeIDs, id)
		}
		for _, afid := range a.Affiliations {
			afid = strings.TrimSpace(afid)
			if afid == "" {
				continue
			}
			docIDs[afid] = true
			authorRows = append(authorRows, models.BenchmarkAuthorAffiliation{DocumentID: documentID, AuthorID: id, Afid: afid, Provenance: provenance})
		}
	}
	if reconcile && len(completeIDs) > 0 {
		if err := tx.Where("document_id=? AND author_id IN ?", documentID, completeIDs).Delete(&models.BenchmarkAuthorAffiliation{}).Error; err != nil {
			return err
		}
	}
	if len(authorRows) > 0 {
		if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"provenance"})}).CreateInBatches(&authorRows, 500).Error; err != nil {
			return err
		}
	}
	if len(completeIDs) > 0 {
		if err := tx.Model(&models.ScopusBenchmarkDocumentAuthor{}).Where("document_id=? AND author_id IN ?", documentID, completeIDs).Update("affiliations_complete", true).Error; err != nil {
			return err
		}
	}
	if reconcile && rosterComplete && len(keep) > 0 {
		if err := tx.Where("document_id=? AND author_id NOT IN ?", documentID, keep).Delete(&models.BenchmarkAuthorAffiliation{}).Error; err != nil {
			return err
		}
		if err := tx.Where("document_id=? AND author_id NOT IN ?", documentID, keep).Delete(&models.ScopusBenchmarkDocumentAuthor{}).Error; err != nil {
			return err
		}
	}
	afids := []string{}
	for afid := range docIDs {
		afids = append(afids, afid)
	}
	sort.Strings(afids)
	docRows := []models.BenchmarkDocumentAffiliation{}
	for _, afid := range afids {
		docRows = append(docRows, models.BenchmarkDocumentAffiliation{DocumentID: documentID, Afid: afid, Provenance: provenance})
	}
	if len(docRows) > 0 {
		if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"provenance"})}).CreateInBatches(&docRows, 500).Error; err != nil {
			return err
		}
	}
	if hasDoc {
		return tx.Model(&models.ScopusBenchmarkDocument{}).Where("id=?", documentID).Update("affiliations_complete", true).Error
	}
	return nil
}

type SummaryAffiliationBackfillResult struct {
	Processed        int `json:"processed"`
	BenchmarkPayload int `json:"benchmark_payload"`
	CorePayload      int `json:"core_payload"`
	LegacyOnly       int `json:"legacy_only"`
	InvalidPayload   int `json:"invalid_payload"`
}

// Shares the harvest lock, commits per document and never calls Scopus. No
// classification, scope membership or core role rows are modified.
func (s *ScopusBenchmarkService) BackfillSummaryAffiliations(ctx context.Context) (out SummaryAffiliationBackfillResult, err error) {
	err = s.db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		local := *s
		local.db = conn
		out, err = local.backfillSummaryAffiliations(ctx)
		return err
	})
	return
}
func (s *ScopusBenchmarkService) backfillSummaryAffiliations(ctx context.Context) (SummaryAffiliationBackfillResult, error) {
	var out SummaryAffiliationBackfillResult
	release, err := s.acquireRunLock(ctx)
	if err != nil {
		return out, err
	}
	defer release()
	var last uint
	for {
		var docs []models.ScopusBenchmarkDocument
		if err := s.db.WithContext(ctx).Select("id,eid,raw_json,affiliations_complete").Where("id>?", last).Order("id").Limit(100).Find(&docs).Error; err != nil {
			return out, err
		}
		if len(docs) == 0 {
			break
		}
		ids := []uint{}
		eids := []string{}
		for _, doc := range docs {
			ids = append(ids, doc.ID)
			eids = append(eids, doc.EID)
		}
		var cores []models.ScopusDocument
		if e := s.db.WithContext(ctx).Select("eid,raw_json").Where("eid IN ? AND raw_json IS NOT NULL", eids).Find(&cores).Error; e != nil {
			return out, e
		}
		coreByEID := map[string]models.ScopusDocument{}
		for _, core := range cores {
			coreByEID[core.EID] = core
		}
		// Seed legacy positives once per batch, rather than making thousands of
		// transactions for documents whose stored JSON has already been cleared.
		if e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.Exec(`INSERT IGNORE INTO scopus_benchmark_author_affiliations(document_id,author_id,afid,provenance) SELECT l.document_id,l.author_id,a.afid,'legacy_first' FROM scopus_benchmark_document_authors l JOIN scopus_benchmark_affiliations a ON a.id=l.affiliation_id WHERE l.document_id IN ? AND l.affiliations_complete=0`, ids).Error; e != nil {
				return e
			}
			return tx.Exec(`INSERT IGNORE INTO scopus_benchmark_document_affiliations(document_id,afid,provenance) SELECT document_id,afid,provenance FROM scopus_benchmark_author_affiliations WHERE document_id IN ?`, ids).Error
		}); e != nil {
			return out, e
		}
		for _, doc := range docs {
			last = doc.ID
			core := coreByEID[doc.EID]
			if len(doc.RawJSON) == 0 && len(core.RawJSON) == 0 {
				out.Processed++
				out.LegacyOnly++
				continue
			}
			err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				used := false
				if len(doc.RawJSON) > 0 {
					entry, e := parseScopusEntry(doc.RawJSON)
					if e == nil && entry.EID == doc.EID {
						if e = replaceBenchmarkAffiliationMetadata(tx, doc.ID, entry, "benchmark_payload", true); e != nil {
							return e
						}
						out.BenchmarkPayload++
						used = true
					} else {
						out.InvalidPayload++
					}
				}
				// Core data supplements gaps, never replaces authoritative benchmark lists.
				if len(core.RawJSON) > 0 {
					entry, e := parseScopusEntry(core.RawJSON)
					if e == nil && entry.EID == doc.EID {
						var incompleteIDs []string
						if e := tx.Table("scopus_benchmark_document_authors l").Select("a.scopus_author_id").Joins("JOIN scopus_benchmark_authors a ON a.id=l.author_id").Where("l.document_id=? AND l.affiliations_complete=0", doc.ID).Pluck("a.scopus_author_id", &incompleteIDs).Error; e != nil {
							return e
						}
						allowed := map[string]bool{}
						for _, id := range incompleteIDs {
							allowed[id] = true
						}
						authors := scopusAuthors{}
						for _, a := range entry.Author {
							if allowed[normalizeScopusID(a.AuthID)] {
								authors = append(authors, a)
							}
						}
						entry.Author = authors
						var complete bool
						if e := tx.Model(&models.ScopusBenchmarkDocument{}).Where("id=?", doc.ID).Pluck("affiliations_complete", &complete).Error; e != nil {
							return e
						}
						if complete {
							entry.Affiliation = nil
							var raw map[string]json.RawMessage
							json.Unmarshal(entry.Raw, &raw)
							delete(raw, "affiliation")
							entry.Raw, _ = json.Marshal(raw)
						}
						if len(authors) > 0 || !complete {
							if e = replaceBenchmarkAffiliationMetadata(tx, doc.ID, entry, "core_payload", false); e != nil {
								return e
							}
							out.CorePayload++
							used = true
						}
					} else {
						out.InvalidPayload++
					}
				}
				if !used {
					out.LegacyOnly++
				}
				return nil
			})
			if err != nil {
				return out, fmt.Errorf("document %s: %w", doc.EID, err)
			}
			out.Processed++
		}
		if out.Processed%500 == 0 {
			fmt.Printf("affiliation backfill processed=%d\n", out.Processed)
		}
	}
	return out, nil
}
