package services

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"fund-management-api/models"
)

// emptyResultStubBody is what Scopus returns for a query/year-slice with zero
// results: a single {"error":"Result set was empty"} entry (len 1, no eid)
// rather than an empty array, with total 0 and an empty next cursor.
const emptyResultStubBody = `{"search-results":{"opensearch:totalResults":"0","entry":[{"error":"Result set was empty"}],"cursor":{"@next":""}}}`

// TestUpsertBenchmarkEntrySkipsEmptyResultStub verifies that the empty-result
// stub maps to the errBenchmarkEntryMissingEID sentinel (so the harvest loop can
// skip it) rather than a generic error that would abort the whole run.
func TestUpsertBenchmarkEntrySkipsEmptyResultStub(t *testing.T) {
	svc := NewScopusBenchmarkService(nil, nil) // eid check returns before any DB access
	stub := json.RawMessage(`{"error":"Result set was empty"}`)

	_, err := svc.upsertBenchmarkEntry(context.Background(), stub, 1, map[string]bool{}, &ScopusBenchmarkHarvestSummary{})
	if !errors.Is(err, errBenchmarkEntryMissingEID) {
		t.Fatalf("err = %v, want errBenchmarkEntryMissingEID", err)
	}
}

// TestSearchPageCursorEmptyResultYieldsSingleStubEntry documents the Scopus quirk
// the fix guards against: a zero-result page reports total 0 but still carries one
// stub entry (so len(entries)==0 alone never triggers), and that stub has no eid.
func TestSearchPageCursorEmptyResultYieldsSingleStubEntry(t *testing.T) {
	client := &http.Client{Transport: benchmarkRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(emptyResultStubBody)),
			Header:     make(http.Header),
		}, nil
	})}

	total, entries, next, err := NewScopusBenchmarkService(nil, client).
		searchPageCursor(context.Background(), "test-key", "AF-ID(60017165) AND SUBJAREA(COMP) AND PUBYEAR = 2001", "*", 25, "COMPLETE")
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (Scopus returns a stub, not an empty array)", len(entries))
	}
	if next != "" {
		t.Fatalf("next = %q, want empty", next)
	}
	entry, err := parseScopusEntry(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(entry.EID) != "" {
		t.Fatalf("stub eid = %q, want empty", entry.EID)
	}
}

// TestHarvestQuerySkipsZeroResultYearWithoutFailing is the end-to-end guard: a
// year with no documents (e.g. KKU COMP in 2001) must not fail the run. The loop
// should break on total==0, upsert nothing, and still reconcile that year's
// (now-empty) scope memberships.
func TestHarvestQuerySkipsZeroResultYearWithoutFailing(t *testing.T) {
	const (
		runID   = uint64(7)
		scopeID = uint64(3)
		year    = 2001
	)

	steps := []*queryStep{
		{ // isCancelRequested: SELECT status FROM scopus_benchmark_harvest_runs WHERE id = ?
			kind:    kindQuery,
			pattern: regexp.MustCompile("FROM `scopus_benchmark_harvest_runs`"),
			args:    []driver.Value{int64(runID)},
			columns: []string{"status"},
			rows:    [][]driver.Value{{"running"}},
		},
		{ // reconcileHarvestedScopeYear: DELETE ... WHERE scope_id = ? AND pub_year = ?
			kind:    kindExec,
			pattern: regexp.MustCompile("DELETE FROM `scopus_benchmark_document_scopes`"),
			args:    []driver.Value{int64(scopeID), int64(year)},
			result:  scriptedResult{rowsAffected: 0},
		},
	}

	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()

	client := &http.Client{Transport: benchmarkRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(emptyResultStubBody)),
			Header:     make(http.Header),
		}, nil
	})}

	// SkipDefaultTransaction: the scripted conn does not implement Begin, and the
	// reconcile DELETE would otherwise be wrapped in GORM's default write tx.
	session := db.Session(&gorm.Session{SkipDefaultTransaction: true, Logger: db.Logger.LogMode(logger.Silent)})
	svc := NewScopusBenchmarkService(session, client)
	afid := "60017165"
	scope := &models.ScopusBenchmarkScope{ID: scopeID, Code: "university_kku", Level: "university", AfID: &afid, SubjectArea: "COMP"}
	run := &models.ScopusBenchmarkHarvestRun{ID: runID}
	summary := &ScopusBenchmarkHarvestSummary{}
	y := year

	if err := svc.harvestQuery(context.Background(), "test-key", scope, &y, map[string]bool{}, run, summary); err != nil {
		t.Fatalf("harvestQuery returned error for a zero-result year: %v", err)
	}
	if summary.RequestsMade != 1 {
		t.Fatalf("RequestsMade = %d, want 1", summary.RequestsMade)
	}
	if summary.PagesFetched != 0 {
		t.Fatalf("PagesFetched = %d, want 0", summary.PagesFetched)
	}
	if summary.DocumentsUpserted != 0 {
		t.Fatalf("DocumentsUpserted = %d, want 0", summary.DocumentsUpserted)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatalf("unmet DB expectations: %v", err)
	}
}
