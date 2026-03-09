-- KKU SSONext support
-- 1) Create auth_identities table (idempotent)
-- 2) Add users.last_login_at column (idempotent)

SET @schema_name := DATABASE();

SET @auth_identities_table_exists := (
  SELECT COUNT(*)
  FROM information_schema.tables
  WHERE table_schema = @schema_name
    AND table_name = 'auth_identities'
);

SET @create_auth_identities_sql := IF(
  @auth_identities_table_exists = 0,
  'CREATE TABLE auth_identities (
    identity_id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    provider VARCHAR(50) NOT NULL DEFAULT ''kku_sso'',
    provider_subject VARCHAR(191) NULL,
    email_at_provider VARCHAR(255) NULL,
    raw_claims JSON NULL,
    last_login_at DATETIME NULL,
    create_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    update_at DATETIME NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    delete_at DATETIME NULL,
    INDEX idx_user_provider (user_id, provider),
    UNIQUE KEY uq_provider_subject (provider, provider_subject),
    CONSTRAINT fk_auth_identities_user_id FOREIGN KEY (user_id) REFERENCES users(user_id)
  ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci',
  'SELECT 1'
);

PREPARE stmt_create_auth_identities FROM @create_auth_identities_sql;
EXECUTE stmt_create_auth_identities;
DEALLOCATE PREPARE stmt_create_auth_identities;

SET @users_last_login_at_exists := (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'users'
    AND column_name = 'last_login_at'
);

SET @add_users_last_login_at_sql := IF(
  @users_last_login_at_exists = 0,
  'ALTER TABLE users ADD COLUMN last_login_at DATETIME NULL',
  'SELECT 1'
);

PREPARE stmt_add_users_last_login_at FROM @add_users_last_login_at_sql;
EXECUTE stmt_add_users_last_login_at;
DEALLOCATE PREPARE stmt_add_users_last_login_at;
