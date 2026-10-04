// Normalize saved core Search payloads. No HTTP/API harvesting capabilities.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"fund-management-api/services"
	gomysql "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func validateTarget(apply bool, expected, actual string) error {
	if expected == "" || expected != actual {
		return fmt.Errorf("exact --expect-database matching configured DB_DATABASE is required")
	}
	if apply && strings.TrimSpace(actual) == "" {
		return fmt.Errorf("apply target must be explicitly named")
	}
	return nil
}

func lockRetryHint(err error) string {
	var failure *gomysql.MySQLError
	if errors.As(err, &failure) && (failure.Number == 1213 || failure.Number == 1205) {
		return "retryable lock conflict; rerun from last_id"
	}
	return "resume from last_id after correcting the failure"
}
func run() error {
	apply := flag.Bool("apply", false, "persist derived evidence (default: read-only dry-run)")
	expect := flag.String("expect-database", "", "required exact target database name")
	after := flag.Uint("after-id", 0, "exclusive resume cursor")
	through := flag.Uint("through-id", 0, "required inclusive fixed high watermark")
	limit := flag.Int("limit", 1000, "maximum documents this invocation (1..100000)")
	batch := flag.Int("batch-size", 100, "bounded SELECT batch size (1..500)")
	flag.Parse()
	_ = godotenv.Load()
	name := os.Getenv("DB_DATABASE")
	if err := validateTarget(*apply, *expect, name); err != nil {
		return err
	}
	if *through <= *after || *limit < 1 || *limit > 100000 || *batch < 1 || *batch > 500 {
		return fmt.Errorf("invalid cursor/limit/batch bounds")
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "3306"
	}
	cfg := gomysql.NewConfig()
	cfg.User = os.Getenv("DB_USERNAME")
	cfg.Passwd = os.Getenv("DB_PASSWORD")
	cfg.Net = "tcp"
	cfg.Addr = os.Getenv("DB_HOST") + ":" + port
	cfg.DBName = name
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Timeout = 10 * time.Second
	cfg.ReadTimeout = 45 * time.Second
	cfg.WriteTimeout = 45 * time.Second
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("configured database connection failed (connection identity withheld)")
	}
	pool, err := db.DB()
	if err != nil {
		return fmt.Errorf("database pool unavailable")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	var actual string
	if err = db.Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil {
		return fmt.Errorf("could not verify connected database")
	}
	if err = validateTarget(*apply, *expect, actual); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	opt := services.ScopusInsightBackfillOptions{AfterID: *after, ThroughID: *through, Limit: *limit, BatchSize: *batch, Apply: *apply}
	var result services.ScopusInsightBackfillResult
	if *apply {
		result, err = services.BackfillCoreScopusInsights(ctx, db, opt)
	} else {
		// MySQL driver emits START TRANSACTION READ ONLY. Never commit dry-run.
		tx := db.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
		if tx.Error != nil {
			return fmt.Errorf("read-only transaction unavailable")
		}
		defer tx.Rollback()
		result, err = services.BackfillCoreScopusInsights(ctx, tx, opt)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if err != nil {
		return fmt.Errorf("audit/backfill failed near document %d (database error withheld; %s)", result.LastID, lockRetryHint(err))
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
