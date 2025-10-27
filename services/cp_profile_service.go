package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CpProfileImportSummary struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Failed  int `json:"failed"`
}

type CpProfileService struct{ db *gorm.DB }

func NewCpProfileService(db *gorm.DB) *CpProfileService {
	if db == nil {
		db = config.DB
	}
	return &CpProfileService{db: db}
}

// Import: run python script -> read kku_people.json -> upsert into cp_profile
func (s *CpProfileService) Import(ctx context.Context, debug bool) (*CpProfileImportSummary, error) {
	py := firstNonEmpty(os.Getenv("VENV_PY"), "python3") // เหมือน scholar
	script := firstNonEmpty(os.Getenv("CP_PROFILE_SCRIPT"), "scripts/scrape_kku_people.py")

	args := []string{script}
	if debug {
		args = append(args, "--debug")
	}
	cmd := exec.CommandContext(ctx, py, args...)
	cmd.Env = append(os.Environ(), "HEADLESS=1") // ให้รัน headless ใน server
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[cp_profile] script error: %v\n--- output ---\n%s\n--------------", err, string(out))
		return nil, fmt.Errorf("run python failed: %w", err)
	}

	// หาไฟล์ JSON อยู่ข้างๆสคริปต์ (ตรงกับที่เราเขียนใน .py)
	jsonPath := filepath.Join(filepath.Dir(script), "kku_people.json")
	if _, err := os.Stat(jsonPath); err != nil {
		return nil, fmt.Errorf("json not found at %s: %w", jsonPath, err)
	}

	// stream decode เพื่อลด memory
	f, err := os.Open(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("open json: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var people []models.CpProfile
	if err := dec.Decode(&people); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	if len(people) == 0 {
		return nil, errors.New("no records in kku_people.json")
	}

	sum := &CpProfileImportSummary{Total: len(people)}

	// upsert โดยใช้ unique(profile_url)
	for _, p := range people {
		res := s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "profile_url"}},
			DoUpdates: clause.AssignmentColumns([]string{"name_th", "name_en", "position", "email", "photo_url", "info", "education"}),
		}).Create(&p)

		if res.Error != nil {
			sum.Failed++
			log.Printf("[cp_profile] upsert failed (%s): %v", p.ProfileURL, res.Error)
			continue
		}
		// MySQL: insert then on-dup-update จะได้ RowsAffected=2 กรณีมีการ update
		if res.RowsAffected == 1 {
			sum.Created++
		} else {
			sum.Updated++
		}
	}

	return sum, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
