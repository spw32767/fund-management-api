-- Shared, editable taxonomy for paper classification. This is intentionally
-- separate from fund_categories, which classifies funding requests.
CREATE TABLE IF NOT EXISTS paper_categories (
  category_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code VARCHAR(64) NOT NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT DEFAULT NULL,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  display_order INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (category_id),
  UNIQUE KEY uq_paper_categories_code (code),
  KEY idx_paper_categories_active_order (is_active, display_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO paper_categories (code, name, description, display_order) VALUES
  ('THEORETICAL_CS', 'Theoretical Computer Science', 'Mathematical and theoretical foundations of computation, algorithms, formal reasoning, complexity, theorem proving, formal verification, model checking, semantics, and computability.', 10),
  ('AI_ALGORITHMS', 'AI Algorithms and Intelligent Systems', 'New AI or machine-learning algorithms, intelligent models, optimization methods, learning architectures, generative AI, explainable AI, graph neural networks, and multimodal learning.', 20),
  ('APPLIED_AI', 'Applied AI, GeoAI and Agentic Applications', 'Applications of established AI to real-world domains, autonomous agents, GeoAI, decision support, healthcare, agriculture, smart cities, GIS, remote sensing, and operational systems.', 30),
  ('NETWORKS_SECURITY_DISTRIBUTED', 'Networks, Security, and Distributed Systems', 'Computer networks, cybersecurity, distributed systems, cloud-edge architectures, blockchain, federated learning, authentication, encryption, routing, protocols, latency, and throughput.', 40),
  ('QUANTUM_INFORMATION', 'Quantum Information Science', 'Quantum computing, quantum communication, quantum cryptography, quantum algorithms, quantum circuits, qubits, quantum machine learning, quantum simulation, quantum networking, and post-quantum cryptography.', 50),
  ('COMPUTER_ENGINEERING_IOT_EMBEDDED', 'Computer Engineering, IoT, and Embedded Systems', 'Hardware-oriented systems, embedded platforms, IoT devices, sensors, robotics, cyber-physical systems, firmware, FPGA, hardware acceleration, and physical device implementation.', 60),
  ('EDTECH_LEARNING_DIGITAL_LIBRARY', 'Educational Technology, Learning Sciences, and Digital Library Systems', 'Education, learning environments, learning analytics, intelligent tutoring, educational data mining, digital libraries, academic information systems, information literacy, and knowledge management.', 70)
ON DUPLICATE KEY UPDATE
  name = VALUES(name),
  description = VALUES(description),
  display_order = VALUES(display_order),
  is_active = 1;

-- Keep historical rows referentially valid if an earlier taxonomy was seeded,
-- while ensuring the classifier receives only the seven current categories.
UPDATE paper_categories
SET is_active = 0
WHERE code NOT IN (
  'THEORETICAL_CS',
  'AI_ALGORITHMS',
  'APPLIED_AI',
  'NETWORKS_SECURITY_DISTRIBUTED',
  'QUANTUM_INFORMATION',
  'COMPUTER_ENGINEERING_IOT_EMBEDDED',
  'EDTECH_LEARNING_DIGITAL_LIBRARY'
);

ALTER TABLE scopus_benchmark_documents
  ADD COLUMN category BIGINT UNSIGNED DEFAULT NULL COMMENT 'FK to paper_categories.category_id',
  ADD COLUMN classification_confidence ENUM('High', 'Medium', 'Low') DEFAULT NULL,
  ADD COLUMN classification_model VARCHAR(128) DEFAULT NULL,
  ADD COLUMN classification_taxonomy_version VARCHAR(64) DEFAULT NULL,
  ADD COLUMN classified_at DATETIME DEFAULT NULL,
  ADD KEY idx_scopus_benchmark_paper_category (category),
  ADD CONSTRAINT fk_scopus_benchmark_paper_category
    FOREIGN KEY (category) REFERENCES paper_categories(category_id)
    ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE publication_reward_details
  ADD COLUMN scopus_benchmark_document_id BIGINT UNSIGNED DEFAULT NULL,
  ADD COLUMN abstract LONGTEXT DEFAULT NULL,
  ADD COLUMN abstract_summary_th LONGTEXT DEFAULT NULL,
  ADD COLUMN paper_category_id BIGINT UNSIGNED DEFAULT NULL,
  ADD COLUMN classification_confidence ENUM('High', 'Medium', 'Low') DEFAULT NULL,
  ADD COLUMN classification_model VARCHAR(128) DEFAULT NULL,
  ADD COLUMN classification_taxonomy_version VARCHAR(64) DEFAULT NULL,
  ADD COLUMN classified_at DATETIME DEFAULT NULL,
  ADD KEY idx_publication_reward_benchmark_document (scopus_benchmark_document_id),
  ADD KEY idx_publication_reward_paper_category (paper_category_id),
  ADD CONSTRAINT fk_publication_reward_benchmark_document
    FOREIGN KEY (scopus_benchmark_document_id) REFERENCES scopus_benchmark_documents(id)
    ON UPDATE CASCADE ON DELETE SET NULL,
  ADD CONSTRAINT fk_publication_reward_paper_category
    FOREIGN KEY (paper_category_id) REFERENCES paper_categories(category_id)
    ON UPDATE CASCADE ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS paper_ai_jobs (
  job_id CHAR(36) NOT NULL,
  user_id INT NOT NULL,
  submission_id INT DEFAULT NULL,
  job_type VARCHAR(32) NOT NULL COMMENT 'extract | summarize | classify',
  status VARCHAR(24) NOT NULL DEFAULT 'pending',
  request_fingerprint CHAR(64) DEFAULT NULL,
  result_json LONGTEXT DEFAULT NULL,
  error_message TEXT DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at DATETIME DEFAULT NULL,
  completed_at DATETIME DEFAULT NULL,
  PRIMARY KEY (job_id),
  KEY idx_paper_ai_jobs_user_created (user_id, created_at),
  KEY idx_paper_ai_jobs_submission (submission_id),
  CONSTRAINT fk_paper_ai_jobs_user FOREIGN KEY (user_id) REFERENCES users(user_id)
    ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT fk_paper_ai_jobs_submission FOREIGN KEY (submission_id) REFERENCES submissions(submission_id)
    ON UPDATE CASCADE ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
