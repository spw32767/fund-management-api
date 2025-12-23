package controllers

import (
	"fmt"
	"html/template"
	"strings"
	"sync"
)

var (
	emailLogoOnce sync.Once
	emailLogoHTML string
)

var defaultEmailLogoURLs = []string{
	"https://api.computing.kku.ac.th//storage/images/1663735797-CPlogo-final-01.png",
	// "http://147.50.230.213:8080/uploads/email_assets/fund_cpkku_logo.png",
}

func getEmailLogoHTML() string {
	emailLogoOnce.Do(func() {
		snippets := make([]string, 0, len(defaultEmailLogoURLs))
		for _, url := range defaultEmailLogoURLs {
			if snippet := renderLogoURL(url); snippet != "" {
				snippets = append(snippets, snippet)
			}
		}

		if len(snippets) == 0 {
			emailLogoHTML = ""
			return
		}

		emailLogoHTML = fmt.Sprintf(
			`<div style="text-align:center;margin:0 auto 18px auto;">%s</div>`,
			strings.Join(snippets, ""),
		)
	})
	return emailLogoHTML
}

func renderLogoURL(url string) string {
	escaped := template.HTMLEscapeString(strings.TrimSpace(url))
	if escaped == "" {
		return ""
	}
	return fmt.Sprintf("<span style=\"display:inline-block;margin:0 12px;\"><img src=\"%s\" alt=\"ระบบบริหารจัดการทุนวิจัย\" style=\"display:block;height:64px;width:auto;max-width:100%%;object-fit:contain;\" /></span>", escaped)
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
