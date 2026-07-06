package services

import (
	"context"
	"encoding/json"
	"fmt"
	"fund-management-api/models"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ─── Diff item types ─────────────────────────────────────────────────────────

type DeleteDiffItem struct {
	ID        uint
	UpdatedAt time.Time
}

type ExpertiseDiffItem struct {
	ID        uint
	UpdatedAt time.Time
	UserID    int
	Expertise string
}

type EducationDiffItem struct {
	models.InstructorEducationTab
	UpdatedAt time.Time
}

type ProjectDiffItem struct {
	models.InstructorResearchProject
	UpdatedAt time.Time
}

type TextbookDiffItem struct {
	models.InstructorTextbook
	UpdatedAt time.Time
}

type PropertyDiffItem struct {
	models.InstructorIntellectualProperty
	UpdatedAt time.Time
	Type      string //ADDED: เพิ่มฟิลด์ Type เพื่อให้ด้านล่างเรียก u.Type ได้ไม่พัง
}

// ─── Interface ───────────────────────────────────────────────────────────────

type InstructorService interface {
	GetFullProfile(ctx context.Context, userID int) (*models.InstructorFullProfile, error)
	UpdateInstructorProfile(ctx context.Context, user models.InstructorFullProfile) error
	GetInstructorList(ctx context.Context) ([]models.InstructorProfileHeader, error)
	UpdateInstructorByAdmin(
		ctx context.Context,
		editorID int,
		targetID int,
		headerChanges map[string]interface{},
		educationsAdded []models.InstructorEducationTab, educationsUpdated []EducationDiffItem, educationsDeleted []DeleteDiffItem,
		expertisesAdded []models.InstructorExpertiseTab, expertisesUpdated []ExpertiseDiffItem, expertisesDeleted []DeleteDiffItem,
		projectsAdded []models.InstructorResearchProject, projectsUpdated []ProjectDiffItem, projectsDeleted []DeleteDiffItem,
		textbooksAdded []models.InstructorTextbook, textbooksUpdated []TextbookDiffItem, textbooksDeleted []DeleteDiffItem,
		propertiesAdded []models.InstructorIntellectualProperty, propertiesUpdated []PropertyDiffItem, propertiesDeleted []DeleteDiffItem,
		courses []models.InstructorCourseResponsibility,
	) error

	// ➕ ADDED: ประกาศ Method เพิ่มใน Interface เพื่อให้ Controller เรียกใช้ได้
	DeleteTextbook(ctx context.Context, editorID int, id uint) error
	DeleteIntellectualProperty(ctx context.Context, editorID int, id uint) error
	DeleteResearchProject(ctx context.Context, editorID int, id uint) error
	DeleteExpertise(ctx context.Context, editorID int, id uint) error
	DeleteEducation(ctx context.Context, editorID int, id uint) error
}

type instructorService struct{ db *gorm.DB }

func NewInstructorService(db *gorm.DB) InstructorService {
	return &instructorService{db: db}
}

// ─── GetFullProfile ───────────────────────────────────────────────────────────

func (s *instructorService) GetFullProfile(ctx context.Context, userID int) (*models.InstructorFullProfile, error) {
	var fp models.InstructorFullProfile
	if err := s.db.WithContext(ctx).Table("users").Where("user_id = ?", userID).Take(&fp.Header).Error; err != nil {
		return nil, err
	}
	errs := []error{
		s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&fp.Educations).Error,
		s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&fp.Expertises).Error,
		s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&fp.InstructorTextbooks).Error,
		s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&fp.InstructorResearchProjects).Error,
		s.db.WithContext(ctx).Preload("Course").Where("user_id = ?", userID).Find(&fp.InstructorCourseResponsibility).Error,
		s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&fp.InstructorIntellectualProperties).Error,
	}
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	var weights []models.RankingTierWeight
	if err := s.db.WithContext(ctx).Where("is_active = ?", true).Find(&weights).Error; err == nil {
		weightMap := make(map[string]models.RankingTierWeight)
		for _, w := range weights {
			weightMap[w.TierCode] = w
		}
		for i := range fp.InstructorIntellectualProperties {
			pType := fp.InstructorIntellectualProperties[i].Type
			if tierData, exists := weightMap[pType]; exists {
				matchedTier := tierData
				fp.InstructorIntellectualProperties[i].TierDetails = &matchedTier
			} else {
				fp.InstructorIntellectualProperties[i].TierDetails = nil
			}
		}
	}
	return &fp, nil
}

