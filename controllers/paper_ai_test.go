package controllers

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

func TestSuggestPaperSDGMapsNumberToActiveDatabaseID(t *testing.T) {
	db, state, cleanup := newScriptedGormDB(t, []*queryStep{{
		kind:    stepQuery,
		pattern: regexp.MustCompile(`(?is)SELECT .* FROM .*sdgs.*delete_at IS NULL`),
		columns: []string{"sdg_id", "sdg_number", "name_th", "name_en"},
		rows:    [][]driver.Value{{int64(42), int64(4), "การศึกษาที่มีคุณภาพ", "Quality Education"}},
	}})
	defer cleanup()
	originalDB := config.DB
	config.DB = db
	defer func() { config.DB = originalDB }()
	reader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			SDGs []struct {
				SDGNumber int `json:"sdg_number"`
			} `json:"sdgs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.SDGs) != 1 || request.SDGs[0].SDGNumber != 4 {
			t.Errorf("unexpected reader payload: %+v, %v", request, err)
		}
		_, _ = w.Write([]byte(`{"sdg_number":4,"reason_th":"ศึกษาการเรียนรู้","relationship":"direct","model":"fake"}`))
	}))
	defer reader.Close()
	t.Setenv("PAPER_READER_API_URL", reader.URL)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/suggest-sdg", SuggestPaperSDG)
	request := httptest.NewRequest(http.MethodPost, "/suggest-sdg", strings.NewReader(`{"title":"Learning study","abstract":"Classroom outcomes"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		SDGID int `json:"sdg_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.SDGID != 42 {
		t.Fatalf("expected database ID 42, got %+v: %v", result, err)
	}
	if err := state.verifyComplete(); err != nil {
		t.Fatal(err)
	}
}

func TestStartPaperSDGJobReturnsBeforeReaderCompletes(t *testing.T) {
	db, state, cleanup := newScriptedGormDB(t, []*queryStep{
		{kind: stepQuery, pattern: regexp.MustCompile(`(?is)SELECT .* FROM .*sdgs.*delete_at IS NULL`),
			columns: []string{"sdg_id", "sdg_number", "name_th", "name_en"},
			rows:    [][]driver.Value{{int64(42), int64(4), "การศึกษาที่มีคุณภาพ", "Quality Education"}}},
		{kind: stepExec, pattern: regexp.MustCompile(`(?is)INSERT INTO .*paper_ai_jobs`)},
		{kind: stepExec, pattern: regexp.MustCompile(`(?is)UPDATE .*paper_ai_jobs`)},
	})
	defer cleanup()
	originalDB := config.DB
	config.DB = db
	defer func() { config.DB = originalDB }()
	releaseReader := make(chan struct{})
	reader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-releaseReader
		_, _ = w.Write([]byte(`{"sdg_number":4,"reason_th":"ศึกษาการเรียนรู้","relationship":"direct","model":"fake"}`))
	}))
	defer reader.Close()
	t.Setenv("PAPER_READER_API_URL", reader.URL)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/suggest-sdg/jobs", func(c *gin.Context) { c.Set("userID", 7); StartPaperSDGJob(c) })
	request := httptest.NewRequest(http.MethodPost, "/suggest-sdg/jobs", strings.NewReader(`{"title":"Learning study","abstract":"Classroom outcomes"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		close(releaseReader)
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.JobID == "" {
		close(releaseReader)
		t.Fatalf("missing job ID: %s: %v", response.Body.String(), err)
	}
	close(releaseReader)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if state.verifyComplete() == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(state.verifyComplete())
}

func TestStartPaperExtractJobReturnsBeforeReaderCompletes(t *testing.T) {
	db, state, cleanup := newScriptedGormDB(t, []*queryStep{
		{kind: stepExec, pattern: regexp.MustCompile(`(?is)INSERT INTO .*paper_ai_jobs`)},
		{kind: stepExec, pattern: regexp.MustCompile(`(?is)UPDATE .*paper_ai_jobs`)},
	})
	defer cleanup()
	originalDB := config.DB
	config.DB = db
	defer func() { config.DB = originalDB }()
	releaseReader := make(chan struct{})
	reader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-releaseReader
		_, _ = w.Write([]byte(`{"title":"Example paper","abstract":"Study abstract","text":"full text"}`))
	}))
	defer reader.Close()
	t.Setenv("PAPER_READER_API_URL", reader.URL)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/extract/jobs", func(c *gin.Context) { c.Set("userID", 7); StartPaperExtractJob(c) })
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, err := writer.CreateFormFile("file", "paper.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("%PDF-1.4\nexample"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/extract/jobs", &upload)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		close(releaseReader)
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.JobID == "" {
		close(releaseReader)
		t.Fatalf("missing job ID: %s: %v", response.Body.String(), err)
	}
	close(releaseReader)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if state.verifyComplete() == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(state.verifyComplete())
}

func TestExtractJobResultKeepsMetadataWithoutFullText(t *testing.T) {
	stored := compactPaperAIJobResult(json.RawMessage(`{"title":"Example paper","abstract":"Study abstract","text":"full PDF text"}`))
	var result map[string]any
	if err := json.Unmarshal([]byte(stored), &result); err != nil {
		t.Fatal(err)
	}
	if result["title"] != "Example paper" || result["abstract"] != "Study abstract" || result["text_omitted_from_job_log"] != true {
		t.Fatalf("required metadata missing from job result: %s", stored)
	}
	if _, exists := result["text"]; exists {
		t.Fatalf("full PDF text stored in job result: %s", stored)
	}
}

func TestExtractPaperRelaysReadableValidationError(t *testing.T) {
	reader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":"ไฟล์ที่แนบไม่พบลักษณะของบทความวิจัย"}`))
	}))
	defer reader.Close()
	t.Setenv("PAPER_READER_API_URL", reader.URL)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "not-a-paper.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("%PDF-1.4\nexample"))
	_ = writer.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/extract", ExtractPaper)
	request := httptest.NewRequest(http.MethodPost, "/extract", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error != "ไฟล์ที่แนบไม่พบลักษณะของบทความวิจัย" {
		t.Fatalf("error = %q", result.Error)
	}
}

func TestNormalizePaperDOI(t *testing.T) {
	if got := normalizePaperDOI(" HTTPS://DOI.ORG/10.1000/ABC "); got != "10.1000/abc" {
		t.Fatalf("normalizePaperDOI() = %q", got)
	}
}

func TestPaperTitleSimilarity(t *testing.T) {
	left := normalizePaperTitle("Deep Learning: A Study of Networks")
	right := normalizePaperTitle("Deep Learning - A Study of Networks")
	if score := paperTitleSimilarity(left, right); score != 1 {
		t.Fatalf("expected punctuation-only difference to match exactly, got %v", score)
	}
	different := normalizePaperTitle("Unrelated database optimization research")
	if score := paperTitleSimilarity(left, different); score >= 0.82 {
		t.Fatalf("expected unrelated title below threshold, got %v", score)
	}
}

func TestIsValidClassificationConfidence(t *testing.T) {
	for _, value := range []string{"High", "Medium", "Low", "Preface"} {
		if !isValidClassificationConfidence(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	for _, value := range []string{"", "Needs Review", "0.95", "high"} {
		if isValidClassificationConfidence(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}
