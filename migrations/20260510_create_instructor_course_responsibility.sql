CREATE TABLE IF NOT EXISTS instructor_course_responsibility (   --แก้ชื่อ
    id          INT AUTO_INCREMENT PRIMARY KEY,
    user_id     INT NOT NULL,
    course_id   INT NOT NULL,
    deleted_at  DATETIME NULL DEFAULT NULL,                     -- เพิ่ม
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,           -- เพิ่ม
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,  -- เพิ่ม
    UNIQUE KEY (user_id, course_id),
    CONSTRAINT fk_user_resp
        FOREIGN KEY (user_id) REFERENCES users(user_id)        -- แก้ FK
        ON DELETE CASCADE,
    CONSTRAINT fk_course_resp
        FOREIGN KEY (course_id) REFERENCES instructor_courses(course_id)  --แก้ FK
        ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;