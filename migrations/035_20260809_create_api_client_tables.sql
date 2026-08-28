-- External / partner API subsystem.
-- Authenticates external systems via API keys (stored as SHA-256 hash, never plaintext),
-- authorized by scope, with per-request audit logging. Designed to host future external
-- APIs beyond the initial Scopus publications endpoint.

CREATE TABLE IF NOT EXISTS api_clients (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(255) NOT NULL,
  description TEXT DEFAULT NULL,
  contact_email VARCHAR(255) DEFAULT NULL,
  status ENUM('active','disabled') NOT NULL DEFAULT 'active',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_api_clients_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS api_scopes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code VARCHAR(128) NOT NULL,
  description VARCHAR(255) DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_api_scopes_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS api_client_scopes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  client_id BIGINT UNSIGNED NOT NULL,
  scope_id BIGINT UNSIGNED NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_api_client_scope (client_id, scope_id),
  KEY idx_api_client_scopes_scope (scope_id),
  CONSTRAINT fk_api_client_scopes_client FOREIGN KEY (client_id) REFERENCES api_clients (id) ON DELETE CASCADE,
  CONSTRAINT fk_api_client_scopes_scope FOREIGN KEY (scope_id) REFERENCES api_scopes (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Only the SHA-256 hash of a key is stored. Multiple active keys per client are allowed
-- so a key can be rotated (issue new, revoke old) with zero downtime.
CREATE TABLE IF NOT EXISTS api_keys (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  client_id BIGINT UNSIGNED NOT NULL,
  label VARCHAR(255) DEFAULT NULL,
  key_prefix VARCHAR(16) NOT NULL,
  key_hash CHAR(64) NOT NULL,
  status ENUM('active','revoked') NOT NULL DEFAULT 'active',
  expires_at DATETIME DEFAULT NULL,
  last_used_at DATETIME DEFAULT NULL,
  revoked_at DATETIME DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_api_keys_hash (key_hash),
  KEY idx_api_keys_client (client_id),
  KEY idx_api_keys_prefix (key_prefix),
  CONSTRAINT fk_api_keys_client FOREIGN KEY (client_id) REFERENCES api_clients (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Append-only audit log. Intentionally has no FK to api_clients/api_keys so that a log
-- insert never fails and history survives client/key deletion. The API key value is
-- never stored here.
CREATE TABLE IF NOT EXISTS api_request_logs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  client_id BIGINT UNSIGNED DEFAULT NULL,
  api_key_id BIGINT UNSIGNED DEFAULT NULL,
  method VARCHAR(8) NOT NULL,
  endpoint VARCHAR(255) NOT NULL,
  requested_user_ids TEXT DEFAULT NULL,
  year_from SMALLINT UNSIGNED DEFAULT NULL,
  year_to SMALLINT UNSIGNED DEFAULT NULL,
  http_status SMALLINT UNSIGNED NOT NULL,
  ip VARCHAR(64) DEFAULT NULL,
  latency_ms INT UNSIGNED DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_api_request_logs_client (client_id),
  KEY idx_api_request_logs_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed the scope catalog.
INSERT INTO api_scopes (code, description)
VALUES ('scopus.publications.read', 'Read Scopus publications of faculty members via the external API')
ON DUPLICATE KEY UPDATE description = VALUES(description);

-- Seed the admin permission that gates management of API clients/keys (mirrors migration 003).
INSERT INTO permissions (code, resource, action, description)
VALUES ('api.clients.manage', 'api_clients', 'manage', 'Manage external API clients and keys')
ON DUPLICATE KEY UPDATE
  resource = VALUES(resource),
  action = VALUES(action),
  description = VALUES(description),
  update_at = CURRENT_TIMESTAMP;

INSERT INTO role_permissions (role_id, permission_id)
SELECT 3, p.permission_id FROM permissions p WHERE p.code = 'api.clients.manage'
ON DUPLICATE KEY UPDATE update_at = CURRENT_TIMESTAMP;
