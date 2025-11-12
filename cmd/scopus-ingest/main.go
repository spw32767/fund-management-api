package main

import (
	"context"
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

	config.ReloadMailerConfig()
	config.InitDB()

	var (
		userIDsRaw string
		limit      int
	)

	flag.StringVar(&userIDsRaw, "user-ids", "", "comma separated list of user IDs to import (optional)")
	flag.IntVar(&limit, "limit", 0, "maximum number of users to process (optional)")
	flag.Parse()

	if limit < 0 {
		log.Fatal("limit must be greater than or equal to 0")
	}

	userIDs, err := parseUserIDs(userIDsRaw)
	if err != nil {
		log.Fatalf("invalid user ids: %v", err)
	}

	job := services.NewScopusIngestJobService(nil)
	summary, err := job.RunForAll(context.Background(), &services.ScopusIngestAllInput{
		UserIDs: userIDs,
		Limit:   limit,
	})
	if err != nil {
		log.Fatalf("scopus ingest failed: %v", err)
	}

	fmt.Printf("Users processed: %d (errors: %d)\n", summary.UsersProcessed, summary.UsersWithErrors)
	fmt.Printf("Documents fetched: %d, created: %d, updated: %d, failed: %d\n",
		summary.DocumentsFetched,
		summary.DocumentsCreated,
		summary.DocumentsUpdated,
		summary.DocumentsFailed,
	)
	fmt.Printf("Authors created: %d, updated: %d\n", summary.AuthorsCreated, summary.AuthorsUpdated)
	fmt.Printf("Affiliations created: %d, updated: %d\n", summary.AffiliationsCreated, summary.AffiliationsUpdated)
	fmt.Printf("Document-author links inserted: %d, updated: %d\n",
		summary.LinksInserted,
		summary.LinksUpdated,
	)

	if summary.UsersWithErrors > 0 || summary.DocumentsFailed > 0 {
		os.Exit(2)
	}
}

func parseUserIDs(raw string) ([]uint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	var ids []uint
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id64, err := strconv.ParseUint(part, 10, 64)
		if err != nil || id64 == 0 {
			return nil, fmt.Errorf("invalid user id '%s'", part)
		}
		ids = append(ids, uint(id64))
	}
	return ids, nil
}
