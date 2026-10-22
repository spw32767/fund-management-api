package controllers

import (
	"database/sql/driver"
	"encoding/json"
	"fund-management-api/config"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDocumentScopeRoundTrip(t *testing.T) {
	raw, err := encodeDocumentScope(documentSubcategoryScope{Mode: "include", IDs: []int{59, 59, 37}})
	if err != nil || raw == nil || *raw != `{"mode":"include","ids":[59,37]}` {
		t.Fatalf("unexpected stored scope: %v %v", raw, err)
	}
	if !documentScopeMatches(raw, 59) || documentScopeMatches(raw, 60) || documentScopeMatches(raw, 0) {
		t.Fatal("include scope leaked to another or unspecified fund")
	}
	excluded := `{"mode":"exclude","ids":[59]}`
	if documentScopeMatches(&excluded, 59) || !documentScopeMatches(&excluded, 60) {
		t.Fatal("exclude scope did not preserve other funds")
	}
	if raw, err := encodeDocumentScope(documentSubcategoryScope{Mode: "all", IDs: []int{59}}); err != nil || raw != nil {
		t.Fatal("all must clear saved restrictions")
	}
	for _, scope := range []documentSubcategoryScope{{Mode: "include"}, {Mode: "exclude", IDs: []int{-1}}, {Mode: "unknown"}} {
		if _, err := encodeDocumentScope(scope); err == nil {
			t.Fatalf("invalid scope accepted: %+v", scope)
		}
	}
	bad := `{"mode":"include","ids":"invalid"}`
	if documentScopeMatches(&bad, 59) || !documentScopeMatches(nil, 59) {
		t.Fatal("corrupt scope must not become universal; NULL must keep existing behavior")
	}
}

func TestDocumentTypesSubcategoryScope(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"?fund_type=fund_application&subcategory_id=59", []int{21, 22, 23, 24}},
		{"?fund_type=fund_application&subcategory_id=60", []int{13, 14}},
		{"?fund_type=fund_application", []int{13, 14}},
		{"?fund_type=publication_reward&subcategory_id=59", []int{2}},
		{"", []int{13, 14, 21, 22, 23, 24, 2}}, // reviewer catalog must retain all names
	} {
		t.Run(tc.query, func(t *testing.T) {
			rows := [][]driver.Value{
				{int64(13), "โครงการวิจัย", `["fund_application"]`, `{"mode":"exclude","ids":[59]}`},
				{int64(14), "งบประมาณ", `["fund_application"]`, `{"mode":"exclude","ids":[59]}`},
			}
			for i, name := range []string{"หนังสือแจ้งความประสงค์ยื่นคำขอ", "แบบฟอร์มแจ้งข้อมูลลิขสิทธิ์", "ผลงานที่ใช้ในการยื่นคำขอ", "สำเนาบัตรผู้สร้างสรรค์ทุกคน"} {
				rows = append(rows, []driver.Value{int64(21 + i), name, `["fund_application"]`, `{"mode":"include","ids":[59]}`})
			}
			rows = append(rows, []driver.Value{int64(2), "Full Reprint", `["publication_reward"]`, nil})
			db, state, cleanup := newScriptedGormDB(t, []*queryStep{{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*document_types.*delete_at IS NULL.*ORDER BY document_order"), columns: []string{"document_type_id", "document_type_name", "fund_types", "subcategory_scope"}, rows: rows}})
			defer cleanup()
			original := config.DB
			config.DB = db
			defer func() { config.DB = original }()
			router := gin.New()
			router.GET("/document-types", GetDocumentTypes)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/document-types"+tc.query, nil))
			if response.Code != 200 {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			var result struct {
				Types []struct {
					ID    int                      `json:"document_type_id"`
					Scope documentSubcategoryScope `json:"subcategory_scope"`
				} `json:"document_types"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Types) != len(tc.want) {
				t.Fatalf("unexpected checklist: %s", response.Body.String())
			}
			for i, doc := range result.Types {
				if doc.ID != tc.want[i] {
					t.Fatalf("type %d: got %d, want %d", i, doc.ID, tc.want[i])
				}
				if doc.Scope.Mode == "" {
					t.Fatal("scope metadata missing")
				}
			}
			if err := state.verifyComplete(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDocumentScopeCreateRejectsEmptySelection(t *testing.T) {
	router := gin.New()
	router.POST("/document-types", func(c *gin.Context) { c.Set("roleID", 3); CreateDocumentType(c) })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/document-types", strings.NewReader(`{"document_type_name":"test","code":"test","subcategory_scope":{"mode":"include","ids":[]}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid selection must be rejected before any DB write: %d", response.Code)
	}
}

func TestDocumentScopeOrderUpdatePreservesScope(t *testing.T) {
	db, state, cleanup := newScriptedGormDB(t, []*queryStep{
		{kind: stepQuery, pattern: regexp.MustCompile("SELECT .*document_types.*document_type_id.*LIMIT"), columns: []string{"document_type_id", "subcategory_scope"}, rows: [][]driver.Value{{int64(21), `{"mode":"include","ids":[59]}`}}},
		// Reordering sends no scope field. SQL must not clear it accidentally.
		{kind: stepExec, pattern: regexp.MustCompile("^UPDATE `document_types` SET `document_order`=.+,`update_at`=.+ WHERE `document_type_id` = "), result: scriptedResult{rowsAffected: 1}},
	})
	defer cleanup()
	original := config.DB
	config.DB = db
	defer func() { config.DB = original }()
	router := gin.New()
	router.PUT("/document-types/:id", func(c *gin.Context) { c.Set("roleID", 3); UpdateDocumentType(c) })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/document-types/21", strings.NewReader(`{"document_order":5}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("order update failed: %d %s", response.Code, response.Body.String())
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}
