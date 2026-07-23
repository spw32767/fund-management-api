CREATE TABLE IF NOT EXISTS submission_sdgs (
  submission_sdg_id INT NOT NULL AUTO_INCREMENT,
  submission_id INT NOT NULL,
  sdg_id INT NOT NULL,
  sdg_number_snapshot TINYINT UNSIGNED NOT NULL,
  name_th_snapshot VARCHAR(255) NOT NULL,
  name_en_snapshot VARCHAR(255) NOT NULL,
  description_th_snapshot TEXT DEFAULT NULL,
  description_en_snapshot TEXT DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (submission_sdg_id),
  UNIQUE KEY uq_submission_sdgs_submission_sdg (submission_id, sdg_id),
  KEY idx_submission_sdgs_sdg (sdg_id),
  CONSTRAINT fk_submission_sdgs_submission FOREIGN KEY (submission_id) REFERENCES submissions (submission_id),
  CONSTRAINT fk_submission_sdgs_sdg FOREIGN KEY (sdg_id) REFERENCES sdgs (sdg_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
