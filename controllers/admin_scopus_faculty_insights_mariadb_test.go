//go:build insight_mariadb

package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"fund-management-api/config"
	"fund-management-api/middleware"
	"fund-management-api/models"
	"fund-management-api/services"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type nativeFacultyLog struct {
	logger.Interface
	mu           sync.Mutex
	enabled      bool
	statements   []string
	milliseconds float64
}

func (l *nativeFacultyLog) Trace(ctx context.Context, start time.Time, fc func() (string, int64), err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.enabled {
		query, _ := fc()
		l.statements = append(l.statements, query)
		l.milliseconds += float64(time.Since(start).Microseconds()) / 1000
	}
}
func (l *nativeFacultyLog) reset(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabled = enabled
	l.statements = nil
	l.milliseconds = 0
}
func (l *nativeFacultyLog) stats() (int, float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.statements), l.milliseconds
}

// NEVER reads application .env/DB_* configuration. It accepts an initially empty
// disposable schema on actual MariaDB 10.11 loopback, then removes its own tables.
func nativeFacultyDB(t *testing.T) (*gorm.DB, *nativeFacultyLog) {
	t.Helper()
	dsn := os.Getenv("SCOPUS_MARIADB_API_TEST_DSN")
	if dsn == "" {
		t.Skip("NATIVE API NOT RUN: SCOPUS_MARIADB_API_TEST_DSN unset")
	}
	c, err := driver.ParseDSN(dsn)
	if err != nil || c.Net != "tcp" || !strings.HasPrefix(c.Addr, "127.0.0.1:") || !strings.HasPrefix(c.DBName, "scopus_insights_test_") {
		t.Fatal("native API guard requires loopback TCP/disposable schema")
	}
	c.ParseTime = true
	c.Timeout = 5 * time.Second
	c.ReadTimeout = 20 * time.Second
	c.WriteTimeout = 20 * time.Second
	l := &nativeFacultyLog{Interface: logger.Default.LogMode(logger.Silent)}
	db, err := gorm.Open(mysql.Open(c.FormatDSN()), &gorm.Config{Logger: l})
	if err != nil {
		t.Fatal("native API connection failed (identity withheld)")
	}
	pool, _ := db.DB()
	pool.SetMaxOpenConns(12)
	t.Cleanup(func() { pool.Close() })
	var identity struct {
		Name, Version string
		Tables        int
	}
	if err = db.Raw("SELECT DATABASE() name,VERSION() version,(SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()) tables").Scan(&identity).Error; err != nil || identity.Name != c.DBName || identity.Tables != 0 || !strings.HasPrefix(identity.Version, "10.11.") || !strings.Contains(identity.Version, "MariaDB") {
		t.Fatal("native API requires MariaDB 10.11 and initially empty matching schema")
	}
	t.Log("Native API fixture server:", identity.Version)
	t.Cleanup(func() {
		for _, table := range []string{"user_sessions", "scopus_document_insights", "scopus_document_countries", "scopus_document_affiliations", "scopus_document_authors", "scopus_authors", "scopus_affiliations", "scopus_source_metrics", "scopus_country_catalogue_guard", "scopus_documents", "users"} {
			if err := db.Exec("DROP TABLE IF EXISTS `" + table + "`").Error; err != nil {
				t.Errorf("owned fixture cleanup: %s", table)
			}
		}
	})
	if err = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci").AutoMigrate(&models.ScopusDocument{}, &models.ScopusAuthor{}, &models.ScopusDocumentAuthor{}, &models.ScopusAffiliation{}, &models.ScopusSourceMetric{}); err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		`ALTER TABLE scopus_documents MODIFY raw_json JSON NULL`,
		`CREATE TABLE users(user_id INTEGER PRIMARY KEY,scopus_id VARCHAR(256),delete_at DATETIME,is_test INTEGER,role_id INTEGER) ENGINE=InnoDB`,
	} {
		if err = db.Exec(ddl).Error; err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join("..", "migrations", "050_20261005_scopus_core_insights.sql"))
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
		if strings.TrimSpace(stmt) != "" {
			if err = db.Exec(stmt).Error; err != nil {
				t.Fatalf("actual migration failed: %T", err)
			}
		}
	}
	old := config.DB
	config.DB = db
	t.Cleanup(func() { config.DB = old })
	return db, l
}

