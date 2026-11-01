package controllers

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"fund-management-api/config"
	"github.com/gin-gonic/gin"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

func importEmailLookup(email string, count int64) *queryStep {
	return &queryStep{
		kind: stepQuery, pattern: regexp.MustCompile("SELECT count\\(\\*\\) FROM `users` WHERE LOWER\\(TRIM\\(email\\)\\) = \\?$"),
		args: []driver.Value{email}, columns: []string{"count"}, rows: [][]driver.Value{{count}},
	}
}

func importUserInsert(err error) *queryStep {
	// Require an ordinary INSERT without any upsert/update clause.
	return &queryStep{kind: stepExec, pattern: regexp.MustCompile("^INSERT INTO `users` \\([^)]+\\) VALUES \\([^)]+\\)$"), err: err,
		result: scriptedResult{lastInsertID: 100, rowsAffected: 1}}
}

func TestAdminImportUsersNeverOverwritesAccounts(t *testing.T) {
	for _, tc := range []struct {
		name           string
		emails         []string
		steps          []*queryStep
		status         int
		message        string
		commits        int
		rollbacks      int
		preview        bool
		skipDuplicates bool
		duplicateRows  int
		newCount       int
	}{
		{
			name: "new users commit together", emails: []string{" First@kku.ac.th ", "second@kku.ac.th"},
			steps:  []*queryStep{importEmailLookup("first@kku.ac.th", 0), importEmailLookup("second@kku.ac.th", 0), importUserInsert(nil), importUserInsert(nil)},
			status: http.StatusOK, message: "นำเข้าผู้ใช้ใหม่สำเร็จ 2 รายการ", commits: 1,
		},
		{
			name: "existing user cancels before any insert", emails: []string{"new@kku.ac.th", " EXISTING@kku.ac.th "},
			steps:  []*queryStep{importEmailLookup("new@kku.ac.th", 0), importEmailLookup("existing@kku.ac.th", 1)},
			status: http.StatusConflict, message: "พบผู้ใช้ซ้ำ", rollbacks: 1, duplicateRows: 1,
		},
		{
			name: "duplicate within file ignores case and whitespace", emails: []string{"same@kku.ac.th", " SAME@KKU.AC.TH "},
			steps:  []*queryStep{importEmailLookup("same@kku.ac.th", 0)},
			status: http.StatusConflict, message: "พบผู้ใช้ซ้ำ", rollbacks: 1, duplicateRows: 2,
		},
		{
			name: "preview lists every duplicate without inserting", emails: []string{"existing@kku.ac.th", "new@kku.ac.th", " SAME@kku.ac.th ", "same@kku.ac.th", "other@kku.ac.th"},
			steps:  []*queryStep{importEmailLookup("existing@kku.ac.th", 1), importEmailLookup("new@kku.ac.th", 0), importEmailLookup("same@kku.ac.th", 0), importEmailLookup("other@kku.ac.th", 1)},
			status: http.StatusOK, message: "duplicates", preview: true, duplicateRows: 4, newCount: 1, rollbacks: 1,
		},
		{
			name: "preview with no duplicates is still read only", emails: []string{"new@kku.ac.th"},
			steps:  []*queryStep{importEmailLookup("new@kku.ac.th", 0)},
			status: http.StatusOK, message: "duplicates", preview: true, newCount: 1, rollbacks: 1,
		},
		{
			name: "confirmed import skips existing and all repeated rows", emails: []string{"existing@kku.ac.th", "new@kku.ac.th", "same@kku.ac.th", "SAME@kku.ac.th"},
			steps:  []*queryStep{importEmailLookup("existing@kku.ac.th", 1), importEmailLookup("new@kku.ac.th", 0), importEmailLookup("same@kku.ac.th", 0), importUserInsert(nil)},
			status: http.StatusOK, message: "นำเข้าผู้ใช้ใหม่สำเร็จ 1 รายการ", skipDuplicates: true, commits: 1,
		},
		{
			name: "all existing users cause zero inserts even when confirmed", emails: []string{"existing@kku.ac.th"},
			steps:  []*queryStep{importEmailLookup("existing@kku.ac.th", 1)},
			status: http.StatusOK, message: "นำเข้าผู้ใช้ใหม่สำเร็จ 0 รายการ", skipDuplicates: true, commits: 1,
		},
		{
			name: "concurrent duplicate rolls back earlier insert", emails: []string{"first@kku.ac.th", "race@kku.ac.th"},
			steps:  []*queryStep{importEmailLookup("first@kku.ac.th", 0), importEmailLookup("race@kku.ac.th", 0), importUserInsert(nil), importUserInsert(&mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry"})},
			status: http.StatusConflict, message: "มีผู้ใช้อยู่แล้วหรือข้อมูลซ้ำ", rollbacks: 1,
		},
		{
			name: "lookup failure cannot insert", emails: []string{"first@kku.ac.th"},
			steps:  []*queryStep{{kind: stepQuery, pattern: regexp.MustCompile("SELECT count"), err: errors.New("database unavailable")}},
			status: http.StatusInternalServerError, message: "ไม่สามารถตรวจสอบผู้ใช้ซ้ำได้", rollbacks: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			db, state, cleanup := newScriptedGormDB(t, tc.steps)
			defer cleanup()
			oldDB := config.DB
			config.DB = db
			defer func() { config.DB = oldDB }()

			book := excelize.NewFile()
			defer book.Close()
			rows := [][]interface{}{{"user_fname", "user_lname", "email", "role_id", "position_title"}}
			for _, email := range tc.emails {
				rows = append(rows, []interface{}{"New", "Person", email, 1, "อาจารย์"})
			}
			for i := range rows {
				if err := book.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+1), &rows[i]); err != nil {
					t.Fatal(err)
				}
			}
			xlsx, err := book.WriteToBuffer()
			if err != nil {
				t.Fatal(err)
			}
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			if tc.skipDuplicates {
				if err := writer.WriteField("skip_duplicates", "true"); err != nil {
					t.Fatal(err)
				}
			}
			part, err := writer.CreateFormFile("file", "users.xlsx")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(xlsx.Bytes()); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/import", func(c *gin.Context) {
				c.Set("permissions", []string{"users.manage"})
				if tc.preview {
					AdminPreviewImportUsers(c)
				} else {
					AdminImportUsers(c)
				}
			})
			req := httptest.NewRequest(http.MethodPost, "/import", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.message) {
				t.Fatalf("status %d, body %s", response.Code, response.Body.String())
			}
			if tc.status == http.StatusConflict {
				var payload map[string]interface{}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload["code"] != "USER_ALREADY_EXISTS" {
					t.Fatalf("expected duplicate user response, got %s", response.Body.String())
				}
			}
			if tc.preview || tc.duplicateRows > 0 {
				var payload struct {
					Duplicates []struct {
						Row    int    `json:"row"`
						Name   string `json:"name"`
						Email  string `json:"email"`
						Reason string `json:"reason"`
					} `json:"duplicates"`
					NewCount int `json:"new_count"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if len(payload.Duplicates) != tc.duplicateRows {
					t.Fatalf("expected %d duplicate rows, got %#v", tc.duplicateRows, payload.Duplicates)
				}
				for _, item := range payload.Duplicates {
					if item.Row < 2 || item.Name != "New Person" || item.Email == "" || item.Reason == "" {
						t.Fatalf("incomplete duplicate details: %#v", item)
					}
				}
				if tc.preview && payload.NewCount != tc.newCount {
					t.Fatalf("new_count=%d want %d", payload.NewCount, tc.newCount)
				}
			}
			if err := state.verifyComplete(); err != nil {
				t.Fatal(err)
			}
			if state.commits != tc.commits || state.rollbacks != tc.rollbacks {
				t.Fatalf("commits=%d rollbacks=%d; want %d/%d", state.commits, state.rollbacks, tc.commits, tc.rollbacks)
			}
		})
	}
}
