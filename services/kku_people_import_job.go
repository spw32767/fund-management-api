package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrKkuPeopleImportAlreadyRunning = errors.New("kku people import already running")
)

type KkuPeopleImportSummary struct {
	FetchedCount int  `json:"fetched_count"`
	CreatedCount int  `json:"created_count"`
	UpdatedCount int  `json:"updated_count"`
	FailedCount  int  `json:"failed_count"`
	DryRun       bool `json:"dry_run"`
}

type profileLinkStats struct {
	ByEmail      int64
	ByProfileURL int64
}

type KkuPeopleImportInput struct {
	DryRun        bool
	Debug         bool
	TriggerSource string
	LockName      string
	RecordRun     bool
}

type KkuPeopleImportJobService struct {
	db        *gorm.DB
	runSvc    *KkuPeopleImportRunService
	cpProfile *CpProfileRepository
}

// ScriptExecutionError represents a failure when running the scraper script.
type ScriptExecutionError struct {
	Err      error
	ExitCode *int
	Stdout   []byte
	Stderr   []byte
}

func (e *ScriptExecutionError) Error() string {
	if e == nil {
		return ""
	}
	if e.ExitCode != nil {
		return fmt.Sprintf("script failed with exit code %d: %v", *e.ExitCode, e.Err)
	}
	return fmt.Sprintf("script failed: %v", e.Err)
}

func (e *ScriptExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type rawKkuPerson struct {
	NameTh     string `json:"name_th"`
	NameEn     string `json:"name_en"`
	Position   string `json:"position"`
	Email      string `json:"email"`
	PhotoURL   string `json:"photo_url"`
	Info       string `json:"info"`
	Education  string `json:"education"`
	ProfileURL string `json:"profile_url"`
}

type CpProfileRepository struct {
	db *gorm.DB
}

func NewCpProfileRepository(db *gorm.DB) *CpProfileRepository {
	if db == nil {
		db = config.DB
	}
	return &CpProfileRepository{db: db}
}

func (r *CpProfileRepository) Upsert(ctx context.Context, profile *models.CpProfile) error {
	if profile == nil {
		return errors.New("profile is nil")
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "profile_url"}},
		DoUpdates: clause.AssignmentColumns([]string{"name_th", "name_en", "position", "email", "photo_url", "info", "education"}),
	}).Create(profile).Error
}

func NewKkuPeopleImportJobService(db *gorm.DB) *KkuPeopleImportJobService {
	if db == nil {
		db = config.DB
	}
	return &KkuPeopleImportJobService{
		db:        db,
		runSvc:    NewKkuPeopleImportRunService(db),
		cpProfile: NewCpProfileRepository(db),
	}
}

func (s *KkuPeopleImportJobService) Run(ctx context.Context, input *KkuPeopleImportInput) (*KkuPeopleImportSummary, *models.KkuPeopleImportRun, error) {
	if input == nil {
		return nil, nil, errors.New("input is nil")
	}
	summary := &KkuPeopleImportSummary{DryRun: input.DryRun}

	lockName := strings.TrimSpace(input.LockName)
	if lockName == "" {
		lockName = "kku_people_import_job"
	}

	release, err := s.acquireLock(ctx, lockName)
	if err != nil {
		return nil, nil, err
	}
	if release != nil {
		defer func() {
			if relErr := release(); relErr != nil {
				log.Printf("failed to release kku people import lock: %v", relErr)
			}
		}()
	}

	trigger := strings.TrimSpace(input.TriggerSource)
	if trigger == "" {
		trigger = "admin_api"
	}

	var run *models.KkuPeopleImportRun
	if input.RecordRun {
		run, err = s.runSvc.Start(trigger, input.DryRun)
		if err != nil {
			return nil, nil, err
		}
	}

	startTime := time.Now()
	var exitCode *int
	var stdoutBuf, stderrBuf []byte
	var finalErr error
	if run != nil {
		startTime = run.StartedAt
	}

	people, stdout, stderr, code, scriptErr := s.executeScript(ctx, input.Debug)
	stdoutBuf = stdout
	stderrBuf = stderr
	if code != nil {
		exitCode = code
	}

	if scriptErr != nil {
		finalErr = scriptErr
	} else {
		if len(people) == 0 {
			finalErr = errors.New("no records in script output")
		} else {
			procErr := s.processPeople(ctx, people, summary, input.DryRun)
			if procErr != nil {
				finalErr = procErr
			} else if !input.DryRun {
				if stats, linkErr := s.reconcileProfiles(ctx); linkErr != nil {
					finalErr = linkErr
				} else if stats != nil {
					log.Printf("linked cp_profile rows: %d by email, %d by profile url", stats.ByEmail, stats.ByProfileURL)
				}
			}
		}
	}

	duration := time.Since(startTime).Seconds()
	if run != nil {
		defer func() {
			var markErr error
			if finalErr != nil {
				markErr = s.runSvc.MarkFailure(run.ID, summary, exitCode, string(stdoutBuf), string(stderrBuf), finalErr, duration)
			} else {
				markErr = s.runSvc.MarkSuccess(run.ID, summary, exitCode, string(stdoutBuf), string(stderrBuf), duration)
			}
			if markErr != nil {
				log.Printf("failed to mark kku people import run status: %v", markErr)
			}
		}()
	}

	if finalErr != nil {
		if run != nil {
			if updated, err := s.runSvc.GetByID(run.ID); err == nil {
				run = updated
			}
		}
		return summary, run, finalErr
	}

	if run != nil {
		if updated, err := s.runSvc.GetByID(run.ID); err == nil {
			run = updated
		}
	}
	return summary, run, nil
}

