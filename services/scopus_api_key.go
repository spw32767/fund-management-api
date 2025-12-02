package services

import (
	"context"
	"errors"
	"strings"

	"fund-management-api/models"

	"gorm.io/gorm"
)

func lookupScopusAPIKey(ctx context.Context, db *gorm.DB) (string, error) {
	keys := append([]string{scopusAPIKeyField}, scopusAPIKeyLegacyFields...)
	for _, key := range keys {
		var row models.ScopusConfig
		if err := db.WithContext(ctx).
			Where("`key` = ?", key).
			First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return "", err
		}
		if row.Value != nil && strings.TrimSpace(*row.Value) != "" {
			return strings.TrimSpace(*row.Value), nil
		}
	}
	return "", errors.New("scopus api key not configured")
}
