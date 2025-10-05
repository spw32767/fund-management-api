// models/announcement.go
package models

import (
	"fmt"
	"time"
)

// Announcement represents the announcements table
type Announcement struct {
	AnnouncementID int     `gorm:"primaryKey;column:announcement_id" json:"announcement_id"`
	Title          string  `gorm:"column:title" json:"title"`
	Description    *string `gorm:"column:description" json:"description"`
	FileName       string  `gorm:"column:file_name" json:"file_name"`
	FilePath       string  `gorm:"column:file_path" json:"file_path"`
	FileSize       *int64  `gorm:"column:file_size" json:"file_size"`
	MimeType       *string `gorm:"column:mime_type" json:"mime_type"`

	// ⬇️ ถ้าจะรองรับค่าจากหน้า admin ที่มีตัวเลือก "fund_application"
	// เปลี่ยน enum ให้ครอบคลุมด้วย
	AnnouncementType string `gorm:"column:announcement_type;type:enum('general','research_fund','promotion_fund','fund_application');default:'general'" json:"announcement_type"`

	Priority     string     `gorm:"column:priority;type:enum('normal','high','urgent');default:'normal'" json:"priority"`
	DisplayOrder *int       `gorm:"column:display_order" json:"display_order"`
	Status       string     `gorm:"column:status;type:enum('active','inactive');default:'active'" json:"status"`
	PublishedAt  *time.Time `gorm:"column:published_at" json:"published_at"`
	ExpiredAt    *time.Time `gorm:"column:expired_at" json:"expired_at"`
	YearID       *int       `gorm:"column:year_id" json:"year_id"`

	// ⬅️ เพิ่มฟิลด์นี้
	AnnouncementReferenceNumber *string `gorm:"column:announcement_reference_number" json:"announcement_reference_number"`

	CreatedBy int        `gorm:"column:created_by" json:"created_by"`
	CreateAt  time.Time  `gorm:"column:create_at" json:"create_at"`
	UpdateAt  time.Time  `gorm:"column:update_at" json:"update_at"`
	DeleteAt  *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	Creator User `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Year    Year `gorm:"foreignKey:YearID;references:YearID" json:"year_info,omitempty"`
}

// FundForm represents the fund_forms table
// ===== FundForm Model (ตามสคีมาจริง) =====
type FundForm struct {
	FormID      int     `gorm:"primaryKey;column:form_id" json:"form_id"`
	Title       string  `gorm:"column:title" json:"title"`
	Description *string `gorm:"column:description" json:"description"`

	FileName string  `gorm:"column:file_name" json:"file_name"`
	FilePath string  `gorm:"column:file_path" json:"file_path"`
	FileSize *int64  `gorm:"column:file_size" json:"file_size"`
	MimeType *string `gorm:"column:mime_type" json:"mime_type"`

	FormType     string `gorm:"column:form_type;type:enum('application','report','evaluation','guidelines','other');default:'application'" json:"form_type"`
	FundCategory string `gorm:"column:fund_category;type:enum('research_fund','promotion_fund','both');default:'both'" json:"fund_category"`

	// ตามตารางจริง
	IsRequired   *bool  `gorm:"column:is_required" json:"is_required"`
	DisplayOrder *int   `gorm:"column:display_order" json:"display_order"`
	Status       string `gorm:"column:status;type:enum('active','inactive','archived');default:'active'" json:"status"`
	YearID       *int   `gorm:"column:year_id" json:"year_id"`

	CreatedBy int        `gorm:"column:created_by" json:"created_by"`
	CreateAt  time.Time  `gorm:"column:create_at" json:"create_at"`
	UpdateAt  time.Time  `gorm:"column:update_at" json:"update_at"`
	DeleteAt  *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	Creator User `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Year    Year `gorm:"foreignKey:YearID;references:YearID" json:"year_info,omitempty"`
}

