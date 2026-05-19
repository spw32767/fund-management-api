package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	
	"fund-management-api/config" 
)

func SearchPublications(c *gin.Context) {
	q := c.Query("q")
	tab := c.DefaultQuery("tab", "all")
	pageStr := c.DefaultQuery("page", "1")
	
	sources := c.QueryArray("source")
	quartiles := c.QueryArray("quartile")
	tiers := c.QueryArray("tier")
	
	year := c.Query("year")
	author := c.Query("author")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	
	limit := 10
	offset := (page - 1) * limit

	var conditions []string
	var args []interface{}

	if q != "" {
		conditions = append(conditions, "(c.title LIKE ? OR c.abstract LIKE ? OR c.keywords LIKE ?)")
		args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}

	if tab == "teacher" {
		conditions = append(conditions, "c.publication_type = 'faculty'")
	} else if tab == "student" {
		conditions = append(conditions, "c.publication_type = 'student'")
	}

	if len(sources) > 0 {
		placeholders := strings.Repeat("?,", len(sources)-1) + "?"
		conditions = append(conditions, fmt.Sprintf("c.source_name IN (%s)", placeholders))
		for _, s := range sources {
			args = append(args, s)
		}
	}

	if year != "" {
		conditions = append(conditions, "c.publication_year = ?")
		args = append(args, year)
	}

	if len(quartiles) > 0 {
		placeholders := strings.Repeat("?,", len(quartiles)-1) + "?"
		conditions = append(conditions, fmt.Sprintf("c.journal_quartile IN (%s)", placeholders))
		for _, val := range quartiles {
			args = append(args, val)
		}
	}

	if len(tiers) > 0 {
		placeholders := strings.Repeat("?,", len(tiers)-1) + "?"
		conditions = append(conditions, fmt.Sprintf("c.journal_tier IN (%s)", placeholders))
		for _, val := range tiers {
			args = append(args, val)
		}
	}

	if author != "" {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM unified_search_authors a WHERE a.unified_publication_id = c.id AND a.name LIKE ?)")
		args = append(args, "%"+author+"%")
	}

	whereSQL := ""
	if len(conditions) > 0 {
		whereSQL = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM unified_search_contents c %s", whereSQL)
	
	if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var results []map[string]interface{}
	
	dataSQL := fmt.Sprintf(`
		SELECT 
			c.*, 
			GROUP_CONCAT(DISTINCT a.name SEPARATOR ', ') as authors
		FROM unified_search_contents c
		LEFT JOIN unified_search_authors a ON c.id = a.unified_publication_id
		%s
		GROUP BY c.id, c.title, c.abstract, c.source_name, c.publication_type, c.publication_year, c.journal_quartile, c.journal_tier, c.detail_type, c.published_at
		ORDER BY c.published_at DESC
		LIMIT ? OFFSET ?
	`, whereSQL)

	args = append(args, limit, offset)

	if err := config.DB.Raw(dataSQL, args...).Scan(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}