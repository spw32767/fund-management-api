package controllers

import (
    "fmt"
    "log"
    "net/http"

    "github.com/gin-gonic/gin"
)

// InternalError logs the full error server-side (with a context label so it can be
// traced in the logs) and returns a response to the client that only reveals the raw
// error detail in debug mode. In release mode (GIN_MODE=release, i.e. production) the
// client gets a generic message so DB/SQL internals, table/column names and file paths
// are never leaked to the browser. Use this instead of:
//     c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
func InternalError(c *gin.Context, context string, err error) {
    // Always keep the full detail in the server log, tagged with the request route so a
    // log line can be traced back to the exact endpoint that failed.
    log.Printf("[InternalError] %s %s | %s: %v", c.Request.Method, c.Request.URL.Path, context, err)

    detail := "เกิดข้อผิดพลาดภายในระบบ กรุณาลองใหม่อีกครั้ง"
    if gin.Mode() != gin.ReleaseMode {
        detail = err.Error()
    }
    c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": detail})
}

func parseDeleteParams(c *gin.Context, idErrMsg string) (id uint, editorID int, ok bool) {
    var rawID uint
    if _, err := fmt.Sscanf(c.Param("id"), "%d", &rawID); err != nil || rawID == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": idErrMsg})
        return 0, 0, false
    }

    anyEditorID, exists := c.Get("userID")
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "กรุณาเข้าสู่ระบบ"})
        return 0, 0, false
    }

    eID, ok2 := anyEditorID.(int)
    if !ok2 {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "รูปแบบ User ID ไม่ถูกต้อง"})
        return 0, 0, false
    }

    return rawID, eID, true
}

func mustGetEditorID(c *gin.Context) (editorID int, ok bool) {
    anyEditorID, exists := c.Get("userID")
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "กรุณาเข้าสู่ระบบ"})
        return 0, false
    }

    eID, valid := anyEditorID.(int)
    if !valid {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "รูปแบบ User ID ไม่ถูกต้อง"})
        return 0, false
    }

    return eID, true
}