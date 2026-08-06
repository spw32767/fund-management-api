package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

func SearchPublications(c *gin.Context) {
	q := c.Query("q")
	tab := c.DefaultQuery("tab", "all")
	pageStr := c.DefaultQuery("page", "1")
	isExport := c.Query("export") == "1"

	sources := c.QueryArray("source")
	yearStart := c.Query("year_start")
	yearEnd := c.Query("year_end")

	quartiles := c.QueryArray("quartile")
	aggTypes := c.QueryArray("agg_type")
	tiers := c.QueryArray("tier")
	projectTypes := c.QueryArray("project_type")
	tracks := c.QueryArray("track")

	sort := c.DefaultQuery("sort", "published_at")
	order := c.DefaultQuery("order", "DESC")
	searchField := c.DefaultQuery("search_field", "all")
	titleQuery := c.Query("title_query")
	authorQuery := c.Query("author_query")
	keywordsQuery := c.Query("keywords_query")
	abstractQuery := c.Query("abstract_query")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	limit := 10
	offset := (page - 1) * limit

	var conditions []string
	var args []interface{}

	// Search field conditions
	if q != "" {
		fieldConditions := map[string][]string{
			"title":    {"c.title LIKE ?"},
			"abstract": {"c.abstract LIKE ?"},
			"keywords": {"c.keywords LIKE ?"},
			"author": {`c.id IN (
				SELECT unified_publication_id FROM unified_search_authors WHERE name LIKE ?
				UNION
				SELECT CONCAT('scopus_', sda.document_id)
				FROM scopus_document_authors sda
				JOIN scopus_authors sa ON sa.id = sda.author_id
				WHERE sa.given_name LIKE ? OR sa.surname LIKE ? OR CONCAT(sa.given_name, ' ', sa.surname) LIKE ?
			)`},
		}
		allFields := []string{
			"c.title LIKE ?",
			"c.abstract LIKE ?",
			"c.keywords LIKE ?",
			`c.id IN (
				SELECT unified_publication_id FROM unified_search_authors WHERE name LIKE ?
				UNION
				SELECT CONCAT('scopus_', sda.document_id)
				FROM scopus_document_authors sda
				JOIN scopus_authors sa ON sa.id = sda.author_id
				WHERE sa.given_name LIKE ? OR sa.surname LIKE ? OR CONCAT(sa.given_name, ' ', sa.surname) LIKE ?
			)`,
		}
		selected, ok := fieldConditions[searchField]
		if !ok {
			selected = allFields
		}
		conditions = append(conditions, fmt.Sprintf("(%s)", strings.Join(selected, " OR ")))
		for _, cond := range selected {
			qCount := strings.Count(cond, "?")
			for i := 0; i < qCount; i++ {
				args = append(args, "%"+q+"%")
			}
		}
	}

	// Per-field queries
	if titleQuery != "" {
		conditions = append(conditions, "c.title LIKE ?")
		args = append(args, "%"+titleQuery+"%")
	}
	if authorQuery != "" {
		conditions = append(conditions, `c.id IN (
			SELECT unified_publication_id FROM unified_search_authors WHERE name LIKE ?
			UNION
			SELECT CONCAT('scopus_', sda.document_id)
			FROM scopus_document_authors sda
			JOIN scopus_authors sa ON sa.id = sda.author_id
			WHERE sa.given_name LIKE ? OR sa.surname LIKE ? OR CONCAT(sa.given_name, ' ', sa.surname) LIKE ?
		)`)
		like := "%" + authorQuery + "%"
		args = append(args, like, like, like, like)
	}
	if keywordsQuery != "" {
		conditions = append(conditions, "c.keywords LIKE ?")
		args = append(args, "%"+keywordsQuery+"%")
	}
	if abstractQuery != "" {
		conditions = append(conditions, "c.abstract LIKE ?")
		args = append(args, "%"+abstractQuery+"%")
	}

	// Tab filter
	if tab == "teacher" {
		conditions = append(conditions, "c.publication_type = 'faculty'")
	}
	if tab == "student" {
		conditions = append(conditions, "c.publication_type = 'student'")
	}

	// Sources filter
	if len(sources) > 0 {
		placeholders := strings.Repeat("?,", len(sources)-1) + "?"
		conditions = append(conditions, fmt.Sprintf("c.source_name IN (%s)", placeholders))
		for _, s := range sources {
			args = append(args, s)
		}
	}

	// Year range filter
	if yearStart != "" || yearEnd != "" {
		start := 1900
		if yearStart != "" {
			if s, err := strconv.Atoi(yearStart); err == nil {
				start = s
			}
		}
		end := time.Now().Year()
		if yearEnd != "" {
			if e, err := strconv.Atoi(yearEnd); err == nil {
				end = e
			}
		}
		conditions = append(conditions, "c.publication_year BETWEEN ? AND ?")
		args = append(args, start, end)
	}

	// Filter blocks (Scopus: aggTypes AND quartiles, TCI tiers) — OR together
	var filterBlocks []string

	// Scopus block
	var scopusParts []string
	if len(aggTypes) > 0 {
		placeholders := strings.Repeat("?,", len(aggTypes)-1) + "?"
		scopusParts = append(scopusParts, fmt.Sprintf("c.detail_type IN (%s)", placeholders))
		for _, a := range aggTypes {
			args = append(args, a)
		}
	}
	if len(quartiles) > 0 {
		var qConditions []string
		var validQ []string
		hasNA := false
		for _, q := range quartiles {
			if q == "N/A" {
				hasNA = true
			} else {
				validQ = append(validQ, q)
			}
		}
		if len(validQ) > 0 {
			placeholders := strings.Repeat("?,", len(validQ)-1) + "?"
			qConditions = append(qConditions, fmt.Sprintf("c.journal_quartile IN (%s)", placeholders))
			for _, vq := range validQ {
				args = append(args, vq)
			}
		}
		if hasNA {
			qConditions = append(qConditions, "(c.journal_quartile IS NULL OR c.journal_quartile = '')")
		}
		if len(qConditions) > 0 {
			scopusParts = append(scopusParts, fmt.Sprintf("(%s)", strings.Join(qConditions, " OR ")))
		}
	}
	if len(scopusParts) > 0 {
		filterBlocks = append(filterBlocks, fmt.Sprintf("(%s)", strings.Join(scopusParts, " AND ")))
	}

	// TCI Tiers block
	if len(tiers) > 0 {
		var tierConditions []string
		var validTiers []string
		hasNotIn := false
		for _, t := range tiers {
			if t == "not_in_tci" {
				hasNotIn = true
			} else {
				validTiers = append(validTiers, t)
			}
		}
		if len(validTiers) > 0 {
			placeholders := strings.Repeat("?,", len(validTiers)-1) + "?"
			tierConditions = append(tierConditions, fmt.Sprintf("c.journal_tier IN (%s)", placeholders))
			for _, vt := range validTiers {
				args = append(args, vt)
			}
		}
		if hasNotIn {
			tierConditions = append(tierConditions, "c.journal_tier IS NULL")
		}
		if len(tierConditions) > 0 {
			filterBlocks = append(filterBlocks, fmt.Sprintf("(%s)", strings.Join(tierConditions, " OR ")))
		}
	}

	if len(filterBlocks) > 0 {
		conditions = append(conditions, fmt.Sprintf("(%s)", strings.Join(filterBlocks, " OR ")))
	}

	// Project types filter
	if len(projectTypes) > 0 {
		placeholders := strings.Repeat("?,", len(projectTypes)-1) + "?"
		conditions = append(conditions, fmt.Sprintf("c.detail_type IN (%s)", placeholders))
		for _, pt := range projectTypes {
			args = append(args, pt)
		}
	}

	// Tracks filter
	if len(tracks) > 0 {
		placeholders := strings.Repeat("?,", len(tracks)-1) + "?"
		conditions = append(conditions, fmt.Sprintf("c.track_id IN (%s)", placeholders))
		for _, t := range tracks {
			args = append(args, t)
		}
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count query
	var total int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM unified_search_contents c %s", whereClause)
	if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		InternalError(c, "publication_search", err)
		return
	}

	// Year stats query
	yearConditions := []string{}
	if tab == "teacher" {
		yearConditions = append(yearConditions, "publication_type = 'faculty'")
	}
	if tab == "student" {
		yearConditions = append(yearConditions, "publication_type = 'student'")
	}
	yearWhere := ""
	if len(yearConditions) > 0 {
		yearWhere = "WHERE " + strings.Join(yearConditions, " AND ")
	}
	var yearStats []map[string]interface{}
	yearStatsSQL := fmt.Sprintf("SELECT MIN(publication_year) as min_year, MAX(publication_year) as max_year FROM unified_search_contents %s", yearWhere)
	if err := config.DB.Raw(yearStatsSQL).Scan(&yearStats).Error; err != nil {
		InternalError(c, "publication_search", err)
		return
	}
	minYear := interface{}(nil)
	maxYear := interface{}(nil)
	if len(yearStats) > 0 {
		minYear = yearStats[0]["min_year"]
		maxYear = yearStats[0]["max_year"]
	}

	// Sort options
	trackOrder := `CASE c.track_id WHEN 'ag' THEN 1 WHEN 'cola' THEN 2 WHEN 'cp' THEN 3 WHEN 'kkbs' THEN 4 WHEN 'md' THEN 5 ELSE 99 END`
	sortable := map[string]string{
		"published_at":      "c.published_at",
		"publication_year":  "c.publication_year",
		"cited_by":          "c.cited_by",
		"journal_quartile":  "c.journal_quartile",
		"group_code":        "c.group_code",
		"track_id":          `CASE c.track_id WHEN 'ag' THEN 'คณะเกษตรศาสตร์' WHEN 'cola' THEN 'วิทยาลัยการปกครองท้องถิ่น' WHEN 'cp' THEN 'วิทยาลัยการคอมพิวเตอร์' WHEN 'kkbs' THEN 'คณะบริหารธุรกิจและการบัญชี' WHEN 'md' THEN 'คณะแพทยศาสตร์' ELSE c.track_id END`,
	}
	sortCol, ok := sortable[sort]
	if !ok {
		sortCol = "c.published_at"
	}
	sortDir := "DESC"
	if order == "ASC" {
		sortDir = "ASC"
	}

	var orderSQL string
	if sort == "published_at" && sortDir == "DESC" {
		orderSQL = fmt.Sprintf("%s ASC, c.published_at DESC", trackOrder)
	} else {
		orderSQL = fmt.Sprintf("%s %s", sortCol, sortDir)
	}

	// Data query
	var rows []map[string]interface{}
	if isExport {
		dataSQL := fmt.Sprintf("SELECT c.* FROM unified_search_contents c %s ORDER BY %s", whereClause, orderSQL)
		if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil {
			InternalError(c, "publication_search", err)
			return
		}
	} else {
		dataArgs := append(args, limit, offset)
		dataSQL := fmt.Sprintf("SELECT c.* FROM unified_search_contents c %s ORDER BY %s LIMIT ? OFFSET ?", whereClause, orderSQL)
		if err := config.DB.Raw(dataSQL, dataArgs...).Scan(&rows).Error; err != nil {
			InternalError(c, "publication_search", err)
			return
		}
	}

	// Fetch authors for resulting IDs
	authorsMap := make(map[string][]map[string]interface{})
	if len(rows) > 0 {
		var ids []string
		idToRow := make(map[string]bool)
		for _, row := range rows {
			if id, ok := row["id"]; ok {
				idStr := fmt.Sprint(id)
				if !idToRow[idStr] {
					ids = append(ids, idStr)
					idToRow[idStr] = true
				}
			}
		}
		if len(ids) > 0 {
			placeholders := strings.Repeat("?,", len(ids)-1) + "?"
			var authorRows []map[string]interface{}
			authorSQL := fmt.Sprintf(
				"SELECT unified_publication_id, name, role FROM unified_search_authors WHERE unified_publication_id IN (%s) ORDER BY author_seq ASC",
				placeholders,
			)
			authorArgs := make([]interface{}, len(ids))
			for i, id := range ids {
				authorArgs[i] = id
			}
			if err := config.DB.Raw(authorSQL, authorArgs...).Scan(&authorRows).Error; err != nil {
				InternalError(c, "publication_search", err)
				return
			}
			for _, a := range authorRows {
				pid := fmt.Sprint(a["unified_publication_id"])
				authorsMap[pid] = append(authorsMap[pid], a)
			}
		}
	}

	// Build response data
	var results []map[string]interface{}
	for _, row := range rows {
		id := fmt.Sprint(row["id"])
		item := make(map[string]interface{})
		for k, v := range row {
			item[k] = v
		}
		var authors []string
		var advisors []string
		if auths, ok := authorsMap[id]; ok {
			for _, a := range auths {
				role := fmt.Sprint(a["role"])
				name := fmt.Sprint(a["name"])
				if role == "advisor" {
					advisors = append(advisors, name)
				} else {
					authors = append(authors, name)
				}
			}
		}
		if authors == nil {
			authors = []string{}
		}
		if advisors == nil {
			advisors = []string{}
		}
		item["authors"] = authors
		item["advisors"] = advisors
		results = append(results, item)
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	if isExport {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    results,
			"total":   total,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"data":     results,
		"total":    total,
		"page":     page,
		"limit":    limit,
		"min_year": minYear,
		"max_year": maxYear,
	})
}
