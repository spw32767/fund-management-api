package controllers

import (
	"net/http"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/admin/courses
func GetCourses(c *gin.Context) {
	svc := services.NewCourseService(config.DB)
	courses, err := svc.GetAllCourses(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ดึงข้อมูลหลักสูตรไม่ได้: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, courses)
}

// POST /api/v1/admin/courses
func CreateCourse(c *gin.Context) {
	editorID, ok := mustGetEditorID(c)
	if !ok {
		return
	}

	var input models.InstructorCourse
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลไม่ถูกต้อง: " + err.Error()})
		return
	}
	if input.CourseNameTh == "" || input.CourseNameEn == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณากรอกชื่อหลักสูตรทั้งภาษาไทยและภาษาอังกฤษ"})
		return
	}
	if input.DegreeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาระบุระดับการศึกษา (degree_id)"})
		return
	}

	svc := services.NewCourseService(config.DB)
	created, err := svc.CreateCourse(c.Request.Context(), editorID, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "สร้างหลักสูตรไม่สำเร็จ: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

// PUT /api/v1/admin/courses/:id
func UpdateCourse(c *gin.Context) {
	id, editorID, ok := parseDeleteParams(c, "ID หลักสูตรไม่ถูกต้อง")
	if !ok {
		return
	}

	var input models.InstructorCourse
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลไม่ถูกต้อง: " + err.Error()})
		return
	}
	if input.CourseNameTh == "" || input.CourseNameEn == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณากรอกชื่อหลักสูตรทั้งภาษาไทยและภาษาอังกฤษ"})
		return
	}
	if input.DegreeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาระบุระดับการศึกษา (degree_id)"})
		return
	}

	svc := services.NewCourseService(config.DB)
	updated, err := svc.UpdateCourse(c.Request.Context(), editorID, id, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "แก้ไขหลักสูตรไม่สำเร็จ: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DELETE /api/v1/admin/courses/:id
func DeleteCourse(c *gin.Context) {
	id, editorID, ok := parseDeleteParams(c, "ID หลักสูตรไม่ถูกต้อง")
	if !ok {
		return
	}

	svc := services.NewCourseService(config.DB)
	if err := svc.DeleteCourse(c.Request.Context(), editorID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ลบหลักสูตรไม่สำเร็จ: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ลบหลักสูตรสำเร็จ"})
}
