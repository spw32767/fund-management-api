package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Standalone runner for the Scopus benchmark harvest / count refresh.
//
// Examples:
//
//	scopus-benchmark -counts-only              # refresh CS counts for all active scopes (snapshots)
//	scopus-benchmark -scope university_kku -years-back 10
//	scopus-benchmark -scope country_thailand   # harvest all years
func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}
	config.ReloadMailerConfig()
	config.InitDB()

	var (
		scopeRef   string
		yearsBack  int
		countsOnly bool
		expectDB   string
		quietSQL   bool
		cancelRun  uint64
		failRun    uint64
	)
	flag.StringVar(&scopeRef, "scope", "", "scope code or id to harvest (e.g. university_kku)")
	flag.IntVar(&yearsBack, "years-back", 0, "limit harvest/counts to the last N years (0 = all years)")
	flag.BoolVar(&countsOnly, "counts-only", false, "only refresh CS counts (no document harvest)")
	flag.StringVar(&expectDB, "expect-database", "", "abort unless connected to this database")
	flag.BoolVar(&quietSQL, "quiet-sql", false, "suppress SQL logging during long harvests")
	flag.Uint64Var(&cancelRun, "cancel-run", 0, "request cancellation of this running harvest; requires -expect-database")
	flag.Uint64Var(&failRun, "recover-failed-run", 0, "mark an orphaned run failed after its process exited; requires -expect-database")
	flag.Parse()
	if expectDB != "" {
		var actual string
		if err := config.DB.Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil {
			log.Fatal(err)
		}
		if actual != expectDB {
			log.Fatalf("database guard: expected %q, connected to %q", expectDB, actual)
		}
		fmt.Printf("database=%s\n", actual)
	}
	if quietSQL {
		config.DB = config.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	}
	if cancelRun > 0 {
		if expectDB == "" {
			log.Fatal("cancellation requires -expect-database")
		}
		result := config.DB.Model(&models.ScopusBenchmarkHarvestRun{}).Where("id=? AND status='running'", cancelRun).Update("status", "cancelled")
		if result.Error != nil {
			log.Fatal(result.Error)
		}
		fmt.Printf("cancel_requested=%d affected=%d\n", cancelRun, result.RowsAffected)
		return
	}
	if failRun > 0 {
		if expectDB == "" {
			log.Fatal("recovery requires -expect-database")
		}
		now := time.Now()
		result := config.DB.Model(&models.ScopusBenchmarkHarvestRun{}).Where("id=? AND status='running'", failRun).Updates(map[string]interface{}{"status": "failed", "finished_at": now, "error_message": "Runner exited before finalization; recovered by operator. Retry harvest to complete the requested range."})
		if result.Error != nil {
			log.Fatal(result.Error)
		}
		fmt.Printf("failed_run_recovered=%d affected=%d\n", failRun, result.RowsAffected)
		return
	}

	svc := services.NewScopusBenchmarkService(nil, nil)
	ctx := context.Background()

	if countsOnly {
		refreshCounts(ctx, svc, yearsBack, scopeRef)
		return
	}

	if strings.TrimSpace(scopeRef) == "" {
		log.Fatal("either -scope or -counts-only is required")
	}

	scope, err := loadScope(scopeRef)
	if err != nil {
		log.Fatalf("load scope: %v", err)
	}

	var yearFrom, yearTo *int
	if yearsBack > 0 {
		current := time.Now().Year()
		from := current - yearsBack + 1
		yearFrom, yearTo = &from, &current
	}

	var summary *services.ScopusBenchmarkHarvestSummary
	// Named locks belong to a physical MySQL connection, so pin the runner.
	err = config.DB.Connection(func(conn *gorm.DB) error {
		var runErr error
		summary, runErr = services.NewScopusBenchmarkService(conn, nil).HarvestScope(ctx, scope, yearFrom, yearTo)
		return runErr
	})
	if err != nil {
		log.Fatalf("harvest failed: %v", err)
	}
	fmt.Printf("scope=%s total=%d pages=%d documents=%d requests=%d faculty_links=%d\n",
		scope.Code, summary.TotalResultsReported, summary.PagesFetched,
		summary.DocumentsUpserted, summary.RequestsMade, summary.FacultyLinks)
}

func loadScope(ref string) (*models.ScopusBenchmarkScope, error) {
	var scope models.ScopusBenchmarkScope
	q := config.DB
	if id, err := strconv.ParseUint(strings.TrimSpace(ref), 10, 64); err == nil && id > 0 {
		q = q.Where("id = ?", id)
	} else {
		q = q.Where("code = ?", strings.TrimSpace(ref))
	}
	if err := q.First(&scope).Error; err != nil {
		return nil, err
	}
	return &scope, nil
}

func refreshCounts(ctx context.Context, svc *services.ScopusBenchmarkService, yearsBack int, scopeRef string) {
	var scopes []models.ScopusBenchmarkScope
	query := config.DB.Where("active = 1")
	if strings.TrimSpace(scopeRef) != "" {
		scope, err := loadScope(scopeRef)
		if err != nil {
			log.Fatal(err)
		}
		query = query.Where("id=?", scope.ID)
	}
	if err := query.Order("id ASC").Find(&scopes).Error; err != nil {
		log.Fatalf("load scopes: %v", err)
	}
	for i := range scopes {
		scope := &scopes[i]
		total, err := svc.CountScope(ctx, scope, nil)
		if err != nil {
			log.Printf("count %s failed: %v", scope.Code, err)
			continue
		}
		fmt.Printf("scope=%s total=%d\n", scope.Code, total)
		if yearsBack > 0 {
			current := time.Now().Year()
			for y := current; y > current-yearsBack; y-- {
				year := y
				n, err := svc.CountScope(ctx, scope, &year)
				if err != nil {
					log.Printf("count %s %d failed: %v", scope.Code, year, err)
					continue
				}
				fmt.Printf("scope=%s year=%d count=%d\n", scope.Code, year, n)
			}
		}
	}
}
