package services

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"
)

type ScholarPub struct {
	Title            string         `json:"title"`
	Authors          []string       `json:"authors"`
	Venue            *string        `json:"venue"`
	Year             *int           `json:"year"`
	URL              *string        `json:"url"`
	DOI              *string        `json:"doi"`
	ScholarClusterID *string        `json:"scholar_cluster_id"`
	NumCitations     *int           `json:"num_citations"`
	CitedByURL       *string        `json:"citedby_url"`
	CitesPerYear     map[string]int `json:"cites_per_year"`
}

// Runs: python3 scripts/scholarly_fetch.py <AUTHOR_ID>
// Returns parsed JSON from the script.
func FetchScholarOnce(authorID string) ([]ScholarPub, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	py := os.Getenv("VENV_PY")
	if py == "" {
		py = "python3" // fallback
	}

	script := os.Getenv("SCHOLAR_SCRIPT")
	if script == "" {
		script = "scripts/scholarly_fetch.py"
	}

	cmd := exec.CommandContext(ctx, py, script, authorID)

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

type ScholarAuthorHit struct {
	AuthorID    string   `json:"author_id"`
	Name        string   `json:"name"`
	Affiliation *string  `json:"affiliation"`
	Interests   []string `json:"interests"`
	CitedBy     *int     `json:"citedby"`
	ProfileURL  *string  `json:"profile_url"`
}

func SearchScholarAuthors(query string) ([]ScholarAuthorHit, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	py := os.Getenv("VENV_PY")
	if py == "" {
		py = "python3"
	}

	// allow override via env; fallback to repo path
	script := os.Getenv("SCHOLAR_SEARCH_SCRIPT")
	if script == "" {
		script = "scripts/scholar_search_authors.py"
	}

	cmd := exec.CommandContext(ctx, py, script, query)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var hits []ScholarAuthorHit
	if e := json.Unmarshal(out, &hits); e != nil {
		return nil, e
	}
	return hits, nil
}