// (ถ้ามีอยู่แล้วข้ามได้) helper ตัวอย่าง
func (f *FundForm) GetFileSizeReadable() string {
	if f.FileSize == nil {
		return ""
	}
	kb := float64(*f.FileSize) / 1024.0
	if kb >= 1024 {
		return fmt.Sprintf("%.2f MB", kb/1024.0)
	}
	return fmt.Sprintf("%.2f KB", kb)
}
func (f *FundForm) IsActive() bool { return f.Status == "active" }
func (f *FundForm) GetStatusName() string {
	if f.Status == "active" {
		return "เปิดใช้งาน"
	}
	return "ปิดใช้งาน"
}
func (f *FundForm) GetFormTypeName() string {
	switch f.FormType {
	case "application":
		return "แบบฟอร์มสมัคร"
	case "report":
		return "แบบฟอร์มรายงาน"
	case "evaluation":
		return "แบบฟอร์มประเมิน"
	case "guidelines":
		return "แนวทางปฏิบัติ"
	default:
		return "อื่นๆ"
	}
}
func (f *FundForm) GetFundCategoryName() string {
	switch f.FundCategory {
	case "research_fund":
		return "ทุนส่งเสริมการวิจัย"
	case "promotion_fund":
		return "ทุนอุดหนุนกิจกรรม"
	case "both":
		return "ทั้งสองประเภท"
	default:
		return ""
	}
}

func (f *FundForm) ToResponse() FundFormResponse {
	resp := FundFormResponse{
		FormID:           f.FormID,
		Title:            f.Title,
		Description:      f.Description,
		FileName:         f.FileName,
		FilePath:         f.FilePath,
		FileSize:         f.FileSize,
		FileSizeReadable: f.GetFileSizeReadable(),
		MimeType:         f.MimeType,
		FormType:         f.FormType,
		FormTypeName:     f.GetFormTypeName(),
		FundCategory:     f.FundCategory,
		FundCategoryName: f.GetFundCategoryName(),
		DisplayOrder:     f.DisplayOrder,
		Status:           f.Status,
		StatusName:       f.GetStatusName(),
		IsActive:         f.IsActive(),
		YearID:           f.YearID,
		CreatedBy:        f.CreatedBy,
		CreateAt:         f.CreateAt,
		UpdateAt:         f.UpdateAt,
	}
	if f.Creator.UserID != 0 {
		resp.CreatorName = f.Creator.UserFname + " " + f.Creator.UserLname
	}
	if f.Year.Year != "" {
		y := f.Year.Year
		resp.Year = &y
	}
	return resp
}

