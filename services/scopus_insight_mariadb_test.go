//go:build insight_mariadb

package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fund-management-api/models"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type nativeInsightLog struct {
	logger.Interface
	mu         sync.Mutex
	statements []string
}

func (l *nativeInsightLog) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.mu.Lock()
	l.statements = append(l.statements, sql)
	l.mu.Unlock()
}

// This opt-in harness NEVER reads .env or DB_* variables. It only accepts an empty
// disposable schema on loopback; the caller provisions it outside this test.
func nativeInsightDB(t *testing.T) (*gorm.DB, *nativeInsightLog) {
	t.Helper()
	dsn := os.Getenv("SCOPUS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("NATIVE MARIADB NOT RUN: SCOPUS_MARIADB_TEST_DSN is unset")
	}
	c, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid native test DSN (identity withheld)")
	}
	if c.Net != "tcp" || !(strings.HasPrefix(c.Addr, "127.0.0.1:") || strings.HasPrefix(c.Addr, "localhost:") || strings.HasPrefix(c.Addr, "[::1]:")) || !strings.HasPrefix(c.DBName, "scopus_insights_test_") {
		t.Fatal("test guard requires loopback TCP and a scopus_insights_test_ disposable database")
	}
	c.ParseTime = true
	c.Timeout = 5 * time.Second
	c.ReadTimeout = 15 * time.Second
	c.WriteTimeout = 15 * time.Second
	l := &nativeInsightLog{Interface: logger.Default.LogMode(logger.Silent)}
	db, err := gorm.Open(mysql.Open(c.FormatDSN()), &gorm.Config{Logger: l})
	if err != nil {
		t.Fatal("native test connection unavailable (identity withheld)")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal("native pool unavailable")
	}
	pool.SetMaxOpenConns(8)
	t.Cleanup(func() { pool.Close() })
	var identity struct {
		Name    string
		Version string
		Tables  int64
	}
	if err = db.Raw("SELECT DATABASE() name,VERSION() version,(SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()) tables").Scan(&identity).Error; err != nil {
		t.Fatal("native identity verification failed")
	}
	if identity.Name != c.DBName || !strings.HasPrefix(identity.Version, "10.11.") || !strings.Contains(identity.Version, "MariaDB") || identity.Tables != 0 {
		t.Fatal("requires actual MariaDB 10.11 and an initially empty matching schema")
	}
	t.Logf("Native fixture server=%s; schema guard passed", identity.Version)
	// Only known fixture tables are ever cleaned up, after the initial empty guard.
	t.Cleanup(func() {
		for _, table := range []string{"scopus_document_insights", "scopus_document_countries", "scopus_document_affiliations", "scopus_document_authors", "scopus_authors", "scopus_affiliations", "scopus_country_catalogue_guard", "scopus_documents"} {
			if err := db.Exec("DROP TABLE IF EXISTS `" + table + "`").Error; err != nil {
				t.Errorf("fixture table cleanup failed: %s", table)
			}
		}
	})
	if err = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci").AutoMigrate(&models.ScopusDocument{}, &models.ScopusAffiliation{}, &models.ScopusAuthor{}, &models.ScopusDocumentAuthor{}); err != nil {
		t.Fatal("fixture base schema failed")
	}
	for _, ddl := range []string{"ALTER TABLE scopus_documents MODIFY raw_json JSON NULL", "ALTER TABLE scopus_affiliations MODIFY afid VARCHAR(32) NOT NULL, MODIFY country TEXT NULL"} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatal("native audited column compatibility setup failed")
		}
	}
	applyNativeInsightMigration(t, db)
	applyNativeInsightMigration(t, db)
	return db, l
}
func applyNativeInsightMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "migrations", "050_20261005_scopus_core_insights.sql"))
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
		if strings.TrimSpace(stmt) != "" {
			if err := db.Exec(stmt).Error; err != nil {
				t.Fatalf("native migration statement failed (%T)", err)
			}
		}
	}
}
func nativeDocument(t *testing.T, db *gorm.DB, id uint, raw string) {
	t.Helper()
	if err := db.Create(&models.ScopusDocument{ID: id, EID: "native-fixture-" + string(rune('a'+id)), RawJSON: []byte(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return synchronizeCoreInsight(tx, id, []byte(raw)) }); err != nil {
		t.Fatal(err)
	}
}
func nativeMeta(t *testing.T, db *gorm.DB, id uint) models.ScopusDocumentInsight {
	t.Helper()
	var m models.ScopusDocumentInsight
	if err := db.Take(&m, "document_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	return m
}
func nativeSync(db *gorm.DB, id uint, raw string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var d models.ScopusDocument
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&d, id).Error; err != nil {
			return err
		}
		return synchronizeCoreInsight(tx, id, []byte(raw))
	})
}

