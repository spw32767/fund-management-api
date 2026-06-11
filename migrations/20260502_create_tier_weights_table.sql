CREATE TABLE IF NOT EXISTS ranking_sources (
    source_id    INT AUTO_INCREMENT PRIMARY KEY,
    source_code  VARCHAR(50) NOT NULL UNIQUE,
    source_name  VARCHAR(255) NOT NULL,
    description  TEXT NULL,
    is_active    TINYINT(1) NOT NULL DEFAULT 1,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    deleted_at   TIMESTAMP NULL DEFAULT NULL    --เพิ่ม
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ranking_tier_weights (
    tier_weight_id  INT AUTO_INCREMENT PRIMARY KEY,
    source_id       INT NOT NULL,
    tier_code       VARCHAR(50) NOT NULL,
    tier_name       VARCHAR(255) NOT NULL,
    description     TEXT NULL,
    thai_description TEXT NULL,            -- เพิ่ม
    weight          DECIMAL(5,2) NOT NULL,
    sort_order      INT NOT NULL DEFAULT 0,
    is_active       TINYINT(1) NOT NULL DEFAULT 1,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP NULL DEFAULT NULL,   --เพิ่ม

    UNIQUE KEY uq_ranking_tier_weights_source_code (source_id, tier_code),
    CONSTRAINT fk_ranking_tier_weights_source
        FOREIGN KEY (source_id) REFERENCES ranking_sources(source_id)
        ON UPDATE CASCADE
        ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO ranking_sources (source_code, source_name, description)
VALUES
('scopus', 'Scopus', 'Scopus journal and conference ranking source'),
('tci', 'TCI', 'Thai-Journal Citation Index ranking source')
ON DUPLICATE KEY UPDATE source_name = VALUES(source_name), description = VALUES(description), updated_at = CURRENT_TIMESTAMP;

INSERT INTO ranking_tier_weights (source_id, tier_code, tier_name, description, weight, sort_order)
VALUES
((SELECT source_id FROM ranking_sources WHERE source_code = 'tci'), 'tci_t1', 'TCI T1', 'TCI Tier 1', 0.80, 1),
((SELECT source_id FROM ranking_sources WHERE source_code = 'tci'), 'tci_t2', 'TCI 2', 'TCI Tier 2', 0.60, 2),
((SELECT source_id FROM ranking_sources WHERE source_code = 'tci'), 'tci_conf', 'TCI Conf', 'TCI Conference', 0.20, 3),
((SELECT source_id FROM ranking_sources WHERE source_code = 'scopus'), 'scopus_journal_q1_q4', 'Scopus Journal Q1-Q4', 'Scopus journal ranked from Q1 to Q4', 1.00, 1),
((SELECT source_id FROM ranking_sources WHERE source_code = 'scopus'), 'scopus_conf', 'Scopus Conf', 'Scopus Conference', 0.40, 2)
ON DUPLICATE KEY UPDATE tier_name = VALUES(tier_name), description = VALUES(description), weight = VALUES(weight), sort_order = VALUES(sort_order), updated_at = CURRENT_TIMESTAMP;