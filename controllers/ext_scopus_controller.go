package controllers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// ExtListScopusPublications is the external (partner) endpoint:
//
//	GET /api/ext/v1/scopus/publications?user_ids=1,2,3&year_from=2020&year_to=2024&limit=50&offset=0
//
// It returns a flat, paginated list of (faculty member, Scopus publication) rows for the
// requested users within the inclusive cover-year range. `user_ids` and `year_from` are
// required; `year_to` defaults to the current year.
func ExtListScopusPublications(c *gin.Context) {
	userIDs, err := parseUserIDsParam(c.Query("user_ids"))
	if err != nil {
		extValidationError(c, err.Error())
		return
	}

	yearFromRaw := strings.TrimSpace(c.Query("year_from"))
	if yearFromRaw == "" {
		extValidationError(c, "year_from is required")
		return
	}
	yearFrom, convErr := strconv.Atoi(yearFromRaw)
	if convErr != nil || yearFrom < 1900 || yearFrom > 3000 {
		extValidationError(c, "year_from must be a valid 4-digit year")
		return
	}

	yearTo := time.Now().Year()
	if yearToRaw := strings.TrimSpace(c.Query("year_to")); yearToRaw != "" {
		yt, convErr := strconv.Atoi(yearToRaw)
		if convErr != nil || yt < 1900 || yt > 3000 {
			extValidationError(c, "year_to must be a valid 4-digit year")
			return
		}
		yearTo = yt
	}
	if yearTo < yearFrom {
		extValidationError(c, "year_to must be greater than or equal to year_from")
		return
	}

	limit := parseIntOrDefault(c.Query("limit"), 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := parseIntOrDefault(c.Query("offset"), 0)
	if offset < 0 {
		offset = 0
	}

	svc := services.NewScopusPublicationService(nil)
	items, total, err := svc.ListForPartner(userIDs, yearFrom, yearTo, limit, offset)
	if err != nil {
		InternalError(c, "ext_scopus", err)
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
		"filters": gin.H{
			"user_ids":  userIDs,
			"year_from": yearFrom,
			"year_to":   yearTo,
		},
	})
}

// parseUserIDsParam parses a required comma-separated list of positive integer user ids.
func parseUserIDsParam(raw string) ([]uint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errExtBadRequest("user_ids is required (comma-separated list of user_id)")
	}
	parts := strings.Split(raw, ",")
	ids := make([]uint, 0, len(parts))
	seen := map[uint]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil || n == 0 {
			return nil, errExtBadRequest("user_ids must contain only positive integers")
		}
		id := uint(n)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errExtBadRequest("user_ids is required (comma-separated list of user_id)")
	}
	return ids, nil
}

type extBadRequest string

func (e extBadRequest) Error() string   { return string(e) }
func errExtBadRequest(msg string) error { return extBadRequest(msg) }

// extValidationError returns the standard 422 envelope for invalid input.
func extValidationError(c *gin.Context, msg string) {
	c.JSON(http.StatusUnprocessableEntity, gin.H{
		"success": false,
		"error":   msg,
		"code":    "INVALID_PARAMETER",
	})
}
