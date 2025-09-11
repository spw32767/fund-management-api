package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
