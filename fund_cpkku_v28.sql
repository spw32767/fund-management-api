-- phpMyAdmin SQL Dump
-- version 4.9.5deb2
-- https://www.phpmyadmin.net/
--
-- Host: localhost:3306
-- Generation Time: Aug 23, 2025 at 11:23 AM
-- Server version: 11.1.6-MariaDB-ubu2004
-- PHP Version: 7.4.3-4ubuntu2.29

SET SQL_MODE = "NO_AUTO_VALUE_ON_ZERO";
SET AUTOCOMMIT = 0;
START TRANSACTION;
SET time_zone = "+00:00";


/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!40101 SET NAMES utf8mb4 */;

--
-- Database: `fund_cpkku`
--

DELIMITER $$
--
-- Procedures
--
CREATE DEFINER=`devuser`@`localhost` PROCEDURE `CreateNotification` (IN `p_user_id` INT, IN `p_title` VARCHAR(255), IN `p_message` TEXT, IN `p_type` VARCHAR(50), IN `p_application_id` INT)  BEGIN
    INSERT INTO notifications (
        user_id, title, message, type, 
        related_application_id, is_read, create_at
    ) VALUES (
        p_user_id, p_title, p_message, p_type, 
        p_application_id, FALSE, NOW()
    );
END$$

CREATE DEFINER=`devuser`@`localhost` PROCEDURE `migrate_fund_applications` ()  BEGIN
    DECLARE done INT DEFAULT FALSE;
    DECLARE v_submission_id INT;
    DECLARE v_application_id INT;
    DECLARE v_subcategory_id INT;
    DECLARE v_project_title VARCHAR(255);
    DECLARE v_project_description TEXT;
    DECLARE v_requested_amount DECIMAL(15,2);
    DECLARE v_approved_amount DECIMAL(15,2);
    DECLARE v_closed_at DATETIME;
    DECLARE v_comment TEXT;
    
    DECLARE cur CURSOR FOR
        SELECT 
            fa.application_id,
            fa.subcategory_id,
            fa.project_title,
            fa.project_description,
            fa.requested_amount,
            fa.approved_amount,
            fa.closed_at,
            fa.comment
        FROM fund_applications fa
        WHERE fa.delete_at IS NULL;
    
    DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = TRUE;
    
    -- Start transaction
    START TRANSACTION;
    
    OPEN cur;
    
    read_loop: LOOP
        FETCH cur INTO v_application_id, v_subcategory_id, v_project_title, 
                      v_project_description, v_requested_amount, v_approved_amount,
                      v_closed_at, v_comment;
        
        IF done THEN
            LEAVE read_loop;
        END IF;
        
        -- Get the submission_id for this application
        SELECT submission_id INTO v_submission_id
        FROM submissions s
        JOIN fund_applications fa ON s.submission_number = fa.application_number
        WHERE fa.application_id = v_application_id
        LIMIT 1;
        
        -- Insert into fund_application_details
        IF v_submission_id IS NOT NULL THEN
            INSERT INTO fund_application_details (
                submission_id, subcategory_id, project_title,
                project_description, requested_amount, approved_amount,
                closed_at, comment
            ) VALUES (
                v_submission_id, v_subcategory_id, v_project_title,
                v_project_description, v_requested_amount, v_approved_amount,
                v_closed_at, v_comment
            )
            ON DUPLICATE KEY UPDATE
                subcategory_id = VALUES(subcategory_id),
                project_title = VALUES(project_title),
                project_description = VALUES(project_description),
                requested_amount = VALUES(requested_amount),
                approved_amount = VALUES(approved_amount),
                closed_at = VALUES(closed_at),
                comment = VALUES(comment);
        END IF;
    END LOOP;
    
    CLOSE cur;
    
    -- Commit transaction
    COMMIT;
    
    -- Report results
    SELECT COUNT(*) as migrated_count FROM fund_application_details;
END$$

DELIMITER ;

-- --------------------------------------------------------

--
-- Table structure for table `announcements`
--

