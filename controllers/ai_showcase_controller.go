package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

const externalAPIURL = "https://ai-dday.computing.kku.ac.th/project/__data.json"
const currentYear = 2026

var trackMap = map[string]struct {
	NameTH string `json:"name_th"`
	NameEN string `json:"name_en"`
}{
	"ag":   {NameTH: "คณะเกษตรศาสตร์", NameEN: "Faculty of Agriculture"},
	"cola": {NameTH: "วิทยาลัยการปกครองท้องถิ่น", NameEN: "College of Local Administration"},
	"cp":   {NameTH: "วิทยาลัยการคอมพิวเตอร์", NameEN: "College of Computing"},
	"kkbs": {NameTH: "คณะบริหารธุรกิจและการบัญชี", NameEN: "Faculty of Business Administration and Accountancy"},
	"md":   {NameTH: "คณะแพทยศาสตร์", NameEN: "Faculty of Medicine"},
}

type svelteKitMember struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type mappedProject struct {
	TitleTH       string
	TitleEN       string
	Abstract      string
	Description   string
	ProjectType   string
	GroupCode     string
	TrackID       string
	PublishedYear int
	Link          string
	PosterURL     string
	Members       []svelteKitMember
}

func parseSvelteKitProjects(rawJSON []byte) ([]map[string]interface{}, error) {
	var doc struct {
		Nodes []struct {
			Data []interface{} `json:"data"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(rawJSON, &doc); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	if len(doc.Nodes) < 2 {
		return nil, fmt.Errorf("unexpected nodes length: %d", len(doc.Nodes))
	}
	data := doc.Nodes[1].Data
	if len(data) < 2 {
		return nil, fmt.Errorf("unexpected data length: %d", len(data))
	}

	var resolve func(idx interface{}) interface{}
	resolve = func(idx interface{}) interface{} {
		idxNum, ok := idx.(float64)
		if !ok || idxNum == 0 {
			return nil
		}
		entryIdx := int(idxNum)
		if entryIdx < 0 || entryIdx >= len(data) {
			return nil
		}
		entry := data[entryIdx]
		if entry == nil {
			return nil
		}
		switch v := entry.(type) {
		case string, float64, bool:
			return v
		case []interface{}:
			result := make([]interface{}, len(v))
			for i, item := range v {
				if itemNum, ok := item.(float64); ok {
					result[i] = resolve(itemNum)
				} else {
					result[i] = item
				}
			}
			return result
		case map[string]interface{}:
			if typ, ok := v["type"]; ok && typ == "data" {
				return v["data"]
			}
			result := make(map[string]interface{})
			for key, val := range v {
				if valNum, ok := val.(float64); ok {
					result[key] = resolve(valNum)
				} else {
					result[key] = val
				}
			}
			return result
		default:
			return entry
		}
	}

	entryIndices, ok := data[1].([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected array at data[1]")
	}

	var projects []map[string]interface{}
	for _, entryIdx := range entryIndices {
		idxNum, ok := entryIdx.(float64)
		if !ok {
			continue
		}
		schemaIdx := int(idxNum)
		if schemaIdx < 0 || schemaIdx >= len(data) {
			continue
		}
		schema, ok := data[schemaIdx].(map[string]interface{})
		if !ok {
			continue
		}
		project := make(map[string]interface{})
		for field, val := range schema {
			if valNum, ok := val.(float64); ok {
				project[field] = resolve(valNum)
			} else {
				project[field] = val
			}
		}
		projects = append(projects, project)
	}
	return projects, nil
}

func mapProject(raw map[string]interface{}) mappedProject {
	p := mappedProject{}

	if name, ok := raw["name"]; ok {
		p.TitleTH = strings.TrimSpace(fmt.Sprint(name))
	}
	if nameEn, ok := raw["name_en"]; ok {
		p.TitleEN = strings.TrimSpace(fmt.Sprint(nameEn))
	}
	if abs, ok := raw["abstract"]; ok && abs != nil {
		p.Abstract = fmt.Sprint(abs)
	}
	if quote, ok := raw["quote"]; ok && quote != nil {
		p.Description = fmt.Sprint(quote)
	}
	if projType, ok := raw["type"]; ok {
		p.ProjectType = strings.TrimSpace(fmt.Sprint(projType))
	}
	if group, ok := raw["group"]; ok {
		p.GroupCode = strings.TrimSpace(fmt.Sprint(group))
	}
	if track, ok := raw["track"]; ok {
		if trackMap, ok := track.(map[string]interface{}); ok {
			if id, ok := trackMap["id"]; ok {
				p.TrackID = strings.TrimSpace(fmt.Sprint(id))
			}
		}
	}

	if ts, ok := raw["timestamp"]; ok && ts != nil {
		tsStr := fmt.Sprint(ts)
		if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
			p.PublishedYear = t.Year()
		} else if t, err := time.Parse("2006-01-02", tsStr); err == nil {
			p.PublishedYear = t.Year()
		} else {
			p.PublishedYear = currentYear
		}
	} else {
		p.PublishedYear = currentYear
	}

	if id, ok := raw["id"]; ok && id != nil {
		idStr := fmt.Sprint(id)
		p.Link = fmt.Sprintf("https://ai-dday.computing.kku.ac.th/project/%s", idStr)
		p.PosterURL = fmt.Sprintf("https://ai-dday.computing.kku.ac.th/project/%s.webp", idStr)
	}

	if membersRaw, ok := raw["members"]; ok {
		switch v := membersRaw.(type) {
		case []interface{}:
			for _, m := range v {
				if mMap, ok := m.(map[string]interface{}); ok {
					member := svelteKitMember{}
					if id, ok := mMap["id"]; ok && id != nil {
						member.ID = strings.TrimSpace(fmt.Sprint(id))
					}
					if name, ok := mMap["name"]; ok && name != nil {
						member.Name = strings.TrimSpace(fmt.Sprint(name))
					}
					p.Members = append(p.Members, member)
				}
			}
		}
	}

	return p
}

func ensureTracks(projects []mappedProject) int {
	seen := make(map[string]bool)
	inserted := 0
	for _, p := range projects {
		if p.TrackID == "" || seen[p.TrackID] {
			continue
		}
		info, ok := trackMap[p.TrackID]
		if !ok {
			seen[p.TrackID] = true
			continue
		}
		seen[p.TrackID] = true
		err := config.DB.Exec(
			"INSERT IGNORE INTO ai_showcase_tracks (id, name_th, name_en) VALUES (?, ?, ?)",
			p.TrackID, info.NameTH, info.NameEN,
		).Error
		if err == nil {
			inserted++
		}
	}
	return inserted
}

func upsertProject(project mappedProject) (string, int64, error) {
	var existingID int64
	var rows []map[string]interface{}

	// Match the AI D-Day source URL first; it is the stable external project key.
	if project.Link != "" {
		err := config.DB.Raw(
			"SELECT id FROM ai_showcase_projects WHERE ai_showcase_link = ? LIMIT 1",
			project.Link,
		).Scan(&rows).Error
		if err != nil {
			return "", 0, fmt.Errorf("find by ai_showcase_link: %w", err)
		}
		if len(rows) > 0 {
			existingID = toInt64(rows[0]["id"])
		}
	}

	// Fallback for legacy rows that may not have a link yet.
	if existingID == 0 {
		err := config.DB.Raw(
			"SELECT id FROM ai_showcase_projects WHERE group_code = ? AND title_th = ? LIMIT 1",
			project.GroupCode, project.TitleTH,
		).Scan(&rows).Error
		if err != nil {
			return "", 0, fmt.Errorf("find by group+title: %w", err)
		}
		if len(rows) > 0 {
			existingID = toInt64(rows[0]["id"])
		}
	}

	if existingID > 0 {
		err := config.DB.Exec(
			`UPDATE ai_showcase_projects SET
				title_th = ?, title_en = ?, abstract = ?, description = ?,
				project_type = ?, track_id = ?, ai_showcase_link = ?, poster_url = ?, updated_at = NOW()
			WHERE id = ?`,
			project.TitleTH, project.TitleEN, project.Abstract, project.Description,
			project.ProjectType, project.TrackID, project.Link, project.PosterURL,
			existingID,
		).Error
		if err != nil {
			return "updated", 0, fmt.Errorf("update: %w", err)
		}
		return "updated", existingID, nil
	}

	result := config.DB.Exec(
		`INSERT INTO ai_showcase_projects
			(title_th, title_en, abstract, description, project_type, group_code, published_year, track_id, ai_showcase_link, poster_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		project.TitleTH, project.TitleEN, project.Abstract, project.Description,
		project.ProjectType, project.GroupCode, project.PublishedYear,
		project.TrackID, project.Link, project.PosterURL,
	)
	if result.Error != nil {
		return "inserted", 0, fmt.Errorf("insert: %w", result.Error)
	}
	// Get the actual insert ID via LAST_INSERT_ID()
	var lastInsertID []map[string]interface{}
	config.DB.Raw("SELECT LAST_INSERT_ID() as id").Scan(&lastInsertID)
	newID := int64(0)
	if len(lastInsertID) > 0 {
		newID = toInt64(lastInsertID[0]["id"])
	}
	return "inserted", newID, nil
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case string:
		var i int64
		fmt.Sscanf(val, "%d", &i)
		return i
	}
	return 0
}

func upsertMembers(projectID int64, members []svelteKitMember) error {
	err := config.DB.Exec("DELETE FROM ai_showcase_project_members WHERE project_id = ?", projectID).Error
	if err != nil {
		return fmt.Errorf("delete members: %w", err)
	}
	for _, member := range members {
		if member.Name == "" && member.ID == "" {
			continue
		}
		err := config.DB.Exec(
			"INSERT INTO ai_showcase_project_members (project_id, student_id, name, role) VALUES (?, ?, ?, 'student')",
			projectID, member.ID, member.Name,
		).Error
		if err != nil {
			return fmt.Errorf("insert member: %w", err)
		}
	}
	return nil
}

func SyncAIShowcase(c *gin.Context) {
	dryRun := c.Query("dry_run") == "true"

	resp, err := http.Get(externalAPIURL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": fmt.Sprintf("fetch failed: %v", err)})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": fmt.Sprintf("API responded with status %d", resp.StatusCode)})
		return
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		InternalError(c, "ai_showcase: read body", err)
		return
	}

	rawProjects, err := parseSvelteKitProjects(raw)
	if err != nil {
		InternalError(c, "ai_showcase: parse", err)
		return
	}

	var projects []mappedProject
	for _, rp := range rawProjects {
		projects = append(projects, mapProject(rp))
	}

	if dryRun {
		trackSet := make(map[string]bool)
		for _, p := range projects {
			if p.TrackID != "" {
				trackSet[p.TrackID] = true
			}
		}
		var tracks []string
		for t := range trackSet {
			tracks = append(tracks, t)
		}
		var projectInfos []gin.H
		for _, p := range projects {
			projectInfos = append(projectInfos, gin.H{
				"title_th":     p.TitleTH,
				"project_type": p.ProjectType,
				"track_id":     p.TrackID,
				"group_code":   p.GroupCode,
				"member_count": len(p.Members),
			})
		}
		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"dry_run":  true,
			"total":    len(projects),
			"tracks":   tracks,
			"projects": projectInfos,
		})
		return
	}

	stats := struct {
		Total        int `json:"total"`
		Inserted     int `json:"inserted"`
		Updated      int `json:"updated"`
		Errors       int `json:"errors"`
		MembersAdded int `json:"members_added"`
		TracksAdded  int `json:"tracks_added"`
	}{Total: len(projects)}

	stats.TracksAdded = ensureTracks(projects)

	for _, project := range projects {
		action, projectID, err := upsertProject(project)
		if err != nil {
			stats.Errors++
			continue
		}
		if action == "inserted" {
			stats.Inserted++
		} else {
			stats.Updated++
		}
		if err := upsertMembers(projectID, project.Members); err != nil {
			stats.Errors++
			continue
		}
		stats.MembersAdded += len(project.Members)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Synced %d projects from AI D-Day API", len(projects)),
		"stats":   stats,
	})
}