// ─── UpdateInstructorProfile (self-edit) ─────────────────────────────────────

func (s *instructorService) UpdateInstructorProfile(ctx context.Context, user models.InstructorFullProfile) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var oldHeader models.InstructorProfileHeader
		if err := tx.Table("users").Where("user_id = ?", user.Header.UserID).First(&oldHeader).Error; err != nil {
			return err
		}
		updateData := map[string]interface{}{
			"prefix": user.Header.InstructorPrefix, "user_fname": user.Header.ThaiFirstName,
			"user_lname": user.Header.ThaiLastName, "position": user.Header.Position,
			"email": user.Header.Email, "tel": user.Header.Tel,
			"scopus_id": user.Header.LinkScopus, "scholar_author_id": user.Header.LinkGoogleScholar,
			"thaijo_author_id": user.Header.LinkThaiJo, "date_of_employment": user.Header.DateOfEmployment,
		}
		if err := tx.Table("users").Where("user_id = ?", user.Header.UserID).Updates(updateData).Error; err != nil {
			return err
		}
		for _, fn := range []func() error{
			func() error {
				if err := tx.Where("user_id = ?", user.Header.UserID).Delete(&models.InstructorEducationTab{}).Error; err != nil {
					return err
				}
				if len(user.Educations) > 0 {
					for i := range user.Educations {
						user.Educations[i].UserID = user.Header.UserID
					}
					return tx.Create(&user.Educations).Error
				}
				return nil
			},
			func() error {
				if err := tx.Where("user_id = ?", user.Header.UserID).Delete(&models.InstructorExpertiseTab{}).Error; err != nil {
					return err
				}
				if len(user.Expertises) > 0 {
					for i := range user.Expertises {
						user.Expertises[i].UserID = user.Header.UserID
					}
					return tx.Create(&user.Expertises).Error
				}
				return nil
			},
		} {
			if err := fn(); err != nil {
				return err
			}
		}
		oldJSON, _ := json.Marshal(oldHeader)
		newJSON, _ := json.Marshal(user.Header)
		return tx.Create(&models.InstructorEditLog{
			UserEditID: user.Header.UserID, TargetUserID: user.Header.UserID,
			Action: "UPDATE", TargetTable: "users", RecordID: user.Header.UserID,
			OldValue: string(oldJSON), NewValue: string(newJSON),
		}).Error
	})
}

// ─── GetInstructorList ────────────────────────────────────────────────────────

func (s *instructorService) GetInstructorList(ctx context.Context) ([]models.InstructorProfileHeader, error) {
	var list []models.InstructorProfileHeader
	err := s.db.WithContext(ctx).Model(&models.InstructorProfileHeader{}).
		Preload("InstructorCourseResponsibility").
		Preload("InstructorCourseResponsibility.Course").
		Where("role_id = ?", 1).Find(&list).Error
	return list, err
}

// ─── UpdateInstructorByAdmin ──────────────────────────────────────────────────

