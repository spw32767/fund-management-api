package controllers

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"fund-management-api/config"
	"fund-management-api/middleware"
	"fund-management-api/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func grantQuery(pattern string, columns []string, rows ...[]driver.Value) *queryStep {
	return &queryStep{kind: stepQuery, pattern: regexp.MustCompile("(?s)" + pattern), columns: columns, rows: rows}
}

func grantExec(pattern string) *queryStep {
	return &queryStep{kind: stepExec, pattern: regexp.MustCompile(pattern), result: scriptedResult{rowsAffected: 1}}
}

func authorizationSteps(codes []string, overrides [][]driver.Value, includeOverrides bool) []*queryStep {
	steps := []*queryStep{
		grantQuery("information_schema.tables", []string{"count"}, []driver.Value{int64(1)}),
		grantQuery("information_schema.tables", []string{"count"}, []driver.Value{int64(1)}),
	}
	rows := make([][]driver.Value, 0, len(codes))
	for _, code := range codes {
		rows = append(rows, []driver.Value{code})
	}
	merge := grantQuery(`SELECT DISTINCT p.code.*INNER JOIN roles r.*r.delete_at IS NULL.*LEFT JOIN user_roles ur.*ur.is_active = 1.*ur.delete_at IS NULL.*rp.role_id = \? OR ur.user_id IS NOT NULL`, []string{"code"}, rows...)
	merge.args = []driver.Value{int64(7), int64(1)}
	steps = append(steps, merge)
	if includeOverrides {
		steps = append(steps, grantQuery("information_schema.tables", []string{"count"}, []driver.Value{int64(1)}), grantQuery("SELECT p.code, up.effect", []string{"code", "effect"}, overrides...))
	}
	return steps
}