// AnnouncementView represents the announcement_views table (Optional tracking)
type AnnouncementView struct {
	ViewID         int       `gorm:"primaryKey;column:view_id" json:"view_id"`
	AnnouncementID int       `gorm:"column:announcement_id" json:"announcement_id"`
	UserID         *int      `gorm:"column:user_id" json:"user_id"`
	IPAddress      *string   `gorm:"column:ip_address" json:"ip_address"`
	UserAgent      *string   `gorm:"column:user_agent" json:"user_agent"`
	ViewedAt       time.Time `gorm:"column:viewed_at" json:"viewed_at"`

	// Relations
	Announcement Announcement `gorm:"foreignKey:AnnouncementID" json:"announcement,omitempty"`
	User         *User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// FormDownload represents the form_downloads table (Optional tracking)
type FormDownload struct {
	DownloadID   int       `gorm:"primaryKey;column:download_id" json:"download_id"`
	FormID       int       `gorm:"column:form_id" json:"form_id"`
	UserID       *int      `gorm:"column:user_id" json:"user_id"`
	IPAddress    *string   `gorm:"column:ip_address" json:"ip_address"`
	UserAgent    *string   `gorm:"column:user_agent" json:"user_agent"`
	DownloadedAt time.Time `gorm:"column:downloaded_at" json:"downloaded_at"`

	// Relations
	FundForm FundForm `gorm:"foreignKey:FormID" json:"fund_form,omitempty"`
	User     *User    `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName overrides
func (Announcement) TableName() string {
	return "announcements"
}

func (FundForm) TableName() string {
	return "fund_forms"
}

func (AnnouncementView) TableName() string {
	return "announcement_views"
}

func (FormDownload) TableName() string {
	return "form_downloads"
}

// ===== Helper methods สำหรับ Announcement =====

// IsExpired ตรวจสอบว่าประกาศหมดอายุหรือไม่
func (a *Announcement) IsExpired() bool {
	if a.ExpiredAt == nil {
		return false
	}
	return a.ExpiredAt.Before(time.Now())
}

// IsActive ตรวจสอบว่าประกาศเปิดใช้งานและยังไม่หมดอายุ
func (a *Announcement) IsActive() bool {
	return a.Status == "active" && !a.IsExpired()
}

// GetAnnouncementTypeName แปลงประเภทประกาศเป็นภาษาไทย
func (a *Announcement) GetAnnouncementTypeName() string {
	switch a.AnnouncementType {
	case "general":
		return "ประกาศทั่วไป"
	case "research_fund":
		return "ทุนส่งเสริมการวิจัย"
	case "promotion_fund":
		return "ทุนอุดหนุนกิจกรรม"
	default:
		return a.AnnouncementType
	}
}

// GetPriorityName แปลงระดับความสำคัญเป็นภาษาไทย
func (a *Announcement) GetPriorityName() string {
	switch a.Priority {
	case "normal":
		return "ปกติ"
	case "high":
		return "สำคัญ"
	case "urgent":
		return "ด่วน"
	default:
		return a.Priority
	}
}

// GetStatusName แปลงสถานะเป็นภาษาไทย
func (a *Announcement) GetStatusName() string {
	switch a.Status {
	case "active":
		return "เปิดใช้งาน"
	case "inactive":
		return "ปิดใช้งาน"
	default:
		return a.Status
	}
}

// GetFileSizeReadable แปลงขนาดไฟล์เป็นรูปแบบที่อ่านง่าย
func (a *Announcement) GetFileSizeReadable() string {
	if a.FileSize == nil {
		return "ไม่ทราบขนาด"
	}

	size := float64(*a.FileSize)
	units := []string{"B", "KB", "MB", "GB"}

	for i, unit := range units {
		if size < 1024 || i == len(units)-1 {
			if i == 0 {
				return fmt.Sprintf("%.0f %s", size, unit)
			}
			return fmt.Sprintf("%.2f %s", size, unit)
		}
		size /= 1024
	}
	return fmt.Sprintf("%.2f %s", size, units[len(units)-1])
}

// ===== Request/Response DTOs =====

// AnnouncementCreateRequest for creating announcements
type AnnouncementCreateRequest struct {
	Title       string  `json:"title" binding:"required"`
	Description *string `json:"description"`
	// ⬇️ ถ้าหน้าบ้านมีค่า "fund_application" ให้เพิ่มใน oneof ด้วย
	AnnouncementType string     `json:"announcement_type" binding:"required,oneof=general research_fund promotion_fund fund_application"`
	Priority         string     `json:"priority" binding:"oneof=normal high urgent"`
	DisplayOrder     *int       `json:"display_order"`
	Status           string     `json:"status" binding:"oneof=active inactive"`
	PublishedAt      *time.Time `json:"published_at"`
	ExpiredAt        *time.Time `json:"expired_at"`

	// ⬅️ เพิ่มสองบรรทัดนี้
	YearID                      *int    `json:"year_id"`
	AnnouncementReferenceNumber *string `json:"announcement_reference_number"`
}

type AnnouncementUpdateRequest struct {
	Title            *string    `json:"title"`
	Description      *string    `json:"description"`
	AnnouncementType *string    `json:"announcement_type" binding:"omitempty,oneof=general research_fund promotion_fund fund_application"`
	Priority         *string    `json:"priority" binding:"omitempty,oneof=normal high urgent"`
	DisplayOrder     *int       `json:"display_order"`
	Status           *string    `json:"status" binding:"omitempty,oneof=active inactive"`
	PublishedAt      *time.Time `json:"published_at"`
	ExpiredAt        *time.Time `json:"expired_at"`

	// ⬅️ เพิ่มสองบรรทัดนี้
	YearID                      *int    `json:"year_id"`
	AnnouncementReferenceNumber *string `json:"announcement_reference_number"`
}

// ===== FundForm DTOs (ตามสคีมา) =====
type FundFormCreateRequest struct {
	Title       string  `json:"title" binding:"required"`
	Description *string `json:"description"`

	FormType     string `json:"form_type" binding:"omitempty,oneof=application report evaluation guidelines other"`
	FundCategory string `json:"fund_category" binding:"omitempty,oneof=research_fund promotion_fund both"`

	IsRequired   *bool  `json:"is_required"`
	DisplayOrder *int   `json:"display_order"`
	Status       string `json:"status" binding:"omitempty,oneof=active inactive archived"`
	YearID       *int   `json:"year_id"`
}

type FundFormUpdateRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`

	FormType     *string `json:"form_type" binding:"omitempty,oneof=application report evaluation guidelines other"`
	FundCategory *string `json:"fund_category" binding:"omitempty,oneof=research_fund promotion_fund both"`

	IsRequired   *bool   `json:"is_required"`
	DisplayOrder *int    `json:"display_order"`
	Status       *string `json:"status" binding:"omitempty,oneof=active inactive archived"`
	YearID       *int    `json:"year_id"`
}

// AnnouncementResponse for API responses
type AnnouncementResponse struct {
	AnnouncementID       int        `json:"announcement_id"`
	Title                string     `json:"title"`
	Description          *string    `json:"description"`
	FileName             string     `json:"file_name"`
	FilePath             string     `json:"file_path"`
	FileSize             *int64     `json:"file_size"`
	FileSizeReadable     string     `json:"file_size_readable"`
	MimeType             *string    `json:"mime_type"`
	AnnouncementType     string     `json:"announcement_type"`
	AnnouncementTypeName string     `json:"announcement_type_name"`
	Priority             string     `json:"priority"`
	PriorityName         string     `json:"priority_name"`
	DisplayOrder         *int       `json:"display_order"`
	Status               string     `json:"status"`
	StatusName           string     `json:"status_name"`
	PublishedAt          *time.Time `json:"published_at"`
	ExpiredAt            *time.Time `json:"expired_at"`
	IsExpired            bool       `json:"is_expired"`
	IsActive             bool       `json:"is_active"`
	YearID               *int       `json:"year_id"`
	Year                 *string    `json:"year"`
	CreatedBy            int        `json:"created_by"`
	CreatorName          string     `json:"creator_name,omitempty"`
	CreateAt             time.Time  `json:"create_at"`
	UpdateAt             time.Time  `json:"update_at"`

	// ⬅️ เพิ่มบรรทัดนี้
	AnnouncementReferenceNumber *string `json:"announcement_reference_number"`
}

type FundFormResponse struct {
	FormID           int     `json:"form_id"`
	Title            string  `json:"title"`
	Description      *string `json:"description"`
	FileName         string  `json:"file_name"`
	FilePath         string  `json:"file_path"`
	FileSize         *int64  `json:"file_size"`
	FileSizeReadable string  `json:"file_size_readable"`
	MimeType         *string `json:"mime_type"`
	FormType         string  `json:"form_type"`
	FormTypeName     string  `json:"form_type_name"`
	FundCategory     string  `json:"fund_category"`
	FundCategoryName string  `json:"fund_category_name"`
	// ✨ แสดงลำดับ
	DisplayOrder  *int       `json:"display_order"`
	Version       string     `json:"version"`
	IsRequired    bool       `json:"is_required"`
	Status        string     `json:"status"`
	StatusName    string     `json:"status_name"`
	EffectiveDate *time.Time `json:"effective_date"`
	ExpiryDate    *time.Time `json:"expiry_date"`
	IsExpired     bool       `json:"is_expired"`
	IsActive      bool       `json:"is_active"`
	YearID        *int       `json:"year_id"`
	Year          *string    `json:"year"`
	CreatedBy     int        `json:"created_by"`
	CreatorName   string     `json:"creator_name,omitempty"`
	CreateAt      time.Time  `json:"create_at"`
	UpdateAt      time.Time  `json:"update_at"`
}

// ===== Conversion methods =====

// ToResponse converts Announcement to AnnouncementResponse
func (a *Announcement) ToResponse() AnnouncementResponse {
	resp := AnnouncementResponse{
		AnnouncementID:       a.AnnouncementID,
		Title:                a.Title,
		Description:          a.Description,
		FileName:             a.FileName,
		FilePath:             a.FilePath,
		FileSize:             a.FileSize,
		FileSizeReadable:     a.GetFileSizeReadable(),
		MimeType:             a.MimeType,
		AnnouncementType:     a.AnnouncementType,
		AnnouncementTypeName: a.GetAnnouncementTypeName(),
		Priority:             a.Priority,
		PriorityName:         a.GetPriorityName(),
		DisplayOrder:         a.DisplayOrder,
		Status:               a.Status,
		StatusName:           a.GetStatusName(),
		PublishedAt:          a.PublishedAt,
		ExpiredAt:            a.ExpiredAt,
		IsExpired:            a.IsExpired(),
		IsActive:             a.IsActive(),
		YearID:               a.YearID,
		CreatedBy:            a.CreatedBy,
		CreateAt:             a.CreateAt,
		UpdateAt:             a.UpdateAt,

		// ⬅️ map ค่าเลขอ้างอิง
		AnnouncementReferenceNumber: a.AnnouncementReferenceNumber,
	}

	if a.Creator.UserID != 0 {
		resp.CreatorName = a.Creator.UserFname + " " + a.Creator.UserLname
	}
	if a.Year.Year != "" {
		year := a.Year.Year
		resp.Year = &year
	}
	return resp
}
