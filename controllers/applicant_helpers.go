package controllers

import (
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"
)

func resolveApplicantPosition(user *models.User) string {
	if user == nil {
		return ""
	}

	if user.PositionTitle != nil {
		if title := strings.TrimSpace(*user.PositionTitle); title != "" {
			return title
		}
	}

	if user.UserID == 0 {
		return ""
	}

	type positionRow struct {
		Position *string `gorm:"column:position"`
	}

	var row positionRow
	if err := config.DB.Table("users").
		Select("position").
		Where("user_id = ?", user.UserID).
		Scan(&row).Error; err == nil {
		if row.Position != nil {
			if title := strings.TrimSpace(*row.Position); title != "" {
				return title
			}
		}
	}

	return ""
}

func lookupPositionFromUserID(userID int) string {
	if userID <= 0 {
		return ""
	}

	var user models.User
	if err := config.DB.
		Select("user_id", "position").
		Where("user_id = ?", userID).
		First(&user).Error; err != nil {
		return ""
	}

	return resolveApplicantPosition(&user)
}
