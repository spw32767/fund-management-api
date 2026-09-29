-- Manual XML author-role backfill history for the Academic Data Import page.
-- A nullable unique slot prevents two web runs from starting concurrently.
CREATE TABLE IF NOT EXISTS scopus_author_role_runs (
  id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_type           VARCHAR(32) NOT NULL COMMENT 'backfill | retry_review | refresh',
  status             VARCHAR(32) NOT NULL DEFAULT 'running' COMMENT 'running | success | partial | failed',
  active_slot        TINYINT NULL DEFAULT NULL,
  eligible           INT NOT NULL DEFAULT 0,
  selected           INT NOT NULL DEFAULT 0,
  skipped_existing   INT NOT NULL DEFAULT 0,
  fetched            INT NOT NULL DEFAULT 0,
  complete           INT NOT NULL DEFAULT 0,
  no_correspondence  INT NOT NULL DEFAULT 0,
  needs_review       INT NOT NULL DEFAULT 0,
  failed             INT NOT NULL DEFAULT 0,
  error_message      TEXT DEFAULT NULL,
  started_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
  finished_at        DATETIME DEFAULT NULL,
  created_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_scopus_author_role_active (active_slot),
  KEY idx_scopus_author_role_started (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
