// models/fund_document_requirement.go
package models

import (
	"encoding/json"
	"time"
)

// FundDocumentRequirement represents the fund_document_requirements table
type FundDocumentRequirement struct {
	RequirementID  int             `gorm:"primaryKey;column:requirement_id" json:"requirement_id"`
	FundType       string          `gorm:"column:fund_type" json:"fund_type"`
	SubcategoryID  *int            `gorm:"column:subcategory_id" json:"subcategory_id"`
	DocumentTypeID int             `gorm:"column:document_type_id" json:"document_type_id"`
	IsRequired     bool            `gorm:"column:is_required" json:"is_required"`
	DisplayOrder   int             `gorm:"column:display_order" json:"display_order"`
	ConditionRules json.RawMessage `gorm:"column:condition_rules;type:json" json:"condition_rules,omitempty"`
	IsActive       bool            `gorm:"column:is_active" json:"is_active"`
	CreatedAt      time.Time       `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time       `gorm:"column:updated_at" json:"updated_at"`

	// Relations
	Subcategory  *FundSubcategory `gorm:"foreignKey:SubcategoryID" json:"subcategory,omitempty"`
	DocumentType DocumentType     `gorm:"foreignKey:DocumentTypeID" json:"document_type,omitempty"`
}

// TableName overrides the table name
func (FundDocumentRequirement) TableName() string {
	return "fund_document_requirements"
}

// ConditionRule represents the structure of condition_rules JSON
type ConditionRule struct {
	Quartile   []string `json:"quartile,omitempty"`    // ["Q1", "T5"]
	AmountMin  *float64 `json:"amount_min,omitempty"`  // 50000
	AmountMax  *float64 `json:"amount_max,omitempty"`  // 100000
	Position   []string `json:"position,omitempty"`    // ["first_author", "corresponding_author"]
	CustomRule string   `json:"custom_rule,omitempty"` // Custom validation rule
}

// ParseConditionRules parses the JSON condition_rules into ConditionRule struct
func (fdr *FundDocumentRequirement) ParseConditionRules() (*ConditionRule, error) {
	if fdr.ConditionRules == nil {
		return nil, nil
	}

	var rules ConditionRule
	err := json.Unmarshal(fdr.ConditionRules, &rules)
	if err != nil {
		return nil, err
	}

	return &rules, nil
}

// SetConditionRules sets the condition_rules from ConditionRule struct
func (fdr *FundDocumentRequirement) SetConditionRules(rules *ConditionRule) error {
	if rules == nil {
		fdr.ConditionRules = nil
		return nil
	}

	data, err := json.Marshal(rules)
	if err != nil {
		return err
	}

	fdr.ConditionRules = data
	return nil
}

// ValidateCondition checks if the given parameters match the condition rules
func (fdr *FundDocumentRequirement) ValidateCondition(params map[string]interface{}) bool {
	rules, err := fdr.ParseConditionRules()
	if err != nil || rules == nil {
		return true // No conditions = always valid
	}

	// Check quartile condition
	if len(rules.Quartile) > 0 {
		if quartile, ok := params["quartile"].(string); ok {
			valid := false
			for _, allowedQuartile := range rules.Quartile {
				if quartile == allowedQuartile {
					valid = true
					break
				}
			}
			if !valid {
				return false
			}
		}
	}

	// Check amount conditions
	if rules.AmountMin != nil {
		if amount, ok := params["amount"].(float64); ok {
			if amount < *rules.AmountMin {
				return false
			}
		}
	}

	if rules.AmountMax != nil {
		if amount, ok := params["amount"].(float64); ok {
			if amount > *rules.AmountMax {
				return false
			}
		}
	}

	// Check position condition
	if len(rules.Position) > 0 {
		if position, ok := params["position"].(string); ok {
			valid := false
			for _, allowedPosition := range rules.Position {
				if position == allowedPosition {
					valid = true
					break
				}
			}
			if !valid {
				return false
			}
		}
	}

	return true
}

// FundDocumentRequirementView represents the v_fund_document_requirements view
type FundDocumentRequirementView struct {
	RequirementID    int             `gorm:"column:requirement_id" json:"requirement_id"`
	FundType         string          `gorm:"column:fund_type" json:"fund_type"`
	SubcategoryID    *int            `gorm:"column:subcategory_id" json:"subcategory_id"`
	SubcategoryName  string          `gorm:"column:subcategory_name" json:"subcategory_name"`
	DocumentTypeID   int             `gorm:"column:document_type_id" json:"document_type_id"`
	DocumentTypeName string          `gorm:"column:document_type_name" json:"document_type_name"`
	DocumentCode     string          `gorm:"column:document_code" json:"document_code"`
	DocumentCategory string          `gorm:"column:document_category" json:"document_category"`
	IsRequired       bool            `gorm:"column:is_required" json:"is_required"`
	DisplayOrder     int             `gorm:"column:display_order" json:"display_order"`
	ConditionRules   json.RawMessage `gorm:"column:condition_rules;type:json" json:"condition_rules,omitempty"`
	IsActive         bool            `gorm:"column:is_active" json:"is_active"`
	CreatedAt        time.Time       `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time       `gorm:"column:updated_at" json:"updated_at"`
}