func (s *instructorService) UpdateInstructorByAdmin(
	ctx context.Context,
	editorID, targetID int,
	headerChanges map[string]interface{},
	educationsAdded []models.InstructorEducationTab, educationsUpdated []EducationDiffItem, educationsDeleted []DeleteDiffItem,
	expertisesAdded []models.InstructorExpertiseTab, expertisesUpdated []ExpertiseDiffItem, expertisesDeleted []DeleteDiffItem,
	projectsAdded []models.InstructorResearchProject, projectsUpdated []ProjectDiffItem, projectsDeleted []DeleteDiffItem,
	textbooksAdded []models.InstructorTextbook, textbooksUpdated []TextbookDiffItem, textbooksDeleted []DeleteDiffItem,
	propertiesAdded []models.InstructorIntellectualProperty, propertiesUpdated []PropertyDiffItem, propertiesDeleted []DeleteDiffItem,
	courses []models.InstructorCourseResponsibility,
) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// 1. Header — log แยกรายฟิลด์ที่เปลี่ยน
		if len(headerChanges) > 0 {
			var oldUser map[string]interface{}
			if err := tx.Table("users").Where("user_id = ?", targetID).Take(&oldUser).Error; err != nil {
				return err
			}
			delete(headerChanges, "user_id")
			if err := tx.Table("users").Where("user_id = ?", targetID).Updates(headerChanges).Error; err != nil {
				return err
			}
			if err := logFieldChanges(tx, editorID, targetID, "users", targetID, oldUser, headerChanges); err != nil {
				return err
			}
		}

		// 2. Educations diff
		if err := applyEducationDiff(tx, editorID, targetID,
			educationsAdded, educationsUpdated, educationsDeleted,
		); err != nil {
			return err
		}

		// 3. Expertises diff
		if err := applyExpertiseDiff(tx, editorID, targetID,
			expertisesAdded, expertisesUpdated, expertisesDeleted,
		); err != nil {
			return err
		}

		// 4. Research Projects diff
		if err := applyProjectDiff(tx, editorID, targetID,
			projectsAdded, projectsUpdated, projectsDeleted,
		); err != nil {
			return err
		}

		// 5. Textbooks diff
		if err := applyTextbookDiff(tx, editorID, targetID,
			textbooksAdded, textbooksUpdated, textbooksDeleted,
		); err != nil {
			return err
		}

		// 6. Intellectual Properties diff
		if err := applyPropertyDiff(tx, editorID, targetID,
			propertiesAdded, propertiesUpdated, propertiesDeleted,
		); err != nil {
			return err
		}

		// 7. Courses (replace — log เป็น snapshot เพราะมัน replace ทั้งชุด)
		if err := applyCoursesReplace(tx, editorID, targetID, courses); err != nil {
			return err
		}

		return nil
	})
}

//ADDED: Implement Single Delete Methods สำหรับใช้งานเดี่ยวๆ 

func (s *instructorService) DeleteTextbook(ctx context.Context, editorID int, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old models.InstructorTextbook
		if err := tx.First(&old, id).Error; err != nil {
			return fmt.Errorf("ไม่พบตำรา ID %d", id)
		}
		if err := tx.Delete(&models.InstructorTextbook{}, id).Error; err != nil {
			return err
		}
		oldJSON, _ := json.Marshal(old)
		return tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: old.UserID,
			Action: "DELETE", TargetTable: "instructor_textbooks",
			FieldName: "textbook_item", RecordID: int(id),
			OldValue: string(oldJSON), NewValue: "",
		}).Error
	})
}

func (s *instructorService) DeleteIntellectualProperty(ctx context.Context, editorID int, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old models.InstructorIntellectualProperty
		if err := tx.First(&old, id).Error; err != nil {
			return fmt.Errorf("ไม่พบทรัพย์สินทางปัญญา ID %d", id)
		}
		if err := tx.Delete(&models.InstructorIntellectualProperty{}, id).Error; err != nil {
			return err
		}
		oldJSON, _ := json.Marshal(old)
		return tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: old.UserID,
			Action: "DELETE", TargetTable: "instructor_intellectual_properties",
			FieldName: "property_item", RecordID: int(id),
			OldValue: string(oldJSON), NewValue: "",
		}).Error
	})
}

