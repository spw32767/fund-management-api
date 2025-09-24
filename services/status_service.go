package services

import (
	"errors"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"strings"
	"sync"
	"time"
)

const (
	// Dept head workflow labels (exact match with application_status.status_name)
	StatusDeptHeadPendingLabel     = "อยู่ระหว่างการพิจารณาจากหัวหน้าสาขา"
	StatusDeptHeadRecommendedLabel = "เห็นควรพิจารณาจากหัวหน้าสาขา"
	StatusDeptHeadRejectedLabel    = "ไม่เห็นควรพิจารณา"
)

var (
	statusCacheMu sync.RWMutex
	statusCache   *statusCacheEntry
	statusTTL     = 5 * time.Minute
)

type statusCacheEntry struct {
	statuses  []models.ApplicationStatus
	byName    map[string]models.ApplicationStatus
	fetchedAt time.Time
}

func loadStatuses(force bool) (*statusCacheEntry, error) {
	statusCacheMu.RLock()
	cached := statusCache
	statusCacheMu.RUnlock()

	if cached != nil && !force && time.Since(cached.fetchedAt) < statusTTL {
		return cached, nil
	}

	statusCacheMu.Lock()
	defer statusCacheMu.Unlock()

	if statusCache != nil && !force && time.Since(statusCache.fetchedAt) < statusTTL {
		return statusCache, nil
	}

	var rows []models.ApplicationStatus
	if err := config.DB.Where("delete_at IS NULL").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load application statuses: %w", err)
	}

	byName := make(map[string]models.ApplicationStatus, len(rows))
	for _, status := range rows {
		if status.StatusName == "" {
			continue
		}
		byName[strings.TrimSpace(status.StatusName)] = status
	}

	entry := &statusCacheEntry{
		statuses:  rows,
		byName:    byName,
		fetchedAt: time.Now(),
	}
	statusCache = entry
	return entry, nil
}

// ClearStatusCache invalidates the in-memory status cache.
func ClearStatusCache() {
	statusCacheMu.Lock()
	defer statusCacheMu.Unlock()
	statusCache = nil
}

// GetStatuses returns all statuses with caching support.
func GetStatuses() ([]models.ApplicationStatus, error) {
	entry, err := loadStatuses(false)
	if err != nil {
		return nil, err
	}
	return entry.statuses, nil
}

// GetStatusByName returns an application status that matches the exact status_name.
func GetStatusByName(name string) (*models.ApplicationStatus, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.New("status name is required")
	}

	entry, err := loadStatuses(false)
	if err != nil {
		return nil, err
	}

	if status, ok := entry.byName[trimmed]; ok {
		return &status, nil
	}

	// Force refresh cache once before giving up
	entry, err = loadStatuses(true)
	if err != nil {
		return nil, err
	}

	if status, ok := entry.byName[trimmed]; ok {
		return &status, nil
	}

	return nil, fmt.Errorf("status '%s' not found", trimmed)
}

// GetStatusIDByName resolves the application_status_id for the given status_name.
func GetStatusIDByName(name string) (int, error) {
	status, err := GetStatusByName(name)
	if err != nil {
		return 0, err
	}
	return status.ApplicationStatusID, nil
}

// GetStatusIDsByNames resolves multiple status IDs keyed by their status_name.
func GetStatusIDsByNames(names []string) (map[string]int, error) {
	result := make(map[string]int, len(names))
	if len(names) == 0 {
		return result, nil
	}

	entry, err := loadStatuses(false)
	if err != nil {
		return nil, err
	}

	missing := make([]string, 0)
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if status, ok := entry.byName[name]; ok {
			result[name] = status.ApplicationStatusID
			continue
		}
		missing = append(missing, name)
	}

	if len(missing) == 0 {
		return result, nil
	}

	entry, err = loadStatuses(true)
	if err != nil {
		return nil, err
	}

	unresolved := make([]string, 0)
	for _, name := range missing {
		if status, ok := entry.byName[name]; ok {
			result[name] = status.ApplicationStatusID
		} else {
			unresolved = append(unresolved, name)
		}
	}

	if len(unresolved) > 0 {
		return nil, fmt.Errorf("missing statuses: %s", strings.Join(unresolved, ", "))
	}

	return result, nil
}
