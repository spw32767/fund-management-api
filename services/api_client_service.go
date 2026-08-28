package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

// apiKeyRawPrefix is the human-visible marker on every raw key ("fund service key").
const apiKeyRawPrefix = "fsk_"

var (
	// ErrAPIKeyInvalid is returned when a presented key does not resolve to an active,
	// non-expired key belonging to an active client.
	ErrAPIKeyInvalid = errors.New("invalid api key")
	// ErrAPIClientNotFound is returned by management operations on a missing client.
	ErrAPIClientNotFound = errors.New("api client not found")
	// ErrAPIScopeNotFound is returned when an unknown scope code is granted to a client.
	ErrAPIScopeNotFound = errors.New("api scope not found")
)

// APIClientService owns creation/verification of external API clients and keys and writes
// the request audit log. It reuses the global config.DB like the other services.
type APIClientService struct {
	db *gorm.DB
}

// NewAPIClientService instantiates the service.
func NewAPIClientService(db *gorm.DB) *APIClientService {
	if db == nil {
		db = config.DB
	}
	return &APIClientService{db: db}
}

// HashAPIKey returns the lowercase hex SHA-256 of a raw key. This is what we store and what
// we look up by on each request — a direct hash lookup (not bcrypt) so verification is O(1).
func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

// GenerateAPIKey mints a new random raw key and returns (rawKey, prefix, hash). The raw key
// is returned to the caller exactly once and never stored; only the hash is persisted.
func GenerateAPIKey() (raw, prefix, hash string, err error) {
	buf := make([]byte, 32) // 256 bits of entropy
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	raw = apiKeyRawPrefix + base64.RawURLEncoding.EncodeToString(buf)
	// key_prefix column is varchar(16); a 12-char prefix is enough to identify a key in listings.
	prefix = raw
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	return raw, prefix, HashAPIKey(raw), nil
}

// VerifiedAPIKey bundles the resolved client, its granted scope codes, and the key record
// that authenticated the request.
type VerifiedAPIKey struct {
	Client *models.APIClient
	Key    *models.APIKey
	Scopes []string
}

// VerifyAPIKey resolves a raw key to its client + scopes, enforcing key status/expiry and
// client status. On success it best-effort updates last_used_at. Returns ErrAPIKeyInvalid
// for any failure so callers cannot distinguish "unknown" from "expired".
func (s *APIClientService) VerifyAPIKey(raw string) (*VerifiedAPIKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrAPIKeyInvalid
	}

	var key models.APIKey
	if err := s.db.Where("key_hash = ? AND status = ?", HashAPIKey(raw), "active").First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAPIKeyInvalid
		}
		return nil, err
	}

	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		return nil, ErrAPIKeyInvalid
	}

	var client models.APIClient
	if err := s.db.Where("id = ?", key.ClientID).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAPIKeyInvalid
		}
		return nil, err
	}
	if client.Status != "active" {
		return nil, ErrAPIKeyInvalid
	}

	scopes, err := s.scopesForClient(client.ID)
	if err != nil {
		return nil, err
	}

	// Best-effort: record last usage. A failure here must not block the request.
	now := time.Now()
	s.db.Model(&models.APIKey{}).Where("id = ?", key.ID).Update("last_used_at", now)

	return &VerifiedAPIKey{Client: &client, Key: &key, Scopes: scopes}, nil
}

// scopesForClient returns the scope codes granted to a client.
func (s *APIClientService) scopesForClient(clientID uint64) ([]string, error) {
	var codes []string
	err := s.db.Table("api_client_scopes AS acs").
		Select("sc.code").
		Joins("INNER JOIN api_scopes sc ON sc.id = acs.scope_id").
		Where("acs.client_id = ?", clientID).
		Pluck("sc.code", &codes).Error
	return codes, err
}

// --- Management operations (admin) ---

// CreateClient creates a client and grants the given scope codes atomically.
func (s *APIClientService) CreateClient(name string, description, contactEmail *string, scopeCodes []string) (*models.APIClient, []string, error) {
	client := &models.APIClient{
		Name:         strings.TrimSpace(name),
		Description:  description,
		ContactEmail: contactEmail,
		Status:       "active",
	}

	var granted []string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(client).Error; err != nil {
			return err
		}
		g, err := setClientScopes(tx, client.ID, scopeCodes)
		if err != nil {
			return err
		}
		granted = g
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return client, granted, nil
}

// setClientScopes replaces a client's scope grants with the given codes and returns the
// codes actually granted. Unknown scope codes cause ErrAPIScopeNotFound.
func setClientScopes(tx *gorm.DB, clientID uint64, scopeCodes []string) ([]string, error) {
	if err := tx.Where("client_id = ?", clientID).Delete(&models.APIClientScope{}).Error; err != nil {
		return nil, err
	}

	granted := make([]string, 0, len(scopeCodes))
	seen := map[string]struct{}{}
	for _, code := range scopeCodes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}

		var scope models.APIScope
		if err := tx.Where("code = ?", code).First(&scope).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrAPIScopeNotFound
			}
			return nil, err
		}
		if err := tx.Create(&models.APIClientScope{ClientID: clientID, ScopeID: scope.ID}).Error; err != nil {
			return nil, err
		}
		granted = append(granted, code)
	}
	return granted, nil
}

