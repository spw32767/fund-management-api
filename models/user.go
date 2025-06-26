package models

import (
	"time"
)

type User struct {
	UserID     int        `gorm:"primaryKey;column:user_id" json:"user_id"`
	UserFname  string     `gorm:"column:user_fname" json:"user_fname"`
	UserLname  string     `gorm:"column:user_lname" json:"user_lname"`
	Gender     string     `gorm:"column:gender" json:"gender"`
	Email      string     `gorm:"column:email;unique" json:"email"`
	Password   string     `gorm:"column:password" json:"-"`
	RoleID     int        `gorm:"column:role_id" json:"role_id"`
	PositionID int        `gorm:"column:position_id" json:"position_id"`
	CreateAt   *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt   *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt   *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	// Relations
	Role     Role     `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	Position Position `gorm:"foreignKey:PositionID" json:"position,omitempty"`
}

type Role struct {
	RoleID   int        `gorm:"primaryKey;column:role_id" json:"role_id"`
	Role     string     `gorm:"column:role" json:"role"`
	CreateAt *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

type Position struct {
	PositionID   int        `gorm:"primaryKey;column:position_id" json:"position_id"`
	PositionName string     `gorm:"column:position_name" json:"position_name"`
	IsActive     string     `gorm:"column:is_active" json:"is_active"`
	CreateAt     *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt     *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt     *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

// TableName overrides
func (User) TableName() string {
	return "users"
}

func (Role) TableName() string {
	return "roles"
}

func (Position) TableName() string {
	return "positions"
}
