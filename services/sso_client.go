package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultSSOProvider = "kku_sso"
)

type SSOCodeExchanger interface {
	ExchangeCodeForToken(ctx context.Context, code string) (*SSOExchangeResult, error)
}

type SSOExchangeResult struct {
	OK              bool
	AccessToken     string
	Email           string
	ProviderSubject string
	FirstName       string
	LastName        string
	RawClaims       json.RawMessage
}

type SSOClient struct {
	httpClient   *http.Client
	tokenURL     string
	redirectURL  string
	clientID     string
	clientSecret string
}

func NewSSOClientFromEnv() *SSOClient {
	timeout := 10 * time.Second
	if raw := strings.TrimSpace(os.Getenv("SSO_REQUEST_TIMEOUT_SECONDS")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			timeout = time.Duration(seconds) * time.Second
		}
	}

	apiBase := strings.TrimSuffix(getSSOAPIBaseURLFromEnv(), "/")

	return &SSOClient{
		httpClient: &http.Client{Timeout: timeout},
		tokenURL:   apiBase + "/auth.token",
		redirectURL: strings.TrimSpace(
			os.Getenv("SSO_REDIRECT_URL"),
		),
		clientID:     strings.TrimSpace(os.Getenv("SSO_CLIENT_ID")),
		clientSecret: strings.TrimSpace(os.Getenv("SSO_CLIENT_SECRET")),
	}
}

func BuildSSOLoginURLFromEnv() string {
	appID := strings.TrimSpace(os.Getenv("SSO_APP_ID"))
	base := strings.TrimSuffix(getSSOWebBaseURLFromEnv(), "/")
	return fmt.Sprintf("%s/login?app=%s", base, url.QueryEscape(appID))
}

func BuildSSOLogoutURLFromEnv() string {
	appID := strings.TrimSpace(os.Getenv("SSO_APP_ID"))
	base := strings.TrimSuffix(getSSOWebBaseURLFromEnv(), "/")
	return fmt.Sprintf("%s/logout?app=%s", base, url.QueryEscape(appID))
}

func (c *SSOClient) ExchangeCodeForToken(ctx context.Context, code string) (*SSOExchangeResult, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("missing authorization code")
	}

	payload := map[string]string{
		"code":         code,
		"redirectUrl":  c.redirectURL,
		"clientId":     c.clientID,
		"clientSecret": c.clientSecret,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var claims map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sso token exchange returned status %d", resp.StatusCode)
	}

	rawClaims, err := json.Marshal(claims)
	if err != nil {
		return nil, err
	}

	result := &SSOExchangeResult{
		OK:              readBoolClaim(claims, "ok", "success"),
		AccessToken:     readStringClaim(claims, "accessToken", "access_token", "token"),
		Email:           strings.ToLower(strings.TrimSpace(readStringClaim(claims, "email", "mail"))),
		ProviderSubject: strings.TrimSpace(readStringClaim(claims, "immutableId", "immutable_id", "sub", "subject", "id")),
		FirstName: strings.TrimSpace(
			readStringClaim(claims, "givenName", "given_name", "firstName", "first_name", "fname"),
		),
		LastName: strings.TrimSpace(
			readStringClaim(claims, "familyName", "family_name", "lastName", "last_name", "lname", "surname"),
		),
		RawClaims: rawClaims,
	}

	return result, nil
}

func readStringClaim(claims map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := claims[key]
		if !ok || value == nil {
			continue
		}

		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed
			}
		case fmt.Stringer:
			if strings.TrimSpace(typed.String()) != "" {
				return typed.String()
			}
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		case int64:
			return strconv.FormatInt(typed, 10)
		}
	}

	return ""
}

func readBoolClaim(claims map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := claims[key]
		if !ok || value == nil {
			continue
		}

		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			normalized := strings.ToLower(strings.TrimSpace(typed))
			if normalized == "true" || normalized == "1" || normalized == "ok" {
				return true
			}
		case float64:
			return typed != 0
		}
	}

	return false
}

func getSSOEnv() string {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("SSO_ENV")))
	if env == "prod" || env == "production" {
		return "prod"
	}
	return "uat"
}

func getSSOAPIBaseURLFromEnv() string {
	if getSSOEnv() == "prod" {
		return "https://ssonext-api.kku.ac.th"
	}
	return "https://sso-uat-api.kku.ac.th"
}

func getSSOWebBaseURLFromEnv() string {
	if getSSOEnv() == "prod" {
		return "https://ssonext.kku.ac.th"
	}
	return "https://sso-uat-web.kku.ac.th"
}
