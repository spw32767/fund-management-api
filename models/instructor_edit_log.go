package models

import (
    "time"  
)

type InstructorEditLog struct {
    ID           int       `json:"id" gorm:"column:id"`
    UserEditID   int       `json:"user_edit_id" gorm:"column:user_edit_id"`
    TargetUserID int       `json:"target_user_id" gorm:"column:target_user_id"`
    Action       string    `json:"action" gorm:"column:action"`
    TargetTable  string    `json:"table_name" gorm:"column:table_name"`  // เปลี่ยน field name เป็น TargetTable
    FieldName    string    `json:"field_name" gorm:"column:field_name"`
    RecordID     int       `json:"record_id" gorm:"column:record_id"`
    OldValue     string    `json:"old_value" gorm:"column:old_value"`
    NewValue     string    `json:"new_value" gorm:"column:new_value"`
    CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
    UserEdit     User      `json:"user_edit,omitempty" gorm:"foreignKey:UserEditID"`
}

func (InstructorEditLog) TableName() string {
    return "instructor_edit_logs"
}