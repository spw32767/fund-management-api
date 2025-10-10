package models

import (
	"time"
)

type User struct {
	UserID           int        `gorm:"primaryKey;column:user_id" json:"user_id"`
	UserFname        string     `gorm:"column:user_fname" json:"user_fname"`
	UserLname        string     `gorm:"column:user_lname" json:"user_lname"`
	Gender           string     `gorm:"column:gender" json:"gender"`
	Email            string     `gorm:"column:email;unique" json:"email"`
	ScholarAuthorID  *string    `gorm:"column:scholar_author_id" json:"scholar_author_id,omitempty"`
	Password         string     `gorm:"column:password" json:"-"`
	RoleID           int        `gorm:"column:role_id" json:"role_id"`
	PositionID       int        `gorm:"column:position_id" json:"position_id"`
	DateOfEmployment *time.Time `gorm:"column:date_of_employment" json:"date_of_employment,omitempty"`
	CreateAt         *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt         *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt         *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	Prefix           *string `gorm:"column:prefix" json:"prefix,omitempty"`
	ManagePosition   *string `gorm:"column:manage_position" json:"manage_position,omitempty"`
	PositionTitle    *string `gorm:"column:position" json:"position_title,omitempty"`
	PositionEn       *string `gorm:"column:position_en" json:"position_en,omitempty"`
	PrefixPositionEn *string `gorm:"column:prefix_position_en" json:"prefix_position_en,omitempty"`
	NameEn           *string `gorm:"column:Name_en" json:"name_en,omitempty"`
	SuffixEn         *string `gorm:"column:suffix_en" json:"suffix_en,omitempty"`
	Tel              *string `gorm:"column:TEL" json:"tel,omitempty"`
	TelFormat        *string `gorm:"column:TELformat" json:"tel_format,omitempty"`
	TelEng           *string `gorm:"column:TEL_ENG" json:"tel_eng,omitempty"`
	ManagePositionEn *string `gorm:"column:manage_position_en" json:"manage_position_en,omitempty"`
	LabName          *string `gorm:"column:LAB_Name" json:"lab_name,omitempty"`
	Room             *string `gorm:"column:Room" json:"room,omitempty"`
	CPWebID          *string `gorm:"column:CP_WEB_ID" json:"cp_web_id,omitempty"`
	ScopusID         *string `gorm:"column:Scopus_id" json:"scopus_id,omitempty"`
	AccountStatus    *string `gorm:"column:Is_active" json:"is_active,omitempty"`

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