func MigrateUnifiedViews(c *gin.Context) {
	migrationSQL := `
CREATE OR REPLACE VIEW unified_search_contents AS

SELECT
    CONCAT('scopus_', d.id) COLLATE utf8mb4_general_ci AS id,
    'scopus' COLLATE utf8mb4_general_ci             AS source_name,
    COALESCE(d.title, 'Untitled')                   AS title,
    NULL                                             AS title_en,
    d.abstract                                       AS abstract,
    'faculty' COLLATE utf8mb4_general_ci             AS publication_type,
    COALESCE(
      YEAR(d.cover_date),
      CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED)
    )                                                AS publication_year,
    d.id                                             AS source_id,
    d.aggregation_type                               AS detail_type,
    d.citedby_count                                  AS cited_by,
    NULL                                             AS track_id,
    CASE WHEN d.aggregation_type = 'Conference Proceeding' THEN NULL ELSE NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), '') END AS journal_quartile,
    metrics.cite_score_percentile                    AS journal_percentile,
    NULL                                             AS journal_tier,
    d.publication_name                               AS journal_name,
    d.scopus_link                                    AS url,
    d.authkeywords                                   AS keywords,
    NULL                                             AS poster_url,
    NULL                                             AS group_code,
    COALESCE(
        d.cover_date,
        MAKEDATE(
            COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED), YEAR(CURRENT_DATE())),
            1
        )
    )                                                AS published_at
FROM scopus_documents d

LEFT JOIN (
    SELECT *,
           ROW_NUMBER() OVER(
               PARTITION BY source_id, metric_year 
               ORDER BY source_metric_id DESC
           ) as rn
    FROM scopus_source_metrics
    WHERE doc_type = 'all'
) metrics
  ON  metrics.source_id   = d.source_id
  AND metrics.rn = 1
  AND metrics.metric_year = COALESCE(
        (
          SELECT ssm_c.metric_year
          FROM   scopus_source_metrics ssm_c
          WHERE  ssm_c.source_id   = d.source_id
            AND  ssm_c.doc_type    = 'all'
            AND  ssm_c.metric_year = COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED))
            AND  LOWER(ssm_c.cite_score_status) = 'complete'
          LIMIT 1
        ),
        CASE
          WHEN EXISTS (
            SELECT 1
            FROM   scopus_source_metrics ssm_ip
            WHERE  ssm_ip.source_id   = d.source_id
              AND  ssm_ip.doc_type    = 'all'
              AND  ssm_ip.metric_year = COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED))
              AND  LOWER(ssm_ip.cite_score_status) = 'in-progress'
          )
          THEN (
            SELECT MAX(ssm_prev.metric_year)
            FROM   scopus_source_metrics ssm_prev
            WHERE  ssm_prev.source_id   = d.source_id
              AND  ssm_prev.doc_type    = 'all'
              AND  ssm_prev.metric_year < COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED))
              AND  LOWER(ssm_prev.cite_score_status) = 'complete'
          )
          ELSE NULL
        END
      )

UNION ALL

SELECT
    CONCAT('thaijo_', d.id) COLLATE utf8mb4_general_ci AS id,
    'thaijo' COLLATE utf8mb4_general_ci              AS source_name,
    COALESCE(d.title_th, d.title_en, 'Untitled')     AS title,
    NULL                                             AS title_en,
    COALESCE(d.abstract_th, d.abstract_en)           AS abstract,
    'faculty' COLLATE utf8mb4_general_ci             AS publication_type,
    d.year                                           AS publication_year,
    d.id                                             AS source_id,
    NULL                                             AS detail_type,
    NULL                                             AS cited_by,
    NULL                                             AS track_id,
    NULL                                             AS journal_quartile,
    NULL                                             AS journal_percentile,
    (
      SELECT j.tier FROM thaijo_journals j WHERE j.journal_id = d.journal_id LIMIT 1
    )                                                AS journal_tier,
    (
      SELECT j.name_th FROM thaijo_journals j WHERE j.journal_id = d.journal_id LIMIT 1
    )                                                AS journal_name,
    d.article_url                                    AS url,
    NULLIF(TRIM(BOTH ',' FROM REPLACE(REPLACE(REPLACE(JSON_UNQUOTE(d.keywords_json), '["', ''), '"]', ''), '","', ', ')), '') AS keywords,
    NULL                                             AS poster_url,
    NULL                                             AS group_code,
    d.date_published                                 AS published_at
FROM thaijo_documents d

UNION ALL

SELECT
    CONCAT('ai_', p.id) COLLATE utf8mb4_general_ci AS id,
    'ai_showcase' COLLATE utf8mb4_general_ci        AS source_name,
    COALESCE(p.title_th, p.title_en, 'Untitled')   AS title,
    p.title_en                                       AS title_en,
    p.abstract                                       AS abstract,
    'student' COLLATE utf8mb4_general_ci             AS publication_type,
    p.published_year                                 AS publication_year,
    p.id                                             AS source_id,
    p.project_type                                   AS detail_type,
    NULL                                             AS cited_by,
    p.track_id                                       AS track_id,
    NULL                                             AS journal_quartile,
    NULL                                             AS journal_percentile,
    NULL                                             AS journal_tier,
    NULL                                             AS journal_name,
    p.ai_showcase_link                               AS url,
    NULL                                             AS keywords,
    p.poster_url                                     AS poster_url,
    p.group_code                                     AS group_code,
    MAKEDATE(p.published_year, 1)                   AS published_at
FROM ai_showcase_projects p;

CREATE OR REPLACE VIEW unified_search_authors AS
SELECT
    CONCAT('scopus_', da.document_id) COLLATE utf8mb4_general_ci  AS unified_publication_id,
    'scopus' COLLATE utf8mb4_general_ci                           AS source_name,
    COALESCE(a.full_name, 'Unknown') COLLATE utf8mb4_general_ci   AS name,
    'author'                                                      AS role,
    da.author_seq
FROM scopus_document_authors da
JOIN scopus_authors a ON a.id = da.author_id
UNION ALL
SELECT
    CONCAT('thaijo_', da.document_id) COLLATE utf8mb4_general_ci  AS unified_publication_id,
    'thaijo' COLLATE utf8mb4_general_ci                           AS source_name,
    COALESCE(da.name_th, da.name_en, 'Unknown') COLLATE utf8mb4_general_ci AS name,
    'author'                                                      AS role,
    da.author_seq
FROM thaijo_document_authors da
UNION ALL
SELECT
    CONCAT('ai_', m.project_id) COLLATE utf8mb4_general_ci        AS unified_publication_id,
    'ai_showcase' COLLATE utf8mb4_general_ci                      AS source_name,
    COALESCE(m.name, 'Unknown') COLLATE utf8mb4_general_ci        AS name,
    m.role COLLATE utf8mb4_general_ci                             AS role,
    NULL                                                          AS author_seq
FROM ai_showcase_project_members m;
`

	statements := strings.Split(migrationSQL, ";")
	for _, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		if err := config.DB.Exec(trimmed + ";").Error; err != nil {
			InternalError(c, "ai_showcase", err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Migration completed successfully",
	})
}

