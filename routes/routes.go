package routes

import (
	"fmt"
	"fund-management-api/controllers"
	"fund-management-api/middleware"
	"fund-management-api/monitor"
	"fund-management-api/utils"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine) {
	// Add security headers middleware
	router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	})

	monitor.RegisterDeployPage(router)

	// API v1 group
	v1 := router.Group("/api/v1")
	{
		// Public routes
		public := v1.Group("")
		{

			RegisterUploadRoutes(public) // สำหรับ POST /upload
			RegisterFileRoutes(public)   // สำหรับ GET /files, DELETE /files/:name

			public.GET("/years", controllers.GetActiveYears)

			// Authentication
			public.POST("/login", controllers.Login)
			public.POST("/password/forgot", controllers.ForgotPassword)
			public.POST("/password/reset", controllers.ResetPassword)

			// NEW: Refresh token endpoint (public)
			public.POST("/refresh", controllers.RefreshTokenWithRefreshToken)

			// Health check
			public.GET("/health", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"status":  "ok",
					"message": "Fund Management API is running",
					"timestamp": gin.H{
						"server": "2025-07-02T10:00:00Z",
					},
					"version": "1.0.0",
					"success": true,
				})
			})

			// API Info
			public.GET("/info", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"name":        "Fund Management API",
					"version":     "1.0.0",
					"description": "API for managing research fund applications",
					"endpoints": gin.H{
						"auth": gin.H{
							"login":           "POST /api/v1/login",
							"refresh":         "POST /api/v1/refresh",
							"profile":         "GET /api/v1/profile",
							"change_password": "PUT /api/v1/change-password",
							"logout":          "POST /api/v1/logout",
							"sessions":        "GET /api/v1/sessions",
						},
						"applications": gin.H{
							"list":   "GET /api/v1/applications",
							"create": "POST /api/v1/applications",
							"detail": "GET /api/v1/applications/:id",
						},
						"dashboard": gin.H{
							"stats": "GET /api/v1/dashboard/stats",
						},
						"role_based": gin.H{
							"teacher_subcategories": "GET /api/v1/teacher/subcategories",
							"staff_subcategories":   "GET /api/v1/staff/subcategories",
							"admin_manage_roles":    "PUT /api/v1/admin/subcategories/:id/roles",
						},
					},
				})
			})
		}

		// Protected routes (require authentication)
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// Authentication routes
			protected.GET("/profile", controllers.GetProfile)
			protected.PUT("/change-password", controllers.ChangePassword)
			protected.POST("/refresh-token", controllers.RefreshToken) // Legacy endpoint

			// NEW: Session management endpoints
			protected.POST("/logout", controllers.Logout)
			protected.GET("/sessions", controllers.GetActiveSessions)
			protected.POST("/sessions/revoke-others", controllers.RevokeOtherSessions)

			// ===== NOTIFICATIONS (NEW) =====
			notifications := protected.Group("/notifications")
			{
				notifications.POST("", controllers.CreateNotification)                                                   // สร้างแจ้งเตือน 1 รายการ
				notifications.GET("", controllers.GetNotifications)                                                      // list ของ user ปัจจุบัน
				notifications.GET("/counter", controllers.GetNotificationCounter)                                        // นับ unread
				notifications.PATCH("/:id/read", controllers.MarkNotificationRead)                                       // อ่าน 1 รายการ
				notifications.POST("/mark-all-read", controllers.MarkAllNotificationsRead)                               // อ่านทั้งหมด
				notifications.POST("/events/submissions/:submissionId/submitted", controllers.NotifySubmissionSubmitted) // อีเวนต์: ส่งคำร้องสำเร็จ
				// Dept Head stage (เห็นควร/ไม่เห็นควร)
				notifications.POST("/events/submissions/:submissionId/dept-head/recommended", controllers.NotifyDeptHeadRecommended)
				notifications.POST("/events/submissions/:submissionId/dept-head/not-recommended", controllers.NotifyDeptHeadNotRecommended)

				// Admin stage (อนุมัติ/ไม่อนุมัติ)
				notifications.POST("/events/submissions/:submissionId/approved", controllers.NotifyAdminApproved)
				notifications.POST("/events/submissions/:submissionId/rejected", controllers.NotifyAdminRejected)
			}

			// Common endpoints (all authenticated users)
			protected.GET("/categories", controllers.GetCategories)
			protected.GET("/subcategories", controllers.GetSubcategories)
			protected.GET("/application-status", controllers.GetApplicationStatuses)
			protected.GET("/system-config/current-year", controllers.GetSystemConfigCurrentYear)
			protected.GET("/system-config/window", controllers.GetSystemConfigWindow)
			protected.GET("/system-config/dept-head/eligible-roles", controllers.GetDeptHeadEligibleRoles)

			protected.GET("/system-config/dept-head/current", controllers.GetCurrentDeptHead)

			protected.GET("/projects", controllers.GetProjectsForMembers)
			protected.GET("/projects/:projectId/attachments/:fileId", controllers.DownloadProjectAttachment)

			// Publication reward agreements / conditions
			protected.GET("/end-of-contract", controllers.GetEndOfContractTerms)

			// Fund installment periods
			protected.GET("/fund-installment-periods", controllers.GetFundInstallmentPeriods)

			// General submissions listing (all users)
			protected.GET("/submissions", controllers.GetAllSubmissions)        // ดูรายการ submissions (filtered by role)
			protected.GET("/submissions/search", controllers.SearchSubmissions) // ค้นหา submissions

			// Teacher-specific endpoints
			teacher := protected.Group("/teacher")
			{
				// ไม่ต้องใส่ RequireRole(1) เพราะ GetSubcategoryForRole จะ check role เอง
				teacher.GET("/subcategories", controllers.GetSubcategoryForRole)
				teacher.GET("/submissions", controllers.GetTeacherSubmissions) // Teacher ดู submissions ของตัวเอง
				// User Publications
				teacher.GET("/user-publications", controllers.GetUserPublications)
				teacher.GET("/user-publications/scopus", controllers.GetUserScopusPublications)
				teacher.GET("/user-publications/scopus/stats", controllers.GetUserScopusPublicationStats)
				teacher.POST("/user-publications/upsert", controllers.UpsertUserPublication)
				teacher.DELETE("/user-publications/:id", controllers.DeleteUserPublication)
				teacher.PATCH("/user-publications/:id/restore", controllers.RestoreUserPublication)
				teacher.GET("/user-publications/scholar/search", controllers.TeacherScholarAuthorSearch)

				// User Innovations
				teacher.GET("/user-innovations", controllers.GetUserInnovations)
			}

			// Staff-specific endpoints
			staff := protected.Group("/staff")
			{
				// ใช้ function เดียวกัน
				staff.GET("/subcategories", controllers.GetSubcategoryForRole)
				staff.GET("/submissions", controllers.GetStaffSubmissions) // Staff ดู submissions ของตัวเอง
				staff.GET("/dashboard/stats", controllers.GetDashboardStats)
			}

			// Dept head review endpoints
			deptHead := protected.Group("/dept-head")
			deptHead.Use(middleware.RequireRole(4))
			{
				deptHead.GET("/submissions", controllers.GetDeptHeadSubmissions)
				deptHead.GET("/submissions/:id/details", controllers.GetDeptHeadSubmissionDetails)
				deptHead.POST("/submissions/:id/recommend", controllers.DeptHeadRecommendSubmission)
				deptHead.POST("/submissions/:id/reject", controllers.DeptHeadRejectSubmission)
				deptHead.POST("/submissions/:id/request-revision", controllers.DeptHeadRequestRevision)
			}

			// Fund Applications
			applications := protected.Group("/applications")
			{
				// Teacher & Admin can view their applications
				applications.GET("", controllers.GetApplications)
				applications.GET("/:id", controllers.GetApplication)

				// Only teachers can create/update/delete applications
				applications.POST("", middleware.RequireRole(1), controllers.CreateApplication) // 1 = teacher
				applications.PUT("/:id", middleware.RequireRole(1), controllers.UpdateApplication)
				applications.DELETE("/:id", middleware.RequireRole(1), controllers.DeleteApplication)

				// Only admin can approve/reject
				applications.POST("/:id/approve", middleware.RequireRole(3), controllers.ApproveApplication) // 3 = admin
				applications.POST("/:id/reject", middleware.RequireRole(3), controllers.RejectApplication)
			}

			submissions := protected.Group("/submissions")
			{
				// Basic CRUD
				submissions.POST("", controllers.CreateSubmission)
				submissions.GET("/:id", controllers.GetSubmission)
				submissions.PUT("/:id", controllers.UpdateSubmission)
				submissions.DELETE("/:id", controllers.DeleteSubmission)
				submissions.DELETE("/:id/hard", controllers.HardDeleteSubmission)

				// Submit submission
				submissions.POST("/:id/submit", controllers.SubmitSubmission)
				submissions.POST("/:id/merge-documents", controllers.MergeSubmissionDocuments)

				// Add specific details
				submissions.POST("/:id/publication-details", controllers.AddPublicationDetails)
				submissions.POST("/:id/fund-details", controllers.AddFundDetails)

				// Documents management
				submissions.POST("/:id/documents", controllers.AttachDocument)
				submissions.GET("/:id/documents", controllers.GetSubmissionDocuments)
				submissions.DELETE("/:id/documents/:doc_id", controllers.DetachDocument)

				// === Co-authors Management (ใหม่) ===
				// submissions.POST("/:id/coauthors", controllers.AddCoauthor)               // เพิ่ม co-author
				// submissions.GET("/:id/coauthors", controllers.GetCoauthors)               // ดู co-authors
				// submissions.PUT("/:id/coauthors/:user_id", controllers.UpdateCoauthor)    // แก้ไข co-author
				// submissions.DELETE("/:id/coauthors/:user_id", controllers.RemoveCoauthor) // ลบ co-author

				// === NEW: Submission Users Management (ให้ตรงกับ Frontend) ===
				submissions.POST("/:id/users", controllers.AddSubmissionUser)               // เพิ่ม user ลงใน submission
				submissions.GET("/:id/users", controllers.GetSubmissionUsers)               // ดู users ใน submission
				submissions.PUT("/:id/users/:user_id", controllers.UpdateSubmissionUser)    // แก้ไข user ใน submission
				submissions.DELETE("/:id/users/:user_id", controllers.RemoveSubmissionUser) // ลบ user จาก submission

				// === NEW: Batch Operations for Frontend ===
				submissions.POST("/:id/users/batch", controllers.AddMultipleUsers)     // เพิ่ม users หลายคนพร้อมกัน
				submissions.POST("/:id/users/set-coauthors", controllers.SetCoauthors) // ตั้งค่า co-authors ทั้งหมด (replace existing)

				// Enhanced submission details with co-authors
				//submissions.GET("/:id/full", controllers.GetSubmissionWithCoauthors) // ดู submission พร้อม co-authors

				// เพิ่ม route ใหม่สำหรับแนบไฟล์
				submissions.POST("/:id/attach-document", controllers.AttachDocumentToSubmission) // แนบไฟล์กับ submission
				submissions.DELETE("/:id/detach-document/:doc_id", controllers.DetachDocument)
			}

			// Files management
			files := protected.Group("/files")
			{
				files.POST("/upload", controllers.UploadFile)
				files.GET("/managed/:id", controllers.GetFile)               // เปลี่ยนเป็น /managed/:id
				files.GET("/managed/:id/download", controllers.DownloadFile) // เปลี่ยนเป็น /managed/:id/download
				files.DELETE("/managed/:id", controllers.DeleteFile)         // เปลี่ยนเป็น /managed/:id
			}

			// Documents
			documents := protected.Group("/documents")
			{
				documents.POST("/upload/:id", controllers.UploadDocument)
				documents.GET("/application/:id", controllers.GetDocuments)
				documents.GET("/download/:document_id", controllers.DownloadDocument)
				documents.DELETE("/:document_id", controllers.DeleteDocument)
				documents.GET("/types", controllers.GetDocumentTypes) // Legacy endpoint
			}

			// Dashboard
			dashboard := protected.Group("/dashboard")
			{
				dashboard.GET("/stats", controllers.GetDashboardStats)
				dashboard.GET("/budget-summary", controllers.GetBudgetSummary)
				dashboard.GET("/applications-summary", controllers.GetApplicationsSummary)
			}

			// Publication Rewards
			publications := protected.Group("/publication-rewards")
			{
				// List and view (teachers can see their own, admin can see all)
				publications.GET("", controllers.GetPublicationRewards)
				publications.GET("/:id", controllers.GetPublicationReward)
				publications.POST("/preview", controllers.PreviewPublicationReward)

				// Only teachers can create/update/delete
				publications.POST("", middleware.RequireRole(1), controllers.CreatePublicationReward)
				publications.PUT("/:id", middleware.RequireRole(1), controllers.UpdatePublicationReward)
				publications.DELETE("/:id", middleware.RequireRole(1), controllers.DeletePublicationReward)

				// Only admin can approve/reject
				publications.POST("/:id/approve", middleware.RequireRole(3), controllers.ApprovePublicationReward)
				publications.POST("/:id/reject", middleware.RequireRole(3), controllers.RejectPublicationReward)

				// Documents
				publications.POST("/:id/documents", controllers.UploadPublicationDocument)
				publications.GET("/:id/documents", controllers.GetPublicationDocuments)

				// Dynamic lookup endpoints
				publications.GET("/enabled-years", controllers.GetEnabledYearsForCategory)
				publications.GET("/options", controllers.GetPublicationOptions)
				publications.GET("/resolve", controllers.ResolvePublicationBudget)
				publications.GET("/availability/:id", controllers.CheckBudgetAvailability)

				// === REWARD RATES API ===
				rates := publications.Group("/rates")
				{
					// Public endpoints (สำหรับ calculation)
					rates.GET("", controllers.GetPublicationRewardRates)             // GET /api/v1/publication-rewards/rates
					rates.GET("/all", controllers.GetAllPublicationRewardRates)      // GET /api/v1/publication-rewards/rates/all
					rates.GET("/lookup", controllers.GetPublicationRewardRateLookup) // GET /api/v1/publication-rewards/rates/lookup
					rates.GET("/years", controllers.GetAvailableYears)               // GET /api/v1/publication-rewards/rates/years

					// Admin only endpoints
					rates.GET("/admin", middleware.RequireRole(3), controllers.GetPublicationRewardRatesAdmin)           // GET /api/v1/publication-rewards/rates/admin (ดูทั้งหมด ไม่ filter is_active)
					rates.POST("", middleware.RequireRole(3), controllers.CreatePublicationRewardRate)                   // POST /api/v1/publication-rewards/rates
					rates.PUT("/bulk", middleware.RequireRole(3), controllers.UpdatePublicationRewardRates)              // PUT /api/v1/publication-rewards/rates/bulk (existing)
					rates.PUT("/:id", middleware.RequireRole(3), controllers.UpdatePublicationRewardRate)                // PUT /api/v1/publication-rewards/rates/:id
					rates.DELETE("/:id", middleware.RequireRole(3), controllers.DeletePublicationRewardRate)             // DELETE /api/v1/publication-rewards/rates/:id
					rates.PATCH("/:id/toggle", middleware.RequireRole(3), controllers.TogglePublicationRewardRateStatus) // PATCH /api/v1/publication-rewards/rates/:id/toggle
					rates.POST("/:id/toggle", middleware.RequireRole(3), controllers.TogglePublicationRewardRateStatus)  // PATCH /api/v1/publication-rewards/rates/:id/toggle
				}
			}

			// Legacy/compatibility route for publication summary preview
			publicationSummary := protected.Group("/publication-summary")
			{
				// POST /api/v1/publication-summary/preview (alias of publication reward preview)
				publicationSummary.POST("/preview", controllers.PreviewPublicationReward)
			}

			rewardConfig := v1.Group("/reward-config")
			rewardConfig.Use(middleware.AuthMiddleware())
			{
				// Public endpoints (สำหรับ teacher และ staff)
				rewardConfig.GET("", controllers.GetRewardConfig)              // GET /api/v1/reward-config
				rewardConfig.GET("/lookup", controllers.GetRewardConfigLookup) // GET /api/v1/reward-config/lookup
			}

			// Users endpoint for form dropdown
			protected.GET("/users", controllers.GetUsers)

			// Document types with category filter
			protected.GET("/document-types", controllers.GetDocumentTypes)

			// ===== ANNOUNCEMENTS AND FUND FORMS =====
			announcements := protected.Group("/announcements")
			{
				// Public routes (all authenticated users can access)
				announcements.GET("", controllers.GetAnnouncements)
				announcements.GET("/:id", controllers.GetAnnouncement)
				announcements.GET("/:id/view", controllers.ViewAnnouncementFile)
				announcements.GET("/:id/download", controllers.DownloadAnnouncementFile)

				// Admin only routes
				announcements.POST("", middleware.RequireRole(3), controllers.CreateAnnouncement)
				announcements.PUT("/:id", middleware.RequireRole(3), controllers.UpdateAnnouncement)
				announcements.DELETE("/:id", middleware.RequireRole(3), controllers.DeleteAnnouncement)
			}

			fundForms := protected.Group("/fund-forms")
			{
				// Public routes (all authenticated users can access)
				fundForms.GET("", controllers.GetFundForms)
				fundForms.GET("/:id", controllers.GetFundForm)
				fundForms.GET("/:id/view", controllers.ViewFundForm)
				fundForms.GET("/:id/download", controllers.DownloadFundForm)

				// Admin only routes
				fundForms.POST("", middleware.RequireRole(3), controllers.CreateFundForm)
				fundForms.PUT("/:id", middleware.RequireRole(3), controllers.UpdateFundForm)
				fundForms.DELETE("/:id", middleware.RequireRole(3), controllers.DeleteFundForm)
			}

			// เพิ่มส่วนนี้ใน admin group หลังจาก middleware.RequireRole(3)
			admin := protected.Group("/admin")
			admin.Use(middleware.RequireRole(3)) // Require admin role
			{
				notificationMessages := admin.Group("/notification-messages")
				{
					notificationMessages.GET("", controllers.ListNotificationMessages)
					notificationMessages.POST("", controllers.CreateNotificationMessage)
					notificationMessages.PUT("/:id", controllers.UpdateNotificationMessage)
					notificationMessages.POST("/:id/reset", controllers.ResetNotificationMessage)
				}

				// Dashboard
				admin.GET("/dashboard/stats", controllers.GetDashboardStats)
				admin.GET("/submissions", controllers.GetAdminSubmissions) // Admin ดู submissions ทั้งหมด

				// User Publications Import from Scholar
				admin.POST("/user-publications/import/scholar", controllers.AdminImportScholarPublications)
				admin.POST("/user-publications/import/scholar/all", controllers.AdminImportScholarForAll)
				admin.GET("/user-publications/import/scholar/runs", controllers.AdminListScholarImportRuns)
				admin.POST("/user-publications/import/scopus", controllers.AdminImportScopusPublications)
				admin.POST("/user-publications/import/scopus/all", controllers.AdminImportScopusForAll)
				admin.GET("/publications/scopus", controllers.AdminListScopusPublications)
				admin.POST("/scopus/metrics/backfill", controllers.AdminBackfillCiteScoreMetrics)
				admin.POST("/scopus/metrics/refresh", controllers.AdminRefreshCiteScoreMetrics)
				admin.GET("/scopus/config", controllers.AdminGetScopusAPIKey)
				admin.PUT("/scopus/config", controllers.AdminUpdateScopusAPIKey)
				admin.GET("/user-publications/scholar/search", controllers.TeacherScholarAuthorSearch)
				admin.GET("/users/search", controllers.AdminSearchUsers)
				admin.GET("/users/scopus", controllers.AdminListUsersWithScopusID)
				admin.POST("/users/:id/scholar-author", controllers.AdminSetUserScholarAuthorID)
				admin.POST("/users/:id/scopus-author", controllers.AdminSetUserScopusAuthorID)

				admin.GET("/approval-records/totals", controllers.GetApprovalTotals)
				admin.GET("/approval-records", controllers.GetApprovalRecords)

				// ===== CP PROFILE IMPORT =====
				admin.POST("/trigger/cp-profile", controllers.AdminTriggerCpProfile)

				// ===== KKU PEOPLE SCRAPER =====
				admin.POST("/kku-people/scrape", controllers.AdminRunKkuPeopleScrape)
				admin.GET("/kku-people/status", controllers.AdminGetKkuPeopleStatus)
				admin.GET("/kku-people/logs", controllers.AdminListKkuPeopleLogs)

				// ========== YEAR MANAGEMENT ==========
				years := admin.Group("/years")
				{
					years.GET("", controllers.GetAllYears)                   // GET /api/v1/admin/years
					years.POST("", controllers.CreateYear)                   // POST /api/v1/admin/years
					years.PUT("/:id", controllers.UpdateYear)                // PUT /api/v1/admin/years/:id
					years.DELETE("/:id", controllers.DeleteYear)             // DELETE /api/v1/admin/years/:id
					years.PATCH("/:id/toggle", controllers.ToggleYearStatus) // PATCH /api/v1/admin/years/:id/toggle
					years.GET("/:id/stats", controllers.GetYearStats)        // GET /api/v1/admin/years/:id/stats
				}

				installments := admin.Group("/installments")
				{
					installments.POST("/copy", controllers.AdminCopyFundInstallmentPeriods)
					installments.GET("", controllers.AdminListFundInstallmentPeriods)
					installments.POST("", controllers.AdminCreateFundInstallmentPeriod)
					installments.PUT("/:id", controllers.AdminUpdateFundInstallmentPeriod)
					installments.PATCH("/:id", controllers.AdminUpdateFundInstallmentPeriod)
					installments.DELETE("/:id", controllers.AdminDeleteFundInstallmentPeriod)
					installments.PATCH("/:id/restore", controllers.AdminRestoreFundInstallmentPeriod)
				}

				// ========== FUND CATEGORIES MANAGEMENT ==========
				categories := admin.Group("/categories")
				{
					categories.GET("", controllers.GetAllCategories)                  // GET /api/v1/admin/categories
					categories.POST("", controllers.CreateCategory)                   // POST /api/v1/admin/categories
					categories.PUT("/:id", controllers.UpdateCategory)                // PUT /api/v1/admin/categories/:id
					categories.DELETE("/:id", controllers.DeleteCategory)             // DELETE /api/v1/admin/categories/:id
					categories.PATCH("/:id/toggle", controllers.ToggleCategoryStatus) // PATCH /api/v1/admin/categories/:id/toggle
				}

				// ========== PROJECT MANAGEMENT ==========
				projects := admin.Group("/projects")
				{
					projects.GET("", controllers.GetProjects)          // GET /api/v1/admin/projects
					projects.POST("", controllers.CreateProject)       // POST /api/v1/admin/projects
					projects.PUT("/:id", controllers.UpdateProject)    // PUT /api/v1/admin/projects/:id
					projects.DELETE("/:id", controllers.DeleteProject) // DELETE /api/v1/admin/projects/:id
					projects.GET("/members/candidates", controllers.GetProjectMemberCandidates)
					projects.GET("/:id/members", controllers.GetProjectMembers)
					projects.POST("/:id/members", controllers.CreateProjectMember)
					projects.PUT("/:id/members/:memberId", controllers.UpdateProjectMember)
					projects.DELETE("/:id/members/:memberId", controllers.DeleteProjectMember)
				}

				projectTypes := admin.Group("/project-types")
				{
					projectTypes.GET("", controllers.GetProjectTypes)       // GET /api/v1/admin/project-types
					projectTypes.POST("", controllers.CreateProjectType)    // POST /api/v1/admin/project-types
					projectTypes.PUT("/:id", controllers.UpdateProjectType) // PUT /api/v1/admin/project-types/:id
					projectTypes.PUT("/reorder", controllers.ReorderProjectTypes)
					projectTypes.DELETE("/:id", controllers.DeleteProjectType) // DELETE /api/v1/admin/project-types/:id
				}

				projectPlans := admin.Group("/project-budget-plans")
				{
					projectPlans.GET("", controllers.GetProjectBudgetPlans)       // GET /api/v1/admin/project-budget-plans
					projectPlans.POST("", controllers.CreateProjectBudgetPlan)    // POST /api/v1/admin/project-budget-plans
					projectPlans.PUT("/:id", controllers.UpdateProjectBudgetPlan) // PUT /api/v1/admin/project-budget-plans/:id
					projectPlans.PUT("/reorder", controllers.ReorderProjectBudgetPlans)
					projectPlans.DELETE("/:id", controllers.DeleteProjectBudgetPlan) // DELETE /api/v1/admin/project-budget-plans/:id
				}

				// ========== FUND SUBCATEGORIES MANAGEMENT ==========
				subcategories := admin.Group("/subcategories")
				{
					subcategories.GET("", controllers.GetAllSubcategories)                  // GET /api/v1/admin/subcategories
					subcategories.POST("", controllers.CreateSubcategory)                   // POST /api/v1/admin/subcategories
					subcategories.PUT("/:id", controllers.UpdateSubcategory)                // PUT /api/v1/admin/subcategories/:id
					subcategories.DELETE("/:id", controllers.DeleteSubcategory)             // DELETE /api/v1/admin/subcategories/:id
					subcategories.PATCH("/:id/toggle", controllers.ToggleSubcategoryStatus) // PATCH /api/v1/admin/subcategories/:id/toggle

					// Target roles management (existing functionality)
					subcategories.PUT("/:id/roles", controllers.UpdateSubcategoryTargetRoles) // PUT /api/v1/admin/subcategories/:id/roles
					subcategories.POST("/bulk-roles", controllers.BulkUpdateSubcategoryRoles) // POST /api/v1/admin/subcategories/bulk-roles
				}

				// ========== SUBCATEGORY BUDGETS MANAGEMENT ==========
				budgets := admin.Group("/budgets")
				{
					budgets.GET("", controllers.GetAllSubcategoryBudgets)                   // GET /api/v1/admin/budgets
					budgets.GET("/:id", controllers.GetSubcategoryBudget)                   // GET /api/v1/admin/budgets/:id
					budgets.POST("", controllers.CreateSubcategoryBudget)                   // POST /api/v1/admin/budgets
					budgets.PUT("/:id", controllers.UpdateSubcategoryBudget)                // PUT /api/v1/admin/budgets/:id
					budgets.DELETE("/:id", controllers.DeleteSubcategoryBudget)             // DELETE /api/v1/admin/budgets/:id
					budgets.PATCH("/:id/toggle", controllers.ToggleSubcategoryBudgetStatus) // PATCH /api/v1/admin/budgets/:id/toggle
				}

				admin.POST("/funds/copy-year", controllers.CopyFundConfigurationToYear)

				// ========== STATISTICS AND REPORTING ==========
				reports := admin.Group("/reports")
				{
					reports.GET("/categories", controllers.GetCategoryStats) // GET /api/v1/admin/reports/categories
				}

				// ========== APPLICATION MANAGEMENT (existing) ==========
				applications := admin.Group("/applications")
				{
					applications.GET("", controllers.GetApplications)                 // GET /api/v1/admin/applications
					applications.GET("/:id", controllers.GetApplication)              // GET /api/v1/admin/applications/:id
					applications.POST("/:id/approve", controllers.ApproveApplication) // POST /api/v1/admin/applications/:id/approve
					applications.POST("/:id/reject", controllers.RejectApplication)   // POST /api/v1/admin/applications/:id/reject
				}

				rewardConfigAdmin := admin.Group("/reward-config")
				{
					rewardConfigAdmin.GET("", controllers.GetRewardConfigAdmin) // GET /api/v1/admin/reward-config (ดูทั้งหมด ไม่ filter is_active)

					rewardConfigAdmin.POST("", controllers.CreateRewardConfig)                   // POST /api/v1/admin/reward-config
					rewardConfigAdmin.PUT("/:id", controllers.UpdateRewardConfig)                // PUT /api/v1/admin/reward-config/:id
					rewardConfigAdmin.DELETE("/:id", controllers.DeleteRewardConfig)             // DELETE /api/v1/admin/reward-config/:id
					rewardConfigAdmin.PATCH("/:id/toggle", controllers.ToggleRewardConfigStatus) // PATCH /api/v1/admin/reward-config/:id/toggle
					rewardConfigAdmin.POST("/:id/toggle", controllers.ToggleRewardConfigStatus)  // alias
				}

				endOfContractAdmin := admin.Group("/end-of-contract")
				{
					endOfContractAdmin.GET("", controllers.GetEndOfContractTermsAdmin)
					endOfContractAdmin.POST("", controllers.CreateEndOfContractTerm)
					endOfContractAdmin.PUT("/:id", controllers.UpdateEndOfContractTerm)
					endOfContractAdmin.DELETE("/:id", controllers.DeleteEndOfContractTerm)
					endOfContractAdmin.PATCH("/reorder", controllers.ReorderEndOfContractTerms)
				}

				// ========== USER MANAGEMENT (if needed in future) ==========
				// users := admin.Group("/users")
				// {
				//     users.GET("", controllers.GetAllUsers)
				//     users.PUT("/:id/role", controllers.UpdateUserRole)
				// }

				// User folders management
				admin.GET("/files/users", controllers.ListUserFolders)   // ดู user folders ทั้งหมด
				admin.GET("/files/users/:id", controllers.ListUserFiles) // ดูไฟล์ของ user
				admin.GET("/files/stats", controllers.GetFileStats)      // สถิติการใช้งานไฟล์

				// ===== ANNOUNCEMENT ADMIN ROUTES =====
				admin.GET("/announcements/stats", controllers.GetAnnouncementStats) // สถิติประกาศ

				// File cleanup and maintenance
				admin.DELETE("/files/cleanup", controllers.CleanupTempFiles) // ลบไฟล์ temp เก่า
				admin.POST("/files/backup/:id", controllers.BackupUserData)  // backup ข้อมูล user

				// File system utilities (เพิ่มเติมในอนาคต)
				// admin.GET("/files/orphaned", controllers.FindOrphanedFiles)     // หาไฟล์ที่ไม่มีใน DB
				// admin.DELETE("/files/orphaned", controllers.DeleteOrphanedFiles) // ลบไฟล์ที่ไม่มีใน DB

				// ===== SYSTEM CONFIG (Admin) =====
				systemConfig := admin.Group("/system-config")
				{
					systemConfig.GET("", controllers.GetSystemConfigAdmin)
					systemConfig.PUT("", controllers.UpdateSystemConfigWindow)
					systemConfig.PATCH("/announcements/:slot", controllers.UpdateSystemConfigAnnouncement)
					systemConfig.GET("/announcements/:slot/history", controllers.ListAnnouncementHistory)
					systemConfig.GET("/dept-head/history", controllers.ListDeptHeadHistory)
					systemConfig.POST("/dept-head/assign", controllers.AssignDeptHead)
				}

				submissionManagement := admin.Group("/submissions")
				{
					submissionManagement.POST("/:id/documents/resequence", controllers.AdminResequenceSubmissionDocuments)
					// Detail view
					submissionManagement.GET("/:id/details", controllers.GetSubmissionDetails)

					researchFund := submissionManagement.Group("/:id/research-fund")
					{
						researchFund.GET("/events", controllers.ListResearchFundEvents)
						researchFund.POST("/events", controllers.CreateResearchFundEvent)
						researchFund.POST("/toggle-closure", controllers.ToggleResearchFundClosure)
					}
					// Approval flow (cleaned: only new endpoints)
					submissionManagement.PATCH("/:id/publication-reward/approval-amounts", controllers.UpdatePublicationRewardApprovalAmounts)
					submissionManagement.POST("/:id/approve", controllers.ApproveSubmission)
					submissionManagement.POST("/:id/reject", controllers.RejectSubmission)
					submissionManagement.POST("/:id/request-revision", controllers.RequestSubmissionRevision)
				}

				legacySubmissions := admin.Group("/legacy-submissions")
				{
					legacySubmissions.GET("", controllers.AdminLegacyListSubmissions)
					legacySubmissions.GET("/:id", controllers.AdminLegacyGetSubmission)
					legacySubmissions.POST("", controllers.AdminLegacyCreateSubmission)
					legacySubmissions.PUT("/:id", controllers.AdminLegacyUpdateSubmission)
					legacySubmissions.DELETE("/:id", controllers.AdminLegacyDeleteSubmission)
				}

				documentTypes := admin.Group("/document-types")
				{
					documentTypes.GET("", controllers.GetDocumentTypesAdmin)     // GET /api/v1/admin/document-types
					documentTypes.POST("", controllers.CreateDocumentType)       // POST /api/v1/admin/document-types
					documentTypes.PUT("/:id", controllers.UpdateDocumentType)    // PUT /api/v1/admin/document-types/:id
					documentTypes.DELETE("/:id", controllers.DeleteDocumentType) // DELETE /api/v1/admin/document-types/:id
				}
			}

			// Fund API - Structured data endpoint
			protected.GET("/funds/structure", controllers.GetFundStructure)

			// Teacher specific fund structure
			teacher.GET("/funds/structure", controllers.GetFundStructure)

			// Staff specific fund structure
			staff.GET("/funds/structure", controllers.GetFundStructure)

		}
	}

	// Catch-all route for 404
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Endpoint not found",
			"path":    c.Request.URL.Path,
			"method":  c.Request.Method,
		})
	})
}

