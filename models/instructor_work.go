package models
import (
    "time"  
    "gorm.io/gorm"
	"github.com/shopspring/decimal"
)

type InstructorTextbook struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int            `gorm:"column:user_id;not null" json:"user_id"`
	Title     string         `gorm:"column:title;type:text;not null" json:"title"`
	Year      int            `gorm:"column:year" json:"year"`
	Publisher string         `gorm:"column:publisher;type:varchar(255)" json:"publisher"`
	Edition   string         `gorm:"column:edition;type:varchar(50)" json:"edition"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type InstructorResearchProject struct {
	ID            uint            `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        int             `gorm:"column:user_id;not null" json:"user_id"`
	FiscalYear    string          `gorm:"column:fiscal_year;type:varchar(4)" json:"fiscal_year"`
	ProjectNameTh string          `gorm:"column:project_name_th;type:text;not null" json:"project_name_th"`
	ProjectNameEn string          `gorm:"column:project_name_en;type:text;not null" json:"project_name_en"`
	SourceOfFund  string          `gorm:"column:source_of_fund;type:varchar(255)" json:"source_of_fund"`
	Budget        decimal.Decimal `gorm:"column:budget;type:decimal(15,2)" json:"budget"` // ถ้าไม่ได้ใช้แพ็กเกจ decimal เปลี่ยนเป็น float64 ได้ครับ
	StartDate *time.Time `gorm:"column:start_date" json:"start_date"`
    EndDate   *time.Time `gorm:"column:end_date" json:"end_date"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	DeletedAt     gorm.DeletedAt  `gorm:"index" json:"deleted_at"`
}

func (InstructorTextbook) TableName() string {
    return "instructor_textbooks"
}

func (InstructorResearchProject) TableName() string {
    return "instructor_research_projects"
}
