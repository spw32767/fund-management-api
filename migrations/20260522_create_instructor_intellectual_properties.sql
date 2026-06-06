CREATE TABLE `instructor_intellectual_properties` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id` INT NOT NULL COMMENT 'ID ของอาจารย์ผู้สร้างสรรค์ผลงาน (เชื่อมกับตาราง users)',
    `type` ENUM('patent', 'petty_patent', 'copyright') NOT NULL COMMENT 'ประเภท: สิทธิบันตร, อนุสิทธิบัตร, ลิขสิทธิ์',
    `title` VARCHAR(255) NOT NULL COMMENT 'ชื่อผลงานวิชาการ/ทรัพย์สินทางปัญญา',
    `registration_number` VARCHAR(100) DEFAULT NULL COMMENT 'เลขที่สิทธิบัตร หรือเลขทะเบียนจดแจ้ง',
    `granted_year` INT DEFAULT NULL COMMENT 'ปี พ.ศ. หรือ ค.ศ. ที่ได้รับอนุมัติ/เผยแพร่',
    
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT 'วันเวลาที่เพิ่มข้อมูล',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'วันเวลาที่แก้ไขล่าสุด',
    `deleted_at` TIMESTAMP NULL DEFAULT NULL COMMENT 'วันเวลาที่ถูกลบ (Soft Delete ถ้าเป็น NULL แปลว่ายังไม่ลบ)',
    
    PRIMARY KEY (`id`),
    CONSTRAINT `fk_instructor_ip_user_id` 
        FOREIGN KEY (`user_id`) 
        REFERENCES `users` (`user_id`) 
        ON DELETE RESTRICT 
        ON UPDATE CASCADE,
        
    -- ย้ายมาสร้าง Index ตรงนี้เพื่อความเป็น Single Statement รันผ่านแน่นอนทุกเครื่องมือ
    INDEX `idx_instructor_ip_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;