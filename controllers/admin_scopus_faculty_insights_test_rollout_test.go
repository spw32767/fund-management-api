//go:build insight_test_rollout

package controllers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"fund-management-api/config"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type rolloutFacultyLog struct {
	logger.Interface
	enabled      bool
	statements   int
	milliseconds float64
	write        bool
}

func (l *rolloutFacultyLog) Trace(_ context.Context, start time.Time, fc func() (string, int64), _ error) {
	if !l.enabled {
		return
	}
	query, _ := fc()
	l.statements++
	l.milliseconds += float64(time.Since(start).Microseconds()) / 1000
	operation := strings.ToUpper(strings.Fields(query)[0])
	if operation != "SELECT" && operation != "SHOW" {
		l.write = true
	}
}
func (l *rolloutFacultyLog) reset() {
	l.enabled = true
	l.statements = 0
	l.milliseconds = 0
	l.write = false
}

func TestConfiguredTestFacultyInsightsReadOnly(t *testing.T) {
	if os.Getenv("SCOPUS_TEST_ROLLOUT_VERIFY") != "authorized-test-read-only" {
		t.Skip("explicit configured TEST verification not requested")
	}
	if err := godotenv.Load(filepath.Join("..", ".env")); err != nil {
		t.Fatal("configured TEST environment unavailable")
	}
	fingerprint := sha256.Sum256([]byte(os.Getenv("DB_HOST") + ":" + os.Getenv("DB_PORT")))
	if os.Getenv("ENVIRONMENT") != "development" || os.Getenv("DB_DATABASE") != "drnadech_fund_cpkku_intern" || hex.EncodeToString(fingerprint[:])[:12] != "473f44f70b3c" {
		t.Fatal("TEST identity differs from audited environment/database/endpoint")
	}
	c := driver.NewConfig()
	c.User = os.Getenv("DB_USERNAME")
	c.Passwd = os.Getenv("DB_PASSWORD")
	c.Net = "tcp"
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "3306"
	}
	c.Addr = os.Getenv("DB_HOST") + ":" + port
	c.DBName = os.Getenv("DB_DATABASE")
	c.ParseTime = true
	c.Loc = time.UTC
	c.Timeout = 10 * time.Second
	c.ReadTimeout = 45 * time.Second
	// Protect every audit connection, including replacements, with session READ ONLY.
	// @@session.tx_read_only alone does not describe a per-transaction override.
	c.Params = map[string]string{"tx_read_only": "1"}
	l := &rolloutFacultyLog{Interface: logger.Default.LogMode(logger.Silent)}
	db, err := gorm.Open(mysql.Open(c.FormatDSN()), &gorm.Config{Logger: l})
	if err != nil {
		t.Fatal("TEST database connection failed (identity withheld)")
	}
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	defer pool.Close()
	var identity struct{ Name, Version string }
	if err = db.Raw("SELECT DATABASE() name,VERSION() version").Scan(&identity).Error; err != nil || identity.Name != c.DBName || !strings.HasPrefix(identity.Version, "10.11.") || !strings.Contains(identity.Version, "MariaDB") {
		t.Fatal("connected TEST identity mismatch")
	}
	old := config.DB
	config.DB = db
	defer func() { config.DB = old }()
	gin.SetMode(gin.TestMode)
	tx := db.Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatal("read-only transaction failed")
	}
	var readOnly int
	if err = tx.Raw("SELECT @@session.tx_read_only").Scan(&readOnly).Error; err != nil || readOnly != 1 {
		t.Fatal("read-only mode not confirmed")
	}
	tx.Rollback()
	report := map[string]interface{}{"timestamp_utc": time.Now().UTC().Format(time.RFC3339), "database": identity.Name, "version": identity.Version, "read_only_confirmed": true, "no_server_started": true, "no_application_initialization": true}
	summaries := map[string]interface{}{}
	for _, sample := range []struct {
		name, query             string
		total, yes, no, unknown int
		roles                   [4]int
	}{
		{"faculty_all_years", "scope=faculty&open_access_mode=all", 743, 251, 489, 3, [4]int{145, 200, 398, 0}},
		{"faculty_BE2567_2569", "scope=faculty&open_access_mode=all&year_start_be=2567&year_end_be=2569", 226, 122, 104, 0, [4]int{20, 82, 124, 0}},
	} {
		w, s := rolloutFacultyRequest(t, AdminGetScopusFacultyInsights, sample.query)
		if w.Code != 200 {
			t.Fatalf("%s summary HTTP %d", sample.name, w.Code)
		}
		totals := s["totals"].(map[string]interface{})
		international := totals["international"].(map[string]interface{})
		roles := totals["roles"].(map[string]interface{})
		if totals["total"] != float64(sample.total) || international["yes"] != float64(sample.yes) || international["no"] != float64(sample.no) || international["unknown"] != float64(sample.unknown) {
			t.Fatalf("%s country/cohort parity mismatch", sample.name)
		}
		for index, role := range insightRoleStates {
			if roles[role] != float64(sample.roles[index]) {
				t.Fatalf("%s %s role mismatch", sample.name, role)
			}
		}
		revision := s["revision"].(string)
		if len(revision) != 64 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("revision/no-store contract missing")
		}
		seen := map[float64]bool{}
		for page := 1; page <= (sample.total+199)/200; page++ {
			w, d := rolloutFacultyRequest(t, AdminGetScopusFacultyInsightsDrilldown, fmt.Sprintf("%s&revision=%s&page=%d&page_size=200", sample.query, revision, page))
			if w.Code != 200 || d["total"] != float64(sample.total) || d["revision"] != revision {
				t.Fatal("real TEST summary/page parity")
			}
			for _, raw := range d["documents"].([]interface{}) {
				row := raw.(map[string]interface{})
				id := row["document_id"].(float64)
				if seen[id] {
					t.Fatal("duplicate TEST page ID")
				}
				seen[id] = true
			}
		}
		if len(seen) != sample.total {
			t.Fatal("TEST >200 population truncated")
		}
		t.Logf("%s totals/roles and %d unique paged documents passed; verifying year/partner dimensions", sample.name, len(seen))
		// Validate every actual year/status/role and every partner dimension.
		cells := 0
		for _, raw := range s["by_year"].([]interface{}) {
			year := raw.(map[string]interface{})
			for _, status := range insightInternationalStates {
				for _, role := range insightRoleStates {
					w, d := rolloutFacultyRequest(t, AdminGetScopusFacultyInsightsDrilldown, sample.query+"&revision="+revision+"&year_be="+year["bucket"].(string)+"&international_status="+status+"&faculty_role="+role)
					if w.Code != 200 || d["total"] != year["country_role"].(map[string]interface{})[status].(map[string]interface{})[role] {
						t.Fatal("TEST cross-tab drilldown mismatch")
					}
					cells++
				}
			}
		}
		for _, raw := range totals["partners"].([]interface{}) {
			partner := raw.(map[string]interface{})
			w, d := rolloutFacultyRequest(t, AdminGetScopusFacultyInsightsDrilldown, sample.query+"&revision="+revision+"&international_status=yes&country_key="+partner["country_key"].(string))
			if w.Code != 200 || d["total"] != partner["documents"] {
				t.Fatal("TEST partner parity mismatch")
			}
		}
		summaries[sample.name] = map[string]interface{}{"summary": s, "unique_paged_documents": len(seen), "cross_tab_cells_verified": cells, "partner_counts_verified": len(totals["partners"].([]interface{}))}
		t.Logf("%s: %d cross-tab cells and %d partner counts passed", sample.name, cells, len(totals["partners"].([]interface{})))
	}
	report["summaries"] = summaries
	_, baseline := rolloutFacultyRequest(t, AdminGetScopusFacultyInsights, "scope=faculty&open_access_mode=all")
	revision := baseline["revision"].(string)
	w, b := rolloutFacultyRequest(t, AdminGetScopusFacultyInsightsDrilldown, "scope=faculty&open_access_mode=all&revision="+strings.Repeat("0", 64))
	if w.Code != 409 || b["documents"] != nil {
		t.Fatal("TEST stale revision must return409 without documents")
	}
	report["stale_revision_409_without_documents"] = true
	w, b = rolloutFacultyRequest(t, AdminGetScopusFacultyInsights, "scope=faculty&open_access_mode=all&year_start_be=2999&year_end_be=2999")
	if w.Code != 200 || b["totals"].(map[string]interface{})["total"] != float64(0) {
		t.Fatal("TEST zero cohort mismatch")
	}
	report["zero_response"] = b
	costs := []map[string]interface{}{}
	for _, filter := range []string{"scope=faculty&open_access_mode=all", "scope=faculty&open_access_mode=all&year_start_be=2567&year_end_be=2569", "scope=faculty&open_access_mode=all&aggregation_types=Journal&quality_buckets=T1,Q1,Q2,Q3,Q4&year_start_be=2568&year_end_be=2569"} {
		_, s := rolloutFacultyRequest(t, AdminGetScopusFacultyInsights, filter)
		rev := s["revision"].(string)
		for _, endpoint := range []struct {
			name    string
			handler gin.HandlerFunc
			query   string
		}{{"summary", AdminGetScopusFacultyInsights, filter}, {"drilldown_200", AdminGetScopusFacultyInsightsDrilldown, filter + "&revision=" + rev + "&page_size=200"}} {
			times := []float64{}
			statements := []int{}
			sqlTimes := []float64{}
			bytes := []int{}
			for n := 0; n < 3; n++ {
				l.reset()
				start := time.Now()
				w, _ := rolloutFacultyRequest(t, endpoint.handler, endpoint.query)
				times = append(times, float64(time.Since(start).Microseconds())/1000)
				if w.Code != 200 || l.write {
					t.Fatal("TEST read-only measured request failed")
				}
				statements = append(statements, l.statements)
				sqlTimes = append(sqlTimes, l.milliseconds)
				bytes = append(bytes, w.Body.Len())
				l.enabled = false
			}
			sort.Float64s(times)
			costs = append(costs, map[string]interface{}{"endpoint": endpoint.name, "query": filter, "documents": s["totals"].(map[string]interface{})["total"], "iterations": 3, "median_ms": times[1], "min_ms": times[0], "max_ms": times[2], "sql_statements_by_request": statements, "sql_ms_by_request": sqlTimes, "response_bytes_by_request": bytes})
		}
	}
	report["request_costs"] = costs
	report["measurement_scope"] = "current configured TEST database over network, actual handler/query/JSON work; SQL instrumentation and test decoding included; HTTP/auth/browser transport excluded; serial warm samples, no production SLA"
	report["revision_after_reads_unchanged"] = false
	_, after := rolloutFacultyRequest(t, AdminGetScopusFacultyInsights, "scope=faculty&open_access_mode=all")
	if after["revision"] != revision {
		t.Fatal("TEST revision changed during read-only verification")
	}
	report["revision_after_reads_unchanged"] = true
	report["passed"] = true
	output := os.Getenv("SCOPUS_TEST_ROLLOUT_REPORT")
	if output == "" {
		t.Fatal("explicit sanitized report path required")
	}
	data, _ := json.MarshalIndent(report, "", "  ")
	if err = os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("Actual TEST handlers passed 743/226 cohort parity, >200 paging, every country/role cell and partner,409/zero/read-only checks; sanitized costs saved")
}

func rolloutFacultyRequest(t *testing.T, handler gin.HandlerFunc, query string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	params, err := url.ParseQuery(query)
	if err != nil {
		t.Fatal("invalid verification query")
	}
	c.Request = httptest.NewRequest("GET", "/?"+params.Encode(), nil)
	handler(c)
	var body map[string]interface{}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil {
		t.Fatal("TEST handler returned non-JSON")
	}
	return w, body
}