func (s *KkuPeopleImportJobService) executeScript(ctx context.Context, debug bool) ([]rawKkuPerson, []byte, []byte, *int, error) {
	py := firstNonEmpty(os.Getenv("VENV_PY"), "python3")
	script := firstNonEmpty(os.Getenv("KKU_PEOPLE_SCRIPT"), os.Getenv("CP_PROFILE_SCRIPT"), "scripts/scrape_kku_people.py")
	if !filepath.IsAbs(script) {
		if wd, err := os.Getwd(); err == nil {
			script = filepath.Join(wd, script)
		}
	}

	args := []string{script}
	if debug {
		args = append(args, "--debug")
	}

	cmd := exec.CommandContext(ctx, py, args...)
	cmd.Env = append(os.Environ(), "HEADLESS=1")

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, nil, nil, err
	}

	stdoutBytes, err := io.ReadAll(stdoutPipe)
	if err != nil {
		_ = cmd.Wait()
		return nil, stdoutBytes, nil, nil, err
	}
	stderrBytes, err := io.ReadAll(stderrPipe)
	if err != nil {
		_ = cmd.Wait()
		return nil, stdoutBytes, stderrBytes, nil, err
	}

	waitErr := cmd.Wait()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	exitPtr := &exitCode

	trimmed := bytes.TrimSpace(bytes.TrimPrefix(stdoutBytes, []byte{0xEF, 0xBB, 0xBF}))
	if len(trimmed) == 0 {
		if waitErr != nil {
			return nil, stdoutBytes, stderrBytes, exitPtr, &ScriptExecutionError{Err: waitErr, ExitCode: exitPtr, Stdout: stdoutBytes, Stderr: stderrBytes}
		}
		return nil, stdoutBytes, stderrBytes, exitPtr, errors.New("empty output from script")
	}

	var people []rawKkuPerson
	if err := json.Unmarshal(trimmed, &people); err != nil {
		return nil, stdoutBytes, stderrBytes, exitPtr, fmt.Errorf("parse json: %w", err)
	}

	if waitErr != nil {
		return people, stdoutBytes, stderrBytes, exitPtr, &ScriptExecutionError{Err: waitErr, ExitCode: exitPtr, Stdout: stdoutBytes, Stderr: stderrBytes}
	}

	return people, stdoutBytes, stderrBytes, exitPtr, nil
}

func (s *KkuPeopleImportJobService) processPeople(ctx context.Context, people []rawKkuPerson, summary *KkuPeopleImportSummary, dryRun bool) error {
	if summary == nil {
		summary = &KkuPeopleImportSummary{}
	}
	summary.FetchedCount = len(people)

	seen := make(map[string]struct{})
	urls := make([]string, 0, len(people))
	normalized := make([]rawKkuPerson, 0, len(people))
	for _, p := range people {
		url := strings.TrimSpace(p.ProfileURL)
		if url == "" {
			summary.FailedCount++
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		urls = append(urls, url)
		normalized = append(normalized, rawKkuPerson{
			NameTh:     strings.TrimSpace(p.NameTh),
			NameEn:     strings.TrimSpace(p.NameEn),
			Position:   strings.TrimSpace(p.Position),
			Email:      strings.TrimSpace(p.Email),
			PhotoURL:   strings.TrimSpace(p.PhotoURL),
			Info:       strings.TrimSpace(p.Info),
			Education:  strings.TrimSpace(p.Education),
			ProfileURL: url,
		})
	}

	if len(normalized) == 0 {
		return errors.New("no valid profiles in script output")
	}

	existing := make(map[string]models.CpProfile)
	var rows []models.CpProfile
	if err := s.db.WithContext(ctx).Where("profile_url IN ?", urls).Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		existing[strings.TrimSpace(r.ProfileURL)] = r
	}

	for _, person := range normalized {
		incoming := &models.CpProfile{
			NameTh:     person.NameTh,
			NameEn:     person.NameEn,
			Position:   person.Position,
			Email:      person.Email,
			PhotoURL:   person.PhotoURL,
			Info:       person.Info,
			Education:  person.Education,
			ProfileURL: person.ProfileURL,
		}

		if current, ok := existing[person.ProfileURL]; !ok {
			if dryRun {
				summary.CreatedCount++
				continue
			}
			if err := s.cpProfile.Upsert(ctx, incoming); err != nil {
				summary.FailedCount++
				log.Printf("failed to insert cp_profile (%s): %v", person.ProfileURL, err)
				continue
			}
			summary.CreatedCount++
		} else if !profilesEqual(&current, incoming) {
			if dryRun {
				summary.UpdatedCount++
				continue
			}
			if err := s.cpProfile.Upsert(ctx, incoming); err != nil {
				summary.FailedCount++
				log.Printf("failed to update cp_profile (%s): %v", person.ProfileURL, err)
				continue
			}
			summary.UpdatedCount++
		}
	}

	return nil
}

