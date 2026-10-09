package services

import (
	"database/sql/driver"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"regexp"
	"strings"
	"testing"
)

func TestCoreInsightMariaDBDriverShareDialect(t *testing.T) {
	db, state, cleanup := newScriptedGormDB(t, []*queryStep{{kind: kindQuery, pattern: regexp.MustCompile(`^SELECT VERSION\(\)$`), columns: []string{"version"}, rows: [][]driver.Value{{"10.11.18-MariaDB"}}}})
	defer cleanup()
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	maria, err := gorm.Open(mysql.New(mysql.Config{Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	var guard struct{ ID uint }
	q := maria.Session(&gorm.Session{DryRun: true}).Table("scopus_country_catalogue_guard").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&guard)
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	stmt := q.Statement.SQL.String()
	if !strings.Contains(stmt, "LOCK IN SHARE MODE") || strings.Contains(stmt, "FOR SHARE") {
		t.Fatal("MariaDB version detection produced wrong lock dialect", stmt)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}
