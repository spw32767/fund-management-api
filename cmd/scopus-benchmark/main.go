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
	)
	flag.StringVar(&scopeRef, "scope", "", "scope code or id to harvest (e.g. university_kku)")
	flag.IntVar(&yearsBack, "years-back", 0, "limit harvest/counts to the last N years (0 = all years)")
	flag.BoolVar(&countsOnly, "counts-only", false, "only refresh CS counts (no document harvest)")
	flag.Parse()

	svc := services.NewScopusBenchmarkService(nil, nil)
	ctx := context.Background()

	if countsOnly {
		refreshCounts(ctx, svc, yearsBack)
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

	summary, err := svc.HarvestScope(ctx, scope, yearFrom, yearTo)
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

func refreshCounts(ctx context.Context, svc *services.ScopusBenchmarkService, yearsBack int) {
	var scopes []models.ScopusBenchmarkScope
	if err := config.DB.Where("active = 1").Order("id ASC").Find(&scopes).Error; err != nil {
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
