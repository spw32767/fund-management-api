-- Core-only derived country evidence. Apply before deploying the new ingest binary.
-- No data backfill and no changes to legacy author links or dashboard cohort.
CREATE TABLE IF NOT EXISTS scopus_document_affiliations (
 document_id BIGINT UNSIGNED NOT NULL,
 afid VARCHAR(32) COLLATE utf8mb4_bin NOT NULL,
 provenance VARCHAR(32) NOT NULL,
 payload_country TEXT NOT NULL,
 PRIMARY KEY(document_id,afid), KEY idx_core_affiliation_reverse(afid,document_id),
 CONSTRAINT fk_core_affiliation_document FOREIGN KEY(document_id) REFERENCES scopus_documents(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS scopus_document_countries (
 document_id BIGINT UNSIGNED NOT NULL,
 country_key VARCHAR(96) COLLATE utf8mb4_bin NOT NULL,
 country_name VARCHAR(128) NOT NULL,
 provenance VARCHAR(32) NOT NULL,
 PRIMARY KEY(document_id,country_key), KEY idx_core_country_reverse(country_key,document_id),
 CONSTRAINT fk_core_country_document FOREIGN KEY(document_id) REFERENCES scopus_documents(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS scopus_document_insights (
 document_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
 international_collaboration TINYINT(1) NULL,
 affiliations_complete TINYINT(1) NOT NULL DEFAULT 0,
 countries_complete TINYINT(1) NOT NULL DEFAULT 0,
 status VARCHAR(32) NOT NULL,
 reasons_json LONGTEXT NOT NULL,
 diagnostics_json LONGTEXT NOT NULL,
 provenance VARCHAR(32) NOT NULL,
 normalizer_version VARCHAR(32) NOT NULL,
 payload_hash CHAR(64) NOT NULL,
 catalogue_hash CHAR(64) NOT NULL,
 checked_at DATETIME(6) NOT NULL,
 KEY idx_core_insight_status(status,document_id),
 CONSTRAINT fk_core_insight_document FOREIGN KEY(document_id) REFERENCES scopus_documents(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Single-statement triggers need no client-specific DELIMITER. All catalogue writers
-- invalidate affected documents atomically, including unrecognized/unmapped AFIDs.
-- Old country rows are retained as evidence but MUST be ignored while status is dirty.
CREATE TABLE IF NOT EXISTS scopus_country_catalogue_guard (
 id TINYINT UNSIGNED NOT NULL PRIMARY KEY, revision BIGINT UNSIGNED NOT NULL DEFAULT 0
) ENGINE=InnoDB;
INSERT IGNORE INTO scopus_country_catalogue_guard(id,revision) VALUES(1,0);

-- BEFORE guards serialize catalogue mutation with normalization, including absent
-- AFIDs whose padded/cased spellings may not occupy the same unique-index gap.
DROP TRIGGER IF EXISTS scopus_core_catalogue_guard_insert;
CREATE TRIGGER scopus_core_catalogue_guard_insert BEFORE INSERT ON scopus_affiliations
 FOR EACH ROW UPDATE scopus_country_catalogue_guard SET revision=revision+1 WHERE id=1;
DROP TRIGGER IF EXISTS scopus_core_catalogue_guard_update;
CREATE TRIGGER scopus_core_catalogue_guard_update BEFORE UPDATE ON scopus_affiliations
 FOR EACH ROW UPDATE scopus_country_catalogue_guard SET revision=revision+1 WHERE id=1
 AND (NOT (BINARY OLD.country <=> BINARY NEW.country) OR NOT (BINARY OLD.afid <=> BINARY NEW.afid));
DROP TRIGGER IF EXISTS scopus_core_catalogue_guard_delete;
CREATE TRIGGER scopus_core_catalogue_guard_delete BEFORE DELETE ON scopus_affiliations
 FOR EACH ROW UPDATE scopus_country_catalogue_guard SET revision=revision+1 WHERE id=1;

DROP TRIGGER IF EXISTS scopus_core_country_catalogue_insert;
CREATE TRIGGER scopus_core_country_catalogue_insert AFTER INSERT ON scopus_affiliations
 FOR EACH ROW UPDATE scopus_document_insights i
 JOIN scopus_document_affiliations a ON a.document_id=i.document_id
 SET i.international_collaboration=NULL,i.countries_complete=0,i.status='dirty_catalogue'
 WHERE a.afid=LOWER(TRIM(NEW.afid));

DROP TRIGGER IF EXISTS scopus_core_country_catalogue_update;
CREATE TRIGGER scopus_core_country_catalogue_update AFTER UPDATE ON scopus_affiliations
 FOR EACH ROW UPDATE scopus_document_insights i
 JOIN scopus_document_affiliations a ON a.document_id=i.document_id
 SET i.international_collaboration=NULL,i.countries_complete=0,i.status='dirty_catalogue'
 WHERE a.afid IN (LOWER(TRIM(OLD.afid)),LOWER(TRIM(NEW.afid)))
 AND (NOT (BINARY OLD.country <=> BINARY NEW.country) OR NOT (BINARY OLD.afid <=> BINARY NEW.afid));

DROP TRIGGER IF EXISTS scopus_core_country_catalogue_delete;
CREATE TRIGGER scopus_core_country_catalogue_delete AFTER DELETE ON scopus_affiliations
 FOR EACH ROW UPDATE scopus_document_insights i
 JOIN scopus_document_affiliations a ON a.document_id=i.document_id
 SET i.international_collaboration=NULL,i.countries_complete=0,i.status='dirty_catalogue'
 WHERE a.afid=LOWER(TRIM(OLD.afid));

-- Also protect direct SQL/import and future non-ingest raw-payload writers.
-- Metadata gates let dashboard readers check freshness without parsing JSON.
DROP TRIGGER IF EXISTS scopus_core_payload_update;
CREATE TRIGGER scopus_core_payload_update AFTER UPDATE ON scopus_documents
 FOR EACH ROW UPDATE scopus_document_insights
 SET international_collaboration=NULL,affiliations_complete=0,countries_complete=0,status='dirty_payload'
 WHERE document_id=NEW.id AND NOT (BINARY OLD.raw_json <=> BINARY NEW.raw_json);
