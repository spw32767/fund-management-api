//go:build insight_integration || insight_mariadb

package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"fund-management-api/config"
	"fund-management-api/middleware"
	"fund-management-api/models"
	"fund-management-api/services"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func facultyAPIIntegrationDB(t *testing.T) *gorm.DB {
	return facultyAPIIntegrationDBAt(t, fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
}

func facultyAPIIntegrationDBAt(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	pool.SetMaxOpenConns(2)
	t.Cleanup(func() { pool.Close() })
	if err = db.AutoMigrate(&models.ScopusDocument{}, &models.ScopusAuthor{}, &models.ScopusDocumentAuthor{}, &models.ScopusAffiliation{}, &models.ScopusDocumentInsight{}, &models.ScopusDocumentCountry{}, &models.ScopusDocumentAffiliation{}, &models.ScopusSourceMetric{}); err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		`CREATE TABLE users(user_id INTEGER PRIMARY KEY,scopus_id TEXT,delete_at DATETIME,is_test INTEGER,role_id INTEGER)`,
		`CREATE TABLE scopus_country_catalogue_guard(id INTEGER PRIMARY KEY,revision INTEGER)`,
	} {
		if err = db.Exec(ddl).Error; err != nil {
			t.Fatal(err)
		}
	}
	old := config.DB
	config.DB = db
	t.Cleanup(func() { config.DB = old })
	return db
}

func seedFacultyAPIFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	check := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	check(db.Exec(`INSERT INTO users VALUES(1,'  author-a  ',NULL,0,2),(2,'author-a',NULL,0,5),(3,'author-b',NULL,0,1),(4,'test-only',NULL,1,3),(5,'deleted-only','2025-01-01',0,3),(6,'outside-only',NULL,0,3)`).Error)
	for _, a := range []models.ScopusAuthor{{ID: 1, ScopusAuthorID: "author-a", FullName: insightString("Alice Example")}, {ID: 2, ScopusAuthorID: "author-b", FullName: insightString("Bob Example")}, {ID: 3, ScopusAuthorID: "test-only"}, {ID: 4, ScopusAuthorID: "deleted-only"}, {ID: 5, ScopusAuthorID: "outside-only"}} {
		check(db.Create(&a).Error)
	}
	for _, a := range []models.ScopusAffiliation{{ID: 1, Afid: "kku", Name: insightString(" Khon Kaen University "), Country: insightString("Thailand"), City: insightString("Khon Kaen")}, {ID: 2, Afid: "sci", Name: insightString("Faculty of Science, Khon Kaen University"), Country: insightString("Thailand")}, {ID: 3, Afid: "outside", Name: insightString("Outside University"), Country: insightString("Japan")}} {
		check(db.Create(&a).Error)
	}
	for _, metric := range []struct {
		source           string
		year             int
		status, quartile string
		percentile       float64
	}{
		{"base", 2026, "complete", "Q4", 30},
		{"a", 2025, "complete", "Q2", 70}, {"a", 2026, "partial", "Q1", 99},
		{"b", 2025, "complete", "Q1", 95},
		{"c", 2023, "complete", "Q3", 50},
		{"fallback", 2024, "complete", "Q2", 80}, {"fallback", 2025, "partial", "Q1", 99},
	} {
		check(db.Exec(`INSERT INTO scopus_source_metrics(source_id,metric_year,doc_type,cite_score_status,cite_score_quartile,cite_score_percentile) VALUES(?,?,'all',?,?,?)`, metric.source, metric.year, metric.status, metric.quartile, metric.percentile).Error)
	}
	// A duplicate metric join must not multiply documents or card counts.
	check(db.Exec(`INSERT INTO scopus_source_metrics(source_id,metric_year,doc_type,cite_score_status,cite_score_quartile,cite_score_percentile) SELECT source_id,metric_year,doc_type,cite_score_status,cite_score_quartile,cite_score_percentile FROM scopus_source_metrics WHERE source_id='base'`).Error)
	for n := 1; n <= 246; n++ {
		id := uint(n)
		date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		yearDate := &date
		source, agg, title, journal := "base", "Journal", "Common Paper", "Common Journal"
		cite := n
		oa := uint8(n % 2)
		var display *string
		switch n {
		case 1:
			source, title, journal = "a", "Needle Paper", "Unique Journal"
		case 2, 4:
			source = "b"
			date = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			if n == 4 {
				agg = "Conference Proceeding"
			}
		case 3:
			source = "c"
			date = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		case 5:
			source = "missing"
		case 245:
			source = "fallback"
			yearDate = nil
			display = insightString("1 Jan 2025")
		case 246:
			yearDate = nil
		}
		status := insightString("complete")
		if n == 4 {
			status = insightString("pending")
		}
		if n == 9 {
			status = nil
		}
		if n == 10 {
			status = insightString("no_correspondence")
		}
		raw := []byte("this deliberately cannot be parsed as JSON")
		if db.Dialector.Name() == "mysql" {
			raw = []byte(`{}`)
		}
		doc := models.ScopusDocument{ID: id, EID: fmt.Sprintf("eid-%d", n), ScopusID: insightString(fmt.Sprintf("sid-%d", n)), Title: &title, DOI: insightString(fmt.Sprintf("doi-%d", n)), PublicationName: &journal, AggregationType: &agg, SourceID: &source, CoverDate: yearDate, CoverDisplayDate: display, CitedByCount: &cite, OpenAccessFlag: &oa, AuthKeywords: []byte(fmt.Sprintf("keyword-%d", n)), AuthorRoleStatus: status, RawJSON: raw}
		check(db.Create(&doc).Error)
		first, corr := insightBool(false), insightBool(false)
		if n == 1 || n == 4 || n == 6 || n == 10 {
			first = insightBool(true)
		}
		if n == 1 || n == 2 {
			corr = insightBool(true)
		}
		if n == 7 {
			first = nil
			corr = insightBool(true)
		}
		if n == 8 {
			corr = nil
		}
		aff := uint(1)
		link := models.ScopusDocumentAuthor{DocumentID: id, AuthorID: 1, AuthorSeq: 1, AffiliationID: &aff, IsFirstAuthor: first, IsCorrespondingAuthor: corr}
		check(db.Create(&link).Error)
		if n == 1 || n == 3 || n == 6 || n == 8 {
			otherFirst, otherCorr := insightBool(false), insightBool(true)
			if n == 3 || n == 6 {
				otherFirst = nil
				otherCorr = nil
			}
			link = models.ScopusDocumentAuthor{DocumentID: id, AuthorID: 2, AuthorSeq: 2, AffiliationID: &aff, IsFirstAuthor: otherFirst, IsCorrespondingAuthor: otherCorr}
			check(db.Create(&link).Error)
		}
		if n == 4 {
			continue
		} // Missing country metadata and a stale membership.
		international, complete, mstatus, version := insightBool(false), true, "complete", services.ScopusInsightNormalizerVersion
		if n == 1 || n == 2 || n == 6 {
			international = insightBool(true)
		}
		if n == 2 {
			complete = false
			mstatus = "incomplete"
		}
		if n == 3 {
			international = insightBool(true)
			mstatus = "dirty_catalogue"
		}
		if n == 5 {
			international = insightBool(true)
			version = "obsolete"
		}
		if n == 246 {
			international = nil
			complete = false
			mstatus = "incomplete"
		}
		m := models.ScopusDocumentInsight{DocumentID: id, International: international, CountriesComplete: complete, Status: mstatus, NormalizerVersion: version, PayloadHash: fmt.Sprintf("hash-%d", n)}
		check(db.Create(&m).Error)
		check(db.Create(&models.ScopusDocumentCountry{DocumentID: id, CountryKey: "thailand", CountryName: "Thailand"}).Error)
		if n <= 6 {
			check(db.Create(&models.ScopusDocumentCountry{DocumentID: id, CountryKey: "japan", CountryName: "Japan"}).Error)
		}
		if n == 1 {
			check(db.Create(&models.ScopusDocumentCountry{DocumentID: id, CountryKey: "china", CountryName: "China"}).Error)
		}
	}
	check(db.Create(&models.ScopusDocumentCountry{DocumentID: 4, CountryKey: "japan", CountryName: "Japan"}).Error)
	// Documents excluded by each independent faculty gate, despite role flags.
	for i, author := range []uint{3, 4, 5} {
		id := uint(300 + i)
		check(db.Create(&models.ScopusDocument{ID: id, EID: fmt.Sprintf("excluded-%d", i)}).Error)
		aff := uint(1)
		if author == 5 {
			aff = 3
		}
		check(db.Create(&models.ScopusDocumentAuthor{DocumentID: id, AuthorID: author, AffiliationID: &aff, IsFirstAuthor: insightBool(true), IsCorrespondingAuthor: insightBool(true)}).Error)
	}
}