func RegisterLogRoute(r *gin.Engine) {
	r.GET("/logs", func(c *gin.Context) {
		// Set your access token here
		const accessToken = "secret-token"

		// Validate query token
		if c.Query("token") != accessToken {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// Path to your log file
		logData, err := os.ReadFile("fund-api.log")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read log"})
			return
		}

		// Return log content
		c.Data(http.StatusOK, "text/plain; charset=utf-8", logData)
	})
}

func RegisterUploadRoutes(rg *gin.RouterGroup) {
	rg.POST("/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No file found"})
			return
		}

		uploadRoot := os.Getenv("UPLOAD_PATH")
		if uploadRoot == "" {
			uploadRoot = "./uploads"
		}

		if err := utils.EnsureDirectoryExists(uploadRoot); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare upload directory"})
			return
		}

		sanitizedFilename := utils.SanitizeForFilename(file.Filename)
		safeFilename := utils.GenerateUniqueFilename(uploadRoot, file.Filename)

		duplicateIndex := 0
		if safeFilename != sanitizedFilename {
			ext := filepath.Ext(sanitizedFilename)
			nameWithoutExt := sanitizedFilename[:len(sanitizedFilename)-len(ext)]
			suffix := strings.TrimSuffix(strings.TrimPrefix(safeFilename, nameWithoutExt+"_"), ext)
			if suffix != safeFilename {
				if n, err := strconv.Atoi(suffix); err == nil {
					duplicateIndex = n
				}
			}
		}
		dst := filepath.Join(uploadRoot, safeFilename)
		if err := c.SaveUploadedFile(file, dst); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		filePayload := gin.H{
			"original_name":   file.Filename,
			"sanitized_name":  sanitizedFilename,
			"stored_name":     safeFilename,
			"url":             fmt.Sprintf("/uploads/%s", safeFilename),
			"was_renamed":     safeFilename != sanitizedFilename,
			"duplicate_index": duplicateIndex,
		}
		if duplicateIndex == 0 {
			delete(filePayload, "duplicate_index")
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "File uploaded successfully",
			"file":    filePayload,
		})
	})
}

