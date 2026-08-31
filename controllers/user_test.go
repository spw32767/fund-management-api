package controllers

import (
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

func TestGetUsersExcludesTestAccounts(t *testing.T) {
	steps := []*queryStep{
		{
			kind:    stepQuery,
			pattern: regexp.MustCompile(`(?is)SELECT user_id, user_fname, user_lname, email, role_id, position_id, prefix FROM .*users.*delete_at IS NULL.*is_test = \?.*role_id = \?`),
			args:    []driver.Value{int64(0), int64(1)},
			columns: []string{"user_id", "user_fname", "user_lname", "email", "role_id", "position_id", "prefix"},
			rows:    [][]driver.Value{},
		},
	}

	db, state, cleanup := newScriptedGormDB(t, steps)
	defer cleanup()
	originalDB := config.DB
	config.DB = db
	defer func() { config.DB = originalDB }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/users", func(c *gin.Context) {
		c.Set("roleID", 1)
		GetUsers(c)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
