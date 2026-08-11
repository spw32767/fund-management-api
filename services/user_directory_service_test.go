package services

import (
	"database/sql/driver"
	"regexp"
	"testing"
)

func TestUserDirectoryListForPartnerBuildsQueries(t *testing.T) {
	countPattern := regexp.MustCompile(`(?is)SELECT count\(\*\) FROM users AS u LEFT JOIN roles AS r.*u\.delete_at IS NULL`)
	listPattern := regexp.MustCompile(`(?is)SELECT u\.user_id.*FROM users AS u LEFT JOIN roles AS r.*u\.delete_at IS NULL.*LIMIT \?`)

	steps := []*queryStep{
		{
			kind:    kindQuery,
			pattern: countPattern,
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(2)}},
		},
		{
			kind:    kindQuery,
			pattern: listPattern,
			args:    []driver.Value{int64(5)},
			columns: []string{"user_id"},
			rows:    [][]driver.Value{},
		},
	}

	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()

	items, total, err := NewUserDirectoryService(db).ListForPartner(nil, 5, 0)
	if err != nil {
		t.Fatalf("ListForPartner returned error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(items) != 0 {
		t.Fatalf("expected no mapped rows from the empty result set, got %d", len(items))
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
