package services

import (
	"context"
	"fund-management-api/models"
	"gorm.io/gorm"
)

type RankingSourceService struct {
	db *gorm.DB
}

func NewRankingSourceService(db *gorm.DB) *RankingSourceService {
	return &RankingSourceService{db: db}
}

// GetSources ดึงข้อมูลแหล่งที่มาทั้งหมดที่ยังไม่ถูกลบ (Soft Delete)
func (s *RankingSourceService) GetSources(ctx context.Context) ([]models.RankingSource, error) {
	var sources []models.RankingSource
	
	err := s.db.WithContext(ctx).
		Order("source_id asc").
		Find(&sources).Error
		
	return sources, err
}

// UpdateSources จัดการสร้างใหม่ หรือ อัปเดตข้อมูลแหล่งที่มา
func (s *RankingSourceService) UpdateSources(ctx context.Context, sources []models.RankingSource) ([]models.RankingSource, error) {
    err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        for i := range sources {
            if sources[i].SourceID == 0 {
                // ตรวจสอบก่อนว่ามี source_code นี้อยู่แล้วหรือไม่ (รวม soft deleted)
                var existing models.RankingSource
                err := tx.Unscoped().
                    Where("source_code = ?", sources[i].SourceCode).
                    First(&existing).Error

                if err == nil {
                    // มีอยู่แล้ว → restore + update แทน
                    sources[i].SourceID = existing.SourceID
                    if err := tx.Unscoped().Model(&existing).Updates(map[string]interface{}{
                        "source_name": sources[i].SourceName,
                        "source_code": sources[i].SourceCode,
                        "description": sources[i].Description,
                        "is_active":   sources[i].IsActive,
                        "deleted_at":  nil, // restore ถ้าถูก soft delete
                    }).Error; err != nil {
                        return err
                    }
                } else {
                    // ไม่มีอยู่จริงๆ → create ใหม่
                    if err := tx.Create(&sources[i]).Error; err != nil {
                        return err
                    }
                }
            } else {
                // มี source_id → update ปกติ
                if err := tx.Model(&models.RankingSource{}).
                    Where("source_id = ?", sources[i].SourceID).
                    Updates(map[string]interface{}{
                        "source_name": sources[i].SourceName,
                        "source_code": sources[i].SourceCode,
                        "description": sources[i].Description,
                        "is_active":   sources[i].IsActive,
                    }).Error; err != nil {
                    return err
                }
            }
        }
        return nil
    })
    return sources, err
}

func (s *RankingSourceService) DeleteSource(ctx context.Context, id uint) error {
    return s.db.WithContext(ctx).Delete(&models.RankingSource{}, id).Error
}