func (s *instructorService) DeleteResearchProject(ctx context.Context, editorID int, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old models.InstructorResearchProject
		if err := tx.First(&old, id).Error; err != nil {
			return fmt.Errorf("ไม่พบโครงการวิจัย ID %d", id)
		}
		if err := tx.Delete(&models.InstructorResearchProject{}, id).Error; err != nil {
			return err
		}
		oldJSON, _ := json.Marshal(old)
		return tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: old.UserID,
			Action: "DELETE", TargetTable: "instructor_research_projects",
			FieldName: "project_item", RecordID: int(id),
			OldValue: string(oldJSON), NewValue: "",
		}).Error
	})
}

func (s *instructorService) DeleteExpertise(ctx context.Context, editorID int, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old models.InstructorExpertiseTab
		if err := tx.First(&old, id).Error; err != nil {
			return fmt.Errorf("ไม่พบข้อมูลความเชี่ยวชาญ ID %d", id)
		}
		if err := tx.Delete(&models.InstructorExpertiseTab{}, id).Error; err != nil {
			return err
		}
		oldJSON, _ := json.Marshal(old)
		return tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: old.UserID,
			Action: "DELETE", TargetTable: "instructor_expertises",
			FieldName: "expertise_item", RecordID: int(id),
			OldValue: string(oldJSON), NewValue: "",
		}).Error
	})
}

func (s *instructorService) DeleteEducation(ctx context.Context, editorID int, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old models.InstructorEducationTab
		if err := tx.First(&old, id).Error; err != nil {
			return fmt.Errorf("ไม่พบประวัติการศึกษา ID %d", id)
		}
		if err := tx.Delete(&models.InstructorEducationTab{}, id).Error; err != nil {
			return err
		}
		oldJSON, _ := json.Marshal(old)
		return tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: old.UserID,
			Action: "DELETE", TargetTable: "instructor_educations",
			FieldName: "education_item", RecordID: int(id),
			OldValue: string(oldJSON), NewValue: "",
		}).Error
	})
}

