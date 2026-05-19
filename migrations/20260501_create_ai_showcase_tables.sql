-- ============================================================
-- Table : ai_showcase_tracks (คณะ)
-- ============================================================

CREATE TABLE IF NOT EXISTS ai_showcase_tracks (
  id        VARCHAR(20)  NOT NULL,
  name_th   TEXT         NOT NULL,
  name_en   TEXT         NOT NULL,
  created_at DATETIME    NOT NULL DEFAULT current_timestamp(),
  updated_at DATETIME    NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO ai_showcase_tracks (id, name_th, name_en) VALUES
  ('ag',   'คณะเกษตรศาสตร์',               'Faculty of Agriculture'),
  ('cola', 'วิทยาลัยการปกครองท้องถิ่น',        'College of Local Administration'),
  ('cp',   'วิทยาลัยการคอมพิวเตอร์',           'College of Computing'),
  ('kkbs', 'คณะบริหารธุรกิจและการบัญชี',       'Faculty of Business Administration and Accountancy'),
  ('md',   'คณะแพทยศาสตร์',               'Faculty of Medicine');


-- ============================================================
-- Table : ai_showcase_projects (โปรเจกต์)
-- ============================================================

CREATE TABLE IF NOT EXISTS ai_showcase_projects (
  id               BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT,
  title_th         TEXT             NOT NULL,
  title_en         TEXT             NOT NULL,
  abstract         LONGTEXT         DEFAULT NULL,
  description      TEXT             DEFAULT NULL,
  project_type     VARCHAR(100)     NOT NULL,
  group_code       VARCHAR(50)      NOT NULL,
  published_year   INT(4)           NOT NULL,
  track_id         VARCHAR(20)      NOT NULL,
  ai_showcase_link TEXT             DEFAULT NULL,
  poster_url       TEXT             DEFAULT NULL,
  imported_at      DATETIME         NOT NULL DEFAULT current_timestamp(),
  created_at       DATETIME         NOT NULL DEFAULT current_timestamp(),
  updated_at       DATETIME         NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (id),
  KEY idx_ai_project_type    (project_type),
  KEY idx_ai_group_code      (group_code),
  KEY idx_ai_published_year  (published_year),
  KEY idx_ai_track_id        (track_id),
  CONSTRAINT fk_ai_project_track
    FOREIGN KEY (track_id)
    REFERENCES ai_showcase_tracks (id)
    ON DELETE RESTRICT
    ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table : ai_showcase_project_members (สมาชิกกลุ่ม)
-- ============================================================

CREATE TABLE IF NOT EXISTS ai_showcase_project_members (
  id          BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT,
  project_id  BIGINT UNSIGNED  NOT NULL,
  student_id  VARCHAR(20)      NOT NULL,
  name        TEXT             NOT NULL,
  created_at  DATETIME         NOT NULL DEFAULT current_timestamp(),
  updated_at  DATETIME         NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (id),
  KEY idx_ai_member_project    (project_id),
  KEY idx_ai_member_student_id (student_id),
  CONSTRAINT fk_ai_member_project
    FOREIGN KEY (project_id)
    REFERENCES ai_showcase_projects (id)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
