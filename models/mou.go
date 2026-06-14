package models

import (
	"time"
)

// MouRecord represents a Memorandum of Understanding record
type MouRecord struct {
	ID              int         `gorm:"primaryKey;column:id" json:"id"`
	MouCode         string      `gorm:"column:mou_code;type:varchar(50)" json:"mou_code"`
	ParentMouID     *int        `gorm:"column:parent_mou_id" json:"parent_mou_id,omitempty"`
	Title           string      `gorm:"column:title" json:"title"`
	Description     string      `gorm:"column:description;type:text" json:"description"`
	StatusID        int         `gorm:"column:Status_id" json:"status_id"`
	Level           string      `gorm:"column:level;type:enum('university','faculty')" json:"level"`
	StartDate       *time.Time  `gorm:"column:start_date" json:"start_date"`
	EndDate         *time.Time  `gorm:"column:end_date" json:"end_date"`
	YearOfSigning   *time.Time  `gorm:"column:year_of_signing" json:"year_of_signing"`
	SignedBy        string      `gorm:"column:signed_by" json:"signed_by"`
	Notes           string      `gorm:"column:notes;type:text" json:"notes"`
	NotifyDaysBefore *int       `gorm:"column:notify_days_before" json:"notify_days_before"`
	CountryID       *int        `gorm:"column:Country_id" json:"country_id"`
	IsInternational bool        `gorm:"column:is_international" json:"is_international"`
	LockMou        bool        `gorm:"column:lock_mou;default:false" json:"lock_mou"`
	CoordinatorID   *int        `gorm:"column:coordinator_id" json:"coordinator_id"`
	CreatedBy       int         `gorm:"column:created_by" json:"created_by"`
	UpdatedBy       *int        `gorm:"column:updated_by" json:"updated_by,omitempty"`
	CreatedAt       time.Time   `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt       time.Time   `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
	DeletedAt       *time.Time  `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`

	// Relations
	Status        MouStatus        `gorm:"foreignKey:StatusID" json:"status,omitempty"`
	Notified      bool             `gorm:"-" json:"notified"`
	Country       *Country         `gorm:"foreignKey:CountryID" json:"country,omitempty"`
	Coordinator   User             `gorm:"foreignKey:CoordinatorID" json:"coordinator,omitempty"`
	Creator       User             `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Updater       *User            `gorm:"foreignKey:UpdatedBy" json:"updater,omitempty"`
	ParentMou     *MouRecord       `gorm:"foreignKey:ParentMouID" json:"parent_mou,omitempty"`
	Partners      []MouPartner     `gorm:"foreignKey:MouID" json:"partners,omitempty"`
	Faculties     []MouFaculty     `gorm:"foreignKey:MouID" json:"faculties,omitempty"`
	Notifications []MouNotification     `gorm:"foreignKey:MouID" json:"notifications,omitempty"`
	MouEvents     []MouNotificationLog  `gorm:"-" json:"mou_events,omitempty"`
	Activities    []MouActivity         `gorm:"foreignKey:MouID" json:"activities,omitempty"`
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
	Email         *string   `gorm:"column:email" json:"email"`
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
	StaffID    *int       `gorm:"column:staff_id" json:"staff_id,omitempty"`
	Email      *string    `gorm:"column:email" json:"email,omitempty"`
	DaysBefore int        `gorm:"-" json:"-"`
	IsSent     bool       `gorm:"column:is_sent;default:false" json:"is_sent"`
	SentAt     *time.Time `gorm:"column:sent_at" json:"sent_at"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`

	// Relations
	Mou   MouRecord `gorm:"foreignKey:MouID" json:"mou,omitempty"`
	Staff User      `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
}

func (MouNotification) TableName() string { return "mou_notification" }

// MouNotificationLog represents a log entry for a sent notification or MOU event
type MouNotificationLog struct {
	ID             int        `gorm:"primaryKey;column:id" json:"id"`
	NotificationID *int       `gorm:"column:notification_id" json:"notification_id,omitempty"`
	MouID          int        `gorm:"column:mou_id" json:"mou_id"`
	Action         string     `gorm:"column:action" json:"action"`
	ActorID        int        `gorm:"column:actor_id" json:"actor_id"`
	SentTo         int        `gorm:"column:sent_to" json:"sent_to"`
	Channel        string     `gorm:"column:channel" json:"channel"`
	Success        bool       `gorm:"column:success" json:"success"`
	Message        *string    `gorm:"column:message" json:"message"`
	SentAt         time.Time  `gorm:"column:sent_at;autoCreateTime:milli" json:"sent_at"`

	// Relations
	Actor User      `gorm:"foreignKey:ActorID" json:"actor,omitempty"`
	Mou   MouRecord `gorm:"foreignKey:MouID" json:"mou,omitempty"`
}

func (MouNotificationLog) TableName() string { return "mou_notification_log" }

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
	MouCode          string     `json:"mou_code"`
	ParentMouID      *int       `json:"parent_mou_id"`
	Title            string     `json:"title" binding:"required"`
	Description      string     `json:"description"`
	Level            string     `json:"level" binding:"required,oneof=university faculty"`
	IsInternational  bool       `json:"is_international"`
	StartDate        *string    `json:"start_date"` // format: "DD/MM/YYYY"
	EndDate          string     `json:"end_date" binding:"required"`
	YearOfSigning    string     `json:"year_of_signing"` // format: "2006-01-02"
	PartnerName      string     `json:"partner_name" binding:"required"`
	PartnerTypeID    int        `json:"partner_type_id"`
	CountryID        *int       `json:"country_id"`
	CoordinatorID    *int       `json:"coordinator_id"`
	CoordinatorName  string     `json:"coordinator_name"`
	SignedBy         *string    `json:"signed_by"`
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
	Email        *string `json:"email"`
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
	MouCode           *string       `json:"mou_code"`
	Title             *string       `json:"title"`
	Description       *string       `json:"description"`
	Level             *string       `json:"level"`
	StatusID          *int          `json:"status_id"`
	IsInternational   *bool         `json:"is_international"`
	StartDate         *string       `json:"start_date"`
	EndDate           *string       `json:"end_date"`
	YearOfSigning     *string       `json:"year_of_signing"`
	PartnerName       *string       `json:"partner_name"`
	PartnerTypeID     *int          `json:"partner_type_id"`
	CountryID         *int          `json:"country_id"`
	CoordinatorID     *int          `json:"coordinator_id"`
	CoordinatorName   *string       `json:"coordinator_name"`
	SignedBy          *string       `json:"signed_by"`
	Notes             *string       `json:"notes"`
	FacultyIDs           []int         `json:"faculty_ids"`
	Faculties            []FacultyUser `json:"faculties"`
	NotifyDaysBefore     *int          `json:"notify_days_before"`
	RemovedAttachmentIDs []int         `json:"removed_attachment_ids"`
}
