package controllers

import (
	"errors"
	"time"

	"fund-management-api/models"
	"gorm.io/gorm"
)

var errBudgetYearBundleNotEmpty = errors.New("ปีปลายทางมีข้อมูลแล้ว กรุณาเลือกปีใหม่หรือปีที่ยังไม่มีข้อมูลสำหรับการคัดลอกทั้งชุด")

type budgetYearExtraCopyCounts struct {
	RewardRates  int `json:"reward_rates"`
	FeeLimits    int `json:"fee_limits"`
	Installments int `json:"installments"`
}

// Lock the destination before checking it so simultaneous copies cannot both
// append a complete bundle to an existing year.
func ensureBudgetYearBundleEmpty(tx *gorm.DB, year models.Year) error {
	var lockedID int
	if err := tx.Raw(`SELECT year_id FROM years WHERE year_id = ? FOR UPDATE`, year.YearID).Scan(&lockedID).Error; err != nil {
		return err
	}
	if lockedID == 0 {
		return errors.New("ไม่พบปีปลายทาง")
	}
	var count int64
	if err := tx.Raw(`SELECT
		(SELECT COUNT(*) FROM fund_categories WHERE year_id = ? AND delete_at IS NULL) +
		(SELECT COUNT(*) FROM fund_subcategories WHERE year_id = ? AND delete_at IS NULL) +
		(SELECT COUNT(*) FROM publication_reward_rates WHERE year = ?) +
		(SELECT COUNT(*) FROM reward_config WHERE year = ? AND delete_at IS NULL) +
		(SELECT COUNT(*) FROM fund_installment_periods WHERE year_id = ? AND deleted_at IS NULL) +
		(SELECT COUNT(*) FROM submissions WHERE year_id = ? AND deleted_at IS NULL)`,
		year.YearID, year.YearID, year.Year, year.Year, year.YearID, year.YearID).Scan(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return errBudgetYearBundleNotEmpty
	}
	return nil
}

// Preserve the month/day even for a leap-day cutoff copied to a non-leap year.
func shiftBudgetYearCutoff(value time.Time, yearDiff int) time.Time {
	if value.IsZero() {
		return value
	}
	year, month, day := value.Date()
	lastDay := time.Date(year+yearDiff, month+1, 0, 0, 0, 0, 0, value.Location()).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year+yearDiff, month, day, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location())
}

// The caller owns the transaction, including the new year and fund structure.
func copyBudgetYearExtras(tx *gorm.DB, source, target models.Year, now time.Time) (budgetYearExtraCopyCounts, error) {
	var counts budgetYearExtraCopyCounts
	sourceCE, sourceOK := parseCalendarYearValue(source.Year)
	targetCE, targetOK := parseCalendarYearValue(target.Year)
	if !sourceOK || !targetOK {
		return counts, errors.New("ปีต้นทางหรือปลายทางไม่ใช่ปีที่ถูกต้อง")
	}
	var rates []models.PublicationRewardRate
	if err := tx.Where("year = ?", source.Year).Order("rate_id").Find(&rates).Error; err != nil {
		return counts, err
	}
	for _, rate := range rates {
		rate.RateID = 0
		rate.Year = target.Year
		rate.CreateAt, rate.UpdateAt = &now, &now
		if err := tx.Create(&rate).Error; err != nil {
			return counts, err
		}
		counts.RewardRates++
	}
	var limits []models.RewardConfig
	if err := tx.Where("year = ? AND delete_at IS NULL", source.Year).Order("config_id").Find(&limits).Error; err != nil {
		return counts, err
	}
	for _, limit := range limits {
		limit.ConfigID = 0
		limit.Year = target.Year
		limit.CreateAt, limit.UpdateAt = &now, &now
		if err := tx.Create(&limit).Error; err != nil {
			return counts, err
		}
		counts.FeeLimits++
	}
	var periods []models.FundInstallmentPeriod
	if err := tx.Where("year_id = ? AND deleted_at IS NULL", source.YearID).Order("installment_period_id").Find(&periods).Error; err != nil {
		return counts, err
	}
	for _, period := range periods {
		period.InstallmentPeriodID = 0
		period.YearID = target.YearID
		period.CutoffDate = shiftBudgetYearCutoff(period.CutoffDate, targetCE-sourceCE)
		period.CreatedAt, period.UpdatedAt = now, now
		if err := tx.Create(&period).Error; err != nil {
			return counts, err
		}
		counts.Installments++
	}
	return counts, nil
}
