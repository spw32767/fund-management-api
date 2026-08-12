package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"fund-management-api/config"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RequireOpenSubmissionFund() gin.HandlerFunc {
	return func(c *gin.Context) {
		if roleID, ok := c.Get("roleID"); ok {
			if role, valid := roleID.(int); valid && role == 3 {
				c.Next()
				return
			}
		}

		submissionID, err := strconv.Atoi(c.Param("id"))
		if err != nil || submissionID <= 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid submission id"})
			return
		}

		err = services.EnsureSubmissionFundOpen(config.DB, submissionID)
		switch {
		case err == nil:
			c.Next()
		case errors.Is(err, services.ErrFundClosed), errors.Is(err, services.ErrFundNotFound):
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"error": "Fund is closed for applications",
				"code":  "fund_closed",
			})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		default:
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate fund availability"})
		}
	}
}
