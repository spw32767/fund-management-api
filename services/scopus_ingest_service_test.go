package services

import (
	"database/sql/driver"
	"regexp"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUpsertAuthorsAndLinks_PreservesExistingORCID(t *testing.T) {
	steps := []*queryStep{
		{
			kind:    kindQuery,
			pattern: regexp.MustCompile(`SELECT .* FROM ` + "`scopus_authors`"),
			args:    []driver.Value{"123"},
			columns: []string{"id", "scopus_author_id", "full_name", "given_name", "surname", "initials", "orcid", "author_url"},
			rows:    [][]driver.Value{},
		},
		{
			kind:    kindExec,
			pattern: regexp.MustCompile(`INSERT INTO ` + "`scopus_authors`"),
			args:    []driver.Value{"123", "Example Author", nil, nil, nil, "0000-0001-2345-6789", nil},
			result:  scriptedResult{lastInsertID: 1, rowsAffected: 1},
		},
		{
			kind:    kindQuery,
			pattern: regexp.MustCompile(`SELECT .* FROM ` + "`scopus_document_authors`"),
			args:    []driver.Value{int64(1), int64(1)},
			columns: []string{"id", "document_id", "author_id", "author_seq", "affiliation_id"},
			rows:    [][]driver.Value{},
		},
		{
			kind:    kindExec,
			pattern: regexp.MustCompile(`INSERT INTO ` + "`scopus_document_authors`"),
			args:    []driver.Value{int64(1), int64(1), int64(1), nil},
			result:  scriptedResult{lastInsertID: 1, rowsAffected: 1},
		},
		{
			kind:    kindQuery,
			pattern: regexp.MustCompile(`SELECT .* FROM ` + "`scopus_authors`"),
			args:    []driver.Value{"123"},
			columns: []string{"id", "scopus_author_id", "full_name", "given_name", "surname", "initials", "orcid", "author_url"},
			rows: [][]driver.Value{{
				int64(1), "123", "Example Author", nil, nil, nil, "0000-0001-2345-6789", nil,
			}},
		},
		{
			kind:    kindExec,
			pattern: regexp.MustCompile(`UPDATE ` + "`scopus_authors`"),
			args:    []driver.Value{"123", "Example Author", nil, nil, nil, "0000-0001-2345-6789", nil, int64(1)},
			result:  scriptedResult{rowsAffected: 1},
		},
		{
			kind:    kindQuery,
			pattern: regexp.MustCompile(`SELECT .* FROM ` + "`scopus_document_authors`"),
			args:    []driver.Value{int64(1), int64(1)},
			columns: []string{"id", "document_id", "author_id", "author_seq", "affiliation_id"},
			rows: [][]driver.Value{{
				int64(1), int64(1), int64(1), int64(1), nil,
			}},
		},
		{
			kind:    kindExec,
			pattern: regexp.MustCompile(`UPDATE ` + "`scopus_document_authors`"),
			args:    []driver.Value{int64(1), int64(1), int64(1), nil, int64(1)},
			result:  scriptedResult{rowsAffected: 1},
		},
	}

	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()

	svc := &ScopusIngestService{db: db}
	tx := db.Session(&gorm.Session{SkipDefaultTransaction: true, Logger: db.Logger.LogMode(logger.Silent)})

	entry := &scopusEntry{
		Author: scopusAuthors{{
			AuthID:   "123",
			AuthName: "Example Author",
			ORCID:    "0000-0001-2345-6789",
		}},
	}
	result := &ScopusIngestResult{}

	if err := svc.upsertAuthorsAndLinks(tx, entry, 1, map[string]uint{}, result); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	entry.Author[0].ORCID = ""

	if err := svc.upsertAuthorsAndLinks(tx, entry, 1, map[string]uint{}, result); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	if err := state.verifyComplete(); err != nil {
		t.Fatalf("unexpected remaining steps: %v", err)
	}
}
