package services

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    "fund-management-api/models"

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
        // educations diff
        educationsAdded   []models.InstructorEducationTab,
        educationsUpdated []EducationDiffItem,
        educationsDeleted []DeleteDiffItem,
        // expertises diff
        expertisesAdded   []models.InstructorExpertiseTab,
        expertisesUpdated []ExpertiseDiffItem,
        expertisesDeleted []DeleteDiffItem,
        // research projects diff
        projectsAdded   []models.InstructorResearchProject,
        projectsUpdated []ProjectDiffItem,
        projectsDeleted []DeleteDiffItem,
        // textbooks diff
        textbooksAdded   []models.InstructorTextbook,
        textbooksUpdated []TextbookDiffItem,
        textbooksDeleted []DeleteDiffItem,
        // intellectual properties diff
        propertiesAdded   []models.InstructorIntellectualProperty,
        propertiesUpdated []PropertyDiffItem,
        propertiesDeleted []DeleteDiffItem,
        // courses (ยังใช้ replace)
        courses []models.InstructorCourseResponsibility,
    ) error
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
        if err != nil { return nil, err }
    }
	var weights []models.RankingTierWeight
	if err := s.db.WithContext(ctx).Where("is_active = ?", true).Find(&weights).Error; err == nil {
		// แก้จาก map[string]float64 เป็น map[string]models.RankingTierWeight เพื่อเก็บข้อมูลทั้ง Object
		weightMap := make(map[string]models.RankingTierWeight)
		for _, w := range weights {
			weightMap[w.TierCode] = w // เก็บ Struct ไว้ทั้งหมด (ได้ทั้ง Weight และ TierName)
		}

		// วนลูปนำข้อมูลเกณฑ์น้ำหนักไปหยอดใส่ฟิลด์ TierDetails ให้แต่ละรายการทรัพย์สินทางปัญญา
		for i := range fp.InstructorIntellectualProperties {
			pType := fp.InstructorIntellectualProperties[i].Type // ค่าเช่น 'patent', 'petty_patent', 'copyright'
			
			// ค้นหาเกณฑ์น้ำหนักจากตารางฐานข้อมูล
			if tierData, exists := weightMap[pType]; exists {
				// ส่ง Pointer ของ Struct ตัวที่จับคู่เจอไปเก็บใน TierDetails
				matchedTier := tierData
				fp.InstructorIntellectualProperties[i].TierDetails = &matchedTier
			} else {
				// เนื่องจากเราเน้น Mapping จากฐานข้อมูลเท่านั้น ถ้าไม่เจอใน DB ให้ใส่เป็น nil
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
                if err := tx.Where("user_id = ?", user.Header.UserID).Delete(&models.InstructorEducationTab{}).Error; err != nil { return err }
                if len(user.Educations) > 0 {
                    for i := range user.Educations { user.Educations[i].UserID = user.Header.UserID }
                    return tx.Create(&user.Educations).Error
                }
                return nil
            },
            func() error {
                if err := tx.Where("user_id = ?", user.Header.UserID).Delete(&models.InstructorExpertiseTab{}).Error; err != nil { return err }
                if len(user.Expertises) > 0 {
                    for i := range user.Expertises { user.Expertises[i].UserID = user.Header.UserID }
                    return tx.Create(&user.Expertises).Error
                }
                return nil
            },
        } {
            if err := fn(); err != nil { return err }
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

        // 1. Header
        if len(headerChanges) > 0 {
            var oldUser map[string]interface{}
            if err := tx.Table("users").Where("user_id = ?", targetID).Take(&oldUser).Error; err != nil { return err }
            delete(headerChanges, "user_id")
            if err := tx.Table("users").Where("user_id = ?", targetID).Updates(headerChanges).Error; err != nil { return err }
            if err := logFieldChanges(tx, editorID, targetID, "users", oldUser, headerChanges); err != nil { return err }
        }

        // 2. Educations diff
        if err := applyDiffWithLog[models.InstructorEducationTab](tx, editorID, targetID, "instructor_educations", "education_list",
            len(educationsAdded)+len(educationsUpdated)+len(educationsDeleted) > 0,
            func() error {
                for _, d := range educationsDeleted {
                    r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).Delete(&models.InstructorEducationTab{})
                    if r.Error != nil { return r.Error }
                    if r.RowsAffected == 0 { return fmt.Errorf("ประวัติการศึกษา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID) }
                }
                for _, u := range educationsUpdated {
                    r := tx.Model(&models.InstructorEducationTab{}).
                        Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
                        Updates(map[string]interface{}{
                            "degree_id": u.DegreeID, "degree_title_th": u.DegreeTitleTh,
                            "university_th": u.UniversityTh, "country": u.Country, "grad_year": u.GradYear,
                        })
                    if r.Error != nil { return r.Error }
                    if r.RowsAffected == 0 { return fmt.Errorf("ประวัติการศึกษา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID) }
                }
                for i := range educationsAdded {
                    educationsAdded[i].UserID = targetID
                    educationsAdded[i].ID = 0
                    if err := tx.Create(&educationsAdded[i]).Error; err != nil { return err }
                }
                return nil
            },
        ); err != nil { return err }

        // 3. Expertises diff
        if err := applyDiffWithLog[models.InstructorExpertiseTab](tx, editorID, targetID, "instructor_expertises", "expertise_list",
            len(expertisesAdded)+len(expertisesUpdated)+len(expertisesDeleted) > 0,
            func() error {
                for _, d := range expertisesDeleted {
                    r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).Delete(&models.InstructorExpertiseTab{})
                    if r.Error != nil { return r.Error }
                    if r.RowsAffected == 0 { return fmt.Errorf("ความเชี่ยวชาญ ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID) }
                }
                for _, u := range expertisesUpdated {
                    r := tx.Model(&models.InstructorExpertiseTab{}).
                        Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
                        Updates(map[string]interface{}{"expertise": u.Expertise})
                    if r.Error != nil { return r.Error }
                    if r.RowsAffected == 0 { return fmt.Errorf("ความเชี่ยวชาญ ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID) }
                }
                for i := range expertisesAdded {
                    expertisesAdded[i].UserID = targetID
                    expertisesAdded[i].ID = 0
                    if err := tx.Create(&expertisesAdded[i]).Error; err != nil { return err }
                }
                return nil
            },
        ); err != nil { return err }

        // 4. Research Projects diff
        if err := applyDiffWithLog[models.InstructorResearchProject](tx, editorID, targetID, "instructor_research_projects", "project_list",
            len(projectsAdded)+len(projectsUpdated)+len(projectsDeleted) > 0,
            func() error {
                for _, d := range projectsDeleted {
                    r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).Delete(&models.InstructorResearchProject{})
                    if r.Error != nil { return r.Error }
                    if r.RowsAffected == 0 { return fmt.Errorf("โครงการวิจัย ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID) }
                }
                for _, u := range projectsUpdated {
                    r := tx.Model(&models.InstructorResearchProject{}).
                        Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
                        Updates(map[string]interface{}{
                            "fiscal_year": u.FiscalYear, "project_name_th": u.ProjectNameTh,
                            "project_name_en": u.ProjectNameEn, "source_of_fund": u.SourceOfFund,
                            "start_date": u.StartDate, "end_date": u.EndDate, "budget": u.Budget,
                        })
                    if r.Error != nil { return r.Error }
                    if r.RowsAffected == 0 { return fmt.Errorf("โครงการวิจัย ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID) }
                }
                for i := range projectsAdded {
                    projectsAdded[i].UserID = targetID
                    projectsAdded[i].ID = 0
                    if err := tx.Create(&projectsAdded[i]).Error; err != nil { return err }
                }
                return nil
            },
        ); err != nil { return err }

        // 5. Textbooks diff
        if err := applyDiffWithLog[models.InstructorTextbook](tx, editorID, targetID, "instructor_textbooks", "textbook_list",
            len(textbooksAdded)+len(textbooksUpdated)+len(textbooksDeleted) > 0,
            func() error {
                for _, d := range textbooksDeleted {
                    r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).Delete(&models.InstructorTextbook{})
                    if r.Error != nil { return r.Error }
                    if r.RowsAffected == 0 { return fmt.Errorf("ตำรา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID) }
                }
                for _, u := range textbooksUpdated {
                    r := tx.Model(&models.InstructorTextbook{}).
                        Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
                        Updates(map[string]interface{}{
                            "title": u.Title, "year": u.Year,
                            "publisher": u.Publisher, "edition": u.Edition,
                        })
                    if r.Error != nil { return r.Error }
                    if r.RowsAffected == 0 { return fmt.Errorf("ตำรา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID) }
                }
                for i := range textbooksAdded {
                    textbooksAdded[i].UserID = targetID
                    if err := tx.Create(&textbooksAdded[i]).Error; err != nil { return err }
                }
                return nil
            },
        ); err != nil { return err }

        // 6. Intellectual Properties diff
        if err := applyDiffWithLog[models.InstructorIntellectualProperty](tx, editorID, targetID, "instructor_intellectual_properties", "property_list",
			len(propertiesAdded)+len(propertiesUpdated)+len(propertiesDeleted) > 0,
			func() error {
				// 1. โหลดเกณฑ์น้ำหนักจากตารางฐานข้อมูลมาทำ Map
				var weights []models.RankingTierWeight
				if err := tx.Where("is_active = ?", true).Find(&weights).Error; err != nil {
					return err
				}
				weightMap := make(map[string]float64)
				for _, w := range weights {
					weightMap[w.TierCode] = w.Weight
				}

				for _, d := range propertiesDeleted {
					r := tx.Where("id = ? AND updated_at = ? AND user_id = ?", d.ID, d.UpdatedAt, targetID).Delete(&models.InstructorIntellectualProperty{})
					if r.Error != nil { return r.Error }
					if r.RowsAffected == 0 { return fmt.Errorf("ทรัพย์สินทางปัญญา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", d.ID) }
				}

				for _, u := range propertiesUpdated {
					//เช็คเฉยๆ ว่ามีประเภทนี้ใน DB จริงไหม (Strict Mapping) ถ้าไม่มีให้รีเทิร์น Error
					_, exists := weightMap[u.Type]
					if !exists {
						return fmt.Errorf("ไม่พบการตั้งค่าคะแนนน้ำหนักสำหรับประเภท '%s' ในฐานข้อมูล", u.Type)
					}

					r := tx.Model(&models.InstructorIntellectualProperty{}).
						Where("id = ? AND updated_at = ? AND user_id = ?", u.ID, u.UpdatedAt, targetID).
						Updates(map[string]interface{}{
							"type":                u.Type, 
							"title":               u.Title,
							"registration_number": u.RegistrationNumber, 
							"granted_year":        u.GrantedYear,
							//ไม่ต้องใส่ฟิลด์ "weight" แล้ว เพราะเราดึง Dynamic จากตารางเกณฑ์แทน
						})
					if r.Error != nil { return r.Error }
					if r.RowsAffected == 0 { return fmt.Errorf("ทรัพย์สินทางปัญญา ID %d ถูกแก้ไขโดยผู้ใช้อื่น กรุณา refresh", u.ID) }
				}

				for i := range propertiesAdded {
					propertiesAdded[i].UserID = targetID
					propertiesAdded[i].ID = 0
					
					// เช็คเฉยๆ สำหรับแถวที่เพิ่มใหม่ ถ้าไม่มีประเภทนี้ใน DB ให้รีเทิร์น Error
					_, exists := weightMap[propertiesAdded[i].Type]
					if !exists {
						return fmt.Errorf("ไม่พบการตั้งค่าคะแนนน้ำหนักสำหรับประเภท '%s' ในฐานข้อมูล", propertiesAdded[i].Type)
					}
					
					if err := tx.Create(&propertiesAdded[i]).Error; err != nil { return err }
				}
				return nil
			},
		); err != nil { return err }

        // 7. Courses (replace เหมือนเดิม)
        var oldCourses []models.InstructorCourseResponsibility
        if err := tx.Where("user_id = ?", targetID).Find(&oldCourses).Error; err != nil { return err }
        courseChanged := dataHasChanged(oldCourses, courses)
        oldCoursesJSON, _ := json.Marshal(oldCourses)
        if err := tx.Where("user_id = ?", targetID).Delete(&models.InstructorCourseResponsibility{}).Error; err != nil { return err }
        for i := range courses {
            courses[i].UserID = targetID
            if err := tx.Clauses(clause.OnConflict{
                Columns:   []clause.Column{{Name: "user_id"}, {Name: "course_id"}},
                DoUpdates: clause.Assignments(map[string]interface{}{"deleted_at": nil}),
            }).Create(&courses[i]).Error; err != nil { return err }
        }
        if courseChanged {
            newCoursesJSON, _ := json.Marshal(courses)
            action := "UPDATE"
            if len(oldCourses) == 0 { action = "INSERT" } else if len(courses) == 0 { action = "DELETE" }
            if err := tx.Create(&models.InstructorEditLog{
                UserEditID: editorID, TargetUserID: targetID, Action: action,
                TargetTable: "instructor_course_responsibility", FieldName: "course_ids",
                RecordID: targetID, OldValue: string(oldCoursesJSON), NewValue: string(newCoursesJSON),
            }).Error; err != nil { return err }
        }

        // 8. Summary log
        return tx.Create(&models.InstructorEditLog{
            UserEditID: editorID, TargetUserID: targetID, Action: "UPDATE",
            TargetTable: "instructor_profile", FieldName: "batch_update", RecordID: targetID,
            OldValue: "-", NewValue: fmt.Sprintf("Successfully updated by Admin ID %d", editorID),
        }).Error
    })
}

// ─── Helper: applyDiffWithLog ─────────────────────────────────────────────────
// snapshot before → apply fn → snapshot after → log
func applyDiffWithLog[T any](
    tx *gorm.DB,
    editorID, targetID int,
    table, fieldName string,
    hasChange bool,
    applyFn func() error,
) error {
    if !hasChange { return nil }

    var before []T
    tx.Where("user_id = ?", targetID).Find(&before)

    if err := applyFn(); err != nil { return err }

    var after []T
    tx.Where("user_id = ?", targetID).Find(&after)

    oldJSON, _ := json.Marshal(before)
    newJSON, _ := json.Marshal(after)

    action := "UPDATE"
    if len(before) == 0 { action = "INSERT" } else if len(after) == 0 { action = "DELETE" }

    return tx.Create(&models.InstructorEditLog{
        UserEditID: editorID, TargetUserID: targetID, Action: action,
        TargetTable: table, FieldName: fieldName, RecordID: targetID,
        OldValue: string(oldJSON), NewValue: string(newJSON),
    }).Error
}

// ─── Helper: logFieldChanges ──────────────────────────────────────────────────

func logFieldChanges(tx *gorm.DB, editorID, targetID int, table string, oldData, newData map[string]interface{}) error {
    for field, newVal := range newData {
        oldVal := oldData[field]
        oldStr := fmt.Sprintf("%v", oldVal)
        newStr := fmt.Sprintf("%v", newVal)
        if oldStr == newStr { continue }
        actionType := "UPDATE"
        if oldVal == nil || oldStr == "" || oldStr == "<nil>" { actionType = "INSERT" }
        if err := tx.Create(&models.InstructorEditLog{
            UserEditID: editorID, TargetUserID: targetID, Action: actionType,
            TargetTable: table, FieldName: field, RecordID: targetID,
            OldValue: oldStr, NewValue: newStr,
        }).Error; err != nil { return err }
    }
    return nil
}

// ─── Helper: dataHasChanged ───────────────────────────────────────────────────

func dataHasChanged[T any](oldData []T, newData []T) bool {
    if len(oldData) != len(newData) { return true }
    strip := func(data []T) []map[string]interface{} {
        b, _ := json.Marshal(data)
        var rows []map[string]interface{}
        json.Unmarshal(b, &rows)
        for _, row := range rows {
            delete(row, "id"); delete(row, "ID")
            delete(row, "created_at"); delete(row, "updated_at"); delete(row, "deleted_at")
        }
        return rows
    }
    o, _ := json.Marshal(strip(oldData))
    n, _ := json.Marshal(strip(newData))
    return string(o) != string(n)
}