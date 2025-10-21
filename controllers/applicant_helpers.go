package controllers

import (
	"database/sql"
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

	if title := strings.TrimSpace(user.Position.PositionName); title != "" {
		return title
	}

	if user.UserID == 0 {
		return ""
	}

	type positionRow struct {
		Position     sql.NullString `gorm:"column:position"`
		PositionName sql.NullString `gorm:"column:position_name"`
	}

	var row positionRow
	if err := config.DB.Table("users").
		Select("position, position_name").
		Where("user_id = ?", user.UserID).
		Scan(&row).Error; err == nil {
		if row.Position.Valid {
			if title := strings.TrimSpace(row.Position.String); title != "" {
				return title
			}
		}
		if row.PositionName.Valid {
			if title := strings.TrimSpace(row.PositionName.String); title != "" {
				return title
			}
		}
	}

	if user.PositionID == 0 {
		return ""
	}

	type positionNameRow struct {
		Name sql.NullString `gorm:"column:position_name"`
	}

	var nameRow positionNameRow
	if err := config.DB.Table("positions").
		Select("position_name").
		Where("position_id = ?", user.PositionID).
		Scan(&nameRow).Error; err != nil {
		return ""
	}

	if nameRow.Name.Valid {
		return strings.TrimSpace(nameRow.Name.String)
	}

	return ""
}

func lookupPositionFromUserID(userID int) string {
	if userID <= 0 {
		return ""
	}

	var user models.User
	if err := config.DB.
		Select("user_id", "position", "position_id").
		Preload("Position").
		Where("user_id = ?", userID).
		First(&user).Error; err != nil {
		return ""
	}

	return resolveApplicantPosition(&user)
}