func (s *KkuPeopleImportJobService) reconcileProfiles(ctx context.Context) (*profileLinkStats, error) {
	stats := &profileLinkStats{}

	emailUpdate := s.db.WithContext(ctx).Exec(`
                UPDATE cp_profile p
                JOIN users u ON u.email IS NOT NULL AND u.email <> ''
                        AND p.email IS NOT NULL AND p.email <> ''
                        AND LOWER(TRIM(u.email)) = LOWER(TRIM(p.email))
                SET p.user_id = u.user_id
                WHERE p.user_id IS NULL
        `)
	if emailUpdate.Error != nil {
		return nil, emailUpdate.Error
	}
	stats.ByEmail = emailUpdate.RowsAffected

	normalize := func(column string) string {
		expr := fmt.Sprintf("LOWER(COALESCE(%s, ''))", column)
		for _, pattern := range []string{"https://", "http://", "www.", "computing.kku.ac.th/", "computing.kku.ac.th"} {
			expr = fmt.Sprintf("REPLACE(%s, '%s', '')", expr, pattern)
		}
		return fmt.Sprintf("TRIM(BOTH '/' FROM %s)", expr)
	}

	profileUpdate := s.db.WithContext(ctx).Exec(fmt.Sprintf(`
                UPDATE cp_profile p
                JOIN users u ON u.CP_WEB_ID IS NOT NULL AND u.CP_WEB_ID <> ''
                        AND p.profile_url IS NOT NULL AND p.profile_url <> ''
                        AND %s <> '' AND %s <> ''
                        AND %s = %s
                SET p.user_id = u.user_id
                WHERE p.user_id IS NULL
        `, normalize("u.CP_WEB_ID"), normalize("p.profile_url"), normalize("u.CP_WEB_ID"), normalize("p.profile_url")))
	if profileUpdate.Error != nil {
		return nil, profileUpdate.Error
	}
	stats.ByProfileURL = profileUpdate.RowsAffected

	return stats, nil
}

func profilesEqual(a *models.CpProfile, b *models.CpProfile) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(a.NameTh) == strings.TrimSpace(b.NameTh) &&
		strings.TrimSpace(a.NameEn) == strings.TrimSpace(b.NameEn) &&
		strings.TrimSpace(a.Position) == strings.TrimSpace(b.Position) &&
		strings.TrimSpace(a.Email) == strings.TrimSpace(b.Email) &&
		strings.TrimSpace(a.PhotoURL) == strings.TrimSpace(b.PhotoURL) &&
		strings.TrimSpace(a.Info) == strings.TrimSpace(b.Info) &&
		strings.TrimSpace(a.Education) == strings.TrimSpace(b.Education)
}

func (s *KkuPeopleImportJobService) acquireLock(ctx context.Context, lockName string) (func() error, error) {
	if strings.TrimSpace(lockName) == "" {
		return nil, nil
	}

	lockCtx := persistentContext(ctx)

	var ok int
	if err := s.db.WithContext(lockCtx).Raw("SELECT GET_LOCK(?, 0)", lockName).Scan(&ok).Error; err != nil {
		return nil, err
	}
	if ok != 1 {
		return nil, ErrKkuPeopleImportAlreadyRunning
	}

	return func() error {
		var released int
		if err := s.db.WithContext(lockCtx).Raw("SELECT RELEASE_LOCK(?)", lockName).Scan(&released).Error; err != nil {
			return err
		}
		if released != 1 {
			return fmt.Errorf("release lock %q returned %d", lockName, released)
		}
		return nil
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
