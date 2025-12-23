package controllers

import (
	"fmt"
	"html/template"
	"strings"
)

type emailMetaItem struct {
	Label string
	Value string
}

var basicHTMLReplacer = strings.NewReplacer(
	"&lt;strong&gt;", "<strong>",
	"&lt;/strong&gt;", "</strong>",
)

func buildEmailTemplate(subject string, paragraphs []string, meta []emailMetaItem, buttonText, buttonURL, footerHTML string) string {
	var contentBuilder strings.Builder
	for _, paragraph := range paragraphs {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			continue
		}
		escaped := template.HTMLEscapeString(trimmed)
		escaped = strings.ReplaceAll(strings.ReplaceAll(escaped, "\r\n", "\n"), "\r", "\n")
		escaped = strings.ReplaceAll(escaped, "\n", "<br />")
		escaped = basicHTMLReplacer.Replace(escaped)
		contentBuilder.WriteString(`<p style="margin:0 0 18px 0;line-height:1.7;word-break:break-word;">`)
		contentBuilder.WriteString(escaped)
		contentBuilder.WriteString(`</p>`)
	}

	metaSection := ""
	if len(meta) > 0 {
		var rows []emailMetaItem
		rows = make([]emailMetaItem, 0, len(meta))
		for _, item := range meta {
			label := strings.TrimSpace(item.Label)
			value := strings.TrimSpace(item.Value)
			if label == "" || value == "" {
				continue
			}
			rows = append(rows, emailMetaItem{Label: label, Value: value})
		}
		if len(rows) > 0 {
			var metaBuilder strings.Builder
			metaBuilder.WriteString(`<div style="margin:0 0 24px 0;">
<table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="border:1px solid #e5e7eb;border-radius:12px;background-color:#f9fafb;">
<tbody>`)
			for i, row := range rows {
				border := "border-bottom:1px solid #e5e7eb;"
				if i == len(rows)-1 {
					border = ""
				}
				metaBuilder.WriteString(fmt.Sprintf(`<tr>
<td style="padding:12px 16px;font-size:13px;color:#6b7280;width:38%%;%s;word-break:break-word;">%s</td>
<td style="padding:12px 16px;font-size:15px;color:#111827;font-weight:600;%s;word-break:break-word;white-space:pre-wrap;">%s</td>
</tr>
`, border, template.HTMLEscapeString(row.Label), border, template.HTMLEscapeString(row.Value)))
			}
			metaBuilder.WriteString(`</tbody>
</table>
</div>`)
			metaSection = metaBuilder.String()
		}
	}

	buttonSection := ""
	if strings.TrimSpace(buttonText) != "" && strings.TrimSpace(buttonURL) != "" {
		buttonSection = fmt.Sprintf(`<div style="text-align:center;margin:12px 0 24px 0;">
<a href="%s" style="display:inline-block;padding:12px 28px;background-color:#2563eb;color:#ffffff;text-decoration:none;border-radius:999px;font-weight:600;word-break:break-word;">%s</a>
</div>`, template.HTMLEscapeString(buttonURL), template.HTMLEscapeString(buttonText))
	}

	footerSection := ""
	if strings.TrimSpace(footerHTML) != "" {
		footerSection = fmt.Sprintf(`<div style="color:#6b7280;font-size:13px;line-height:1.7;">%s</div>`, footerHTML)
	}

	logoHTML := strings.TrimSpace(getEmailLogoHTML())
	logoSection := ""
	if logoHTML != "" {
		logoSection = logoHTML
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
</head>
<body style="margin:0;padding:0;background-color:#f9fafb;font-family:'Segoe UI',Tahoma,Arial,sans-serif;">
<div style="max-width:640px;margin:0 auto;padding:24px 20px;">
<div style="background-color:#ffffff;border:1px solid #e5e7eb;border-radius:12px;padding:24px 24px 28px 24px;">
<div style="text-align:center;">
%s
<h1 style="margin:18px 0 0 0;font-size:22px;font-weight:700;color:#111827;line-height:1.35;word-break:break-word;">%s</h1>
</div>
<div style="margin-top:20px;color:#1f2937;font-size:16px;line-height:1.75;word-break:break-word;">
%s
</div>
%s
%s
%s
</div>
</div>
</body>
</html>`, template.HTMLEscapeString(subject), logoSection, template.HTMLEscapeString(subject), contentBuilder.String(), metaSection, buttonSection, footerSection)
}