CREATE TABLE `announcements` (
  `announcement_id` int(11) NOT NULL,
  `title` varchar(255) NOT NULL COMMENT 'หัวข้อประกาศ',
  `description` text DEFAULT NULL COMMENT 'รายละเอียดประกาศ',
  `file_name` varchar(255) NOT NULL COMMENT 'ชื่อไฟล์ต้นฉบับ',
  `file_path` varchar(500) NOT NULL COMMENT 'path ไฟล์ในระบบ',
  `file_size` bigint(20) DEFAULT NULL COMMENT 'ขนาดไฟล์ (bytes)',
  `mime_type` varchar(100) DEFAULT NULL COMMENT 'ประเภทไฟล์',
  `announcement_type` enum('general','research_fund','promotion_fund') DEFAULT 'general' COMMENT 'ประเภทประกาศ',
  `priority` enum('normal','high','urgent') DEFAULT 'normal' COMMENT 'ความสำคัญ',
  `status` enum('active','inactive') DEFAULT 'active' COMMENT 'สถานะการเผยแพร่',
  `published_at` datetime DEFAULT NULL COMMENT 'วันที่เผยแพร่',
  `expired_at` datetime DEFAULT NULL COMMENT 'วันที่หมดอายุ',
  `created_by` int(11) NOT NULL COMMENT 'ผู้สร้าง (user_id)',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci COMMENT='ตารางเก็บประกาศจากกองทุนวิจัยและนวัตกรรม';

--
-- Dumping data for table `announcements`
--

INSERT INTO `announcements` (`announcement_id`, `title`, `description`, `file_name`, `file_path`, `file_size`, `mime_type`, `announcement_type`, `priority`, `status`, `published_at`, `expired_at`, `created_by`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 'ประกาศเปิดรับสมัครทุนส่งเสริมการวิจัย ประจำปี 2568', 'กองทุนส่งเสริมการวิจัยและนวัตกรรม เปิดรับสมัครทุนส่งเสริมการวิจัย ประจำปี 2568', 'ประกาศทุนวิจัย2568.pdf', 'uploads/announcements/2025/01/ประกาศทุนวิจัย2568.pdf', 1024000, 'application/pdf', 'research_fund', 'high', 'active', '2025-01-15 09:00:00', NULL, 1, '2025-08-20 11:40:31', '2025-08-20 11:40:31', NULL),
(2, 'แนวทางการเขียนข้อเสนอโครงการวิจัย', 'เอกสารแนวทางและข้อแนะนำสำหรับการเขียนข้อเสนอโครงการวิจัย', 'แนวทางการเขียนข้อเสนอโครงการ.pdf', 'uploads/announcements/2025/01/แนวทางการเขียนข้อเสนอโครงการ.pdf', 2048000, 'application/pdf', 'research_fund', 'normal', 'active', '2025-01-10 10:00:00', NULL, 1, '2025-08-20 11:40:31', '2025-08-20 11:40:31', NULL),
(3, 'ประกาศเปิดรับสมัครทุนอุดหนุนกิจกรรม ไตรมาส 1/2568', 'เปิดรับสมัครทุนอุดหนุนกิจกรรมประจำไตรมาส 1 ประจำปี 2568', 'ประกาศทุนกิจกรรมไตรมาส1-2568.pdf', 'uploads/announcements/2025/01/ประกาศทุนกิจกรรมไตรมาส1-2568.pdf', 800000, 'application/pdf', 'promotion_fund', 'normal', 'active', '2025-01-05 14:00:00', NULL, 1, '2025-08-20 11:40:31', '2025-08-20 11:40:31', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `application_status`
--

CREATE TABLE `application_status` (
  `application_status_id` int(11) NOT NULL,
  `status_code` varchar(255) DEFAULT NULL,
  `status_name` varchar(255) DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

--
-- Dumping data for table `application_status`
--

INSERT INTO `application_status` (`application_status_id`, `status_code`, `status_name`, `create_at`, `update_at`, `delete_at`) VALUES
(1, '0', 'รอพิจารณา', '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(2, '1', 'อนุมัติ', '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(3, '2', 'ปฏิเสธ', '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(4, '3', 'ต้องการข้อมูลเพิ่มเติม', '2025-08-12 15:50:00', '2025-08-12 10:50:45', NULL),
(5, '4', 'ร่าง', '2025-08-12 15:50:22', '2025-08-12 10:50:25', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `audit_logs`
--

CREATE TABLE `audit_logs` (
  `log_id` int(11) NOT NULL,
  `user_id` int(11) DEFAULT NULL,
  `action` enum('create','update','delete','login','logout','view','download','approve','reject','submit','review') NOT NULL,
  `entity_type` varchar(50) NOT NULL,
  `entity_id` int(11) DEFAULT NULL,
  `entity_number` varchar(50) DEFAULT NULL,
  `old_values` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (json_valid(`old_values`)),
  `new_values` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (json_valid(`new_values`)),
  `changed_fields` text DEFAULT NULL,
  `ip_address` varchar(45) DEFAULT NULL,
  `user_agent` varchar(255) DEFAULT NULL,
  `description` text DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

--
-- Dumping data for table `audit_logs`
--

INSERT INTO `audit_logs` (`log_id`, `user_id`, `action`, `entity_type`, `entity_id`, `entity_number`, `old_values`, `new_values`, `changed_fields`, `ip_address`, `user_agent`, `description`, `created_at`) VALUES
(1, 8, 'create', 'submission', 1, 'PR-20250731-0001', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-07-31 12:58:57'),
(2, 8, 'submit', 'submission', 1, 'PR-20250731-0001', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-07-31 12:58:57'),
(3, 8, 'create', 'submission', 2, 'PR-20250731-0002', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-07-31 13:28:33'),
(4, 8, 'submit', 'submission', 2, 'PR-20250731-0002', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-07-31 13:28:34'),
(5, 8, 'create', 'submission', 3, 'PR-20250731-0003', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-07-31 13:38:11'),
(6, 8, 'submit', 'submission', 3, 'PR-20250731-0003', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-07-31 13:38:11'),
(7, 8, 'create', 'submission', 4, 'PR-20250731-0004', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-07-31 13:42:38'),
(8, 8, 'submit', 'submission', 4, 'PR-20250731-0004', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-07-31 13:42:38'),
(9, 8, 'create', 'submission', 5, 'PR-20250731-0005', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-07-31 13:44:25'),
(10, 8, 'submit', 'submission', 5, 'PR-20250731-0005', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-07-31 13:44:25'),
(11, 8, 'create', 'submission', 6, 'PR-20250801-0001', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 10:24:28'),
(12, 8, 'submit', 'submission', 6, 'PR-20250801-0001', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 10:24:29'),
(13, 8, 'create', 'submission', 7, 'PR-20250801-0002', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 10:25:34'),
(14, 8, 'submit', 'submission', 7, 'PR-20250801-0002', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 10:25:35'),
(15, 8, 'create', 'submission', 8, 'PR-20250801-0003', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 10:38:16'),
(16, 8, 'submit', 'submission', 8, 'PR-20250801-0003', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 10:38:16'),
(17, 8, 'create', 'submission', 9, 'PR-20250801-0004', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 10:39:09'),
(18, 8, 'submit', 'submission', 9, 'PR-20250801-0004', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 10:39:09'),
(19, 8, 'create', 'submission', 10, 'PR-20250801-0005', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 10:42:00'),
(20, 8, 'submit', 'submission', 10, 'PR-20250801-0005', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 10:42:00'),
(21, 8, 'create', 'submission', 11, 'PR-20250801-0006', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 10:50:27'),
(22, 8, 'submit', 'submission', 11, 'PR-20250801-0006', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 10:50:27'),
(23, 8, 'create', 'submission', 12, 'PR-20250801-0007', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 11:09:50'),
(24, 8, 'submit', 'submission', 12, 'PR-20250801-0007', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 11:09:50'),
(25, 8, 'create', 'submission', 13, 'PR-20250801-0008', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 11:19:12'),
(26, 8, 'submit', 'submission', 13, 'PR-20250801-0008', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 11:19:12'),
(27, 8, 'create', 'submission', 14, 'PR-20250801-0009', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 11:22:17'),
(28, 8, 'submit', 'submission', 14, 'PR-20250801-0009', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 11:22:17'),
(29, 8, 'create', 'submission', 15, 'PR-20250801-0010', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 11:23:53'),
(30, 8, 'submit', 'submission', 15, 'PR-20250801-0010', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 11:23:53'),
(31, 8, 'create', 'submission', 16, 'PR-20250801-0011', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 11:34:30'),
(32, 8, 'submit', 'submission', 16, 'PR-20250801-0011', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 11:34:30'),
(33, 8, 'create', 'submission', 17, 'PR-20250801-0012', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 11:37:53'),
(34, 8, 'submit', 'submission', 17, 'PR-20250801-0012', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 11:37:54'),
(35, 8, 'create', 'submission', 18, 'PR-20250801-0013', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 11:40:47'),
(36, 8, 'submit', 'submission', 18, 'PR-20250801-0013', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 11:40:47'),
(37, 8, 'create', 'submission', 19, 'PR-20250801-0014', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 12:08:25'),
(38, 8, 'submit', 'submission', 19, 'PR-20250801-0014', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 12:08:25'),
(39, 8, 'create', 'submission', 20, 'PR-20250801-0015', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 12:11:19'),
(40, 8, 'submit', 'submission', 20, 'PR-20250801-0015', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 12:11:19'),
(41, 8, 'create', 'submission', 21, 'PR-20250801-0016', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-01 22:29:44'),
(42, 8, 'submit', 'submission', 21, 'PR-20250801-0016', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-01 22:29:44'),
(43, 8, 'create', 'submission', 22, 'PR-20250802-0001', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-02 12:18:25'),
(44, 8, 'submit', 'submission', 22, 'PR-20250802-0001', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-02 12:18:25'),
(45, 8, 'create', 'submission', 23, 'PR-20250805-0001', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-05 22:41:57'),
(46, 8, 'submit', 'submission', 23, 'PR-20250805-0001', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-05 22:41:57'),
(47, 8, 'create', 'submission', 24, 'PR-20250805-0002', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-05 22:46:58'),
(48, 8, 'submit', 'submission', 24, 'PR-20250805-0002', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-05 22:46:59'),
(49, 8, 'create', 'submission', 25, 'PR-20250807-0001', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 01:19:17'),
(50, 8, 'submit', 'submission', 25, 'PR-20250807-0001', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 01:19:18'),
(51, 8, 'create', 'submission', 26, 'PR-20250807-0002', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 02:34:33'),
(52, 8, 'submit', 'submission', 26, 'PR-20250807-0002', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 02:34:34'),
(53, 8, 'create', 'submission', 27, 'PR-20250807-0003', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 02:56:56'),
(54, 8, 'create', 'submission', 28, 'PR-20250807-0004', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 02:57:59'),
(55, 8, 'create', 'submission', 29, 'PR-20250807-0005', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 03:27:58'),
(56, 8, 'submit', 'submission', 29, 'PR-20250807-0005', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 03:28:43'),
(57, 8, 'create', 'submission', 30, 'PR-20250807-0006', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 04:00:14'),
(58, 8, 'create', 'submission', 31, 'PR-20250807-0007', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 06:11:59'),
(59, 8, 'submit', 'submission', 31, 'PR-20250807-0007', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 06:11:59'),
(60, 8, 'create', 'submission', 32, 'PR-20250807-0008', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 13:34:30'),
(61, 8, 'submit', 'submission', 32, 'PR-20250807-0008', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 13:34:30'),
(62, 8, 'create', 'submission', 33, 'PR-20250807-0009', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 13:38:22'),
(63, 8, 'submit', 'submission', 33, 'PR-20250807-0009', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 13:38:22'),
(64, 8, 'create', 'submission', 34, 'PR-20250807-0010', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 13:41:31'),
(65, 8, 'submit', 'submission', 34, 'PR-20250807-0010', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 13:41:31'),
(66, 8, 'create', 'submission', 35, 'PR-20250807-0011', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 13:53:27'),
(67, 8, 'submit', 'submission', 35, 'PR-20250807-0011', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 13:53:27'),
(68, 8, 'create', 'submission', 36, 'PR-20250807-0012', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 13:58:00'),
(69, 8, 'submit', 'submission', 36, 'PR-20250807-0012', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 13:58:00'),
(70, 8, 'create', 'submission', 37, 'PR-20250807-0013', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-07 14:01:47'),
(71, 8, 'submit', 'submission', 37, 'PR-20250807-0013', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-07 14:01:47'),
(72, 8, 'approve', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-13 12:54:33'),
(73, 8, 'reject', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'reject submission', '2025-08-13 12:56:18'),
(74, 8, 'update', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-13 12:56:39'),
(75, 8, 'approve', 'submission', 2, 'PR-20250731-0002', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-15 12:03:43'),
(76, 8, 'reject', 'submission', 2, 'PR-20250731-0002', NULL, NULL, 'status', NULL, NULL, 'reject submission', '2025-08-15 12:07:09'),
(77, 8, 'update', 'submission', 2, 'PR-20250731-0002', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-15 12:13:44'),
(78, 8, 'approve', 'submission', 3, 'PR-20250731-0003', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-15 12:14:11'),
(79, 8, 'approve', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-15 17:45:19'),
(80, 8, 'update', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-15 17:45:43'),
(81, 8, 'approve', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-15 17:46:02'),
(82, 8, 'update', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-15 17:46:15'),
(83, 8, 'update', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-15 17:46:27'),
(84, 8, 'update', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-15 17:46:35'),
(85, 8, 'approve', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-15 17:54:27'),
(86, 8, 'update', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-15 17:54:48'),
(87, 8, 'update', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-15 17:54:58'),
(88, 8, 'reject', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'reject submission', '2025-08-15 17:55:11'),
(89, 8, 'approve', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-15 18:01:11'),
(90, 8, 'update', 'submission', 1, 'PR-20250731-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-16 11:27:36'),
(91, 8, 'approve', 'submission', 25, 'PR-20250807-0001', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-16 11:28:55'),
(92, 8, 'reject', 'submission', 25, 'PR-20250807-0001', NULL, NULL, 'status', NULL, NULL, 'reject submission', '2025-08-16 11:29:04'),
(93, 8, 'update', 'submission', 25, 'PR-20250807-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-16 11:29:09'),
(94, 8, 'update', 'submission', 25, 'PR-20250807-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-16 11:29:15'),
(95, 8, 'update', 'submission', 25, 'PR-20250807-0001', NULL, NULL, 'status', NULL, NULL, 'update submission', '2025-08-16 11:29:38'),
(96, 8, 'reject', 'submission', 37, 'PR-20250807-0013', NULL, NULL, 'status', NULL, NULL, 'reject submission', '2025-08-18 18:05:41'),
(97, 7, 'reject', 'submission', 37, 'PR-20250807-0013', NULL, NULL, NULL, '58.11.83.106', NULL, 'asd', '2025-08-18 18:05:41'),
(98, 7, 'approve', 'submission', 35, 'PR-20250807-0011', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-18 18:13:00'),
(99, 7, 'approve', 'submission', 35, 'PR-20250807-0011', NULL, NULL, NULL, '58.11.83.106', NULL, '', '2025-08-18 18:13:00'),
(100, 8, 'reject', 'submission', 36, 'PR-20250807-0012', NULL, NULL, 'status', NULL, NULL, 'reject submission', '2025-08-18 18:14:30'),
(101, 7, 'reject', 'submission', 36, 'PR-20250807-0012', NULL, NULL, NULL, '58.11.83.106', NULL, 'awwww', '2025-08-18 18:14:30'),
(102, 7, 'approve', 'submission', 34, 'PR-20250807-0010', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-18 22:17:00'),
(103, 7, 'approve', 'submission', 34, 'PR-20250807-0010', NULL, NULL, NULL, '58.11.83.106', NULL, '', '2025-08-18 22:17:00'),
(104, 8, 'submit', 'submission', 27, 'PR-20250807-0003', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-19 11:35:46'),
(105, 8, 'submit', 'submission', 28, 'PR-20250807-0004', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-19 11:35:52'),
(106, 8, 'submit', 'submission', 30, 'PR-20250807-0006', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-19 11:35:54'),
(107, 8, 'create', 'submission', 38, 'PR-20250819-0001', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-19 18:02:45'),
(108, 8, 'create', 'submission', 39, 'PR-20250819-0002', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-19 21:38:18'),
(109, 8, 'create', 'submission', 40, 'PR-20250820-0001', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-20 01:33:59'),
(110, 8, 'create', 'submission', 41, 'PR-20250820-0002', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-20 01:45:00'),
(111, 8, 'create', 'submission', 42, 'PR-20250820-0003', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-20 14:16:13'),
(112, 8, 'create', 'submission', 43, 'PR-20250822-0001', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-22 03:18:56'),
(113, 8, 'create', 'submission', 44, 'PR-20250822-0002', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-22 04:15:27'),
(114, 8, 'create', 'submission', 45, 'PR-20250822-0003', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-22 09:33:24'),
(115, 8, 'submit', 'submission', 45, 'PR-20250822-0003', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-22 09:34:39'),
(116, 8, 'create', 'submission', 46, 'PR-20250822-0004', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-22 09:54:13'),
(117, 8, 'submit', 'submission', 46, 'PR-20250822-0004', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-22 09:54:14'),
(118, 8, 'reject', 'submission', 46, 'PR-20250822-0004', NULL, NULL, 'status', NULL, NULL, 'reject submission', '2025-08-22 11:19:48'),
(119, 7, 'reject', 'submission', 46, 'PR-20250822-0004', NULL, NULL, NULL, '58.10.155.36', NULL, 'ไม่ให้', '2025-08-22 11:19:48'),
(120, 7, 'approve', 'submission', 45, 'PR-20250822-0003', NULL, NULL, 'status', NULL, NULL, 'approve submission', '2025-08-22 13:28:49'),
(121, 7, 'approve', 'submission', 45, 'PR-20250822-0003', NULL, NULL, NULL, '58.10.155.36', NULL, 'This is Approval Comment.\n', '2025-08-22 13:28:49'),
(122, 8, 'create', 'submission', 47, 'PR-20250822-0005', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-22 16:40:11'),
(123, 8, 'submit', 'submission', 47, 'PR-20250822-0005', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-22 16:40:11'),
(124, 8, 'create', 'submission', 48, 'PR-20250823-0001', NULL, '{\"submission_type\": \"publication_reward\", \"status_id\": 1, \"year_id\": 3}', NULL, NULL, NULL, 'Created new publication_reward', '2025-08-23 07:27:47'),
(125, 8, 'submit', 'submission', 48, 'PR-20250823-0001', NULL, NULL, '', NULL, NULL, 'submit submission', '2025-08-23 07:27:48');

-- --------------------------------------------------------

--
-- Table structure for table `document_types`
--

CREATE TABLE `document_types` (
  `document_type_id` int(11) NOT NULL,
  `document_type_name` varchar(255) DEFAULT NULL,
  `code` varchar(50) DEFAULT NULL,
  `category` varchar(50) DEFAULT 'general',
  `required` tinyint(1) DEFAULT 0,
  `multiple` tinyint(1) DEFAULT 0,
  `document_order` int(11) DEFAULT 0,
  `is_required` enum('yes','no') DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp(),
  `delete_at` datetime DEFAULT NULL,
  `fund_types` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL COMMENT 'ประเภททุนที่ใช้ได้ ["publication_reward", "fund_application"]' CHECK (json_valid(`fund_types`)),
  `subcategory_ids` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL COMMENT 'รหัส subcategory เฉพาะ [1,2,3] หรือ NULL = ทุก subcategory' CHECK (json_valid(`subcategory_ids`))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

--
-- Dumping data for table `document_types`
--

INSERT INTO `document_types` (`document_type_id`, `document_type_name`, `code`, `category`, `required`, `multiple`, `document_order`, `is_required`, `create_at`, `update_at`, `delete_at`, `fund_types`, `subcategory_ids`) VALUES
(1, 'QS WUR 1-400', 'qs_wur_1-400', '', 0, 0, 1, NULL, NULL, NULL, NULL, NULL, NULL),
(2, 'Full reprint (บทความตีพิมพ์)', 'full_reprint_(บทความตีพิมพ์)', 'publication', 1, 0, 2, NULL, NULL, NULL, NULL, NULL, NULL),
(3, 'Scopus-ISI (หลักฐานการจัดอันดับ)', 'scopus-isi_(หลักฐานการจัดอันดับ)', 'publication', 1, 0, 3, NULL, NULL, NULL, NULL, NULL, NULL),
(4, 'สำเนาบัญชีธนาคาร', 'สำเนาบัญชีธนาคาร', '', 1, 0, 4, NULL, NULL, NULL, NULL, NULL, NULL),
(5, 'Payment / Exchange rate', 'payment_/_exchange_rate', 'publication', 0, 0, 5, NULL, NULL, NULL, NULL, NULL, NULL),
(6, 'Page charge Invoice', 'page_charge_invoice', '', 0, 0, 6, NULL, NULL, NULL, NULL, NULL, NULL),
(7, 'Page charge Receipt', 'page_charge_receipt', 'publication', 0, 0, 7, NULL, NULL, NULL, NULL, NULL, NULL),
(8, 'Manuscript Editor Invoice', 'manuscript_editor_invoice', 'publication', 0, 0, 8, NULL, NULL, NULL, NULL, NULL, NULL),
(9, 'Manuscript Receipt', 'manuscript_receipt', '', 0, 0, 9, NULL, NULL, NULL, NULL, NULL, NULL),
(10, 'Review Response (Special issue)', 'review_response_(special_issue)', '', 0, 0, 10, NULL, NULL, NULL, NULL, NULL, NULL),
(11, 'เอกสารอื่นๆ', 'เอกสารอื่นๆ', 'publication', 0, 1, 11, NULL, NULL, NULL, NULL, NULL, NULL),
(12, 'เอกสารเบิกจ่ายภายนอก', 'เอกสารเบิกจ่ายภายนอก', 'publication', 0, 1, 12, 'no', NULL, NULL, NULL, NULL, NULL);

-- --------------------------------------------------------

--
-- Table structure for table `file_uploads`
--

CREATE TABLE `file_uploads` (
  `file_id` int(11) NOT NULL,
  `original_name` varchar(255) NOT NULL,
  `stored_path` varchar(500) NOT NULL,
  `folder_type` enum('temp','submission','profile','other') DEFAULT 'temp',
  `submission_id` int(11) DEFAULT NULL,
  `file_size` bigint(20) DEFAULT NULL,
  `mime_type` varchar(100) DEFAULT NULL,
  `file_hash` varchar(64) DEFAULT NULL,
  `is_public` tinyint(1) DEFAULT 0,
  `uploaded_by` int(11) DEFAULT NULL,
  `uploaded_at` datetime DEFAULT current_timestamp(),
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

--
-- Dumping data for table `file_uploads`
--

INSERT INTO `file_uploads` (`file_id`, `original_name`, `stored_path`, `folder_type`, `submission_id`, `file_size`, `mime_type`, `file_hash`, `is_public`, `uploaded_by`, `uploaded_at`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub1_2025-07-31/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 12:58:57', '2025-07-31 12:58:57', '2025-07-31 12:58:57', NULL),
(2, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub1_2025-07-31/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 12:58:57', '2025-07-31 12:58:57', '2025-07-31 12:58:57', NULL),
(3, 'pdf-sample_3.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub1_2025-07-31/pdf-sample_3.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 12:58:57', '2025-07-31 12:58:57', '2025-07-31 12:58:57', NULL),
(4, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub1_2025-07-31/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 12:58:57', '2025-07-31 12:58:57', '2025-07-31 12:58:57', NULL),
(5, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub2_2025-07-31/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 13:28:33', '2025-07-31 13:28:33', '2025-07-31 13:28:34', NULL),
(6, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub2_2025-07-31/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 13:28:34', '2025-07-31 13:28:34', '2025-07-31 13:28:34', NULL),
(7, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub3_2025-07-31/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 13:38:11', '2025-07-31 13:38:11', '2025-07-31 13:38:11', NULL),
(8, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub3_2025-07-31/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 13:38:11', '2025-07-31 13:38:11', '2025-07-31 13:38:11', NULL),
(9, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub4_2025-07-31/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 13:42:38', '2025-07-31 13:42:38', '2025-07-31 13:42:38', NULL),
(10, 'pdf-sample_3.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub4_2025-07-31/pdf-sample_3.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 13:42:38', '2025-07-31 13:42:38', '2025-07-31 13:42:38', NULL),
(11, 'pdf-sample_3.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub5_2025-07-31/pdf-sample_3.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-07-31 13:44:25', '2025-07-31 13:44:25', '2025-07-31 13:44:25', NULL),
(12, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub6_2025-08-01/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:24:29', '2025-08-01 10:24:29', '2025-08-01 10:24:29', NULL),
(13, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub6_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:24:29', '2025-08-01 10:24:29', '2025-08-01 10:24:29', NULL),
(14, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub7_2025-08-01/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:25:34', '2025-08-01 10:25:34', '2025-08-01 10:25:34', NULL),
(15, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub7_2025-08-01/pdf-sample_2_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:25:34', '2025-08-01 10:25:34', '2025-08-01 10:25:34', NULL),
(16, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub8_2025-08-01/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:38:16', '2025-08-01 10:38:16', '2025-08-01 10:38:16', NULL),
(17, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub8_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:38:16', '2025-08-01 10:38:16', '2025-08-01 10:38:16', NULL),
(18, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub9_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:39:09', '2025-08-01 10:39:09', '2025-08-01 10:39:09', NULL),
(19, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub9_2025-08-01/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:39:09', '2025-08-01 10:39:09', '2025-08-01 10:39:09', NULL),
(20, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub10_2025-08-01/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:42:00', '2025-08-01 10:42:00', '2025-08-01 10:42:00', NULL),
(21, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub11_2025-08-01/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 10:50:27', '2025-08-01 10:50:27', '2025-08-01 10:50:27', NULL),
(22, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub12_2025-08-01/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 11:09:50', '2025-08-01 11:09:50', '2025-08-01 11:09:50', NULL),
(23, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub12_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 11:09:50', '2025-08-01 11:09:50', '2025-08-01 11:09:50', NULL),
(24, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub13_2025-08-01/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 11:19:12', '2025-08-01 11:19:12', '2025-08-01 11:19:12', NULL),
(25, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub14_2025-08-01/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 11:22:17', '2025-08-01 11:22:17', '2025-08-01 11:22:17', NULL),
(26, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub15_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 11:23:53', '2025-08-01 11:23:53', '2025-08-01 11:23:53', NULL),
(27, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub16_2025-08-01/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 11:34:30', '2025-08-01 11:34:30', '2025-08-01 11:34:30', NULL),
(28, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub17_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 11:37:54', '2025-08-01 11:37:54', '2025-08-01 11:37:54', NULL),
(29, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub18_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 11:40:47', '2025-08-01 11:40:47', '2025-08-01 11:40:47', NULL),
(30, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub19_2025-08-01/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 12:08:25', '2025-08-01 12:08:25', '2025-08-01 12:08:25', NULL),
(31, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub19_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 12:08:25', '2025-08-01 12:08:25', '2025-08-01 12:08:25', NULL),
(32, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub20_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 12:11:19', '2025-08-01 12:11:19', '2025-08-01 12:11:19', NULL),
(33, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub21_2025-08-01/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 22:29:44', '2025-08-01 22:29:44', '2025-08-01 22:29:44', NULL),
(34, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub21_2025-08-01/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 22:29:44', '2025-08-01 22:29:44', '2025-08-01 22:29:44', NULL),
(35, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub21_2025-08-01/pdf-sample_0_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-01 22:29:44', '2025-08-01 22:29:44', '2025-08-01 22:29:44', NULL),
(36, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub22_2025-08-02/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-02 12:18:25', '2025-08-02 12:18:25', '2025-08-02 12:18:25', NULL),
(37, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub22_2025-08-02/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-02 12:18:25', '2025-08-02 12:18:25', '2025-08-02 12:18:25', NULL),
(38, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub22_2025-08-02/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-02 12:18:25', '2025-08-02 12:18:25', '2025-08-02 12:18:25', NULL),
(39, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub22_2025-08-02/pdf-sample_2_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-02 12:18:25', '2025-08-02 12:18:25', '2025-08-02 12:18:25', NULL),
(40, 'merged_documents.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub22_2025-08-02/merged_documents.pdf', 'temp', NULL, 23937, 'application/pdf', '', 0, 8, '2025-08-02 12:18:25', '2025-08-02 12:18:25', '2025-08-02 12:18:25', NULL),
(41, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub23_2025-08-05/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-05 22:41:57', '2025-08-05 22:41:57', '2025-08-05 22:41:57', NULL),
(42, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub23_2025-08-05/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-05 22:41:57', '2025-08-05 22:41:57', '2025-08-05 22:41:57', NULL),
(43, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub24_2025-08-05/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-05 22:46:59', '2025-08-05 22:46:59', '2025-08-05 22:46:59', NULL),
(44, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub24_2025-08-05/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-05 22:46:59', '2025-08-05 22:46:59', '2025-08-05 22:46:59', NULL),
(45, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub24_2025-08-05/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-05 22:46:59', '2025-08-05 22:46:59', '2025-08-05 22:46:59', NULL),
(46, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub24_2025-08-05/pdf-sample_0_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-05 22:46:59', '2025-08-05 22:46:59', '2025-08-05 22:46:59', NULL),
(47, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub25_2025-08-07/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 01:19:18', '2025-08-07 01:19:18', '2025-08-07 01:19:18', NULL),
(48, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub25_2025-08-07/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 01:19:18', '2025-08-07 01:19:18', '2025-08-07 01:19:18', NULL),
(49, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub25_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 01:19:18', '2025-08-07 01:19:18', '2025-08-07 01:19:18', NULL),
(50, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub25_2025-08-07/pdf-sample_0_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 01:19:18', '2025-08-07 01:19:18', '2025-08-07 01:19:18', NULL),
(51, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub26_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 02:34:34', '2025-08-07 02:34:34', '2025-08-07 02:34:34', NULL),
(52, 'pdf-sample_3.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub26_2025-08-07/pdf-sample_3.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 02:34:34', '2025-08-07 02:34:34', '2025-08-07 02:34:34', NULL),
(53, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub29_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 03:28:42', '2025-08-07 03:28:42', '2025-08-07 03:28:42', NULL),
(54, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub29_2025-08-07/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 03:28:42', '2025-08-07 03:28:42', '2025-08-07 03:28:42', NULL),
(55, 'merged_documents.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub29_2025-08-07/merged_documents.pdf', 'temp', NULL, 23937, 'application/pdf', '', 0, 8, '2025-08-07 03:28:43', '2025-08-07 03:28:43', '2025-08-07 03:28:43', NULL),
(56, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub31_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 06:11:59', '2025-08-07 06:11:59', '2025-08-07 06:11:59', NULL),
(57, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub31_2025-08-07/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 06:11:59', '2025-08-07 06:11:59', '2025-08-07 06:11:59', NULL),
(58, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub32_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:34:30', '2025-08-07 13:34:30', '2025-08-07 13:34:30', NULL),
(59, 'pdf-sample_3.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub32_2025-08-07/pdf-sample_3.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:34:30', '2025-08-07 13:34:30', '2025-08-07 13:34:30', NULL),
(60, 'pdf-sample_2.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub33_2025-08-07/pdf-sample_2.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:38:22', '2025-08-07 13:38:22', '2025-08-07 13:38:22', NULL),
(61, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub33_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:38:22', '2025-08-07 13:38:22', '2025-08-07 13:38:22', NULL),
(62, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub34_2025-08-07/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:41:31', '2025-08-07 13:41:31', '2025-08-07 13:41:31', NULL),
(63, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub34_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:41:31', '2025-08-07 13:41:31', '2025-08-07 13:41:31', NULL),
(64, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub35_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:53:27', '2025-08-07 13:53:27', '2025-08-07 13:53:27', NULL),
(65, 'pdf-sample_3.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub35_2025-08-07/pdf-sample_3.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:53:27', '2025-08-07 13:53:27', '2025-08-07 13:53:27', NULL),
(66, 'pdf-sample_3.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub36_2025-08-07/pdf-sample_3.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:58:00', '2025-08-07 13:58:00', '2025-08-07 13:58:00', NULL),
(67, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub36_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:58:00', '2025-08-07 13:58:00', '2025-08-07 13:58:00', NULL),
(68, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub36_2025-08-07/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:58:00', '2025-08-07 13:58:00', '2025-08-07 13:58:00', NULL),
(69, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub36_2025-08-07/pdf-sample_1_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 13:58:00', '2025-08-07 13:58:00', '2025-08-07 13:58:00', NULL),
(70, 'pdf-sample_0.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub37_2025-08-07/pdf-sample_0.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 14:01:47', '2025-08-07 14:01:47', '2025-08-07 14:01:47', NULL),
(71, 'pdf-sample_1.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub37_2025-08-07/pdf-sample_1.pdf', 'temp', NULL, 13264, 'application/pdf', '', 0, 8, '2025-08-07 14:01:47', '2025-08-07 14:01:47', '2025-08-07 14:01:47', NULL),
(72, 'form.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub45_2025-08-22/form.pdf', 'temp', NULL, 1312, 'application/pdf', '', 0, 8, '2025-08-22 09:34:39', '2025-08-22 09:34:39', '2025-08-22 09:34:39', NULL),
(73, 'form.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub45_2025-08-22/form_1.pdf', 'temp', NULL, 1312, 'application/pdf', '', 0, 8, '2025-08-22 09:34:39', '2025-08-22 09:34:39', '2025-08-22 09:34:39', NULL),
(74, 'form.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub45_2025-08-22/form_2.pdf', 'temp', NULL, 1312, 'application/pdf', '', 0, 8, '2025-08-22 09:34:39', '2025-08-22 09:34:39', '2025-08-22 09:34:39', NULL),
(75, 'form.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub45_2025-08-22/form_3.pdf', 'temp', NULL, 1312, 'application/pdf', '', 0, 8, '2025-08-22 09:34:39', '2025-08-22 09:34:39', '2025-08-22 09:34:39', NULL),
(76, 'merged_documents.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub45_2025-08-22/merged_documents.pdf', 'temp', NULL, 2307, 'application/pdf', '', 0, 8, '2025-08-22 09:34:39', '2025-08-22 09:34:39', '2025-08-22 09:34:39', NULL),
(77, 'form.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub46_2025-08-22/form.pdf', 'temp', NULL, 1312, 'application/pdf', '', 0, 8, '2025-08-22 09:54:13', '2025-08-22 09:54:13', '2025-08-22 09:54:13', NULL),
(78, 'form.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub46_2025-08-22/form_1.pdf', 'temp', NULL, 1312, 'application/pdf', '', 0, 8, '2025-08-22 09:54:13', '2025-08-22 09:54:13', '2025-08-22 09:54:13', NULL),
(79, 'form.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub47_2025-08-22/form.pdf', 'temp', NULL, 1312, 'application/pdf', '', 0, 8, '2025-08-22 16:40:11', '2025-08-22 16:40:11', '2025-08-22 16:40:11', NULL),
(80, 'file-sample_150kB.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub48_2025-08-23/file-sample_150kB.pdf', 'temp', NULL, 142786, 'application/pdf', '', 0, 8, '2025-08-23 07:27:47', '2025-08-23 07:27:47', '2025-08-23 07:27:47', NULL),
(81, 'sample.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub48_2025-08-23/sample.pdf', 'temp', NULL, 18810, 'application/pdf', '', 0, 8, '2025-08-23 07:27:47', '2025-08-23 07:27:47', '2025-08-23 07:27:47', NULL),
(82, 'sample-local-pdf.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub48_2025-08-23/sample-local-pdf.pdf', 'temp', NULL, 49672, 'application/pdf', '', 0, 8, '2025-08-23 07:27:47', '2025-08-23 07:27:47', '2025-08-23 07:27:47', NULL),
(83, 'c4611_sample_explain.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub48_2025-08-23/c4611_sample_explain.pdf', 'temp', NULL, 88226, 'application/pdf', '', 0, 8, '2025-08-23 07:27:47', '2025-08-23 07:27:47', '2025-08-23 07:27:47', NULL),
(84, 'file-example_PDF_500_kB.pdf', 'uploads/users/user_8_สมชาย_ใจด/submissions/pub48_2025-08-23/file-example_PDF_500_kB.pdf', 'temp', NULL, 469513, 'application/pdf', '', 0, 8, '2025-08-23 07:27:47', '2025-08-23 07:27:47', '2025-08-23 07:27:47', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `fund_application_details`
--

CREATE TABLE `fund_application_details` (
  `detail_id` int(11) NOT NULL,
  `submission_id` int(11) NOT NULL,
  `subcategory_id` int(11) NOT NULL,
  `project_title` varchar(255) DEFAULT NULL,
  `project_description` text DEFAULT NULL,
  `requested_amount` decimal(15,2) DEFAULT NULL,
  `approved_amount` decimal(15,2) DEFAULT NULL,
  `approved_by` int(11) DEFAULT NULL,
  `approved_at` datetime DEFAULT NULL,
  `rejected_by` int(11) DEFAULT NULL,
  `rejected_at` datetime DEFAULT NULL,
  `closed_at` datetime DEFAULT NULL,
  `comment` text DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `fund_categories`
--

CREATE TABLE `fund_categories` (
  `category_id` int(11) NOT NULL,
  `category_name` varchar(255) DEFAULT NULL,
  `status` enum('active','disable') DEFAULT NULL,
  `year_id` int(11) DEFAULT NULL,
  `comment` text DEFAULT NULL,
  `create_at` datetime DEFAULT NULL,
  `update_at` datetime DEFAULT NULL,
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

--
-- Dumping data for table `fund_categories`
--

INSERT INTO `fund_categories` (`category_id`, `category_name`, `status`, `year_id`, `comment`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 'ทุนส่งเสริมการวิจัย', 'active', 1, NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(2, 'ทุนอุดหนุนกิจกรรม', 'active', 1, NULL, '2025-06-24 16:49:13', '2025-08-10 23:25:26', NULL),
(5, 'ฟหก', 'active', 1, NULL, '2025-08-11 13:39:17', '2025-08-11 13:39:17', '2025-08-11 13:39:30');

-- --------------------------------------------------------

--
-- Table structure for table `fund_forms`
--

CREATE TABLE `fund_forms` (
  `form_id` int(11) NOT NULL,
  `title` varchar(255) NOT NULL COMMENT 'ชื่อแบบฟอร์ม',
  `description` text DEFAULT NULL COMMENT 'รายละเอียดแบบฟอร์ม',
  `file_name` varchar(255) NOT NULL COMMENT 'ชื่อไฟล์ต้นฉบับ',
  `file_path` varchar(500) NOT NULL COMMENT 'path ไฟล์ในระบบ',
  `file_size` bigint(20) DEFAULT NULL COMMENT 'ขนาดไฟล์ (bytes)',
  `mime_type` varchar(100) DEFAULT NULL COMMENT 'ประเภทไฟล์',
  `form_type` enum('application','report','evaluation','guidelines','other') DEFAULT 'application' COMMENT 'ประเภทแบบฟอร์ม',
  `fund_category` enum('research_fund','promotion_fund','both') DEFAULT 'both' COMMENT 'หมวดหมู่กองทุน',
  `is_required` tinyint(1) DEFAULT 0 COMMENT 'บังคับใช้หรือไม่',
  `status` enum('active','inactive','archived') DEFAULT 'active' COMMENT 'สถานะแบบฟอร์ม',
  `created_by` int(11) NOT NULL COMMENT 'ผู้สร้าง (user_id)',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci COMMENT='ตารางเก็บแบบฟอร์มและเอกสารที่เกี่ยวข้องกับการขอทุน';

--
-- Dumping data for table `fund_forms`
--

INSERT INTO `fund_forms` (`form_id`, `title`, `description`, `file_name`, `file_path`, `file_size`, `mime_type`, `form_type`, `fund_category`, `is_required`, `status`, `created_by`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 'แบบฟอร์มสมัครทุนส่งเสริมการวิจัย', 'แบบฟอร์มสำหรับสมัครขอรับทุนส่งเสริมการวิจัย', 'แบบฟอร์มสมัครทุนวิจัย.docx', 'uploads/fund_forms/research/แบบฟอร์มสมัครทุนวิจัย.docx', 512000, 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', 'application', 'research_fund', 1, 'active', 1, '2025-08-20 11:40:31', '2025-08-20 11:40:31', NULL),
(2, 'แบบฟอร์มรายงานความก้าวหน้าโครงการวิจัย', 'แบบฟอร์มสำหรับรายงานความก้าวหน้าของโครงการวิจัย', 'แบบฟอร์มรายงานความก้าวหน้า.docx', 'uploads/fund_forms/research/แบบฟอร์มรายงานความก้าวหน้า.docx', 480000, 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', 'report', 'research_fund', 1, 'active', 1, '2025-08-20 11:40:31', '2025-08-20 11:40:31', NULL),
(3, 'แบบฟอร์มสมัครทุนอุดหนุนกิจกรรม', 'แบบฟอร์มสำหรับสมัครขอรับทุนอุดหนุนกิจกรรม', 'แบบฟอร์มสมัครทุนกิจกรรม.docx', 'uploads/fund_forms/promotion/แบบฟอร์มสมัครทุนกิจกรรม.docx', 600000, 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', 'application', 'promotion_fund', 1, 'active', 1, '2025-08-20 11:40:31', '2025-08-20 11:40:31', NULL),
(4, 'แบบประเมินผลกิจกรรม', 'แบบฟอร์มสำหรับประเมินผลการดำเนินกิจกรรม', 'แบบประเมินผลกิจกรรม.xlsx', 'uploads/fund_forms/promotion/แบบประเมินผลกิจกรรม.xlsx', 256000, 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 'evaluation', 'promotion_fund', 0, 'active', 1, '2025-08-20 11:40:31', '2025-08-20 11:40:31', NULL),
(5, 'แนวทางการเขียนข้อเสนอโครงการ', 'เอกสารแนวทางและข้อแนะนำสำหรับการเขียนข้อเสนอโครงการ', 'แนวทางการเขียนข้อเสนอโครงการ.pdf', 'uploads/fund_forms/guidelines/แนวทางการเขียนข้อเสนอโครงการ.pdf', 1024000, 'application/pdf', 'guidelines', 'both', 0, 'inactive', 1, '2025-08-20 11:40:31', '2025-08-23 09:49:06', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `fund_subcategories`
--

CREATE TABLE `fund_subcategories` (
  `subcategory_id` int(11) NOT NULL,
  `category_id` int(11) DEFAULT NULL,
  `subcategory_name` varchar(255) DEFAULT NULL,
  `fund_condition` text DEFAULT NULL,
  `target_roles` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL COMMENT 'บทบาทที่สามารถเห็นทุนนี้ได้ (เก็บเป็น JSON array)',
  `form_type` varchar(50) DEFAULT 'download' COMMENT 'ประเภทฟอร์ม: download, publication_reward, research_proposal, etc.',
  `form_url` varchar(255) DEFAULT NULL COMMENT 'URL สำหรับดาวน์โหลดฟอร์ม (ถ้า form_type = download)',
  `status` enum('active','disable') DEFAULT NULL,
  `comment` text DEFAULT NULL,
  `create_at` datetime DEFAULT NULL,
  `update_at` datetime DEFAULT NULL,
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

--
-- Dumping data for table `fund_subcategories`
--

INSERT INTO `fund_subcategories` (`subcategory_id`, `category_id`, `subcategory_name`, `fund_condition`, `target_roles`, `form_type`, `form_url`, `status`, `comment`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 1, '1.1 ทุนสนับสนุนผู้เชี่ยวชาญต่างประเทศ', 'ผู้ได้รับทุนสนับสนุนผู้เชี่ยวชาญต่างประเทศ ต้องเผยแพร่บทความในฐานข้อมูลระดับ WOS หรือ ISI หรือ SCOPUS ในควอร์ไทล์ที่ 1 (Q1)', '', 'download', 'http://147.50.230.213:8080/uploads/form.pdf', 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(2, 1, '1.2 ทุนวิจัยสถาบัน', 'ผู้ได้รับทุนสนับสนุนวิจัยสถาบัน ทุนวิจัยเพื่องานประจำ หรือทุนวิจัยในชั้นเรียนจะต้องมีชิ้นงาน', '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(3, 1, '1.3 ทุนวิจัยเพื่อพัฒนางานประจํา', NULL, '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(4, 1, '1.4 ทุนวิจัยในชั้นเรียน', NULL, '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(5, 1, 'ทุนสนับสนุนงานวิจัย นวัตกรรมและสิ่งประดิษฐ์เพื่อการเรียนการสอน', NULL, '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(6, 1, '1.5 ทุนวิจัยความเป็นเลิศ', 'ผู้ได้รับทุนวิจัยความเป็นเลิศ ต้องเผยแพร่บทความในฐานข้อมูลระดับ WOS หรือ ISI หรือ SCOPUS', '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(7, 1, '1.10 ทุนพัฒนากลุ่มวิจัยบูรณาการ', 'ทุนพัฒนากลุ่มวิจัยบูรณาการ แบ่งออกเป็น 3 ระดับ', '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(8, 1, 'ทุนนักวิจัยอาวุโส', NULL, '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(9, 1, '1.7 ทุนพัฒนาศูนย์วิจัย', NULL, '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(10, 1, 'ทุนฝึกอบรมนักวิจัยหลังปริญญาเอก', 'ทุนฝึกอบรมนักวิจัยหลังปริญญาเอก เป็นการสนับสนุนนักวิจัยผ่านการรับทุนของอาจารย์', '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(11, 1, '1.6 ทุนนวัตกรรมความเป็นเลิศ', 'ผู้ได้รับทุนนวัตกรรมความเป็นเลิศ จะต้องสร้างนวัตกรรมหรือเครื่องมือ เครื่องใช้', '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(12, 1, '1.9 ทุนสนับสนุนการได้รับทุนวิจัยภายนอก', NULL, '', 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-08-08 22:57:24', NULL),
(13, 2, 'ทุนทําวิจัยในต่างประเทศ', NULL, NULL, 'download', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-08-11 11:50:06', NULL),
(14, 2, 'เงินรางวัลการตีพิมพ์เผยแพร่ผลงานวิจัยที่ได้รับการตีพิมพ์ในสาขาวิทยาศาสตร์และเทคโนโลยี (กรณีเป็นผู้แต่งชื่อแรก)', NULL, NULL, 'publication_reward', NULL, 'active', NULL, '2025-06-24 16:49:13', '2025-08-08 22:57:15', NULL),
(15, 2, 'เงินรางวัลการตีพิมพ์เผยแพร่ผลงานวิจัยที่ได้รับการตีพิมพ์ในสาขาวิทยาศาสตร์และเทคโนโลยี (กรณีเป็นผู้ประพันธ์บรรณกิจ)', NULL, '[\"1\",\"2\"]', 'publication_reward', NULL, 'active', NULL, NULL, '2025-08-11 11:49:58', NULL),
(16, 2, '123', '123', '[\"1\"]', '', '', 'active', NULL, '2025-08-08 23:04:31', '2025-08-11 12:52:11', '2025-08-11 12:52:23'),
(17, 2, 'das', NULL, '[\"2\",\"1\"]', '', '', 'active', NULL, '2025-08-11 12:52:31', '2025-08-11 13:20:08', '2025-08-11 13:56:03'),
(18, 5, 'ฟหก', NULL, NULL, '', '', 'active', NULL, '2025-08-11 13:39:22', '2025-08-11 13:39:22', '2025-08-11 13:39:28');

-- --------------------------------------------------------

--
-- Table structure for table `notifications`
--

CREATE TABLE `notifications` (
  `notification_id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `title` varchar(255) NOT NULL,
  `message` text DEFAULT NULL,
  `type` enum('info','success','warning','error') DEFAULT 'info',
  `is_read` tinyint(1) DEFAULT 0,
  `related_submission_id` int(11) DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

-- --------------------------------------------------------

--
-- Table structure for table `positions`
--

CREATE TABLE `positions` (
  `position_id` int(11) NOT NULL,
  `position_name` varchar(255) DEFAULT NULL,
  `is_active` enum('yes','no') DEFAULT 'yes',
  `create_at` datetime DEFAULT NULL,
  `update_at` datetime DEFAULT NULL,
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

--
-- Dumping data for table `positions`
--

INSERT INTO `positions` (`position_id`, `position_name`, `is_active`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 'อาจารย์', 'yes', '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(2, 'รองศาสตราจารย์', 'yes', '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(3, 'พนักงานธุรการ', 'yes', '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `publication_reward_details`
--

CREATE TABLE `publication_reward_details` (
  `detail_id` int(11) NOT NULL,
  `submission_id` int(11) NOT NULL,
  `paper_title` varchar(500) NOT NULL,
  `journal_name` varchar(255) NOT NULL,
  `publication_date` date NOT NULL,
  `publication_type` enum('journal','conference','book_chapter','other') DEFAULT 'journal',
  `quartile` enum('Q1','Q2','Q3','Q4','T5','T10','TCI','N/A') DEFAULT 'N/A',
  `impact_factor` decimal(10,3) DEFAULT NULL,
  `doi` varchar(255) DEFAULT NULL,
  `url` varchar(500) DEFAULT NULL,
  `page_numbers` varchar(50) DEFAULT NULL,
  `volume_issue` varchar(100) DEFAULT NULL,
  `indexing` varchar(255) DEFAULT NULL,
  `reward_amount` decimal(15,2) DEFAULT 0.00 COMMENT 'เงินรางวัลอ้างอิงจาก Author และ Quartile',
  `reward_approve_amount` decimal(15,2) DEFAULT 0.00 COMMENT 'จำนวนเงินรางวัลที่อนุมัติ',
  `revision_fee` decimal(15,2) DEFAULT 0.00 COMMENT 'ค่าปรับปรุง',
  `revision_fee_approve_amount` decimal(15,2) DEFAULT 0.00 COMMENT 'ค่าปรับปรุงที่ได้รับการอนุมัติ',
  `publication_fee` decimal(15,2) DEFAULT 0.00 COMMENT 'ค่าตีพิมพ์',
  `publication_fee_approve_amount` decimal(15,2) DEFAULT 0.00 COMMENT 'ค่าตีพิมพ์ที่อนุมัติ',
  `external_funding_amount` decimal(15,2) DEFAULT 0.00 COMMENT 'รวมจำนวนเงินจากทุนที่ user แนบเข้ามา',
  `total_amount` decimal(15,2) DEFAULT 0.00 COMMENT 'เกิดจากการหักลบค่าปรับปรุง+ค่าตีพิมพ์ ลบกับ รายการที่เบิกจากหน่วยงานนอก',
  `total_approve_amount` decimal(15,2) DEFAULT 0.00 COMMENT 'จำนวนเงินจริงที่วิทยาลัยจ่ายให้ (หลังจากได้รับการอนุมัติ)',
  `announce_reference_number` varchar(50) DEFAULT NULL,
  `author_count` int(11) DEFAULT 1,
  `author_type` enum('first_author','corresponding_author','coauthor') DEFAULT 'coauthor',
  `has_university_funding` enum('yes','no') DEFAULT 'no' COMMENT 'ได้รับการสนับสนุนทุนจากมหาวิทยาลัยหรือไม่',
  `funding_references` text DEFAULT NULL COMMENT 'หมายเลขอ้างอิงทุน (คั่นด้วยจุลภาค)',
  `university_rankings` text DEFAULT NULL COMMENT 'อันดับมหาวิทยาลัย/สถาบัน (คั่นด้วยจุลภาค)',
  `approved_amount` decimal(15,2) DEFAULT NULL,
  `approval_comment` text DEFAULT NULL,
  `approved_by` int(11) DEFAULT NULL,
  `approved_at` datetime DEFAULT NULL,
  `rejection_reason` text DEFAULT NULL,
  `rejected_by` int(11) DEFAULT NULL,
  `rejected_at` datetime DEFAULT NULL,
  `revision_request` text DEFAULT NULL,
  `revision_requested_by` int(11) DEFAULT NULL,
  `revision_requested_at` datetime DEFAULT NULL,
  `create_at` datetime NOT NULL DEFAULT current_timestamp(),
  `update_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='ตารางเก็บรายละเอียดการขอรับเงินรางวัลผลงานวิชาการ พร้อมข้อมูลเพิ่มเติม';

--
-- Dumping data for table `publication_reward_details`
--

INSERT INTO `publication_reward_details` (`detail_id`, `submission_id`, `paper_title`, `journal_name`, `publication_date`, `publication_type`, `quartile`, `impact_factor`, `doi`, `url`, `page_numbers`, `volume_issue`, `indexing`, `reward_amount`, `reward_approve_amount`, `revision_fee`, `revision_fee_approve_amount`, `publication_fee`, `publication_fee_approve_amount`, `external_funding_amount`, `total_amount`, `total_approve_amount`, `announce_reference_number`, `author_count`, `author_type`, `has_university_funding`, `funding_references`, `university_rankings`, `approved_amount`, `approval_comment`, `approved_by`, `approved_at`, `rejection_reason`, `rejected_by`, `rejected_at`, `revision_request`, `revision_requested_by`, `revision_requested_at`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 1, 'asd', 'asdasd', '2025-07-31', 'journal', 'Q3', '0.000', '', '', '', '', '', '20000.00', '0.00', '123.00', '0.00', '321.00', '0.00', '2.00', '20442.00', '0.00', '', 3, 'corresponding_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-13 12:56:03', '0000-00-00 00:00:00'),
(2, 2, 'asdasd', 'asd', '2025-07-31', 'journal', 'Q2', '0.000', '', '', '', '', '', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 3, 'corresponding_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(3, 3, 'asd', 'asd', '2025-07-31', 'journal', 'Q1', '0.000', '', '', '', '', '', '40000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '40000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(4, 4, 'asdasd', 'asd', '2025-07-31', 'journal', 'Q4', '0.000', '', '', '', '', '', '10000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '10000.00', '0.00', '', 3, 'corresponding_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(5, 5, 'polow', 'asd', '2025-07-31', 'journal', 'Q3', '0.000', '', '', '', '', '', '20000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '20000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(6, 6, 'ฟหกฟหก', 'ฟฟฟ', '2025-08-01', 'journal', 'T5', '0.000', '', '', '', '', '', '50000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '50000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(7, 7, 'asdasd', 'asd', '2025-08-01', 'journal', 'Q3', '0.000', '', '', '', '', '', '20000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '20000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(8, 8, 'asdasd', 'asd', '2025-08-01', 'journal', 'Q1', '0.000', '', '', '', '', '', '40000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '40000.00', '0.00', '', 3, 'corresponding_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(9, 9, 'ฟหกฟหก', 'asd', '2025-08-01', 'journal', 'Q2', '0.000', '', '', '', '', '', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(10, 10, 'ฟหกฟหก', 'mom', '2025-08-01', 'journal', 'Q2', '0.000', '', '', '', '', '', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(11, 11, 'ฟหกฟหก', 'asd', '2025-08-01', 'journal', 'Q4', '0.000', '', '', '', '', '', '10000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '10000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(12, 12, 'ฟหกฟหก', 'asd', '2025-08-01', 'journal', 'Q4', '0.000', '', '', '', '', '', '10000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '10000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(13, 13, 'ฟหกฟหก', 'asd', '2025-08-01', 'journal', 'T5', '0.000', '', '', '', '', '', '50000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '50000.00', '0.00', '', 3, 'corresponding_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(14, 14, 'polow', 'ฟฟฟ', '2025-08-01', 'journal', 'T10', '0.000', '', '', '', '', '', '45000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '45000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(15, 15, 'ฟหกฟหก', 'asd', '2025-08-01', 'journal', 'Q1', '0.000', '', '', '', '', '', '40000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '40000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(16, 16, 'polow', 'asd', '2025-08-01', 'journal', 'T10', '0.000', '', '', '', '', '', '45000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '45000.00', '0.00', '', 3, 'corresponding_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(17, 17, 'ฟหกฟหก', 'mom', '2025-08-01', 'journal', 'Q4', '0.000', '', '', '', '', '', '10000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '10000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(18, 18, 'polow', 'ฟฟฟ', '2025-08-01', 'journal', 'Q2', '0.000', '', '', '', '', '', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 3, 'corresponding_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(19, 19, 'ฟหกฟหก', 'ฟหกฟหกฟหก', '2025-08-01', 'journal', 'Q1', '0.000', '', '', '', '', '', '40000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '40000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(20, 20, 'ฟหกฟหก', 'ฟหกฟหกฟหก', '2025-08-01', 'journal', 'Q2', '0.000', '', '', '', '', '', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(21, 21, 'ฟหกฟหก', 'asdasd', '2025-03-01', 'journal', 'T10', '0.000', '123', '123', '123', '123', '', '45000.00', '0.00', '123.00', '0.00', '123.00', '0.00', '500.00', '44746.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(22, 22, 'polow', 'asdasd', '2025-02-01', 'journal', 'Q3', '0.000', '', '', '', '', '', '20000.00', '0.00', '5000.00', '0.00', '27000.00', '0.00', '5000.00', '47000.00', '0.00', '', 1, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(23, 23, 'ฟหกฟหก', 'asdasd', '2025-01-01', 'journal', 'Q2', '0.000', '123', '123', '123', '123', '', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(24, 24, 'ฟหกฟหก', 'asdasd', '2025-03-01', 'journal', 'T5', '0.000', '', '', '', '123', '', '50000.00', '0.00', '1000.00', '0.00', '5000.00', '0.00', '300.00', '55700.00', '0.00', '', 2, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(25, 25, 'Article Title', 'Journal Name', '2025-01-01', 'journal', 'T5', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'TCI', '50000.00', '0.00', '4600.00', '0.00', '7800.00', '0.00', '500.00', '61900.00', '0.00', '', 3, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-16 11:30:24', '0000-00-00 00:00:00'),
(26, 26, 'Article Title', 'Journal Name', '2025-03-01', 'journal', 'Q2', '0.000', '10.1016/j', 'https://example.ac.th', '123', '123', 'ISI, Web of Science, TCI', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 2, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(27, 29, 'Article Title', 'Journal Name', '2025-04-01', 'journal', 'Q1', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'ISI, Web of Science', '40000.00', '0.00', '123.00', '0.00', '321.00', '0.00', '123.00', '40321.00', '0.00', '', 2, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(28, 30, 'Article Title', 'Journal Name', '2025-05-01', 'journal', 'Q1', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'Scopus, Web of Science', '40000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '40000.00', '0.00', '', 2, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(29, 31, 'Article Title', 'Journal Name', '2025-05-01', 'journal', 'Q1', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'Scopus, Web of Science', '40000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '40000.00', '0.00', '', 2, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(30, 32, 'Article Title', 'Journal Name', '2025-03-01', 'journal', 'Q2', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'ISI, Web of Science', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 2, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(31, 33, 'Article Title', 'Journal Name', '2025-06-01', 'journal', 'Q4', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'ISI, Web of Science', '10000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '10000.00', '0.00', '', 1, 'first_author', 'no', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(32, 34, 'Article Title', 'Journal Name', '2025-07-01', 'journal', 'Q2', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'ISI, TCI', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 2, 'first_author', 'no', NULL, NULL, '12000.00', '', 7, '2025-08-18 22:17:00', NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-18 22:17:00', '0000-00-00 00:00:00'),
(33, 35, 'Article Title', 'Journal Name', '2025-05-01', 'journal', 'Q2', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'Scopus, TCI', '30000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '30000.00', '0.00', '', 2, 'first_author', 'no', NULL, NULL, '1200.00', '', 7, '2025-08-18 18:13:00', NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-18 18:13:00', '0000-00-00 00:00:00'),
(34, 36, 'Article Title', 'Journal Name', '2025-03-01', 'journal', 'T10', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'Scopus, Web of Science', '45000.00', '0.00', '1500.00', '0.00', '20000.00', '0.00', '900.00', '65600.00', '0.00', '', 2, 'first_author', 'yes', 'PR-20250731-0001', 'QS World University Rankings #543', NULL, NULL, NULL, NULL, 'awwww', 7, '2025-08-18 18:14:30', NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-18 18:14:30', '0000-00-00 00:00:00'),
(35, 37, 'Article Title', 'Journal Name', '2025-05-01', 'journal', 'Q3', '0.000', '10.1016/j', '', '123-145', 'Vol.10', 'ISI, TCI', '20000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '20000.00', '0.00', '', 1, 'corresponding_author', 'no', '', 'QS World University Rankings #543', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-12 10:46:01', '2025-08-12 10:46:01', '0000-00-00 00:00:00'),
(36, 45, 'Test Article', 'Test Journal', '2025-01-01', 'journal', 'T5', '0.000', '10.10.10', 'www.test.com', '123-456', 'Vol.Test', 'ISI', '50000.00', '0.00', '2000.00', '0.00', '3000.00', '0.00', '5000.00', '50000.00', '0.00', '', 3, 'first_author', 'yes', 'F-123456', 'QS 500', '50000.00', 'This is Approval Comment.\n', 7, '2025-08-22 13:28:49', NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-22 09:34:39', '2025-08-22 13:28:49', NULL),
(37, 46, 'Test Article', 'Test Journal', '2025-01-01', 'journal', 'T5', '0.000', '10.10.10', 'www.test.com', '123-456', 'Vol.Test', 'ISI, Scopus, Web of Science, TCI', '50000.00', '0.00', '0.00', '0.00', '0.00', '0.00', '0.00', '50000.00', '0.00', '', 3, 'first_author', 'no', '', '', NULL, NULL, NULL, NULL, 'ไม่ให้', 7, '2025-08-22 11:19:48', NULL, NULL, NULL, '2025-08-22 09:54:13', '2025-08-22 11:19:48', NULL),
(38, 47, 'Test Article Title', 'Test Journal Name', '2025-01-01', 'journal', 'T5', '0.000', '10.10.16', 'test.artical.url', '123-456', 'Vol.Test', 'ISI', '50000.00', '0.00', '5000.00', '0.00', '5000.00', '0.00', '1000.00', '59000.00', '0.00', '', 3, 'corresponding_author', 'no', '', '', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-22 16:40:11', '2025-08-22 16:40:11', NULL),
(39, 48, 'Test Article Title', 'Test Journal Name', '2025-12-01', 'journal', 'T10', '0.000', '10.1016/j', 'https://example.ac.th', '123-145', 'Vol.10', 'ISI, Scopus, Web of Science, TCI', '45000.00', '0.00', '3700.00', '0.00', '15000.00', '0.00', '0.00', '63700.00', '0.00', '', 3, 'first_author', 'no', '', 'QS World University Rankings #543', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2025-08-23 07:27:47', '2025-08-23 07:27:47', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `publication_reward_rates`
--

CREATE TABLE `publication_reward_rates` (
  `rate_id` int(11) NOT NULL,
  `year` varchar(4) NOT NULL,
  `author_status` enum('first_author','corresponding_author') NOT NULL,
  `journal_quartile` enum('Q1','Q2','Q3','Q4','T5','T10','TCI','N/A') NOT NULL,
  `reward_amount` decimal(15,2) NOT NULL COMMENT 'จำนวนเงินรางวัล',
  `is_active` tinyint(1) DEFAULT 1,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

--
-- Dumping data for table `publication_reward_rates`
--

INSERT INTO `publication_reward_rates` (`rate_id`, `year`, `author_status`, `journal_quartile`, `reward_amount`, `is_active`, `create_at`, `update_at`) VALUES
(1, '2568', 'first_author', 'Q1', '40000.00', 1, '2025-07-02 09:35:58', '2025-08-08 16:07:17'),
(2, '2568', 'first_author', 'Q2', '30000.00', 1, '2025-07-02 09:35:58', '2025-08-04 15:33:28'),
(3, '2568', 'first_author', 'Q3', '20000.00', 1, '2025-07-02 09:35:58', '2025-08-04 15:33:29'),
(4, '2568', 'first_author', 'Q4', '10000.00', 1, '2025-07-02 09:35:58', '2025-08-04 15:33:30'),
(5, '2568', 'corresponding_author', 'Q1', '40000.00', 1, '2025-07-02 09:35:58', '2025-08-04 15:33:31'),
(6, '2568', 'corresponding_author', 'Q2', '30000.00', 1, '2025-07-02 09:35:58', '2025-08-04 15:33:31'),
(7, '2568', 'corresponding_author', 'Q3', '20000.00', 1, '2025-07-02 09:35:58', '2025-08-04 15:33:32'),
(8, '2568', 'corresponding_author', 'Q4', '10000.00', 1, '2025-07-02 09:35:58', '2025-08-04 15:33:33'),
(9, '2568', 'first_author', 'T5', '50000.00', 1, '2025-08-04 12:17:36', '2025-08-04 15:33:33'),
(10, '2568', 'first_author', 'T10', '45000.00', 1, '2025-08-04 12:18:14', '2025-08-04 15:33:34'),
(11, '2568', 'first_author', 'TCI', '5000.00', 1, '2025-08-04 12:18:24', '2025-08-04 15:33:35'),
(12, '2568', 'corresponding_author', 'T5', '50000.00', 1, '2025-08-04 12:19:45', '2025-08-04 15:33:36'),
(13, '2568', 'corresponding_author', 'T10', '45000.00', 1, '2025-08-04 12:19:54', '2025-08-04 15:33:37'),
(14, '2568', 'corresponding_author', 'TCI', '5000.00', 1, '2025-08-04 12:20:04', '2025-08-04 15:33:38'),
(15, '2569', 'first_author', 'Q1', '40000.00', 1, '2025-08-08 02:50:39', '2025-08-08 02:50:39'),
(16, '2569', 'corresponding_author', 'Q2', '30000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(17, '2569', 'first_author', 'Q3', '20000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(18, '2569', 'first_author', 'Q4', '10000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(19, '2569', 'corresponding_author', 'Q3', '20000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(20, '2569', 'first_author', 'Q2', '30000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(21, '2569', 'corresponding_author', 'Q1', '40000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(22, '2569', 'corresponding_author', 'Q4', '10000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(23, '2569', 'first_author', 'T5', '50000.00', 1, '2025-08-08 02:50:40', '2025-08-08 17:14:51'),
(24, '2569', 'first_author', 'T10', '45000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(25, '2569', 'corresponding_author', 'T5', '50000.00', 1, '2025-08-08 02:50:40', '2025-08-08 21:02:41'),
(26, '2569', 'corresponding_author', 'T10', '45000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(27, '2569', 'first_author', 'TCI', '5000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40'),
(28, '2569', 'corresponding_author', 'TCI', '5000.00', 1, '2025-08-08 02:50:40', '2025-08-08 02:50:40');

-- --------------------------------------------------------

--
-- Table structure for table `reward_config`
--

CREATE TABLE `reward_config` (
  `config_id` int(11) NOT NULL,
  `year` varchar(4) NOT NULL COMMENT 'ปีงบประมาณ (พ.ศ.)',
  `journal_quartile` enum('Q1','Q2','Q3','Q4','T5','T10','TCI','N/A') DEFAULT NULL COMMENT 'ระดับ Quartile ของวารสาร',
  `max_amount` decimal(15,2) NOT NULL DEFAULT 0.00 COMMENT 'จำนวนเงินสูงสุดที่รับสนับสนุน',
  `condition_description` text DEFAULT NULL COMMENT 'เงื่อนไขเพิ่มเติม',
  `is_active` tinyint(1) DEFAULT 1 COMMENT 'สถานะการใช้งาน',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

--
-- Dumping data for table `reward_config`
--

INSERT INTO `reward_config` (`config_id`, `year`, `journal_quartile`, `max_amount`, `condition_description`, `is_active`, `create_at`, `update_at`, `delete_at`) VALUES
(1, '2568', 'T5', '50000.00', 'วงเงินสูงสุดสำหรับ T5 วารสาร', 1, '2025-08-04 15:33:53', '2025-08-08 12:16:38', NULL),
(2, '2568', 'T10', '40000.00', 'วงเงินสูงสุดสำหรับ T10 วารสาร', 1, '2025-08-04 15:33:53', '2025-08-04 15:35:55', NULL),
(3, '2568', 'Q1', '30000.00', 'วงเงินสูงสุดสำหรับ Q1 วารสาร', 1, '2025-08-04 15:33:53', '2025-08-04 15:37:32', NULL),
(4, '2568', 'Q2', '20000.00', 'วงเงินสูงสุดสำหรับ Q2 วารสาร', 0, '2025-08-04 15:33:53', '2025-08-08 15:55:07', NULL),
(5, '2568', 'Q3', '15000.00', 'วงเงินสูงสุดสำหรับ Q3 วารสาร', 0, '2025-08-04 15:33:53', '2025-08-04 15:37:44', NULL),
(6, '2568', 'Q4', '10000.00', 'วงเงินสูงสุดสำหรับ Q4 วารสาร', 0, '2025-08-04 15:33:53', '2025-08-04 15:37:46', NULL),
(7, '2568', 'TCI', '5000.00', 'วงเงินสูงสุดสำหรับ TCI วารสาร', 0, '2025-08-04 15:33:53', '2025-08-04 15:37:49', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `roles`
--

CREATE TABLE `roles` (
  `role_id` int(11) NOT NULL,
  `role` varchar(255) DEFAULT NULL,
  `create_at` datetime DEFAULT NULL,
  `update_at` datetime DEFAULT NULL,
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

--
-- Dumping data for table `roles`
--

INSERT INTO `roles` (`role_id`, `role`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 'teacher', '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(2, 'staff', '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL),
(3, 'admin', '2025-06-24 16:49:13', '2025-06-24 16:49:13', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `subcategory_budgets`
--

CREATE TABLE `subcategory_budgets` (
  `subcategory_budget_id` int(11) NOT NULL,
  `subcategory_id` int(11) NOT NULL,
  `allocated_amount` decimal(15,2) DEFAULT NULL COMMENT 'จำนวนทุนต่อไป',
  `remaining_budget` decimal(15,2) DEFAULT NULL,
  `used_amount` decimal(15,2) DEFAULT NULL,
  `max_amount_per_grant` decimal(15,2) DEFAULT NULL,
  `max_grants` int(11) DEFAULT NULL,
  `remaining_grant` int(11) DEFAULT NULL,
  `level` enum('ต้น','กลาง','สูง') DEFAULT NULL,
  `status` enum('active','disable') DEFAULT NULL,
  `fund_description` text DEFAULT NULL,
  `comment` text DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

--
-- Dumping data for table `subcategory_budgets`
--

INSERT INTO `subcategory_budgets` (`subcategory_budget_id`, `subcategory_id`, `allocated_amount`, `remaining_budget`, `used_amount`, `max_amount_per_grant`, `max_grants`, `remaining_grant`, `level`, `status`, `fund_description`, `comment`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 1, '1200000.00', '1200000.00', '0.00', '200000.00', 6, 6, NULL, 'active', NULL, NULL, '2025-06-24 16:49:13', '2025-07-04 11:06:35', NULL),
(2, 2, '50000.00', '50000.00', '0.00', '25000.00', 2, 2, NULL, 'active', NULL, NULL, NULL, '2025-06-30 12:10:37', NULL),
(3, 3, '80000.00', '80000.00', '0.00', '20000.00', 4, 4, NULL, 'active', NULL, NULL, NULL, '2025-06-30 12:10:40', NULL),
(4, 4, '20000.00', '20000.00', '0.00', '10000.00', 2, 2, NULL, 'active', NULL, NULL, NULL, '2025-06-30 12:10:44', NULL),
(5, 5, '40000.00', '40000.00', '0.00', '20000.00', 2, 2, NULL, 'active', NULL, NULL, '2025-06-30 11:25:57', '2025-06-30 12:10:48', NULL),
(6, 6, '1000000.00', '1000000.00', '0.00', '150000.00', NULL, NULL, 'ต้น', 'active', 'ต้น', NULL, '2025-06-30 11:35:31', '2025-07-20 20:25:18', NULL),
(7, 6, '1000000.00', '1000000.00', '0.00', '200000.00', NULL, NULL, 'กลาง', 'active', 'กลาง', NULL, '2025-06-30 11:35:31', '2025-07-20 20:25:22', NULL),
(8, 6, '1000000.00', '1000000.00', '0.00', '250000.00', NULL, NULL, 'สูง', 'active', 'สูง', NULL, '2025-06-30 11:35:31', '2025-07-20 20:25:24', NULL),
(9, 7, '2500000.00', '2500000.00', '0.00', '1250000.00', 2, 2, NULL, 'active', NULL, NULL, '2025-06-30 11:35:31', '2025-06-30 12:14:15', NULL),
(10, 8, '1000000.00', '1000000.00', '0.00', '1000000.00', 1, 1, NULL, 'active', NULL, NULL, '2025-06-30 11:35:31', '2025-06-30 12:14:23', NULL),
(11, 9, '2500000.00', '2500000.00', '0.00', '2500000.00', 1, 1, NULL, 'active', NULL, NULL, '2025-06-30 11:35:31', '2025-06-30 12:14:26', NULL),
(12, 10, '1200000.00', '1200000.00', '0.00', '400000.00', 3, 3, NULL, 'active', NULL, NULL, '2025-06-30 11:35:31', '2025-06-30 12:14:30', NULL),
(13, 11, '500000.00', '500000.00', '0.00', '100000.00', NULL, NULL, 'ต้น', 'active', 'ต้น', NULL, '2025-06-30 11:46:07', '2025-07-20 20:25:26', NULL),
(14, 11, '500000.00', '500000.00', '0.00', '200000.00', NULL, NULL, 'กลาง', 'active', 'กลาง', NULL, '2025-06-30 11:46:07', '2025-07-20 20:25:30', NULL),
(15, 11, '500000.00', '500000.00', '0.00', '250000.00', NULL, NULL, 'สูง', 'active', 'สูง', NULL, '2025-06-30 11:46:07', '2025-07-20 20:25:33', NULL),
(16, 12, '200000.00', '200000.00', '0.00', '100000.00', 2, 2, NULL, 'active', NULL, 'ไม่เกิน 10% ของเงินทุนที่ได้รับจากภายนอก แต่ไม่เกิน 100,000 บาท', '2025-06-30 11:48:03', '2025-08-08 22:57:24', NULL),
(17, 13, '400000.00', '400000.00', '0.00', '150000.00', NULL, NULL, NULL, 'active', 'ประเทศต่างๆ ยกเว้นประเทศกลุ่มอาเซียน', 'กรณีเดินทางไปทําวิจัยในสถาบันการศึกษาที่อยู่ในอันดับ 1-300 ของโลกหรือสถาบันวิจัยที่มีชื่อเสียง', '2025-06-30 11:48:03', '2025-08-11 11:50:06', NULL),
(18, 13, '400000.00', '400000.00', '0.00', '100000.00', NULL, NULL, NULL, 'active', 'ประเทศกลุ่มอาเซียน', 'กรณีเดินทางไปทําวิจัยในสถาบันการศึกษาที่อยู่ในอันดับ 1-300 ของโลกหรือสถาบันวิจัยที่มีชื่อเสียง', '2025-06-30 11:48:03', '2025-08-11 11:50:06', NULL),
(19, 14, '3500000.00', '3500000.00', '0.00', '150000.00', NULL, NULL, NULL, 'active', 'วารสารระดับนานาชาติในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS (ลําดับ 5% แรก)', NULL, '2025-06-30 11:48:03', '2025-08-08 22:57:15', NULL),
(20, 14, '3500000.00', '3500000.00', '0.00', '125000.00', NULL, NULL, NULL, 'active', 'วารสารระดับนานาชาติในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS (ลําดับ 10% แรก)', NULL, '2025-06-30 11:48:03', '2025-08-08 22:57:15', NULL),
(21, 14, '3500000.00', '3500000.00', '0.00', '100000.00', NULL, NULL, NULL, 'active', 'วารสารระดับนานาชาติในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS ควอร์ไทล์ 1', NULL, '2025-06-30 11:48:03', '2025-08-08 22:57:15', NULL),
(22, 14, '3500000.00', '3500000.00', '0.00', '75000.00', NULL, NULL, NULL, 'active', 'วารสารระดับนานาชาติในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS ควอร์ไทล์ 2', NULL, '2025-06-30 11:48:03', '2025-08-08 22:57:15', NULL),
(23, 15, '3500000.00', '3500000.00', '0.00', '80000.00', NULL, NULL, NULL, 'active', 'วารสารระดับนานาชาติในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS (ลําดับ 5% แรก)', NULL, '2025-06-30 11:48:03', '2025-08-11 11:49:58', NULL),
(24, 15, '3500000.00', '3500000.00', '0.00', '60000.00', NULL, NULL, NULL, 'active', 'วารสารระดับนานาชาติในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS (ลําดับ 10% แรก)', NULL, '2025-06-30 11:48:03', '2025-08-11 11:49:58', NULL),
(25, 15, '3500000.00', '3500000.00', '0.00', '50000.00', NULL, NULL, NULL, 'active', 'วารสารระดับนานาชาติในฐานข้อมูล WOS หรือ ISI หรือ SCOPUS ควอร์ไทล์ 1', NULL, '2025-06-30 11:48:03', '2025-08-11 11:49:58', NULL),
(26, 16, '321.00', '321.00', '0.00', '312.00', NULL, NULL, NULL, 'active', NULL, NULL, '2025-08-08 23:09:28', '2025-08-08 23:09:43', '2025-08-08 23:09:43'),
(27, 16, '123.00', '123.00', '0.00', '123333.00', NULL, NULL, NULL, 'active', NULL, NULL, '2025-08-11 11:51:19', '2025-08-11 12:52:07', '2025-08-11 12:52:07'),
(28, 17, '1123123.00', '1123123.00', '0.00', '11233.00', NULL, NULL, NULL, 'active', 'งำ', NULL, '2025-08-11 12:52:40', '2025-08-11 13:19:39', '2025-08-11 13:19:39'),
(29, 17, '0.00', '0.00', '0.00', '123.00', NULL, NULL, NULL, 'active', 'aaa', NULL, '2025-08-11 13:19:51', '2025-08-11 13:55:52', '2025-08-11 13:55:52'),
(30, 17, '0.00', '0.00', '0.00', '123.00', NULL, NULL, NULL, 'active', NULL, NULL, '2025-08-11 13:55:58', '2025-08-11 13:56:01', '2025-08-11 13:56:01');

-- --------------------------------------------------------

--
-- Table structure for table `submissions`
--

CREATE TABLE `submissions` (
  `submission_id` int(11) NOT NULL,
  `submission_type` enum('fund_application','publication_reward') NOT NULL,
  `submission_number` varchar(255) DEFAULT NULL,
  `user_id` int(11) NOT NULL,
  `year_id` int(11) NOT NULL,
  `status_id` int(11) NOT NULL,
  `submitted_at` datetime DEFAULT NULL,
  `reviewed_at` datetime DEFAULT NULL,
  `approved_at` datetime DEFAULT NULL,
  `approved_by` int(11) DEFAULT NULL,
  `completed_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

--
-- Dumping data for table `submissions`
--

INSERT INTO `submissions` (`submission_id`, `submission_type`, `submission_number`, `user_id`, `year_id`, `status_id`, `submitted_at`, `reviewed_at`, `approved_at`, `approved_by`, `completed_at`, `created_at`, `updated_at`, `deleted_at`) VALUES
(1, 'publication_reward', 'PR-20250731-0001', 8, 3, 1, '2025-07-31 12:58:57', NULL, NULL, NULL, NULL, '2025-07-31 12:58:57', '2025-08-16 11:27:36', NULL),
(2, 'publication_reward', 'PR-20250731-0002', 8, 3, 1, '2025-07-31 13:28:34', NULL, NULL, NULL, NULL, '2025-07-31 13:28:33', '2025-08-15 12:13:44', NULL),
(3, 'publication_reward', 'PR-20250731-0003', 8, 3, 2, '2025-07-31 13:38:11', NULL, NULL, NULL, NULL, '2025-07-31 13:38:11', '2025-08-15 12:14:11', NULL),
(4, 'publication_reward', 'PR-20250731-0004', 8, 3, 1, '2025-07-31 13:42:38', NULL, NULL, NULL, NULL, '2025-07-31 13:42:38', '2025-07-31 13:42:38', NULL),
(5, 'publication_reward', 'PR-20250731-0005', 8, 3, 1, '2025-07-31 13:44:25', NULL, NULL, NULL, NULL, '2025-07-31 13:44:25', '2025-07-31 13:44:25', NULL),
(6, 'publication_reward', 'PR-20250801-0001', 8, 3, 1, '2025-08-01 10:24:29', NULL, NULL, NULL, NULL, '2025-08-01 10:24:28', '2025-08-01 10:24:29', NULL),
(7, 'publication_reward', 'PR-20250801-0002', 8, 3, 1, '2025-08-01 10:25:35', NULL, NULL, NULL, NULL, '2025-08-01 10:25:34', '2025-08-01 10:25:35', NULL),
(8, 'publication_reward', 'PR-20250801-0003', 8, 3, 1, '2025-08-01 10:38:16', NULL, NULL, NULL, NULL, '2025-08-01 10:38:16', '2025-08-01 10:38:16', NULL),
(9, 'publication_reward', 'PR-20250801-0004', 8, 3, 1, '2025-08-01 10:39:09', NULL, NULL, NULL, NULL, '2025-08-01 10:39:09', '2025-08-01 10:39:09', NULL),
(10, 'publication_reward', 'PR-20250801-0005', 8, 3, 1, '2025-08-01 10:42:00', NULL, NULL, NULL, NULL, '2025-08-01 10:42:00', '2025-08-01 10:42:00', NULL),
(11, 'publication_reward', 'PR-20250801-0006', 8, 3, 1, '2025-08-01 10:50:27', NULL, NULL, NULL, NULL, '2025-08-01 10:50:27', '2025-08-01 10:50:27', NULL),
(12, 'publication_reward', 'PR-20250801-0007', 8, 3, 1, '2025-08-01 11:09:50', NULL, NULL, NULL, NULL, '2025-08-01 11:09:50', '2025-08-01 11:09:50', NULL),
(13, 'publication_reward', 'PR-20250801-0008', 8, 3, 1, '2025-08-01 11:19:12', NULL, NULL, NULL, NULL, '2025-08-01 11:19:12', '2025-08-01 11:19:12', NULL),
(14, 'publication_reward', 'PR-20250801-0009', 8, 3, 1, '2025-08-01 11:22:17', NULL, NULL, NULL, NULL, '2025-08-01 11:22:17', '2025-08-01 11:22:17', NULL),
(15, 'publication_reward', 'PR-20250801-0010', 8, 3, 1, '2025-08-01 11:23:53', NULL, NULL, NULL, NULL, '2025-08-01 11:23:53', '2025-08-01 11:23:53', NULL),
(16, 'publication_reward', 'PR-20250801-0011', 8, 3, 1, '2025-08-01 11:34:30', NULL, NULL, NULL, NULL, '2025-08-01 11:34:30', '2025-08-01 11:34:30', NULL),
(17, 'publication_reward', 'PR-20250801-0012', 8, 3, 1, '2025-08-01 11:37:54', NULL, NULL, NULL, NULL, '2025-08-01 11:37:53', '2025-08-01 11:37:54', NULL),
(18, 'publication_reward', 'PR-20250801-0013', 8, 3, 1, '2025-08-01 11:40:47', NULL, NULL, NULL, NULL, '2025-08-01 11:40:47', '2025-08-01 11:40:47', NULL),
(19, 'publication_reward', 'PR-20250801-0014', 8, 3, 1, '2025-08-01 12:08:25', NULL, NULL, NULL, NULL, '2025-08-01 12:08:25', '2025-08-01 12:08:25', NULL),
(20, 'publication_reward', 'PR-20250801-0015', 8, 3, 1, '2025-08-01 12:11:19', NULL, NULL, NULL, NULL, '2025-08-01 12:11:19', '2025-08-01 12:11:19', NULL),
(21, 'publication_reward', 'PR-20250801-0016', 8, 3, 1, '2025-08-01 22:29:44', NULL, NULL, NULL, NULL, '2025-08-01 22:29:44', '2025-08-01 22:29:44', NULL),
(22, 'publication_reward', 'PR-20250802-0001', 8, 3, 1, '2025-08-02 12:18:25', NULL, NULL, NULL, NULL, '2025-08-02 12:18:25', '2025-08-02 12:18:25', NULL),
(23, 'publication_reward', 'PR-20250805-0001', 8, 3, 1, '2025-08-05 22:41:57', NULL, NULL, NULL, NULL, '2025-08-05 22:41:57', '2025-08-05 22:41:57', NULL),
(24, 'publication_reward', 'PR-20250805-0002', 8, 3, 1, '2025-08-05 22:46:59', NULL, NULL, NULL, NULL, '2025-08-05 22:46:58', '2025-08-05 22:46:59', NULL),
(25, 'publication_reward', 'PR-20250807-0001', 8, 3, 1, '2025-08-07 01:19:18', NULL, NULL, NULL, NULL, '2025-08-07 01:19:17', '2025-08-16 11:29:38', NULL),
(26, 'publication_reward', 'PR-20250807-0002', 8, 3, 1, '2025-08-07 02:34:34', NULL, NULL, NULL, NULL, '2025-08-07 02:34:33', '2025-08-07 02:34:34', NULL),
(27, 'publication_reward', 'PR-20250807-0003', 8, 3, 1, '2025-08-19 16:35:41', NULL, NULL, NULL, NULL, '2025-08-07 02:56:56', '2025-08-19 11:35:46', NULL),
(28, 'publication_reward', 'PR-20250807-0004', 8, 3, 1, '2025-08-19 16:35:50', NULL, NULL, NULL, NULL, '2025-08-07 02:57:59', '2025-08-19 11:35:52', NULL),
(29, 'publication_reward', 'PR-20250807-0005', 8, 3, 1, '2025-08-07 03:28:43', NULL, NULL, NULL, NULL, '2025-08-07 03:27:58', '2025-08-07 03:28:43', NULL),
(30, 'publication_reward', 'PR-20250807-0006', 8, 3, 1, '2025-08-19 16:35:53', NULL, NULL, NULL, NULL, '2025-08-07 04:00:14', '2025-08-19 11:35:54', NULL),
(31, 'publication_reward', 'PR-20250807-0007', 8, 3, 1, '2025-08-07 06:11:59', NULL, NULL, NULL, NULL, '2025-08-07 06:11:59', '2025-08-07 06:11:59', NULL),
(32, 'publication_reward', 'PR-20250807-0008', 8, 3, 1, '2025-08-07 13:34:30', NULL, NULL, NULL, NULL, '2025-08-07 13:34:30', '2025-08-07 13:34:30', NULL),
(33, 'publication_reward', 'PR-20250807-0009', 8, 3, 1, '2025-08-07 13:38:22', NULL, NULL, NULL, NULL, '2025-08-07 13:38:22', '2025-08-07 13:38:22', NULL),
(34, 'publication_reward', 'PR-20250807-0010', 8, 3, 2, '2025-08-07 13:41:31', NULL, '2025-08-18 22:17:00', 7, NULL, '2025-08-07 13:41:31', '2025-08-18 22:17:00', NULL),
(35, 'publication_reward', 'PR-20250807-0011', 8, 3, 2, '2025-08-07 13:53:27', NULL, '2025-08-18 18:13:00', 7, NULL, '2025-08-07 13:53:27', '2025-08-18 18:13:00', NULL),
(36, 'publication_reward', 'PR-20250807-0012', 8, 3, 3, '2025-08-07 13:58:00', NULL, NULL, NULL, NULL, '2025-08-07 13:58:00', '2025-08-19 12:03:54', NULL),
(37, 'fund_application', 'PR-20250807-0013', 8, 3, 3, '2025-08-07 14:01:47', NULL, NULL, NULL, NULL, '2025-08-07 14:01:47', '2025-08-18 18:05:41', NULL),
(38, 'publication_reward', 'PR-20250819-0001', 8, 3, 1, NULL, NULL, NULL, NULL, NULL, '2025-08-19 18:02:45', '2025-08-19 18:02:45', NULL),
(39, 'publication_reward', 'PR-20250819-0002', 8, 3, 1, NULL, NULL, NULL, NULL, NULL, '2025-08-19 21:38:18', '2025-08-19 21:38:18', NULL),
(40, 'publication_reward', 'PR-20250820-0001', 8, 3, 1, NULL, NULL, NULL, NULL, NULL, '2025-08-20 01:33:59', '2025-08-20 01:33:59', NULL),
(41, 'publication_reward', 'PR-20250820-0002', 8, 3, 1, NULL, NULL, NULL, NULL, NULL, '2025-08-20 01:45:00', '2025-08-20 01:45:00', NULL),
(42, 'publication_reward', 'PR-20250820-0003', 8, 3, 1, NULL, NULL, NULL, NULL, NULL, '2025-08-20 14:16:13', '2025-08-20 14:16:13', NULL),
(43, 'publication_reward', 'PR-20250822-0001', 8, 3, 1, NULL, NULL, NULL, NULL, NULL, '2025-08-22 03:18:56', '2025-08-22 03:18:56', NULL),
(44, 'publication_reward', 'PR-20250822-0002', 8, 3, 1, NULL, NULL, NULL, NULL, NULL, '2025-08-22 04:15:27', '2025-08-22 04:15:27', NULL),
(45, 'publication_reward', 'PR-20250822-0003', 8, 3, 2, '2025-08-22 09:34:39', NULL, '2025-08-22 13:28:49', 7, NULL, '2025-08-22 09:33:24', '2025-08-22 13:28:49', NULL),
(46, 'publication_reward', 'PR-20250822-0004', 8, 3, 3, '2025-08-22 09:54:14', NULL, NULL, NULL, NULL, '2025-08-22 09:54:13', '2025-08-22 11:19:48', NULL),
(47, 'publication_reward', 'PR-20250822-0005', 8, 3, 1, '2025-08-22 16:40:11', NULL, NULL, NULL, NULL, '2025-08-22 16:40:11', '2025-08-22 16:40:11', NULL),
(48, 'publication_reward', 'PR-20250823-0001', 8, 3, 1, '2025-08-23 07:27:48', NULL, NULL, NULL, NULL, '2025-08-23 07:27:47', '2025-08-23 07:27:48', NULL);

--
-- Triggers `submissions`
--
DELIMITER $$
CREATE TRIGGER `audit_submissions_delete` AFTER UPDATE ON `submissions` FOR EACH ROW BEGIN
    IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
        INSERT INTO audit_logs (
            user_id, action, entity_type, entity_id, entity_number,
            description, created_at
        ) VALUES (
            NEW.user_id,
            'delete',
            'submission',
            NEW.submission_id,
            NEW.submission_number,
            'Deleted submission',
            NOW()
        );
    END IF;
END
$$
DELIMITER ;
DELIMITER $$
CREATE TRIGGER `audit_submissions_insert` AFTER INSERT ON `submissions` FOR EACH ROW BEGIN
    DECLARE v_user_id INT;
    SET v_user_id = NEW.user_id;
    
    INSERT INTO audit_logs (
        user_id, action, entity_type, entity_id, entity_number,
        new_values, description, created_at
    ) VALUES (
        v_user_id, 
        'create', 
        'submission', 
        NEW.submission_id, 
        NEW.submission_number,
        JSON_OBJECT(
            'submission_type', NEW.submission_type,
            'status_id', NEW.status_id,
            'year_id', NEW.year_id
        ),
        CONCAT('Created new ', NEW.submission_type),
        NOW()
    );
END
$$
DELIMITER ;
DELIMITER $$
CREATE TRIGGER `audit_submissions_update` AFTER UPDATE ON `submissions` FOR EACH ROW BEGIN
   DECLARE v_user_id INT;
   DECLARE v_action VARCHAR(20);
   DECLARE v_changed_fields TEXT DEFAULT '';
   
   -- Determine user_id
   SET v_user_id = IFNULL(NEW.approved_by, NEW.user_id);
   
   -- Check what changed
   IF IFNULL(OLD.status_id, 0) != IFNULL(NEW.status_id, 0) THEN
       SET v_changed_fields = CONCAT(v_changed_fields, 'status,');
   END IF;
   
   -- Determine action type
   IF OLD.status_id != NEW.status_id AND NEW.status_id = 2 THEN
       SET v_action = 'approve';
   ELSEIF OLD.status_id != NEW.status_id AND NEW.status_id = 3 THEN
       SET v_action = 'reject';
   ELSEIF OLD.submitted_at IS NULL AND NEW.submitted_at IS NOT NULL THEN
       SET v_action = 'submit';
   ELSE
       SET v_action = 'update';
   END IF;
   
   -- Only log if something changed
   IF v_changed_fields != '' OR v_action != 'update' THEN
       INSERT INTO audit_logs (
           user_id, action, entity_type, entity_id, entity_number,
           changed_fields, description, created_at
       ) VALUES (
           v_user_id,
           v_action,
           'submission',
           NEW.submission_id,
           NEW.submission_number,
           TRIM(TRAILING ',' FROM v_changed_fields),
           CONCAT(v_action, ' submission'),
           NOW()
       );
   END IF;
END
$$
DELIMITER ;

-- --------------------------------------------------------

--
-- Table structure for table `submission_documents`
--

CREATE TABLE `submission_documents` (
  `document_id` int(11) NOT NULL,
  `submission_id` int(11) NOT NULL,
  `file_id` int(11) NOT NULL,
  `document_type_id` int(11) NOT NULL,
  `description` text DEFAULT NULL,
  `display_order` int(11) DEFAULT 0,
  `is_required` tinyint(1) DEFAULT 0,
  `is_verified` tinyint(1) DEFAULT 0,
  `verified_by` int(11) DEFAULT NULL,
  `verified_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

--
-- Dumping data for table `submission_documents`
--

INSERT INTO `submission_documents` (`document_id`, `submission_id`, `file_id`, `document_type_id`, `description`, `display_order`, `is_required`, `is_verified`, `verified_by`, `verified_at`, `created_at`) VALUES
(1, 1, 1, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-07-31 12:58:57'),
(2, 1, 2, 3, 'pdf-sample_2.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-07-31 12:58:57'),
(3, 1, 3, 11, 'เอกสารอื่นๆ 1: pdf-sample_3.pdf', 3, 0, 0, NULL, NULL, '2025-07-31 12:58:57'),
(4, 1, 4, 12, 'เอกสารเบิกจ่ายภายนอก: ทุนที่ 1', 4, 0, 0, NULL, NULL, '2025-07-31 12:58:57'),
(5, 2, 5, 2, 'pdf-sample_2.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-07-31 13:28:34'),
(6, 2, 6, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-07-31 13:28:34'),
(7, 3, 7, 2, 'pdf-sample_2.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-07-31 13:38:11'),
(8, 3, 8, 3, 'pdf-sample_0.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-07-31 13:38:11'),
(9, 4, 9, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-07-31 13:42:38'),
(10, 4, 10, 3, 'pdf-sample_3.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-07-31 13:42:38'),
(11, 5, 11, 2, 'pdf-sample_3.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-07-31 13:44:25'),
(12, 6, 12, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 10:24:29'),
(13, 6, 13, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-01 10:24:29'),
(14, 7, 14, 2, 'pdf-sample_2.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 10:25:34'),
(15, 7, 15, 3, 'pdf-sample_2.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-01 10:25:34'),
(16, 8, 16, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 10:38:16'),
(17, 8, 17, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-01 10:38:16'),
(18, 9, 18, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 10:39:09'),
(19, 9, 19, 3, 'pdf-sample_0.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-01 10:39:09'),
(20, 10, 20, 3, 'pdf-sample_0.pdf (ประเภท 3)', 1, 0, 0, NULL, NULL, '2025-08-01 10:42:00'),
(21, 11, 21, 3, 'pdf-sample_2.pdf (ประเภท 3)', 1, 0, 0, NULL, NULL, '2025-08-01 10:50:27'),
(22, 12, 22, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 11:09:50'),
(23, 12, 23, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-01 11:09:50'),
(24, 13, 24, 2, 'pdf-sample_2.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 11:19:12'),
(25, 14, 25, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 11:22:17'),
(26, 15, 26, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 11:23:53'),
(27, 16, 27, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 11:34:30'),
(28, 17, 28, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 11:37:54'),
(29, 18, 29, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 11:40:47'),
(30, 19, 30, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 12:08:25'),
(31, 19, 31, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-01 12:08:25'),
(32, 20, 32, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 12:11:19'),
(33, 21, 33, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-01 22:29:44'),
(34, 21, 34, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-01 22:29:44'),
(35, 21, 35, 12, 'เอกสารเบิกจ่ายภายนอก: ทุนที่ 1', 3, 0, 0, NULL, NULL, '2025-08-01 22:29:44'),
(36, 22, 36, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-02 12:18:25'),
(37, 22, 37, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-02 12:18:25'),
(38, 22, 38, 11, 'เอกสารเบิกจ่ายภายนอก - pdf-sample_2.pdf', 3, 0, 0, NULL, NULL, '2025-08-02 12:18:25'),
(39, 22, 39, 12, 'เอกสารเบิกจ่ายภายนอก: ทุนที่ 1', 4, 0, 0, NULL, NULL, '2025-08-02 12:18:25'),
(40, 22, 40, 1, 'เอกสารรวม (Merged PDF)', 5, 0, 0, NULL, NULL, '2025-08-02 12:18:25'),
(41, 23, 41, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-05 22:41:57'),
(42, 23, 42, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-05 22:41:57'),
(43, 24, 43, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-05 22:46:59'),
(44, 24, 44, 3, 'pdf-sample_2.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-05 22:46:59'),
(45, 24, 45, 11, 'เอกสารเบิกจ่ายภายนอก - pdf-sample_0.pdf', 3, 0, 0, NULL, NULL, '2025-08-05 22:46:59'),
(46, 24, 46, 12, 'เอกสารเบิกจ่ายภายนอก: ทุนที่ 1', 4, 0, 0, NULL, NULL, '2025-08-05 22:46:59'),
(47, 25, 47, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 01:19:18'),
(48, 25, 48, 3, 'pdf-sample_2.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 01:19:18'),
(49, 25, 49, 11, 'เอกสารเบิกจ่ายภายนอก - pdf-sample_0.pdf', 3, 0, 0, NULL, NULL, '2025-08-07 01:19:18'),
(50, 25, 50, 12, 'เอกสารเบิกจ่ายภายนอก: ทุนที่ 1', 4, 0, 0, NULL, NULL, '2025-08-07 01:19:18'),
(51, 26, 51, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 02:34:34'),
(52, 26, 52, 3, 'pdf-sample_3.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 02:34:34'),
(53, 29, 53, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 03:28:42'),
(54, 29, 54, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 03:28:42'),
(55, 29, 55, 1, 'เอกสารรวม (Merged PDF)', 3, 0, 0, NULL, NULL, '2025-08-07 03:28:43'),
(56, 31, 56, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 06:11:59'),
(57, 31, 57, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 06:11:59'),
(58, 32, 58, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 13:34:30'),
(59, 32, 59, 3, 'pdf-sample_3.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 13:34:30'),
(60, 33, 60, 2, 'pdf-sample_2.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 13:38:22'),
(61, 33, 61, 3, 'pdf-sample_0.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 13:38:22'),
(62, 34, 62, 2, 'pdf-sample_1.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 13:41:31'),
(63, 34, 63, 3, 'pdf-sample_0.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 13:41:31'),
(64, 35, 64, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 13:53:27'),
(65, 35, 65, 3, 'pdf-sample_3.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 13:53:27'),
(66, 36, 66, 2, 'pdf-sample_3.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 13:58:00'),
(67, 36, 67, 3, 'pdf-sample_0.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 13:58:00'),
(68, 36, 68, 11, 'เอกสารเบิกจ่ายภายนอก - pdf-sample_1.pdf', 3, 0, 0, NULL, NULL, '2025-08-07 13:58:00'),
(69, 36, 69, 12, 'เอกสารเบิกจ่ายภายนอก: ทุนที่ 1', 4, 0, 0, NULL, NULL, '2025-08-07 13:58:00'),
(70, 37, 70, 2, 'pdf-sample_0.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-07 14:01:47'),
(71, 37, 71, 3, 'pdf-sample_1.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-07 14:01:47'),
(72, 45, 72, 2, 'form.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-22 09:34:39'),
(73, 45, 73, 3, 'form.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-22 09:34:39'),
(74, 45, 74, 11, 'เอกสารเบิกจ่ายภายนอก - form.pdf', 3, 0, 0, NULL, NULL, '2025-08-22 09:34:39'),
(75, 45, 75, 12, 'เอกสารเบิกจ่ายภายนอก: ทุนที่ 1', 4, 0, 0, NULL, NULL, '2025-08-22 09:34:39'),
(76, 45, 76, 1, 'เอกสารรวม (Merged PDF)', 5, 0, 0, NULL, NULL, '2025-08-22 09:34:39'),
(77, 46, 77, 2, 'form.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-22 09:54:13'),
(78, 46, 78, 3, 'form.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-22 09:54:13'),
(79, 47, 79, 2, 'form.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-22 16:40:11'),
(80, 48, 80, 2, 'file-sample_150kB.pdf (ประเภท 2)', 1, 0, 0, NULL, NULL, '2025-08-23 07:27:47'),
(81, 48, 81, 3, 'sample.pdf (ประเภท 3)', 2, 0, 0, NULL, NULL, '2025-08-23 07:27:47'),
(82, 48, 82, 11, 'เอกสารอื่นๆ 1: sample-local-pdf.pdf', 3, 0, 0, NULL, NULL, '2025-08-23 07:27:47'),
(83, 48, 83, 12, 'เอกสารเบิกจ่ายภายนอก: ไม่ระบุ', 4, 0, 0, NULL, NULL, '2025-08-23 07:27:47'),
(84, 48, 84, 12, 'เอกสารเบิกจ่ายภายนอก: ไม่ระบุ', 5, 0, 0, NULL, NULL, '2025-08-23 07:27:47');

-- --------------------------------------------------------

--
-- Table structure for table `submission_reviews`
--

CREATE TABLE `submission_reviews` (
  `review_id` int(11) NOT NULL,
  `submission_id` int(11) NOT NULL,
  `reviewer_id` int(11) NOT NULL,
  `review_round` int(11) DEFAULT 1,
  `review_status` enum('approved','revision_required','rejected') NOT NULL,
  `comments` text DEFAULT NULL,
  `internal_notes` text DEFAULT NULL,
  `reviewed_at` datetime DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `submission_status_history`
--

CREATE TABLE `submission_status_history` (
  `history_id` int(11) NOT NULL,
  `submission_id` int(11) NOT NULL,
  `old_status_id` int(11) DEFAULT NULL,
  `new_status_id` int(11) NOT NULL,
  `changed_by` int(11) NOT NULL,
  `reason` text DEFAULT NULL,
  `notes` text DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `submission_users`
--

CREATE TABLE `submission_users` (
  `id` int(11) NOT NULL,
  `submission_id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `role` enum('owner','coauthor','team_member','advisor','coordinator','co_author') DEFAULT 'coauthor',
  `is_primary` tinyint(1) DEFAULT 0,
  `display_order` int(11) DEFAULT 0,
  `created_at` datetime DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

--
-- Dumping data for table `submission_users`
--

INSERT INTO `submission_users` (`id`, `submission_id`, `user_id`, `role`, `is_primary`, `display_order`, `created_at`) VALUES
(1, 5, 1, 'coauthor', 0, 2, '2025-07-31 13:44:25'),
(2, 5, 9, 'coauthor', 0, 3, '2025-07-31 13:44:25'),
(3, 13, 8, 'coauthor', 0, 1, '2025-08-01 11:19:12'),
(4, 13, 1, 'coauthor', 0, 2, '2025-08-01 11:19:12'),
(5, 13, 9, 'coauthor', 0, 3, '2025-08-01 11:19:12'),
(6, 14, 8, 'coauthor', 0, 1, '2025-08-01 11:22:17'),
(7, 14, 1, 'coauthor', 0, 2, '2025-08-01 11:22:17'),
(8, 14, 9, 'coauthor', 0, 3, '2025-08-01 11:22:17'),
(9, 15, 8, 'coauthor', 0, 1, '2025-08-01 11:23:53'),
(10, 15, 1, 'coauthor', 0, 2, '2025-08-01 11:23:53'),
(11, 15, 9, 'coauthor', 0, 3, '2025-08-01 11:23:53'),
(12, 16, 8, 'coauthor', 0, 1, '2025-08-01 11:34:30'),
(13, 16, 1, 'coauthor', 0, 2, '2025-08-01 11:34:30'),
(14, 16, 9, 'coauthor', 0, 3, '2025-08-01 11:34:30'),
(15, 17, 8, 'owner', 0, 1, '2025-08-01 11:37:53'),
(16, 17, 1, 'coauthor', 0, 2, '2025-08-01 11:37:53'),
(17, 17, 9, 'coauthor', 0, 3, '2025-08-01 11:37:53'),
(18, 18, 8, 'owner', 0, 1, '2025-08-01 11:40:47'),
(19, 18, 9, 'coauthor', 0, 2, '2025-08-01 11:40:47'),
(20, 18, 1, 'coauthor', 0, 3, '2025-08-01 11:40:47'),
(21, 19, 8, 'owner', 1, 1, '2025-08-01 12:08:25'),
(22, 19, 1, 'coauthor', 1, 2, '2025-08-01 12:08:25'),
(23, 19, 9, 'coauthor', 1, 3, '2025-08-01 12:08:25'),
(24, 20, 8, 'owner', 1, 1, '2025-08-01 12:11:19'),
(25, 20, 1, 'coauthor', 0, 2, '2025-08-01 12:11:19'),
(26, 20, 9, 'coauthor', 0, 3, '2025-08-01 12:11:19'),
(27, 21, 8, 'owner', 1, 1, '2025-08-01 22:29:44'),
(28, 21, 1, 'coauthor', 0, 2, '2025-08-01 22:29:44'),
(29, 21, 9, 'coauthor', 0, 3, '2025-08-01 22:29:44'),
(30, 22, 8, 'owner', 1, 1, '2025-08-02 12:18:25'),
(31, 23, 8, 'owner', 1, 1, '2025-08-05 22:41:57'),
(32, 23, 1, 'coauthor', 0, 2, '2025-08-05 22:41:57'),
(33, 23, 9, 'coauthor', 0, 3, '2025-08-05 22:41:57'),
(34, 24, 8, 'owner', 1, 1, '2025-08-05 22:46:59'),
(35, 24, 1, 'coauthor', 0, 2, '2025-08-05 22:46:59'),
(36, 25, 8, 'owner', 1, 1, '2025-08-07 01:19:17'),
(37, 25, 1, 'coauthor', 0, 2, '2025-08-07 01:19:18'),
(38, 25, 9, 'coauthor', 0, 3, '2025-08-07 01:19:18'),
(39, 26, 8, 'owner', 1, 1, '2025-08-07 02:34:34'),
(40, 26, 1, 'coauthor', 0, 2, '2025-08-07 02:34:34'),
(41, 27, 8, 'owner', 1, 1, '2025-08-07 02:56:56'),
(42, 28, 8, 'owner', 1, 1, '2025-08-07 02:57:59'),
(43, 29, 8, 'owner', 1, 1, '2025-08-07 03:27:58'),
(44, 29, 1, 'coauthor', 0, 2, '2025-08-07 03:27:58'),
(45, 30, 8, 'owner', 1, 1, '2025-08-07 04:00:15'),
(46, 30, 1, 'coauthor', 0, 2, '2025-08-07 04:00:15'),
(47, 31, 8, 'owner', 1, 1, '2025-08-07 06:11:59'),
(48, 31, 1, 'coauthor', 0, 2, '2025-08-07 06:11:59'),
(49, 32, 8, 'owner', 1, 1, '2025-08-07 13:34:30'),
(50, 32, 1, 'coauthor', 0, 2, '2025-08-07 13:34:30'),
(51, 33, 8, 'owner', 1, 1, '2025-08-07 13:38:22'),
(52, 34, 8, 'owner', 1, 1, '2025-08-07 13:41:31'),
(53, 34, 1, 'coauthor', 0, 2, '2025-08-07 13:41:31'),
(54, 35, 8, 'owner', 1, 1, '2025-08-07 13:53:27'),
(55, 35, 1, 'coauthor', 0, 2, '2025-08-07 13:53:27'),
(56, 36, 8, 'owner', 1, 1, '2025-08-07 13:58:00'),
(57, 36, 9, 'coauthor', 0, 2, '2025-08-07 13:58:00'),
(58, 37, 8, 'owner', 1, 1, '2025-08-07 14:01:47'),
(59, 38, 8, 'owner', 1, 1, '2025-08-19 18:02:45'),
(60, 38, 1, 'coauthor', 0, 2, '2025-08-19 18:02:45'),
(61, 38, 9, 'coauthor', 0, 3, '2025-08-19 18:02:45'),
(62, 39, 8, 'owner', 1, 1, '2025-08-19 21:38:18'),
(63, 39, 1, 'coauthor', 0, 2, '2025-08-19 21:38:18'),
(64, 39, 9, 'coauthor', 0, 3, '2025-08-19 21:38:18'),
(65, 40, 8, 'owner', 1, 1, '2025-08-20 01:33:59'),
(66, 40, 1, 'coauthor', 0, 2, '2025-08-20 01:33:59'),
(67, 40, 9, 'coauthor', 0, 3, '2025-08-20 01:33:59'),
(68, 41, 8, 'owner', 1, 1, '2025-08-20 01:45:00'),
(69, 41, 1, 'coauthor', 0, 2, '2025-08-20 01:45:00'),
(70, 41, 9, 'coauthor', 0, 3, '2025-08-20 01:45:00'),
(71, 42, 8, 'owner', 1, 1, '2025-08-20 14:16:14'),
(72, 42, 1, 'coauthor', 0, 2, '2025-08-20 14:16:14'),
(73, 42, 9, 'coauthor', 0, 3, '2025-08-20 14:16:14'),
(74, 43, 8, 'owner', 1, 1, '2025-08-22 03:18:56'),
(75, 43, 1, 'coauthor', 0, 2, '2025-08-22 03:18:56'),
(76, 43, 9, 'coauthor', 0, 3, '2025-08-22 03:18:56'),
(77, 44, 8, 'owner', 1, 1, '2025-08-22 04:15:27'),
(78, 44, 1, 'coauthor', 0, 2, '2025-08-22 04:15:27'),
(79, 44, 9, 'coauthor', 0, 3, '2025-08-22 04:15:27'),
(80, 45, 8, 'owner', 1, 1, '2025-08-22 09:33:25'),
(81, 45, 1, 'coauthor', 0, 2, '2025-08-22 09:33:25'),
(82, 45, 9, 'coauthor', 0, 3, '2025-08-22 09:33:25'),
(83, 46, 8, 'owner', 1, 1, '2025-08-22 09:54:13'),
(84, 46, 1, 'coauthor', 0, 2, '2025-08-22 09:54:13'),
(85, 46, 9, 'coauthor', 0, 3, '2025-08-22 09:54:13'),
(86, 47, 8, 'owner', 1, 1, '2025-08-22 16:40:11'),
(87, 47, 1, 'coauthor', 0, 2, '2025-08-22 16:40:11'),
(88, 47, 9, 'coauthor', 0, 3, '2025-08-22 16:40:11'),
(89, 48, 8, 'owner', 1, 1, '2025-08-23 07:27:47'),
(90, 48, 1, 'coauthor', 0, 2, '2025-08-23 07:27:47'),
(91, 48, 9, 'coauthor', 0, 3, '2025-08-23 07:27:47');

-- --------------------------------------------------------

--
-- Table structure for table `system_config`
--

CREATE TABLE `system_config` (
  `config_id` int(11) NOT NULL,
  `system_version` varchar(20) DEFAULT '1.0.0',
  `last_updated` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `updated_by` int(11) DEFAULT NULL,
  `budget_2568` decimal(15,2) DEFAULT 2000000.00,
  `budget_2569` decimal(15,2) DEFAULT NULL,
  `budget_2570` decimal(15,2) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- --------------------------------------------------------

--
-- Table structure for table `users`
--

CREATE TABLE `users` (
  `user_id` int(11) NOT NULL,
  `user_fname` varchar(255) DEFAULT NULL,
  `user_lname` varchar(255) DEFAULT NULL,
  `gender` varchar(255) DEFAULT NULL,
  `email` varchar(255) DEFAULT NULL,
  `password` varchar(255) DEFAULT NULL,
  `role_id` int(11) DEFAULT NULL,
  `position_id` int(11) DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT NULL,
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

--
-- Dumping data for table `users`
--

INSERT INTO `users` (`user_id`, `user_fname`, `user_lname`, `gender`, `email`, `password`, `role_id`, `position_id`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 'Somchai', 'Suwan', 'male', 'somchai@example.com', '$2a$10$LCtvqEswW1dTIOwJdTrvZuFmQF61aepTdC9HgI78UdnuyVJs3pxIm', 1, 1, '2025-06-24 16:49:13', '2025-07-02 22:02:51', NULL),
(2, 'Suda', 'Kong', 'female', 'suda@example.com', '$2a$10$.UeSuOiuMSwJRwZyxplaSOd7DsD/q/0S7zozjFWGP9F2Dm1ZCN8rK', 2, 3, '2025-06-24 16:49:13', '2025-07-02 22:02:51', NULL),
(7, 'ผู้ดูแล', 'ระบบ', 'male', 'admin@cpkku.ac.th', '$2a$10$f8kTbCx57o6gCNItJMUczeTmwPK1TUudS85U.wF6keW2cAVApjYN6', 3, 3, '2025-07-31 17:52:45', '2025-07-31 17:52:45', NULL),
(8, 'สมชาย', 'ใจดี', 'male', 'teacher@cpkku.ac.th', '$2a$10$sPaTxAZ.Bp4fxHGBg.awZ.a5jq72uWXeRAQHLK.3LTluhNoliaRYG', 1, 1, '2025-07-31 17:52:46', '2025-07-31 17:52:46', NULL),
(9, 'สมหญิง', 'รักการศึกษา', 'female', 'teacher2@cpkku.ac.th', '$2a$10$mgxuR9pZ5HfndfDoHd/ZquUQYAKztvxZBpT417iX05TLOC.axULf2', 1, 2, '2025-07-31 17:52:46', '2025-07-31 17:52:46', NULL),
(10, 'สุดา', 'ช่วยเหลือ', 'female', 'staff@cpkku.ac.th', '$2a$10$Df2y47XVO7Eugd/DLXJSAuIqXktScmsvhTSRzANBQzqSOCmuPSi1C', 2, 3, '2025-07-31 17:52:46', '2025-07-31 17:52:46', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `user_fund_eligibilities`
--

CREATE TABLE `user_fund_eligibilities` (
  `user_fund_eligibility_id` int(11) NOT NULL,
  `user_id` int(11) DEFAULT NULL,
  `year_id` int(11) DEFAULT NULL,
  `category_id` int(11) DEFAULT NULL,
  `remaining_quota` decimal(15,2) DEFAULT NULL,
  `max_allowed_amount` decimal(15,2) DEFAULT NULL,
  `remaining_applications` int(11) DEFAULT NULL,
  `is_eligible` varchar(255) DEFAULT NULL,
  `restriction_reason` text DEFAULT NULL,
  `calculated_at` datetime DEFAULT NULL,
  `create_at` datetime DEFAULT NULL,
  `update_at` datetime DEFAULT NULL,
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

--
-- Dumping data for table `user_fund_eligibilities`
--

INSERT INTO `user_fund_eligibilities` (`user_fund_eligibility_id`, `user_id`, `year_id`, `category_id`, `remaining_quota`, `max_allowed_amount`, `remaining_applications`, `is_eligible`, `restriction_reason`, `calculated_at`, `create_at`, `update_at`, `delete_at`) VALUES
(1, 1, 1, 1, '200000.00', '200000.00', 1, 'yes', NULL, '2025-06-24 17:52:30', '2025-06-24 17:52:30', '2025-06-24 17:52:30', NULL),
(2, 2, 2, 2, '80000.00', '80000.00', 1, 'yes', NULL, '2025-06-24 17:52:30', '2025-06-24 17:52:30', '2025-06-24 17:52:30', NULL);

-- --------------------------------------------------------

--
-- Table structure for table `user_sessions`
--

CREATE TABLE `user_sessions` (
  `session_id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `access_token_jti` varchar(255) DEFAULT NULL,
  `refresh_token` varchar(500) DEFAULT NULL,
  `device_name` varchar(255) DEFAULT NULL,
  `device_type` varchar(50) DEFAULT NULL,
  `ip_address` varchar(45) DEFAULT NULL,
  `user_agent` text DEFAULT NULL,
  `last_activity` datetime DEFAULT NULL,
  `expires_at` datetime NOT NULL,
  `is_active` tinyint(1) DEFAULT 1,
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

--
-- Dumping data for table `user_sessions`
--

INSERT INTO `user_sessions` (`session_id`, `user_id`, `access_token_jti`, `refresh_token`, `device_name`, `device_type`, `ip_address`, `user_agent`, `last_activity`, `expires_at`, `is_active`, `created_at`, `updated_at`) VALUES
(1, 8, '6cf24752-08eb-4497-9694-732a8acb9d96', '43B-KeFXrOD_lC0--z6IQxuwnUhboU20R3gBY9TYaUc=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-07-31 13:49:10', '2025-08-30 12:53:02', 1, '2025-07-31 12:53:02', '2025-07-31 13:49:10'),
(2, 8, '2199d26c-c883-4d53-9105-e96778e717f8', 'QhD-W6fwnfKAVhkbr7gLtuEZqF_9VuhVOA8kbcCIlwk=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-01 11:09:55', '2025-08-31 10:15:46', 1, '2025-08-01 10:15:46', '2025-08-01 11:09:55'),
(3, 8, '07d8477f-0c12-428b-9798-2afabb85e6e9', 'WVd4_TWI7MbRjRqqypZjKF-rm8MZYOcyzj0y0eOGHQU=', '', 'api_client', '58.11.71.81', 'PostmanRuntime/7.44.1', '2025-08-01 10:53:38', '2025-08-31 10:47:04', 1, '2025-08-01 10:47:04', '2025-08-01 10:53:38'),
(4, 8, 'a38c347e-60e8-4551-b5ce-3ec8c5de5e3d', '7rrvrM4re7D-aBNjprazG2mV4ERsG4HeCrY0JTmssws=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-01 12:38:50', '2025-08-31 11:18:50', 0, '2025-08-01 11:18:50', '2025-08-01 12:38:50'),
(5, 7, 'd76f7681-177e-4bd2-87e2-3a59b7384dfb', 'qWJwclseH-mQGLeitlCefczy8Krool17SxR9aLuxWMg=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-01 12:39:08', '2025-08-31 12:39:02', 1, '2025-08-01 12:39:02', '2025-08-01 12:39:08'),
(6, 8, '20d9f565-a45f-419b-b0e9-04afe15cf91e', 'VCy7bUlUN2oVJzftirdn5_6BZ5P_MEkpvHaRqbVsalo=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-01 23:28:10', '2025-08-31 22:20:17', 0, '2025-08-01 22:20:17', '2025-08-01 23:28:10'),
(7, 8, '32c223d3-098c-437f-82be-aabb41dd005d', 'YC99GxtX3hkbFXcI8byT5gsb0N0SdvRhbzVtWQre9TI=', 'Firefox Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0', '2025-08-01 23:28:35', '2025-08-31 23:28:32', 1, '2025-08-01 23:28:32', '2025-08-01 23:28:35'),
(8, 8, 'f92c5caa-3b62-4689-96bc-224a64d51678', 'a4r9RANIdKjeiBeVH_Z8Wpfuvb975SInLfOVpl0p178=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-01 23:34:08', '2025-08-31 23:28:46', 1, '2025-08-01 23:28:46', '2025-08-01 23:34:08'),
(9, 7, '5620991e-5ad9-43ae-a83e-a766c3cb64fe', 'ZaaMGnGs6zSWew6d6mrvgppHV5xlAQHwOCYK-cHLQKY=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-02 11:19:54', '2025-09-01 09:33:36', 0, '2025-08-02 09:33:36', '2025-08-02 11:19:54'),
(10, 8, 'f2f80462-c5e2-4240-8c0d-502954c84c81', 'iFPBvyz_os7_bwBZSW7BlcR2ziK6TejceOLt3nCtUyc=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-02 12:18:39', '2025-09-01 11:20:00', 0, '2025-08-02 11:20:00', '2025-08-02 12:18:39'),
(11, 8, '7d54ac1c-e509-4961-8224-b4a362a42f2c', 'LI-uQrJmZm72wg_pEySnuwjLH4UtqRjmq19vgreIbnA=', 'Chrome Browser', 'web', '58.11.71.81', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-02 11:48:31', '2025-09-01 11:38:55', 0, '2025-08-02 11:38:55', '2025-08-02 11:48:31'),
(12, 7, 'd66d2ff0-8227-4e83-9a4d-a8f68c325fcc', 'gSy68A17rp2f5UAwHCQCf_EShV3Ey42DP4IPAJkaxRE=', 'Chrome Browser', 'web', '58.11.71.81', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-02 12:44:07', '2025-09-01 11:49:07', 1, '2025-08-02 11:49:07', '2025-08-02 12:44:07'),
(13, 8, 'aea43d43-8566-440c-b792-b31d7e57881e', 'x5JgcPQU6nexukJoOL1ptMl4fP3YfZWZoZMVvdpjkTQ=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-02 12:19:29', '2025-09-01 12:18:51', 0, '2025-08-02 12:18:51', '2025-08-02 12:19:29'),
(14, 7, '5d2eeadd-e4a9-43de-99c3-1e477553f494', '8KWDhDFGFpovsh4eF6V8jB_FDvlCzUhTfmq2nNs0Hkg=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-02 12:55:22', '2025-09-01 12:19:34', 0, '2025-08-02 12:19:34', '2025-08-02 12:55:22'),
(15, 8, 'd9f974a4-9fb5-4d33-9543-a460607517f8', '0dYkJ1jWUDUw5biyKEbT8Ov7H_wrYKrfIzYBlwZ-YlY=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-02 12:55:35', '2025-09-01 12:55:30', 1, '2025-08-02 12:55:30', '2025-08-02 12:55:35'),
(16, 8, '530005e7-0b72-4c33-8715-d4e916c07fb6', 'BK8Ti6v5sXAfb9X3h16af16bv7-9mngUDHEVu08Z6Cw=', 'Chrome Browser', 'web', '58.10.107.148', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-02 20:32:49', '2025-09-01 20:13:21', 1, '2025-08-02 20:13:21', '2025-08-02 20:32:49'),
(17, 8, '1358ac14-9895-4064-b1cc-96d29836b801', '5W6do2fLNXTtsfiDa2n2Gy93fJK--VKw0j4guLVE6c4=', 'Chrome Browser', 'web', '124.122.123.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-04 21:10:38', '2025-09-03 13:06:25', 1, '2025-08-04 13:06:25', '2025-08-04 21:10:38'),
(18, 8, '7844df3f-e67c-4f86-81f1-396fb5d4e46e', 'EABql958klcSJO0gVVFqJBaqymknHtld3hOW-SFSY4E=', 'Chrome Browser', 'web', '124.122.123.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-05 01:28:04', '2025-09-04 01:04:32', 1, '2025-08-05 01:04:32', '2025-08-05 01:28:04'),
(19, 8, 'cfd6960b-ffca-41b2-9778-dbafcc3e5ae6', 'lyqgKhvv2sCDJZ-WP6FmIoMREDYllYnjZguXjGU_NLw=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-05 22:46:59', '2025-09-04 11:43:31', 1, '2025-08-05 11:43:31', '2025-08-05 22:46:59'),
(20, 8, '35b471e0-3892-4446-9580-1f89fd7396ad', 'mqWtgKjFHhlQO77MikMrljHlIZbUqnRnGY5AQHKTazE=', 'Chrome Browser', 'web', '58.11.71.81', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-05 12:47:41', '2025-09-04 12:01:21', 1, '2025-08-05 12:01:21', '2025-08-05 12:47:41'),
(21, 8, '92904a9f-aa67-4327-8d09-950d8338dcce', 'fmnJrIYgYGGLftYtEwIgi-rJaFgEjn7W-JViZZvY-hE=', 'Chrome Browser', 'web', '58.11.71.81', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-05 13:10:28', '2025-09-04 13:10:04', 1, '2025-08-05 13:10:04', '2025-08-05 13:10:28'),
(22, 8, 'cf2856f9-b3cd-4961-bc7d-0c16be3780ef', 'UuvjFwMCJUuPx3GDXZlL9lasHOU8BnqCbmxSHWitA1U=', 'Chrome Browser', 'web', '58.11.71.81', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-05 22:50:58', '2025-09-04 22:50:52', 1, '2025-08-05 22:50:52', '2025-08-05 22:50:58'),
(23, 8, '9c452368-1a61-4e8e-b7d9-d68140c5dfda', 'h2_uVFJ_W2GG4726q0cNkZEjMU4BsASAKMP3Ido5G7w=', 'Chrome Browser', 'web', '58.11.71.81', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-06 12:04:59', '2025-09-05 11:40:43', 1, '2025-08-06 11:40:43', '2025-08-06 12:04:59'),
(24, 7, 'ad02a5db-95aa-4696-ae7e-30a937694fe1', '8amfrnG6pKEDlzI4AOK4IJqyqy1xu4iITV2QK611fHA=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-07 01:15:50', '2025-09-05 22:03:49', 0, '2025-08-06 22:03:49', '2025-08-07 01:15:50'),
(25, 8, '3c8f1b54-11fb-4ec0-8813-25543e237d83', 'ZkKXFYm2Ph-st0HezFOH1KSTZcx6nbVfv_y48-L2hM8=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-07 06:11:59', '2025-09-06 01:15:59', 1, '2025-08-07 01:15:59', '2025-08-07 06:11:59'),
(26, 8, '9dfd4664-ec7b-4b69-ba74-6efc3f93b5bc', 'yXI19rJ0peiTAJR3pObJ30SedqpOHQO41pOZ6hrrgpY=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-07 14:01:47', '2025-09-06 13:33:42', 1, '2025-08-07 13:33:42', '2025-08-07 14:01:47'),
(27, 7, '1038c846-1075-4783-b694-827ce31baba3', 'NrQYwvk--sVk3NqWHMWDtrEqvpQHsxCsEMaHHdxY5o0=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-07 20:17:42', '2025-09-06 16:49:03', 1, '2025-08-07 16:49:03', '2025-08-07 20:17:42'),
(28, 7, 'd5405339-1bb9-4d0e-983f-75003bee696b', '_pZeHz6qf7AzqYvrNjDue7n9biAUV_Vtl-_GxLp2xRE=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-08 12:18:12', '2025-09-07 02:33:09', 1, '2025-08-08 02:33:09', '2025-08-08 12:18:12'),
(29, 7, '673eced7-2284-441e-a38f-3507562624ab', 'L_YZpdgZYWhwOSxNMP4AnpL-Rl_ENfZ0kKbZeHEtx1w=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-08 14:58:51', '2025-09-07 14:21:38', 1, '2025-08-08 14:21:38', '2025-08-08 14:58:51'),
(30, 7, 'aa0439b1-cbf5-41b1-9a1a-24f33993fef4', 'hnsuVtjVScBDfg6wWB4PCWHXPS70IaJg7HKeCni3_Sk=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-08 18:10:44', '2025-09-07 15:40:31', 1, '2025-08-08 15:40:31', '2025-08-08 18:10:44'),
(31, 7, 'd2100b6f-b3df-4850-84fa-71ee185e3db3', 'b6A0SuteEP1MdHwP9b5HxM8CVxxJFZ7J5Gg5jjrjR0k=', '', 'api_client', '110.168.238.46', 'PostmanRuntime/7.45.0', '2025-08-08 16:07:17', '2025-09-07 16:06:12', 1, '2025-08-08 16:06:12', '2025-08-08 16:07:17'),
(32, 7, '32d78ca4-4e73-476b-b916-989094b55c3c', 'psKmPkYcoYCCXd-fK7N8YfU5aWCQX7WbHg6j4lnNjWQ=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-08 21:37:45', '2025-09-07 19:36:30', 1, '2025-08-08 19:36:30', '2025-08-08 21:37:45'),
(33, 7, '6d056236-3ae0-4d46-ac2a-33249292b20b', '26HwD33jydwP4ur37hPNWxwumGRhEcJR32h2eEk9Zs8=', 'Chrome Browser', 'web', '58.11.85.73', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-08 23:15:04', '2025-09-07 21:51:58', 1, '2025-08-08 21:51:58', '2025-08-08 23:15:04'),
(34, 7, 'f877555a-0393-4ef8-b812-a8d4cbbcbd2e', 'gm21KeuRY_yYCchc6g2lqhLRUjGPRDi0gRuctmr0UpI=', 'Chrome Browser', 'web', '58.10.128.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-10 13:13:24', '2025-09-09 12:34:52', 1, '2025-08-10 12:34:52', '2025-08-10 13:13:24'),
(35, 7, 'fbb38821-d672-4a58-8281-6a7b54daf88f', 'zeOfHeY0_oCp1iHku1MP3ovzT1AdXMm6TQ3nCQ0zjCQ=', 'Chrome Browser', 'web', '58.10.128.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-10 16:17:07', '2025-09-09 14:40:27', 1, '2025-08-10 14:40:27', '2025-08-10 16:17:07'),
(36, 7, '71d90938-ffde-40ee-abdf-2b37d563731b', 'OaWrbI5YN_2ANsFDOFVNo7B6MvRWHanWWIzryWvSWNQ=', 'Chrome Browser', 'web', '58.10.128.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-10 23:25:26', '2025-09-09 18:37:16', 1, '2025-08-10 18:37:16', '2025-08-10 23:25:26'),
(37, 7, '6bb3f0f2-16ac-435f-9db8-36dceefb18e5', 'kbV58DjZt7Vv482MW9p7d0LHcuW-kfR8DGw1DA24iLs=', 'Chrome Browser', 'web', '58.10.128.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-11 12:10:58', '2025-09-10 11:48:46', 0, '2025-08-11 11:48:46', '2025-08-11 12:10:58'),
(38, 8, 'b2558194-5108-4ccf-8cd1-a4b4c7718966', 'fwkRgmgs1wu4RCJ1V1j7JlB4PuHCSg6IhYLPoWG2pdc=', 'Chrome Browser', 'web', '110.168.238.46', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-11 12:07:59', '2025-09-10 12:04:26', 1, '2025-08-11 12:04:26', '2025-08-11 12:07:59'),
(39, 8, '2e7b2cd4-26a3-4b76-8ce9-57afb257ee44', 'N7yzEXoJiIFLxDN55tbs6hxUGGh4FGOT6hXKAAt5Lfs=', 'Chrome Browser', 'web', '58.10.128.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-11 12:14:58', '2025-09-10 12:11:04', 0, '2025-08-11 12:11:04', '2025-08-11 12:14:58'),
(40, 7, '024be4c9-9002-4bbf-bedf-5cb12146d138', 'TRaeEZ4VDx2V6gTLdTKTbd2YKXQFMqyN_tF-_3G5p7s=', 'Chrome Browser', 'web', '58.10.128.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-11 13:14:19', '2025-09-10 12:15:03', 1, '2025-08-11 12:15:03', '2025-08-11 13:14:19'),
(41, 7, 'b6d68c42-2ac6-494c-a58d-323b5cabe4a5', 'iAjnkwYNartyyJUy1K3ExGqFcFABFexI1ZRlmISb51Q=', 'Chrome Browser', 'web', '58.10.128.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-11 13:56:03', '2025-09-10 13:15:19', 1, '2025-08-11 13:15:19', '2025-08-11 13:56:03'),
(42, 8, 'bd0c9df4-12d0-4519-8c66-ba559a3b95f5', 'jkFrX8YMasUmiKJ07vtsjc1ASPL9qT0Fs6NYZjc3zLg=', 'Chrome Browser', 'web', '110.168.238.46', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-11 14:16:19', '2025-09-10 13:35:18', 1, '2025-08-11 13:35:18', '2025-08-11 14:16:19'),
(43, 8, '4a3b1bfc-4f2d-4acb-9ec6-e76071d5c0b1', 'VctAjiipLu81JtbhK5KH-uu1NnoWfjmUbEZ5FszLTBs=', 'Chrome Browser', 'web', '110.168.238.46', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-11 22:14:31', '2025-09-10 20:11:29', 1, '2025-08-11 20:11:29', '2025-08-11 22:14:31'),
(44, 8, '8270dd78-dc35-43ae-8690-6b881520df6a', 'IXFyHy46RSfjtGoy171aYO21NSHQQ6y1wMKWJJ54qCc=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-12 05:09:35', '2025-09-11 05:08:41', 0, '2025-08-12 05:08:41', '2025-08-12 05:09:35'),
(45, 7, 'dc875c4f-2038-4b43-93ad-dd9559eb49a0', 'FoOd1dkiuA3mpwzaKQncos7ktpidbmLEGhBx-24baNk=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-12 05:35:26', '2025-09-11 05:09:39', 0, '2025-08-12 05:09:39', '2025-08-12 05:35:26'),
(46, 7, '5b6b14b3-ab5d-4db9-a1a3-ea2f58d41b49', 'bwg9ZafDSKoEZ9QxeIk-N_L-jk4ByrNM87Pwj1Wlb6s=', 'Chrome Browser', 'web', '58.10.128.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-12 05:17:46', '2025-09-11 05:14:30', 0, '2025-08-12 05:14:30', '2025-08-12 05:17:46'),
(47, 8, '9f02833e-b5c4-48a5-a1d5-73c4205625b6', '2IoetuIMkYs-w7X_i5LOl3aS-13uiTRv3Wywwo-FahQ=', 'Chrome Browser', 'web', '58.10.128.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-12 06:03:48', '2025-09-11 05:17:57', 1, '2025-08-12 05:17:57', '2025-08-12 06:03:48'),
(48, 8, 'b598c193-6e30-4be1-a9ae-4c35e9cda80a', 'phBNr1qYw2w51aud1Z6tieJ92BZtQ0HrBA5eovKlwBM=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36', '2025-08-13 13:18:19', '2025-09-12 11:42:59', 1, '2025-08-13 11:42:59', '2025-08-13 13:18:19'),
(49, 8, '02471e2e-e7b0-4c1d-8a00-ac3e8b44f10d', 'hsoMuTwW7o2ZBVsqx1OCapUBlv5xbWDry6BxNG_8gg0=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-15 12:15:07', '2025-09-14 11:37:05', 1, '2025-08-15 11:37:05', '2025-08-15 12:15:07'),
(50, 8, 'd409b552-abb7-4136-81c3-d7dd31001f62', 'Gy-6dbq7KGtG-A8JVQMaASP52Dq28Te_66nnGMT4xt0=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-15 18:27:28', '2025-09-14 17:44:19', 1, '2025-08-15 17:44:19', '2025-08-15 18:27:28'),
(51, 8, 'fcc9b58f-a30d-4033-bc39-a33808093c3d', 'h7PLs6dU27wd7R-mMClhhLJokiCSlOB8KFsXidb8eeo=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-15 22:42:45', '2025-09-14 22:02:31', 1, '2025-08-15 22:02:31', '2025-08-15 22:42:45'),
(52, 8, 'e079e4cc-b0a3-43b5-bcd0-1bb98cf38df2', 'p7YYvscatu-GtkTDd2eOYgkyUyqY5klGCYx4Lf1oV_M=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-16 11:55:41', '2025-09-15 11:22:23', 1, '2025-08-16 11:22:23', '2025-08-16 11:55:41'),
(53, 8, '23098be3-a9f9-4f18-9380-0b9009e7d04e', 'iud6o_YnknREU2ZudtWtuwCv1rvNsRIYy3M0mesrh-U=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-16 17:18:38', '2025-09-15 17:18:20', 1, '2025-08-16 17:18:20', '2025-08-16 17:18:38'),
(54, 8, '8f6e9a09-f247-4158-8373-63fb24194672', 'Od8MoBHOugpZzJcD9wT-v4TSaQsSbh8FEyZmQBea2TQ=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-16 23:25:07', '2025-09-15 22:40:32', 1, '2025-08-16 22:40:32', '2025-08-16 23:25:07'),
(55, 8, 'caa87d66-e49e-44e7-b60b-5a14b40c8da5', 'gfrLMUYBAKQqaIdFVDA6bnasF1bgrExuJHbNoWeOvGM=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-17 09:00:57', '2025-09-16 08:56:04', 0, '2025-08-17 08:56:04', '2025-08-17 09:00:57'),
(56, 7, '60938321-5ae9-4318-a289-544c597de98e', '-uAin-AvWvdaZwBfLWHwD4_jFPwCpsbFpsiWpUuA5gg=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-17 09:00:48', '2025-09-16 09:00:41', 1, '2025-08-17 09:00:41', '2025-08-17 09:00:48'),
(57, 8, '411e242a-c558-4c45-ab67-42ba0f3398d5', 'fZhVhKszDMkdl6Usyrw--x1ZHRAccYH4I81UyTxR_0g=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-17 09:49:01', '2025-09-16 09:01:00', 1, '2025-08-17 09:01:00', '2025-08-17 09:49:01'),
(58, 8, '738f887d-786e-45c5-a12f-789db51a92f7', '8S8qQ126HW4pnZAE1PeAg-MyQKyqUlA_gAlWWP7N260=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-17 19:27:48', '2025-09-16 19:19:04', 1, '2025-08-17 19:19:04', '2025-08-17 19:27:48'),
(59, 8, '79ce9534-846f-4318-9cbf-1bee608b2ba9', 'dTUjHjPRWgTBuy4OqKzlDblDQYd8JlAZcH-q6LAtydc=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-18 13:42:49', '2025-09-17 10:57:49', 1, '2025-08-18 10:57:49', '2025-08-18 13:42:49'),
(60, 8, 'a5282275-aa7c-4509-ae2a-c72a4787b42e', '2Ic2IZTbUCkIqWltTJ1EyIpsxrC4E8qqmrRNE-6UA2M=', 'Chrome Browser', 'web', '58.11.84.239', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-18 11:44:35', '2025-09-17 11:27:11', 1, '2025-08-18 11:27:11', '2025-08-18 11:44:35'),
(61, 8, 'fcb318a8-be6c-4a2a-95a9-a72f7ef9c7d2', 'byIbHv1zmuhDU0U81JxT0Can0kbqizmU8QJ2Wyjtdmc=', 'Chrome Browser', 'web', '58.11.83.106', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-18 11:45:14', '2025-09-17 11:29:03', 1, '2025-08-18 11:29:03', '2025-08-18 11:45:14'),
(62, 7, 'a66e3f14-c78f-40fe-b8aa-3ea8ef194c98', 'nqnDsdZMJ7ZFK4k2OESwtcoWyO5EhYLZr8dIw6lEg0c=', 'Chrome Browser', 'web', '58.11.83.106', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-18 23:11:59', '2025-09-17 17:43:57', 1, '2025-08-18 17:43:57', '2025-08-18 23:11:59'),
(63, 7, 'c22f049d-478e-4846-9f7f-03d9b658ccb5', 'o4V4iixAUCe0rEssC12tVjkOpNoiIDxfwDNfwaMMtpk=', 'Chrome Browser', 'web', '58.10.73.81', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-19 09:55:34', '2025-09-18 09:30:36', 0, '2025-08-19 09:30:36', '2025-08-19 09:55:34'),
(64, 8, 'fbbb8a9d-684a-41d1-a4ba-0f563077cd1b', 'a6ukk8xJmp01v6JZKwlGSnp8GUxU-FssgXuEd5gyYm4=', 'Chrome Browser', 'web', '58.10.73.81', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36 Edg/139.0.0.0', '2025-08-19 09:55:12', '2025-09-18 09:51:20', 1, '2025-08-19 09:51:20', '2025-08-19 09:55:12'),
(65, 8, '5cdcd954-b131-422d-a843-8b5cec3ff508', 'YGoHFeH4JejOzsONRGI58j5lMLZjHAm2sif-6OAU53I=', 'Chrome Browser', 'web', '58.10.73.81', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-20 01:45:00', '2025-09-18 09:55:41', 1, '2025-08-19 09:55:41', '2025-08-20 01:45:00'),
(66, 8, 'a38a56f8-98f9-47b7-9e66-7e6f2efb1c8b', 'h9ksWLBEnZyvmvVJh1yLPsvDqI8qR7ZFux4_0ISWvlM=', 'Chrome Browser', 'web', '58.10.140.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-19 12:03:34', '2025-09-18 11:05:04', 1, '2025-08-19 11:05:04', '2025-08-19 12:03:34'),
(67, 8, '8d2cab0e-0800-45f9-80e5-8894cb6eb7db', 'BU-NKZes65X9MYUXOLxTYNnWstt-B4yyTmztb_lPdGY=', 'Chrome Browser', 'web', '58.10.140.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-19 16:16:57', '2025-09-18 15:38:50', 1, '2025-08-19 15:38:50', '2025-08-19 16:16:57'),
(68, 8, '85791753-ea53-4668-acec-3202cf5e9d91', 'fN8nsKW_54FeoM7ghqVhmAO9FLngqPdzzyOScTVJZSU=', 'Chrome Browser', 'web', '58.10.140.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-19 18:03:56', '2025-09-18 17:47:44', 1, '2025-08-19 17:47:44', '2025-08-19 18:03:56'),
(69, 8, 'c2b8726a-c73b-4215-befd-d3391fbb5ec8', '8d10y94bqHEeVWAEscyfGc6kIR5aXEHziwL8UiJDpVg=', 'Chrome Browser', 'web', '58.10.140.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-20 14:04:17', '2025-09-19 13:28:07', 1, '2025-08-20 13:28:07', '2025-08-20 14:04:17'),
(70, 8, 'd34d01a4-93bb-4be0-b9ea-e35277668480', 'fHp_ODwVbl1puidJ_v0VDB-tJrVl6KaksA3TISMDd_o=', 'Chrome Browser', 'web', '58.11.79.179', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-21 05:56:31', '2025-09-19 14:15:40', 0, '2025-08-20 14:15:40', '2025-08-21 05:56:31'),
(71, 8, 'c23e323e-9ad1-4275-875a-79a5899d5cd5', 'd4Yb2tGiqcnC3zXFPmncroSF_fZ9LPv-lLL4-ko2zY4=', 'Chrome Browser', 'web', '58.10.140.241', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-20 17:06:00', '2025-09-19 16:25:09', 1, '2025-08-20 16:25:09', '2025-08-20 17:06:00'),
(72, 7, '2090f135-1651-47a7-b74a-fd59f1bf9704', 'sYtEOv2COuoDn6tdgvAqaTtX1-nSfdDfQ33cNj5vh9A=', 'Chrome Browser', 'web', '58.11.79.179', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-21 08:41:37', '2025-09-20 05:56:38', 1, '2025-08-21 05:56:38', '2025-08-21 08:41:37'),
(73, 7, 'b2319d9c-45f7-4044-9949-e2104d590a42', '4_Ek8UW6Ihk48NSHxfRlWJFLyiySlhAY2U0m32DcKFg=', 'Chrome Browser', 'web', '58.10.71.142', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-21 18:05:20', '2025-09-20 14:50:16', 1, '2025-08-21 14:50:16', '2025-08-21 18:05:20'),
(74, 8, '5bd4aadf-5f18-487c-b0f5-cb9da6c2c275', 'XG3M7Fj4OLIXtEUIPdvBjqbQr89169Q1Jp2heETdIJ4=', 'Chrome Browser', 'web', '58.10.71.142', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 03:27:17', '2025-09-21 03:18:03', 0, '2025-08-22 03:18:03', '2025-08-22 03:27:17'),
(75, 7, 'a3487363-5ab0-4807-af34-3b4b77da171a', 'E_geKerdRbB64ATv30Eag_ggWH2dcao4M6lMFyvdhQw=', 'Chrome Browser', 'web', '58.10.71.142', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 04:07:07', '2025-09-21 03:27:27', 0, '2025-08-22 03:27:27', '2025-08-22 04:07:07'),
(76, 8, 'e3503c19-1e7c-4ea7-803c-caa46cb6c824', 'HqeNHEtTCvutm1rftYO74evxkhx0PGcqmxeIHO5OF1k=', 'Chrome Browser', 'web', '58.10.71.142', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 10:28:37', '2025-09-21 04:07:13', 0, '2025-08-22 04:07:13', '2025-08-22 10:28:37'),
(77, 8, '4c15cf27-0df1-41d0-b54c-f12e5b701003', 'LR_aUatosWPXIsflxe6YuTBdXj1Keah2DXQDE1BIzQc=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 11:05:38', '2025-09-21 09:31:16', 0, '2025-08-22 09:31:16', '2025-08-22 11:05:38'),
(78, 7, '6638cea7-ea1d-4fa3-9e35-379f87ba2e2b', 'dT9QFYhyL-L9aKBHqZ6XK4ZgJFyxacHLY-ZsV5753MU=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 10:29:02', '2025-09-21 10:28:45', 0, '2025-08-22 10:28:45', '2025-08-22 10:29:02'),
(79, 8, 'c1df6592-3c7c-43e7-8dce-a82437364db5', 'WcLaSyCy1ecuLK0hdZdmZh2kbSSmU5V830Pc-k_2Tzc=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 11:27:59', '2025-09-21 10:29:12', 1, '2025-08-22 10:29:12', '2025-08-22 11:27:59'),
(80, 8, '7e04bcb8-232a-4821-9ef1-a0e5f63af66c', '0Aeu-wqFq8zOAFN6ZakwvLoI30X3SRTNkYuPmjQjcmc=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 11:09:02', '2025-09-21 11:06:16', 0, '2025-08-22 11:06:16', '2025-08-22 11:09:02'),
(81, 7, '1ddd6a0b-ba96-42be-a324-d591a3fb4a8b', '-mJvRL5XXqcx_WUBXWKXX2Fbtu8Eww3-jcjGeYbP4Mk=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 12:17:02', '2025-09-21 11:09:17', 0, '2025-08-22 11:09:17', '2025-08-22 12:17:02'),
(82, 8, '836f256b-c573-489d-a604-c7151a98a55f', '66APBp0GokYRrZ8hkfIsE4LtT-jhx1b89HEBucYKSpw=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 11:31:48', '2025-09-21 11:20:18', 1, '2025-08-22 11:20:18', '2025-08-22 11:31:48'),
(83, 8, '94123437-4df3-4150-9899-f5341bd2c22e', 'rrBY5pqtfMOFtSJGHCykffh0dX46U2_S_U_V00ofq3Q=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 12:05:26', '2025-09-21 11:47:35', 1, '2025-08-22 11:47:35', '2025-08-22 12:05:26'),
(84, 8, '49eed656-30bb-4679-bf9f-22776c858850', 'xNxLws0kgG7T-slD4d1fseGlgsW1vrOYRiFoT3gWk2I=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 13:16:36', '2025-09-21 12:17:05', 1, '2025-08-22 12:17:05', '2025-08-22 13:16:36'),
(85, 8, '308e51d3-4207-4960-be44-c0f0e001b72f', 'uJ_EgTBO1Q3gQKDwC2p4zYf-AkMErbhxnUJ5ICK5A98=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 13:05:20', '2025-09-21 13:05:10', 0, '2025-08-22 13:05:10', '2025-08-22 13:05:20'),
(86, 7, '264c4e90-20bd-4ea8-8c4b-09c3ba8e1ad8', 'UVL5pIjTze90_fgO4SLD3OmuIT1hdM3Zf7HlOOy8b8Q=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 13:08:17', '2025-09-21 13:05:23', 1, '2025-08-22 13:05:23', '2025-08-22 13:08:17'),
(87, 8, 'e86b2f39-0850-4f37-a3e2-7a32d0612050', 'XkpusDTD5o2Obeencyn70Ib34pEIDj53RTolijWZItw=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 13:17:31', '2025-09-21 13:17:27', 1, '2025-08-22 13:17:27', '2025-08-22 13:17:31'),
(88, 8, '539b4ff4-2dcf-403b-a71c-b2f772d72b40', 'FHjPT1446XEQDS6pBXTHbEdSfZ4iJcD7H0MQnG-yuwk=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 14:02:30', '2025-09-21 13:25:47', 1, '2025-08-22 13:25:47', '2025-08-22 14:02:30'),
(89, 7, 'ecf93f12-fb0e-4a46-964f-9709a85baf63', 'aFyhanlOuCbRMIF4ZkLNuW-ddY0cL_NMrQP5kC0AOWA=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 13:28:49', '2025-09-21 13:27:03', 1, '2025-08-22 13:27:03', '2025-08-22 13:28:49'),
(90, 8, 'c2bf5741-1cf6-469c-ad87-335cc18b7212', '9bgN593g-ZNPFLcmRh-czV5VeEVr4aK94uV5HiLE_WE=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 15:15:29', '2025-09-21 14:34:30', 1, '2025-08-22 14:34:30', '2025-08-22 15:15:29'),
(91, 8, '8c6b5cc7-85cc-489e-af12-a784a8d1de92', 'FpFNDIDgM_k65aEgFlAAkGB8ZiRvtTfJFmhK4Ar8ywQ=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 15:38:34', '2025-09-21 15:31:15', 1, '2025-08-22 15:31:15', '2025-08-22 15:38:34'),
(92, 8, '73ca3714-2880-4dd8-b66e-3720e4d6d1e8', 'C3oPsD_qT2Q187NPxFYPDZJElc8dP-1CJJRGz07osBI=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 17:15:50', '2025-09-21 15:58:26', 1, '2025-08-22 15:58:26', '2025-08-22 17:15:50'),
(93, 8, '4c7b26e9-62aa-42ca-a19b-b2e268c250dd', 'sMPg7MWAFCw4Q823S7aFmj_SrxRtcJelLJ0XCJc-JJo=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 22:40:31', '2025-09-21 18:41:48', 1, '2025-08-22 18:41:48', '2025-08-22 22:40:31'),
(94, 7, '9ad8f802-36e9-457c-9245-40eddb01c194', 'TqrEznLz2oAkOmqTB5U_PkL6I4xtJtxqZPEcQmsiRTw=', '', 'api_client', '58.10.155.36', 'PostmanRuntime/7.45.0', '2025-08-22 19:02:25', '2025-09-21 19:02:25', 1, '2025-08-22 19:02:25', '2025-08-22 19:02:25'),
(95, 8, '141282c7-5bc2-478f-95fc-e8e38bd203aa', 'VlZSRTAwBBcMY7SNdXOz984v9wgi7JXS6DUUiBxcMK0=', '', 'api_client', '58.10.155.36', 'PostmanRuntime/7.45.0', '2025-08-22 19:03:47', '2025-09-21 19:02:47', 1, '2025-08-22 19:02:47', '2025-08-22 19:03:47'),
(96, 8, '903bfe35-2e1c-4652-8b86-333c609fa6ff', 'ZSX5cbHwrJfLPE35ffalDyusEYG58NGf2SM5Gz6t_q8=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-22 23:41:31', '2025-09-21 22:57:56', 1, '2025-08-22 22:57:56', '2025-08-22 23:41:31'),
(97, 7, '008d1278-25f8-4104-8f3b-b2ad8ff2a7d3', 'iOPDusVzQkASkmRVmHn3FbFqu_Azc95PCYgbRDbb4G8=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 04:59:24', '2025-09-22 04:58:44', 1, '2025-08-23 04:58:44', '2025-08-23 04:59:24'),
(98, 7, '4f76dc6b-e6c6-4095-9544-af093368723c', 'zQ-hmQBc0_f386Au_V8SPvNjBThy7A0ByqryiUCipxI=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 07:26:58', '2025-09-22 07:26:49', 0, '2025-08-23 07:26:49', '2025-08-23 07:26:58'),
(99, 8, '489658c4-4a2c-4089-8ab9-bd0de274c769', '5au8V4OH8SRnAWT7cByqmrNzP1TXdZo3naiGQfPQVR8=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 07:28:43', '2025-09-22 07:27:11', 0, '2025-08-23 07:27:11', '2025-08-23 07:28:43'),
(100, 7, '81438979-96f1-4ca2-ba1b-4418cb309ab2', 'WIKJVQLIEE5lTprk9onoWOrfhOHiendslI4AO1aR5T8=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 09:03:07', '2025-09-22 07:28:49', 0, '2025-08-23 07:28:49', '2025-08-23 09:03:07'),
(101, 8, 'b70e8278-bf9b-4e60-9b99-d4c111de29b0', 'LARTpQPggSvLyLO63aPULXfy1nu9SjA_tMgHi0bzsdg=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 09:12:07', '2025-09-22 08:21:13', 0, '2025-08-23 08:21:13', '2025-08-23 09:12:07'),
(102, 7, '7d5355a0-ba82-46b3-92d9-f07c695d0cb4', '4PZdZQOPy_4tDCA6gh2YfYM_VqXuJs-mp31A4gk4rGw=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 09:07:37', '2025-09-22 09:03:22', 0, '2025-08-23 09:03:22', '2025-08-23 09:07:37'),
(103, 7, '44f72392-6e4a-4169-b8e4-eab0307fbee8', 'utDCbBJwLnPLlqYgHan1welntpvzkqozbHFBErQRhfQ=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 09:11:01', '2025-09-22 09:03:37', 0, '2025-08-23 09:03:37', '2025-08-23 09:11:01'),
(104, 7, 'ccf5ca5d-6970-4e6a-9ca6-e618346eadb3', 'a1fxhAFIJ_ztw67g8Q4womW5AHFV6pxcMXDXUDOFDeo=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 09:14:05', '2025-09-22 09:07:44', 1, '2025-08-23 09:07:44', '2025-08-23 09:14:05'),
(105, 8, '25001a18-6134-43e1-ad93-8d214f7ee12f', 'AFQJeaenmNbz2DbYtpEe_zcnGIZo21zoJGbz63CbMTc=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 10:21:06', '2025-09-22 09:11:12', 0, '2025-08-23 09:11:12', '2025-08-23 10:21:06'),
(106, 7, 'a3307ccc-2284-40a2-b1c8-78c0061a5191', 'JjUFYtBYGfSYTY5II7vHUznwSm1feamoonLkDekf65c=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 11:04:39', '2025-09-22 09:12:10', 0, '2025-08-23 09:12:10', '2025-08-23 11:04:39'),
(107, 7, 'cefa2b03-9ded-4910-ae66-4140faa3c11a', '-G9pid_z8ZEtLW30FKhpdW2ru1IGp-_L_gXchk2zkno=', 'Chrome Browser', 'web', '27.145.211.127', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 11:11:56', '2025-09-22 10:20:23', 1, '2025-08-23 10:20:23', '2025-08-23 11:11:56'),
(108, 8, 'a51e248a-0e4a-4caa-90e8-bf5ee73d5177', 'MFKgUR2MvXXt6vpkWo7De02HClNBnQwzaJtWTksjLjI=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 10:25:31', '2025-09-22 10:22:44', 0, '2025-08-23 10:22:44', '2025-08-23 10:25:31'),
(109, 7, '2fa9a9df-2d54-4fbf-8b62-c8b2133ec59f', 'n0FMnXHWB9mDpqPVS_rMxm_CgTCjptc9jy04YmsZNY4=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 11:03:40', '2025-09-22 10:25:33', 0, '2025-08-23 10:25:33', '2025-08-23 11:03:40'),
(110, 8, '7796cf26-8527-4257-913d-b56d656ed624', 'jvRtFXk7d6AAuwW9xqBR8TM7INEmnpKoHJjzaAUrD4w=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 11:10:19', '2025-09-22 11:04:41', 0, '2025-08-23 11:04:41', '2025-08-23 11:10:19'),
(111, 7, '9519dcb3-8c57-4310-bf83-dbfd1025b28b', 'Lbc_8wSXjHSlZQc_F4ifBqpYtQ676HblpBYn5yGimGo=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 11:06:21', '2025-09-22 11:05:57', 1, '2025-08-23 11:05:57', '2025-08-23 11:06:21'),
(112, 7, '1fff7b83-1f65-4a33-97ce-c937f48a91bc', '1sT50x0OUVOwZBH6e2N3Td4EjXDXjlTEyckkctP3g18=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 11:10:38', '2025-09-22 11:10:22', 0, '2025-08-23 11:10:22', '2025-08-23 11:10:38'),
(113, 8, '23822771-8e6c-4acd-9399-63244057a65a', 'V6Ix5RsnPo3LMKGeaPT7gh7Dj4T0qaVdPEfZseUxoAw=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 11:12:45', '2025-09-22 11:10:42', 1, '2025-08-23 11:10:42', '2025-08-23 11:12:45'),
(114, 8, '839a8d9d-7db1-4d04-a406-63704bee47c9', 'eYHHm2B6wMNSHi-Usx-QtdyO9SrFNt2UxUphwbVcDZ4=', 'Chrome Browser', 'web', '58.10.155.36', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36', '2025-08-23 11:20:49', '2025-09-22 11:19:55', 1, '2025-08-23 11:19:55', '2025-08-23 11:20:49');

-- --------------------------------------------------------

--
-- Table structure for table `user_tokens`
--

CREATE TABLE `user_tokens` (
  `token_id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `token_type` varchar(50) DEFAULT 'refresh',
  `token` varchar(500) NOT NULL,
  `expires_at` datetime NOT NULL,
  `is_revoked` tinyint(1) DEFAULT 0,
  `device_info` varchar(255) DEFAULT NULL,
  `ip_address` varchar(45) DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

--
-- Dumping data for table `user_tokens`
--

INSERT INTO `user_tokens` (`token_id`, `user_id`, `token_type`, `token`, `expires_at`, `is_revoked`, `device_info`, `ip_address`, `created_at`, `updated_at`) VALUES
(1, 8, 'refresh', '43B-KeFXrOD_lC0--z6IQxuwnUhboU20R3gBY9TYaUc=', '2025-08-30 12:53:02', 0, 'Chrome Browser / web', '58.10.107.148', '2025-07-31 12:53:02', '2025-07-31 12:53:02'),
(2, 8, 'refresh', 'QhD-W6fwnfKAVhkbr7gLtuEZqF_9VuhVOA8kbcCIlwk=', '2025-08-31 10:15:46', 0, 'Chrome Browser / web', '58.10.107.148', '2025-08-01 10:15:46', '2025-08-01 10:15:46'),
(3, 8, 'refresh', 'WVd4_TWI7MbRjRqqypZjKF-rm8MZYOcyzj0y0eOGHQU=', '2025-08-31 10:47:04', 0, ' / api_client', '58.11.71.81', '2025-08-01 10:47:04', '2025-08-01 10:47:04'),
(4, 8, 'refresh', '7rrvrM4re7D-aBNjprazG2mV4ERsG4HeCrY0JTmssws=', '2025-08-31 11:18:50', 1, 'Chrome Browser / web', '58.10.107.148', '2025-08-01 11:18:50', '2025-08-01 12:38:50'),
(5, 7, 'refresh', 'qWJwclseH-mQGLeitlCefczy8Krool17SxR9aLuxWMg=', '2025-08-31 12:39:02', 0, 'Chrome Browser / web', '58.10.107.148', '2025-08-01 12:39:02', '2025-08-01 12:39:02'),
(6, 8, 'refresh', 'VCy7bUlUN2oVJzftirdn5_6BZ5P_MEkpvHaRqbVsalo=', '2025-08-31 22:20:17', 1, 'Chrome Browser / web', '58.10.107.148', '2025-08-01 22:20:17', '2025-08-01 23:28:10'),
(7, 8, 'refresh', 'YC99GxtX3hkbFXcI8byT5gsb0N0SdvRhbzVtWQre9TI=', '2025-08-31 23:28:32', 0, 'Firefox Browser / web', '58.10.107.148', '2025-08-01 23:28:32', '2025-08-01 23:28:32'),
(8, 8, 'refresh', 'a4r9RANIdKjeiBeVH_Z8Wpfuvb975SInLfOVpl0p178=', '2025-08-31 23:28:46', 0, 'Chrome Browser / web', '58.10.107.148', '2025-08-01 23:28:46', '2025-08-01 23:28:46'),
(9, 7, 'refresh', 'ZaaMGnGs6zSWew6d6mrvgppHV5xlAQHwOCYK-cHLQKY=', '2025-09-01 09:33:36', 1, 'Chrome Browser / web', '58.10.107.148', '2025-08-02 09:33:36', '2025-08-02 11:19:54'),
(10, 8, 'refresh', 'iFPBvyz_os7_bwBZSW7BlcR2ziK6TejceOLt3nCtUyc=', '2025-09-01 11:20:00', 1, 'Chrome Browser / web', '58.10.107.148', '2025-08-02 11:20:00', '2025-08-02 12:18:39'),
(11, 8, 'refresh', 'LI-uQrJmZm72wg_pEySnuwjLH4UtqRjmq19vgreIbnA=', '2025-09-01 11:38:55', 1, 'Chrome Browser / web', '58.11.71.81', '2025-08-02 11:38:55', '2025-08-02 11:48:31'),
(12, 7, 'refresh', 'gSy68A17rp2f5UAwHCQCf_EShV3Ey42DP4IPAJkaxRE=', '2025-09-01 11:49:07', 0, 'Chrome Browser / web', '58.11.71.81', '2025-08-02 11:49:07', '2025-08-02 11:49:07'),
(13, 8, 'refresh', 'x5JgcPQU6nexukJoOL1ptMl4fP3YfZWZoZMVvdpjkTQ=', '2025-09-01 12:18:51', 1, 'Chrome Browser / web', '58.10.107.148', '2025-08-02 12:18:51', '2025-08-02 12:19:29'),
(14, 7, 'refresh', '8KWDhDFGFpovsh4eF6V8jB_FDvlCzUhTfmq2nNs0Hkg=', '2025-09-01 12:19:34', 1, 'Chrome Browser / web', '58.10.107.148', '2025-08-02 12:19:34', '2025-08-02 12:55:22'),
(15, 8, 'refresh', '0dYkJ1jWUDUw5biyKEbT8Ov7H_wrYKrfIzYBlwZ-YlY=', '2025-09-01 12:55:30', 0, 'Chrome Browser / web', '58.10.107.148', '2025-08-02 12:55:30', '2025-08-02 12:55:30'),
(16, 8, 'refresh', 'BK8Ti6v5sXAfb9X3h16af16bv7-9mngUDHEVu08Z6Cw=', '2025-09-01 20:13:21', 0, 'Chrome Browser / web', '58.10.107.148', '2025-08-02 20:13:21', '2025-08-02 20:13:21'),
(17, 8, 'refresh', '5W6do2fLNXTtsfiDa2n2Gy93fJK--VKw0j4guLVE6c4=', '2025-09-03 13:06:25', 0, 'Chrome Browser / web', '124.122.123.241', '2025-08-04 13:06:25', '2025-08-04 13:06:25'),
(18, 8, 'refresh', 'EABql958klcSJO0gVVFqJBaqymknHtld3hOW-SFSY4E=', '2025-09-04 01:04:32', 0, 'Chrome Browser / web', '124.122.123.241', '2025-08-05 01:04:32', '2025-08-05 01:04:32'),
(19, 8, 'refresh', 'lyqgKhvv2sCDJZ-WP6FmIoMREDYllYnjZguXjGU_NLw=', '2025-09-04 11:43:31', 0, 'Chrome Browser / web', '58.11.85.73', '2025-08-05 11:43:31', '2025-08-05 11:43:31'),
(20, 8, 'refresh', 'mqWtgKjFHhlQO77MikMrljHlIZbUqnRnGY5AQHKTazE=', '2025-09-04 12:01:21', 0, 'Chrome Browser / web', '58.11.71.81', '2025-08-05 12:01:21', '2025-08-05 12:01:21'),
(21, 8, 'refresh', 'fmnJrIYgYGGLftYtEwIgi-rJaFgEjn7W-JViZZvY-hE=', '2025-09-04 13:10:04', 0, 'Chrome Browser / web', '58.11.71.81', '2025-08-05 13:10:04', '2025-08-05 13:10:04'),
(22, 8, 'refresh', 'UuvjFwMCJUuPx3GDXZlL9lasHOU8BnqCbmxSHWitA1U=', '2025-09-04 22:50:52', 0, 'Chrome Browser / web', '58.11.71.81', '2025-08-05 22:50:52', '2025-08-05 22:50:52'),
(23, 8, 'refresh', 'h2_uVFJ_W2GG4726q0cNkZEjMU4BsASAKMP3Ido5G7w=', '2025-09-05 11:40:43', 0, 'Chrome Browser / web', '58.11.71.81', '2025-08-06 11:40:43', '2025-08-06 11:40:43'),
(24, 7, 'refresh', '8amfrnG6pKEDlzI4AOK4IJqyqy1xu4iITV2QK611fHA=', '2025-09-05 22:03:49', 1, 'Chrome Browser / web', '58.11.85.73', '2025-08-06 22:03:49', '2025-08-07 01:15:50'),
(25, 8, 'refresh', 'ZkKXFYm2Ph-st0HezFOH1KSTZcx6nbVfv_y48-L2hM8=', '2025-09-06 01:15:59', 0, 'Chrome Browser / web', '58.11.85.73', '2025-08-07 01:15:59', '2025-08-07 01:15:59'),
(26, 8, 'refresh', 'yXI19rJ0peiTAJR3pObJ30SedqpOHQO41pOZ6hrrgpY=', '2025-09-06 13:33:42', 0, 'Chrome Browser / web', '58.11.85.73', '2025-08-07 13:33:42', '2025-08-07 13:33:42'),
(27, 7, 'refresh', 'NrQYwvk--sVk3NqWHMWDtrEqvpQHsxCsEMaHHdxY5o0=', '2025-09-06 16:49:03', 0, 'Chrome Browser / web', '58.11.85.73', '2025-08-07 16:49:03', '2025-08-07 16:49:03'),
(28, 7, 'refresh', '_pZeHz6qf7AzqYvrNjDue7n9biAUV_Vtl-_GxLp2xRE=', '2025-09-07 02:33:09', 0, 'Chrome Browser / web', '58.11.85.73', '2025-08-08 02:33:09', '2025-08-08 02:33:09'),
(29, 7, 'refresh', 'L_YZpdgZYWhwOSxNMP4AnpL-Rl_ENfZ0kKbZeHEtx1w=', '2025-09-07 14:21:38', 0, 'Chrome Browser / web', '58.11.85.73', '2025-08-08 14:21:38', '2025-08-08 14:21:38'),
(30, 7, 'refresh', 'hnsuVtjVScBDfg6wWB4PCWHXPS70IaJg7HKeCni3_Sk=', '2025-09-07 15:40:31', 0, 'Chrome Browser / web', '58.11.85.73', '2025-08-08 15:40:31', '2025-08-08 15:40:31'),
(31, 7, 'refresh', 'b6A0SuteEP1MdHwP9b5HxM8CVxxJFZ7J5Gg5jjrjR0k=', '2025-09-07 16:06:12', 0, ' / api_client', '110.168.238.46', '2025-08-08 16:06:12', '2025-08-08 16:06:12'),
(32, 7, 'refresh', 'psKmPkYcoYCCXd-fK7N8YfU5aWCQX7WbHg6j4lnNjWQ=', '2025-09-07 19:36:30', 0, 'Chrome Browser / web', '58.11.85.73', '2025-08-08 19:36:30', '2025-08-08 19:36:30'),
(33, 7, 'refresh', '26HwD33jydwP4ur37hPNWxwumGRhEcJR32h2eEk9Zs8=', '2025-09-07 21:51:58', 0, 'Chrome Browser / web', '58.11.85.73', '2025-08-08 21:51:58', '2025-08-08 21:51:58'),
(34, 7, 'refresh', 'gm21KeuRY_yYCchc6g2lqhLRUjGPRDi0gRuctmr0UpI=', '2025-09-09 12:34:52', 0, 'Chrome Browser / web', '58.10.128.241', '2025-08-10 12:34:52', '2025-08-10 12:34:52'),
(35, 7, 'refresh', 'zeOfHeY0_oCp1iHku1MP3ovzT1AdXMm6TQ3nCQ0zjCQ=', '2025-09-09 14:40:27', 0, 'Chrome Browser / web', '58.10.128.241', '2025-08-10 14:40:27', '2025-08-10 14:40:27'),
(36, 7, 'refresh', 'OaWrbI5YN_2ANsFDOFVNo7B6MvRWHanWWIzryWvSWNQ=', '2025-09-09 18:37:16', 0, 'Chrome Browser / web', '58.10.128.241', '2025-08-10 18:37:16', '2025-08-10 18:37:16'),
(37, 7, 'refresh', 'kbV58DjZt7Vv482MW9p7d0LHcuW-kfR8DGw1DA24iLs=', '2025-09-10 11:48:46', 1, 'Chrome Browser / web', '58.10.128.241', '2025-08-11 11:48:46', '2025-08-11 12:10:58'),
(38, 8, 'refresh', 'fwkRgmgs1wu4RCJ1V1j7JlB4PuHCSg6IhYLPoWG2pdc=', '2025-09-10 12:04:26', 0, 'Chrome Browser / web', '110.168.238.46', '2025-08-11 12:04:26', '2025-08-11 12:04:26'),
(39, 8, 'refresh', 'N7yzEXoJiIFLxDN55tbs6hxUGGh4FGOT6hXKAAt5Lfs=', '2025-09-10 12:11:04', 1, 'Chrome Browser / web', '58.10.128.241', '2025-08-11 12:11:04', '2025-08-11 12:14:58'),
(40, 7, 'refresh', 'TRaeEZ4VDx2V6gTLdTKTbd2YKXQFMqyN_tF-_3G5p7s=', '2025-09-10 12:15:03', 0, 'Chrome Browser / web', '58.10.128.241', '2025-08-11 12:15:03', '2025-08-11 12:15:03'),
(41, 7, 'refresh', 'iAjnkwYNartyyJUy1K3ExGqFcFABFexI1ZRlmISb51Q=', '2025-09-10 13:15:19', 0, 'Chrome Browser / web', '58.10.128.241', '2025-08-11 13:15:19', '2025-08-11 13:15:19'),
(42, 8, 'refresh', 'jkFrX8YMasUmiKJ07vtsjc1ASPL9qT0Fs6NYZjc3zLg=', '2025-09-10 13:35:18', 0, 'Chrome Browser / web', '110.168.238.46', '2025-08-11 13:35:18', '2025-08-11 13:35:18'),
(43, 8, 'refresh', 'VctAjiipLu81JtbhK5KH-uu1NnoWfjmUbEZ5FszLTBs=', '2025-09-10 20:11:29', 0, 'Chrome Browser / web', '110.168.238.46', '2025-08-11 20:11:29', '2025-08-11 20:11:29'),
(44, 8, 'refresh', 'IXFyHy46RSfjtGoy171aYO21NSHQQ6y1wMKWJJ54qCc=', '2025-09-11 05:08:41', 1, 'Chrome Browser / web', '58.11.84.239', '2025-08-12 05:08:41', '2025-08-12 05:09:35'),
(45, 7, 'refresh', 'FoOd1dkiuA3mpwzaKQncos7ktpidbmLEGhBx-24baNk=', '2025-09-11 05:09:39', 1, 'Chrome Browser / web', '58.11.84.239', '2025-08-12 05:09:39', '2025-08-12 05:35:26'),
(46, 7, 'refresh', 'bwg9ZafDSKoEZ9QxeIk-N_L-jk4ByrNM87Pwj1Wlb6s=', '2025-09-11 05:14:30', 1, 'Chrome Browser / web', '58.10.128.241', '2025-08-12 05:14:30', '2025-08-12 05:17:46'),
(47, 8, 'refresh', '2IoetuIMkYs-w7X_i5LOl3aS-13uiTRv3Wywwo-FahQ=', '2025-09-11 05:17:57', 0, 'Chrome Browser / web', '58.10.128.241', '2025-08-12 05:17:57', '2025-08-12 05:17:57'),
(48, 8, 'refresh', 'phBNr1qYw2w51aud1Z6tieJ92BZtQ0HrBA5eovKlwBM=', '2025-09-12 11:42:59', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-13 11:42:59', '2025-08-13 11:42:59'),
(49, 8, 'refresh', 'hsoMuTwW7o2ZBVsqx1OCapUBlv5xbWDry6BxNG_8gg0=', '2025-09-14 11:37:05', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-15 11:37:05', '2025-08-15 11:37:05'),
(50, 8, 'refresh', 'Gy-6dbq7KGtG-A8JVQMaASP52Dq28Te_66nnGMT4xt0=', '2025-09-14 17:44:19', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-15 17:44:19', '2025-08-15 17:44:19'),
(51, 8, 'refresh', 'h7PLs6dU27wd7R-mMClhhLJokiCSlOB8KFsXidb8eeo=', '2025-09-14 22:02:31', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-15 22:02:31', '2025-08-15 22:02:31'),
(52, 8, 'refresh', 'p7YYvscatu-GtkTDd2eOYgkyUyqY5klGCYx4Lf1oV_M=', '2025-09-15 11:22:23', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-16 11:22:23', '2025-08-16 11:22:23'),
(53, 8, 'refresh', 'iud6o_YnknREU2ZudtWtuwCv1rvNsRIYy3M0mesrh-U=', '2025-09-15 17:18:20', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-16 17:18:20', '2025-08-16 17:18:20'),
(54, 8, 'refresh', 'Od8MoBHOugpZzJcD9wT-v4TSaQsSbh8FEyZmQBea2TQ=', '2025-09-15 22:40:32', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-16 22:40:32', '2025-08-16 22:40:32'),
(55, 8, 'refresh', 'gfrLMUYBAKQqaIdFVDA6bnasF1bgrExuJHbNoWeOvGM=', '2025-09-16 08:56:04', 1, 'Chrome Browser / web', '58.11.84.239', '2025-08-17 08:56:04', '2025-08-17 09:00:57'),
(56, 7, 'refresh', '-uAin-AvWvdaZwBfLWHwD4_jFPwCpsbFpsiWpUuA5gg=', '2025-09-16 09:00:41', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-17 09:00:41', '2025-08-17 09:00:41'),
(57, 8, 'refresh', 'fZhVhKszDMkdl6Usyrw--x1ZHRAccYH4I81UyTxR_0g=', '2025-09-16 09:01:00', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-17 09:01:00', '2025-08-17 09:01:00'),
(58, 8, 'refresh', '8S8qQ126HW4pnZAE1PeAg-MyQKyqUlA_gAlWWP7N260=', '2025-09-16 19:19:04', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-17 19:19:04', '2025-08-17 19:19:04'),
(59, 8, 'refresh', 'dTUjHjPRWgTBuy4OqKzlDblDQYd8JlAZcH-q6LAtydc=', '2025-09-17 10:57:49', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-18 10:57:49', '2025-08-18 10:57:49'),
(60, 8, 'refresh', '2Ic2IZTbUCkIqWltTJ1EyIpsxrC4E8qqmrRNE-6UA2M=', '2025-09-17 11:27:11', 0, 'Chrome Browser / web', '58.11.84.239', '2025-08-18 11:27:11', '2025-08-18 11:27:11'),
(61, 8, 'refresh', 'byIbHv1zmuhDU0U81JxT0Can0kbqizmU8QJ2Wyjtdmc=', '2025-09-17 11:29:03', 0, 'Chrome Browser / web', '58.11.83.106', '2025-08-18 11:29:03', '2025-08-18 11:29:03'),
(62, 7, 'refresh', 'nqnDsdZMJ7ZFK4k2OESwtcoWyO5EhYLZr8dIw6lEg0c=', '2025-09-17 17:43:57', 0, 'Chrome Browser / web', '58.11.83.106', '2025-08-18 17:43:57', '2025-08-18 17:43:57'),
(63, 7, 'refresh', 'o4V4iixAUCe0rEssC12tVjkOpNoiIDxfwDNfwaMMtpk=', '2025-09-18 09:30:36', 1, 'Chrome Browser / web', '58.10.73.81', '2025-08-19 09:30:36', '2025-08-19 09:55:34'),
(64, 8, 'refresh', 'a6ukk8xJmp01v6JZKwlGSnp8GUxU-FssgXuEd5gyYm4=', '2025-09-18 09:51:20', 0, 'Chrome Browser / web', '58.10.73.81', '2025-08-19 09:51:20', '2025-08-19 09:51:20'),
(65, 8, 'refresh', 'YGoHFeH4JejOzsONRGI58j5lMLZjHAm2sif-6OAU53I=', '2025-09-18 09:55:41', 0, 'Chrome Browser / web', '58.10.73.81', '2025-08-19 09:55:41', '2025-08-19 09:55:41'),
(66, 8, 'refresh', 'h9ksWLBEnZyvmvVJh1yLPsvDqI8qR7ZFux4_0ISWvlM=', '2025-09-18 11:05:04', 0, 'Chrome Browser / web', '58.10.140.241', '2025-08-19 11:05:04', '2025-08-19 11:05:04'),
(67, 8, 'refresh', 'BU-NKZes65X9MYUXOLxTYNnWstt-B4yyTmztb_lPdGY=', '2025-09-18 15:38:50', 0, 'Chrome Browser / web', '58.10.140.241', '2025-08-19 15:38:50', '2025-08-19 15:38:50'),
(68, 8, 'refresh', 'fN8nsKW_54FeoM7ghqVhmAO9FLngqPdzzyOScTVJZSU=', '2025-09-18 17:47:44', 0, 'Chrome Browser / web', '58.10.140.241', '2025-08-19 17:47:44', '2025-08-19 17:47:44'),
(69, 8, 'refresh', '8d10y94bqHEeVWAEscyfGc6kIR5aXEHziwL8UiJDpVg=', '2025-09-19 13:28:07', 0, 'Chrome Browser / web', '58.10.140.241', '2025-08-20 13:28:07', '2025-08-20 13:28:07'),
(70, 8, 'refresh', 'fHp_ODwVbl1puidJ_v0VDB-tJrVl6KaksA3TISMDd_o=', '2025-09-19 14:15:40', 1, 'Chrome Browser / web', '58.11.79.179', '2025-08-20 14:15:40', '2025-08-21 05:56:31'),
(71, 8, 'refresh', 'd4Yb2tGiqcnC3zXFPmncroSF_fZ9LPv-lLL4-ko2zY4=', '2025-09-19 16:25:09', 0, 'Chrome Browser / web', '58.10.140.241', '2025-08-20 16:25:09', '2025-08-20 16:25:09'),
(72, 7, 'refresh', 'sYtEOv2COuoDn6tdgvAqaTtX1-nSfdDfQ33cNj5vh9A=', '2025-09-20 05:56:38', 0, 'Chrome Browser / web', '58.11.79.179', '2025-08-21 05:56:38', '2025-08-21 05:56:38'),
(73, 7, 'refresh', '4_Ek8UW6Ihk48NSHxfRlWJFLyiySlhAY2U0m32DcKFg=', '2025-09-20 14:50:16', 0, 'Chrome Browser / web', '58.10.71.142', '2025-08-21 14:50:16', '2025-08-21 14:50:16'),
(74, 8, 'refresh', 'XG3M7Fj4OLIXtEUIPdvBjqbQr89169Q1Jp2heETdIJ4=', '2025-09-21 03:18:03', 1, 'Chrome Browser / web', '58.10.71.142', '2025-08-22 03:18:03', '2025-08-22 03:27:17'),
(75, 7, 'refresh', 'E_geKerdRbB64ATv30Eag_ggWH2dcao4M6lMFyvdhQw=', '2025-09-21 03:27:27', 1, 'Chrome Browser / web', '58.10.71.142', '2025-08-22 03:27:27', '2025-08-22 04:07:07'),
(76, 8, 'refresh', 'HqeNHEtTCvutm1rftYO74evxkhx0PGcqmxeIHO5OF1k=', '2025-09-21 04:07:13', 1, 'Chrome Browser / web', '58.10.71.142', '2025-08-22 04:07:13', '2025-08-22 10:28:37'),
(77, 8, 'refresh', 'LR_aUatosWPXIsflxe6YuTBdXj1Keah2DXQDE1BIzQc=', '2025-09-21 09:31:16', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 09:31:16', '2025-08-22 11:05:38'),
(78, 7, 'refresh', 'dT9QFYhyL-L9aKBHqZ6XK4ZgJFyxacHLY-ZsV5753MU=', '2025-09-21 10:28:45', 1, 'Chrome Browser / web', '27.145.211.127', '2025-08-22 10:28:45', '2025-08-22 10:29:02'),
(79, 8, 'refresh', 'WcLaSyCy1ecuLK0hdZdmZh2kbSSmU5V830Pc-k_2Tzc=', '2025-09-21 10:29:12', 0, 'Chrome Browser / web', '27.145.211.127', '2025-08-22 10:29:12', '2025-08-22 10:29:12'),
(80, 8, 'refresh', '0Aeu-wqFq8zOAFN6ZakwvLoI30X3SRTNkYuPmjQjcmc=', '2025-09-21 11:06:16', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 11:06:16', '2025-08-22 11:09:02'),
(81, 7, 'refresh', '-mJvRL5XXqcx_WUBXWKXX2Fbtu8Eww3-jcjGeYbP4Mk=', '2025-09-21 11:09:17', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 11:09:17', '2025-08-22 12:17:02'),
(82, 8, 'refresh', '66APBp0GokYRrZ8hkfIsE4LtT-jhx1b89HEBucYKSpw=', '2025-09-21 11:20:18', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 11:20:18', '2025-08-22 11:20:18'),
(83, 8, 'refresh', 'rrBY5pqtfMOFtSJGHCykffh0dX46U2_S_U_V00ofq3Q=', '2025-09-21 11:47:35', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 11:47:35', '2025-08-22 11:47:35'),
(84, 8, 'refresh', 'xNxLws0kgG7T-slD4d1fseGlgsW1vrOYRiFoT3gWk2I=', '2025-09-21 12:17:05', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 12:17:05', '2025-08-22 12:17:05'),
(85, 8, 'refresh', 'uJ_EgTBO1Q3gQKDwC2p4zYf-AkMErbhxnUJ5ICK5A98=', '2025-09-21 13:05:10', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 13:05:10', '2025-08-22 13:05:20'),
(86, 7, 'refresh', 'UVL5pIjTze90_fgO4SLD3OmuIT1hdM3Zf7HlOOy8b8Q=', '2025-09-21 13:05:23', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 13:05:23', '2025-08-22 13:05:23'),
(87, 8, 'refresh', 'XkpusDTD5o2Obeencyn70Ib34pEIDj53RTolijWZItw=', '2025-09-21 13:17:27', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 13:17:27', '2025-08-22 13:17:27'),
(88, 8, 'refresh', 'FHjPT1446XEQDS6pBXTHbEdSfZ4iJcD7H0MQnG-yuwk=', '2025-09-21 13:25:47', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 13:25:47', '2025-08-22 13:25:47'),
(89, 7, 'refresh', 'aFyhanlOuCbRMIF4ZkLNuW-ddY0cL_NMrQP5kC0AOWA=', '2025-09-21 13:27:03', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 13:27:03', '2025-08-22 13:27:03'),
(90, 8, 'refresh', '9bgN593g-ZNPFLcmRh-czV5VeEVr4aK94uV5HiLE_WE=', '2025-09-21 14:34:30', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 14:34:30', '2025-08-22 14:34:30'),
(91, 8, 'refresh', 'FpFNDIDgM_k65aEgFlAAkGB8ZiRvtTfJFmhK4Ar8ywQ=', '2025-09-21 15:31:15', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 15:31:15', '2025-08-22 15:31:15'),
(92, 8, 'refresh', 'C3oPsD_qT2Q187NPxFYPDZJElc8dP-1CJJRGz07osBI=', '2025-09-21 15:58:26', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-22 15:58:26', '2025-08-22 15:58:26'),
(93, 8, 'refresh', 'sMPg7MWAFCw4Q823S7aFmj_SrxRtcJelLJ0XCJc-JJo=', '2025-09-21 18:41:48', 0, 'Chrome Browser / web', '27.145.211.127', '2025-08-22 18:41:48', '2025-08-22 18:41:48'),
(94, 7, 'refresh', 'TqrEznLz2oAkOmqTB5U_PkL6I4xtJtxqZPEcQmsiRTw=', '2025-09-21 19:02:25', 0, ' / api_client', '58.10.155.36', '2025-08-22 19:02:25', '2025-08-22 19:02:25'),
(95, 8, 'refresh', 'VlZSRTAwBBcMY7SNdXOz984v9wgi7JXS6DUUiBxcMK0=', '2025-09-21 19:02:47', 0, ' / api_client', '58.10.155.36', '2025-08-22 19:02:47', '2025-08-22 19:02:47'),
(96, 8, 'refresh', 'ZSX5cbHwrJfLPE35ffalDyusEYG58NGf2SM5Gz6t_q8=', '2025-09-21 22:57:56', 0, 'Chrome Browser / web', '27.145.211.127', '2025-08-22 22:57:56', '2025-08-22 22:57:56'),
(97, 7, 'refresh', 'iOPDusVzQkASkmRVmHn3FbFqu_Azc95PCYgbRDbb4G8=', '2025-09-22 04:58:44', 0, 'Chrome Browser / web', '27.145.211.127', '2025-08-23 04:58:44', '2025-08-23 04:58:44'),
(98, 7, 'refresh', 'zQ-hmQBc0_f386Au_V8SPvNjBThy7A0ByqryiUCipxI=', '2025-09-22 07:26:49', 1, 'Chrome Browser / web', '27.145.211.127', '2025-08-23 07:26:49', '2025-08-23 07:26:58'),
(99, 8, 'refresh', '5au8V4OH8SRnAWT7cByqmrNzP1TXdZo3naiGQfPQVR8=', '2025-09-22 07:27:11', 1, 'Chrome Browser / web', '27.145.211.127', '2025-08-23 07:27:11', '2025-08-23 07:28:43'),
(100, 7, 'refresh', 'WIKJVQLIEE5lTprk9onoWOrfhOHiendslI4AO1aR5T8=', '2025-09-22 07:28:49', 1, 'Chrome Browser / web', '27.145.211.127', '2025-08-23 07:28:49', '2025-08-23 09:03:07'),
(101, 8, 'refresh', 'LARTpQPggSvLyLO63aPULXfy1nu9SjA_tMgHi0bzsdg=', '2025-09-22 08:21:13', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 08:21:13', '2025-08-23 09:12:07'),
(102, 7, 'refresh', '4PZdZQOPy_4tDCA6gh2YfYM_VqXuJs-mp31A4gk4rGw=', '2025-09-22 09:03:22', 1, 'Chrome Browser / web', '27.145.211.127', '2025-08-23 09:03:22', '2025-08-23 09:07:37'),
(103, 7, 'refresh', 'utDCbBJwLnPLlqYgHan1welntpvzkqozbHFBErQRhfQ=', '2025-09-22 09:03:37', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 09:03:37', '2025-08-23 09:11:01'),
(104, 7, 'refresh', 'a1fxhAFIJ_ztw67g8Q4womW5AHFV6pxcMXDXUDOFDeo=', '2025-09-22 09:07:44', 0, 'Chrome Browser / web', '27.145.211.127', '2025-08-23 09:07:44', '2025-08-23 09:07:44'),
(105, 8, 'refresh', 'AFQJeaenmNbz2DbYtpEe_zcnGIZo21zoJGbz63CbMTc=', '2025-09-22 09:11:12', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 09:11:12', '2025-08-23 10:21:06'),
(106, 7, 'refresh', 'JjUFYtBYGfSYTY5II7vHUznwSm1feamoonLkDekf65c=', '2025-09-22 09:12:10', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 09:12:10', '2025-08-23 11:04:39'),
(107, 7, 'refresh', '-G9pid_z8ZEtLW30FKhpdW2ru1IGp-_L_gXchk2zkno=', '2025-09-22 10:20:23', 0, 'Chrome Browser / web', '27.145.211.127', '2025-08-23 10:20:23', '2025-08-23 10:20:23'),
(108, 8, 'refresh', 'MFKgUR2MvXXt6vpkWo7De02HClNBnQwzaJtWTksjLjI=', '2025-09-22 10:22:44', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 10:22:44', '2025-08-23 10:25:31'),
(109, 7, 'refresh', 'n0FMnXHWB9mDpqPVS_rMxm_CgTCjptc9jy04YmsZNY4=', '2025-09-22 10:25:33', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 10:25:33', '2025-08-23 11:03:40'),
(110, 8, 'refresh', 'jvRtFXk7d6AAuwW9xqBR8TM7INEmnpKoHJjzaAUrD4w=', '2025-09-22 11:04:41', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 11:04:41', '2025-08-23 11:10:19'),
(111, 7, 'refresh', 'Lbc_8wSXjHSlZQc_F4ifBqpYtQ676HblpBYn5yGimGo=', '2025-09-22 11:05:57', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 11:05:57', '2025-08-23 11:05:57'),
(112, 7, 'refresh', '1sT50x0OUVOwZBH6e2N3Td4EjXDXjlTEyckkctP3g18=', '2025-09-22 11:10:22', 1, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 11:10:22', '2025-08-23 11:10:38'),
(113, 8, 'refresh', 'V6Ix5RsnPo3LMKGeaPT7gh7Dj4T0qaVdPEfZseUxoAw=', '2025-09-22 11:10:42', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 11:10:42', '2025-08-23 11:10:42'),
(114, 8, 'refresh', 'eYHHm2B6wMNSHi-Usx-QtdyO9SrFNt2UxUphwbVcDZ4=', '2025-09-22 11:19:55', 0, 'Chrome Browser / web', '58.10.155.36', '2025-08-23 11:19:55', '2025-08-23 11:19:55');

-- --------------------------------------------------------

--
-- Stand-in structure for view `view_budget_summary`
-- (See below for the actual view)
--
CREATE TABLE `view_budget_summary` (
`year` varchar(255)
,`category_name` varchar(255)
,`subcategory_name` varchar(255)
,`allocated_amount` decimal(15,2)
,`used_amount` decimal(15,2)
,`remaining_budget` decimal(15,2)
,`max_grants` int(11)
,`remaining_grant` int(11)
,`total_applications` bigint(21)
,`approved_applications` bigint(21)
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `view_fund_applications_summary`
-- (See below for the actual view)
--
CREATE TABLE `view_fund_applications_summary` (
`application_id` int(11)
,`application_number` varchar(255)
,`project_title` varchar(255)
,`applicant_name` varchar(511)
,`email` varchar(255)
,`position_name` varchar(255)
,`category_name` varchar(255)
,`subcategory_name` varchar(255)
,`year` varchar(255)
,`status_name` varchar(255)
,`requested_amount` decimal(15,2)
,`approved_amount` decimal(15,2)
,`submitted_at` datetime
,`approved_at` datetime
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_active_reward_config`
-- (See below for the actual view)
--
CREATE TABLE `v_active_reward_config` (
`config_id` int(11)
,`year` varchar(4)
,`journal_quartile` enum('Q1','Q2','Q3','Q4','T5','T10','TCI','N/A')
,`max_amount` decimal(15,2)
,`condition_description` text
,`create_at` datetime
,`update_at` datetime
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_announcements_readable`
-- (See below for the actual view)
--
CREATE TABLE `v_announcements_readable` (
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_file_uploads_readable`
-- (See below for the actual view)
--
CREATE TABLE `v_file_uploads_readable` (
`file_id` int(11)
,`original_name` varchar(255)
,`stored_path` varchar(500)
,`folder_type` enum('temp','submission','profile','other')
,`submission_id` int(11)
,`file_size` bigint(20)
,`mime_type` varchar(100)
,`file_hash` varchar(64)
,`is_public` tinyint(1)
,`uploaded_by` int(11)
,`uploaded_at` datetime
,`create_at` datetime
,`update_at` datetime
,`delete_at` datetime
,`user_fname` varchar(255)
,`user_lname` varchar(255)
,`uploader_name` varchar(511)
,`user_folder` varchar(500)
,`folder_type_name` varchar(16)
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_file_usage_stats`
-- (See below for the actual view)
--
CREATE TABLE `v_file_usage_stats` (
`user_id` int(11)
,`user_name` varchar(511)
,`email` varchar(255)
,`total_files` bigint(21)
,`total_size` decimal(41,0)
,`avg_file_size` decimal(23,4)
,`temp_files` bigint(21)
,`submission_files` bigint(21)
,`profile_files` bigint(21)
,`last_upload` datetime
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_fund_applications`
-- (See below for the actual view)
--
CREATE TABLE `v_fund_applications` (
`application_id` int(11)
,`application_number` varchar(255)
,`user_id` int(11)
,`year_id` int(11)
,`subcategory_id` int(11)
,`application_status_id` int(11)
,`approved_by` int(11)
,`project_title` varchar(255)
,`project_description` text
,`requested_amount` decimal(15,2)
,`approved_amount` decimal(15,2)
,`submitted_at` datetime
,`approved_at` datetime
,`closed_at` datetime
,`comment` text
,`create_at` datetime
,`update_at` datetime
,`delete_at` datetime
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_fund_forms_readable`
-- (See below for the actual view)
--
CREATE TABLE `v_fund_forms_readable` (
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_publication_rewards`
-- (See below for the actual view)
--
CREATE TABLE `v_publication_rewards` (
`reward_id` int(11)
,`reward_number` varchar(255)
,`user_id` int(11)
,`paper_title` varchar(500)
,`journal_name` varchar(255)
,`publication_date` date
,`journal_quartile` enum('Q1','Q2','Q3','Q4','T5','T10','TCI','N/A')
,`doi` varchar(255)
,`reward_amount` decimal(15,2)
,`status_id` int(11)
,`submitted_at` datetime
,`created_at` datetime
,`updated_at` datetime
,`deleted_at` datetime
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_recent_audit_logs`
-- (See below for the actual view)
--
CREATE TABLE `v_recent_audit_logs` (
`log_id` int(11)
,`created_at` datetime
,`user_name` varchar(511)
,`action` enum('create','update','delete','login','logout','view','download','approve','reject','submit','review')
,`entity_type` varchar(50)
,`entity_number` varchar(50)
,`description` text
,`ip_address` varchar(45)
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_submission_audit_trail`
-- (See below for the actual view)
--
CREATE TABLE `v_submission_audit_trail` (
`submission_number` varchar(255)
,`submission_type` enum('fund_application','publication_reward')
,`created_at` datetime
,`action_by` varchar(511)
,`action` enum('create','update','delete','login','logout','view','download','approve','reject','submit','review')
,`changed_fields` text
,`description` text
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_user_activity_summary`
-- (See below for the actual view)
--
CREATE TABLE `v_user_activity_summary` (
`user_id` int(11)
,`user_name` varchar(511)
,`login_count` bigint(21)
,`create_count` bigint(21)
,`update_count` bigint(21)
,`download_count` bigint(21)
,`last_login` datetime /* mariadb-5.3 */
,`total_actions` bigint(21)
);

-- --------------------------------------------------------

--
-- Table structure for table `years`
--

CREATE TABLE `years` (
  `year_id` int(11) NOT NULL,
  `year` varchar(255) DEFAULT NULL,
  `budget` decimal(15,2) DEFAULT NULL,
  `status` enum('active','inactive') DEFAULT 'active',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

--
-- Dumping data for table `years`
--

INSERT INTO `years` (`year_id`, `year`, `budget`, `status`, `create_at`, `update_at`, `delete_at`) VALUES
(1, '2566', '1000000.00', 'active', '2025-06-24 16:49:13', '2025-07-23 10:18:36', NULL),
(2, '2567', '1500000.00', 'active', '2025-06-24 16:49:13', '2025-08-08 21:22:24', NULL),
(3, '2568', '2000000.00', 'active', '2025-07-08 10:44:10', '2025-08-10 19:49:28', NULL);

-- --------------------------------------------------------

--
-- Structure for view `view_budget_summary`
--
DROP TABLE IF EXISTS `view_budget_summary`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `view_budget_summary`  AS  select `y`.`year` AS `year`,`fc`.`category_name` AS `category_name`,`fs`.`subcategory_name` AS `subcategory_name`,`sb`.`allocated_amount` AS `allocated_amount`,`sb`.`used_amount` AS `used_amount`,`sb`.`remaining_budget` AS `remaining_budget`,`sb`.`max_grants` AS `max_grants`,`sb`.`remaining_grant` AS `remaining_grant`,count(`fa`.`application_id`) AS `total_applications`,count(case when `fa`.`application_status_id` = 2 then 1 end) AS `approved_applications` from ((((`subcategory_budgets` `sb` left join `fund_subcategories` `fs` on(`sb`.`subcategory_id` = `fs`.`subcategory_id`)) left join `fund_categories` `fc` on(`fs`.`category_id` = `fc`.`category_id`)) left join `years` `y` on(`fc`.`year_id` = `y`.`year_id`)) left join `v_fund_applications` `fa` on(`fs`.`subcategory_id` = `fa`.`subcategory_id` and `fa`.`delete_at` is null)) where `sb`.`delete_at` is null group by `sb`.`subcategory_budget_id`,`y`.`year`,`fc`.`category_name`,`fs`.`subcategory_name`,`sb`.`allocated_amount`,`sb`.`used_amount`,`sb`.`remaining_budget`,`sb`.`max_grants`,`sb`.`remaining_grant` ;

-- --------------------------------------------------------

--
-- Structure for view `view_fund_applications_summary`
--
DROP TABLE IF EXISTS `view_fund_applications_summary`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `view_fund_applications_summary`  AS  select `fa`.`application_id` AS `application_id`,`fa`.`application_number` AS `application_number`,`fa`.`project_title` AS `project_title`,concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `applicant_name`,`u`.`email` AS `email`,`p`.`position_name` AS `position_name`,`fc`.`category_name` AS `category_name`,`fs`.`subcategory_name` AS `subcategory_name`,`y`.`year` AS `year`,`ast`.`status_name` AS `status_name`,`fa`.`requested_amount` AS `requested_amount`,`fa`.`approved_amount` AS `approved_amount`,`fa`.`submitted_at` AS `submitted_at`,`fa`.`approved_at` AS `approved_at` from ((((((`v_fund_applications` `fa` left join `users` `u` on(`fa`.`user_id` = `u`.`user_id`)) left join `positions` `p` on(`u`.`position_id` = `p`.`position_id`)) left join `fund_subcategories` `fs` on(`fa`.`subcategory_id` = `fs`.`subcategory_id`)) left join `fund_categories` `fc` on(`fs`.`category_id` = `fc`.`category_id`)) left join `years` `y` on(`fa`.`year_id` = `y`.`year_id`)) left join `application_status` `ast` on(`fa`.`application_status_id` = `ast`.`application_status_id`)) where `fa`.`delete_at` is null ;

-- --------------------------------------------------------

--
-- Structure for view `v_active_reward_config`
--
DROP TABLE IF EXISTS `v_active_reward_config`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_active_reward_config`  AS  select `reward_config`.`config_id` AS `config_id`,`reward_config`.`year` AS `year`,`reward_config`.`journal_quartile` AS `journal_quartile`,`reward_config`.`max_amount` AS `max_amount`,`reward_config`.`condition_description` AS `condition_description`,`reward_config`.`create_at` AS `create_at`,`reward_config`.`update_at` AS `update_at` from `reward_config` where `reward_config`.`is_active` = 1 and `reward_config`.`delete_at` is null order by `reward_config`.`year` desc,`reward_config`.`journal_quartile` ;

-- --------------------------------------------------------

--
-- Structure for view `v_announcements_readable`
--
DROP TABLE IF EXISTS `v_announcements_readable`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_announcements_readable`  AS  select `a`.`announcement_id` AS `announcement_id`,`a`.`title` AS `title`,`a`.`description` AS `description`,`a`.`file_name` AS `file_name`,`a`.`file_path` AS `file_path`,`a`.`file_size` AS `file_size`,`a`.`mime_type` AS `mime_type`,`a`.`announcement_type` AS `announcement_type`,`a`.`priority` AS `priority`,`a`.`status` AS `status`,`a`.`published_at` AS `published_at`,`a`.`expired_at` AS `expired_at`,`a`.`view_count` AS `view_count`,`a`.`download_count` AS `download_count`,`a`.`created_by` AS `created_by`,`a`.`create_at` AS `create_at`,`a`.`update_at` AS `update_at`,`a`.`delete_at` AS `delete_at`,concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `creator_name`,`u`.`email` AS `creator_email`,case when `a`.`announcement_type` = 'general' then 'ประกาศทั่วไป' when `a`.`announcement_type` = 'research_fund' then 'ทุนส่งเสริมการวิจัย' when `a`.`announcement_type` = 'promotion_fund' then 'ทุนอุดหนุนกิจกรรม' else `a`.`announcement_type` end AS `announcement_type_name`,case when `a`.`priority` = 'normal' then 'ปกติ' when `a`.`priority` = 'high' then 'สำคัญ' when `a`.`priority` = 'urgent' then 'ด่วน' else `a`.`priority` end AS `priority_name`,case when `a`.`status` = 'active' then 'เปิดใช้งาน' when `a`.`status` = 'inactive' then 'ปิดใช้งาน' else `a`.`status` end AS `status_name`,case when `a`.`expired_at` is null then 0 when `a`.`expired_at` < current_timestamp() then 1 else 0 end AS `is_expired` from (`announcements` `a` left join `users` `u` on(`a`.`created_by` = `u`.`user_id`)) where `a`.`delete_at` is null ;

-- --------------------------------------------------------

--
-- Structure for view `v_file_uploads_readable`
--
DROP TABLE IF EXISTS `v_file_uploads_readable`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_file_uploads_readable`  AS  select `f`.`file_id` AS `file_id`,`f`.`original_name` AS `original_name`,`f`.`stored_path` AS `stored_path`,`f`.`folder_type` AS `folder_type`,`f`.`submission_id` AS `submission_id`,`f`.`file_size` AS `file_size`,`f`.`mime_type` AS `mime_type`,`f`.`file_hash` AS `file_hash`,`f`.`is_public` AS `is_public`,`f`.`uploaded_by` AS `uploaded_by`,`f`.`uploaded_at` AS `uploaded_at`,`f`.`create_at` AS `create_at`,`f`.`update_at` AS `update_at`,`f`.`delete_at` AS `delete_at`,`u`.`user_fname` AS `user_fname`,`u`.`user_lname` AS `user_lname`,concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `uploader_name`,case when `f`.`stored_path` like '%/users/%' then substring_index(substring_index(`f`.`stored_path`,'/users/',-1),'/',1) else 'unknown' end AS `user_folder`,case `f`.`folder_type` when 'temp' then 'Temporary Files' when 'submission' then 'Submission Files' when 'profile' then 'Profile Files' else 'Other Files' end AS `folder_type_name` from (`file_uploads` `f` left join `users` `u` on(`f`.`uploaded_by` = `u`.`user_id`)) where `f`.`delete_at` is null ;

-- --------------------------------------------------------

--
-- Structure for view `v_file_usage_stats`
--
DROP TABLE IF EXISTS `v_file_usage_stats`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_file_usage_stats`  AS  select `u`.`user_id` AS `user_id`,concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `user_name`,`u`.`email` AS `email`,count(`f`.`file_id`) AS `total_files`,sum(`f`.`file_size`) AS `total_size`,avg(`f`.`file_size`) AS `avg_file_size`,count(case when `f`.`folder_type` = 'temp' then 1 end) AS `temp_files`,count(case when `f`.`folder_type` = 'submission' then 1 end) AS `submission_files`,count(case when `f`.`folder_type` = 'profile' then 1 end) AS `profile_files`,max(`f`.`uploaded_at`) AS `last_upload` from (`users` `u` left join `file_uploads` `f` on(`u`.`user_id` = `f`.`uploaded_by` and `f`.`delete_at` is null)) where `u`.`delete_at` is null group by `u`.`user_id`,`u`.`user_fname`,`u`.`user_lname`,`u`.`email` order by count(`f`.`file_id`) desc ;

-- --------------------------------------------------------

--
-- Structure for view `v_fund_applications`
--
DROP TABLE IF EXISTS `v_fund_applications`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_fund_applications`  AS  select `s`.`submission_id` AS `application_id`,`s`.`submission_number` AS `application_number`,`s`.`user_id` AS `user_id`,`s`.`year_id` AS `year_id`,`fad`.`subcategory_id` AS `subcategory_id`,`s`.`status_id` AS `application_status_id`,`s`.`approved_by` AS `approved_by`,`fad`.`project_title` AS `project_title`,`fad`.`project_description` AS `project_description`,`fad`.`requested_amount` AS `requested_amount`,`fad`.`approved_amount` AS `approved_amount`,`s`.`submitted_at` AS `submitted_at`,`s`.`approved_at` AS `approved_at`,`fad`.`closed_at` AS `closed_at`,`fad`.`comment` AS `comment`,`s`.`created_at` AS `create_at`,`s`.`updated_at` AS `update_at`,`s`.`deleted_at` AS `delete_at` from (`submissions` `s` join `fund_application_details` `fad` on(`s`.`submission_id` = `fad`.`submission_id`)) where `s`.`submission_type` = 'fund_application' ;

-- --------------------------------------------------------

--
-- Structure for view `v_fund_forms_readable`
--
DROP TABLE IF EXISTS `v_fund_forms_readable`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_fund_forms_readable`  AS  select `f`.`form_id` AS `form_id`,`f`.`title` AS `title`,`f`.`description` AS `description`,`f`.`file_name` AS `file_name`,`f`.`file_path` AS `file_path`,`f`.`file_size` AS `file_size`,`f`.`mime_type` AS `mime_type`,`f`.`form_type` AS `form_type`,`f`.`fund_category` AS `fund_category`,`f`.`version` AS `version`,`f`.`is_required` AS `is_required`,`f`.`status` AS `status`,`f`.`effective_date` AS `effective_date`,`f`.`expiry_date` AS `expiry_date`,`f`.`download_count` AS `download_count`,`f`.`created_by` AS `created_by`,`f`.`create_at` AS `create_at`,`f`.`update_at` AS `update_at`,`f`.`delete_at` AS `delete_at`,concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `creator_name`,`u`.`email` AS `creator_email`,case when `f`.`form_type` = 'application' then 'แบบฟอร์มสมัคร' when `f`.`form_type` = 'report' then 'แบบฟอร์มรายงาน' when `f`.`form_type` = 'evaluation' then 'แบบฟอร์มประเมิน' when `f`.`form_type` = 'guidelines' then 'แนวทางปฏิบัติ' when `f`.`form_type` = 'other' then 'อื่นๆ' else `f`.`form_type` end AS `form_type_name`,case when `f`.`fund_category` = 'research_fund' then 'ทุนส่งเสริมการวิจัย' when `f`.`fund_category` = 'promotion_fund' then 'ทุนอุดหนุนกิจกรรม' when `f`.`fund_category` = 'both' then 'ทั้งสองประเภท' else `f`.`fund_category` end AS `fund_category_name`,case when `f`.`status` = 'active' then 'เปิดใช้งาน' when `f`.`status` = 'inactive' then 'ปิดใช้งาน' when `f`.`status` = 'archived' then 'เก็บถาวร' else `f`.`status` end AS `status_name`,case when `f`.`expiry_date` is null then 0 when `f`.`expiry_date` < current_timestamp() then 1 else 0 end AS `is_expired` from (`fund_forms` `f` left join `users` `u` on(`f`.`created_by` = `u`.`user_id`)) where `f`.`delete_at` is null ;

-- --------------------------------------------------------

--
-- Structure for view `v_publication_rewards`
--
DROP TABLE IF EXISTS `v_publication_rewards`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_publication_rewards`  AS  select `s`.`submission_id` AS `reward_id`,`s`.`submission_number` AS `reward_number`,`s`.`user_id` AS `user_id`,`prd`.`paper_title` AS `paper_title`,`prd`.`journal_name` AS `journal_name`,`prd`.`publication_date` AS `publication_date`,`prd`.`quartile` AS `journal_quartile`,`prd`.`doi` AS `doi`,`prd`.`reward_amount` AS `reward_amount`,`s`.`status_id` AS `status_id`,`s`.`submitted_at` AS `submitted_at`,`s`.`created_at` AS `created_at`,`s`.`updated_at` AS `updated_at`,`s`.`deleted_at` AS `deleted_at` from (`submissions` `s` join `publication_reward_details` `prd` on(`s`.`submission_id` = `prd`.`submission_id`)) where `s`.`submission_type` = 'publication_reward' ;

-- --------------------------------------------------------

--
-- Structure for view `v_recent_audit_logs`
--
DROP TABLE IF EXISTS `v_recent_audit_logs`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_recent_audit_logs`  AS  select `al`.`log_id` AS `log_id`,`al`.`created_at` AS `created_at`,concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `user_name`,`al`.`action` AS `action`,`al`.`entity_type` AS `entity_type`,`al`.`entity_number` AS `entity_number`,`al`.`description` AS `description`,`al`.`ip_address` AS `ip_address` from (`audit_logs` `al` left join `users` `u` on(`al`.`user_id` = `u`.`user_id`)) order by `al`.`created_at` desc limit 100 ;

-- --------------------------------------------------------

--
-- Structure for view `v_submission_audit_trail`
--
DROP TABLE IF EXISTS `v_submission_audit_trail`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_submission_audit_trail`  AS  select `s`.`submission_number` AS `submission_number`,`s`.`submission_type` AS `submission_type`,`al`.`created_at` AS `created_at`,concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `action_by`,`al`.`action` AS `action`,`al`.`changed_fields` AS `changed_fields`,`al`.`description` AS `description` from ((`submissions` `s` join `audit_logs` `al` on(`al`.`entity_type` = 'submission' and `al`.`entity_id` = `s`.`submission_id`)) left join `users` `u` on(`al`.`user_id` = `u`.`user_id`)) order by `s`.`submission_id`,`al`.`created_at` ;

-- --------------------------------------------------------

--
-- Structure for view `v_user_activity_summary`
--
DROP TABLE IF EXISTS `v_user_activity_summary`;

CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_user_activity_summary`  AS  select `u`.`user_id` AS `user_id`,concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `user_name`,count(case when `al`.`action` = 'login' then 1 end) AS `login_count`,count(case when `al`.`action` = 'create' then 1 end) AS `create_count`,count(case when `al`.`action` = 'update' then 1 end) AS `update_count`,count(case when `al`.`action` = 'download' then 1 end) AS `download_count`,max(case when `al`.`action` = 'login' then `al`.`created_at` end) AS `last_login`,count(0) AS `total_actions` from (`users` `u` left join `audit_logs` `al` on(`u`.`user_id` = `al`.`user_id`)) group by `u`.`user_id` ;

--
-- Indexes for dumped tables
--

--
-- Indexes for table `announcements`
--
ALTER TABLE `announcements`
  ADD PRIMARY KEY (`announcement_id`),
  ADD KEY `idx_announcement_type` (`announcement_type`),
  ADD KEY `idx_status` (`status`),
  ADD KEY `idx_priority` (`priority`),
  ADD KEY `idx_published_at` (`published_at`),
  ADD KEY `idx_expired_at` (`expired_at`),
  ADD KEY `idx_created_by` (`created_by`),
  ADD KEY `idx_delete_at` (`delete_at`),
  ADD KEY `idx_announcements_type_status_published` (`announcement_type`,`status`,`published_at`),
  ADD KEY `idx_announcements_status_priority_published` (`status`,`priority`,`published_at`);

--
-- Indexes for table `application_status`
--
ALTER TABLE `application_status`
  ADD PRIMARY KEY (`application_status_id`);

--
-- Indexes for table `audit_logs`
--
ALTER TABLE `audit_logs`
  ADD PRIMARY KEY (`log_id`),
  ADD KEY `idx_user` (`user_id`),
  ADD KEY `idx_entity` (`entity_type`,`entity_id`),
  ADD KEY `idx_action` (`action`),
  ADD KEY `idx_created` (`created_at`),
  ADD KEY `idx_entity_number` (`entity_number`);

--
-- Indexes for table `document_types`
--
ALTER TABLE `document_types`
  ADD PRIMARY KEY (`document_type_id`);

--
-- Indexes for table `file_uploads`
--
ALTER TABLE `file_uploads`
  ADD PRIMARY KEY (`file_id`),
  ADD KEY `idx_uploaded_by` (`uploaded_by`),
  ADD KEY `idx_uploaded_at` (`uploaded_at`),
  ADD KEY `idx_mime_type` (`mime_type`),
  ADD KEY `idx_file_hash` (`file_hash`),
  ADD KEY `idx_file_uploads_hash` (`file_hash`),
  ADD KEY `idx_file_uploads_user_path` (`uploaded_by`,`stored_path`),
  ADD KEY `idx_file_uploads_original_name` (`original_name`),
  ADD KEY `idx_file_uploads_active` (`delete_at`,`uploaded_by`),
  ADD KEY `idx_file_uploads_uploaded_date` (`uploaded_at`,`uploaded_by`);

--
-- Indexes for table `fund_application_details`
--
ALTER TABLE `fund_application_details`
  ADD PRIMARY KEY (`detail_id`),
  ADD UNIQUE KEY `submission_id` (`submission_id`),
  ADD UNIQUE KEY `idx_submission` (`submission_id`),
  ADD KEY `idx_subcategory` (`subcategory_id`),
  ADD KEY `idx_approved_by` (`approved_by`),
  ADD KEY `idx_rejected_by` (`rejected_by`);

--
-- Indexes for table `fund_categories`
--
ALTER TABLE `fund_categories`
  ADD PRIMARY KEY (`category_id`),
  ADD KEY `year_id` (`year_id`);

--
-- Indexes for table `fund_forms`
--
ALTER TABLE `fund_forms`
  ADD PRIMARY KEY (`form_id`),
  ADD KEY `idx_form_type` (`form_type`),
  ADD KEY `idx_fund_category` (`fund_category`),
  ADD KEY `idx_status` (`status`),
  ADD KEY `idx_created_by` (`created_by`),
  ADD KEY `idx_delete_at` (`delete_at`),
  ADD KEY `idx_fund_forms_category_type_status` (`fund_category`,`form_type`,`status`),
  ADD KEY `idx_fund_forms_status_effective_expiry` (`status`);

--
-- Indexes for table `fund_subcategories`
--
ALTER TABLE `fund_subcategories`
  ADD PRIMARY KEY (`subcategory_id`),
  ADD KEY `category_id` (`category_id`);

--
-- Indexes for table `notifications`
--
ALTER TABLE `notifications`
  ADD PRIMARY KEY (`notification_id`),
  ADD KEY `idx_user_id` (`user_id`),
  ADD KEY `idx_is_read` (`is_read`),
  ADD KEY `idx_create_at` (`create_at`),
  ADD KEY `idx_type` (`type`),
  ADD KEY `idx_user_unread` (`user_id`,`is_read`),
  ADD KEY `fk_notif_submission` (`related_submission_id`);

--
-- Indexes for table `positions`
--
ALTER TABLE `positions`
  ADD PRIMARY KEY (`position_id`);

--
-- Indexes for table `publication_reward_details`
--
ALTER TABLE `publication_reward_details`
  ADD PRIMARY KEY (`detail_id`),
  ADD UNIQUE KEY `submission_id` (`submission_id`),
  ADD UNIQUE KEY `idx_submission` (`submission_id`),
  ADD KEY `idx_publication_date` (`publication_date`),
  ADD KEY `idx_quartile` (`quartile`),
  ADD KEY `idx_approved_by` (`approved_by`),
  ADD KEY `idx_rejected_by` (`rejected_by`),
  ADD KEY `idx_revision_requested_by` (`revision_requested_by`),
  ADD KEY `idx_approved_at` (`approved_at`),
  ADD KEY `idx_rejected_at` (`rejected_at`);

--
-- Indexes for table `publication_reward_rates`
--
ALTER TABLE `publication_reward_rates`
  ADD PRIMARY KEY (`rate_id`),
  ADD UNIQUE KEY `year_status_quartile` (`year`,`author_status`,`journal_quartile`);

--
-- Indexes for table `reward_config`
--
ALTER TABLE `reward_config`
  ADD PRIMARY KEY (`config_id`),
  ADD UNIQUE KEY `unique_config` (`year`,`journal_quartile`,`delete_at`),
  ADD KEY `idx_active` (`is_active`,`delete_at`),
  ADD KEY `idx_reward_config_year_type` (`year`,`is_active`),
  ADD KEY `idx_year_quartile` (`year`,`journal_quartile`),
  ADD KEY `idx_reward_config_lookup` (`year`,`journal_quartile`,`is_active`);

--
-- Indexes for table `roles`
--
ALTER TABLE `roles`
  ADD PRIMARY KEY (`role_id`);

--
-- Indexes for table `subcategory_budgets`
--
ALTER TABLE `subcategory_budgets`
  ADD PRIMARY KEY (`subcategory_budget_id`),
  ADD KEY `subcategories_budgets_ibfk_1` (`subcategory_id`);

--
-- Indexes for table `submissions`
--
ALTER TABLE `submissions`
  ADD PRIMARY KEY (`submission_id`),
  ADD UNIQUE KEY `submission_number` (`submission_number`),
  ADD UNIQUE KEY `idx_submission_number` (`submission_number`),
  ADD KEY `idx_type` (`submission_type`),
  ADD KEY `idx_user` (`user_id`),
  ADD KEY `idx_year` (`year_id`),
  ADD KEY `idx_status` (`status_id`),
  ADD KEY `idx_dates` (`submitted_at`,`approved_at`),
  ADD KEY `fk_submission_approver` (`approved_by`),
  ADD KEY `idx_submission_type_status` (`submission_type`,`status_id`),
  ADD KEY `idx_submission_user_year` (`user_id`,`year_id`);

--
-- Indexes for table `submission_documents`
--
ALTER TABLE `submission_documents`
  ADD PRIMARY KEY (`document_id`),
  ADD KEY `idx_submission` (`submission_id`),
  ADD KEY `idx_file` (`file_id`),
  ADD KEY `idx_type` (`document_type_id`),
  ADD KEY `fk_doc_verifier` (`verified_by`),
  ADD KEY `idx_submission_documents_submission` (`submission_id`,`document_type_id`),
  ADD KEY `idx_submission_documents_file` (`file_id`);

--
-- Indexes for table `submission_reviews`
--
ALTER TABLE `submission_reviews`
  ADD PRIMARY KEY (`review_id`),
  ADD KEY `idx_submission` (`submission_id`),
  ADD KEY `idx_reviewer` (`reviewer_id`),
  ADD KEY `idx_status` (`review_status`),
  ADD KEY `idx_round` (`review_round`);

--
-- Indexes for table `submission_status_history`
--
ALTER TABLE `submission_status_history`
  ADD PRIMARY KEY (`history_id`),
  ADD KEY `idx_submission` (`submission_id`),
  ADD KEY `idx_changed_by` (`changed_by`),
  ADD KEY `idx_created` (`created_at`),
  ADD KEY `fk_history_old_status` (`old_status_id`),
  ADD KEY `fk_history_new_status` (`new_status_id`);

--
-- Indexes for table `submission_users`
--
ALTER TABLE `submission_users`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `unique_submission_user` (`submission_id`,`user_id`),
  ADD KEY `idx_submission` (`submission_id`),
  ADD KEY `idx_user` (`user_id`),
  ADD KEY `idx_role` (`role`),
  ADD KEY `idx_submission_users_search` (`user_id`,`role`);

--
-- Indexes for table `system_config`
--
ALTER TABLE `system_config`
  ADD PRIMARY KEY (`config_id`),
  ADD KEY `updated_by` (`updated_by`);

--
-- Indexes for table `users`
--
ALTER TABLE `users`
  ADD PRIMARY KEY (`user_id`),
  ADD KEY `role_id` (`role_id`),
  ADD KEY `position_id` (`position_id`),
  ADD KEY `idx_email` (`email`),
  ADD KEY `idx_fullname` (`user_fname`,`user_lname`);

--
-- Indexes for table `user_fund_eligibilities`
--
ALTER TABLE `user_fund_eligibilities`
  ADD PRIMARY KEY (`user_fund_eligibility_id`),
  ADD KEY `user_id` (`user_id`),
  ADD KEY `year_id` (`year_id`),
  ADD KEY `category_id` (`category_id`);

--
-- Indexes for table `user_sessions`
--
ALTER TABLE `user_sessions`
  ADD PRIMARY KEY (`session_id`),
  ADD UNIQUE KEY `access_token_jti` (`access_token_jti`),
  ADD KEY `idx_user_active` (`user_id`,`is_active`),
  ADD KEY `idx_expires` (`expires_at`),
  ADD KEY `idx_refresh_token` (`refresh_token`),
  ADD KEY `idx_cleanup` (`is_active`,`expires_at`);

--
-- Indexes for table `user_tokens`
--
ALTER TABLE `user_tokens`
  ADD PRIMARY KEY (`token_id`),
  ADD KEY `idx_token` (`token`),
  ADD KEY `idx_user_expires` (`user_id`,`expires_at`);

--
-- Indexes for table `years`
--
ALTER TABLE `years`
  ADD PRIMARY KEY (`year_id`);

--
-- AUTO_INCREMENT for dumped tables
--

--
-- AUTO_INCREMENT for table `announcements`
--
ALTER TABLE `announcements`
  MODIFY `announcement_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=4;

--
-- AUTO_INCREMENT for table `application_status`
--
ALTER TABLE `application_status`
  MODIFY `application_status_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=6;

--
-- AUTO_INCREMENT for table `audit_logs`
--
ALTER TABLE `audit_logs`
  MODIFY `log_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=126;

--
-- AUTO_INCREMENT for table `document_types`
--
ALTER TABLE `document_types`
  MODIFY `document_type_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=13;

--
-- AUTO_INCREMENT for table `file_uploads`
--
ALTER TABLE `file_uploads`
  MODIFY `file_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=85;

--
-- AUTO_INCREMENT for table `fund_application_details`
--
ALTER TABLE `fund_application_details`
  MODIFY `detail_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `fund_categories`
--
ALTER TABLE `fund_categories`
  MODIFY `category_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=6;

--
-- AUTO_INCREMENT for table `fund_forms`
--
ALTER TABLE `fund_forms`
  MODIFY `form_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=6;

--
-- AUTO_INCREMENT for table `fund_subcategories`
--
ALTER TABLE `fund_subcategories`
  MODIFY `subcategory_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=19;

--
-- AUTO_INCREMENT for table `notifications`
--
ALTER TABLE `notifications`
  MODIFY `notification_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `positions`
--
ALTER TABLE `positions`
  MODIFY `position_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=4;

--
-- AUTO_INCREMENT for table `publication_reward_details`
--
ALTER TABLE `publication_reward_details`
  MODIFY `detail_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=40;

--
-- AUTO_INCREMENT for table `publication_reward_rates`
--
ALTER TABLE `publication_reward_rates`
  MODIFY `rate_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=29;

--
-- AUTO_INCREMENT for table `reward_config`
--
ALTER TABLE `reward_config`
  MODIFY `config_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=24;

--
-- AUTO_INCREMENT for table `roles`
--
ALTER TABLE `roles`
  MODIFY `role_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=4;

--
-- AUTO_INCREMENT for table `subcategory_budgets`
--
ALTER TABLE `subcategory_budgets`
  MODIFY `subcategory_budget_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=31;

--
-- AUTO_INCREMENT for table `submissions`
--
ALTER TABLE `submissions`
  MODIFY `submission_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=49;

--
-- AUTO_INCREMENT for table `submission_documents`
--
ALTER TABLE `submission_documents`
  MODIFY `document_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=85;

--
-- AUTO_INCREMENT for table `submission_reviews`
--
ALTER TABLE `submission_reviews`
  MODIFY `review_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `submission_status_history`
--
ALTER TABLE `submission_status_history`
  MODIFY `history_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `submission_users`
--
ALTER TABLE `submission_users`
  MODIFY `id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=92;

--
-- AUTO_INCREMENT for table `system_config`
--
ALTER TABLE `system_config`
  MODIFY `config_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `users`
--
ALTER TABLE `users`
  MODIFY `user_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=11;

--
-- AUTO_INCREMENT for table `user_fund_eligibilities`
--
ALTER TABLE `user_fund_eligibilities`
  MODIFY `user_fund_eligibility_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=3;

--
-- AUTO_INCREMENT for table `user_sessions`
--
ALTER TABLE `user_sessions`
  MODIFY `session_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=115;

--
-- AUTO_INCREMENT for table `user_tokens`
--
ALTER TABLE `user_tokens`
  MODIFY `token_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=115;

--
-- AUTO_INCREMENT for table `years`
--
ALTER TABLE `years`
  MODIFY `year_id` int(11) NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=4;

--
-- Constraints for dumped tables
--

--
-- Constraints for table `announcements`
--
ALTER TABLE `announcements`
  ADD CONSTRAINT `fk_announcements_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `audit_logs`
--
ALTER TABLE `audit_logs`
  ADD CONSTRAINT `fk_audit_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `file_uploads`
--
ALTER TABLE `file_uploads`
  ADD CONSTRAINT `fk_file_uploads_user` FOREIGN KEY (`uploaded_by`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `fund_application_details`
--
ALTER TABLE `fund_application_details`
  ADD CONSTRAINT `fk_fund_approved_by_user` FOREIGN KEY (`approved_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  ADD CONSTRAINT `fk_fund_detail_subcategory` FOREIGN KEY (`subcategory_id`) REFERENCES `fund_subcategories` (`subcategory_id`),
  ADD CONSTRAINT `fk_fund_detail_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`),
  ADD CONSTRAINT `fk_fund_rejected_by_user` FOREIGN KEY (`rejected_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL;

--
-- Constraints for table `fund_categories`
--
ALTER TABLE `fund_categories`
  ADD CONSTRAINT `fund_categories_ibfk_1` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`);

--
-- Constraints for table `fund_forms`
--
ALTER TABLE `fund_forms`
  ADD CONSTRAINT `fk_fund_forms_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `fund_subcategories`
--
ALTER TABLE `fund_subcategories`
  ADD CONSTRAINT `fund_subcategories_ibfk_1` FOREIGN KEY (`category_id`) REFERENCES `fund_categories` (`category_id`);

--
-- Constraints for table `notifications`
--
ALTER TABLE `notifications`
  ADD CONSTRAINT `fk_notif_submission` FOREIGN KEY (`related_submission_id`) REFERENCES `submissions` (`submission_id`),
  ADD CONSTRAINT `fk_notifications_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `publication_reward_details`
--
ALTER TABLE `publication_reward_details`
  ADD CONSTRAINT `fk_approved_by_user` FOREIGN KEY (`approved_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  ADD CONSTRAINT `fk_pub_detail_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`),
  ADD CONSTRAINT `fk_rejected_by_user` FOREIGN KEY (`rejected_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  ADD CONSTRAINT `fk_revision_requested_by_user` FOREIGN KEY (`revision_requested_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL;

--
-- Constraints for table `subcategory_budgets`
--
ALTER TABLE `subcategory_budgets`
  ADD CONSTRAINT `subcategories_budgets_ibfk_1` FOREIGN KEY (`subcategory_id`) REFERENCES `fund_subcategories` (`subcategory_id`);

--
-- Constraints for table `submissions`
--
ALTER TABLE `submissions`
  ADD CONSTRAINT `fk_submission_approver` FOREIGN KEY (`approved_by`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_submission_status` FOREIGN KEY (`status_id`) REFERENCES `application_status` (`application_status_id`),
  ADD CONSTRAINT `fk_submission_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_submission_year` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`);

--
-- Constraints for table `submission_documents`
--
ALTER TABLE `submission_documents`
  ADD CONSTRAINT `fk_doc_file` FOREIGN KEY (`file_id`) REFERENCES `file_uploads` (`file_id`),
  ADD CONSTRAINT `fk_doc_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`),
  ADD CONSTRAINT `fk_doc_type` FOREIGN KEY (`document_type_id`) REFERENCES `document_types` (`document_type_id`),
  ADD CONSTRAINT `fk_doc_verifier` FOREIGN KEY (`verified_by`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `submission_reviews`
--
ALTER TABLE `submission_reviews`
  ADD CONSTRAINT `fk_review_reviewer` FOREIGN KEY (`reviewer_id`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_review_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`);

--
-- Constraints for table `submission_status_history`
--
ALTER TABLE `submission_status_history`
  ADD CONSTRAINT `fk_history_new_status` FOREIGN KEY (`new_status_id`) REFERENCES `application_status` (`application_status_id`),
  ADD CONSTRAINT `fk_history_old_status` FOREIGN KEY (`old_status_id`) REFERENCES `application_status` (`application_status_id`),
  ADD CONSTRAINT `fk_history_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`),
  ADD CONSTRAINT `fk_history_user` FOREIGN KEY (`changed_by`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `submission_users`
--
ALTER TABLE `submission_users`
  ADD CONSTRAINT `fk_submission_user_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`),
  ADD CONSTRAINT `fk_submission_user_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `system_config`
--
ALTER TABLE `system_config`
  ADD CONSTRAINT `system_config_ibfk_1` FOREIGN KEY (`updated_by`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `users`
--
ALTER TABLE `users`
  ADD CONSTRAINT `users_ibfk_1` FOREIGN KEY (`role_id`) REFERENCES `roles` (`role_id`),
  ADD CONSTRAINT `users_ibfk_2` FOREIGN KEY (`position_id`) REFERENCES `positions` (`position_id`);

--
-- Constraints for table `user_fund_eligibilities`
--
ALTER TABLE `user_fund_eligibilities`
  ADD CONSTRAINT `user_fund_eligibilities_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `user_fund_eligibilities_ibfk_2` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`),
  ADD CONSTRAINT `user_fund_eligibilities_ibfk_3` FOREIGN KEY (`category_id`) REFERENCES `fund_categories` (`category_id`);

--
-- Constraints for table `user_sessions`
--
ALTER TABLE `user_sessions`
  ADD CONSTRAINT `user_sessions_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `user_tokens`
--
ALTER TABLE `user_tokens`
  ADD CONSTRAINT `user_tokens_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);
COMMIT;

/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
