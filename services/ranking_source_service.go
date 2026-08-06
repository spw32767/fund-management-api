package services

import (
	"context"
	"encoding/json"
	"fmt"
	"fund-management-api/models"

	"gorm.io/gorm"
)

type RankingSourceService struct {
	db *gorm.DB
}

func sourceHasChanged(old models.RankingSource, new models.RankingSource) bool {
    return old.SourceCode != new.SourceCode ||
           old.SourceName != new.SourceName ||
           old.Description != new.Description ||
            old.IsActive != new.IsActive 
}


func NewRankingSourceService(db *gorm.DB) *RankingSourceService {
	return &RankingSourceService{db: db}
}

// GetSources ดึงข้อมูลแหล่งที่มาทั้งหมด
func (s *RankingSourceService) GetSources(ctx context.Context) ([]models.RankingSource, error) {
	var sources []models.RankingSource
	err := s.db.WithContext(ctx).Order("source_id ASC").Find(&sources).Error
	return sources, err
}

// UpdateSources รับ slice ของแหล่งข้อมูลเพื่อ INSERT หรือ UPDATE พร้อม Audit Log
func (s *RankingSourceService) UpdateSources(ctx context.Context, sources []models.RankingSource, editorID int) ([]models.RankingSource, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range sources {
			// INSERT
			if sources[i].SourceID == 0 {
				if err := tx.Create(&sources[i]).Error; err != nil {
					return err
				}

				newJSON, _ := json.Marshal(sources[i])
				if err := tx.Create(&models.InstructorEditLog{
					UserEditID:   editorID,
					TargetUserID: nil,
					Action:       "INSERT",
					TargetTable:  "ranking_sources",
					FieldName:    "source_item",
					RecordID:     int(sources[i].SourceID),
					OldValue:     "",
					NewValue:     string(newJSON),
				}).Error; err != nil {
					return err
				}
				continue
			}

			// UPDATE
			var old models.RankingSource
if err := tx.First(&old, sources[i].SourceID).Error; err != nil {
    return fmt.Errorf("ไม่พบแหล่งข้อมูล ID %d", sources[i].SourceID)
}

			if !sourceHasChanged(old, sources[i]) {
    continue
}

//Marshal ค่าเก่าก่อน UPDATE
oldJSON, _ := json.Marshal(old)

updateData := sources[i]
updateData.SourceID = 0
if err := tx.Model(&old).Updates(updateData).Error; err != nil {
    return err
}

//ใช้ input เป็น newJSON แทนการ query ใหม่
newJSON, _ := json.Marshal(sources[i])

if err := tx.Create(&models.InstructorEditLog{
    UserEditID:   editorID,
    TargetUserID: nil,
    Action:       "UPDATE",
    TargetTable:  "ranking_sources",
    FieldName:    "source_item",
    RecordID:     int(sources[i].SourceID),
    OldValue:     string(oldJSON),
    NewValue:     string(newJSON),
}).Error; err != nil {
    return err
}
		}
		return nil
	})

	return sources, err
}

// DeleteSource ลบข้อมูลแหล่งข้อมูล (Soft Delete) พร้อมบันทึก audit log
func (s *RankingSourceService) DeleteSource(ctx context.Context, id uint, editorID int) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old models.RankingSource
		if err := tx.First(&old, id).Error; err != nil {
			return fmt.Errorf("ไม่พบแหล่งข้อมูล ID %d", id)
		}

		if err := tx.Delete(&models.RankingSource{}, id).Error; err != nil {
			return err
		}

		oldJSON, _ := json.Marshal(old)
		return tx.Create(&models.InstructorEditLog{
			UserEditID:   editorID,
			TargetUserID: nil,
			Action:       "DELETE",
			TargetTable:  "ranking_sources",
			FieldName:    "source_item",
			RecordID:     int(id),
			OldValue:     string(oldJSON),
			NewValue:     "",
		}).Error
	})
}