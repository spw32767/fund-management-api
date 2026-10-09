package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"fund-management-api/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Read only relevant AFIDs, never the whole catalogue. In write transactions share
// locks serialize normalization with catalogue correction/dirty-marker triggers.
func normalizeCoreInsight(tx *gorm.DB, raw []byte, lock bool) (ScopusInsightNormalization, error) {
	probe := NormalizeScopusInsight(raw, nil)
	if lock {
		var guard struct{ ID uint }
		if err := tx.Table("scopus_country_catalogue_guard").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&guard).Error; err != nil {
			return probe, err
		}
	}
	ids := make([]string, 0, len(probe.Affiliations))
	for _, a := range probe.Affiliations {
		ids = append(ids, a.Afid)
	}
	cat := map[string]string{}
	if len(ids) > 0 {
		var rows []models.ScopusAffiliation
		match := "LOWER(TRIM(afid)) IN ?"
		if tx.Dialector.Name() == "mysql" {
			match = "BINARY LOWER(TRIM(afid)) IN ?"
		}
		q := tx.Select("afid,country").Where(match, ids).Order("afid")
		if lock {
			q = q.Clauses(clause.Locking{Strength: "SHARE"})
		}
		if err := q.Find(&rows).Error; err != nil {
			return probe, err
		}
		for _, r := range rows {
			value := ""
			if r.Country != nil {
				value = *r.Country
			}
			cat[r.Afid] = value
		}
	}
	return NormalizeScopusInsight(raw, cat), nil
}

// Caller must hold the document row and run within its ingestion/backfill transaction.
func synchronizeCoreInsight(tx *gorm.DB, documentID uint, raw []byte) error {
	n, err := normalizeCoreInsight(tx, raw, true)
	if err != nil {
		return err
	}
	var old models.ScopusDocumentInsight
	err = tx.Where("document_id=?", documentID).Take(&old).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && (old.Status == "complete" || old.Status == "incomplete") && old.PayloadHash == n.PayloadHash && old.CatalogueHash == n.CatalogueHash && old.NormalizerVersion == ScopusInsightNormalizerVersion {
		return nil
	}
	// Current-payload replacement also removes stale relations after partial payloads.
	// No unmarked historical union and no inheritance of previous domestic certainty.
	if err = tx.Where("document_id=?", documentID).Delete(&models.ScopusDocumentCountry{}).Error; err != nil {
		return err
	}
	if err = tx.Where("document_id=?", documentID).Delete(&models.ScopusDocumentAffiliation{}).Error; err != nil {
		return err
	}
	for i := range n.Affiliations {
		n.Affiliations[i].DocumentID = documentID
	}
	for i := range n.Countries {
		n.Countries[i].DocumentID = documentID
	}
	if len(n.Affiliations) > 0 {
		if err = tx.CreateInBatches(n.Affiliations, 200).Error; err != nil {
			return err
		}
	}
	if len(n.Countries) > 0 {
		if err = tx.CreateInBatches(n.Countries, 200).Error; err != nil {
			return err
		}
	}
	reasons, _ := json.Marshal(n.Reasons)
	diagnostics, _ := json.Marshal(map[string]int{"expected_authors": n.ExpectedAuthors, "provided_authors": n.ProvidedAuthors, "missing_author_afids": n.MissingAuthorAFIDs, "unresolved_countries": n.UnresolvedCountries})
	status := "incomplete"
	if n.CountriesComplete {
		status = "complete"
	}
	meta := models.ScopusDocumentInsight{DocumentID: documentID, International: n.International, AffiliationsComplete: n.AffiliationsComplete, CountriesComplete: n.CountriesComplete, Status: status, ReasonsJSON: string(reasons), DiagnosticsJSON: string(diagnostics), Provenance: "saved_search_payload", NormalizerVersion: ScopusInsightNormalizerVersion, PayloadHash: n.PayloadHash, CatalogueHash: n.CatalogueHash, CheckedAt: time.Now().UTC()}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&meta).Error
}

type ScopusInsightBackfillOptions struct {
	AfterID   uint
	ThroughID uint
	Limit     int
	BatchSize int
	Apply     bool
}
type ScopusInsightBackfillResult struct {
	Mode              string         `json:"mode"`
	NormalizerVersion string         `json:"normalizer_version"`
	Processed         int            `json:"processed"`
	LastID            uint           `json:"last_id"`
	ThroughID         uint           `json:"through_id"`
	Yes               int            `json:"yes"`
	No                int            `json:"no"`
	Unknown           int            `json:"unknown"`
	Complete          int            `json:"complete"`
	Reasons           map[string]int `json:"reasons"`
}

// BackfillCoreScopusInsights never calls Scopus. Dry-run needs only existing core
// tables and is compatible with pre-migration DBs; Apply requires migration 050.
// The caller supplies a READ ONLY transaction for dry-run, or an ordinary DB for apply.
func BackfillCoreScopusInsights(ctx context.Context, db *gorm.DB, opt ScopusInsightBackfillOptions) (ScopusInsightBackfillResult, error) {
	out := ScopusInsightBackfillResult{Mode: "dry_run", NormalizerVersion: ScopusInsightNormalizerVersion, LastID: opt.AfterID, ThroughID: opt.ThroughID, Reasons: map[string]int{}}
	if opt.Apply {
		out.Mode = "apply"
	}
	if opt.Limit < 1 || opt.Limit > 100000 || opt.BatchSize < 1 || opt.BatchSize > 500 || opt.ThroughID <= opt.AfterID {
		return out, fmt.Errorf("invalid bounds: limit 1..100000, batch 1..500, through-id > after-id required")
	}
	for out.Processed < opt.Limit {
		batch := opt.BatchSize
		if remaining := opt.Limit - out.Processed; remaining < batch {
			batch = remaining
		}
		var docs []struct {
			ID      uint
			RawJSON []byte
		}
		if err := db.WithContext(ctx).Table("scopus_documents").Select("id,raw_json").Where("id>? AND id<=?", out.LastID, opt.ThroughID).Order("id").Limit(batch).Find(&docs).Error; err != nil {
			return out, err
		}
		if len(docs) == 0 {
			break
		}
		for _, d := range docs {
			var n ScopusInsightNormalization
			var err error
			if opt.Apply {
				err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					// Lock/re-read instead of persisting a payload loaded before concurrent ingest.
					var current models.ScopusDocument
					if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id,raw_json").Take(&current, d.ID).Error; e != nil {
						return e
					}
					var e error
					n, e = normalizeCoreInsight(tx, current.RawJSON, true)
					if e != nil {
						return e
					}
					return synchronizeCoreInsight(tx, d.ID, current.RawJSON)
				})
			} else {
				n, err = normalizeCoreInsight(db.WithContext(ctx), d.RawJSON, false)
			}
			if err != nil {
				return out, fmt.Errorf("document %d failed: %w", d.ID, err)
			}
			if n.International == nil {
				out.Unknown++
			} else if *n.International {
				out.Yes++
			} else {
				out.No++
			}
			if n.CountriesComplete {
				out.Complete++
			}
			for _, r := range n.Reasons {
				out.Reasons[r]++
			}
			out.Processed++
			out.LastID = d.ID
		}
	}
	return out, nil
}
