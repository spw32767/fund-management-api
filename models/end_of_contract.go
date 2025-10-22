package models

// EndOfContract represents a single reward agreement/condition row.
type EndOfContract struct {
	EocID        int    `gorm:"column:eoc_id;primaryKey" json:"eoc_id"`
	Content      string `gorm:"column:content" json:"content"`
	DisplayOrder int    `gorm:"column:display_order" json:"display_order"`
}

// TableName specifies the exact database table name.
func (EndOfContract) TableName() string {
	return "end_of_contract"
}
