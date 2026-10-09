//go:build insight_integration

package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"fund-management-api/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Local in-memory SQLite proves transaction/replacement behavior. Catalogue
// triggers below are SQLite equivalents; MariaDB migration 050 still needs staging QA.
func coreInsightTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	if err = db.AutoMigrate(&models.ScopusDocument{}, &models.ScopusAuthor{}, &models.ScopusDocumentAuthor{}, &models.ScopusAffiliation{}, &models.ScopusDocumentInsight{}, &models.ScopusDocumentAffiliation{}, &models.ScopusDocumentCountry{}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE scopus_country_catalogue_guard(id INTEGER PRIMARY KEY,revision INTEGER NOT NULL); INSERT INTO scopus_country_catalogue_guard VALUES(1,0)`,
		`CREATE TRIGGER catalogue_insert AFTER INSERT ON scopus_affiliations BEGIN UPDATE scopus_document_insights SET international_collaboration=NULL,countries_complete=0,status='dirty_catalogue' WHERE document_id IN (SELECT document_id FROM scopus_document_affiliations WHERE afid=LOWER(TRIM(NEW.afid))); END`,
		`CREATE TRIGGER catalogue_update AFTER UPDATE ON scopus_affiliations WHEN OLD.country IS NOT NEW.country OR OLD.afid IS NOT NEW.afid BEGIN UPDATE scopus_document_insights SET international_collaboration=NULL,countries_complete=0,status='dirty_catalogue' WHERE document_id IN (SELECT document_id FROM scopus_document_affiliations WHERE afid IN (LOWER(TRIM(OLD.afid)),LOWER(TRIM(NEW.afid)))); END`,
		`CREATE TRIGGER catalogue_delete AFTER DELETE ON scopus_affiliations BEGIN UPDATE scopus_document_insights SET international_collaboration=NULL,countries_complete=0,status='dirty_catalogue' WHERE document_id IN (SELECT document_id FROM scopus_document_affiliations WHERE afid=LOWER(TRIM(OLD.afid))); END`,
		`CREATE TRIGGER payload_update AFTER UPDATE ON scopus_documents WHEN OLD.raw_json IS NOT NEW.raw_json BEGIN UPDATE scopus_document_insights SET international_collaboration=NULL,affiliations_complete=0,countries_complete=0,status='dirty_payload' WHERE document_id=NEW.id; END`,
	} {
		if err = db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}
func saveCoreInsight(t *testing.T, db *gorm.DB, id uint, raw string) {
	t.Helper()
	if err := db.FirstOrCreate(&models.ScopusDocument{ID: id, EID: fmt.Sprintf("fixture-%d", id)}, "id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return synchronizeCoreInsight(tx, id, []byte(raw)) }); err != nil {
		t.Fatal(err)
	}
}
func readCoreInsight(t *testing.T, db *gorm.DB, id uint) models.ScopusDocumentInsight {
	t.Helper()
	var x models.ScopusDocumentInsight
	if err := db.Take(&x, "document_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	return x
}
func TestCoreInsightPersistenceReplacementAndAtomicity(t *testing.T) {
	db := coreInsightTestDB(t)
	saveCoreInsight(t, db, 1, insightDomestic)
	before := readCoreInsight(t, db, 1)
	if before.International == nil || *before.International || !before.CountriesComplete {
		t.Fatal(before)
	}
	saveCoreInsight(t, db, 1, insightDomestic)
	after := readCoreInsight(t, db, 1)
	if !after.CheckedAt.Equal(before.CheckedAt) {
		t.Fatal("idempotent repeat changed check time")
	}
	saveCoreInsight(t, db, 1, insightDual)
	if x := readCoreInsight(t, db, 1); x.International == nil || !*x.International {
		t.Fatal("full replacement did not become foreign")
	}
	var countries []models.ScopusDocumentCountry
	db.Where("document_id=1").Find(&countries)
	if len(countries) != 2 {
		t.Fatal("dual countries were not retained", countries)
	}
	partial := `{"author-count":1,"author":{"authid":"one","afid":"A"}}`
	saveCoreInsight(t, db, 1, partial)
	if x := readCoreInsight(t, db, 1); x.International != nil || x.CountriesComplete {
		t.Fatal("partial inherited old certainty", x)
	}
	db.Where("document_id=1").Find(&countries)
	if len(countries) != 0 {
		t.Fatal("stale foreign memberships retained", countries)
	}
	saveCoreInsight(t, db, 1, insightDomestic)
	if x := readCoreInsight(t, db, 1); x.International == nil || *x.International {
		t.Fatal("full replacement did not restore domestic", x)
	}
	before = readCoreInsight(t, db, 1)
	if err := db.Callback().Create().Before("gorm:create").Register("fail_insight_metadata", func(tx *gorm.DB) {
		if tx.Statement.Table == "scopus_document_insights" {
			tx.AddError(errors.New("injected metadata failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := db.Transaction(func(tx *gorm.DB) error { return synchronizeCoreInsight(tx, 1, []byte(insightDual)) })
	if err == nil {
		t.Fatal("failure was not propagated")
	}
	db.Callback().Create().Remove("fail_insight_metadata")
	after = readCoreInsight(t, db, 1)
	if before.PayloadHash != after.PayloadHash || after.International == nil || *after.International {
		t.Fatal("failed write changed metadata")
	}
	db.Where("document_id=1").Find(&countries)
	if len(countries) != 1 || countries[0].CountryKey != "thailand" {
		t.Fatal("failed transaction changed relations", countries)
	}
}
func TestCoreInsightCatalogueInvalidationAndCorrection(t *testing.T) {
	db := coreInsightTestDB(t)
	saveCoreInsight(t, db, 1, insightDomestic)
	saveCoreInsight(t, db, 2, insightDomestic)
	if err := db.Create(&models.ScopusAffiliation{Afid: "A", Country: stringPtr("Thailand")}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{1, 2} {
		if x := readCoreInsight(t, db, id); x.Status != "dirty_catalogue" || x.International != nil || x.CountriesComplete {
			t.Fatal("catalogue insert missed dependent", id, x)
		}
		saveCoreInsight(t, db, id, insightDomestic)
	}
	if err := db.Model(&models.ScopusAffiliation{}).Where("afid='A'").Update("country", "India").Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{1, 2} {
		if x := readCoreInsight(t, db, id); x.International != nil || x.Status != "dirty_catalogue" {
			t.Fatal("stale domestic after update", id, x)
		}
		saveCoreInsight(t, db, id, insightDomestic)
		if x := readCoreInsight(t, db, id); x.International == nil || !*x.International || x.CountriesComplete {
			t.Fatal("catalogue conflict policy", id, x)
		}
	}
	if err := db.Model(&models.ScopusAffiliation{}).Where("afid='A'").Update("country", nil).Error; err != nil {
		t.Fatal(err)
	}
	saveCoreInsight(t, db, 1, insightDomestic)
	if x := readCoreInsight(t, db, 1); x.International != nil || x.CountriesComplete {
		t.Fatal("cleared catalogue reused payload", x)
	}
	if err := db.Where("afid='A'").Delete(&models.ScopusAffiliation{}).Error; err != nil {
		t.Fatal(err)
	}
	if x := readCoreInsight(t, db, 1); x.Status != "dirty_catalogue" || x.International != nil {
		t.Fatal("delete did not invalidate", x)
	}
	saveCoreInsight(t, db, 1, insightDomestic)
	if x := readCoreInsight(t, db, 1); x.International == nil || *x.International || !x.CountriesComplete {
		t.Fatal("unmapped payload evidence not restored", x)
	}
}
func TestCoreInsightIngestPreservesRolesAndInvalidatesChangedRoster(t *testing.T) {
	db := coreInsightTestDB(t)
	svc := &ScopusIngestService{db: db}
	raw := mutateInsight(insightDomestic, func(p map[string]interface{}) { p["eid"] = "one" })
	doc, _, err := svc.processEntry(context.Background(), []byte(raw), &ScopusIngestResult{})
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&models.ScopusDocument{}).Where("id=?", doc.ID).Update("author_role_status", "complete")
	db.Model(&models.ScopusDocumentAuthor{}).Where("document_id=?", doc.ID).Updates(map[string]interface{}{"is_first_author": true, "is_corresponding_author": false})
	if _, _, err = svc.processEntry(context.Background(), []byte(raw), &ScopusIngestResult{}); err != nil {
		t.Fatal(err)
	}
	var links []models.ScopusDocumentAuthor
	db.Where("document_id=?", doc.ID).Find(&links)
	if len(links) != 1 || links[0].IsFirstAuthor == nil || !*links[0].IsFirstAuthor {
		t.Fatal("repeat discarded XML flags", links)
	}
	noCountry := mutateInsight(raw, func(p map[string]interface{}) { p["affiliation"] = map[string]interface{}{"afid": "A"} })
	if _, _, err = svc.processEntry(context.Background(), []byte(noCountry), &ScopusIngestResult{}); err != nil {
		t.Fatal(err)
	}
	var affiliation models.ScopusAffiliation
	db.Take(&affiliation, "afid='A'")
	if affiliation.Country == nil || *affiliation.Country != "Thailand" {
		t.Fatal("partial Search cleared a reliable catalogue country")
	}
	changed := mutateInsight(raw, func(p map[string]interface{}) {
		p["author-count"] = 2
		p["author"] = []map[string]interface{}{{"authid": "one", "afid": "A"}, {"authid": "two", "afid": "A"}}
	})
	if _, _, err = svc.processEntry(context.Background(), []byte(changed), &ScopusIngestResult{}); err != nil {
		t.Fatal(err)
	}
	db.Where("document_id=?", doc.ID).Find(&links)
	if len(links) != 2 {
		t.Fatal(links)
	}
	for _, l := range links {
		if l.IsFirstAuthor != nil || l.IsCorrespondingAuthor != nil {
			t.Fatal("roster change did not clear flags", links)
		}
	}
	var current models.ScopusDocument
	db.Take(&current, doc.ID)
	if current.AuthorRoleStatus != nil {
		t.Fatal("role status not cleared")
	}
	if x := readCoreInsight(t, db, doc.ID); x.International == nil || *x.International || !x.CountriesComplete {
		t.Fatal(x)
	}
	// A derived-state failure rolls back the entire ingest, not only its country rows.
	db.Callback().Create().Before("gorm:create").Register("fail_insight", func(tx *gorm.DB) {
		if tx.Statement.Table == "scopus_document_insights" {
			tx.AddError(errors.New("failure"))
		}
	})
	newRaw := mutateInsight(insightDual, func(p map[string]interface{}) { p["eid"] = "failed-new" })
	failedCounters := &ScopusIngestResult{DocumentsFetched: 1}
	if _, _, err = svc.processEntry(context.Background(), []byte(newRaw), failedCounters); err == nil {
		t.Fatal("ingest failure accepted")
	}
	if *failedCounters != (ScopusIngestResult{DocumentsFetched: 1}) {
		t.Fatal("aborted ingest inflated committed-work counters", failedCounters)
	}
	var count int64
	db.Model(&models.ScopusDocument{}).Where("eid='failed-new'").Count(&count)
	if count != 0 {
		t.Fatal("ingest document escaped transaction rollback")
	}
}

func TestCoreInsightRawPayloadInvalidationAndRestore(t *testing.T) {
	db := coreInsightTestDB(t)
	saveCoreInsight(t, db, 1, insightDomestic)
	// Use a distinct baseline rather than assuming two fast writes receive
	// different Windows clock ticks. The repair must still replace this timestamp.
	if err := db.Model(&models.ScopusDocumentInsight{}).Where("document_id=1").Update("checked_at", "2000-01-01 00:00:00").Error; err != nil {
		t.Fatal(err)
	}
	before := readCoreInsight(t, db, 1)
	if err := db.Model(&models.ScopusDocument{}).Where("id=1").Update("author_role_status", "complete").Error; err != nil {
		t.Fatal(err)
	}
	if readCoreInsight(t, db, 1).Status != "complete" {
		t.Fatal("unrelated role update dirtied country metadata")
	}
	for _, raw := range []string{insightDual, insightDomestic} {
		if err := db.Model(&models.ScopusDocument{}).Where("id=1").Update("raw_json", []byte(raw)).Error; err != nil {
			t.Fatal(err)
		}
	}
	if m := readCoreInsight(t, db, 1); m.Status != "dirty_payload" || m.International != nil || m.CountriesComplete {
		t.Fatal("raw writer left stale country state", m)
	}
	saveCoreInsight(t, db, 1, insightDomestic)
	if m := readCoreInsight(t, db, 1); m.Status != "complete" || m.International == nil || *m.International || m.CheckedAt.Equal(before.CheckedAt) {
		t.Fatal("dirty status did not force same-hash repair", m)
	}
}
func TestCoreInsightBackfillBoundsResumeAndIdempotence(t *testing.T) {
	db := coreInsightTestDB(t)
	for _, d := range []models.ScopusDocument{{ID: 2, EID: "two", RawJSON: []byte(insightDomestic)}, {ID: 7, EID: "seven", RawJSON: []byte(insightDual)}, {ID: 9, EID: "nine", RawJSON: []byte(`{`)}} {
		if err := db.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
	}
	opt := ScopusInsightBackfillOptions{ThroughID: 9, Limit: 2, BatchSize: 1}
	r, err := BackfillCoreScopusInsights(context.Background(), db, opt)
	if err != nil {
		t.Fatal(err)
	}
	if r.Processed != 2 || r.LastID != 7 || r.Yes != 1 || r.No != 1 {
		t.Fatal(r)
	}
	var count int64
	db.Model(&models.ScopusDocumentInsight{}).Count(&count)
	if count != 0 {
		t.Fatal("dry run wrote metadata")
	}
	opt.Apply = true
	r, err = BackfillCoreScopusInsights(context.Background(), db, opt)
	if err != nil {
		t.Fatal(err)
	}
	before := readCoreInsight(t, db, 2)
	if _, err = BackfillCoreScopusInsights(context.Background(), db, opt); err != nil {
		t.Fatal(err)
	}
	if after := readCoreInsight(t, db, 2); !before.CheckedAt.Equal(after.CheckedAt) {
		t.Fatal("replay not idempotent")
	}
	opt.AfterID = r.LastID
	r, err = BackfillCoreScopusInsights(context.Background(), db, opt)
	if err != nil || r.Processed != 1 || r.LastID != 9 || r.Unknown != 1 {
		t.Fatal(r, err)
	}
	if _, err = BackfillCoreScopusInsights(context.Background(), db, ScopusInsightBackfillOptions{ThroughID: 9}); err == nil {
		t.Fatal("unbounded invocation accepted")
	}
}
