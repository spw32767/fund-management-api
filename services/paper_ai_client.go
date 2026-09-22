package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxPaperAIResponseBytes = 15 * 1024 * 1024

type PaperAIClient struct {
	readerBaseURL         string
	classificationBaseURL string
	apiKey                string
	httpClient            *http.Client
}

func NewPaperAIClient() *PaperAIClient {
	timeout := 600
	if raw := strings.TrimSpace(os.Getenv("PAPER_AI_TIMEOUT_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	return &PaperAIClient{
		readerBaseURL:         strings.TrimRight(strings.TrimSpace(os.Getenv("PAPER_READER_API_URL")), "/"),
		classificationBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("PAPER_CLASSIFICATION_API_URL")), "/"),
		apiKey:                strings.TrimSpace(os.Getenv("PAPER_AI_API_KEY")),
		httpClient:            &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

func (c *PaperAIClient) ReaderConfigured() bool         { return c.readerBaseURL != "" }
func (c *PaperAIClient) ClassificationConfigured() bool { return c.classificationBaseURL != "" }

func (c *PaperAIClient) do(req *http.Request) (int, json.RawMessage, error) {
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPaperAIResponseBytes+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if len(body) > maxPaperAIResponseBytes {
		return resp.StatusCode, nil, fmt.Errorf("AI response exceeds size limit")
	}
	if !json.Valid(body) {
		return resp.StatusCode, nil, fmt.Errorf("AI service returned invalid JSON")
	}
	return resp.StatusCode, json.RawMessage(body), nil
}

func (c *PaperAIClient) postJSON(ctx context.Context, baseURL, path string, payload []byte) (int, json.RawMessage, error) {
	if baseURL == "" {
		return 0, nil, fmt.Errorf("AI service URL is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *PaperAIClient) Extract(ctx context.Context, filename string, data []byte) (int, json.RawMessage, error) {
	if !c.ReaderConfigured() {
		return 0, nil, fmt.Errorf("paper reader URL is not configured")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return 0, nil, err
	}
	if _, err := part.Write(data); err != nil {
		return 0, nil, err
	}
	if err := writer.Close(); err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.readerBaseURL+"/v1/papers/extract", &body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return c.do(req)
}

func (c *PaperAIClient) Summarize(ctx context.Context, payload []byte) (int, json.RawMessage, error) {
	return c.postJSON(ctx, c.readerBaseURL, "/v1/papers/summarize", payload)
}

func (c *PaperAIClient) Classify(ctx context.Context, payload []byte) (int, json.RawMessage, error) {
	return c.postJSON(ctx, c.classificationBaseURL, "/v1/papers/classify", payload)
}
