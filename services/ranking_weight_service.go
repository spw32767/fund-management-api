package services

import (
	"context"
	"fund-management-api/models"
	"gorm.io/gorm"
)

type RankingWeightService struct {
	db *gorm.DB
}

func NewRankingWeightService(db *gorm.DB) *RankingWeightService {
	return &RankingWeightService{db: db}
}

// GetWeights ดึงข้อมูลค่าน้ำหนักทั้งหมด (เปิด Preload ดึงตารางแม่ร่วมด้วย)
func (s *RankingWeightService) GetWeights(ctx context.Context) ([]models.RankingTierWeight, error) {
	var weights []models.RankingTierWeight
	
	// เติม .Preload("RankingSource") เข้าไปก่อน Find 
	// เพื่อให้ GORM ไปดึง source_name จากตาราง ranking_sources ออกมาส่งให้ Frontend ครับ
	err := s.db.WithContext(ctx).
		Preload("RankingSource").
		Order("sort_order asc").
		Find(&weights).Error
		
	return weights, err
}

// UpdateWeights อัปเดตค่าน้ำหนักและอัปเดต/สร้างข้อมูลในตารางแม่ด้วย
func (s *RankingWeightService) UpdateWeights(ctx context.Context, weights []models.RankingTierWeight) ([]models.RankingTierWeight, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range weights {
			
			// เคสที่ 1: เพิ่มเกณฑ์น้ำหนักใหม่ (INSERT)
			if weights[i].TierWeightID == 0 {
				if err := tx.Create(&weights[i]).Error; err != nil {
					return err
				}
			} else {
				// เคสที่ 2: แก้ไขเกณฑ์น้ำหนักเดิมที่มีอยู่แล้ว (UPDATE)
				// ปกติคำสั่ง tx.Save จะอัปเดตแค่ตารางหลักตารางเดียว 
				// เราจำเป็นต้องเพิ่ม Session FullSaveAssociations เพื่อบังคับให้มันอัปเดตตารางแม่ (ranking_sources) ที่อยู่ข้างในด้วยครับ
				err := tx.Session(&gorm.Session{FullSaveAssociations: true}).Save(&weights[i]).Error
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	return weights, err
}

// DeleteWeight ลบเกณฑ์น้ำหนักออกจากฐานข้อมูล (รองรับ Soft Delete ถ้าโมเดลมี deleted_at)
func (s *RankingWeightService) DeleteWeight(ctx context.Context, id uint) error {
    return s.db.WithContext(ctx).Delete(&models.RankingTierWeight{}, id).Error
}