func TestNativeMariaDBCoreInsights(t *testing.T) {
	db, l := nativeInsightDB(t)
	nativeDocument(t, db, 1, insightDomestic)
	nativeDocument(t, db, 2, insightDomestic)
	initial := nativeMeta(t, db, 1)
	applyNativeInsightMigration(t, db)
	if m := nativeMeta(t, db, 1); m.PayloadHash != initial.PayloadHash || !m.CheckedAt.Equal(initial.CheckedAt) {
		t.Fatal("migration rerun changed existing evidence")
	}
	// Actual FK rejection and cascade, not an AutoMigrate approximation.
	if err := db.Create(&models.ScopusDocumentCountry{DocumentID: 999, CountryKey: "thailand", CountryName: "Thailand", Provenance: "fixture"}).Error; err == nil {
		t.Fatal("migration FK allowed orphan")
	}
	before := nativeMeta(t, db, 1)
	if err := nativeSync(db, 1, insightDomestic); err != nil {
		t.Fatal(err)
	}
	if !before.CheckedAt.Equal(nativeMeta(t, db, 1).CheckedAt) {
		t.Fatal("native replay was not idempotent")
	}
	if err := nativeSync(db, 1, insightDual); err != nil {
		t.Fatal(err)
	}
	if m := nativeMeta(t, db, 1); m.International == nil || !*m.International {
		t.Fatal("native foreign replacement failed")
	}
	partial := `{"author-count":1,"author":{"authid":"one","afid":"A"}}`
	if err := nativeSync(db, 1, partial); err != nil {
		t.Fatal(err)
	}
	if m := nativeMeta(t, db, 1); m.International != nil || m.CountriesComplete {
		t.Fatal("native partial retained certainty")
	}
	if err := nativeSync(db, 1, insightDomestic); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ScopusAffiliation{Afid: " A ", Country: stringPtr("Thailand")}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{1, 2} {
		if m := nativeMeta(t, db, id); m.International != nil || m.Status != "dirty_catalogue" {
			t.Fatal("padded AFID insert trigger missed dependent")
		}
		if err := nativeSync(db, id, insightDomestic); err != nil {
			t.Fatal(err)
		}
		if m := nativeMeta(t, db, id); m.International == nil || *m.International {
			t.Fatal("canonical lookup did not match padded catalogue AFID")
		}
	}
	if err := db.Model(&models.ScopusAffiliation{}).Where("afid=?", " A ").Update("country", "India").Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{1, 2} {
		if m := nativeMeta(t, db, id); m.International != nil {
			t.Fatal("update left stale classification")
		}
	}
	if err := db.Model(&models.ScopusAffiliation{}).Where("afid=?", " A ").Update("afid", "RENAMED").Error; err != nil {
		t.Fatal(err)
	}
	if err := nativeSync(db, 1, insightDomestic); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ScopusAffiliation{}).Where("afid=?", "RENAMED").Update("afid", "a").Error; err != nil {
		t.Fatal(err)
	}
	if m := nativeMeta(t, db, 1); m.Status != "dirty_catalogue" {
		t.Fatal("AFID rename missed new key")
	}
	if err := nativeSync(db, 1, insightDomestic); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("afid=?", "a").Delete(&models.ScopusAffiliation{}).Error; err != nil {
		t.Fatal(err)
	}
	if m := nativeMeta(t, db, 1); m.Status != "dirty_catalogue" || m.International != nil {
		t.Fatal("delete trigger missed dependent")
	}
	if err := nativeSync(db, 1, insightDomestic); err != nil {
		t.Fatal(err)
	}
	// Raw update + restore still forces recomputation even when the old hash matches.
	if err := db.Create(&models.ScopusAffiliation{Afid: "á", Country: stringPtr("India")}).Error; err != nil {
		t.Fatal(err)
	}
	if err := nativeSync(db, 1, insightDomestic); err != nil {
		t.Fatal(err)
	}
	if m := nativeMeta(t, db, 1); m.International == nil || *m.International {
		t.Fatal("accent-insensitive catalogue collation invented AFID match")
	}
	accentRaw := `{"author-count":1,"author":{"authid":"one","afid":["a","á"]},"affiliation":[{"afid":"a","affiliation-country":"Thailand"},{"afid":"á","affiliation-country":"India"}]}`
	nativeDocument(t, db, 5, accentRaw)
	var distinct int64
	db.Model(&models.ScopusDocumentAffiliation{}).Where("document_id=5").Count(&distinct)
	if distinct != 2 {
		t.Fatal("binary AFID relation key lost distinct accent spelling")
	}
	if err := db.Model(&models.ScopusAffiliation{}).Where("afid=?", "á").Update("afid", " A ").Error; err != nil {
		t.Fatal(err)
	}
	if m := nativeMeta(t, db, 1); m.Status != "dirty_catalogue" {
		t.Fatal("binary rename change detection missed collation-equivalent source")
	}
	if err := db.Where("afid=?", " A ").Delete(&models.ScopusAffiliation{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := nativeSync(db, 1, insightDomestic); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ScopusDocument{}).Where("id=1").Update("raw_json", []byte(insightDual)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ScopusDocument{}).Where("id=1").Update("raw_json", []byte(insightDomestic)).Error; err != nil {
		t.Fatal(err)
	}
	if m := nativeMeta(t, db, 1); m.Status != "dirty_payload" || m.International != nil {
		t.Fatal("non-ingest payload writer not invalidated")
	}
	if err := nativeSync(db, 1, insightDomestic); err != nil {
		t.Fatal(err)
	}
	if nativeMeta(t, db, 1).Status != "complete" {
		t.Fatal("dirty payload hash shortcut")
	}
	// Real transaction rollback after replacement restores all relations and flags.
	err := db.Transaction(func(tx *gorm.DB) error {
		if e := synchronizeCoreInsight(tx, 1, []byte(insightDual)); e != nil {
			return e
		}
		return errors.New("fixture rollback")
	})
	if err == nil || nativeMeta(t, db, 1).International == nil || *nativeMeta(t, db, 1).International {
		t.Fatal("native rollback changed metadata")
	}
	var n int64
	db.Model(&models.ScopusDocumentCountry{}).Where("document_id=1").Count(&n)
	if n != 1 {
		t.Fatal("native rollback changed country memberships")
	}
	t.Run("concurrent_missing_padded_AFID", func(t *testing.T) { nativeConcurrentCatalogue(t, db, false) })
	t.Run("concurrent_existing_country_update", func(t *testing.T) { nativeConcurrentCatalogue(t, db, true) })
	t.Run("deadlock_rolls_back", func(t *testing.T) { nativeDeadlockRollback(t, db) })
	if err := db.Delete(&models.ScopusDocument{}, 2).Error; err != nil {
		t.Fatal(err)
	}
	db.Model(&models.ScopusDocumentInsight{}).Where("document_id=2").Count(&n)
	if n != 0 {
		t.Fatal("document delete did not cascade")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	found := false
	for _, q := range l.statements {
		if strings.Contains(q, "LOCK IN SHARE MODE") {
			found = true
		}
	}
	if !found {
		t.Fatal("MariaDB GORM SHARE syntax was not exercised")
	}
	t.Log("Executed real MariaDB LOCK IN SHARE MODE; migration/rerun/FK/triggers/replacement/concurrency checks passed")
}

func nativeConcurrentCatalogue(t *testing.T, db *gorm.DB, existing bool) {
	t.Helper()
	afid := " MISSING "
	id := uint(3)
	if existing {
		afid = " EXISTING "
		id = 4
		if err := db.Create(&models.ScopusAffiliation{Afid: afid, Country: stringPtr("Thailand")}).Error; err != nil {
			t.Fatal(err)
		}
	}
	raw := `{"author-count":1,"author":{"authid":"one","afid":"` + strings.TrimSpace(afid) + `"},"affiliation":{"afid":"` + strings.TrimSpace(afid) + `","affiliation-country":"Thailand"}}`
	nativeDocument(t, db, id, raw)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if _, err := normalizeCoreInsight(tx, []byte(raw), true); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if existing {
			done <- db.WithContext(ctx).Model(&models.ScopusAffiliation{}).Where("afid=?", afid).Update("country", "India").Error
		} else {
			done <- db.WithContext(ctx).Create(&models.ScopusAffiliation{Afid: afid, Country: stringPtr("India")}).Error
		}
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("catalogue mutation bypassed guard before normalize commit (err type %T)", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := synchronizeCoreInsight(tx, id, []byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(9 * time.Second):
		t.Fatal("catalogue mutation did not resume")
	}
	if m := nativeMeta(t, db, id); m.International != nil || m.Status != "dirty_catalogue" {
		t.Fatal("concurrent mutation published stale domestic flag")
	}
	if err := nativeSync(db, id, raw); err != nil {
		t.Fatal(err)
	}
	if m := nativeMeta(t, db, id); m.International == nil || !*m.International {
		t.Fatal("concurrent country correction was not recomputed")
	}
}

func nativeDeadlockRollback(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, id := range []uint{1, 2} {
		if err := nativeSync(db, id, insightDomestic); err != nil {
			t.Fatal(err)
		}
	}
	for _, afid := range []string{"LOCK-A", "LOCK-B"} {
		if err := db.Create(&models.ScopusAffiliation{Afid: afid, Country: stringPtr("Thailand")}).Error; err != nil {
			t.Fatal(err)
		}
	}
	a := db.Begin()
	b := db.Begin()
	defer a.Rollback()
	defer b.Rollback()
	if a.Error != nil || b.Error != nil {
		t.Fatal("native fixture begin failed")
	}
	// Two normalization transactions hold catalogue SHARE locks, then each asks
	// to mutate a different catalogue row. Guard upgrades/row locks form a cycle.
	for i, tx := range []*gorm.DB{a, b} {
		id := i + 1
		if err := tx.Model(&models.ScopusDocument{}).Where("id=?", id).Update("raw_json", []byte(insightDual)).Error; err != nil {
			t.Fatal(err)
		}
		if err := synchronizeCoreInsight(tx, uint(id), []byte(insightDual)); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, 2)
	go func() {
		e := a.Model(&models.ScopusAffiliation{}).Where("afid='LOCK-A'").Update("country", "India").Error
		if e == nil {
			e = a.Commit().Error
		} else {
			a.Rollback()
		}
		results <- e
	}()
	go func() {
		e := b.Model(&models.ScopusAffiliation{}).Where("afid='LOCK-B'").Update("country", "India").Error
		if e == nil {
			e = b.Commit().Error
		} else {
			b.Rollback()
		}
		results <- e
	}()
	failures := 0
	for i := 0; i < 2; i++ {
		select {
		case e := <-results:
			if e != nil {
				var me *driver.MySQLError
				if !errors.As(e, &me) || me.Number != 1213 {
					t.Fatalf("unexpected native lock failure (%T)", e)
				}
				failures++
			}
		case <-time.After(10 * time.Second):
			t.Fatal("deadlock fixture timed out")
		}
	}
	if failures != 1 {
		t.Fatalf("expected one deadlock victim, got %d", failures)
	}
	// Exactly one full unit commits. The victim restores raw JSON, metadata and
	// memberships together, rather than retaining a partial derived replacement.
	domestic := 0
	for _, id := range []uint{1, 2} {
		var d models.ScopusDocument
		if err := db.Take(&d, id).Error; err != nil {
			t.Fatal(err)
		}
		m := nativeMeta(t, db, id)
		if m.International == nil {
			t.Fatal("deadlock left incomplete publication")
		}
		if string(d.RawJSON) == insightDomestic {
			domestic++
			if *m.International {
				t.Fatal("victim retained uncommitted foreign flag")
			}
			var count int64
			db.Model(&models.ScopusDocumentCountry{}).Where("document_id=?", id).Count(&count)
			if count != 1 {
				t.Fatal("victim retained uncommitted countries")
			}
		} else if string(d.RawJSON) != insightDual || !*m.International {
			t.Fatal("survivor metadata disagrees with raw JSON")
		}
	}
	if domestic != 1 {
		t.Fatal("exactly one normalization transaction must roll back")
	}
}
