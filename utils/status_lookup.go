package utils

import (
	"fmt"
	"strings"
	"sync"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

const (
	// Canonical status codes mirror application_status.status_code.
	StatusCodePending         = "0" // อยู่ระหว่างการพิจารณา
	StatusCodeApproved        = "1" // อนุมัติ
	StatusCodeRejected        = "2" // ปฏิเสธ
	StatusCodeNeedsMoreInfo   = "3" // ต้องการข้อมูลเพิ่มเติม
	StatusCodeDraft           = "4" // ร่าง
	StatusCodeDeptHeadPending = "5" // อยู่ระหว่างการพิจารณาจากหัวหน้าสาขา
	StatusCodeAdminClosed     = "6" // ปิดทุน

	// Legacy aliases kept for backwards compatibility with existing controller logic.
	StatusCodeDeptHeadRecommended    = StatusCodeDeptHeadPending
	StatusCodeDeptHeadNotRecommended = StatusCodeRejected
)

var (
	statusCodeSynonyms = map[string][]string{
		StatusCodePending: {
			"0",
			"pending",
			"อยู่ระหว่างการพิจารณา",
		},
		StatusCodeApproved: {
			"1",
			"approved",
			"อนุมัติ",
		},
		StatusCodeRejected: {
			"2",
			"rejected",
			"ปฏิเสธ",
			"dept_head_rejected",
			"dept_head_not_recommended",
		},
		StatusCodeNeedsMoreInfo: {
			"3",
			"revision",
			"needs_more_info",
			"ต้องการข้อมูลเพิ่มเติม",
		},
		StatusCodeDraft: {
			"4",
			"draft",
			"ร่าง",
		},
		StatusCodeDeptHeadPending: {
			"5",
			"dept_head_pending",
			"department_pending",
			"อยู่ระหว่างการพิจารณาจากหัวหน้าสาขา",
			"dept_head_recommended",
			"dept_head_recommend",
		},
		StatusCodeAdminClosed: {
			"6",
			"admin_closed",
			"closed",
			"ปิดทุน",
		},
	}
	statusAliasToCanonical = buildStatusAliasMap()
)

func buildStatusAliasMap() map[string]string {
	aliasMap := make(map[string]string)
	for canonical, synonyms := range statusCodeSynonyms {
		canonicalKey := normalizeStatusCode(canonical)
		if canonicalKey != "" {
			aliasMap[canonicalKey] = canonical
		}
		for _, alias := range synonyms {
			if normalized := normalizeStatusCode(alias); normalized != "" {
				aliasMap[normalized] = canonical
			}
		}
	}
	return aliasMap
}

func normalizeStatusCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func canonicalStatusCode(code string) string {
	normalized := normalizeStatusCode(code)
	if canonical, ok := statusAliasToCanonical[normalized]; ok {
		return canonical
	}
	return normalized
}

func codeCandidates(code string) []string {
	canonical := canonicalStatusCode(code)
	seen := make(map[string]struct{})
	candidates := make([]string, 0, 1)

	add := func(value string) {
		key := normalizeStatusCode(value)
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, value)
	}

	add(canonical)

	if synonyms, ok := statusCodeSynonyms[canonical]; ok {
		for _, alias := range synonyms {
			add(alias)
		}
	} else if normalized := normalizeStatusCode(code); normalized != canonical {
		add(code)
	}

	return candidates
}

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

	for _, candidate := range codeCandidates(status.StatusCode) {
		key := normalizeStatusCode(candidate)
		if key == "" {
			continue
		}
		applicationStatusCache.byCode[key] = status
	}
}

func getCachedStatusByCode(code string) (models.ApplicationStatus, bool) {
	key := normalizeStatusCode(code)
	if key == "" {
		return models.ApplicationStatus{}, false
	}

	applicationStatusCache.RLock()
	defer applicationStatusCache.RUnlock()

	status, ok := applicationStatusCache.byCode[key]
	return status, ok && status.ApplicationStatusID != 0
}

func getCachedStatusByID(id int) (models.ApplicationStatus, bool) {
	applicationStatusCache.RLock()
	defer applicationStatusCache.RUnlock()

	status, ok := applicationStatusCache.byID[id]
	return status, ok && status.ApplicationStatusID != 0
}

func GetApplicationStatusByCode(code string) (models.ApplicationStatus, error) {
	candidates := codeCandidates(code)
	for _, candidate := range candidates {
		if status, ok := getCachedStatusByCode(candidate); ok {
			return status, nil
		}
	}

	var lastNotFound error
	for _, candidate := range candidates {
		var status models.ApplicationStatus
		err := config.DB.Where("status_code = ? AND (delete_at IS NULL)", candidate).First(&status).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				lastNotFound = err
				continue
			}
			return models.ApplicationStatus{}, err
		}

		cacheStatus(status)
		return status, nil
	}

	if lastNotFound != nil {
		return models.ApplicationStatus{}, fmt.Errorf("application status with code %s not found", code)
	}

	return models.ApplicationStatus{}, fmt.Errorf("application status with code %s not found", code)
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
	seen := make(map[int]struct{})
	for _, code := range codes {
		id, err := GetStatusIDByCode(code)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func StatusMatchesCodes(statusID int, codes ...string) (bool, error) {
	status, err := GetApplicationStatusByID(statusID)
	if err != nil {
		return false, err
	}
	statusKey := normalizeStatusCode(status.StatusCode)

	for _, code := range codes {
		for _, candidate := range codeCandidates(code) {
			if statusKey == normalizeStatusCode(candidate) {
				return true, nil
			}
		}
	}
	return false, nil
}

// IsSubmissionClosed reports whether the provided status represents an admin closed submission.
func IsSubmissionClosed(statusID int) (bool, error) {
	return StatusMatchesCodes(statusID, StatusCodeAdminClosed)
}

// EnsureStatusIn verifies a status matches at least one of the provided codes.
func EnsureStatusIn(statusID int, codes ...string) error {
	if len(codes) == 0 {
		return fmt.Errorf("no status codes provided")
	}
	ok, err := StatusMatchesCodes(statusID, codes...)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("status %d is not in allowed codes %v", statusID, codes)
	}
	return nil
}
