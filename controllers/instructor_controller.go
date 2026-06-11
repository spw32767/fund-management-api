package controllers

import (
    "fmt"
    "net/http"
    "time"
    "fund-management-api/config"
    "fund-management-api/models"
    "fund-management-api/services"
    "strconv"
    "github.com/gin-gonic/gin"
)

func GetMyProfile(c *gin.Context) {
    anyID, exists := c.Get("userID")
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "กรุณาเข้าสู่ระบบ"})
        return
    }
    userID, ok := anyID.(int)
    if !ok {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "รูปแบบ User ID ไม่ถูกต้อง"})
        return
    }
    svc := services.NewInstructorService(config.DB)
    profile, err := svc.GetFullProfile(c.Request.Context(), userID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ดึงข้อมูลไม่ได้: " + err.Error()})
        return
    }
    c.JSON(http.StatusOK, profile)
}

func UpdateMyProfile(c *gin.Context) {
    anyID, _ := c.Get("userID")
    userID := anyID.(int)
    var input models.InstructorFullProfile
    if err := c.ShouldBindJSON(&input); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลไม่ถูกต้อง: " + err.Error()})
        return
    }
    input.Header.UserID = userID
    svc := services.NewInstructorService(config.DB)
    if err := svc.UpdateInstructorProfile(c.Request.Context(), input); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึกไม่สำเร็จ: " + err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "บันทึกข้อมูลเรียบร้อยแล้ว"})
}

func GetInstructors(c *gin.Context) {
    svc := services.NewInstructorService(config.DB)
    list, err := svc.GetInstructorList(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ดึงข้อมูลไม่ได้"})
        return
    }
    c.JSON(http.StatusOK, list)
}

func GetInstructorByID(c *gin.Context) {
    idStr := c.Param("id")
    var targetID int
    if _, err := fmt.Sscanf(idStr, "%d", &targetID); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID ไม่ถูกต้อง"})
        return
    }
    svc := services.NewInstructorService(config.DB)
    profile, err := svc.GetFullProfile(c.Request.Context(), targetID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบข้อมูลอาจารย์"})
        return
    }
    c.JSON(http.StatusOK, profile)
}

