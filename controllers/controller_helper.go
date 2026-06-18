package controllers

import (
    "fmt"
    "net/http"

    "github.com/gin-gonic/gin"
)

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