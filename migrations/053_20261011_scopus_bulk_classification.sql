-- Run after 046, against the fund-management database.
ALTER TABLE scopus_documents
  ADD COLUMN category BIGINT UNSIGNED DEFAULT NULL,
  ADD COLUMN classification_confidence ENUM('High','Medium','Low','Preface') DEFAULT NULL,
  ADD COLUMN classification_model VARCHAR(128) DEFAULT NULL,
  ADD COLUMN classification_taxonomy_version VARCHAR(64) DEFAULT NULL,
  ADD COLUMN classified_at DATETIME DEFAULT NULL,
  ADD KEY idx_scopus_documents_paper_category (category),
  ADD CONSTRAINT fk_scopus_documents_paper_category FOREIGN KEY (category)
    REFERENCES paper_categories(category_id) ON UPDATE CASCADE ON DELETE SET NULL;

CREATE TABLE paper_classification_runs (
  run_id CHAR(36) NOT NULL PRIMARY KEY,
  source VARCHAR(16) NOT NULL,
  publication_year INT DEFAULT NULL,
  scope VARCHAR(16) NOT NULL,
  taxonomy_version VARCHAR(64) NOT NULL,
  status VARCHAR(20) NOT NULL,
  stop_requested TINYINT(1) NOT NULL DEFAULT 0,
  user_id INT NOT NULL,
  total INT NOT NULL DEFAULT 0,
  completed INT NOT NULL DEFAULT 0,
  preface INT NOT NULL DEFAULT 0,
  failed INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at DATETIME DEFAULT NULL,
  completed_at DATETIME DEFAULT NULL,
  KEY idx_classification_runs_status (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE paper_classification_run_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  run_id CHAR(36) NOT NULL,
  document_id BIGINT UNSIGNED NOT NULL,
  title_snapshot TEXT DEFAULT NULL,
  input_hash CHAR(64) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  prior_json TEXT DEFAULT NULL,
  result_json TEXT DEFAULT NULL,
  error_message TEXT DEFAULT NULL,
  started_at DATETIME DEFAULT NULL,
  completed_at DATETIME DEFAULT NULL,
  UNIQUE KEY uq_classification_run_document (run_id, document_id),
  KEY idx_classification_items_status (run_id, status),
  CONSTRAINT fk_classification_items_run FOREIGN KEY (run_id)
    REFERENCES paper_classification_runs(run_id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- A transactional singleton prevents concurrent runs across API processes.
CREATE TABLE paper_classification_control (
  id TINYINT NOT NULL PRIMARY KEY,
  active_run_id CHAR(36) DEFAULT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO paper_classification_control (id) VALUES (1);