func facultyAPIRequest(t *testing.T, handler gin.HandlerFunc, query string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?"+query, nil)
	handler(c)
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid response %s", w.Body.String())
	}
	return w, body
}

func facultyAPITotal(t *testing.T, query string) (int, string) {
	t.Helper()
	w, b := facultyAPIRequest(t, AdminGetScopusFacultyInsights, query)
	if w.Code != http.StatusOK {
		t.Fatalf("summary %s: %d %s", query, w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("summary must not cache")
	}
	return int(b["totals"].(map[string]interface{})["total"].(float64)), b["revision"].(string)
}

func TestFacultyInsightAPIParityPagingAndDimensions(t *testing.T) {
	db := facultyAPIIntegrationDB(t)
	seedFacultyAPIFixture(t, db)
	total, revision := facultyAPITotal(t, "")
	if total != 246 {
		t.Fatalf("unique cohort total %d", total)
	}
	_, b := facultyAPIRequest(t, AdminGetScopusFacultyInsights, "")
	totals := b["totals"].(map[string]interface{})
	if !reflect.DeepEqual(totals["international"], map[string]interface{}{"yes": float64(3), "no": float64(239), "unknown": float64(4)}) {
		t.Fatal(totals["international"])
	}
	if !reflect.DeepEqual(totals["roles"], map[string]interface{}{"first": float64(3), "corresponding": float64(1), "coauthor": float64(237), "unknown": float64(5)}) {
		t.Fatal(totals["roles"])
	}
	partners := totals["partners"].([]interface{})
	if len(partners) != 2 || partners[0].(map[string]interface{})["documents"] != float64(3) {
		t.Fatal(partners)
	}
	seen := map[float64]bool{}
	for page := 1; page <= 6; page++ {
		w, d := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, fmt.Sprintf("revision=%s&page=%d&page_size=50", revision, page))
		if w.Code != 200 || int(d["total"].(float64)) != total || d["revision"] != revision {
			t.Fatal(w.Body.String())
		}
		for _, raw := range d["documents"].([]interface{}) {
			id := raw.(map[string]interface{})["document_id"].(float64)
			if seen[id] {
				t.Fatal("duplicate page membership")
			}
			seen[id] = true
		}
	}
	if len(seen) != 246 || !seen[246] {
		t.Fatal("paging truncated after 200")
	}
	for _, rawYear := range b["by_year"].([]interface{}) {
		y := rawYear.(map[string]interface{})
		bucket := y["bucket"].(string)
		for _, s := range insightInternationalStates {
			for _, r := range insightRoleStates {
				_, d := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "revision="+revision+"&year_be="+bucket+"&international_status="+s+"&faculty_role="+r)
				want := y["country_role"].(map[string]interface{})[s].(map[string]interface{})[r]
				if d["total"] != want {
					t.Fatalf("cross-tab %s %s %s: %v != %v", bucket, s, r, d["total"], want)
				}
			}
		}
	}
	for _, raw := range partners {
		p := raw.(map[string]interface{})
		_, d := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "revision="+revision+"&international_status=yes&country_key="+p["country_key"].(string))
		if d["total"] != p["documents"] {
			t.Fatal("partner parity")
		}
	}
	_, d := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "country_key=japan")
	if d["total"] != float64(3) {
		t.Fatal("stale memberships leaked", d)
	}
	_, d = facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "year_be=undated")
	rows := d["documents"].([]interface{})
	if len(rows) != 1 || rows[0].(map[string]interface{})["year_be"] != nil {
		t.Fatal("missing explicit undated document")
	}
	_, d = facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "search_eid=eid-1")
	row := d["documents"].([]interface{})[0].(map[string]interface{})
	if row["faculty_role"] != "first" || len(row["eligible_authors"].([]interface{})) != 2 || row["metric_year"] != float64(2025) {
		t.Fatal("role overlap/duplicate users/metric fallback", row)
	}
}