type csvMember struct {
	StudentID string `json:"student_id"`
	Name      string `json:"name"`
}

var trackAliases = map[string]string{
	"ag": "ag", "agriculture": "ag", "คณะเกษตรศาสตร์": "ag",
	"cola": "cola", "college of local administration": "cola", "วิทยาลัยการปกครองท้องถิ่น": "cola",
	"cp": "cp", "college of computing": "cp", "วิทยาลัยการคอมพิวเตอร์": "cp",
	"kkbs": "kkbs", "business administration": "kkbs", "คณะบริหารธุรกิจและการบัญชี": "kkbs",
	"md": "md", "medicine": "md", "คณะแพทยศาสตร์": "md",
}

func normalizeCSVTrack(val string) string {
	v := strings.TrimSpace(strings.ToLower(val))
	if mapped, ok := trackAliases[v]; ok {
		return mapped
	}
	return v
}

func parseCSVMembers(text string) []csvMember {
	if text == "" {
		return nil
	}
	text = strings.TrimSpace(text)
	var members []csvMember

	// Try parenthesized groups first: (id, name), (name)
	parenRe := regexp.MustCompile(`\(([^)]+)\)`)
	parenMatches := parenRe.FindAllStringSubmatch(text, -1)
	if len(parenMatches) > 0 {
		seen := make(map[string]bool)
		for _, m := range parenMatches {
			content := strings.TrimSpace(m[1])
			parts := strings.Split(content, ",")
			var cleaned []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					cleaned = append(cleaned, p)
				}
			}
			if len(cleaned) == 0 {
				continue
			}
			idPart := cleaned[0]
			re := regexp.MustCompile(`\d{7,12}`)
			sid := re.FindString(idPart)
			name := ""
			if len(cleaned) > 1 {
				name = strings.Join(cleaned[1:], ", ")
			} else if sid == "" {
				name = idPart
			}
			name = strings.TrimSpace(name)
			if name != "" && !seen[name] {
				seen[name] = true
				members = append(members, csvMember{StudentID: sid, Name: name})
			}
		}
		if len(members) > 0 {
			return members
		}
	}

	// Fallback: split by comma/semicolon/newline
	segments := regexp.MustCompile(`[,;\n\r]+`).Split(text, -1)
	seen := make(map[string]bool)
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		// Find student IDs in segment
		idRe := regexp.MustCompile(`\b(\d{7,12}(?:-\d+)?)\b`)
		ids := idRe.FindAllStringSubmatchIndex(seg, -1)
		if len(ids) == 0 {
			name := cleanCSVName(seg)
			if name != "" && !seen[name] {
				seen[name] = true
				members = append(members, csvMember{StudentID: "", Name: name})
			}
			continue
		}
		for i, match := range ids {
			idStr := seg[match[2]:match[3]]
			prevEnd := 0
			if i > 0 {
				prevEnd = ids[i-1][1]
			}
			beforeText := strings.TrimSpace(seg[prevEnd:match[0]])
			nextStart := len(seg)
			if i < len(ids)-1 {
				nextStart = ids[i+1][0]
			}
			afterText := strings.TrimSpace(seg[match[1]:nextStart])
			rawName := afterText
			if beforeText != "" && afterText == "" {
				rawName = beforeText
			}
			name := cleanCSVName(rawName)
			if name != "" && !seen[name] {
				seen[name] = true
				members = append(members, csvMember{StudentID: idStr, Name: name})
			}
		}
	}
	return members
}

