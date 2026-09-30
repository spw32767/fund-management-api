package services

import (
	"fund-management-api/models"
	"gorm.io/gorm"
	"strings"
	"testing"
)

// Connection hands callers an initialized handle. A condition added to the
// document subquery must not leak into the following year's author upsert or
// run finalization query on that same physical connection.
func TestBenchmarkServiceIsolatesPinnedConnectionStatements(t *testing.T) {
	db, _, cleanup := newScriptedGormDB(t, nil)
	defer cleanup()
	pinned := db.Session(&gorm.Session{Initialized: true})
	svc := NewScopusBenchmarkService(pinned, nil)
	_ = svc.db.Model(&models.ScopusBenchmarkDocument{}).Where("eid IN ?", []string{"2-s2.0-123"}).Select("id")
	q := svc.db.Model(&models.ScopusBenchmarkHarvestRun{}).Where("id=?", 7).Session(&gorm.Session{DryRun: true, SkipDefaultTransaction: true}).Updates(map[string]interface{}{"status": "success"})
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	sql := q.Statement.SQL.String()
	if !strings.Contains(sql, "scopus_benchmark_harvest_runs") || strings.Contains(sql, "eid") || len(q.Statement.Vars) != 3 {
		t.Fatalf("polluted run statement: %s %#v", sql, q.Statement.Vars)
	}
}
