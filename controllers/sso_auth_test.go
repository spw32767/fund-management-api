package controllers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type stepKind int

const (
	stepQuery stepKind = iota
	stepExec
)

type queryStep struct {
	kind    stepKind
	pattern *regexp.Regexp
	args    []driver.Value
	columns []string
	rows    [][]driver.Value
	err     error
	result  driver.Result
}

type scriptedDB struct {
	mu    sync.Mutex
	steps []*queryStep
}

func (db *scriptedDB) next(kind stepKind, query string, args []driver.NamedValue) (*queryStep, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if len(db.steps) == 0 {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}

	step := db.steps[0]
	if step.kind != kind {
		return nil, fmt.Errorf("unexpected query kind for %s", query)
	}
	if !step.pattern.MatchString(query) {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}

	if step.args != nil {
		if len(step.args) != len(args) {
			return nil, fmt.Errorf("unexpected arg count for %s: got %d want %d", query, len(args), len(step.args))
		}
		for i := range args {
			if args[i].Value != step.args[i] {
				return nil, fmt.Errorf("unexpected arg %d for %s: got %v want %v", i, query, args[i].Value, step.args[i])
			}
		}
	}

	db.steps = db.steps[1:]
	return step, nil
}

func (db *scriptedDB) verifyComplete() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(db.steps) != 0 {
		return fmt.Errorf("unmet expectations: %d", len(db.steps))
	}
	return nil
}

type scriptedDriver struct{ db *scriptedDB }

func (d *scriptedDriver) Open(string) (driver.Conn, error) {
	return &scriptedConn{db: d.db}, nil
}

type scriptedConn struct{ db *scriptedDB }

func (c *scriptedConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *scriptedConn) Close() error              { return nil }
func (c *scriptedConn) Begin() (driver.Tx, error) { return scriptedTx{}, nil }
func (c *scriptedConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return scriptedTx{}, nil
}

func (c *scriptedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := c.db.next(stepQuery, query, args)
	if err != nil {
		return nil, err
	}
	if step.err != nil {
		return nil, step.err
	}
	return &scriptedRows{columns: step.columns, rows: step.rows}, nil
}

func (c *scriptedConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return c.QueryContext(context.Background(), query, named)
}

func (c *scriptedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	step, err := c.db.next(stepExec, query, args)
	if err != nil {
		return nil, err
	}
	if step.err != nil {
		return nil, step.err
	}
	if step.result != nil {
		return step.result, nil
	}
	return scriptedResult{}, nil
}

func (c *scriptedConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return c.ExecContext(context.Background(), query, named)
}

type scriptedRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *scriptedRows) Columns() []string { return r.columns }
func (r *scriptedRows) Close() error      { return nil }
func (r *scriptedRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	for i := range dest {
		dest[i] = nil
	}
	for i := range r.rows[r.idx] {
		dest[i] = r.rows[r.idx][i]
	}
	r.idx++
	return nil
}

type scriptedTx struct{}

func (scriptedTx) Commit() error   { return nil }
func (scriptedTx) Rollback() error { return nil }

type scriptedResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r scriptedResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r scriptedResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

func newScriptedGormDB(t *testing.T, steps []*queryStep) (*gorm.DB, *scriptedDB, func()) {
	t.Helper()

	state := &scriptedDB{steps: steps}
	driverName := fmt.Sprintf("scripted_controller_%d", time.Now().UnixNano())
	sql.Register(driverName, &scriptedDriver{db: state})

	sqlDB, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("failed to open sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create gorm db: %v", err)
	}

	cleanup := func() { _ = sqlDB.Close() }
	return gormDB, state, cleanup
}

type stubSSOClient struct {
	result *services.SSOExchangeResult
	err    error
}

func (s stubSSOClient) ExchangeCodeForToken(context.Context, string) (*services.SSOExchangeResult, error) {
	return s.result, s.err
}

func setupSSOCallbackRoute() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/auth/sso/callback", SSOCallback)
	return r
}

func setupLogoutRoute() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/auth/logout", LogoutWithSSORedirect)
	return r
}

func TestSSOCallbackMissingCodeRedirects(t *testing.T) {
	r := setupSSOCallbackRoute()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/login?error=sso_missing_code" {
		t.Fatalf("unexpected redirect location: %s", got)
	}
}

