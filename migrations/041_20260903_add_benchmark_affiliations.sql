-- Store Scopus affiliations inside the isolated benchmark dataset.
-- This intentionally does not reference or modify the dashboard scopus_* tables.

CREATE TABLE IF NOT EXISTS scopus_benchmark_affiliations (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  afid            VARCHAR(32)  NOT NULL,
  name            TEXT         DEFAULT NULL,
  city            TEXT         DEFAULT NULL,
  country         TEXT         DEFAULT NULL,
  affiliation_url TEXT         DEFAULT NULL,
  created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_scopus_benchmark_affiliations_afid (afid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE scopus_benchmark_document_authors
  ADD COLUMN affiliation_id BIGINT UNSIGNED DEFAULT NULL AFTER author_seq,
  ADD KEY idx_scopus_benchmark_doc_authors_affiliation (affiliation_id);
