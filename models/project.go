package models

import "time"

// Project represents the projects table
type Project struct {
	ProjectID          uint       `gorm:"primaryKey;column:project_id" json:"project_id"`
	ProjectName        string     `gorm:"column:project_name" json:"project_name"`
	TypeID             uint       `gorm:"column:type_id" json:"type_id"`
	EventDate          time.Time  `gorm:"column:event_date" json:"event_date"`
	PlanID             uint       `gorm:"column:plan_id" json:"plan_id"`
	BudgetAmount       float64    `gorm:"column:budget_amount" json:"budget_amount"`
	Participants       int        `gorm:"column:participants" json:"participants"`
	BeneficiariesCount int        `gorm:"column:beneficiaries_count" json:"beneficiaries_count"`
	Notes              *string    `gorm:"column:notes" json:"notes"`
	CreatedBy          *int       `gorm:"column:created_by" json:"created_by"`
	CreatedAt          time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt          *time.Time `gorm:"column:updated_at" json:"updated_at"`

	Type        ProjectType         `gorm:"foreignKey:TypeID;references:TypeID" json:"type"`
	BudgetPlan  ProjectBudgetPlan   `gorm:"foreignKey:PlanID;references:PlanID" json:"budget_plan"`
	Attachments []ProjectAttachment `gorm:"foreignKey:ProjectID;references:ProjectID" json:"attachments"`
	Members     []ProjectMember     `gorm:"foreignKey:ProjectID;references:ProjectID" json:"members"`
}

// TableName overrides the table name for Project
func (Project) TableName() string {
	return "projects"
}

// ProjectType represents the project_types table
type ProjectType struct {
	TypeID       uint   `gorm:"primaryKey;column:type_id" json:"type_id"`
	NameTH       string `gorm:"column:name_th" json:"name_th"`
	NameEN       string `gorm:"column:name_en" json:"name_en"`
	DisplayOrder int    `gorm:"column:display_order" json:"display_order"`
	IsActive     bool   `gorm:"column:is_active" json:"is_active"`
}

// TableName overrides the table name for ProjectType
func (ProjectType) TableName() string {
	return "project_types"
}

// ProjectBudgetPlan represents the project_budget_plans table
type ProjectBudgetPlan struct {
	PlanID       uint   `gorm:"primaryKey;column:plan_id" json:"plan_id"`
	NameTH       string `gorm:"column:name_th" json:"name_th"`
	NameEN       string `gorm:"column:name_en" json:"name_en"`
	DisplayOrder int    `gorm:"column:display_order" json:"display_order"`
	IsActive     bool   `gorm:"column:is_active" json:"is_active"`
}

// TableName overrides the table name for ProjectBudgetPlan
func (ProjectBudgetPlan) TableName() string {
	return "project_budget_plans"
}

// ProjectAttachment represents the project_attachments table
type ProjectAttachment struct {
	FileID       uint       `gorm:"primaryKey;column:file_id" json:"file_id"`
	ProjectID    uint       `gorm:"column:project_id" json:"project_id"`
	OriginalName string     `gorm:"column:original_name" json:"original_name"`
	StoredPath   string     `gorm:"column:stored_path" json:"stored_path"`
	FileSize     uint64     `gorm:"column:file_size" json:"file_size"`
	MimeType     string     `gorm:"column:mime_type" json:"mime_type"`
	FileHash     *string    `gorm:"column:file_hash" json:"file_hash"`
	IsPublic     bool       `gorm:"column:is_public" json:"is_public"`
	UploadedBy   *int       `gorm:"column:uploaded_by" json:"uploaded_by"`
	UploadedAt   time.Time  `gorm:"column:uploaded_at" json:"uploaded_at"`
	CreateAt     time.Time  `gorm:"column:create_at" json:"create_at"`
	UpdateAt     time.Time  `gorm:"column:update_at" json:"update_at"`
	DeleteAt     *time.Time `gorm:"column:delete_at" json:"delete_at"`
	DisplayOrder int        `gorm:"column:display_order" json:"display_order"`
}

// TableName overrides the table name for ProjectAttachment
func (ProjectAttachment) TableName() string {
	return "project_attachments"
}

// ProjectMember represents the project_members table
type ProjectMember struct {
	MemberID      uint       `gorm:"primaryKey;column:member_id" json:"member_id"`
	ProjectID     uint       `gorm:"column:project_id" json:"project_id"`
	UserID        uint       `gorm:"column:user_id" json:"user_id"`
	Duty          string     `gorm:"column:duty" json:"duty"`
	WorkloadHours float64    `gorm:"column:workload_hours" json:"workload_hours"`
	DisplayOrder  int        `gorm:"column:display_order" json:"display_order"`
	Notes         *string    `gorm:"column:notes" json:"notes"`
	CreatedAt     time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     *time.Time `gorm:"column:updated_at" json:"updated_at"`

	User User `gorm:"foreignKey:UserID;references:UserID" json:"user"`
}

// TableName overrides the table name for ProjectMember
func (ProjectMember) TableName() string {
	return "project_members"
}
