-- ตารางประวัติการรัน "ดึงข้อมูล conference" จาก Scopus Abstract Retrieval API
-- ใช้กับเอกสาร aggregation_type = 'Conference Proceeding' (เติมคอลัมน์ conference_* จาก migration 026)
-- prefix scopus_ ให้อยู่กลุ่มเดียวกับ scopus_batch_import_runs / scopus_api_import_jobs
CREATE TABLE IF NOT EXISTS scopus_conference_fetch_runs (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_type          VARCHAR(32)  NOT NULL COMMENT 'backfill | refresh',
  status            VARCHAR(32)  NOT NULL DEFAULT 'running' COMMENT 'running | success | failed',
  error_message     TEXT         DEFAULT NULL,
  documents_scanned INT          NOT NULL DEFAULT 0,
  documents_fetched INT          NOT NULL DEFAULT 0,
  skipped_existing  INT          NOT NULL DEFAULT 0,
  documents_failed  INT          NOT NULL DEFAULT 0,
  started_at        DATETIME     DEFAULT CURRENT_TIMESTAMP,
  finished_at       DATETIME     DEFAULT NULL,
  duration_seconds  DOUBLE       DEFAULT NULL,
  created_at        DATETIME     DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME     DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_scopus_conf_runs_status (status),
  KEY idx_scopus_conf_runs_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