// TableName for the view
func (FundDocumentRequirementView) TableName() string {
	return "v_fund_document_requirements"
}

// CreateFundDocumentRequirementRequest represents the request for creating a new requirement
type CreateFundDocumentRequirementRequest struct {
	FundType       string         `json:"fund_type" binding:"required,oneof=publication_reward fund_application"`
	SubcategoryID  *int           `json:"subcategory_id"`
	DocumentTypeID int            `json:"document_type_id" binding:"required"`
	IsRequired     bool           `json:"is_required"`
	DisplayOrder   int            `json:"display_order"`
	ConditionRules *ConditionRule `json:"condition_rules,omitempty"`
	IsActive       bool           `json:"is_active"`
}

// UpdateFundDocumentRequirementRequest represents the request for updating a requirement
type UpdateFundDocumentRequirementRequest struct {
	FundType       *string        `json:"fund_type,omitempty" binding:"omitempty,oneof=publication_reward fund_application"`
	SubcategoryID  *int           `json:"subcategory_id,omitempty"`
	DocumentTypeID *int           `json:"document_type_id,omitempty"`
	IsRequired     *bool          `json:"is_required,omitempty"`
	DisplayOrder   *int           `json:"display_order,omitempty"`
	ConditionRules *ConditionRule `json:"condition_rules,omitempty"`
	IsActive       *bool          `json:"is_active,omitempty"`
}

// DocumentRequirementQuery represents query parameters for filtering requirements
type DocumentRequirementQuery struct {
	FundType      string `form:"fund_type" binding:"omitempty,oneof=publication_reward fund_application"`
	SubcategoryID *int   `form:"subcategory_id"`
	IsRequired    *bool  `form:"is_required"`
	IsActive      *bool  `form:"is_active"`
}

// GetDocumentRequirementsResponse represents the API response
type GetDocumentRequirementsResponse struct {
	Requirements []FundDocumentRequirementView `json:"requirements"`
	Summary      RequirementsSummary           `json:"summary"`
}

// RequirementsSummary provides summary statistics
type RequirementsSummary struct {
	TotalDocuments    int `json:"total_documents"`
	RequiredDocuments int `json:"required_documents"`
	OptionalDocuments int `json:"optional_documents"`
}

// Helper constants for fund types
const (
	FundTypePublicationReward = "publication_reward"
	FundTypeFundApplication   = "fund_application"
)

// ValidFundTypes returns a slice of valid fund types
func ValidFundTypes() []string {
	return []string{FundTypePublicationReward, FundTypeFundApplication}
}

// IsFundTypeValid checks if the given fund type is valid
func IsFundTypeValid(fundType string) bool {
	for _, validType := range ValidFundTypes() {
		if fundType == validType {
			return true
		}
	}
	return false
}
