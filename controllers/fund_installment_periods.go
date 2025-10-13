package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
)

// GetFundInstallmentPeriods returns installment periods ordered by cutoff date.
func GetFundInstallmentPeriods(c *gin.Context) {
	yearParam := strings.TrimSpace(c.Query("year_id"))

	query := config.DB.Model(&models.FundInstallmentPeriod{}).
		Where("deleted_at IS NULL")

	if yearParam != "" {
		yearID, err := strconv.Atoi(yearParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "invalid year_id",
			})
			return
		}
		query = query.Where("year_id = ?", yearID)
	}

	var periods []models.FundInstallmentPeriod
	if err := query.Order("cutoff_date ASC, installment_number ASC").Find(&periods).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to fetch fund installment periods",
		})
		return
	}

	responses := make([]fundInstallmentPeriodResponse, 0, len(periods))
	for _, period := range periods {
		responses = append(responses, newFundInstallmentPeriodResponse(period))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"periods": responses,
		"data":    responses,
	})
}

type fundInstallmentPeriodResponse struct {
	InstallmentPeriodID int     `json:"installment_period_id"`
	YearID              int     `json:"year_id"`
	InstallmentNumber   int     `json:"installment_number"`
	CutoffDate          string  `json:"cutoff_date"`
	Name                *string `json:"name,omitempty"`
	Status              *string `json:"status,omitempty"`
	Remark              *string `json:"remark,omitempty"`
}

func newFundInstallmentPeriodResponse(period models.FundInstallmentPeriod) fundInstallmentPeriodResponse {
	cutoff := ""
	if !period.CutoffDate.IsZero() {
		cutoff = period.CutoffDate.Format("2006-01-02")
	}

	return fundInstallmentPeriodResponse{
		InstallmentPeriodID: period.InstallmentPeriodID,
		YearID:              period.YearID,
		InstallmentNumber:   period.InstallmentNumber,
		CutoffDate:          cutoff,
		Name:                period.Name,
		Status:              period.Status,
		Remark:              period.Remark,
	}
}
