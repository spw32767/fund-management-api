// Command scholar-import imports publications from Google Scholar for users in the database.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"fund-management-api/config"
	"fund-management-api/services"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	config.InitDB()

	var (
		userIDsRaw string
		limit      int
		dryRun     bool
		trigger    string
		lockName   string
	)

	flag.StringVar(&userIDsRaw, "user-ids", "", "comma-separated list of user IDs to import (optional)")
	flag.IntVar(&limit, "limit", 0, "maximum number of users to process (optional)")
	flag.BoolVar(&dryRun, "dry-run", false, "fetch data without writing to the database")
	flag.StringVar(&trigger, "trigger", "cli", "trigger source label stored in scholar_import_runs")
	flag.StringVar(&lockName, "lock-name", "scholar_import_job", "MySQL advisory lock name (empty to disable)")
	flag.Parse()

	if limit < 0 {
		log.Fatal("limit must be greater than or equal to 0")
	}

	var userIDs []uint
	if strings.TrimSpace(userIDsRaw) != "" {
		parts := strings.Split(userIDsRaw, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id64, err := strconv.ParseUint(part, 10, 64)
			if err != nil || id64 == 0 {
				log.Fatalf("invalid user id '%s'", part)
			}
			userIDs = append(userIDs, uint(id64))
		}
	}

	job := services.NewScholarImportJobService(nil)
	summary, err := job.RunForAll(context.Background(), &services.ScholarImportAllInput{
		UserIDs:       userIDs,
		Limit:         limit,
		TriggerSource: trigger,
		LockName:      lockName,
		DryRun:        dryRun,
		RecordRun:     !dryRun,
	})
	if err != nil {
		if errors.Is(err, services.ErrScholarImportAlreadyRunning) {
			log.Fatal("scholar import already running (advisory lock held)")
		}
		log.Fatalf("scholar import failed: %v", err)
	}

	fmt.Printf("Users processed: %d (errors: %d)\n", summary.UsersProcessed, summary.UsersWithErrors)
	fmt.Printf("Publications fetched: %d, created: %d, updated: %d, failed: %d\n",
		summary.PublicationsFetched,
		summary.PublicationsCreated,
		summary.PublicationsUpdated,
		summary.PublicationsFailed,
	)

	if dryRun {
		fmt.Println("Dry run complete. No database changes were made.")
	}

	if summary.UsersWithErrors > 0 || summary.PublicationsFailed > 0 {
		os.Exit(2)
	}
}
