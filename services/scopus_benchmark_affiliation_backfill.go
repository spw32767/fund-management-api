package services

import (
	"context"
	"fmt"
	"strings"

	"fund-management-api/models"

	"gorm.io/gorm"
)

// BenchmarkAffiliationBackfillSummary reports the one-time raw_json backfill.
type BenchmarkAffiliationBackfillSummary struct {
	DocumentsProcessed        int64 `json:"documents_processed"`
	DocumentsWithAffiliations int64 `json:"documents_with_affiliations"`
	AffiliationsCreated       int64 `json:"affiliations_created"`
	AuthorsWithFirstAfID      int64 `json:"authors_with_first_afid"`
	AuthorLinksMatched        int64 `json:"author_links_matched"`
	AuthorLinksUpdated        int64 `json:"author_links_updated"`
	AuthorsOrLinksMissing     int64 `json:"authors_or_links_missing"`
	LinkedBefore              int64 `json:"linked_before"`
	LinkedAfter               int64 `json:"linked_after"`
}

type benchmarkAffiliationLinkKey struct {
	DocumentID uint
	AuthorID   uint
}

type benchmarkAffiliationLinkUpdate struct {
	DocumentID    uint
	AuthorID      uint
	AffiliationID uint
}

// BackfillBenchmarkAffiliations parses stored benchmark raw_json without calling
// Scopus. It is idempotent and shares the harvest lock so it cannot race a live run.
func (s *ScopusBenchmarkService) BackfillBenchmarkAffiliations(ctx context.Context, batchSize int, progress func(BenchmarkAffiliationBackfillSummary)) (BenchmarkAffiliationBackfillSummary, error) {
	var summary BenchmarkAffiliationBackfillSummary
	if batchSize <= 0 {
		batchSize = 100
	}

	releaseLock, err := s.acquireRunLock(ctx)
	if err != nil {
		return summary, err
	}
	defer releaseLock()

	if err := s.db.WithContext(ctx).Model(&models.ScopusBenchmarkDocumentAuthor{}).
		Where("affiliation_id IS NOT NULL").Count(&summary.LinkedBefore).Error; err != nil {
		return summary, fmt.Errorf("count benchmark affiliation links before backfill: %w", err)
	}

	var affiliations []models.ScopusBenchmarkAffiliation
	if err := s.db.WithContext(ctx).Find(&affiliations).Error; err != nil {
		return summary, fmt.Errorf("load benchmark affiliations: %w", err)
	}
	affiliationIDs := make(map[string]uint, len(affiliations))
	for _, affiliation := range affiliations {
		affiliationIDs[strings.TrimSpace(affiliation.Afid)] = affiliation.ID
	}

	var authors []models.ScopusBenchmarkAuthor
	if err := s.db.WithContext(ctx).Select("id", "scopus_author_id").Find(&authors).Error; err != nil {
		return summary, fmt.Errorf("load benchmark authors: %w", err)
	}
	authorIDs := make(map[string]uint, len(authors))
	for _, author := range authors {
		authorIDs[normalizeScopusID(author.ScopusAuthorID)] = author.ID
	}

	var lastID uint
	for {
		var documents []models.ScopusBenchmarkDocument
		if err := s.db.WithContext(ctx).
			Select("id", "eid", "raw_json").
			Where("id > ? AND raw_json IS NOT NULL AND LENGTH(raw_json) > 0", lastID).
			Order("id ASC").Limit(batchSize).Find(&documents).Error; err != nil {
			return summary, fmt.Errorf("load benchmark raw_json batch: %w", err)
		}
		if len(documents) == 0 {
			break
		}

		parsed := make(map[uint]*scopusEntry, len(documents))
		documentIDs := make([]uint, 0, len(documents))
		for i := range documents {
			document := &documents[i]
			entry, err := parseScopusEntry(document.RawJSON)
			if err != nil {
				return summary, fmt.Errorf("parse benchmark document id=%d eid=%s: %w", document.ID, document.EID, err)
			}
			parsed[document.ID] = entry
			documentIDs = append(documentIDs, document.ID)
			lastID = document.ID
		}

		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for _, document := range documents {
				entry := parsed[document.ID]
				if len(entry.Affiliation) > 0 {
					summary.DocumentsWithAffiliations++
				}
				for _, affiliation := range entry.Affiliation {
					afid := strings.TrimSpace(affiliation.Afid)
					if afid == "" {
						continue
					}
					if _, exists := affiliationIDs[afid]; exists {
						continue
					}
					model := &models.ScopusBenchmarkAffiliation{
						Afid:           afid,
						Name:           optionalString(affiliation.AffilName),
						City:           optionalString(affiliation.City),
						Country:        optionalString(affiliation.Country),
						AffiliationURL: optionalString(affiliation.URL),
					}
					if err := tx.Create(model).Error; err != nil {
						return fmt.Errorf("create benchmark affiliation %s: %w", afid, err)
					}
					affiliationIDs[afid] = model.ID
					summary.AffiliationsCreated++
				}
			}

			var existingLinks []models.ScopusBenchmarkDocumentAuthor
			if err := tx.Select("document_id", "author_id", "affiliation_id").
				Where("document_id IN ?", documentIDs).Find(&existingLinks).Error; err != nil {
				return fmt.Errorf("load benchmark document-author links: %w", err)
			}
			linkAffiliations := make(map[benchmarkAffiliationLinkKey]*uint, len(existingLinks))
			for _, link := range existingLinks {
				linkAffiliations[benchmarkAffiliationLinkKey{DocumentID: link.DocumentID, AuthorID: link.AuthorID}] = link.AffiliationID
			}

			updates := make([]benchmarkAffiliationLinkUpdate, 0)
			for _, document := range documents {
				entry := parsed[document.ID]
				for _, author := range entry.Author {
					firstAfid := strings.TrimSpace(author.Affiliations.First())
					if firstAfid == "" {
						continue
					}
					summary.AuthorsWithFirstAfID++
					authorID, authorExists := authorIDs[normalizeScopusID(author.AuthID)]
					affiliationID, affiliationExists := affiliationIDs[firstAfid]
					if !authorExists || !affiliationExists {
						summary.AuthorsOrLinksMissing++
						continue
					}
					key := benchmarkAffiliationLinkKey{DocumentID: document.ID, AuthorID: authorID}
					currentAffiliationID, linkExists := linkAffiliations[key]
					if !linkExists {
						summary.AuthorsOrLinksMissing++
						continue
					}
					summary.AuthorLinksMatched++
					if currentAffiliationID != nil && *currentAffiliationID == affiliationID {
						continue
					}
					updates = append(updates, benchmarkAffiliationLinkUpdate{
						DocumentID: document.ID, AuthorID: authorID, AffiliationID: affiliationID,
					})
				}
			}
			for _, update := range updates {
				result := tx.Model(&models.ScopusBenchmarkDocumentAuthor{}).
					Where("document_id = ? AND author_id = ?", update.DocumentID, update.AuthorID).
					Update("affiliation_id", update.AffiliationID)
				if result.Error != nil {
					return fmt.Errorf("update benchmark affiliation link document=%d author=%d: %w", update.DocumentID, update.AuthorID, result.Error)
				}
				summary.AuthorLinksUpdated += result.RowsAffected
			}
			return nil
		})
		if err != nil {
			return summary, err
		}

		summary.DocumentsProcessed += int64(len(documents))
		if progress != nil {
			progress(summary)
		}
	}

	if err := s.db.WithContext(ctx).Model(&models.ScopusBenchmarkDocumentAuthor{}).
		Where("affiliation_id IS NOT NULL").Count(&summary.LinkedAfter).Error; err != nil {
		return summary, fmt.Errorf("count benchmark affiliation links after backfill: %w", err)
	}
	return summary, nil
}
