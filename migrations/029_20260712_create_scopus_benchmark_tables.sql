-- ระบบเทียบผลงาน Scopus ระดับคณะ vs มหาวิทยาลัย (KKU) vs ประเทศ (Thailand) เฉพาะ Computer Science
-- แยกตารางออกจาก scopus_documents ทั้งหมด เพื่อไม่ให้ข้อมูลระดับ uni/country รั่วเข้า
-- unified_search views / public publication search / admin ListAll (ที่อ่าน scopus_documents ทั้งตาราง)
-- prefix scopus_benchmark_ ให้อยู่กลุ่มเดียวกัน

-- 1) ทะเบียน scope ที่จะ harvest
CREATE TABLE IF NOT EXISTS scopus_benchmark_scopes (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code          VARCHAR(64)  NOT NULL COMMENT 'university_kku | country_thailand | custom',
  label         VARCHAR(255) NOT NULL,
  level         VARCHAR(32)  NOT NULL COMMENT 'university | country | custom',
  af_id         VARCHAR(32)  DEFAULT NULL COMMENT 'Scopus Affiliation ID (สำหรับ level=university)',
  affil_country VARCHAR(64)  DEFAULT NULL COMMENT 'ชื่อประเทศสำหรับ AFFILCOUNTRY (สำหรับ level=country)',
  subject_area  VARCHAR(16)  NOT NULL DEFAULT 'COMP' COMMENT 'SUBJAREA code เช่น COMP',
  extra_query   TEXT         DEFAULT NULL COMMENT 'เงื่อนไข Scopus เพิ่มเติม (optional)',
  active        TINYINT(1)   NOT NULL DEFAULT 1,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_scopus_benchmark_scopes_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2) เอกสาร (mirror ของ scopus_documents + เก็บ raw ครบ) — dedup ด้วย eid
