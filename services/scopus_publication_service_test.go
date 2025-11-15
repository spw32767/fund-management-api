package services

import (
	"database/sql/driver"
	"regexp"
	"testing"
)

func TestStatsByUserDeduplicatesDocumentsAndUsesMaxCitations(t *testing.T) {
	userQueryPattern := regexp.MustCompile(`SELECT .*Scopus_id.*FROM .*users.*user_id = \?`)
	authorQueryPattern := regexp.MustCompile(`SELECT .*id.*FROM .*scopus_authors.*scopus_author_id = \?`)
	dedupCountPattern := regexp.MustCompile(`(?is)SELECT COUNT\(\*\) FROM \(SELECT .*scopus_document_authors.*GROUP BY.*\) AS doc_ids`)
	rawCountPattern := regexp.MustCompile(`(?is)SELECT COUNT.*FROM scopus_documents AS sd.*sda\.author_id = \?`)
	trendPattern := regexp.MustCompile(`(?s)SELECT year, COUNT\(\*\) AS documents.*FROM \(SELECT .*doc_ids.*\) AS doc_rows GROUP BY .*year.*ORDER BY year ASC`)

	steps := []*queryStep{
		{
			kind:    kindQuery,
			pattern: userQueryPattern,
			args:    []driver.Value{int64(1)},
			columns: []string{"Scopus_id"},
			rows:    [][]driver.Value{{"12345"}},
		},
		{
			kind:    kindQuery,
			pattern: authorQueryPattern,
			args:    []driver.Value{"12345"},
			columns: []string{"id"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		{
			kind:    kindQuery,
			pattern: dedupCountPattern,
			args:    []driver.Value{int64(1)},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(2)}},
		},
		{
			kind:    kindQuery,
			pattern: rawCountPattern,
			args:    []driver.Value{int64(1)},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(3)}},
		},
		{
			kind:    kindQuery,
			pattern: trendPattern,
			args:    []driver.Value{int64(1)},
			columns: []string{"year", "documents", "citations"},
			rows: [][]driver.Value{
				{int64(2019), int64(1), int64(50)},
				{int64(2020), int64(1), int64(5)},
			},
		},
	}

	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()

	svc := NewScopusPublicationService(db)

	stats, meta, err := svc.StatsByUser(1)
	if err != nil {
		t.Fatalf("StatsByUser returned error: %v", err)
	}

	if !meta.HasScopusID || !meta.HasAuthor {
		t.Fatalf("expected meta flags to be true, got %#v", meta)
	}

	if stats.TotalDocuments != 2 {
		t.Fatalf("expected 2 documents, got %d", stats.TotalDocuments)
	}

	if stats.TotalCitations != 55 {
		t.Fatalf("expected 55 citations, got %d", stats.TotalCitations)
	}

	if len(stats.Trend) != 2 {
		t.Fatalf("expected 2 trend entries, got %d", len(stats.Trend))
	}

	if stats.Trend[0].Year != 2019 || stats.Trend[0].Citations != 50 {
		t.Fatalf("unexpected first trend row: %+v", stats.Trend[0])
	}

	if stats.Trend[1].Year != 2020 || stats.Trend[1].Citations != 5 {
		t.Fatalf("unexpected second trend row: %+v", stats.Trend[1])
	}

	if err := state.verifyComplete(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
