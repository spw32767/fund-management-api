package utils

import (
	"fmt"
	"sync"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

const (
	StatusCodePending                = "0"
	StatusCodeApproved               = "1"
	StatusCodeRejected               = "2"
	StatusCodeNeedsMoreInfo          = "3"
	StatusCodeDraft                  = "4"
	StatusCodeDeptHeadPending        = "5"
	StatusCodeDeptHeadRecommended    = "6"
	StatusCodeDeptHeadNotRecommended = "7"
)

type statusCache struct {
	sync.RWMutex
	byCode map[string]models.ApplicationStatus
	byID   map[int]models.ApplicationStatus
}

var applicationStatusCache = statusCache{
	byCode: make(map[string]models.ApplicationStatus),
	byID:   make(map[int]models.ApplicationStatus),
}

func cacheStatus(status models.ApplicationStatus) {
	applicationStatusCache.Lock()
	defer applicationStatusCache.Unlock()

	if status.ApplicationStatusID != 0 {
		applicationStatusCache.byID[status.ApplicationStatusID] = status
	}
	if status.StatusCode != "" {
		applicationStatusCache.byCode[status.StatusCode] = status
	}
}

func getCachedStatusByCode(code string) (models.ApplicationStatus, bool) {
	applicationStatusCache.RLock()
	defer applicationStatusCache.RUnlock()

	status, ok := applicationStatusCache.byCode[code]
	return status, ok && status.ApplicationStatusID != 0
}

func getCachedStatusByID(id int) (models.ApplicationStatus, bool) {
	applicationStatusCache.RLock()
	defer applicationStatusCache.RUnlock()

	status, ok := applicationStatusCache.byID[id]
	return status, ok && status.ApplicationStatusID != 0
}

func GetApplicationStatusByCode(code string) (models.ApplicationStatus, error) {
	if status, ok := getCachedStatusByCode(code); ok {
		return status, nil
	}

	var status models.ApplicationStatus
	err := config.DB.Where("status_code = ? AND (delete_at IS NULL)", code).First(&status).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return models.ApplicationStatus{}, fmt.Errorf("application status with code %s not found", code)
		}
		return models.ApplicationStatus{}, err
	}

	cacheStatus(status)
	return status, nil
}

func GetApplicationStatusByID(id int) (models.ApplicationStatus, error) {
	if status, ok := getCachedStatusByID(id); ok {
		return status, nil
	}

	var status models.ApplicationStatus
	err := config.DB.Where("application_status_id = ? AND (delete_at IS NULL)", id).First(&status).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return models.ApplicationStatus{}, fmt.Errorf("application status with id %d not found", id)
		}
		return models.ApplicationStatus{}, err
	}

	cacheStatus(status)
	return status, nil
}

func GetStatusIDByCode(code string) (int, error) {
	status, err := GetApplicationStatusByCode(code)
	if err != nil {
		return 0, err
	}
	return status.ApplicationStatusID, nil
}

func GetStatusIDsByCodes(codes ...string) ([]int, error) {
	ids := make([]int, 0, len(codes))
	for _, code := range codes {
		id, err := GetStatusIDByCode(code)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func StatusMatchesCodes(statusID int, codes ...string) (bool, error) {
	status, err := GetApplicationStatusByID(statusID)
	if err != nil {
		return false, err
	}
	for _, code := range codes {
		if status.StatusCode == code {
			return true, nil
		}
	}
	return false, nil
}