CREATE TABLE IF NOT EXISTS scopus_benchmark_documents (
  id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  eid                 VARCHAR(64)  NOT NULL,
  scopus_id           VARCHAR(64)  DEFAULT NULL,
  scopus_link         TEXT         DEFAULT NULL,
  title               TEXT         DEFAULT NULL,
  abstract            LONGTEXT     DEFAULT NULL,
  aggregation_type    VARCHAR(32)  DEFAULT NULL,
  subtype             VARCHAR(32)  DEFAULT NULL,
  subtype_description  TEXT         DEFAULT NULL,
  source_id           VARCHAR(32)  DEFAULT NULL,
  publication_name    TEXT         DEFAULT NULL,
  issn                VARCHAR(32)  DEFAULT NULL,
  eissn               VARCHAR(32)  DEFAULT NULL,
  isbn                VARCHAR(64)  DEFAULT NULL,
  volume              VARCHAR(32)  DEFAULT NULL,
  issue               VARCHAR(32)  DEFAULT NULL,
  page_range          VARCHAR(64)  DEFAULT NULL,
  article_number      VARCHAR(64)  DEFAULT NULL,
  cover_date          DATE         DEFAULT NULL,
  cover_display_date  TEXT         DEFAULT NULL,
  doi                 VARCHAR(255) DEFAULT NULL,
  pii                 VARCHAR(64)  DEFAULT NULL,
  citedby_count       INT          DEFAULT NULL,
  openaccess          TINYINT      DEFAULT NULL,
  openaccess_flag     TINYINT(1)   DEFAULT NULL,
  authkeywords        LONGTEXT     DEFAULT NULL,
  fund_acr            TEXT         DEFAULT NULL,
  fund_sponsor        TEXT         DEFAULT NULL,
  pub_year            SMALLINT     DEFAULT NULL COMMENT 'ปีจาก cover_date (denormalize)',
  raw_json            LONGTEXT     DEFAULT NULL COMMENT 'entry ดิบเต็มจาก Search API (view=COMPLETE)',
  first_seen_at       DATETIME     DEFAULT NULL,
  last_seen_at        DATETIME     DEFAULT NULL,
  created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_scopus_benchmark_documents_eid (eid),
  KEY idx_scopus_benchmark_documents_pub_year (pub_year),
  KEY idx_scopus_benchmark_documents_source_id (source_id),
  KEY idx_scopus_benchmark_documents_issn (issn),
  KEY idx_scopus_benchmark_documents_aggregation_type (aggregation_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3) ผู้เขียน (จาก author list ของ view=COMPLETE) — dedup ด้วย scopus_author_id
CREATE TABLE IF NOT EXISTS scopus_benchmark_authors (
  id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  scopus_author_id VARCHAR(32)  NOT NULL,
  full_name        VARCHAR(255) DEFAULT NULL,
  given_name       VARCHAR(255) DEFAULT NULL,
  surname          VARCHAR(255) DEFAULT NULL,
  initials         VARCHAR(64)  DEFAULT NULL,
  author_url       TEXT         DEFAULT NULL,
  created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_scopus_benchmark_authors_author_id (scopus_author_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 4) ลิงก์ doc <-> author + flag ว่าเป็นอาจารย์ในระบบเรา (ใช้ derive จำนวนระดับคณะ)
CREATE TABLE IF NOT EXISTS scopus_benchmark_document_authors (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  document_id BIGINT UNSIGNED NOT NULL,
  author_id   BIGINT UNSIGNED NOT NULL,
  author_seq  INT          DEFAULT NULL,
  is_faculty  TINYINT(1)   NOT NULL DEFAULT 0 COMMENT '1 = authid ตรงกับ users.scopus_id ของอาจารย์ในระบบ',
  PRIMARY KEY (id),
  UNIQUE KEY uq_scopus_benchmark_doc_authors (document_id, author_id),
  KEY idx_scopus_benchmark_doc_authors_author (author_id),
  KEY idx_scopus_benchmark_doc_authors_is_faculty (is_faculty)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 5) membership: เอกสารนี้อยู่ใน scope ไหนบ้าง (uni/country ซ้อนกันได้)
CREATE TABLE IF NOT EXISTS scopus_benchmark_document_scopes (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  document_id BIGINT UNSIGNED NOT NULL,
  scope_id    BIGINT UNSIGNED NOT NULL,
  pub_year    SMALLINT     DEFAULT NULL,
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_scopus_benchmark_doc_scopes (document_id, scope_id),
  KEY idx_scopus_benchmark_doc_scopes_scope_year (scope_id, pub_year)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 6) ประวัติการรัน harvest/count
CREATE TABLE IF NOT EXISTS scopus_benchmark_harvest_runs (
  id                     BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  scope_id               BIGINT UNSIGNED DEFAULT NULL,
  run_type               VARCHAR(32)  NOT NULL COMMENT 'count | harvest',
  year_from              SMALLINT     DEFAULT NULL,
  year_to                SMALLINT     DEFAULT NULL,
  status                 VARCHAR(32)  NOT NULL DEFAULT 'running' COMMENT 'running | success | failed',
  total_results_reported INT          DEFAULT NULL,
  pages_fetched          INT          NOT NULL DEFAULT 0,
  documents_upserted     INT          NOT NULL DEFAULT 0,
  requests_made          INT          NOT NULL DEFAULT 0,
  cursor_state           TEXT         DEFAULT NULL COMMENT 'cursor ล่าสุดสำหรับ resume',
  error_message          TEXT         DEFAULT NULL,
  started_at             DATETIME     DEFAULT CURRENT_TIMESTAMP,
  finished_at            DATETIME     DEFAULT NULL,
  duration_seconds       DOUBLE       DEFAULT NULL,
  created_at             DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at             DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_scopus_benchmark_runs_scope (scope_id),
  KEY idx_scopus_benchmark_runs_status (status),
  KEY idx_scopus_benchmark_runs_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 7) snapshot จำนวนต่อ (scope, ปี) ณ เวลาหนึ่ง — สำหรับกราฟเทรนด์เทียบ
CREATE TABLE IF NOT EXISTS scopus_benchmark_count_snapshots (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  scope_id      BIGINT UNSIGNED NOT NULL,
  subject_area  VARCHAR(16)  NOT NULL DEFAULT 'COMP',
  pub_year      SMALLINT     DEFAULT NULL COMMENT 'NULL = รวมทุกปี',
  total_results INT          NOT NULL,
  captured_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_scopus_benchmark_snapshots_scope_year (scope_id, pub_year),
  KEY idx_scopus_benchmark_snapshots_captured (captured_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- seed 2 scope เริ่มต้น (af_id ของ KKU เติมภายหลังผ่าน affiliation lookup)
INSERT INTO scopus_benchmark_scopes (code, label, level, af_id, affil_country, subject_area, active)
SELECT * FROM (SELECT 'university_kku' AS code, 'Khon Kaen University' AS label, 'university' AS level,
                      NULL AS af_id, NULL AS affil_country, 'COMP' AS subject_area, 1 AS active) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM scopus_benchmark_scopes WHERE code = 'university_kku');

INSERT INTO scopus_benchmark_scopes (code, label, level, af_id, affil_country, subject_area, active)
SELECT * FROM (SELECT 'country_thailand' AS code, 'Thailand' AS label, 'country' AS level,
                      NULL AS af_id, 'Thailand' AS affil_country, 'COMP' AS subject_area, 1 AS active) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM scopus_benchmark_scopes WHERE code = 'country_thailand');