func TestRoleGrantResolutionTracksCurrentGrantsAndDenies(t *testing.T) {
	stages := [][]string{
		{"submission.read.own", "access.manage", "dept_head.review.recommend"},
		{"submission.read.own", "access.manage", "dept_head.review.recommend", "projects.manage"},
		{"submission.read.own", "dept_head.review.recommend"},
	}
	var steps []*queryStep
	for _, codes := range stages {
		steps = append(steps, authorizationSteps(codes, [][]driver.Value{{"access.manage", "deny"}, {"report.export", "allow"}}, true)...)
	}
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	service := services.NewAuthorizationService(db)
	for i := range stages {
		codes, err := service.ResolvePermissionCodes(7, 1)
		if err != nil {
			t.Fatal(err)
		}
		set := map[string]bool{}
		for _, code := range codes {
			set[code] = true
		}
		if set["access.manage"] || !set["submission.read.own"] || !set["dept_head.review.recommend"] || !set["report.export"] {
			t.Fatalf("stage %d: %#v", i, codes)
		}
		if set["projects.manage"] != (i == 1) {
			t.Fatalf("changed/revoked role did not take effect: %#v", codes)
		}
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationDatabaseFailureDoesNotRestoreDefaultAdmin(t *testing.T) {
	step := grantQuery("information_schema.tables", []string{"count"})
	step.err = errors.New("database unavailable")
	db, state, cleanup := newScriptedGormDB(t, []*queryStep{step})
	defer cleanup()
	codes, err := services.NewAuthorizationService(db).ResolvePermissionCodes(7, 3)
	if err == nil || len(codes) != 0 {
		t.Fatalf("failed query restored access: %#v %v", codes, err)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}

func TestAdditionalRoleIDsKeepPrimaryAndDeduplicate(t *testing.T) {
	got, err := normalizeAdditionalRoleIDs([]int{4, 3, 1, 3}, 1)
	if err != nil || !reflect.DeepEqual(got, []int{3, 4}) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := normalizeAdditionalRoleIDs([]int{0}, 1); !errors.Is(err, errInvalidAccessRoles) {
		t.Fatal("invalid role accepted")
	}
	var omitted, empty updateUserOverridesRequest
	if err := json.Unmarshal([]byte(`{"overrides":[]}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"overrides":[],"additional_role_ids":[]}`), &empty); err != nil {
		t.Fatal(err)
	}
	if omitted.AdditionalRoleIDs != nil || empty.AdditionalRoleIDs == nil || len(*empty.AdditionalRoleIDs) != 0 {
		t.Fatal("omitted and empty requests must differ")
	}
}

func TestCombinedUserAccessSave(t *testing.T) {
	for _, mode := range []string{"replace", "omit", "clear", "override failure", "audit failure", "invalid role"} {
		t.Run(mode, func(t *testing.T) {
			steps := []*queryStep{
				grantQuery("FROM users u", []string{"user_id", "role_id", "role", "email"}, []driver.Value{int64(7), int64(1), "teacher", "teacher@example.test"}),
				grantQuery("FROM users.*FOR UPDATE", []string{"user_id", "role_id"}, []driver.Value{int64(7), int64(1)}),
				grantQuery("SELECT p.code, up.effect", []string{"code", "effect"}),
			}
			body := `{"overrides":[],"additional_role_ids":[3,4,3,1]}`
			if mode == "omit" {
				body = `{"overrides":[]}`
			}
			if mode == "clear" {
				body = `{"overrides":[],"additional_role_ids":[]}`
			}
			if mode != "omit" {
				if mode != "clear" {
					query := grantQuery("FROM roles.*delete_at IS NULL FOR UPDATE", []string{"role_id", "role"}, []driver.Value{int64(3), "admin"}, []driver.Value{int64(4), "dept_head"})
					if mode == "invalid role" {
						query.rows = nil
					}
					steps = append(steps, query)
				}
				if mode != "invalid role" {
					steps = append(steps, grantQuery("SELECT role_id FROM user_roles", []string{"role_id"}, []driver.Value{int64(3)}), grantExec("UPDATE user_roles SET is_active = 0.*role_id <>"))
					if mode != "clear" {
						for range []int{3, 4} {
							steps = append(steps, grantExec("(?s)INSERT INTO user_roles.*is_primary = 0, is_active = 1"))
						}
					}
					steps = append(steps, grantExec("INSERT INTO `audit_logs`"))
				}
			}
			failed := mode == "override failure" || mode == "audit failure" || mode == "invalid role"
			if mode != "invalid role" {
				update := grantExec("UPDATE user_permissions SET delete_at")
				if mode == "override failure" {
					update.err = errors.New("override update failed")
				}
				steps = append(steps, update)
				if mode != "override failure" {
					audit := grantExec("INSERT INTO `audit_logs`")
					if mode == "audit failure" {
						audit.err = errors.New("audit failed")
					}
					steps = append(steps, audit)
				}
			}
			if !failed {
				steps = append(steps, grantQuery("SELECT p.code, up.effect", []string{"code", "effect"}))
				codes := []string{"submission.read.own"}
				if mode != "clear" {
					codes = append(codes, "access.manage")
				}
				steps = append(steps, authorizationSteps(codes, nil, true)...)
				steps = append(steps, authorizationSteps(codes, nil, false)...)
				roles := grantQuery("FROM user_roles ur", []string{"role_id", "role"})
				if mode != "clear" {
					roles.rows = [][]driver.Value{{int64(3), "admin"}}
				}
				steps = append(steps, roles)
			}
			db, state, cleanup := newScriptedGormDB(t, steps)
			defer cleanup()
			previous := config.DB
			config.DB = db
			defer func() { config.DB = previous }()
			router := gin.New()
			router.PUT("/users/:id/overrides", func(c *gin.Context) {
				c.Set("userID", 99)
				c.Set("roleID", 1)
				c.Set("permissions", []string{"access.manage"})
			}, middleware.RequirePermission("access.manage"), AdminUpdateUserPermissionOverrides)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/users/7/overrides", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			want := http.StatusOK
			if mode == "invalid role" {
				want = http.StatusBadRequest
			} else if failed {
				want = http.StatusInternalServerError
			}
			if response.Code != want {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if !failed {
				var result struct {
					User  accessUserDetail       `json:"user"`
					Roles []accessUserRoleDetail `json:"roles"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.User.RoleID != 1 || len(result.Roles) == 0 || !result.Roles[0].IsPrimary {
					t.Fatalf("primary role changed: %s", response.Body.String())
				}
			}
			if failed && (state.rollbacks != 1 || state.commits != 0) {
				t.Fatalf("partial save: commits=%d rollbacks=%d", state.commits, state.rollbacks)
			}
			if !failed && state.commits != 1 {
				t.Fatal("save was not committed")
			}
			if err := state.verifyComplete(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUserRoleGrantRequiresAccessManage(t *testing.T) {
	router := gin.New()
	router.PUT("/users/:id/overrides", func(c *gin.Context) {
		c.Set("userID", 7)
		c.Set("roleID", 3)
		c.Set("permissions", []string{"access.view"})
	}, middleware.RequirePermission("access.manage"), func(c *gin.Context) { t.Fatal("unauthorized grant reached handler") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/users/7/overrides", strings.NewReader(`{"overrides":[],"additional_role_ids":[3]}`)))
	if response.Code != http.StatusForbidden {
		t.Fatal(response.Code)
	}
}

func TestApprovalDenyCannotBeBypassedByAnotherApprovalPermission(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("roleID", 3)
	c.Set("permissions", []string{"fund.request.approve", "submission.manage.all"})
	if canApproveSubmissionType(c, "publication_reward") {
		t.Fatal("fund approval bypassed denied publication approval")
	}
	if !canApproveSubmissionType(c, "fund_application") {
		t.Fatal("valid fund approval lost")
	}
	c.Set("permissions", []string{"publication.reward.approve", "submission.manage.all"})
	if canApproveSubmissionType(c, "fund_application") {
		t.Fatal("publication approval bypassed denied fund approval")
	}
}

func TestRoleGrantAuditFailureRollsBack(t *testing.T) {
	// A role audit failure occurs before overrides are touched.
	steps := []*queryStep{grantQuery("FROM roles", []string{"role_id", "role"}, []driver.Value{int64(3), "admin"}), grantQuery("FROM user_roles", []string{"role_id"}), grantExec("UPDATE user_roles"), grantExec("(?s)INSERT INTO user_roles")}
	audit := grantExec("INSERT INTO `audit_logs`")
	audit.err = errors.New("audit unavailable")
	steps = append(steps, audit)
	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	err := db.Transaction(func(tx *gorm.DB) error { return replaceAdditionalAccessRoles(tx, c, 7, 1, []int{3}, time.Now()) })
	if err == nil || state.rollbacks != 1 || state.commits != 0 {
		t.Fatalf("%v commits=%d rollbacks=%d", err, state.commits, state.rollbacks)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}