func TestSSOCallbackCreatesTeacherUserAndSetsCookie(t *testing.T) {
	steps := []*queryStep{
		{
			kind:    stepQuery,
			pattern: regexp.MustCompile(`SELECT .* FROM .*users.*email = \?.*LIMIT 1`),
			args:    []driver.Value{"new.teacher@kku.ac.th"},
			columns: []string{"user_id"},
			rows:    [][]driver.Value{},
		},
		{
			kind:    stepExec,
			pattern: regexp.MustCompile(`INSERT INTO .*users.*`),
			result:  scriptedResult{lastInsertID: 1, rowsAffected: 1},
		},
		{
			kind:    stepQuery,
			pattern: regexp.MustCompile(`SELECT .* FROM .*users.*email = \?.*LIMIT 1`),
			args:    []driver.Value{"new.teacher@kku.ac.th"},
			columns: []string{"user_id", "email", "role_id", "delete_at"},
			rows:    [][]driver.Value{{int64(1), "new.teacher@kku.ac.th", int64(1), nil}},
		},
		{
			kind:    stepQuery,
			pattern: regexp.MustCompile(`SELECT .* FROM .*auth_identities.*provider = \?.*provider_subject = \?.*LIMIT 1`),
			args:    []driver.Value{services.DefaultSSOProvider, "immutable-001"},
			columns: []string{"identity_id"},
			rows:    [][]driver.Value{},
		},
		{
			kind:    stepExec,
			pattern: regexp.MustCompile(`INSERT INTO .*auth_identities`),
			result:  scriptedResult{lastInsertID: 1, rowsAffected: 1},
		},
		{
			kind:    stepExec,
			pattern: regexp.MustCompile(`UPDATE .*users.*last_login_at`),
			result:  scriptedResult{rowsAffected: 1},
		},
	}

	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	config.DB = db

	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("AUTH_COOKIE_NAME", "auth_token")

	originalFactory := ssoClientFactory
	ssoClientFactory = func() services.SSOCodeExchanger {
		return stubSSOClient{result: &services.SSOExchangeResult{
			OK:              true,
			Email:           "new.teacher@kku.ac.th",
			ProviderSubject: "immutable-001",
			FirstName:       "New",
			LastName:        "Teacher",
			RawClaims:       []byte(`{"ok":true}`),
		}}
	}
	defer func() { ssoClientFactory = originalFactory }()

	r := setupSSOCallbackRoute()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?code=test-code", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/" {
		t.Fatalf("unexpected redirect location: %s", got)
	}

	setCookie := w.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "auth_token=") {
		t.Fatalf("expected auth cookie in response, got: %s", setCookie)
	}
	if !strings.Contains(setCookie, "HttpOnly") || !strings.Contains(setCookie, "Secure") || !strings.Contains(setCookie, "SameSite=Lax") {
		t.Fatalf("cookie flags missing, got: %s", setCookie)
	}

	if err := state.verifyComplete(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestSSOCallbackMergesExistingUserByEmail(t *testing.T) {
	steps := []*queryStep{
		{
			kind:    stepQuery,
			pattern: regexp.MustCompile(`SELECT .* FROM .*users.*email = \?.*LIMIT 1`),
			args:    []driver.Value{"existing.user@kku.ac.th"},
			columns: []string{"user_id", "email", "role_id", "delete_at"},
			rows:    [][]driver.Value{{int64(7), "existing.user@kku.ac.th", int64(2), nil}},
		},
		{
			kind:    stepQuery,
			pattern: regexp.MustCompile(`SELECT .* FROM .*auth_identities.*provider = \?.*provider_subject = \?.*LIMIT 1`),
			args:    []driver.Value{services.DefaultSSOProvider, "immutable-existing"},
			columns: []string{"identity_id"},
			rows:    [][]driver.Value{},
		},
		{
			kind:    stepExec,
			pattern: regexp.MustCompile(`INSERT INTO .*auth_identities`),
			result:  scriptedResult{lastInsertID: 1, rowsAffected: 1},
		},
		{
			kind:    stepExec,
			pattern: regexp.MustCompile(`UPDATE .*users.*last_login_at`),
			result:  scriptedResult{rowsAffected: 1},
		},
	}

	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	config.DB = db

	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("AUTH_COOKIE_NAME", "auth_token")

	originalFactory := ssoClientFactory
	ssoClientFactory = func() services.SSOCodeExchanger {
		return stubSSOClient{result: &services.SSOExchangeResult{
			OK:              true,
			Email:           "existing.user@kku.ac.th",
			ProviderSubject: "immutable-existing",
			RawClaims:       []byte(`{"ok":true}`),
		}}
	}
	defer func() { ssoClientFactory = originalFactory }()

	r := setupSSOCallbackRoute()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?code=test-code", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/" {
		t.Fatalf("unexpected redirect location: %s", got)
	}

	if err := state.verifyComplete(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestLogoutWithSSORedirect_LocalTokenGoesToLogin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("AUTH_COOKIE_NAME", "auth_token")
	t.Setenv("SSO_APP_ID", "my-app")
	t.Setenv("SSO_LOGOUT_REDIRECT_URL", "/login")

	token, _, err := generateAccessTokenWithMethod(models.User{UserID: 1, Email: "local@example.com", RoleID: 1}, "", AuthMethodLocal)
	if err != nil {
		t.Fatalf("failed to generate local token: %v", err)
	}

	r := setupLogoutRoute()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token, Path: "/"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/login" {
		t.Fatalf("unexpected redirect location: %s", got)
	}
}

func TestLogoutWithSSORedirect_SSOTokenGoesToProvider(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("AUTH_COOKIE_NAME", "auth_token")
	t.Setenv("SSO_ENV", "uat")
	t.Setenv("SSO_APP_ID", "my-app")
	t.Setenv("SSO_LOGOUT_REDIRECT_URL", "/login")

	token, _, err := generateAccessTokenWithMethod(models.User{UserID: 1, Email: "sso@example.com", RoleID: 1}, "", AuthMethodSSO)
	if err != nil {
		t.Fatalf("failed to generate sso token: %v", err)
	}

	r := setupLogoutRoute()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token, Path: "/"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "https://sso-uat-web.kku.ac.th/logout?app=my-app" {
		t.Fatalf("unexpected redirect location: %s", got)
	}
}