func parseCSVAdvisors(text string) []csvMember {
	text = strings.TrimSpace(text)
	if text == "" || text == "-" {
		return nil
	}
	parts := strings.Split(text, ",")
	var advisors []csvMember
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name != "" {
			advisors = append(advisors, csvMember{Name: name})
		}
	}
	return advisors
}

func cleanCSVName(raw string) string {
	result := raw
	result = regexp.MustCompile(`^[\s,;.\n\r\-()]+`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`[\s,;.\n\r\-()]+$`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`^\d+\.\s*`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`\b\d{7,12}(?:-\d+)?\b\s*`).ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

func parseCSV(text string) [][]string {
	var lines [][]string
	var current []string
	field := ""
	inQuotes := false
	runes := []rune(text)

	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if inQuotes {
			if ch == '"' && i+1 < len(runes) && runes[i+1] == '"' {
				field += "\""
				i++
			} else if ch == '"' {
				inQuotes = false
			} else {
				field += string(ch)
			}
		} else {
			if ch == '"' {
				inQuotes = true
			} else if ch == ',' {
				current = append(current, strings.TrimSpace(field))
				field = ""
			} else if ch == '\n' {
				current = append(current, strings.TrimSpace(field))
				hasContent := false
				for _, f := range current {
					if f != "" {
						hasContent = true
						break
					}
				}
				if hasContent {
					lines = append(lines, current)
				}
				current = nil
				field = ""
			} else if ch == '\r' {
				// skip
			} else {
				field += string(ch)
			}
		}
	}
	// Last field
	current = append(current, strings.TrimSpace(field))
	hasContent := false
	for _, f := range current {
		if f != "" {
			hasContent = true
			break
		}
	}
	if hasContent {
		lines = append(lines, current)
	}
	return lines
}

