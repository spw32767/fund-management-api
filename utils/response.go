package utils

//"github.com/gin-gonic/gin"

// // SuccessResponse sends success response
// func SuccessResponse(c *gin.Context, code int, message string, data interface{}) {
// 	c.JSON(code, gin.H{
// 		"success": true,
// 		"message": message,
// 		"data":    data,
// 	})
// }

// // ErrorResponse sends error response
// func ErrorResponse(c *gin.Context, code int, message string) {
// 	c.JSON(code, gin.H{
// 		"success": false,
// 		"error":   message,
// 	})
// }

// // PaginatedResponse sends paginated response
// func PaginatedResponse(c *gin.Context, code int, data interface{}, total int64, page int, pageSize int) {
// 	totalPages := int(total) / pageSize
// 	if int(total)%pageSize > 0 {
// 		totalPages++
// 	}

// 	c.JSON(code, gin.H{
// 		"success": true,
// 		"data":    data,
// 		"pagination": gin.H{
// 			"total":       total,
// 			"page":        page,
// 			"page_size":   pageSize,
// 			"total_pages": totalPages,
// 		},
// 	})
// }

// utils/response_utils.go
// เพิ่มฟังก์ชันสำหรับจัดการ response

// ErrorResponse represents an error response
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
}

// SuccessResponse represents a success response
type SuccessResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Count   *int        `json:"count,omitempty"`
}

// PaginatedResponse represents a paginated response
type PaginatedResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Count   int         `json:"count"`
	Page    int         `json:"page"`
	Limit   int         `json:"limit"`
	Total   int64       `json:"total"`
}

// NewErrorResponse creates a new error response
func NewErrorResponse(error string, code ...string) ErrorResponse {
	resp := ErrorResponse{
		Success: false,
		Error:   error,
	}
	if len(code) > 0 {
		resp.Code = code[0]
	}
	return resp
}

// NewSuccessResponse creates a new success response
func NewSuccessResponse(message string, data interface{}, count ...int) SuccessResponse {
	resp := SuccessResponse{
		Success: true,
		Message: message,
		Data:    data,
	}
	if len(count) > 0 {
		resp.Count = &count[0]
	}
	return resp
}

// NewPaginatedResponse creates a new paginated response
func NewPaginatedResponse(data interface{}, count int, page int, limit int, total int64) PaginatedResponse {
	return PaginatedResponse{
		Success: true,
		Data:    data,
		Count:   count,
		Page:    page,
		Limit:   limit,
		Total:   total,
	}
}
