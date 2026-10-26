package controllers

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"fund-management-api/config"
	"github.com/gin-gonic/gin"
)

func activeYearStep(rows ...[]driver.Value) *queryStep {
	return &queryStep{kind: stepQuery, pattern: regexp.MustCompile(`(?s)SELECT y.year_id, y.year, y.status.*SELECT current_year FROM system_config.*y.status = 'active'`), columns: []string{"year_id", "year", "status"}, rows: rows}
}

func readinessStep(values ...driver.Value) *queryStep {
	return &queryStep{kind: stepQuery, pattern: regexp.MustCompile(`(?s)SELECT.*AS open_funds.*AS missing_budgets.*AS publication_funds.*AS reward_rates.*AS fee_limits.*AS installments`),
		args:    []driver.Value{int64(10), int64(10), int64(10), "2570", "2570", int64(10)},
		columns: []string{"open_funds", "missing_budgets", "publication_funds", "reward_rates", "fee_limits", "installments"}, rows: [][]driver.Value{values}}
}

func serveBudgetYearTest(t *testing.T, steps []*queryStep, method, path, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	original := config.DB
	config.DB = db
	defer func() { config.DB = original }()
	r := gin.New()
	r.Handle(method, "/test", func(c *gin.Context) { c.Set("userID", 42); c.Set("roleID", 1); handler(c) })
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(response, req)
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestActiveBudgetYearUsesOnlyConfiguredYear(t *testing.T) {
	response := serveBudgetYearTest(t, []*queryStep{activeYearStep([]driver.Value{int64(10), "2570", "active"})}, "GET", "/test", "", GetSystemConfigCurrentYear)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"year_id":10`) || !strings.Contains(response.Body.String(), `"current_year":"2570"`) {
		t.Fatal(response.Code, response.Body.String())
	}
	response = serveBudgetYearTest(t, []*queryStep{activeYearStep()}, "GET", "/test", "", GetSystemConfigCurrentYear)
	if response.Code != 503 {
		t.Fatal("must not guess a calendar/latest year:", response.Body.String())
	}
}

func TestBudgetYearActivationRechecksReadinessBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		totals  []driver.Value
		status  int
		dbError bool
	}{
		{"ready", []driver.Value{int64(2), int64(0), int64(1), int64(8), int64(4), int64(3)}, 200, false},
		{"missing budgets", []driver.Value{int64(2), int64(1), int64(1), int64(8), int64(4), int64(3)}, 422, false},
		{"missing rates", []driver.Value{int64(2), int64(0), int64(1), int64(0), int64(4), int64(3)}, 422, false},
		{"missing fees", []driver.Value{int64(2), int64(0), int64(1), int64(8), int64(0), int64(3)}, 422, false},
		{"missing periods", []driver.Value{int64(2), int64(0), int64(1), int64(8), int64(4), int64(0)}, 422, false},
		{"research only", []driver.Value{int64(1), int64(0), int64(0), int64(0), int64(0), int64(3)}, 200, false},
		{"database failure", nil, 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := []*queryStep{
				{kind: stepQuery, pattern: regexp.MustCompile(`(?s)SELECT config_id, current_year, max_submissions_per_year.*FOR UPDATE`), columns: []string{"config_id", "current_year", "max_submissions_per_year"}, rows: [][]driver.Value{{int64(7), "2569", int64(5)}}},
				{kind: stepQuery, pattern: regexp.MustCompile("SELECT year_id, year, status FROM `years`"), args: []driver.Value{int64(10), int64(1)}, columns: []string{"year_id", "year", "status"}, rows: [][]driver.Value{{int64(10), "2570", "active"}}},
			}
			check := readinessStep(tc.totals...)
			if tc.dbError {
				check.err = errors.New("unavailable")
			}
			steps = append(steps, check)
			if tc.status == 200 {
				steps = append(steps, &queryStep{kind: stepExec, pattern: regexp.MustCompile(`UPDATE system_config SET current_year = \?`), args: []driver.Value{"2570", nil, nil, nil, int64(5), int64(42), int64(7)}})
			}
			response := serveBudgetYearTest(t, steps, "PUT", "/test", `{"current_year_id":10,"current_year":"2570"}`, UpdateSystemConfigWindow)
			if response.Code != tc.status {
				t.Fatal(response.Code, response.Body.String())
			}
		})
	}
}

func TestNewSubmissionRejectsStaleYearBeforeCreating(t *testing.T) {
	response := serveBudgetYearTest(t, []*queryStep{activeYearStep([]driver.Value{int64(10), "2570", "active"})}, http.MethodPost, "/test", `{"submission_type":"publication_reward","year_id":9}`, CreateSubmission)
	if response.Code != 409 || !strings.Contains(response.Body.String(), `"code":"budget_year_changed"`) {
		t.Fatal(response.Code, response.Body.String())
	}
}

func TestBudgetYearReadinessPreviewDoesNotActivate(t *testing.T) {
	steps := []*queryStep{
		{kind: stepQuery, pattern: regexp.MustCompile("SELECT year_id, year, status FROM `years`"), args: []driver.Value{"2570", int64(1)}, columns: []string{"year_id", "year", "status"}, rows: [][]driver.Value{{int64(10), "2570", "inactive"}}},
		readinessStep(int64(1), int64(0), int64(0), int64(0), int64(0), int64(2)),
	}
	response := serveBudgetYearTest(t, steps, "GET", "/test?year=2570", "", GetBudgetYearReadiness)
	var result budgetYearReadiness
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || result.Ready || result.Checks[0].Passed {
		t.Fatal(response.Code, response.Body.String())
	}
}

func TestSavingCurrentYearSettingsDoesNotRequirePreparingAnotherYear(t *testing.T) {
	steps := []*queryStep{
		{kind: stepQuery, pattern: regexp.MustCompile(`(?s)SELECT config_id, current_year, max_submissions_per_year.*FOR UPDATE`), columns: []string{"config_id", "current_year", "max_submissions_per_year"}, rows: [][]driver.Value{{int64(7), "2570", int64(5)}}},
		{kind: stepQuery, pattern: regexp.MustCompile("SELECT year_id, year, status FROM `years`"), args: []driver.Value{"2570", int64(1)}, columns: []string{"year_id", "year", "status"}, rows: [][]driver.Value{{int64(10), "2570", "active"}}},
		{kind: stepExec, pattern: regexp.MustCompile(`UPDATE system_config SET current_year = \?`), args: []driver.Value{"2570", nil, nil, "contact", int64(6), int64(42), int64(7)}},
	}
	response := serveBudgetYearTest(t, steps, "PUT", "/test", `{"current_year":"2570","contact_info":"contact","max_submissions_per_year":6}`, UpdateSystemConfigWindow)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
}

func TestSubmissionUsageFollowsTheSameActiveYearAsForms(t *testing.T) {
	steps := []*queryStep{
		{kind: stepQuery, pattern: regexp.MustCompile(`SELECT max_submissions_per_year, current_year`), columns: []string{"max_submissions_per_year", "current_year"}, rows: [][]driver.Value{{int64(5), "2570"}}},
		activeYearStep([]driver.Value{int64(10), "2570", "active"}),
		{kind: stepQuery, pattern: regexp.MustCompile(`(?s)SELECT COUNT\(\*\).*s.year_id = \?.*st.status_code NOT IN`), args: []driver.Value{int64(42), int64(10), "2", "4"}, columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
	}
	response := serveBudgetYearTest(t, steps, "GET", "/test", "", GetSubmissionUsageLimit)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"year":"2570"`) || !strings.Contains(response.Body.String(), `"over_limit":false`) {
		t.Fatal(response.Code, response.Body.String())
	}
}
