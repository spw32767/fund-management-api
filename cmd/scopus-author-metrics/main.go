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

	flag.StringVar(&userIDsRaw, "user-ids", "", "comma separated list of user IDs to process (optional)")
	flag.IntVar(&limit, "limit", 0, "maximum number of users to process (optional)")
	flag.Parse()

	if limit < 0 {
		log.Fatal("limit must be greater than or equal to 0")
	}

	userIDs, err := parseUserIDs(userIDsRaw)
	if err != nil {
		log.Fatalf("invalid user ids: %v", err)
	}

	svc := services.NewAuthorMetricsService(nil, nil)
	summary, err := svc.RunForAll(context.Background(), &services.ScopusAuthorMetricsAllInput{
		UserIDs:       userIDs,
		Limit:         limit,
		TriggerSource: "cli",
	})
	if err != nil {
		log.Fatalf("scopus author metrics failed: %v", err)
	}

	fmt.Printf("Users processed: %d (errors: %d, not found: %d)\n",
		summary.UsersProcessed, summary.UsersWithErrors, summary.NotFound)
	fmt.Printf("Metrics upserted: %d\n", summary.MetricsUpserted)

	if summary.UsersWithErrors > 0 {
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
