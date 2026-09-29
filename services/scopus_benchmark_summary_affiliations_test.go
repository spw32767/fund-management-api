package services

import (
	"database/sql/driver"
	"encoding/json"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"regexp"
	"testing"
	"time"
)

func TestSummaryNormalizeAllAuthorAffiliationsAndReconcile(t *testing.T) {
	tick := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	exec := func(pattern string, args ...driver.Value) *queryStep {
		return &queryStep{kind: kindExec, pattern: regexp.MustCompile(pattern), args: args, result: scriptedResult{rowsAffected: 1}}
	}
	steps := []*queryStep{
		exec("DELETE FROM `scopus_benchmark_document_affiliations`", int64(1)),
		{kind: kindQuery, pattern: regexp.MustCompile("FROM scopus_benchmark_document_authors l"), args: []driver.Value{int64(1)}, columns: []string{"author_id", "scopus_author_id", "affiliations_complete"}, rows: [][]driver.Value{{int64(10), "A", false}}},
		exec("DELETE FROM `scopus_benchmark_author_affiliations`.*IN", int64(1), int64(10)),
		exec("INSERT INTO `scopus_benchmark_author_affiliations`", int64(1), int64(10), "other", "benchmark_payload", int64(1), int64(10), "60017165", "benchmark_payload"),
		exec("UPDATE `scopus_benchmark_document_authors`", true, int64(1), int64(10)),
		exec("DELETE FROM `scopus_benchmark_author_affiliations`.*NOT IN", int64(1), int64(10)),
		exec("DELETE FROM `scopus_benchmark_document_authors`.*NOT IN", int64(1), int64(10)),
		exec("INSERT INTO `scopus_benchmark_document_affiliations`", int64(1), "60017165", "benchmark_payload", int64(1), "other", "benchmark_payload"),
		exec("UPDATE `scopus_benchmark_documents`", true, tick, int64(1)),
	}
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	entry, e := parseScopusEntry(json.RawMessage(`{"eid":"one","author-count":"1","affiliation":[{"afid":"60017165"}],"author":[{"authid":"A","afid":["other","60017165"]}]}`))
	if e != nil {
		t.Fatal(e)
	}
	tx := db.Session(&gorm.Session{SkipDefaultTransaction: true, NowFunc: func() time.Time { return tick }, Logger: logger.Default.LogMode(logger.Silent)})
	if e = replaceBenchmarkAffiliationMetadata(tx, 1, entry, "benchmark_payload", true); e != nil {
		t.Fatal(e)
	}
	if e = state.verifyComplete(); e != nil {
		t.Fatal(e)
	}
}
func TestSummaryTruncatedPayloadDoesNotDeleteAuthorRosterOrClaimComplete(t *testing.T) {
	steps := []*queryStep{{kind: kindQuery, pattern: regexp.MustCompile("FROM scopus_benchmark_document_authors l"), args: []driver.Value{int64(1)}, columns: []string{"author_id", "scopus_author_id", "affiliations_complete"}, rows: [][]driver.Value{{int64(10), "A", false}}}}
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	entry, e := parseScopusEntry(json.RawMessage(`{"eid":"one","author-count":2,"author":[{"authid":"A"}]}`))
	if e != nil {
		t.Fatal(e)
	}
	tx := db.Session(&gorm.Session{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if e = replaceBenchmarkAffiliationMetadata(tx, 1, entry, "benchmark_payload", true); e != nil {
		t.Fatal(e)
	}
	if e = state.verifyComplete(); e != nil {
		t.Fatal(e)
	}
}
