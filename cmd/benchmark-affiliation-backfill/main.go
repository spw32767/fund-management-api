package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"fund-management-api/config"
	"fund-management-api/services"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}
	config.InitDB()

	batchSize := flag.Int("batch-size", 100, "number of benchmark documents per batch")
	flag.Parse()
	if *batchSize <= 0 {
		log.Fatal("batch-size must be greater than zero")
	}

	db := config.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	service := services.NewScopusBenchmarkService(db, nil)
	summary, err := service.BackfillBenchmarkAffiliations(context.Background(), *batchSize, func(progress services.BenchmarkAffiliationBackfillSummary) {
		log.Printf("processed=%d affiliations_created=%d links_matched=%d links_updated=%d missing=%d",
			progress.DocumentsProcessed, progress.AffiliationsCreated, progress.AuthorLinksMatched,
			progress.AuthorLinksUpdated, progress.AuthorsOrLinksMissing)
	})
	if err != nil {
		log.Fatalf("benchmark affiliation backfill failed: %v", err)
	}

	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(encoded))
}