func TestNativeMariaDBFacultyInsights(t *testing.T) {
	db, l := nativeFacultyDB(t)
	seedFacultyAPIFixture(t, db)
	if err := db.Exec(`INSERT INTO scopus_document_affiliations(document_id,afid,provenance,payload_country) SELECT id,'kku','fixture','Thailand' FROM scopus_documents WHERE id<=246`).Error; err != nil {
		t.Fatal(err)
	}
	report := map[string]interface{}{"fixture_only": true, "native_mariadb": true, "baseline_documents": 246}
	t.Run("filters_partitions_and_paging", func(t *testing.T) {
		for _, tt := range []struct {
			query string
			total int
		}{
			{"", 246}, {"year_start_be=2567&year_end_be=2568", 4}, {"year_start_be=2568&year_end_be=2567", 4}, {"year_start_be=2569", 241}, {"year_end_be=2567", 1}, {"year_start_be=2024&year_end_be=2025", 4},
			{"aggregation_types=Conference%20Proceeding", 1}, {"open_access_mode=oa", 123}, {"open_access_mode=non_oa", 123}, {"citation_min=245", 2}, {"citation_max=2", 2}, {"citation_min=1&citation_max=10", 10},
			{"search_title=Needle", 1}, {"search_doi=doi-1", 1}, {"search_eid=eid-1", 1}, {"search_scopus_id=sid-1", 1}, {"search_journal=Unique", 1}, {"search_keyword=keyword-246", 1}, {"search_author=Bob", 4}, {"search_affiliation=kku", 246}, {"search_affiliation=Japan", 0},
			{"quality_buckets=T1", 1}, {"quality_buckets=Q1", 0}, {"quality_buckets=Q2", 2}, {"quality_buckets=Q3", 1}, {"quality_buckets=Q4", 239}, {"quality_buckets=N%2FA", 2}, {"quality_buckets=TCI", 0}, {"quality_buckets=T1,Q2", 3},
			{"year_start_be=2569&year_end_be=2569&quality_buckets=Q2&open_access_mode=oa&citation_max=5&search_author=Bob&search_title=Needle&search_journal=Unique&aggregation_types=Journal&search_affiliation=Khon&search_keyword=keyword-1", 1}, {"search_title=%27%20OR%201%3D1%20--", 0}, {"scope=individual", 246},
		} {
			total, revision := facultyAPITotal(t, tt.query)
			if total != tt.total {
				t.Fatalf("native filter %s = %d want %d", tt.query, total, tt.total)
			}
			w, d := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, tt.query+"&revision="+revision)
			if w.Code != 200 || d["total"] != float64(total) {
				t.Fatal("native summary/page parity", w.Body.String())
			}
		}
		_, s := facultyAPIRequest(t, AdminGetScopusFacultyInsights, "")
		totals := s["totals"].(map[string]interface{})
		if totals["international"].(map[string]interface{})["yes"] != float64(3) || totals["roles"].(map[string]interface{})["first"] != float64(3) {
			t.Fatal("native partition baseline")
		}
		rev := s["revision"].(string)
		seen := map[float64]bool{}
		for page := 1; page <= 2; page++ {
			_, d := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, fmt.Sprintf("revision=%s&page_size=200&page=%d", rev, page))
			for _, row := range d["documents"].([]interface{}) {
				id := row.(map[string]interface{})["document_id"].(float64)
				if seen[id] {
					t.Fatal("native duplicate page")
				}
				seen[id] = true
			}
		}
		if len(seen) != 246 {
			t.Fatal("native >200 truncation")
		}
		for _, raw := range s["by_year"].([]interface{}) {
			year := raw.(map[string]interface{})
			for _, status := range insightInternationalStates {
				for _, role := range insightRoleStates {
					_, d := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "year_be="+year["bucket"].(string)+"&international_status="+status+"&faculty_role="+role+"&revision="+rev)
					if d["total"] != year["country_role"].(map[string]interface{})[status].(map[string]interface{})[role] {
						t.Fatal("native cross-tab mismatch")
					}
				}
			}
		}
		report["baseline_summary"] = s
	})

	server, token := nativeFacultyHTTPServer(t, db)
	t.Run("authenticated_HTTP", func(t *testing.T) {
		for _, tt := range []struct {
			token string
			want  int
		}{{"", 401}, {"invalid", 401}, {token, 200}, {nativeFacultyToken(t, 9001, "admin@fixture.invalid", 3, "missing-session"), 401}, {nativeFacultyToken(t, 9001, "admin@fixture.invalid", 3, "expired-session"), 401}, {nativeFacultyToken(t, 9001, "admin@fixture.invalid", 3, ""), 200}} {
			req, _ := http.NewRequest("GET", server.URL+"/api/v1/admin/scopus/dashboard/faculty-insights", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Fatalf("native auth %d want %d", resp.StatusCode, tt.want)
			}
		}
		// The database role must override an admin role claim in a signed token.
		member := nativeFacultyToken(t, 9002, "member@fixture.invalid", 3, "")
		req, _ := http.NewRequest("GET", server.URL+"/api/v1/admin/scopus/dashboard/faculty-insights/drilldown", nil)
		req.Header.Set("Authorization", "Bearer "+member)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatal("member/admin-claim bypassed actual permission")
		}
		report["authenticated_http"] = map[string]interface{}{"missing_invalid_token": 401, "active_session": 200, "missing_expired_session": 401, "legacy_sessionless_token": 200, "database_member_with_admin_claim": 403}
	})
	t.Run("frontend_real_API_optional", func(t *testing.T) {
		manifest := os.Getenv("SCOPUS_NATIVE_UI_HANDSHAKE_PATH")
		if manifest == "" {
			t.Skip("UI handshake not requested; native API tests still execute")
		}
		if !filepath.IsAbs(manifest) || !strings.HasPrefix(filepath.Clean(manifest), filepath.Clean(`G:\works-fund-project\.phase5-mariadb`)+string(os.PathSeparator)) {
			t.Fatal("UI manifest must stay inside owned local runtime")
		}
		// A previous completion file must never satisfy a new integrated run.
		if err := os.Remove(manifest + ".done"); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		bytes, _ := json.Marshal(map[string]interface{}{"api_origin": server.URL, "access_token": token, "fixture_only": true, "documents": 246})
		if err := os.WriteFile(manifest, bytes, 0600); err != nil {
			t.Fatal(err)
		}
		deadline := time.NewTimer(240 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-deadline.C:
				t.Fatal("local UI handshake timed out")
			case <-ticker.C:
				if b, err := os.ReadFile(manifest + ".done"); err == nil {
					var result map[string]interface{}
					if json.Unmarshal(b, &result) != nil || result["passed"] != true {
						t.Fatal("frontend-real API checks failed")
					}
					report["frontend_real_api"] = result
					return
				}
			}
		}
	})
	t.Run("readonly_repeatable_snapshot_and_dirty_revision", func(t *testing.T) {
		filters := scopusDashboardFilters{Scope: "faculty", OpenAccessMode: "all"}
		_, before := facultyAPITotal(t, "")
		tx := db.Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		var id uint
		if err := tx.Table("scopus_documents").Select("id").Order("id").Limit(1).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		err := tx.Exec(`UPDATE scopus_documents SET citedby_count=999 WHERE id=1`).Error
		var me *driver.MySQLError
		if !errors.As(err, &me) || me.Number != 1792 {
			t.Fatalf("read-only protection not enforced: %T", err)
		}
		if err := db.Transaction(func(writer *gorm.DB) error {
			return writer.Exec(`UPDATE scopus_documents SET citedby_count=999,author_role_status='pending',raw_json='{"changed":true}' WHERE id=1`).Error
		}); err != nil {
			t.Fatal(err)
		}
		old, err := loadFacultyInsightSnapshot(tx, filters)
		if err != nil {
			t.Fatal(err)
		}
		if old.Revision != before || old.Documents[0].Citations != 1 || old.Documents[0].FacultyRole != "first" || old.Documents[0].InternationalStatus != "yes" {
			t.Fatal("MariaDB snapshot mixed concurrent state")
		}
		tx.Rollback()
		current, err := loadFacultyInsightSnapshot(db, filters)
		if err != nil {
			t.Fatal(err)
		}
		if current.Revision == before || current.Documents[0].InternationalStatus != "unknown" || current.Documents[0].FacultyRole != "unknown" || len(current.Documents[0].Countries) != 0 {
			t.Fatal("dirty native trigger evidence leaked")
		}
		w, b := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "revision="+before)
		if w.Code != 409 || b["documents"] != nil {
			t.Fatal("native stale revision must reject page")
		}
		if err := db.Exec(`UPDATE scopus_affiliations SET country='India' WHERE id=1`).Error; err != nil {
			t.Fatal(err)
		}
		_, b = facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "country_key=japan")
		if b["total"] != float64(0) {
			t.Fatal("catalogue dirty stale countries leaked")
		}
		report["read_only_error_code"] = 1792
		report["concurrent_snapshot_passed"] = true
	})
	t.Run("representative_cost", func(t *testing.T) { nativeFacultyPerformance(t, db, l, report) })
	t.Run("missing_migration_safe_unavailable", func(t *testing.T) {
		if err := db.Migrator().DropTable("scopus_document_insights"); err != nil {
			t.Fatal(err)
		}
		w, b := facultyAPIRequest(t, AdminGetScopusFacultyInsights, "")
		if w.Code != 503 || b["totals"] != nil {
			t.Fatal("missing native schema looked empty")
		}
	})
	if path := os.Getenv("SCOPUS_NATIVE_API_REPORT_PATH"); path != "" {
		bytes, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(path, bytes, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func nativeFacultyToken(t *testing.T, id int, email string, role int, jti string) string {
	t.Helper()
	claims := middleware.Claims{UserID: id, Email: email, RoleID: role, RegisteredClaims: jwt.RegisteredClaims{ID: jti, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func nativeFacultyHTTPServer(t *testing.T, db *gorm.DB) (*httptest.Server, string) {
	t.Helper()
	t.Setenv("JWT_SECRET", "native-phase5-synthetic-secret")
	for _, stmt := range []string{`ALTER TABLE users ADD COLUMN email VARCHAR(128)`, `INSERT INTO users(user_id,scopus_id,is_test,role_id,email) VALUES(9001,'',0,3,'admin@fixture.invalid'),(9002,'',0,1,'member@fixture.invalid')`, `CREATE TABLE user_sessions(session_id INTEGER PRIMARY KEY,user_id INTEGER,access_token_jti VARCHAR(128),is_active BOOLEAN,expires_at DATETIME,last_activity DATETIME,updated_at DATETIME) ENGINE=InnoDB`, `INSERT INTO user_sessions(session_id,user_id,access_token_jti,is_active,expires_at) VALUES(1,9001,'active-session',1,DATE_ADD(UTC_TIMESTAMP(),INTERVAL 1 HOUR)),(2,9001,'expired-session',1,DATE_SUB(UTC_TIMESTAMP(),INTERVAL 1 HOUR))`} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "http://127.0.0.1:3106" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type")
			c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	protected := r.Group("/api/v1", middleware.AuthMiddleware())
	permission := middleware.RequirePermission("ui.page.admin.research_dashboard.view")
	protected.GET("/admin/scopus/dashboard/faculty-insights", permission, AdminGetScopusFacultyInsights)
	protected.GET("/admin/scopus/dashboard/faculty-insights/drilldown", permission, AdminGetScopusFacultyInsightsDrilldown)
	protected.GET("/profile", func(c *gin.Context) { user, _ := c.Get("user"); c.JSON(200, gin.H{"user": user}) })
	// Exists only in this disposable httptest server, never in application routes.
	protected.POST("/__fixture/mutate", permission, func(c *gin.Context) {
		var body struct {
			Action string `json:"action"`
		}
		if c.ShouldBindJSON(&body) != nil || (body.Action != "change" && body.Action != "restore") {
			c.AbortWithStatus(400)
			return
		}
		citations := 2
		if body.Action == "restore" {
			citations = 1
		}
		if err := db.Exec("UPDATE scopus_documents SET citedby_count=? WHERE id=1", citations).Error; err != nil {
			c.AbortWithStatus(500)
			return
		}
		c.JSON(200, gin.H{"fixture_only": true})
	})
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	return server, nativeFacultyToken(t, 9001, "admin@fixture.invalid", 3, "active-session")
}

func nativeFacultyPerformance(t *testing.T, db *gorm.DB, l *nativeFacultyLog, report map[string]interface{}) {
	t.Helper()
	// Match the relevant index shapes in the repository SQL dump after duplicate
	// join correctness was tested. This is a synthetic cost model, not production.
	for _, stmt := range []string{
		`DELETE b FROM scopus_source_metrics b JOIN scopus_source_metrics a ON a.source_id=b.source_id AND a.metric_year=b.metric_year AND a.doc_type=b.doc_type AND a.source_metric_id<b.source_metric_id`,
		`ALTER TABLE scopus_source_metrics MODIFY source_id VARCHAR(32),MODIFY doc_type VARCHAR(32),ADD UNIQUE KEY uq_scopus_source_year_type(source_id,metric_year,doc_type)`,
		`ALTER TABLE scopus_document_authors ADD UNIQUE KEY uq_scopus_document_authors_doc_author(document_id,author_id),ADD KEY idx_scopus_document_authors_document(document_id,author_seq),ADD KEY idx_scopus_document_authors_author(author_id),ADD KEY fk_scopus_document_authors_affiliation(affiliation_id)`,
		`ALTER TABLE scopus_documents MODIFY source_id VARCHAR(32),ADD KEY idx_scopus_documents_source_id(source_id),ADD KEY idx_scopus_documents_cover_date(cover_date)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	docs := []models.ScopusDocument{}
	authors := []models.ScopusDocumentAuthor{}
	metadata := []models.ScopusDocumentInsight{}
	countries := []models.ScopusDocumentCountry{}
	for n := 1000; n < 6000; n++ {
		id := uint(n)
		date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		aff := uint(1)
		international := n%2 == 0
		first, corr := insightBool(n%5 == 0), insightBool(n%4 == 0)
		if n%9 == 0 {
			first = nil
		}
		docs = append(docs, models.ScopusDocument{ID: id, EID: fmt.Sprintf("cost-%d", n), Title: insightString(fmt.Sprintf("Synthetic cost paper %d", n)), CoverDate: &date, SourceID: insightString("base"), AggregationType: insightString("Journal"), AuthorRoleStatus: insightString("complete"), RawJSON: []byte(`{}`)})
		authors = append(authors, models.ScopusDocumentAuthor{DocumentID: id, AuthorID: 1, AffiliationID: &aff, AuthorSeq: 1, IsFirstAuthor: first, IsCorrespondingAuthor: corr})
		metadata = append(metadata, models.ScopusDocumentInsight{DocumentID: id, International: &international, CountriesComplete: true, Status: "complete", NormalizerVersion: services.ScopusInsightNormalizerVersion, CheckedAt: date})
		countries = append(countries, models.ScopusDocumentCountry{DocumentID: id, CountryKey: "thailand", CountryName: "Thailand", Provenance: "fixture"})
		if international {
			countries = append(countries, models.ScopusDocumentCountry{DocumentID: id, CountryKey: "japan", CountryName: "Japan", Provenance: "fixture"})
		}
	}
	for _, records := range []interface{}{&docs, &authors, &metadata, &countries} {
		if err := db.CreateInBatches(records, 400).Error; err != nil {
			t.Fatal(err)
		}
	}
	results := []map[string]interface{}{}
	for _, query := range []string{"", "year_start_be=2569&year_end_be=2569&quality_buckets=Q4", "search_title=Synthetic%20cost%20paper%201000"} {
		total, rev := facultyAPITotal(t, query)
		for _, endpoint := range []struct {
			name    string
			handler gin.HandlerFunc
			query   string
		}{{"summary", AdminGetScopusFacultyInsights, query}, {"drilldown_200", AdminGetScopusFacultyInsightsDrilldown, query + "&revision=" + rev + "&page_size=200"}} {
			elapsed := []float64{}
			sqlMS := []float64{}
			queries := []int{}
			bytesReturned := []int{}
			var memoryBefore, memoryAfter runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&memoryBefore)
			for i := 0; i < 5; i++ {
				l.reset(true)
				start := time.Now()
				w, _ := facultyAPIRequest(t, endpoint.handler, endpoint.query)
				elapsed = append(elapsed, float64(time.Since(start).Microseconds())/1000)
				n, ms := l.stats()
				queries = append(queries, n)
				sqlMS = append(sqlMS, ms)
				bytesReturned = append(bytesReturned, w.Body.Len())
				if w.Code != 200 {
					t.Fatal("cost request failed")
				}
			}
			l.reset(false)
			runtime.ReadMemStats(&memoryAfter)
			sort.Float64s(elapsed)
			results = append(results, map[string]interface{}{"endpoint": endpoint.name, "filter": query, "filtered_documents": total, "iterations": 5, "min_ms": elapsed[0], "median_ms": elapsed[2], "max_ms": elapsed[4], "sql_ms_by_request": sqlMS, "sql_statements_by_request": queries, "response_bytes_by_request": bytesReturned, "allocated_bytes_per_request": (memoryAfter.TotalAlloc - memoryBefore.TotalAlloc) / 5})
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		filters := parseScopusDashboardFilters(c)
		sqlText := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return facultyInsightBaseQuery(tx, filters).Select(facultyInsightProjection(tx)).Order("sd.id ASC").Find(&[]facultyInsightDocument{})
		})
		var plan string
		if err := db.Raw("EXPLAIN FORMAT=JSON " + sqlText).Row().Scan(&plan); err != nil {
			t.Fatal(err)
		}
		var decoded interface{}
		json.Unmarshal([]byte(plan), &decoded)
		report["explain_"+query] = decoded
	}
	var metricRows, metricSources int64
	db.Table("scopus_source_metrics").Count(&metricRows)
	db.Table("scopus_source_metrics").Distinct("source_id").Count(&metricSources)
	report["cost_scale"] = map[string]interface{}{"eligible_documents": 5246, "additional_documents": 5000, "country_rows_added": 7500, "duplicate_users": true, "representative_indexes": "repository db/fund_cpkku.sql relevant index shapes", "metric_sources": metricSources, "metric_year_rows": metricRows, "request_measurement": "controller snapshot/aggregation/JSON; excludes HTTP/auth network overhead"}
	report["cost_results"] = results
	t.Log("Measured native summary/drilldown at 5246 synthetic eligible documents; no production latency claim")
}
