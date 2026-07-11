
-- lookup tables ที่ FK ของ MOU อ้างถึง
CREATE TABLE IF NOT EXISTS `faculties` (
  `id` int(11) NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `name_th` varchar(200) NOT NULL,
  `name_en` varchar(200) NOT NULL,
  `is_active` tinyint(1) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` datetime DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO `faculties` (`id`, `name_th`, `name_en`, `is_active`) VALUES
(1,  'คณะวิทยาศาสตร์', 'Faculty of Science', 1),
(2,  'คณะวิศวกรรมศาสตร์', 'Faculty of Engineering', 1),
(3,  'คณะเทคโนโลยี', 'Faculty of Technology', 1),
(4,  'คณะเกษตรศาสตร์', 'Faculty of Agriculture', 1),
(5,  'คณะสถาปัตยกรรมศาสตร์', 'Faculty of Architecture', 1),
(6,  'วิทยาลัยการคอมพิวเตอร์', 'College of Computing', 1),
(7,  'คณะแพทยศาสตร์', 'Faculty of Medicine', 1),
(8,  'คณะทันตแพทยศาสตร์', 'Faculty of Dentistry', 1),
(9,  'คณะเภสัชศาสตร์', 'Faculty of Pharmaceutical Sciences', 1),
(10, 'คณะพยาบาลศาสตร์', 'Faculty of Nursing', 1),
(11, 'คณะสาธารณสุขศาสตร์', 'Faculty of Public Health', 1),
(12, 'คณะเทคนิคการแพทย์', 'Faculty of Associated Medical Sciences', 1),
(13, 'คณะมนุษยศาสตร์และสังคมศาสตร์', 'Faculty of Humanities and Social Sciences', 1),
(14, 'คณะศึกษาศาสตร์', 'Faculty of Education', 1),
(15, 'คณะนิติศาสตร์', 'Faculty of Law', 1),
(16, 'คณะบริหารธุรกิจและการบัญชี', 'Faculty of Business Administration and Accountancy', 1),
(17, 'คณะเศรษฐศาสตร์', 'Faculty of Economics', 1),
(18, 'คณะศิลปกรรมศาสตร์', 'Faculty of Fine and Applied Arts', 1),
(19, 'วิทยาลัยนานาชาติ', 'KKU International College', 1);

CREATE TABLE IF NOT EXISTS `countries` (
  `id` int(11) NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `name_th` varchar(200) NOT NULL,
  `name_en` varchar(200) NOT NULL,
  `is_active` tinyint(1) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` datetime DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO `countries` (`id`, `name_th`, `name_en`, `is_active`) VALUES
(1,  'ไทย', 'Thailand', 1),
(2,  'ญี่ปุ่น', 'Japan', 1),
(3,  'สหรัฐอเมริกา', 'United States', 1),
(4,  'สหราชอาณาจักร', 'United Kingdom', 1),
(5,  'สาธารณรัฐประชาชนจีน', 'China', 1),
(6,  'เกาหลีใต้', 'South Korea', 1),
(7,  'ออสเตรเลีย', 'Australia', 1),
(8,  'เยอรมนี', 'Germany', 1),
(9,  'ฝรั่งเศส', 'France', 1),
(10, 'แคนาดา', 'Canada', 1),
(11, 'สิงคโปร์', 'Singapore', 1),
(12, 'มาเลเซีย', 'Malaysia', 1),
(13, 'อินโดนีเซีย', 'Indonesia', 1),
(14, 'เวียดนาม', 'Vietnam', 1),
(15, 'ฟิลิปปินส์', 'Philippines', 1),
(16, 'อินเดีย', 'India', 1),
(17, 'ไต้หวัน', 'Taiwan', 1),
(18, 'ฮ่องกง', 'Hong Kong', 1),
(19, 'เนเธอร์แลนด์', 'Netherlands', 1),
(20, 'สวีเดน', 'Sweden', 1),
(21, 'นิวซีแลนด์', 'New Zealand', 1),
(22, 'รัสเซีย', 'Russia', 1),
(23, 'บราซิล', 'Brazil', 1),
(24, 'เม็กซิโก', 'Mexico', 1),
(25, 'สหรัฐอาหรับเอมิเรตส์', 'United Arab Emirates', 1),
(26, 'ฟินแลนด์', 'Finland', 1),
(27, 'เบลเยียม', 'Belgium', 1),
(28, 'สวิตเซอร์แลนด์', 'Switzerland', 1),
(29, 'พม่า', 'Myanmar', 1),
(30, 'กัมพูชา', 'Cambodia', 1);

CREATE TABLE IF NOT EXISTS `mou_status` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `name` varchar(200) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT current_timestamp(),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO `mou_status` (`id`, `name`) VALUES
(2, 'มีผลบังคับใช้'),
(3, 'หมดอายุ'),
(4, 'ยกเลิก'),
(5, 'กำลังดำเนินการ'),
(6, 'ต่ออายุ'),
(7, 'ใกล้หมดอายุ');

CREATE TABLE IF NOT EXISTS `mou_okr` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `title` varchar(300) NOT NULL,
  `description` text DEFAULT NULL,
  `category` varchar(100) DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT NULL ON UPDATE current_timestamp(),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_partner_type` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `name_th` varchar(100) NOT NULL,
  `description` varchar(255) DEFAULT NULL,
  `is_active` tinyint(1) NOT NULL DEFAULT 1,
  `deleted_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT NULL ON UPDATE current_timestamp(),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_partner_type_name` (`name_th`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO `mou_partner_type` (`id`, `name_th`, `description`, `is_active`) VALUES
(1, 'ภาคเอกชน', NULL, 1),
(2, 'หน่วยงานรัฐ', NULL, 1),
(3, 'สถาบันต่างประเทศ', NULL, 1),
(4, 'หน่วยงานการศึกษาในประเทศ', 'คณะ/สถาบันการศึกษาภายในประเทศไทย', 1);

CREATE TABLE IF NOT EXISTS `mou_activity_type` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `name` varchar(200) NOT NULL,
  `description` text DEFAULT NULL,
  `is_active` tinyint(1) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT current_timestamp(),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO `mou_activity_type` (`id`, `name`, `is_active`) VALUES
(1, 'ลงนาม MOU', 1),
(2, 'การบริการวิชาการ', 1),
(3, 'หารือความร่วมมือ', 1),
(4, 'อื่นๆ', 1),
(5, 'การเรียนการสอน', 1);

CREATE TABLE IF NOT EXISTS `mou_records` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `mou_code` varchar(50) DEFAULT NULL,
  `title` varchar(500) NOT NULL,
  `description` text DEFAULT NULL,
  `Status_id` int(11) NOT NULL,
  `level` enum('university','faculty') NOT NULL,
  `start_date` date DEFAULT NULL,
  `end_date` date DEFAULT NULL,
  `year_of_signing` date DEFAULT NULL,
  `signed_by` varchar(255) DEFAULT NULL,
  `notes` text DEFAULT NULL,
  `notify_days_before` int(11) DEFAULT NULL,
  `is_international` tinyint(1) NOT NULL DEFAULT 0,
  `Country_id` int(11) DEFAULT NULL,
  `coordinator_id` int(11) DEFAULT NULL,
  `coordinator_other` varchar(200) DEFAULT NULL COMMENT 'ชื่อผู้ประสานงาน กรณีไม่อยู่ในระบบ',
  `lock_mou` tinyint(1) NOT NULL DEFAULT 0,
  `created_by` int(11) NOT NULL,
  `updated_by` int(11) DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT NULL ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `Status_id` (`Status_id`),
  KEY `Country_id` (`Country_id`),
  KEY `coordinator_id` (`coordinator_id`),
  KEY `created_by` (`created_by`),
  KEY `updated_by` (`updated_by`),
  CONSTRAINT `mou_records_ibfk_1` FOREIGN KEY (`Status_id`) REFERENCES `mou_status` (`id`),
  CONSTRAINT `mou_records_ibfk_2` FOREIGN KEY (`Country_id`) REFERENCES `countries` (`id`) ON DELETE SET NULL,
  CONSTRAINT `mou_records_ibfk_3` FOREIGN KEY (`coordinator_id`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  CONSTRAINT `mou_records_ibfk_4` FOREIGN KEY (`created_by`) REFERENCES `users` (`user_id`),
  CONSTRAINT `mou_records_ibfk_5` FOREIGN KEY (`updated_by`) REFERENCES `users` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_partner` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `mou_id` int(11) NOT NULL,
  `partner_org` varchar(300) NOT NULL,
  `partner_type_id` int(11) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT NULL ON UPDATE current_timestamp(),
  PRIMARY KEY (`id`),
  KEY `mou_id` (`mou_id`),
  KEY `partner_type_id` (`partner_type_id`),
  CONSTRAINT `mou_partner_ibfk_1` FOREIGN KEY (`mou_id`) REFERENCES `mou_records` (`id`) ON DELETE CASCADE,
  CONSTRAINT `mou_partner_ibfk_2` FOREIGN KEY (`partner_type_id`) REFERENCES `mou_partner_type` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_faculty` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `mou_id` int(11) NOT NULL,
  `user_id` int(11) DEFAULT NULL,
  `cp_employee_id` int(11) DEFAULT NULL,
  `faculty_id` int(11) DEFAULT NULL,
  `external_name` varchar(200) DEFAULT NULL,
  `external_org` varchar(300) DEFAULT NULL,
  `email` varchar(300) DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  PRIMARY KEY (`id`),
  KEY `mou_id` (`mou_id`),
  KEY `user_id` (`user_id`),
  KEY `cp_employee_id` (`cp_employee_id`),
  KEY `faculty_id` (`faculty_id`),
  CONSTRAINT `mou_faculty_ibfk_1` FOREIGN KEY (`mou_id`) REFERENCES `mou_records` (`id`) ON DELETE CASCADE,
  CONSTRAINT `mou_faculty_ibfk_2` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  CONSTRAINT `mou_faculty_ibfk_3` FOREIGN KEY (`cp_employee_id`) REFERENCES `cp_employee` (`ID`) ON DELETE SET NULL,
  CONSTRAINT `mou_faculty_ibfk_4` FOREIGN KEY (`faculty_id`) REFERENCES `faculties` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_attachment` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `mou_id` int(11) NOT NULL,
  `file_name` varchar(255) NOT NULL,
  `file_path` varchar(500) NOT NULL,
  `mime_type` varchar(100) NOT NULL,
  `uploaded_by` int(11) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `mou_id` (`mou_id`),
  KEY `uploaded_by` (`uploaded_by`),
  CONSTRAINT `mou_attachment_ibfk_1` FOREIGN KEY (`mou_id`) REFERENCES `mou_records` (`id`) ON DELETE CASCADE,
  CONSTRAINT `mou_attachment_ibfk_2` FOREIGN KEY (`uploaded_by`) REFERENCES `users` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_notification` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `mou_id` int(11) NOT NULL,
  `staff_id` int(11) DEFAULT NULL,
  `email` varchar(255) DEFAULT NULL,
  `days_before` int(11) NOT NULL,
  `is_sent` tinyint(1) NOT NULL DEFAULT 0,
  `sent_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  PRIMARY KEY (`id`),
  KEY `mou_id` (`mou_id`),
  KEY `staff_id` (`staff_id`),
  CONSTRAINT `mou_notification_ibfk_1` FOREIGN KEY (`mou_id`) REFERENCES `mou_records` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_notification_log` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `mou_id` int(11) NOT NULL,
  `action` varchar(255) NOT NULL,
  `actor_id` int(11) NOT NULL,
  `notification_id` int(11) NOT NULL,
  `sent_to` int(11) NOT NULL,
  `channel` varchar(50) NOT NULL,
  `success` tinyint(1) NOT NULL,
  `message` text DEFAULT NULL,
  `sent_at` datetime NOT NULL DEFAULT current_timestamp(),
  PRIMARY KEY (`id`),
  KEY `notification_id` (`notification_id`),
  KEY `sent_to` (`sent_to`),
  KEY `idx_mou_notification_log_mou_id` (`mou_id`),
  CONSTRAINT `mou_notification_log_ibfk_1` FOREIGN KEY (`notification_id`) REFERENCES `mou_notification` (`id`) ON DELETE CASCADE,
  CONSTRAINT `mou_notification_log_ibfk_2` FOREIGN KEY (`sent_to`) REFERENCES `users` (`user_id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_notification_settings` (
  `id` int(11) NOT NULL DEFAULT 1,
  `default_days_before` int(11) NOT NULL DEFAULT 30,
  `notify_coordinator` tinyint(1) NOT NULL DEFAULT 1,
  `notify_faculty_responsible` tinyint(1) NOT NULL DEFAULT 0,
  `notify_external` tinyint(1) NOT NULL DEFAULT 0,
  `include_mou_code` tinyint(1) NOT NULL DEFAULT 1,
  `include_title` tinyint(1) NOT NULL DEFAULT 1,
  `include_partner` tinyint(1) NOT NULL DEFAULT 1,
  `include_dates` tinyint(1) NOT NULL DEFAULT 1,
  `include_level` tinyint(1) NOT NULL DEFAULT 0,
  `include_status` tinyint(1) NOT NULL DEFAULT 1,
  `updated_by` int(11) DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT current_timestamp(),
  `updated_at` timestamp NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (`id`),
  KEY `updated_by` (`updated_by`),
  CONSTRAINT `mou_notification_settings_ibfk_1` FOREIGN KEY (`updated_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `mou_activity` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `mou_id` int(11) NOT NULL,
  `activity_type_id` int(11) NOT NULL,
  `title` varchar(300) NOT NULL,
  `objective` text DEFAULT NULL,
  `description` text DEFAULT NULL,
  `notes` text DEFAULT NULL,
  `participant_count` int(11) DEFAULT 0,
  `activity_start` date NOT NULL,
  `activity_end` date NOT NULL,
  `location` varchar(300) NOT NULL DEFAULT '',
  `plan` text DEFAULT NULL,
  `coordinator_id` int(11) NOT NULL,
  `coordinator_other` varchar(200) DEFAULT NULL,
  `coordinator_org` varchar(300) NOT NULL,
  `links` text DEFAULT NULL COMMENT 'JSON array of reference URLs',
  `created_by` int(11) NOT NULL,
  `updated_by` int(11) DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_mou_activity_deleted_at` (`deleted_at`),
  KEY `fk_mou_activity_mou` (`mou_id`),
  KEY `fk_mou_activity_activity_type` (`activity_type_id`),
  KEY `fk_mou_activity_coordinator` (`coordinator_id`),
  KEY `fk_mou_activity_created_by` (`created_by`),
  KEY `fk_mou_activity_updated_by` (`updated_by`),
  CONSTRAINT `mou_activity_ibfk_1` FOREIGN KEY (`mou_id`) REFERENCES `mou_records` (`id`) ON DELETE CASCADE,
  CONSTRAINT `mou_activity_ibfk_2` FOREIGN KEY (`activity_type_id`) REFERENCES `mou_activity_type` (`id`),
  CONSTRAINT `mou_activity_ibfk_3` FOREIGN KEY (`coordinator_id`) REFERENCES `users` (`user_id`),
  CONSTRAINT `mou_activity_ibfk_4` FOREIGN KEY (`created_by`) REFERENCES `users` (`user_id`),
  CONSTRAINT `mou_activity_ibfk_5` FOREIGN KEY (`updated_by`) REFERENCES `users` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_activity_attachment` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `activity_id` int(11) NOT NULL,
  `file_name` varchar(255) NOT NULL,
  `file_path` varchar(500) NOT NULL,
  `mime_type` varchar(100) NOT NULL,
  `uploaded_by` int(11) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `activity_id` (`activity_id`),
  KEY `uploaded_by` (`uploaded_by`),
  CONSTRAINT `mou_activity_attachment_ibfk_1` FOREIGN KEY (`activity_id`) REFERENCES `mou_activity` (`id`) ON DELETE CASCADE,
  CONSTRAINT `mou_activity_attachment_ibfk_2` FOREIGN KEY (`uploaded_by`) REFERENCES `users` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_activity_okr` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `activity_id` int(11) NOT NULL,
  `okr_id` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `activity_id` (`activity_id`),
  KEY `okr_id` (`okr_id`),
  CONSTRAINT `mou_activity_okr_ibfk_1` FOREIGN KEY (`activity_id`) REFERENCES `mou_activity` (`id`) ON DELETE CASCADE,
  CONSTRAINT `mou_activity_okr_ibfk_2` FOREIGN KEY (`okr_id`) REFERENCES `mou_okr` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `mou_activity_activity_type` (
  `activity_id` int(11) NOT NULL,
  `activity_type_id` int(11) NOT NULL,
  PRIMARY KEY (`activity_id`,`activity_type_id`),
  KEY `activity_type_id` (`activity_type_id`),
  CONSTRAINT `mou_activity_activity_type_ibfk_1` FOREIGN KEY (`activity_id`) REFERENCES `mou_activity` (`id`) ON DELETE CASCADE,
  CONSTRAINT `mou_activity_activity_type_ibfk_2` FOREIGN KEY (`activity_type_id`) REFERENCES `mou_activity_type` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