func findColumnIndex(headers []string, patterns []string) int {
	for _, pat := range patterns {
		patLower := strings.ToLower(pat)
		for i, h := range headers {
			if strings.Contains(strings.ToLower(h), patLower) {
				return i
			}
		}
	}
	return -1
}

func SyncAIShowcaseFromCSV(c *gin.Context) {
	csvURL := c.Query("csv_url")
	dryRun := c.Query("dry_run") == "true"

	if csvURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Missing csv_url parameter"})
		return
	}

	resp, err := http.Get(csvURL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": fmt.Sprintf("CSV fetch failed: %v", err)})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": fmt.Sprintf("CSV fetch failed: %d", resp.StatusCode)})
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		InternalError(c, "ai_showcase: read body", err)
		return
	}

	text := string(body)
	rows := parseCSV(text)
	if len(rows) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "CSV has no data rows"})
		return
	}

	headers := rows[0]
	dataRows := rows[1:]

	colGroup := findColumnIndex(headers, []string{"ชื่อกลุ่ม", "กลุ่ม", "group"})
	colMembers := findColumnIndex(headers, []string{"รหัสนักศึกษา", "สมาชิก", "member", "student"})
	colTrack := findColumnIndex(headers, []string{"Track", "track"})
	colTitleTh := findColumnIndex(headers, []string{"ชื่อโครงงาน (ภาษาไทย)", "ชื่อโครงงานไทย", "project_th", "title_th"})
	colTitleEn := findColumnIndex(headers, []string{"ชื่อโครงงาน (ภาษาอังกฤษ)", "ชื่อโครงงานอังกฤษ", "project_en", "title_en"})
	colType := findColumnIndex(headers, []string{"ประเภทของโครงงาน", "ประเภท", "type", "project_type"})
	colAbstract := findColumnIndex(headers, []string{"บทคัดย่อ", "abstract"})
	colLink := findColumnIndex(headers, []string{"ลิงก์", "link", "url", "demo"})
	colAdvisor := findColumnIndex(headers, []string{"อาจารย์ที่ปรึกษา", "ที่ปรึกษา", "advisor"})

	publishedYear := 2026

	if dryRun {
		var projects []gin.H
		for _, row := range dataRows {
			groupCode := ""
			titleTh := ""
			trackID := ""
			projectType := ""
			if colGroup >= 0 && colGroup < len(row) {
				groupCode = strings.TrimSpace(row[colGroup])
			}
			if colTitleTh >= 0 && colTitleTh < len(row) {
				titleTh = strings.TrimSpace(row[colTitleTh])
			}
			if colTrack >= 0 && colTrack < len(row) {
				trackID = normalizeCSVTrack(row[colTrack])
			}
			if colType >= 0 && colType < len(row) {
				projectType = strings.TrimSpace(row[colType])
			}
			if groupCode == "" && titleTh == "" {
				continue
			}
			var members []gin.H
			if colMembers >= 0 && colMembers < len(row) {
				for _, m := range parseCSVMembers(row[colMembers]) {
					members = append(members, gin.H{"name": m.Name})
				}
			}
			var advisors []gin.H
			if colAdvisor >= 0 && colAdvisor < len(row) {
				for _, a := range parseCSVAdvisors(row[colAdvisor]) {
					advisors = append(advisors, gin.H{"name": a.Name})
				}
			}
			projects = append(projects, gin.H{
				"group_code":   groupCode,
				"title_th":     titleTh,
				"track_id":     trackID,
				"project_type": projectType,
				"members":      members,
				"advisors":     advisors,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"dry_run":  true,
			"total":    len(projects),
			"projects": projects,
		})
		return
	}

	stats := struct {
		Inserted      int `json:"inserted"`
		Updated       int `json:"updated"`
		Errors        int `json:"errors"`
		StudentsAdded int `json:"students_added"`
		AdvisorsAdded int `json:"advisors_added"`
	}{}

	for _, row := range dataRows {
		groupCode := ""
		titleTh := ""
		titleEn := ""
		trackID := ""
		projectType := ""
		abstract := ""
		link := ""

		if colGroup >= 0 && colGroup < len(row) {
			groupCode = strings.TrimSpace(row[colGroup])
		}
		if colTitleTh >= 0 && colTitleTh < len(row) {
			titleTh = strings.TrimSpace(row[colTitleTh])
		}
		if colTitleEn >= 0 && colTitleEn < len(row) {
			titleEn = strings.TrimSpace(row[colTitleEn])
		}
		if colTrack >= 0 && colTrack < len(row) {
			trackID = normalizeCSVTrack(row[colTrack])
		}
		if colType >= 0 && colType < len(row) {
			projectType = strings.TrimSpace(row[colType])
		}
		if colAbstract >= 0 && colAbstract < len(row) {
			abstract = strings.TrimSpace(row[colAbstract])
		}
		if colLink >= 0 && colLink < len(row) {
			link = strings.TrimSpace(row[colLink])
		}

		if groupCode == "" && titleTh == "" {
			continue
		}

		// Ensure track
		if trackID != "" {
			if info, ok := trackMap[trackID]; ok {
				config.DB.Exec("INSERT IGNORE INTO ai_showcase_tracks (id, name_th, name_en) VALUES (?, ?, ?)",
					trackID, info.NameTH, info.NameEN)
			}
		}

		// Find existing — match by ai_showcase_link first, fallback to group_code + title_th
		var existing []map[string]interface{}
		if link != "" {
			config.DB.Raw("SELECT id FROM ai_showcase_projects WHERE ai_showcase_link = ? LIMIT 1",
				link).Scan(&existing)
		}
		if len(existing) == 0 {
			config.DB.Raw("SELECT id FROM ai_showcase_projects WHERE group_code = ? AND title_th = ? LIMIT 1",
				groupCode, titleTh).Scan(&existing)
		}

		posterURL := ""
		if link != "" {
			posterRe := regexp.MustCompile(`(?i)(canva\.com|drive\.google\.com|\.png|\.jpg|\.jpeg|\.gif|\.webp)`)
			if posterRe.MatchString(link) {
				posterURL = link
			}
		}

		if len(existing) > 0 {
			projectID := toInt64(existing[0]["id"])
			config.DB.Exec(`UPDATE ai_showcase_projects SET
				title_th = ?, title_en = ?, abstract = ?, project_type = ?,
				track_id = ?, updated_at = NOW()
			WHERE id = ?`, titleTh, titleEn, abstract, projectType, trackID, projectID)
			stats.Updated++

			// Members
			config.DB.Exec("DELETE FROM ai_showcase_project_members WHERE project_id = ?", projectID)
			var members []csvMember
			if colMembers >= 0 && colMembers < len(row) {
				members = parseCSVMembers(row[colMembers])
			}
			for _, m := range members {
				if m.Name == "" {
					continue
				}
				config.DB.Exec("INSERT INTO ai_showcase_project_members (project_id, student_id, name, role) VALUES (?, ?, ?, 'student')",
					projectID, m.StudentID, m.Name)
				stats.StudentsAdded++
			}
			var advisors []csvMember
			if colAdvisor >= 0 && colAdvisor < len(row) {
				advisors = parseCSVAdvisors(row[colAdvisor])
			}
			for _, a := range advisors {
				if a.Name == "" {
					continue
				}
				config.DB.Exec("INSERT INTO ai_showcase_project_members (project_id, student_id, name, role) VALUES (?, ?, ?, 'advisor')",
					projectID, a.StudentID, a.Name)
				stats.AdvisorsAdded++
			}
		} else {
			result := config.DB.Exec(`INSERT INTO ai_showcase_projects
				(title_th, title_en, abstract, project_type, group_code, published_year, track_id, ai_showcase_link, poster_url)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				titleTh, titleEn, abstract, projectType, groupCode, publishedYear, trackID, link, posterURL)
			if result.Error != nil {
				stats.Errors++
				continue
			}
			var lastID []map[string]interface{}
			config.DB.Raw("SELECT LAST_INSERT_ID() as id").Scan(&lastID)
			projectID := int64(0)
			if len(lastID) > 0 {
				projectID = toInt64(lastID[0]["id"])
			}
			stats.Inserted++

			var members []csvMember
			if colMembers >= 0 && colMembers < len(row) {
				members = parseCSVMembers(row[colMembers])
			}
			for _, m := range members {
				if m.Name == "" {
					continue
				}
				config.DB.Exec("INSERT INTO ai_showcase_project_members (project_id, student_id, name, role) VALUES (?, ?, ?, 'student')",
					projectID, m.StudentID, m.Name)
				stats.StudentsAdded++
			}
			var advisors []csvMember
			if colAdvisor >= 0 && colAdvisor < len(row) {
				advisors = parseCSVAdvisors(row[colAdvisor])
			}
			for _, a := range advisors {
				if a.Name == "" {
					continue
				}
				config.DB.Exec("INSERT INTO ai_showcase_project_members (project_id, student_id, name, role) VALUES (?, ?, ?, 'advisor')",
					projectID, a.StudentID, a.Name)
				stats.AdvisorsAdded++
			}
		}
	}

	// Get final track count for stats
	var trackCount []map[string]interface{}
	config.DB.Raw("SELECT COUNT(*) as cnt FROM ai_showcase_tracks").Scan(&trackCount)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Synced %d projects from CSV", stats.Inserted+stats.Updated),
		"stats":   stats,
	})
}
