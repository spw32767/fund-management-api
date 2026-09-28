// Command scopus-author-roles fills first/corresponding flags from Scopus
// Abstract Retrieval XML for locally stored faculty publications.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"

	"fund-management-api/config"
	"fund-management-api/services"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	limit := flag.Int("limit", 0, "maximum pending documents to fetch (0 = all)")
	refresh := flag.Bool("refresh", false, "re-fetch documents already checked")
	reviewOnly := flag.Bool("retry-review", false, "re-fetch only documents marked needs_review")
	flag.Parse()
	if err := godotenv.Load(); err != nil {
		log.Printf(".env not loaded: %v", err)
	}
	config.InitDB()
	config.DB = config.DB.Session(&gorm.Session{Logger: config.DB.Logger.LogMode(logger.Warn)})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	summary, err := services.NewScopusAuthorRoleService(nil, nil).Backfill(ctx, *limit, *refresh, *reviewOnly)
	if summary != nil {
		_ = json.NewEncoder(os.Stdout).Encode(summary)
	}
	if err != nil {
		log.Fatal(err)
	}
}
