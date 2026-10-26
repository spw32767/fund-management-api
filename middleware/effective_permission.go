package middleware

import (
	"github.com/gin-gonic/gin"
	"strings"
)

// HasEffectivePermission only uses the request's resolved snapshot. In
// particular, a primary or additional role must never bypass an explicit deny.
func HasEffectivePermission(c *gin.Context, codes ...string) bool {
	value, exists := c.Get("permissions")
	if !exists {
		return false
	}
	granted, ok := value.([]string)
	if !ok {
		return false
	}
	for _, code := range codes {
		for _, permission := range granted {
			if strings.EqualFold(strings.TrimSpace(permission), strings.TrimSpace(code)) {
				return true
			}
		}
	}
	return false
}
