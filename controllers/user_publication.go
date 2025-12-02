package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/teacher/user-publications?year=2025&limit=50&offset=0
func GetUserPublications(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		// fallback for testing: ?user_id=123
		q := c.Query("user_id")
		if q == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "user_id not found"})
			return
		}
		id64, err := strconv.ParseUint(q, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
			return
		}
		userID = uint(id64)
	}

	var yearPtr *int
	if y := c.Query("year"); y != "" {
		if yInt, err := strconv.Atoi(y); err == nil {
			yearPtr = &yInt
		}
	}
	limit := parseIntOrDefault(c.Query("limit"), 50)
	offset := parseIntOrDefault(c.Query("offset"), 0)

	svc := services.NewPublicationService(nil)
	items, total, err := svc.ListByUser(userID, yearPtr, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"paging": gin.H{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

// GET /api/v1/teacher/user-publications/scopus?limit=25&offset=0&sort=year&direction=desc&q=keyword
func GetUserScopusPublications(c *gin.Context) {
	userID, err := resolveTargetUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	limit := parseIntOrDefault(c.Query("limit"), 10)
	offset := parseIntOrDefault(c.Query("offset"), 0)
	sortField := c.DefaultQuery("sort", "year")
	sortDirection := strings.ToLower(c.DefaultQuery("direction", "desc"))
	search := c.Query("q")

	svc := services.NewScopusPublicationService(nil)
	items, total, meta, err := svc.ListByUser(userID, limit, offset, sortField, sortDirection, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"paging": gin.H{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
		"meta": gin.H{
			"has_scopus_id":     meta.HasScopusID,
			"has_author_record": meta.HasAuthor,
		},
	})
}

// GET /api/v1/teacher/user-publications/scopus/stats
func GetUserScopusPublicationStats(c *gin.Context) {
	userID, err := resolveTargetUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	svc := services.NewScopusPublicationService(nil)
	stats, meta, err := svc.StatsByUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
		"meta": gin.H{
			"has_scopus_id":     meta.HasScopusID,
			"has_author_record": meta.HasAuthor,
		},
	})
}

// GET /api/v1/admin/publications/scopus?limit=25&offset=0&sort=year&direction=desc&q=keyword
func AdminListScopusPublications(c *gin.Context) {
	limit := parseIntOrDefault(c.Query("limit"), 10)
	offset := parseIntOrDefault(c.Query("offset"), 0)
	sortField := c.DefaultQuery("sort", "year")
	sortDirection := strings.ToLower(c.DefaultQuery("direction", "desc"))
	search := c.Query("q")

	svc := services.NewScopusPublicationService(nil)
	items, total, err := svc.ListAll(limit, offset, sortField, sortDirection, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"paging": gin.H{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

// POST /api/v1/teacher/user-publications/upsert
// Body: { title, authors, journal, publication_type, publication_date, publication_year, doi, url, source, external_ids, is_verified, fingerprint }
func UpsertUserPublication(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		q := c.Query("user_id")
		if q == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "user_id not found"})
			return
		}
		id64, err := strconv.ParseUint(q, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
			return
		}
		userID = uint(id64)
	}

	var req struct {
		Title           string     `json:"title" binding:"required"`
		Authors         *string    `json:"authors"`
		Journal         *string    `json:"journal"`
		PublicationType *string    `json:"publication_type"`
		PublicationDate *time.Time `json:"publication_date"` // YYYY-MM-DD
		PublicationYear *uint16    `json:"publication_year"`
		DOI             *string    `json:"doi"`
		URL             *string    `json:"url"`
		CitedBy         *uint      `json:"cited_by"`
		CitedByURL      *string    `json:"cited_by_url"`
		CitationHistory *string    `json:"citation_history"`
		Source          *string    `json:"source"`       // scholar|openalex|orcid|crossref
		ExternalIDs     *string    `json:"external_ids"` // JSON string
		IsVerified      *bool      `json:"is_verified"`
		Fingerprint     *string    `json:"fingerprint"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	pub := &models.UserPublication{
		UserID:          userID,
		Title:           req.Title,
		Authors:         req.Authors,
		Journal:         req.Journal,
		PublicationType: req.PublicationType,
		PublicationDate: req.PublicationDate,
		PublicationYear: req.PublicationYear,
		DOI:             req.DOI,
		URL:             req.URL,
		CitedBy:         req.CitedBy,
		CitedByURL:      req.CitedByURL,
		CitationHistory: req.CitationHistory,
		Source:          req.Source,
		ExternalIDs:     req.ExternalIDs,
		Fingerprint:     req.Fingerprint,
		IsVerified:      false,
	}
	if req.IsVerified != nil {
		pub.IsVerified = *req.IsVerified
	}

	svc := services.NewPublicationService(nil)
	created, saved, err := svc.Upsert(pub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "created": created, "data": saved})
}

// DELETE /api/v1/teacher/user-publications/:id  (soft delete)
func DeleteUserPublication(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "user_id not found"})
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	svc := services.NewPublicationService(nil)
	if err := svc.SoftDelete(uint(id64), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// PATCH /api/v1/teacher/user-publications/:id/restore
func RestoreUserPublication(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "user_id not found"})
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	svc := services.NewPublicationService(nil)
	if err := svc.Restore(uint(id64), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ------- helpers (same as before) -------

func getUserIDFromContext(c *gin.Context) (uint, bool) {
	if v, ok := c.Get("userID"); ok {
		switch t := v.(type) {
		case uint:
			return t, true
		case int:
			if t > 0 {
				return uint(t), true
			}
		case int64:
			if t > 0 {
				return uint(t), true
			}
		case string:
			if id64, err := strconv.ParseUint(t, 10, 64); err == nil && id64 > 0 {
				return uint(id64), true
			}
		}
	}
	return 0, false
}

// resolveTargetUserID returns the user id to query publications for. Admins can
// explicitly request another user via the user_id query parameter; otherwise we
// fall back to the authenticated user.
func resolveTargetUserID(c *gin.Context) (uint, error) {
	queryUser := strings.TrimSpace(c.Query("user_id"))
	if queryUser != "" {
		if roleRaw, ok := c.Get("roleID"); ok {
			switch role := roleRaw.(type) {
			case int:
				if role == 3 {
					id64, err := strconv.ParseUint(queryUser, 10, 64)
					if err != nil || id64 == 0 {
						return 0, fmt.Errorf("invalid user_id")
					}
					return uint(id64), nil
				}
			case string:
				if parsed, err := strconv.Atoi(role); err == nil && parsed == 3 {
					id64, err := strconv.ParseUint(queryUser, 10, 64)
					if err != nil || id64 == 0 {
						return 0, fmt.Errorf("invalid user_id")
					}
					return uint(id64), nil
				}
			}
		}
	}

	if userID, ok := getUserIDFromContext(c); ok {
		return userID, nil
	}

	if queryUser != "" {
		id64, err := strconv.ParseUint(queryUser, 10, 64)
		if err != nil || id64 == 0 {
			return 0, fmt.Errorf("invalid user_id")
		}
		return uint(id64), nil
	}

	return 0, fmt.Errorf("user_id not found")
}

func parseIntOrDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// GET /api/v1/teacher/user-publications/scholar/search?q=<name/affiliation>
func TeacherScholarAuthorSearch(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(400, gin.H{"success": false, "error": "missing q"})
		return
	}
	hits, err := services.SearchScholarAuthors(q)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": hits})
}
