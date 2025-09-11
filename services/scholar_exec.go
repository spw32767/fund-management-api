package services

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

type ScholarPub struct {
	Title            string   `json:"title"`
	Authors          []string `json:"authors"`
	Venue            *string  `json:"venue"`
	Year             *int     `json:"year"`
	URL              *string  `json:"url"`
	DOI              *string  `json:"doi"`
	ScholarClusterID *string  `json:"scholar_cluster_id"`
}

// Runs: python3 scripts/scholarly_fetch.py <AUTHOR_ID>
// Returns parsed JSON from the script.
func FetchScholarOnce(authorID string) ([]ScholarPub, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"/root/fundproject/fund-management-api/venv/bin/python",
		"/root/fundproject/fund-management-api/scripts/scholarly_fetch.py",
		authorID,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var pubs []ScholarPub
	if err := json.Unmarshal(out, &pubs); err != nil {
		return nil, err
	}
	return pubs, nil
}
