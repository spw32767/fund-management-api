package controllers

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	emailLogoEnvPath  = "EMAIL_LOGO_PATH"
	emailLogoEnvURL   = "EMAIL_LOGO_URL"
	emailLogoEnvPaths = "EMAIL_LOGO_PATHS"
	emailLogoEnvURLs  = "EMAIL_LOGO_URLS"
)

var (
	emailLogoOnce sync.Once
	emailLogoHTML string
)

var defaultEmailLogoPaths = []string{
	"uploads/email_assets/iconcpkku.png",
	"uploads/email_assets/fund_cpkku_logo.png",
}

func getEmailLogoHTML() string {
	emailLogoOnce.Do(func() {
		logos := loadLogoHTMLSnippets()
		if len(logos) == 0 {
			emailLogoHTML = ""
			return
		}

		emailLogoHTML = fmt.Sprintf(
			`<div style="display:flex;justify-content:center;align-items:center;gap:18px;margin:0 auto 18px auto;flex-wrap:wrap;">%s</div>`,
			strings.Join(logos, ""),
		)
	})
	return emailLogoHTML
}

func loadLogoHTMLSnippets() []string {
	urls := parseLogoList(os.Getenv(emailLogoEnvURLs))
	if len(urls) == 0 {
		urls = parseLogoList(os.Getenv(emailLogoEnvURL))
	}
	if len(urls) > 0 {
		snippets := make([]string, 0, len(urls))
		for _, url := range urls {
			if snippet := renderLogoURL(url); snippet != "" {
				snippets = append(snippets, snippet)
			}
		}
		return snippets
	}

	paths := parseLogoList(os.Getenv(emailLogoEnvPaths))
	if len(paths) == 0 {
		if single := strings.TrimSpace(os.Getenv(emailLogoEnvPath)); single != "" {
			paths = append(paths, single)
		}
	}

	if len(paths) == 0 {
		paths = append(paths, defaultEmailLogoPaths...)
	}

	snippets := make([]string, 0, len(paths))
	for _, candidate := range paths {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		html, err := renderLogoPath(candidate)
		if err != nil {
			log.Printf("email header logo not loaded from %s: %v", candidate, err)
			continue
		}
		if html != "" {
			snippets = append(snippets, html)
		}
	}
	return snippets
}

func parseLogoList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r':
			return true
		}
		return false
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func renderLogoURL(url string) string {
	escaped := template.HTMLEscapeString(strings.TrimSpace(url))
	if escaped == "" {
		return ""
	}
	return fmt.Sprintf(`<img src="%s" alt="ระบบบริหารจัดการทุนวิจัย" style="display:block;height:72px;width:auto;" />`, escaped)
}

func renderLogoPath(candidate string) (string, error) {
	path, err := resolveEmailAssetPath(candidate)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf(`<img src="data:image/png;base64,%s" alt="ระบบบริหารจัดการทุนวิจัย" style="display:block;height:72px;width:auto;" />`, encoded), nil
}

func resolveEmailAssetPath(candidate string) (string, error) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "", fmt.Errorf("empty asset path")
	}

	baseDirs := []string{""}
	if wd, err := os.Getwd(); err == nil {
		baseDirs = append(baseDirs, wd)
	}
	if exe, err := os.Executable(); err == nil {
		execDir := filepath.Dir(exe)
		baseDirs = append(baseDirs, execDir)
		baseDirs = append(baseDirs, filepath.Dir(execDir))
		baseDirs = append(baseDirs, filepath.Dir(filepath.Dir(execDir)))
	}

	tried := make([]string, 0, len(baseDirs))

	if filepath.IsAbs(candidate) {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		return "", fmt.Errorf("asset file not found at %s", candidate)
	}

	for _, base := range baseDirs {
		path := candidate
		if base != "" {
			path = filepath.Join(base, candidate)
		}
		tried = append(tried, path)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("asset file not found (tried %s)", strings.Join(tried, ", "))
}
