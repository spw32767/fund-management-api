package controllers

import (
	"database/sql/driver"
	"errors"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"github.com/gin-gonic/gin"
)

func TestShiftBudgetYearCutoff(t *testing.T) {
	for _, tc := range []struct {
		value string
		delta int
		want  string
	}{
		{"2026-09-30T23:00:00+07:00", 1, "2027-09-30T23:00:00+07:00"},
		{"2025-10-01T09:00:00+07:00", 2, "2027-10-01T09:00:00+07:00"},
		{"2024-02-29T23:59:00+07:00", 1, "2025-02-28T23:59:00+07:00"},
		{"2027-01-31T10:00:00+07:00", -1, "2026-01-31T10:00:00+07:00"},
	} {
		value, _ := time.Parse(time.RFC3339, tc.value)
		if got := shiftBudgetYearCutoff(value, tc.delta).Format(time.RFC3339); got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.value, got, tc.want)
		}
	}
}

func TestCopyBudgetYearExtrasPreservesValuesAndChangesYear(t *testing.T) {
	now := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC)
	steps := []*queryStep{
		{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*publication_reward_rates"), args: []driver.Value{"2569"}, columns: []string{"rate_id", "year", "author_status", "journal_quartile", "reward_amount", "is_active"}, rows: [][]driver.Value{{int64(1), "2569", "first_author", "Q1", float64(5000), true}}},
		{kind: stepExec, pattern: regexp.MustCompile("INSERT INTO `publication_reward_rates`"), args: []driver.Value{"2570", "first_author", "Q1", float64(5000), true, now, now}},
		{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*reward_config.*delete_at IS NULL"), args: []driver.Value{"2569"}, columns: []string{"config_id", "year", "journal_quartile", "max_amount", "condition_description", "is_active"}, rows: [][]driver.Value{{int64(2), "2569", "Q1", float64(10000), "condition", false}}},
		{kind: stepExec, pattern: regexp.MustCompile("INSERT INTO `reward_config`"), args: []driver.Value{"2570", "Q1", float64(10000), "condition", false, now, now, nil}},
		{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*fund_installment_periods.*deleted_at IS NULL"), args: []driver.Value{int64(9)}, columns: []string{"installment_period_id", "year_id", "fund_level", "fund_keyword", "installment_number", "cutoff_date", "status"}, rows: [][]driver.Value{{int64(3), int64(9), "category", "Research", int64(2), cutoff, "active"}}},
		{kind: stepExec, pattern: regexp.MustCompile("INSERT INTO `fund_installment_periods`"), args: []driver.Value{"category", "Research", nil, int64(10), int64(2), shiftBudgetYearCutoff(cutoff, 1), nil, "active", nil, now, now, nil}},
	}
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	counts, err := copyBudgetYearExtras(db, models.Year{YearID: 9, Year: "2569"}, models.Year{YearID: 10, Year: "2570"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if counts != (budgetYearExtraCopyCounts{RewardRates: 1, FeeLimits: 1, Installments: 1}) {
		t.Fatal(counts)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}

func TestWholeBudgetYearCopyCommitsOrRollsBackAsOneBundle(t *testing.T) {
	for _, scenario := range []struct{ fail, newTarget bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		fail, newTarget := scenario.fail, scenario.newTarget
		prefix := "existing/"
		if newTarget {
			prefix = "new/"
		}
		t.Run(prefix+map[bool]string{false: "success", true: "fee copy fails"}[fail], func(t *testing.T) {
			steps := []*queryStep{
				{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*years.*year_id ="), columns: []string{"year_id", "year", "status"}, rows: [][]driver.Value{{int64(9), "2569", "active"}}},
				{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*years.*year_id ="), columns: []string{"year_id", "year", "status"}, rows: [][]driver.Value{{int64(10), "2570", "active"}}},
				{kind: stepQuery, pattern: regexp.MustCompile("SELECT year_id FROM years.*FOR UPDATE"), args: []driver.Value{int64(10)}, columns: []string{"year_id"}, rows: [][]driver.Value{{int64(10)}}},
				{kind: stepQuery, pattern: regexp.MustCompile("(?s)SELECT.*COUNT.*fund_categories.*submissions"), args: []driver.Value{int64(10), int64(10), "2570", "2570", int64(10), int64(10)}, columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
				{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*fund_categories"), columns: []string{"category_id"}},
				{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*publication_reward_rates"), columns: []string{"rate_id", "year", "author_status", "journal_quartile", "reward_amount", "is_active"}, rows: [][]driver.Value{{int64(1), "2569", "first_author", "Q1", float64(5000), true}}},
				{kind: stepExec, pattern: regexp.MustCompile("INSERT INTO `publication_reward_rates`")},
			}
			if newTarget {
				steps[1].pattern = regexp.MustCompile("SELECT .*years.*year =")
				steps[1].rows = nil
				createStep := &queryStep{kind: stepExec, pattern: regexp.MustCompile("INSERT INTO `years`"), result: scriptedResult{lastInsertID: 10, rowsAffected: 1}}
				steps = append(steps[:2], append([]*queryStep{createStep}, steps[2:]...)...)
			}
			feeStep := &queryStep{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*reward_config"), columns: []string{"config_id"}}
			if fail {
				feeStep.err = errors.New("test fee read failed")
			}
			steps = append(steps, feeStep)
			if !fail {
				steps = append(steps, &queryStep{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*fund_installment_periods"), columns: []string{"installment_period_id"}})
			}
			db, state, cleanup := newScriptedGormDB(t, steps)
			defer cleanup()
			original := config.DB
			config.DB = db
			defer func() { config.DB = original }()
			r := gin.New()
			r.POST("/copy", func(c *gin.Context) { c.Set("roleID", 3); CopyFundConfigurationToYear(c) })
			response := httptest.NewRecorder()
			body := `{"source_year_id":9,"target_year_id":10,"copy_all":true}`
			if newTarget {
				body = `{"source_year_id":9,"target_year":"2570","copy_all":true}`
			}
			req := httptest.NewRequest("POST", "/copy", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(response, req)
			if fail {
				if response.Code != 500 || state.commits != 0 || state.rollbacks != 1 {
					t.Fatal(response.Code, state.commits, state.rollbacks, response.Body.String())
				}
			} else if response.Code != 200 || state.commits != 1 || state.rollbacks != 0 || !strings.Contains(response.Body.String(), `"reward_rates":1`) {
				t.Fatal(response.Code, state.commits, state.rollbacks, response.Body.String())
			}
			if err := state.verifyComplete(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWholeBudgetYearCopyRejectsPopulatedTarget(t *testing.T) {
	steps := []*queryStep{
		{kind: stepQuery, pattern: regexp.MustCompile("SELECT year_id FROM years.*FOR UPDATE"), columns: []string{"year_id"}, rows: [][]driver.Value{{int64(10)}}},
		{kind: stepQuery, pattern: regexp.MustCompile("(?s)SELECT.*COUNT.*fund_categories.*submissions"), columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}},
	}
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	if err := ensureBudgetYearBundleEmpty(db, models.Year{YearID: 10, Year: "2570"}); !errors.Is(err, errBudgetYearBundleNotEmpty) {
		t.Fatal(err)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}
