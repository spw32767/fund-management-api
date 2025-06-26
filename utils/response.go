package utils

import (
	"github.com/gin-gonic/gin"
)

// SuccessResponse sends success response
func SuccessResponse(c *gin.Context, code int, message string, data interface{}) {
	c.JSON(code, gin.H{
		"success": true,
		"message": message,
		"data":    data,
	})
}

// ErrorResponse sends error response
func ErrorResponse(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{
		"success": false,
		"error":   message,
	})
}

// PaginatedResponse sends paginated response
func PaginatedResponse(c *gin.Context, code int, data interface{}, total int64, page int, pageSize int) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	c.JSON(code, gin.H{
		"success": true,
		"data":    data,
		"pagination": gin.H{
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"total_pages": totalPages,
		},
	})
}
