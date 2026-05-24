package models

import (
	"time"
)

// MouRecord represents a Memorandum of Understanding record
type MouRecord struct {
	ID              int         `gorm:"primaryKey;column:id" json:"id"`
	MouCode         string      `gorm:"column:mou_code;uniqueIndex" json:"mou_code"`
	Title           string      `gorm:"column:title" json:"title"`
	Description     string      `gorm:"column:description;type:text" json:"description"`
	StatusID        int         `gorm:"column:Status_id" json:"status_id"`
	MouTypeID       int         `gorm:"column:mou_type_id" json:"mou_type_id"`
	Level           string      `gorm:"column:level;type:enum('university','faculty')" json:"level"`
	StartDate       time.Time   `gorm:"column:start_date" json:"start_date"`
	EndDate         *time.Time  `gorm:"column:end_date" json:"end_date"`
	YearOfSigning   *int        `gorm:"column:year_of_signing" json:"year_of_signing"`
	SignedBy        *int        `gorm:"column:signed_by" json:"signed_by"`
	Notes           string      `gorm:"column:notes;type:text" json:"notes"`
	NotifyDaysBefore *int       `gorm:"column:notify_days_before" json:"notify_days_before"`
	CountryID       *int        `gorm:"column:country_id" json:"country_id"`
	IsInternational bool        `gorm:"column:is_international" json:"is_international"`
	CoordinatorID   *int        `gorm:"column:coordinator_id" json:"coordinator_id"`
	CreatedBy       int         `gorm:"column:created_by" json:"created_by"`
	UpdatedBy       *int        `gorm:"column:updated_by" json:"updated_by,omitempty"`
	CreatedAt       time.Time   `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt       time.Time   `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
	DeletedAt       *time.Time  `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`

	// Relations
	Status        MouStatus        `gorm:"foreignKey:StatusID" json:"status,omitempty"`
	MouType       MouType          `gorm:"foreignKey:MouTypeID" json:"mou_type,omitempty"`
	Country       Country          `gorm:"foreignKey:CountryID" json:"country,omitempty"`
	Coordinator   User             `gorm:"foreignKey:CoordinatorID" json:"coordinator,omitempty"`
	SignedByUser  User             `gorm:"foreignKey:SignedBy" json:"signed_by_user,omitempty"`
	Creator       User             `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Updater       *User            `gorm:"foreignKey:UpdatedBy" json:"updater,omitempty"`
	Partners      []MouPartner     `gorm:"foreignKey:MouID" json:"partners,omitempty"`
	Faculties     []MouFaculty     `gorm:"foreignKey:MouID" json:"faculties,omitempty"`
	Notifications []MouNotification `gorm:"foreignKey:MouID" json:"notifications,omitempty"`
	Activities    []MouActivity    `gorm:"foreignKey:MouID" json:"activities,omitempty"`
	Attachments   []MouAttachment  `gorm:"foreignKey:MouID" json:"attachments,omitempty"`
}

// MouStatus represents the status of an MOU
type MouStatus struct {
	ID        int       `gorm:"primaryKey;column:id" json:"id"`
	Name      string    `gorm:"column:name" json:"name"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

func (MouStatus) TableName() string { return "mou_status" }

// MouType represents the type of MOU
type MouType struct {
	ID        int       `gorm:"primaryKey;column:id" json:"id"`
	Name      string    `gorm:"column:name" json:"name"`
	IsActive  bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

func (MouType) TableName() string { return "mou_type" }

// Country represents a country
type Country struct {
	ID        int       `gorm:"primaryKey;column:id" json:"id"`
	NameTh    string    `gorm:"column:name_th" json:"name_th"`
	NameEn    string    `gorm:"column:name_en" json:"name_en"`
	IsActive  bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

// Faculty represents a faculty or department
type Faculty struct {
	ID        int       `gorm:"primaryKey;column:id" json:"id"`
	NameTh    string    `gorm:"column:name_th" json:"name_th"`
	NameEn    string    `gorm:"column:name_en" json:"name_en"`
	IsActive  bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

// MouPartner represents a partner organization for an MOU
type MouPartner struct {
	ID            int       `gorm:"primaryKey;column:id" json:"id"`
	MouID         int       `gorm:"column:mou_id" json:"mou_id"`
	PartnerOrg    string    `gorm:"column:partner_org" json:"partner_org"`
	PartnerTypeID int       `gorm:"column:partner_type_id" json:"partner_type_id"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`

	// Relations
	Mou         MouRecord       `gorm:"foreignKey:MouID" json:"mou,omitempty"`
	PartnerType MouPartnerType  `gorm:"foreignKey:PartnerTypeID" json:"partner_type,omitempty"`
}

func (MouPartner) TableName() string { return "mou_partner" }

// MouPartnerType represents a partner type lookup
type MouPartnerType struct {
	ID          int        `gorm:"primaryKey;column:id" json:"id"`
	NameTh      string     `gorm:"column:name_th" json:"name_th"`
	Description *string    `gorm:"column:description" json:"description"`
	IsActive    bool       `gorm:"column:is_active;default:true" json:"is_active"`
	DeletedAt   *time.Time `gorm:"column:deleted_at;index" json:"deleted_at"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
}

func (MouPartnerType) TableName() string { return "mou_partner_type" }

// MouFaculty represents faculty mapping for an MOU
type MouFaculty struct {
	ID            int       `gorm:"primaryKey;column:id" json:"id"`
	MouID         int       `gorm:"column:mou_id" json:"mou_id"`
	UserID        *int      `gorm:"column:user_id" json:"user_id"`
	CpEmployeeID  *int      `gorm:"column:cp_employee_id" json:"cp_employee_id"`
	FacultyID     *int      `gorm:"column:faculty_id" json:"faculty_id"`
	ExternalName  *string   `gorm:"column:external_name" json:"external_name"`
	ExternalOrg   *string   `gorm:"column:external_org" json:"external_org"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`

	// Relations
	Mou     MouRecord `gorm:"foreignKey:MouID" json:"mou,omitempty"`
	Faculty *Faculty  `gorm:"foreignKey:FacultyID" json:"faculty,omitempty"`
	User    *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (MouFaculty) TableName() string { return "mou_faculty" }

// MouNotification represents notification settings for an MOU
type MouNotification struct {
	ID         int        `gorm:"primaryKey;column:id" json:"id"`
	MouID      int        `gorm:"column:mou_id" json:"mou_id"`
	StaffID    int        `gorm:"column:staff_id" json:"staff_id"`
	DaysBefore int        `gorm:"column:days_before" json:"days_before"`
	IsSent     bool       `gorm:"column:is_sent;default:false" json:"is_sent"`
	SentAt     *time.Time `gorm:"column:sent_at" json:"sent_at"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`

	// Relations
	Mou   MouRecord `gorm:"foreignKey:MouID" json:"mou,omitempty"`
	Staff User      `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
}

func (MouNotification) TableName() string { return "mou_notification" }

// MouOKR represents OKRs associated with an MOU
type MouOKR struct {
	ID          int       `gorm:"primaryKey;column:id" json:"id"`
	Title       string    `gorm:"column:title" json:"title"`
	Description string    `gorm:"column:description;type:text" json:"description"`
	Category    string    `gorm:"column:category" json:"category"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

// MouActivity represents an activity under an MOU
type MouActivity struct {
	ID              int        `gorm:"primaryKey;column:id" json:"id"`
	MouID           int        `gorm:"column:mou_id" json:"mou_id"`
	ActivityTypeID  *int       `gorm:"column:activity_type_id" json:"activity_type_id,omitempty"`
	Title           string     `gorm:"column:title" json:"title"`
	Objective       string     `gorm:"column:objective;type:text" json:"objective"`
	Description     string     `gorm:"column:description;type:text" json:"description"`
	Notes           string     `gorm:"column:notes;type:text" json:"notes"`
	ParticipantCount int       `gorm:"column:participant_count;default:0" json:"participant_count"`
	ActivityStart   time.Time  `gorm:"column:activity_start" json:"activity_start"`
	ActivityEnd     time.Time  `gorm:"column:activity_end" json:"activity_end"`
	Location        string     `gorm:"column:location" json:"location"`
	Plan            string     `gorm:"column:plan;type:text" json:"plan"`
	CoordinatorID   *int       `gorm:"column:coordinator_id" json:"coordinator_id"`
	CoordinatorOther string    `gorm:"column:coordinator_other" json:"coordinator_other"`
	CoordinatorOrg  string     `gorm:"column:coordinator_org" json:"coordinator_org"`
	CreatedBy       int        `gorm:"column:created_by" json:"created_by"`
	UpdatedBy       *int       `gorm:"column:updated_by" json:"updated_by"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
	DeletedAt       *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`

	// Relations
	Mou             MouRecord           `gorm:"foreignKey:MouID" json:"mou,omitempty"`
	ActivityType    *MouActivityType    `gorm:"foreignKey:ActivityTypeID" json:"activity_type,omitempty"`
	ActivityTypes   []MouActivityType   `gorm:"many2many:mou_activity_activity_type;foreignKey:ID;joinForeignKey:activity_id;References:ID;joinReferences:activity_type_id" json:"activity_types,omitempty"`
	Coordinator     User                `gorm:"foreignKey:CoordinatorID" json:"coordinator,omitempty"`
	Creator         User                `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Updater         User                `gorm:"foreignKey:UpdatedBy" json:"updater,omitempty"`
	Okrs            []MouOKR            `gorm:"many2many:mou_activity_okr;foreignKey:ID;joinForeignKey:activity_id;References:ID;joinReferences:okr_id" json:"okrs,omitempty"`
	Attachments     []MouActivityAttachment `gorm:"foreignKey:ActivityID" json:"attachments,omitempty"`
}

// MouActivityType represents types of activities
type MouActivityType struct {
	ID          int       `gorm:"primaryKey;column:id" json:"id"`
	Name        string    `gorm:"column:name" json:"name"`
	Description string    `gorm:"column:description;type:text" json:"description"`
	IsActive    bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

// MouAttachment represents attachments for an MOU
type MouAttachment struct {
	ID        int       `gorm:"primaryKey;column:id" json:"id"`
	MouID     int       `gorm:"column:mou_id" json:"mou_id"`
	FileName  string    `gorm:"column:file_name" json:"file_name"`
	FilePath  string    `gorm:"column:file_path" json:"file_path"`
	MimeType  string    `gorm:"column:mime_type" json:"mime_type"`
	UploadedBy int      `gorm:"column:uploaded_by" json:"uploaded_by"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`

	// Relations
	Mou    MouRecord `gorm:"foreignKey:MouID" json:"mou,omitempty"`
	Uploader User     `gorm:"foreignKey:UploadedBy" json:"uploader,omitempty"`
}

// MouActivityAttachment represents attachments for activities
type MouActivityAttachment struct {
	ID        int       `gorm:"primaryKey;column:id" json:"id"`
	ActivityID int      `gorm:"column:activity_id" json:"activity_id"`
	FileName  string    `gorm:"column:file_name" json:"file_name"`
	FilePath  string    `gorm:"column:file_path" json:"file_path"`
	MimeType  string    `gorm:"column:mime_type" json:"mime_type"`
	UploadedBy int      `gorm:"column:uploaded_by" json:"uploaded_by"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`

	// Relations
	Activity  MouActivity `gorm:"foreignKey:ActivityID" json:"activity,omitempty"`
	Uploader  User        `gorm:"foreignKey:UploadedBy" json:"uploader,omitempty"`
}

// CreateMouRequest is the request body for creating an MOU
type CreateMouRequest struct {
	MouCode          string     `json:"mou_code" binding:"required"`
	Title            string     `json:"title" binding:"required"`
	Description      string     `json:"description"`
	Level            string     `json:"level" binding:"required,oneof=university faculty"`
	MouTypeID        int        `json:"mou_type_id" binding:"required"`
	IsInternational  bool       `json:"is_international"`
	StartDate        string     `json:"start_date" binding:"required"` // format: "02/01/2025"
	EndDate          string     `json:"end_date" binding:"required"`
	YearOfSigning    int        `json:"year_of_signing"` // พ.ศ. หรือ ค.ศ. (ตัวเลข 4 หลัก)
	PartnerName      string     `json:"partner_name" binding:"required"`
	PartnerTypeID    int        `json:"partner_type_id"`
	CountryID        *int       `json:"country_id"`
	CoordinatorID    *int       `json:"coordinator_id"`
	CoordinatorName  string     `json:"coordinator_name"`
	SignedBy         *int       `json:"signed_by"`
	Notes            string     `json:"notes"`
	FacultyIDs       []int         `json:"faculty_ids"`
	Faculties        []FacultyUser `json:"faculties"`
	NotifyDaysBefore *int           `json:"notify_days_before"`
	StatusID         *int           `json:"status_id"`
}

// FacultyUser represents faculty with assigned responsible person
type FacultyUser struct {
	FacultyID    int     `json:"faculty_id"`
	UserID       int     `json:"user_id"`
	ExternalName *string `json:"external_name"`
	ExternalOrg  *string `json:"external_org"`
}

// CreateMouActivityRequest is the request body for creating an activity
type CreateMouActivityRequest struct {
	MouID            int      `json:"mou_id" binding:"required"`
	Title            string   `json:"title" binding:"required"`
	ActivityTypeIDs  []int    `json:"activity_type_ids" binding:"required,min=1"`
	ActivityStart    string   `json:"activity_start" binding:"required"`
	ActivityEnd      string   `json:"activity_end" binding:"required"`
	Location         string   `json:"location" binding:"required"`
	ParticipantCount int      `json:"participant_count"`
	OKRIDs           []int    `json:"okr_ids"`
	Objective        string   `json:"objective"`
	Description      string   `json:"description"`
	Plan             string   `json:"plan"`
	Notes            string   `json:"notes"`
	CoordinatorID    *int     `json:"coordinator_id"`
	CoordinatorOther string   `json:"coordinator_other"`
	CoordinatorOrg   string   `json:"coordinator_org"`
}

type UpdateMouActivityRequest struct {
	Title            string   `json:"title"`
	ActivityTypeIDs  []int    `json:"activity_type_ids"`
	ActivityStart    string   `json:"activity_start"`
	ActivityEnd      string   `json:"activity_end"`
	Location         string   `json:"location"`
	ParticipantCount int      `json:"participant_count"`
	OKRIDs           []int    `json:"okr_ids"`
	Objective        string   `json:"objective"`
	Description      string   `json:"description"`
	Plan             string   `json:"plan"`
	Notes            string   `json:"notes"`
	CoordinatorID    *int     `json:"coordinator_id"`
	CoordinatorOther string   `json:"coordinator_other"`
	CoordinatorOrg   string   `json:"coordinator_org"`
}

func (MouActivity) TableName() string { return "mou_activity" }

func (MouAttachment) TableName() string { return "mou_attachment" }

func (MouActivityAttachment) TableName() string { return "mou_activity_attachment" }

func (MouActivityType) TableName() string { return "mou_activity_type" }

func (MouOKR) TableName() string { return "mou_okr" }

// UpdateMouRequest is the request body for updating an MOU
type UpdateMouRequest struct {
	Title             *string       `json:"title"`
	Description       *string       `json:"description"`
	MouCode           *string       `json:"mou_code"`
	Level             *string       `json:"level"`
	MouTypeID         *int          `json:"mou_type_id"`
	StatusID          *int          `json:"status_id"`
	IsInternational   *bool         `json:"is_international"`
	StartDate         *string       `json:"start_date"`
	EndDate           *string       `json:"end_date"`
	YearOfSigning     *int          `json:"year_of_signing"`
	PartnerName       *string       `json:"partner_name"`
	PartnerTypeID     *int          `json:"partner_type_id"`
	CountryID         *int          `json:"country_id"`
	CoordinatorID     *int          `json:"coordinator_id"`
	CoordinatorName   *string       `json:"coordinator_name"`
	SignedBy          *int          `json:"signed_by"`
	Notes             *string       `json:"notes"`
	FacultyIDs           []int         `json:"faculty_ids"`
	Faculties            []FacultyUser `json:"faculties"`
	NotifyDaysBefore     *int          `json:"notify_days_before"`
	RemovedAttachmentIDs []int         `json:"removed_attachment_ids"`
}
