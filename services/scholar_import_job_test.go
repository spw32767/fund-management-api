package services

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type stepKind int

const (
	kindQuery stepKind = iota
	kindExec
)

type queryStep struct {
	kind    stepKind
	pattern *regexp.Regexp
	args    []driver.Value
	delay   time.Duration
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
		return nil, fmt.Errorf("unexpected kind for query %s: got %v want %v", query, kind, step.kind)
	}
	if !step.pattern.MatchString(query) {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	if len(step.args) != len(args) {
		return nil, fmt.Errorf("unexpected arg count for %s: got %d want %d", query, len(args), len(step.args))
	}
	for i := range args {
		if args[i].Value != step.args[i] {
			return nil, fmt.Errorf("unexpected arg %d for %s: got %v want %v", i, query, args[i].Value, step.args[i])
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

type scriptedDriver struct {
	db *scriptedDB
}

func (d *scriptedDriver) Open(string) (driver.Conn, error) {
	return &scriptedConn{db: d.db}, nil
}

type scriptedConn struct {
	db *scriptedDB
}

func (c *scriptedConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}

func (c *scriptedConn) Close() error { return nil }

func (c *scriptedConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions not supported")
}

func (c *scriptedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := c.db.next(kindQuery, query, args)
	if err != nil {
		return nil, err
	}
	if step.delay > 0 {
		select {
		case <-time.After(step.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if step.err != nil {
		if errors.Is(step.err, context.Canceled) && ctx.Err() != nil {
			return nil, ctx.Err()
		}
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
	step, err := c.db.next(kindExec, query, args)
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

type scriptedResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r scriptedResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }

func (r scriptedResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

type scriptedRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *scriptedRows) Columns() []string { return r.columns }

func (r *scriptedRows) Close() error { return nil }

func (r *scriptedRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.idx]
	for i := range dest {
		dest[i] = nil
	}
	for i := range row {
		dest[i] = row[i]
	}
	r.idx++
	return nil
}

func newScriptedGormDB(t *testing.T, steps []*queryStep) (*gorm.DB, *scriptedDB, func()) {
	t.Helper()
	state := &scriptedDB{steps: steps}
	driverName := fmt.Sprintf("scripted_%d", time.Now().UnixNano())
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

	cleanup := func() {
		_ = sqlDB.Close()
	}
	return gormDB, state, cleanup
}

func TestRunForAllReleasesLockWhenContextCanceled(t *testing.T) {
	lockName := "scholar_import_job"
	userQueryPattern := regexp.MustCompile(`SELECT user_id, scholar_author_id FROM .*users.*`)

	steps := []*queryStep{
		{
			pattern: regexp.MustCompile(`SELECT GET_LOCK`),
			args:    []driver.Value{lockName},
			columns: []string{"status"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		{
			pattern: userQueryPattern,
			delay:   50 * time.Millisecond,
			columns: []string{"user_id", "scholar_author_id"},
			rows:    [][]driver.Value{},
			err:     context.Canceled,
		},
		{
			pattern: regexp.MustCompile(`SELECT RELEASE_LOCK`),
			args:    []driver.Value{lockName},
			columns: []string{"status"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		{
			pattern: regexp.MustCompile(`SELECT GET_LOCK`),
			args:    []driver.Value{lockName},
			columns: []string{"status"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		{
			pattern: userQueryPattern,
			columns: []string{"user_id", "scholar_author_id"},
			rows:    [][]driver.Value{},
		},
		{
			pattern: regexp.MustCompile(`SELECT RELEASE_LOCK`),
			args:    []driver.Value{lockName},
			columns: []string{"status"},
			rows:    [][]driver.Value{{int64(1)}},
		},
	}

	gormDB, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()

	service := NewScholarImportJobService(gormDB)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, runErr := service.RunForAll(ctx, &ScholarImportAllInput{
			LockName:      lockName,
			TriggerSource: "test",
		})
		errCh <- runErr
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}

	summary, err := service.RunForAll(context.Background(), &ScholarImportAllInput{
		LockName:      lockName,
		TriggerSource: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}
	if summary == nil {
		t.Fatalf("expected summary, got nil")
	}

	if err := state.verifyComplete(); err != nil {
		t.Fatalf("%v", err)
	}
}