//Diff helpers 
func applyEducationDiff(
	tx *gorm.DB, editorID, targetID int,
	added []models.InstructorEducationTab,
	updated []EducationDiffItem,
	deleted []DeleteDiffItem,
) error {
	for _, d := range deleted {
		var old models.InstructorEducationTab
		if err := tx.First(&old, d.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบประวัติการศึกษา ID %d", d.ID)
		}
		r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).
			Delete(&models.InstructorEducationTab{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("ประวัติการศึกษา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID)
		}
		oldJSON, _ := json.Marshal(old)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "DELETE", TargetTable: "instructor_educations",
			FieldName: "education_item", RecordID: int(d.ID),
			OldValue: string(oldJSON), NewValue: "",
		}).Error; err != nil {
			return err
		}
	}

	for _, u := range updated {
		var old models.InstructorEducationTab
		if err := tx.First(&old, u.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบประวัติการศึกษา ID %d", u.ID)
		}
		r := tx.Model(&models.InstructorEducationTab{}).
			Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
			Updates(map[string]interface{}{
				"degree_id": u.DegreeID, "degree_title_th": u.DegreeTitleTh,
				"university_th": u.UniversityTh, "country": u.Country, "grad_year": u.GradYear,
			})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("ประวัติการศึกษา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID)
		}
		var newVal models.InstructorEducationTab
		tx.First(&newVal, u.ID)
		oldJSON, _ := json.Marshal(old)
		newJSON, _ := json.Marshal(newVal)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "UPDATE", TargetTable: "instructor_educations",
			FieldName: "education_item", RecordID: int(u.ID),
			OldValue: string(oldJSON), NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}

	for i := range added {
		added[i].UserID = targetID
		added[i].ID = 0
		if err := tx.Create(&added[i]).Error; err != nil {
			return err
		}
		newJSON, _ := json.Marshal(added[i])
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "INSERT", TargetTable: "instructor_educations",
			FieldName: "education_item", RecordID: int(added[i].ID),
			OldValue: "", NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyExpertiseDiff(
	tx *gorm.DB, editorID, targetID int,
	added []models.InstructorExpertiseTab,
	updated []ExpertiseDiffItem,
	deleted []DeleteDiffItem,
) error {
	for _, d := range deleted {
		var old models.InstructorExpertiseTab
		if err := tx.First(&old, d.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบข้อมูลความเชี่ยวชาญ ID %d", d.ID)
		}
		r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).
			Delete(&models.InstructorExpertiseTab{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("ข้อมูลความเชี่ยวชาญ ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID)
		}
		oldJSON, _ := json.Marshal(old)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "DELETE", TargetTable: "instructor_expertises",
			FieldName: "expertise_item", RecordID: int(d.ID),
			OldValue: string(oldJSON), NewValue: "",
		}).Error; err != nil {
			return err
		}
	}

	for _, u := range updated {
		var old models.InstructorExpertiseTab
		if err := tx.First(&old, u.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบข้อมูลความเชี่ยวชาญ ID %d", u.ID)
		}
		r := tx.Model(&models.InstructorExpertiseTab{}).
			Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
			Updates(map[string]interface{}{"expertise": u.Expertise})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("ข้อมูลความเชี่ยวชาญ ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID)
		}
		var newVal models.InstructorExpertiseTab
		tx.First(&newVal, u.ID)
		oldJSON, _ := json.Marshal(old)
		newJSON, _ := json.Marshal(newVal)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "UPDATE", TargetTable: "instructor_expertises",
			FieldName: "expertise_item", RecordID: int(u.ID),
			OldValue: string(oldJSON), NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}

	for i := range added {
		added[i].UserID = targetID
		added[i].ID = 0
		if err := tx.Create(&added[i]).Error; err != nil {
			return err
		}
		newJSON, _ := json.Marshal(added[i])
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "INSERT", TargetTable: "instructor_expertises",
			FieldName: "expertise_item", RecordID: int(added[i].ID),
			OldValue: "", NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyProjectDiff(
	tx *gorm.DB, editorID, targetID int,
	added []models.InstructorResearchProject,
	updated []ProjectDiffItem,
	deleted []DeleteDiffItem,
) error {
	for _, d := range deleted {
		var old models.InstructorResearchProject
		if err := tx.First(&old, d.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบโครงการวิจัย ID %d", d.ID)
		}
		r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).
			Delete(&models.InstructorResearchProject{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("โครงการวิจัย ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID)
		}
		oldJSON, _ := json.Marshal(old)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "DELETE", TargetTable: "instructor_research_projects",
			FieldName: "project_item", RecordID: int(d.ID),
			OldValue: string(oldJSON), NewValue: "",
		}).Error; err != nil {
			return err
		}
	}

	for _, u := range updated {
		var old models.InstructorResearchProject
		if err := tx.First(&old, u.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบโครงการวิจัย ID %d", u.ID)
		}
		r := tx.Model(&models.InstructorResearchProject{}).
			Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
			Updates(map[string]interface{}{
				"fiscal_year": u.FiscalYear, "project_name_th": u.ProjectNameTh,
				"project_name_en": u.ProjectNameEn, "source_of_fund": u.SourceOfFund,
				"start_date": u.StartDate, "end_date": u.EndDate, "budget": u.Budget,
			})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("โครงการวิจัย ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID)
		}
		var newVal models.InstructorResearchProject
		tx.First(&newVal, u.ID)
		oldJSON, _ := json.Marshal(old)
		newJSON, _ := json.Marshal(newVal)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "UPDATE", TargetTable: "instructor_research_projects",
			FieldName: "project_item", RecordID: int(u.ID),
			OldValue: string(oldJSON), NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}

	for i := range added {
		added[i].UserID = targetID
		added[i].ID = 0
		if err := tx.Create(&added[i]).Error; err != nil {
			return err
		}
		newJSON, _ := json.Marshal(added[i])
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "INSERT", TargetTable: "instructor_research_projects",
			FieldName: "project_item", RecordID: int(added[i].ID),
			OldValue: "", NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyTextbookDiff(
	tx *gorm.DB, editorID, targetID int,
	added []models.InstructorTextbook,
	updated []TextbookDiffItem,
	deleted []DeleteDiffItem,
) error {
	for _, d := range deleted {
		var old models.InstructorTextbook
		if err := tx.First(&old, d.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบตำรา ID %d", d.ID)
		}
		r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).
			Delete(&models.InstructorTextbook{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("ตำรา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID)
		}
		oldJSON, _ := json.Marshal(old)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "DELETE", TargetTable: "instructor_textbooks",
			FieldName: "textbook_item", RecordID: int(d.ID),
			OldValue: string(oldJSON), NewValue: "",
		}).Error; err != nil {
			return err
		}
	}

	for _, u := range updated {
		var old models.InstructorTextbook
		if err := tx.First(&old, u.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบตำรา ID %d", u.ID)
		}
		r := tx.Model(&models.InstructorTextbook{}).
			Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
			Updates(map[string]interface{}{
				"title": u.Title, "year": u.Year,
				"publisher": u.Publisher, "edition": u.Edition,
			})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("ตำรา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID)
		}
		var newVal models.InstructorTextbook
		tx.First(&newVal, u.ID)
		oldJSON, _ := json.Marshal(old)
		newJSON, _ := json.Marshal(newVal)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "UPDATE", TargetTable: "instructor_textbooks",
			FieldName: "textbook_item", RecordID: int(u.ID),
			OldValue: string(oldJSON), NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}

	for i := range added {
		added[i].UserID = targetID
		added[i].ID = 0
		if err := tx.Create(&added[i]).Error; err != nil {
			return err
		}
		newJSON, _ := json.Marshal(added[i])
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "INSERT", TargetTable: "instructor_textbooks",
			FieldName: "textbook_item", RecordID: int(added[i].ID),
			OldValue: "", NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyPropertyDiff(
	tx *gorm.DB, editorID, targetID int,
	added []models.InstructorIntellectualProperty,
	updated []PropertyDiffItem,
	deleted []DeleteDiffItem,
) error {
	// โหลดเกณฑ์น้ำหนักจาก DB มาทำ Map ครั้งเดียว
	var weights []models.RankingTierWeight
	if err := tx.Where("is_active = ?", true).Find(&weights).Error; err != nil {
		return err
	}
	weightMap := make(map[string]float64)
	for _, w := range weights {
		weightMap[w.TierCode] = w.Weight
	}

	for _, d := range deleted {
		var old models.InstructorIntellectualProperty
		if err := tx.First(&old, d.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบทรัพย์สินทางปัญญา ID %d", d.ID)
		}
		r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).
			Delete(&models.InstructorIntellectualProperty{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("ทรัพย์สินทางปัญญา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID)
		}
		oldJSON, _ := json.Marshal(old)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "DELETE", TargetTable: "instructor_intellectual_properties",
			FieldName: "property_item", RecordID: int(d.ID),
			OldValue: string(oldJSON), NewValue: "",
		}).Error; err != nil {
			return err
		}
	}

	for _, u := range updated {
		if _, exists := weightMap[u.Type]; !exists {
			return fmt.Errorf("ไม่พบการตั้งค่าคะแนนน้ำหนักสำหรับประเภท '%s' ในฐานข้อมูล", u.Type)
		}
		var old models.InstructorIntellectualProperty
		if err := tx.First(&old, u.ID).Error; err != nil {
			return fmt.Errorf("ไม่พบทรัพย์สินทางปัญญา ID %d", u.ID)
		}
		r := tx.Model(&models.InstructorIntellectualProperty{}).
			Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
			Updates(map[string]interface{}{
				"type":                u.Type,
				"title":               u.Title,
				"registration_number": u.RegistrationNumber,
				"granted_year":        u.GrantedYear,
			})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("ทรัพย์สินทางปัญญา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID)
		}
		var newVal models.InstructorIntellectualProperty
		tx.First(&newVal, u.ID)
		oldJSON, _ := json.Marshal(old)
		newJSON, _ := json.Marshal(newVal)
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "UPDATE", TargetTable: "instructor_intellectual_properties",
			FieldName: "property_item", RecordID: int(u.ID),
			OldValue: string(oldJSON), NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}

	for i := range added {
		if _, exists := weightMap[added[i].Type]; !exists {
			return fmt.Errorf("ไม่พบการตั้งค่าคะแนนน้ำหนักสำหรับประเภท '%s' ในฐานข้อมูล", added[i].Type)
		}
		added[i].UserID = targetID
		added[i].ID = 0
		if err := tx.Create(&added[i]).Error; err != nil {
			return err
		}
		newJSON, _ := json.Marshal(added[i])
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID,
			Action: "INSERT", TargetTable: "instructor_intellectual_properties",
			FieldName: "property_item", RecordID: int(added[i].ID),
			OldValue: "", NewValue: string(newJSON),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyCoursesReplace(
	tx *gorm.DB, editorID, targetID int,
	courses []models.InstructorCourseResponsibility,
) error {
	var oldCourses []models.InstructorCourseResponsibility
	if err := tx.Where("user_id = ?", targetID).Find(&oldCourses).Error; err != nil {
		return err
	}

	if !dataHasChanged(oldCourses, courses) {
		return nil
	}

	oldCoursesJSON, _ := json.Marshal(oldCourses)

	if err := tx.Where("user_id = ?", targetID).Delete(&models.InstructorCourseResponsibility{}).Error; err != nil {
		return err
	}
	for i := range courses {
		courses[i].UserID = targetID
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "course_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"deleted_at": nil}),
		}).Create(&courses[i]).Error; err != nil {
			return err
		}
	}

	newCoursesJSON, _ := json.Marshal(courses)

	action := "UPDATE"
	if len(oldCourses) == 0 {
		action = "INSERT"
	} else if len(courses) == 0 {
		action = "DELETE"
	}

	return tx.Create(&models.InstructorEditLog{
		UserEditID: editorID, TargetUserID: targetID,
		Action: action, TargetTable: "instructor_course_responsibility",
		FieldName: "course_ids",
		RecordID:  targetID,
		OldValue:  string(oldCoursesJSON), NewValue: string(newCoursesJSON),
	}).Error
}

func logFieldChanges(tx *gorm.DB, editorID, targetID int, table string, recordID int, oldData, newData map[string]interface{}) error {
	for field, newVal := range newData {
		oldVal := oldData[field]
		oldStr := fmt.Sprintf("%v", oldVal)
		newStr := fmt.Sprintf("%v", newVal)
		if oldStr == newStr {
			continue
		}
		actionType := "UPDATE"
		if oldVal == nil || oldStr == "" || oldStr == "<nil>" {
			actionType = "INSERT"
		}
		if err := tx.Create(&models.InstructorEditLog{
			UserEditID: editorID, TargetUserID: targetID, Action: actionType,
			TargetTable: table, FieldName: field,
			RecordID: recordID,
			OldValue: oldStr, NewValue: newStr,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func dataHasChanged[T any](oldData []T, newData []T) bool {
	if len(oldData) != len(newData) {
		return true
	}
	strip := func(data []T) []map[string]interface{} {
		b, _ := json.Marshal(data)
		var rows []map[string]interface{}
		json.Unmarshal(b, &rows)
		for _, row := range rows {
			delete(row, "id")
			delete(row, "ID")
			delete(row, "created_at")
			delete(row, "updated_at")
			delete(row, "deleted_at")
		}
		return rows
	}
	o, _ := json.Marshal(strip(oldData))
	n, _ := json.Marshal(strip(newData))
	return string(o) != string(n)
}