func UpdateInstructorByID(c *gin.Context) {
    idStr := c.Param("id")
    var targetID int
    fmt.Sscanf(idStr, "%d", &targetID)

    // ─── DiffItem helpers ────────────────────────────────────────────────────
    type DeleteItem struct {
        ID        uint      `json:"id"`
        UpdatedAt time.Time `json:"updated_at"`
    }

    type ExpertiseUpdateItem struct {
        ID        uint      `json:"id"`
        UpdatedAt time.Time `json:"updated_at"`
        UserID    int       `json:"user_id"`
        Expertise string    `json:"expertise"`
    }

    type EducationUpdateItem struct {
        models.InstructorEducationTab
        UpdatedAt time.Time `json:"updated_at"`
    }

    type ProjectUpdateItem struct {
        models.InstructorResearchProject
        UpdatedAt time.Time `json:"updated_at"`
    }

    type TextbookUpdateItem struct {
        models.InstructorTextbook
        UpdatedAt time.Time `json:"updated_at"`
    }

    type PropertyUpdateItem struct {
        models.InstructorIntellectualProperty
        UpdatedAt time.Time `json:"updated_at"`
    }

    var input struct {
        Header map[string]interface{} `json:"header"`

        EducationsDiff struct {
            Added   []models.InstructorEducationTab `json:"added"`
            Updated []EducationUpdateItem           `json:"updated"`
            Deleted []DeleteItem                    `json:"deleted"`
        } `json:"educations_diff"`

        ExpertisesDiff struct {
            Added   []models.InstructorExpertiseTab `json:"added"`
            Updated []ExpertiseUpdateItem           `json:"updated"`
            Deleted []DeleteItem                    `json:"deleted"`
        } `json:"expertises_diff"`

        ProjectsDiff struct {
            Added   []models.InstructorResearchProject `json:"added"`
            Updated []ProjectUpdateItem                `json:"updated"`
            Deleted []DeleteItem                       `json:"deleted"`
        } `json:"projects_diff"`

        TextbooksDiff struct {
            Added   []models.InstructorTextbook `json:"added"`
            Updated []TextbookUpdateItem        `json:"updated"`
            Deleted []DeleteItem                `json:"deleted"`
        } `json:"textbooks_diff"`

        PropertiesDiff struct {
            Added   []models.InstructorIntellectualProperty `json:"added"`
            Updated []PropertyUpdateItem                    `json:"updated"`
            Deleted []DeleteItem                            `json:"deleted"`
        } `json:"properties_diff"`

        InstructorCourseResponsibility []models.InstructorCourseResponsibility `json:"instructor_course_responsibility"`
    }

    if err := c.ShouldBindJSON(&input); err != nil {
        fmt.Println("❌ Bind error:", err.Error())
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    anyEditorID, exists := c.Get("userID")
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }
    editorID, ok := anyEditorID.(int)
    if !ok {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID"})
        return
    }

    if input.Header == nil {
        input.Header = map[string]interface{}{}
    }
    input.Header["user_id"] = targetID

    // แปลง UpdateItem → services.DiffItem
    eduUpdated := make([]services.EducationDiffItem, len(input.EducationsDiff.Updated))
    for i, u := range input.EducationsDiff.Updated {
        eduUpdated[i] = services.EducationDiffItem{InstructorEducationTab: u.InstructorEducationTab, UpdatedAt: u.UpdatedAt}
    }
    eduDeleted := make([]services.DeleteDiffItem, len(input.EducationsDiff.Deleted))
    for i, d := range input.EducationsDiff.Deleted {
        eduDeleted[i] = services.DeleteDiffItem{ID: d.ID, UpdatedAt: d.UpdatedAt}
    }

    expUpdated := make([]services.ExpertiseDiffItem, len(input.ExpertisesDiff.Updated))
    for i, u := range input.ExpertisesDiff.Updated {
        expUpdated[i] = services.ExpertiseDiffItem{ID: u.ID, UpdatedAt: u.UpdatedAt, UserID: u.UserID, Expertise: u.Expertise}
    }
    expDeleted := make([]services.DeleteDiffItem, len(input.ExpertisesDiff.Deleted))
    for i, d := range input.ExpertisesDiff.Deleted {
        expDeleted[i] = services.DeleteDiffItem{ID: d.ID, UpdatedAt: d.UpdatedAt}
    }

    projUpdated := make([]services.ProjectDiffItem, len(input.ProjectsDiff.Updated))
    for i, u := range input.ProjectsDiff.Updated {
        projUpdated[i] = services.ProjectDiffItem{InstructorResearchProject: u.InstructorResearchProject, UpdatedAt: u.UpdatedAt}
    }
    projDeleted := make([]services.DeleteDiffItem, len(input.ProjectsDiff.Deleted))
    for i, d := range input.ProjectsDiff.Deleted {
        projDeleted[i] = services.DeleteDiffItem{ID: d.ID, UpdatedAt: d.UpdatedAt}
    }

    tbUpdated := make([]services.TextbookDiffItem, len(input.TextbooksDiff.Updated))
    for i, u := range input.TextbooksDiff.Updated {
        tbUpdated[i] = services.TextbookDiffItem{InstructorTextbook: u.InstructorTextbook, UpdatedAt: u.UpdatedAt}
    }
    tbDeleted := make([]services.DeleteDiffItem, len(input.TextbooksDiff.Deleted))
    for i, d := range input.TextbooksDiff.Deleted {
        tbDeleted[i] = services.DeleteDiffItem{ID: d.ID, UpdatedAt: d.UpdatedAt}
    }

    propUpdated := make([]services.PropertyDiffItem, len(input.PropertiesDiff.Updated))
    for i, u := range input.PropertiesDiff.Updated {
        propUpdated[i] = services.PropertyDiffItem{InstructorIntellectualProperty: u.InstructorIntellectualProperty, UpdatedAt: u.UpdatedAt}
    }
    propDeleted := make([]services.DeleteDiffItem, len(input.PropertiesDiff.Deleted))
    for i, d := range input.PropertiesDiff.Deleted {
        propDeleted[i] = services.DeleteDiffItem{ID: d.ID, UpdatedAt: d.UpdatedAt}
    }

    svc := services.NewInstructorService(config.DB)
    err := svc.UpdateInstructorByAdmin(
        c.Request.Context(),
        editorID, targetID,
        input.Header,
        input.EducationsDiff.Added, eduUpdated, eduDeleted,
        input.ExpertisesDiff.Added, expUpdated, expDeleted,
        input.ProjectsDiff.Added,   projUpdated, projDeleted,
        input.TextbooksDiff.Added,  tbUpdated,  tbDeleted,
        input.PropertiesDiff.Added, propUpdated, propDeleted,
        input.InstructorCourseResponsibility,
    )
    if err != nil {
        c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "Admin แก้ไขข้อมูลสำเร็จ"})
}