func TestFacultyInsightAPIFiltersAndExistingDashboardParity(t *testing.T) {
	db := facultyAPIIntegrationDB(t)
	seedFacultyAPIFixture(t, db)
	for _, tt := range []struct {
		query string
		total int
	}{
		{"year_start_be=2567&year_end_be=2568", 4}, {"year_start_be=2568&year_end_be=2567", 4},
		{"year_start_be=2569", 241}, {"year_end_be=2567", 1}, {"year_start_be=2024&year_end_be=2025", 4},
		{"aggregation_types=Conference%20Proceeding", 1}, {"open_access_mode=oa", 123}, {"open_access_mode=non_oa", 123},
		{"citation_min=1&citation_max=10", 10}, {"search_title=Needle", 1}, {"search_doi=doi-1", 1}, {"search_eid=eid-1", 1}, {"search_scopus_id=sid-1", 1}, {"search_journal=Unique", 1}, {"search_keyword=keyword-246", 1},
		{"citation_min=245", 2}, {"citation_max=2", 2},
		{"search_author=Bob", 4}, {"search_affiliation=kku", 246}, {"search_affiliation=Japan", 0},
		{"quality_buckets=T1", 1}, {"quality_buckets=Q1", 0}, {"quality_buckets=Q2", 2}, {"quality_buckets=Q3", 1}, {"quality_buckets=N%2FA", 2}, {"quality_buckets=TCI", 0}, {"quality_buckets=T1,Q2", 3},
		{"quality_buckets=Q4", 239},
		{"year_start_be=2569&year_end_be=2569&quality_buckets=Q2&open_access_mode=oa&citation_max=5&search_author=Bob&search_title=Needle&search_journal=Unique&aggregation_types=Journal&search_affiliation=Khon&search_keyword=keyword-1", 1},
		{"search_title=%27%20OR%201%3D1%20--", 0}, {"scope=individual", 246},
	} {
		t.Run(tt.query, func(t *testing.T) {
			total, rev := facultyAPITotal(t, tt.query)
			if total != tt.total {
				t.Fatalf("total %d want %d", total, tt.total)
			}
			_, body := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, tt.query+"&revision="+rev)
			if body["total"] != float64(total) {
				t.Fatal("summary/drilldown filter parity", body)
			}
			// Compare the shared legacy dashboard filter helper separately against the
			// new query population; SQLite year adapter preserves its expression rules.
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/?"+tt.query, nil)
			f := parseScopusDashboardFilters(c)
			var ids []uint
			if err := applyScopusDashboardFiltersWithYear(db.Table("scopus_documents AS sd").Joins("LEFT JOIN scopus_source_metrics metrics ON metrics.source_id=sd.source_id AND metrics.doc_type='all' AND metrics.metric_year="+scopusMetricYearForPublicationExpr(facultyInsightYearExpr(db))), f, true, facultyInsightYearExpr(db)).Distinct("sd.id").Pluck("sd.id", &ids).Error; err != nil {
				t.Fatal(err)
			}
			if len(ids) != total {
				t.Fatal("legacy cohort differs")
			}
		})
	}
}

