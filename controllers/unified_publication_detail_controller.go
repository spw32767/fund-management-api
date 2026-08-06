package controllers

import (
	"fmt"
	"net/http"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

func GetPublicationDetail(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing id"})
		return
	}

	// Fetch unified_search_contents row
	var items []map[string]interface{}
	if err := config.DB.Raw("SELECT * FROM unified_search_contents WHERE id = ?", id).Scan(&items).Error; err != nil {
		InternalError(c, "publication_detail", err)
		return
	}
	if len(items) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "not found"})
		return
	}
	item := items[0]

	// Fetch authors
	var authorRows []map[string]interface{}
	config.DB.Raw("SELECT name, role FROM unified_search_authors WHERE unified_publication_id = ? ORDER BY author_seq ASC", id).Scan(&authorRows)

	var authors []string
	var advisors []string
	for _, a := range authorRows {
		role := fmt.Sprint(a["role"])
		name := fmt.Sprint(a["name"])
		if role == "advisor" {
			advisors = append(advisors, name)
		} else {
			authors = append(authors, name)
		}
	}
	if authors == nil {
		authors = []string{}
	}
	if advisors == nil {
		advisors = []string{}
	}
	if authorRows == nil {
		authorRows = []map[string]interface{}{}
	}

	sourceName := fmt.Sprint(item["source_name"])
	sourceID := item["source_id"]

	response := gin.H{
		"item":     item,
		"authors":  authors,
		"advisors": advisors,
	}

	// Source-specific detail
	switch sourceName {
	case "scopus":
		var scopusRows []map[string]interface{}
		err := config.DB.Raw(`
			SELECT doi, issn, eissn, volume, issue, page_range, article_number, aggregation_type, raw_json
			FROM scopus_documents WHERE id = ? LIMIT 1
		`, sourceID).Scan(&scopusRows).Error
		if err == nil && len(scopusRows) > 0 {
			response["scopus_detail"] = scopusRows[0]
		} else {
			response["scopus_detail"] = nil
		}

	case "thaijo":
		var thaijoRows []map[string]interface{}
		err := config.DB.Raw(`
			SELECT
				j.name_th AS journal_name_th,
				j.name_en AS journal_name_en,
				j.category AS journal_category,
				j.acronym AS journal_acronym,
				j.online_issn,
				j.print_issn,
				j.tier AS journal_tier,
				j.tier_period,
				j.path AS journal_path,
				j.journal_url,
				d.title_en,
				d.pdf_url,
				d.doi
			FROM thaijo_documents d
			JOIN thaijo_journals j ON d.journal_id = j.journal_id
			WHERE d.id = ?
			LIMIT 1
		`, sourceID).Scan(&thaijoRows).Error
		if err == nil && len(thaijoRows) > 0 {
			response["thaijo_detail"] = thaijoRows[0]
		} else {
			response["thaijo_detail"] = nil
		}

	case "ai_showcase":
		var aiRows []map[string]interface{}
		err := config.DB.Raw(`
			SELECT group_code, poster_url, title_en FROM ai_showcase_projects WHERE id = ? LIMIT 1
		`, sourceID).Scan(&aiRows).Error
		if err == nil && len(aiRows) > 0 {
			aiDetail := aiRows[0]
			var members []map[string]interface{}
			config.DB.Raw(`
				SELECT name, student_id FROM ai_showcase_project_members WHERE project_id = ? AND role = 'student'
			`, sourceID).Scan(&members)
			if members == nil {
				members = []map[string]interface{}{}
			}
			aiDetail["members"] = members
			response["ai_detail"] = aiDetail
		} else {
			response["ai_detail"] = nil
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}