// ─── Delete handlers (ไม่เปลี่ยน) ──────────────────────────────────────────

func DeleteInstructorTextbook(c *gin.Context) {
    var id uint
    if _, err := fmt.Sscanf(c.Param("id"), "%d", &id); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID ตำราไม่ถูกต้อง"})
        return
    }
    if err := config.DB.WithContext(c.Request.Context()).Delete(&models.InstructorTextbook{}, id).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลตำราได้"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "ลบข้อมูลตำราสำเร็จ"})
}

func DeleteInstructorIntellectualProperty(c *gin.Context) {
    var id uint
    if _, err := fmt.Sscanf(c.Param("id"), "%d", &id); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID ทรัพย์สินทางปัญญาไม่ถูกต้อง"})
        return
    }
    if err := config.DB.WithContext(c.Request.Context()).Delete(&models.InstructorIntellectualProperty{}, id).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลทรัพย์สินทางปัญญาได้"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "ลบข้อมูลทรัพย์สินทางปัญญาสำเร็จ"})
}

func DeleteInstructorResearchProject(c *gin.Context) {
    var id uint
    if _, err := fmt.Sscanf(c.Param("id"), "%d", &id); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID โครงการวิจัยไม่ถูกต้อง"})
        return
    }
    if err := config.DB.WithContext(c.Request.Context()).Delete(&models.InstructorResearchProject{}, id).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลโครงการวิจัยได้"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "ลบข้อมูลโครงการวิจัยสำเร็จ"})
}

func DeleteInstructorExpertise(c *gin.Context) {
    var id uint
    if _, err := fmt.Sscanf(c.Param("id"), "%d", &id); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID ความเชี่ยวชาญไม่ถูกต้อง"})
        return
    }
    if err := config.DB.WithContext(c.Request.Context()).Delete(&models.InstructorExpertiseTab{}, id).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลความเชี่ยวชาญได้"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "ลบข้อมูลความเชี่ยวชาญสำเร็จ"})
}

func DeleteInstructorEducation(c *gin.Context) {
    var id uint
    if _, err := fmt.Sscanf(c.Param("id"), "%d", &id); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID ประวัติการศึกษาไม่ถูกต้อง"})
        return
    }
    if err := config.DB.WithContext(c.Request.Context()).Delete(&models.InstructorEducationTab{}, id).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลประวัติการศึกษาได้"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "ลบข้อมูลประวัติการศึกษาสำเร็จ"})
}

func GetFullProfile(c *gin.Context) {
    userID, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบ ID ไม่ถูกต้อง"})
        return
    }
    svc := services.NewInstructorService(config.DB)
    profile, err := svc.GetFullProfile(c.Request.Context(), userID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ดึงข้อมูลโปรไฟล์ไม่ได้: " + err.Error()})
        return
    }
    c.JSON(http.StatusOK, profile)
}