func TestFacultyInsightAPIRevisionUnavailableAndSafety(t *testing.T) {
	db := facultyAPIIntegrationDB(t)
	seedFacultyAPIFixture(t, db)
	_, revision := facultyAPITotal(t, "")
	_, repeat := facultyAPITotal(t, "")
	if revision != repeat {
		t.Fatal("unstable revision")
	}
	_, ordered := facultyAPITotal(t, "quality_buckets=Q2,T1,T1")
	_, reordered := facultyAPITotal(t, "quality_buckets=T1,Q2")
	if ordered != reordered {
		t.Fatal("set ordering changed revision")
	}
	for _, change := range []string{
		`UPDATE scopus_documents SET citedby_count=999 WHERE id=1`,
		`UPDATE scopus_document_authors SET is_first_author=0 WHERE document_id=1`,
		`UPDATE scopus_document_insights SET status='dirty_payload' WHERE document_id=2`,
		`UPDATE scopus_document_countries SET country_name='Updated' WHERE document_id=3 AND country_key='japan'`,
		`UPDATE scopus_source_metrics SET cite_score_quartile='Q3' WHERE source_id='a' AND metric_year=2025`,
		`UPDATE users SET is_test=1 WHERE scopus_id='author-b'`,
	} {
		if err := db.Exec(change).Error; err != nil {
			t.Fatal(err)
		}
		_, next := facultyAPITotal(t, "")
		if next == revision {
			t.Fatalf("revision missed %s", change)
		}
		w, b := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "revision="+revision)
		if w.Code != 409 || b["code"] != "insight_revision_mismatch" || b["documents"] != nil {
			t.Fatal(w.Body.String())
		}
		revision = next
	}
	for _, query := range []string{"faculty_role=nope", "country_key=" + url.QueryEscape("japan'; DROP TABLE users;--"), "page_size=999"} {
		w, _ := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, query)
		if w.Code != 400 {
			t.Fatal("invalid dimension accepted")
		}
	}
	_, body := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "page=999")
	if len(body["documents"].([]interface{})) != 0 {
		t.Fatal("out-of-range page is not empty")
	}
	for _, endpoint := range []struct {
		handler gin.HandlerFunc
		summary bool
	}{{AdminGetScopusFacultyInsights, true}, {AdminGetScopusFacultyInsightsDrilldown, false}} {
		w, b := facultyAPIRequest(t, endpoint.handler, "search_title=does-not-exist")
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		if endpoint.summary {
			totals := b["totals"].(map[string]interface{})
			for _, p := range totals["international_percent"].(map[string]interface{}) {
				if p != nil {
					t.Fatal("zero percentage must be null")
				}
			}
		}
	}
	if err := db.Migrator().DropTable("scopus_document_insights"); err != nil {
		t.Fatal(err)
	}
	for _, handler := range []gin.HandlerFunc{AdminGetScopusFacultyInsights, AdminGetScopusFacultyInsightsDrilldown} {
		w, b := facultyAPIRequest(t, handler, "")
		if w.Code != 503 || b["code"] != "faculty_insights_unavailable" || b["totals"] != nil {
			t.Fatal("missing migration misleading response", b)
		}
	}
}

func TestFacultyInsightAPIPermissionGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := config.DB
	config.DB = nil
	t.Cleanup(func() { config.DB = old })
	for _, handler := range []gin.HandlerFunc{AdminGetScopusFacultyInsights, AdminGetScopusFacultyInsightsDrilldown} {
		r := gin.New()
		r.GET("/", middleware.RequirePermission("ui.page.admin.research_dashboard.view"), handler)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 403 {
			t.Fatal("missing auth context bypassed")
		}
		for _, role := range []int{1, 3} {
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set("userID", 1); c.Set("roleID", role); c.Next() })
			r.GET("/", middleware.RequirePermission("ui.page.admin.research_dashboard.view"), handler)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			want := 403
			if role == 3 {
				want = 503
			}
			if w.Code != want {
				t.Fatalf("role %d code %d want %d", role, w.Code, want)
			}
		}
	}
}

