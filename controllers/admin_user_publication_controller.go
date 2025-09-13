package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// POST /api/v1/admin/user-publications/import/scholar?user_id=123&author_id=W2k2JXwAAAAJ
func AdminImportScholarPublications(c *gin.Context) {
	uid := c.Query("user_id")
	authorID := c.Query("author_id")
	if uid == "" || authorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing user_id or author_id"})
		return
	}

	id64, err := strconv.ParseUint(uid, 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
		return
	}
	userID := uint(id64)

	// 1) Run script once
	pubs, err := services.FetchScholarOnce(authorID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": fmt.Sprintf("script error: %v", err)})
		return
	}

	// 2) Upsert into DB
	svc := services.NewPublicationService(nil)
	created, updated, failed := 0, 0, 0

	for _, sp := range pubs {
		title := sp.Title
		authorsStr := strings.Join(sp.Authors, ", ")
		source := "scholar"

		var journal *string
		if sp.Venue != nil && *sp.Venue != "" {
			journal = sp.Venue
		}

		var yearPtr *uint16
		if sp.Year != nil && *sp.Year > 0 {
			yy := uint16(*sp.Year)
			yearPtr = &yy
		}

		var externalJSON *string
		if sp.ScholarClusterID != nil && *sp.ScholarClusterID != "" {
			js := fmt.Sprintf(`{"scholar_cluster_id":"%s"}`, *sp.ScholarClusterID)
			externalJSON = &js
		}

		pub := &models.UserPublication{
			UserID:          userID,
			Title:           title,
			Authors:         &authorsStr,
			Journal:         journal,
			PublicationType: nil,
			PublicationDate: nil,
			PublicationYear: yearPtr,
			DOI:             sp.DOI,
			URL:             sp.URL,
			Source:          &source,
			ExternalIDs:     externalJSON,
			// Fingerprint is auto-computed by model hook if missing
		}

		ok, _, e := svc.Upsert(pub)
		if e != nil {
			failed++
			continue
		}
		if ok {
			created++
		} else {
			updated++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"summary": gin.H{
			"fetched": len(pubs),
			"created": created,
			"updated": updated,
			"failed":  failed,
		},
	})
}

// POST /api/v1/admin/user-publications/import/scholar/all
// Optional: ?user_ids=1,2,3  (CSV subset)
// Optional: ?limit=50  (max users to process in one call)
func AdminImportScholarForAll(c *gin.Context) {
	type U struct {
		UserID          uint
		ScholarAuthorID string
	}

	// Build a base query
	db := config.DB.Table("users").Select("user_id, scholar_author_id").
		Where("scholar_author_id IS NOT NULL AND scholar_author_id <> ''")

	// Optional CSV subset
	if csv := c.Query("user_ids"); csv != "" {
		ids := strings.Split(csv, ",")
		db = db.Where("user_id IN ?", ids)
	}
	// Optional safety limit
	if limStr := c.Query("limit"); limStr != "" {
		if lim, err := strconv.Atoi(limStr); err == nil && lim > 0 {
			db = db.Limit(lim)
		}
	}

	var users []U
	if err := db.Find(&users).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	svc := services.NewPublicationService(nil)
	tot := struct {
		Users, Fetched, Created, Updated, Failed int
	}{}

	for _, u := range users {
		pubs, err := services.FetchScholarOnce(u.ScholarAuthorID)
		if err != nil {
			continue
		}
		tot.Users++
		tot.Fetched += len(pubs)

		for _, sp := range pubs {
			title := sp.Title
			authorsStr := strings.Join(sp.Authors, ", ")
			source := "scholar"
			var journal *string
			if sp.Venue != nil && *sp.Venue != "" {
				journal = sp.Venue
			}
			var yearPtr *uint16
			if sp.Year != nil && *sp.Year > 0 {
				yy := uint16(*sp.Year)
				yearPtr = &yy
			}
			var externalJSON *string
			if sp.ScholarClusterID != nil && *sp.ScholarClusterID != "" {
				js := fmt.Sprintf(`{"scholar_cluster_id":"%s"}`, *sp.ScholarClusterID)
				externalJSON = &js
			}

			pub := &models.UserPublication{
				UserID:          u.UserID,
				Title:           title,
				Authors:         &authorsStr,
				Journal:         journal,
				PublicationType: nil,
				PublicationDate: nil,
				PublicationYear: yearPtr,
				DOI:             sp.DOI,
				URL:             sp.URL,
				Source:          &source,
				ExternalIDs:     externalJSON,
			}

			created, _, e := svc.Upsert(pub)
			if e != nil {
				tot.Failed++
				continue
			}
			if created {
				tot.Created++
			} else {
				tot.Updated++
			}
		}
	}

	c.JSON(200, gin.H{"success": true, "summary": tot})
}

type AdminUserLite struct {
	UserID uint   `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

// GET /api/v1/admin/users/search?q=smith&limit=10
func AdminSearchUsers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing q"})
		return
	}
	limit := 10
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		// ignore parse errors silently
	}

	type row struct {
		UserID    uint
		UserFname *string
		UserLname *string
		Email     *string
	}

	var rows []row
	like := "%" + q + "%"
	if err := config.DB.
		Table("users").
		Select("user_id, user_fname, user_lname, email").
		Where("CONCAT(COALESCE(user_fname,''),' ',COALESCE(user_lname,'')) LIKE ? OR email LIKE ?", like, like).
		Limit(limit).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	out := make([]AdminUserLite, 0, len(rows))
	for _, r := range rows {
		fn, ln := "", ""
		if r.UserFname != nil {
			fn = *r.UserFname
		}
		if r.UserLname != nil {
			ln = *r.UserLname
		}
		name := strings.TrimSpace(fn + " " + ln)
		email := ""
		if r.Email != nil {
			email = *r.Email
		}
		out = append(out, AdminUserLite{UserID: r.UserID, Name: name, Email: email})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
