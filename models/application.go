package models

import (
	"encoding/json"
	"fmt"
	"time"
)

type FundApplication struct {
	ApplicationID       int        `gorm:"primaryKey;column:application_id" json:"application_id"`
	UserID              int        `gorm:"column:user_id" json:"user_id"`
	YearID              int        `gorm:"column:year_id" json:"year_id"`
	SubcategoryID       int        `gorm:"column:subcategory_id" json:"subcategory_id"`
	ApplicationStatusID int        `gorm:"column:application_status_id" json:"application_status_id"`
	ApplicationNumber   string     `gorm:"column:application_number" json:"application_number"`
	ApprovedBy          string     `gorm:"column:approved_by" json:"approved_by,omitempty"`
	ProjectTitle        string     `gorm:"column:project_title" json:"project_title"`
	ProjectDescription  string     `gorm:"column:project_description" json:"project_description"`
	RequestedAmount     float64    `gorm:"column:requested_amount" json:"requested_amount"`
	ApprovedAmount      float64    `gorm:"column:approved_amount" json:"approved_amount"`
	SubmittedAt         *time.Time `gorm:"column:submitted_at" json:"submitted_at"`
	ApprovedAt          *time.Time `gorm:"column:approved_at" json:"approved_at,omitempty"`
	ClosedAt            *time.Time `gorm:"column:closed_at" json:"closed_at,omitempty"`
	Comment             string     `gorm:"column:comment" json:"comment,omitempty"`
	CreateAt            *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt            *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt            *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	// Relations
	User              User              `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Year              Year              `gorm:"foreignKey:YearID" json:"year,omitempty"`
	Subcategory       FundSubcategory   `gorm:"foreignKey:SubcategoryID" json:"subcategory,omitempty"`
	ApplicationStatus ApplicationStatus `gorm:"foreignKey:ApplicationStatusID" json:"application_status,omitempty"`
}

type ApplicationStatus struct {
	ApplicationStatusID int        `gorm:"primaryKey;column:application_status_id" json:"application_status_id"`
	StatusCode          string     `gorm:"column:status_code" json:"status_code"`
	StatusName          string     `gorm:"column:status_name" json:"status_name"`
	CreateAt            *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt            *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt            *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

type Year struct {
	YearID   int        `gorm:"primaryKey;column:year_id" json:"year_id"`
	Year     string     `gorm:"column:year" json:"year"`
	Budget   float64    `gorm:"column:budget" json:"budget"`
	Status   string     `gorm:"column:status" json:"status"`
	CreateAt *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

type FundCategory struct {
	CategoryID   int        `gorm:"primaryKey;column:category_id" json:"category_id"`
	CategoryName string     `gorm:"column:category_name" json:"category_name"`
	Status       string     `gorm:"column:status" json:"status"`
	YearID       int        `gorm:"column:year_id" json:"year_id"`
	CreateAt     *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt     *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt     *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

type FundSubcategory struct {
	SubcategoryID   int    `gorm:"primaryKey;column:subcategory_id" json:"subcategory_id"`
	CategoryID      int    `gorm:"column:category_id" json:"category_id"`
	SubcategoryName string `gorm:"column:subcategory_name" json:"subcategory_name"`
	//YearID          int        `gorm:"column:year_id" json:"year_id"`
	FundCondition *string    `gorm:"column:fund_condition" json:"fund_condition"`
	TargetRoles   *string    `gorm:"column:target_roles" json:"target_roles"` // Updated to include target_roles
	FormType      string     `gorm:"column:form_type" json:"form_type"`       // 'download', 'publication_reward', 'research_proposal', etc.
	FormURL       string     `gorm:"column:form_url" json:"form_url"`         // URL สำหรับดาวน์โหลดฟอร์ม
	Status        string     `gorm:"column:status" json:"status"`
	Comment       *string    `gorm:"column:comment" json:"comment"`
	CreateAt      *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt      *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt      *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	// Relations
	Category          FundCategory      `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	SubcategoryBudget SubcategoryBudget `gorm:"foreignKey:SubcategoryID" json:"subcategory_budget,omitempty"`
}

type SubcategoryBudget struct {
	SubcategoryBudgetID int        `gorm:"primaryKey;column:subcategory_budget_id" json:"subcategory_budget_id"`
	SubcategoryID       int        `gorm:"column:subcategory_id" json:"subcategory_id"` // Note: typo in DB
	AllocatedAmount     float64    `gorm:"column:allocated_amount" json:"allocated_amount"`
	UsedAmount          float64    `gorm:"column:used_amount" json:"used_amount"`
	RemainingBudget     float64    `gorm:"column:remaining_budget" json:"remaining_budget"`
	MaxGrants           int        `gorm:"column:max_grants" json:"max_grants"`
	MaxAmountPerGrant   float64    `gorm:"column:max_amount_per_grant" json:"max_amount_per_grant"`
	RemainingGrant      int        `gorm:"column:remaining_grant" json:"remaining_grant"`
	Level               string     `gorm:"column:level" json:"level,omitempty"`
	Status              string     `gorm:"column:status" json:"status"`
	FundDescription     string     `gorm:"column:fund_description" json:"fund_description,omitempty"`
	Comment             string     `gorm:"column:comment" json:"comment,omitempty"`
	CreateAt            *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt            *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt            *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

// TableName overrides
func (FundApplication) TableName() string {
	return "fund_applications"
}

func (ApplicationStatus) TableName() string {
	return "application_status"
}

func (Year) TableName() string {
	return "years"
}

func (FundCategory) TableName() string {
	return "fund_categories"
}

func (FundSubcategory) TableName() string {
	return "fund_subcategories"
}

func (SubcategoryBudget) TableName() string {
	return "subcategory_budgets"
}

// Helper methods for target_roles JSON handling
func (fs *FundSubcategory) GetTargetRoles() []string {
	if fs.TargetRoles == nil || *fs.TargetRoles == "" {
		return nil
	}

	var roles []string
	if err := json.Unmarshal([]byte(*fs.TargetRoles), &roles); err != nil {
		return nil
	}
	return roles
}

func (fs *FundSubcategory) SetTargetRoles(roles []string) error {
	if roles == nil || len(roles) == 0 {
		fs.TargetRoles = nil
		return nil
	}

	jsonBytes, err := json.Marshal(roles)
	if err != nil {
		return err
	}

	jsonStr := string(jsonBytes)
	fs.TargetRoles = &jsonStr
	return nil
}

func (fs *FundSubcategory) IsVisibleToRole(roleID int) bool {
	// Admin (role_id = 3) sees everything
	if roleID == 3 {
		return true
	}

	// If target_roles is null or empty, everyone can see it
	if fs.TargetRoles == nil || *fs.TargetRoles == "" {
		return true
	}

	// Check if role is in target_roles
	roles := fs.GetTargetRoles()
	if roles == nil {
		return true
	}

	roleStr := fmt.Sprintf("%d", roleID)
	for _, role := range roles {
		if role == roleStr {
			return true
		}
	}

	return false
}