func TestFacultyInsightSnapshotConcurrentChange(t *testing.T) {
	// WAL allows an independent writer to commit while this read snapshot is
	// active. This verifies multi-query consistency, not MariaDB locking semantics.
	db := facultyAPIIntegrationDBAt(t, filepath.Join(t.TempDir(), "insights.sqlite")+"?_journal_mode=WAL")
	seedFacultyAPIFixture(t, db)
	_, before := facultyAPITotal(t, "")
	filters := scopusDashboardFilters{Scope: "faculty", OpenAccessMode: "all"}
	tx := db.Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	var id uint
	if err := tx.Table("scopus_documents").Select("id").Order("id").Limit(1).Scan(&id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(writer *gorm.DB) error {
		if err := writer.Exec(`UPDATE scopus_documents SET citedby_count=999,author_role_status='pending' WHERE id=1`).Error; err != nil {
			return err
		}
		return writer.Exec(`UPDATE scopus_document_insights SET status='dirty_catalogue' WHERE document_id=1`).Error
	}); err != nil {
		t.Fatal(err)
	}
	old, err := loadFacultyInsightSnapshot(tx, filters)
	if err != nil {
		t.Fatal(err)
	}
	if old.Revision != before || old.Documents[0].Citations != 1 || old.Documents[0].FacultyRole != "first" || old.Documents[0].InternationalStatus != "yes" {
		t.Fatal("request mixed concurrent snapshots")
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	current, err := loadFacultyInsightSnapshot(db, filters)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision == before || current.Documents[0].Citations != 999 || current.Documents[0].FacultyRole != "unknown" || current.Documents[0].InternationalStatus != "unknown" || len(current.Documents[0].Countries) != 0 {
		t.Fatal("new snapshot missed committed change")
	}
}

func TestFacultyInsightSnapshotBatchesAndAmbiguousMetrics(t *testing.T) {
	db := facultyAPIIntegrationDB(t)
	seedFacultyAPIFixture(t, db)
	for n := 1000; n < 1200; n++ {
		date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		aff := uint(1)
		for _, record := range []interface{}{
			&models.ScopusDocument{ID: uint(n), EID: fmt.Sprintf("batch-%d", n), CoverDate: &date, AuthorRoleStatus: insightString("complete")},
			&models.ScopusDocumentAuthor{DocumentID: uint(n), AuthorID: 1, AffiliationID: &aff, IsFirstAuthor: insightBool(false), IsCorrespondingAuthor: insightBool(false)},
			&models.ScopusDocumentInsight{DocumentID: uint(n), International: insightBool(true), Status: "complete", CountriesComplete: true, NormalizerVersion: services.ScopusInsightNormalizerVersion},
			&models.ScopusDocumentCountry{DocumentID: uint(n), CountryKey: "japan", CountryName: "Japan"},
		} {
			if err := db.Create(record).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	total, rev := facultyAPITotal(t, "")
	if total != 446 {
		t.Fatal("batch population truncated", total)
	}
	_, b := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "revision="+rev+"&country_key=japan&international_status=yes&faculty_role=coauthor&page=2&page_size=200")
	if b["total"] != float64(200) || len(b["documents"].([]interface{})) != 0 {
		t.Fatal("cross-batch evidence missing", b)
	}
	if err := db.Exec(`UPDATE scopus_source_metrics SET cite_score_quartile='Q3' WHERE source_metric_id=(SELECT MIN(source_metric_id) FROM scopus_source_metrics WHERE source_id='base')`).Error; err != nil {
		t.Fatal(err)
	}
	w, b := facultyAPIRequest(t, AdminGetScopusFacultyInsights, "")
	if w.Code != 503 || b["code"] != "faculty_insights_unavailable" {
		t.Fatal("ambiguous metric joins silently classified", b)
	}
}
