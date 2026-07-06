-- 1. ตารางระดับการศึกษา (เช่น ปริญญาตรี ปริญญาโท ปริญญาเอก)
CREATE TABLE IF NOT EXISTS instructor_degrees (
  degree_id     INT AUTO_INCREMENT PRIMARY KEY,
  degree_nameTH VARCHAR(255) NOT NULL,
  degree_nameEN VARCHAR(255) NOT NULL,
  created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at    TIMESTAMP NULL DEFAULT NULL,
  INDEX idx_degrees_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 2. ตารางหลักสูตร (มี FK ไปยัง instructor_degrees)
CREATE TABLE IF NOT EXISTS instructor_courses (
  course_id      INT AUTO_INCREMENT PRIMARY KEY,
  degree_id      INT NOT NULL,
  course_nameTH  VARCHAR(255) NOT NULL,
  course_nameEN  VARCHAR(255) NOT NULL,
  degree_fullTH  VARCHAR(255) NOT NULL,
  degree_shortTH VARCHAR(100) NOT NULL,
  degree_fullEN  VARCHAR(255) NOT NULL,
  degree_shortEN VARCHAR(100) NOT NULL,
  created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at     TIMESTAMP NULL DEFAULT NULL,
  CONSTRAINT fk_course_degree FOREIGN KEY (degree_id) REFERENCES instructor_degrees(degree_id),
  INDEX idx_courses_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 3. ตารางประวัติการศึกษาของอาจารย์ (มี FK ไปยัง instructor_degrees และ users)
CREATE TABLE IF NOT EXISTS instructor_educations (
  id              INT AUTO_INCREMENT PRIMARY KEY,
  degree_id       INT NOT NULL,
  user_id         INT NOT NULL, 
  degree_title_th VARCHAR(255) NOT NULL,
  university_th   VARCHAR(255) NOT NULL,
  country         VARCHAR(255) NULL,
  grad_year       CHAR(4) NOT NULL,
  created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at      TIMESTAMP NULL DEFAULT NULL,
  CONSTRAINT fk_edu_degree FOREIGN KEY (degree_id) REFERENCES instructor_degrees(degree_id),
  CONSTRAINT fk_edu_user FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE,
  INDEX idx_educations_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;