func RegisterFileRoutes(rg *gin.RouterGroup) {
	// List files and folders in a directory (supports nested paths)
	rg.GET("/files", func(c *gin.Context) {
		// Get path parameter (empty string means root directory)
		requestPath := c.Query("path")

		// Sanitize path to prevent directory traversal
		if strings.Contains(requestPath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// Build full path
		fullPath := "./uploads"
		if requestPath != "" {
			fullPath = filepath.Join("./uploads", requestPath)
		}

		// Check if directory exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Directory not found"})
			return
		}

		// Read directory contents
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read directory"})
			return
		}

		var fileList []gin.H
		var folderList []gin.H

		for _, entry := range entries {
			entryPath := filepath.Join(fullPath, entry.Name())
			info, err := os.Stat(entryPath)
			if err != nil {
				continue // Skip problematic entries
			}

			if entry.IsDir() {
				// It's a folder
				folderList = append(folderList, gin.H{
					"name": entry.Name(),
					"type": "folder",
				})
			} else {
				// It's a file
				var fileURL string
				if requestPath != "" {
					fileURL = "/uploads/" + requestPath + "/" + entry.Name()
				} else {
					fileURL = "/uploads/" + entry.Name()
				}

				fileList = append(fileList, gin.H{
					"name": entry.Name(),
					"url":  fileURL,
					"size": info.Size(),
					"type": "file",
				})
			}
		}

		// Sort folders and files alphabetically
		sort.Slice(folderList, func(i, j int) bool {
			return folderList[i]["name"].(string) < folderList[j]["name"].(string)
		})
		sort.Slice(fileList, func(i, j int) bool {
			return fileList[i]["name"].(string) < fileList[j]["name"].(string)
		})

		c.JSON(http.StatusOK, gin.H{
			"folders": folderList,
			"files":   fileList,
		})
	})

	// Create a new folder
	rg.POST("/folders", func(c *gin.Context) {
		var request struct {
			Path string `json:"path" binding:"required"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}

		// Sanitize path
		if strings.Contains(request.Path, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// Build full path
		fullPath := filepath.Join("./uploads", request.Path)

		// Check if folder already exists
		if _, err := os.Stat(fullPath); err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Folder already exists"})
			return
		}

		// Create folder (with parent directories if needed)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to create folder"})
			return
		}

		log.Printf("✅ Created folder: %s", request.Path)
		c.JSON(http.StatusOK, gin.H{"message": "Folder created successfully"})
	})

	// Delete a folder
	rg.DELETE("/folders/*path", func(c *gin.Context) {
		rawPath := strings.TrimPrefix(c.Param("path"), "/")

		// Decode URL
		folderPath, err := url.QueryUnescape(rawPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder path"})
			return
		}

		// Sanitize path
		if strings.Contains(folderPath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		fullPath := filepath.Join("./uploads", folderPath)

		// Check if folder exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Folder not found"})
			return
		}

		// Remove folder and all its contents
		if err := os.RemoveAll(fullPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to delete folder"})
			return
		}

		log.Printf("✅ Deleted folder: %s", folderPath)
		c.JSON(http.StatusOK, gin.H{"message": "Folder deleted successfully"})
	})

	// Delete a file (enhanced to support nested paths)
	rg.DELETE("/files/*path", func(c *gin.Context) {
		rawPath := strings.TrimPrefix(c.Param("path"), "/")

		// Decode URL
		filePath, err := url.QueryUnescape(rawPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file path"})
			return
		}

		// Sanitize path
		if strings.Contains(filePath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// Prevent deletion of HTML files
		if strings.HasSuffix(filePath, ".html") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete HTML files"})
			return
		}

		fullPath := filepath.Join("./uploads", filePath)

		// Check if file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}

		if err := os.Remove(fullPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to delete file"})
			return
		}

		log.Printf("✅ Deleted file: %s", filePath)
		c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
	})
}

// You'll also need to update your upload handler to support paths
// Add this to your upload route handler:
func HandleFileUpload(c *gin.Context) {
	// Get the uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	// Get the optional path parameter
	uploadPath := c.PostForm("path")

	// Sanitize path
	if strings.Contains(uploadPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
		return
	}

	// Build destination directory
	destDir := "./uploads"
	if uploadPath != "" {
		destDir = filepath.Join("./uploads", uploadPath)

		// Create directory if it doesn't exist
		if err := os.MkdirAll(destDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to create directory"})
			return
		}
	}

	// Build full file path
	filePath := filepath.Join(destDir, file.Filename)

	// Save the file
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to save file"})
		return
	}

	log.Printf("✅ Uploaded file: %s to %s", file.Filename, uploadPath)
	c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully"})
}