// UpdateClient updates mutable client fields and (when scopeCodes is non-nil) replaces its
// scope grants. A nil pointer field is left unchanged.
func (s *APIClientService) UpdateClient(clientID uint64, name, description, contactEmail, status *string, scopeCodes []string) (*models.APIClient, []string, error) {
	var client models.APIClient
	if err := s.db.Where("id = ?", clientID).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrAPIClientNotFound
		}
		return nil, nil, err
	}

	updates := map[string]interface{}{}
	if name != nil {
		updates["name"] = strings.TrimSpace(*name)
	}
	if description != nil {
		updates["description"] = description
	}
	if contactEmail != nil {
		updates["contact_email"] = contactEmail
	}
	if status != nil && (*status == "active" || *status == "disabled") {
		updates["status"] = *status
	}

	granted, err := s.scopesForClient(client.ID)
	if err != nil {
		return nil, nil, err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&models.APIClient{}).Where("id = ?", client.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		if scopeCodes != nil {
			g, err := setClientScopes(tx, client.ID, scopeCodes)
			if err != nil {
				return err
			}
			granted = g
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	if err := s.db.Where("id = ?", client.ID).First(&client).Error; err != nil {
		return nil, nil, err
	}
	return &client, granted, nil
}

// ClientWithScopes is a client plus its granted scope codes, for listing.
type ClientWithScopes struct {
	models.APIClient
	Scopes []string `json:"scopes"`
}

// ListClients returns all clients with their scope codes.
func (s *APIClientService) ListClients() ([]ClientWithScopes, error) {
	var clients []models.APIClient
	if err := s.db.Order("created_at DESC").Find(&clients).Error; err != nil {
		return nil, err
	}
	out := make([]ClientWithScopes, 0, len(clients))
	for _, c := range clients {
		scopes, err := s.scopesForClient(c.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, ClientWithScopes{APIClient: c, Scopes: scopes})
	}
	return out, nil
}

// GetClient returns one client with its scopes.
func (s *APIClientService) GetClient(clientID uint64) (*ClientWithScopes, error) {
	var client models.APIClient
	if err := s.db.Where("id = ?", clientID).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAPIClientNotFound
		}
		return nil, err
	}
	scopes, err := s.scopesForClient(client.ID)
	if err != nil {
		return nil, err
	}
	return &ClientWithScopes{APIClient: client, Scopes: scopes}, nil
}

// ListKeys returns the (non-secret) key records for a client.
func (s *APIClientService) ListKeys(clientID uint64) ([]models.APIKey, error) {
	var keys []models.APIKey
	err := s.db.Where("client_id = ?", clientID).Order("created_at DESC").Find(&keys).Error
	return keys, err
}

// IssuedKey is the one-time result of issuing a key: the plaintext value plus its record.
type IssuedKey struct {
	RawKey string        `json:"raw_key"`
	Key    models.APIKey `json:"key"`
}

// IssueKey mints a new key for a client and returns the raw value ONCE. Only the hash is stored.
func (s *APIClientService) IssueKey(clientID uint64, label *string, expiresAt *time.Time) (*IssuedKey, error) {
	var client models.APIClient
	if err := s.db.Where("id = ?", clientID).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAPIClientNotFound
		}
		return nil, err
	}

	raw, prefix, hash, err := GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	key := models.APIKey{
		ClientID:  clientID,
		Label:     label,
		KeyPrefix: prefix,
		KeyHash:   hash,
		Status:    "active",
		ExpiresAt: expiresAt,
	}
	if err := s.db.Create(&key).Error; err != nil {
		return nil, err
	}
	return &IssuedKey{RawKey: raw, Key: key}, nil
}

// RevokeKey marks a key revoked. It is idempotent and scoped to the owning client.
func (s *APIClientService) RevokeKey(clientID, keyID uint64) error {
	now := time.Now()
	res := s.db.Model(&models.APIKey{}).
		Where("id = ? AND client_id = ?", keyID, clientID).
		Updates(map[string]interface{}{"status": "revoked", "revoked_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAPIClientNotFound
	}
	return nil
}

// ListRequestLogs returns recent audit log rows for a client (newest first).
func (s *APIClientService) ListRequestLogs(clientID uint64, limit int) ([]models.APIRequestLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var logs []models.APIRequestLog
	err := s.db.Where("client_id = ?", clientID).Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// WriteRequestLog appends one audit row. Errors are swallowed by the caller (best effort);
// this method returns the error so tests can assert on it.
func (s *APIClientService) WriteRequestLog(entry *models.APIRequestLog) error {
	return s.db.Create(entry).Error
}
