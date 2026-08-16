-- Log tables สำหรับ subsystem ดึง h-index (scopus_author_metrics)
-- ให้ครบมาตรฐานเดียวกับ ingest ตัวอื่น: runs summary (แบบ scholar_import_runs / scopus_batch_import_runs)
-- + per-request log (แบบ scopus_api_requests) ไว้ audit โควตา Author API (5,000/สัปดาห์)

-- 1) สรุปต่อการรัน 1 ครั้ง
CREATE TABLE IF NOT EXISTS scopus_author_metrics_runs (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  trigger_source    VARCHAR(64) NOT NULL DEFAULT 'unknown' COMMENT 'cli | admin_ui | schedule | ...',
  status            ENUM('running','success','failed') NOT NULL DEFAULT 'running',
  error_message     TEXT         DEFAULT NULL,
  users_processed   INT          NOT NULL DEFAULT 0,
  users_with_errors INT          NOT NULL DEFAULT 0,
  metrics_upserted  INT          NOT NULL DEFAULT 0,
  not_found         INT          NOT NULL DEFAULT 0,
  api_calls         INT          NOT NULL DEFAULT 0 COMMENT 'จำนวน request ที่ยิงไป Author API (ไว้ดูโควตา)',
  started_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at       DATETIME     DEFAULT NULL,
  duration_seconds  DOUBLE       DEFAULT NULL,
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_scopus_author_metrics_runs_status (status),
  KEY idx_scopus_author_metrics_runs_started (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2) log ทุก HTTP request ที่ยิงไป Author Retrieval API
CREATE TABLE IF NOT EXISTS scopus_author_metric_requests (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_id            BIGINT UNSIGNED DEFAULT NULL COMMENT 'อ้าง scopus_author_metrics_runs.id (nullable สำหรับ single run)',
  user_id           INT          DEFAULT NULL,
  scopus_author_id  VARCHAR(32)  NOT NULL,
  http_method       VARCHAR(8)   NOT NULL,
  endpoint          TEXT         NOT NULL,
  response_status   INT          DEFAULT NULL,
  response_time_ms  INT          DEFAULT NULL,
  h_index           INT          DEFAULT NULL COMMENT 'ค่าที่ parse ได้จาก response (ไว้ดูย้อนหลังเร็ว ๆ)',
  error_message     TEXT         DEFAULT NULL,
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_scopus_author_metric_requests_run (run_id),
  KEY idx_scopus_author_metric_requests_author (scopus_author_id),
  KEY idx_scopus_author_metric_requests_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
