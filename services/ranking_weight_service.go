package services

import (
    "context"
    "encoding/json"
    "fmt"
    "fund-management-api/models"

    "gorm.io/gorm"
)

// weightHasChanged ตรวจสอบว่าข้อมูลมีการเปลี่ยนแปลงหรือไม่
func weightHasChanged(old models.RankingTierWeight, new models.RankingTierWeight) bool {
    return old.TierCode        != new.TierCode        ||
           old.TierName        != new.TierName        ||
           old.Weight          != new.Weight          ||
           old.SourceID        != new.SourceID        ||
           old.SortOrder       != new.SortOrder       ||
           old.IsActive        != new.IsActive        ||
           old.Description     != new.Description     ||
           old.ThaiDescription != new.ThaiDescription
}

type RankingWeightService struct {
    db *gorm.DB
}

func NewRankingWeightService(db *gorm.DB) *RankingWeightService {
    return &RankingWeightService{db: db}
}

// GetWeights ดึงข้อมูลเกณฑ์น้ำหนักทั้งหมด พร้อม preload แหล่งที่มา เรียงตาม sort_order
func (s *RankingWeightService) GetWeights(ctx context.Context) ([]models.RankingTierWeight, error) {
    var weights []models.RankingTierWeight
    err := s.db.WithContext(ctx).
        Preload("RankingSource").
        Order("sort_order ASC").
        Find(&weights).Error
    return weights, err
}

// UpdateWeights รับ slice ของเกณฑ์น้ำหนัก แล้ว INSERT หรือ UPDATE พร้อมบันทึก audit log
func (s *RankingWeightService) UpdateWeights(
    ctx context.Context,
    weights []models.RankingTierWeight,
    editorID int,
) ([]models.RankingTierWeight, error) {

    err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        for i := range weights {

            // INSERT  (TierWeightID == 0)
            if weights[i].TierWeightID == 0 {
                if err := tx.Create(&weights[i]).Error; err != nil {
                    return err
                }

                // ใช้ input เป็น newJSON เหมือน ranking_source
                newJSON, _ := json.Marshal(weights[i])
                if err := tx.Create(&models.InstructorEditLog{
                    UserEditID:   editorID,
                    TargetUserID: 0,
                    Action:       "INSERT",
                    TargetTable:  "ranking_tier_weights",
                    FieldName:    "weight_item",
                    RecordID:     int(weights[i].TierWeightID),
                    OldValue:     "",
                    NewValue:     string(newJSON),
                }).Error; err != nil {
                    return err
                }
                continue
            }

            // UPDATE  (TierWeightID != 0)
            var old models.RankingTierWeight
            if err := tx.First(&old, weights[i].TierWeightID).Error; err != nil {
                return fmt.Errorf("ไม่พบเกณฑ์น้ำหนัก ID %d", weights[i].TierWeightID)
            }

            // ใช้ weightHasChanged() แทน JSON compare — เหมือน ranking_source
            if !weightHasChanged(old, weights[i]) {
                continue
            }

            // Marshal ค่าเก่าก่อน UPDATE
            oldJSON, _ := json.Marshal(old)

            // เคลียร์ ID ป้องกัน GORM สับสน
            updateData := weights[i]
            updateData.TierWeightID = 0
            if err := tx.Model(&old).Updates(updateData).Error; err != nil {
                return err
            }

            // ใช้ input เป็น newJSON แทนการ query ใหม่ — เหมือน ranking_source
            newJSON, _ := json.Marshal(weights[i])
            if err := tx.Create(&models.InstructorEditLog{
                UserEditID:   editorID,
                TargetUserID: 0,
                Action:       "UPDATE",
                TargetTable:  "ranking_tier_weights",
                FieldName:    "weight_item",
                RecordID:     int(weights[i].TierWeightID),
                OldValue:     string(oldJSON),
                NewValue:     string(newJSON),
            }).Error; err != nil {
                return err
            }
        }
        return nil
    })

    return weights, err
}

// DeleteWeight ลบเกณฑ์น้ำหนัก (Soft Delete) พร้อมบันทึก audit log
func (s *RankingWeightService) DeleteWeight(ctx context.Context, id uint, editorID int) error {
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        var old models.RankingTierWeight
        if err := tx.First(&old, id).Error; err != nil {
            return fmt.Errorf("ไม่พบเกณฑ์น้ำหนัก ID %d", id)
        }

        if err := tx.Delete(&models.RankingTierWeight{}, id).Error; err != nil {
            return err
        }

        oldJSON, _ := json.Marshal(old)
        return tx.Create(&models.InstructorEditLog{
            UserEditID:   editorID,
            TargetUserID: 0,
            Action:       "DELETE",
            TargetTable:  "ranking_tier_weights",
            FieldName:    "weight_item",
            RecordID:     int(id),
            OldValue:     string(oldJSON),
            NewValue:     "",
        }).Error
    })
}