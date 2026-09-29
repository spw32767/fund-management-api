-- Additive, rerunnable report schema. Never reset existing classifications or taxonomy.
CREATE TABLE IF NOT EXISTS paper_categories (category_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY, code VARCHAR(64) NOT NULL UNIQUE, name VARCHAR(255) NOT NULL, description TEXT NULL, is_active TINYINT(1) NOT NULL DEFAULT 1, display_order INT NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

SET @summary_ddl = IF((SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='scopus_benchmark_documents' AND column_name='category')=0, 'ALTER TABLE scopus_benchmark_documents ADD COLUMN category BIGINT UNSIGNED NULL', 'SELECT 1');
PREPARE summary_stmt FROM @summary_ddl;
EXECUTE summary_stmt;
DEALLOCATE PREPARE summary_stmt;

SET @summary_ddl = IF((SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='scopus_benchmark_documents' AND column_name='classification_confidence')=0, 'ALTER TABLE scopus_benchmark_documents ADD COLUMN classification_confidence ENUM(''High'',''Medium'',''Low'',''Preface'') NULL', 'SELECT 1');
PREPARE summary_stmt FROM @summary_ddl;
EXECUTE summary_stmt;
DEALLOCATE PREPARE summary_stmt;

SET @summary_ddl = IF((SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='scopus_benchmark_documents' AND column_name='classification_model')=0, 'ALTER TABLE scopus_benchmark_documents ADD COLUMN classification_model VARCHAR(128) NULL', 'SELECT 1');
PREPARE summary_stmt FROM @summary_ddl;
EXECUTE summary_stmt;
DEALLOCATE PREPARE summary_stmt;

SET @summary_ddl = IF((SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='scopus_benchmark_documents' AND column_name='classification_taxonomy_version')=0, 'ALTER TABLE scopus_benchmark_documents ADD COLUMN classification_taxonomy_version VARCHAR(64) NULL', 'SELECT 1');
PREPARE summary_stmt FROM @summary_ddl;
EXECUTE summary_stmt;
DEALLOCATE PREPARE summary_stmt;

SET @summary_ddl = IF((SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='scopus_benchmark_documents' AND column_name='classified_at')=0, 'ALTER TABLE scopus_benchmark_documents ADD COLUMN classified_at DATETIME NULL', 'SELECT 1');
PREPARE summary_stmt FROM @summary_ddl;
EXECUTE summary_stmt;
DEALLOCATE PREPARE summary_stmt;

SET @summary_ddl = IF((SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='scopus_benchmark_documents' AND column_name='affiliations_complete')=0, 'ALTER TABLE scopus_benchmark_documents ADD COLUMN affiliations_complete TINYINT(1) NOT NULL DEFAULT 0', 'SELECT 1');
PREPARE summary_stmt FROM @summary_ddl;
EXECUTE summary_stmt;
DEALLOCATE PREPARE summary_stmt;

SET @summary_ddl = IF((SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='scopus_benchmark_document_authors' AND column_name='affiliations_complete')=0, 'ALTER TABLE scopus_benchmark_document_authors ADD COLUMN affiliations_complete TINYINT(1) NOT NULL DEFAULT 0', 'SELECT 1');
PREPARE summary_stmt FROM @summary_ddl;
EXECUTE summary_stmt;
DEALLOCATE PREPARE summary_stmt;

CREATE TABLE IF NOT EXISTS scopus_benchmark_document_affiliations (document_id BIGINT UNSIGNED NOT NULL, afid VARCHAR(32) NOT NULL, provenance VARCHAR(32) NOT NULL, PRIMARY KEY(document_id,afid), KEY idx_summary_doc_afid(afid,document_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS scopus_benchmark_author_affiliations (document_id BIGINT UNSIGNED NOT NULL, author_id BIGINT UNSIGNED NOT NULL, afid VARCHAR(32) NOT NULL, provenance VARCHAR(32) NOT NULL, PRIMARY KEY(document_id,author_id,afid), KEY idx_summary_author_afid(afid,author_id,document_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO paper_categories(code,name,display_order) SELECT 'THEORETICAL_CS','Theoretical Computer Science',1 WHERE NOT EXISTS (SELECT 1 FROM paper_categories WHERE code='THEORETICAL_CS' OR name='Theoretical Computer Science');

INSERT INTO paper_categories(code,name,display_order) SELECT 'AI_ALGORITHMS','AI Algorithms and Intelligent Systems',2 WHERE NOT EXISTS (SELECT 1 FROM paper_categories WHERE code='AI_ALGORITHMS' OR name='AI Algorithms and Intelligent Systems');

INSERT INTO paper_categories(code,name,display_order) SELECT 'APPLIED_AI','Applied AI, GeoAI and Agentic Applications',3 WHERE NOT EXISTS (SELECT 1 FROM paper_categories WHERE code='APPLIED_AI' OR name='Applied AI, GeoAI and Agentic Applications');

INSERT INTO paper_categories(code,name,display_order) SELECT 'NETWORKS_SECURITY_DISTRIBUTED','Networks, Security, and Distributed Systems',4 WHERE NOT EXISTS (SELECT 1 FROM paper_categories WHERE code='NETWORKS_SECURITY_DISTRIBUTED' OR name='Networks, Security, and Distributed Systems');

INSERT INTO paper_categories(code,name,display_order) SELECT 'QUANTUM_INFORMATION','Quantum Information Science',5 WHERE NOT EXISTS (SELECT 1 FROM paper_categories WHERE code='QUANTUM_INFORMATION' OR name='Quantum Information Science');

INSERT INTO paper_categories(code,name,display_order) SELECT 'COMPUTER_ENGINEERING_IOT_EMBEDDED','Computer Engineering, IoT, and Embedded Systems',6 WHERE NOT EXISTS (SELECT 1 FROM paper_categories WHERE code='COMPUTER_ENGINEERING_IOT_EMBEDDED' OR name='Computer Engineering, IoT, and Embedded Systems');

INSERT INTO paper_categories(code,name,display_order) SELECT 'EDTECH_LEARNING_DIGITAL_LIBRARY','Educational Technology, Learning Sciences, and Digital Library Systems',7 WHERE NOT EXISTS (SELECT 1 FROM paper_categories WHERE code='EDTECH_LEARNING_DIGITAL_LIBRARY' OR name='Educational Technology, Learning Sciences, and Digital Library Systems');
