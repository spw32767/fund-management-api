package controllers

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fund-management-api/config"
	"github.com/gin-gonic/gin"
)

func TestAdminUserTestVisibility(t *testing.T) {
	for _, endpoint := range []string{"list", "access search"} {
		for _, visibility := range []string{"", "false", "true", "invalid"} {
			t.Run(endpoint+"/"+visibility, func(t *testing.T) {
				include := visibility == "true"
				predicate := `WHERE .*u.is_test = \?`
				var steps []*queryStep
				rows := [][]driver.Value{{int64(1), "Teacher", "teacher@example.test", false}}
				if include {
					rows = append(rows, []driver.Value{int64(2), "Test", "teacher-test@example.test", true})
				}
				if endpoint == "list" {
					pattern := `SELECT count\(\*\).*FROM users AS u.*` + predicate
					if include {
						pattern = `SELECT count\(\*\).*FROM users AS u.*WHERE u.delete_at IS NULL$`
					}
					count := grantQuery(pattern, []string{"count"}, []driver.Value{int64(len(rows))})
					count.args = []driver.Value{int64(0)}
					if include {
						count.args = []driver.Value{}
					}
					steps = append(steps, count)
				}
				pattern := `SELECT .*u.is_test.*FROM users AS u.*` + predicate
				if include {
					pattern = `SELECT .*u.is_test.*FROM users AS u.*WHERE u.delete_at IS NULL`
				}
				selectRows := grantQuery(pattern, []string{"user_id", "user_fname", "email", "is_test"}, rows...)
				if endpoint == "access search" {
					selectRows.args = []driver.Value{int64(0), "%teacher%", int64(100)}
					if include {
						selectRows.args = []driver.Value{"%teacher%", int64(100)}
					}
				} else {
					selectRows.args = []driver.Value{int64(0), int64(20)}
					if include {
						selectRows.args = []driver.Value{int64(20)}
					}
				}
				steps = append(steps, selectRows)
				db, state, cleanup := newScriptedGormDB(t, steps)
				defer cleanup()
				previous := config.DB
				config.DB = db
				defer func() { config.DB = previous }()
				router := gin.New()
				handler := AdminListManagedUsers
				query := "?include_test=" + visibility
				if endpoint == "access search" {
					handler = AdminSearchAccessUsers
					query += "&q=Teacher"
				}
				router.GET("/users", handler)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users"+query, nil))
				if response.Code != http.StatusOK {
					t.Fatalf("status %d: %s", response.Code, response.Body.String())
				}
				var result struct {
					Data []struct {
						IsTest bool `json:"is_test"`
					} `json:"data"`
					Pagination struct {
						Total int `json:"total"`
					} `json:"pagination"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Data) != len(rows) {
					t.Fatalf("wrong visibility: %s", response.Body.String())
				}
				if include && !result.Data[1].IsTest {
					t.Fatal("test-account label missing")
				}
				if endpoint == "list" && result.Pagination.Total != len(rows) {
					t.Fatal("pagination ignored visibility filter")
				}
				if err := state.verifyComplete(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
