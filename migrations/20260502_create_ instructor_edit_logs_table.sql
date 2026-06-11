CREATE TABLE IF NOT EXISTS instructor_edit_logs (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    user_edit_id    INT NOT NULL,
    target_user_id  INT NOT NULL,
    action          ENUM('INSERT', 'UPDATE', 'DELETE') NOT NULL,
    table_name      VARCHAR(100) NOT NULL,
    field_name      VARCHAR(100) NULL,
    record_id       INT NOT NULL,
    old_value       TEXT DEFAULT NULL,
    new_value       TEXT DEFAULT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_instructor_edit_logs_user
        FOREIGN KEY (user_edit_id)
        REFERENCES users(user_id)
        ON DELETE RESTRICT
        ON UPDATE CASCADE,

    CONSTRAINT fk_instructor_edit_logs_target          -- เพิ่ม
        FOREIGN KEY (target_user_id)
        REFERENCES users(user_id)
        ON DELETE RESTRICT
        ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;