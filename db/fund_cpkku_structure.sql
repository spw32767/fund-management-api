-- phpMyAdmin SQL Dump
-- version 5.2.3
-- https://www.phpmyadmin.net/
--
-- Host: localhost:3306
-- Generation Time: Mar 21, 2026 at 12:56 AM
-- Server version: 10.11.11-MariaDB-0+deb12u1-log
-- PHP Version: 8.4.17

SET SQL_MODE = "NO_AUTO_VALUE_ON_ZERO";
START TRANSACTION;
SET time_zone = "+00:00";


/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!40101 SET NAMES utf8mb4 */;

--
-- Database: `drnadech_fund_cpkku`
--

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
  `announcement_type` enum('general','research_fund','promotion_fund','publication_reward','fund_application') DEFAULT 'general' COMMENT 'ประเภทประกาศ',
  `announcement_reference_number` varchar(50) DEFAULT NULL,
  `priority` enum('normal','high','urgent') DEFAULT 'normal' COMMENT 'ความสำคัญ',
  `display_order` int(11) DEFAULT NULL,
  `status` enum('active','inactive') DEFAULT 'active' COMMENT 'สถานะการเผยแพร่',
  `published_at` datetime DEFAULT NULL COMMENT 'วันที่เผยแพร่',
  `expired_at` datetime DEFAULT NULL COMMENT 'วันที่หมดอายุ',
  `year_id` int(11) DEFAULT NULL COMMENT 'ปีของประกาศ',
  `created_by` int(11) NOT NULL COMMENT 'ผู้สร้าง (user_id)',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='ตารางเก็บประกาศจากกองทุนวิจัยและนวัตกรรม';

-- --------------------------------------------------------

--
-- Table structure for table `announcement_assignments`
--

CREATE TABLE `announcement_assignments` (
  `assignment_id` int(11) NOT NULL,
  `slot_code` enum('main','reward','activity_support','conference','service') NOT NULL COMMENT 'ช่องประกาศที่ FE กำหนด',
  `announcement_id` int(11) DEFAULT NULL COMMENT 'อาจเป็น NULL เพื่อระบุช่วงที่ไม่มีประกาศ',
  `start_date` datetime NOT NULL,
  `end_date` datetime DEFAULT NULL,
  `changed_by` int(11) DEFAULT NULL,
  `changed_at` datetime NOT NULL DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `application_status`
--

CREATE TABLE `application_status` (
  `application_status_id` int(11) NOT NULL,
  `status_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `status_name` varchar(255) DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `audit_logs`
--

CREATE TABLE `audit_logs` (
  `log_id` int(11) NOT NULL,
  `user_id` int(11) DEFAULT NULL,
  `action` enum('create','update','delete','login','logout','view','download','approve','reject','submit','review','request_revision') NOT NULL,
  `entity_type` varchar(50) NOT NULL,
  `entity_id` int(11) DEFAULT NULL,
  `entity_number` varchar(50) DEFAULT NULL,
  `old_values` longtext DEFAULT NULL CHECK (json_valid(`old_values`)),
  `new_values` longtext DEFAULT NULL CHECK (json_valid(`new_values`)),
  `changed_fields` text DEFAULT NULL,
  `ip_address` varchar(45) DEFAULT NULL,
  `user_agent` varchar(255) DEFAULT NULL,
  `description` text DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `auth_identities`
--

CREATE TABLE `auth_identities` (
  `identity_id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `provider` varchar(50) NOT NULL DEFAULT 'kku_sso',
  `provider_subject` varchar(191) DEFAULT NULL,
  `email_at_provider` varchar(255) DEFAULT NULL,
  `raw_claims` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (json_valid(`raw_claims`)),
  `last_login_at` datetime DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT NULL ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `citescore_metrics_runs`
--

CREATE TABLE `citescore_metrics_runs` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `run_type` varchar(32) NOT NULL,
  `status` varchar(32) NOT NULL DEFAULT 'running',
  `error_message` text DEFAULT NULL,
  `sources_scanned` int(11) NOT NULL DEFAULT 0,
  `sources_refreshed` int(11) NOT NULL DEFAULT 0,
  `skipped` int(11) NOT NULL DEFAULT 0,
  `errors` int(11) NOT NULL DEFAULT 0,
  `journals_scanned` int(11) NOT NULL DEFAULT 0,
  `metrics_fetched` int(11) NOT NULL DEFAULT 0,
  `skipped_existing` int(11) NOT NULL DEFAULT 0,
  `started_at` datetime NOT NULL DEFAULT current_timestamp(),
  `finished_at` datetime DEFAULT NULL,
  `duration_seconds` double DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `cp_employee`
--

CREATE TABLE `cp_employee` (
  `ID` int(11) NOT NULL,
  `prefix` varchar(50) DEFAULT NULL,
  `FullName` varchar(255) DEFAULT NULL,
  `Name` varchar(100) DEFAULT NULL,
  `Lname` varchar(100) DEFAULT NULL,
  `manage_position` varchar(255) DEFAULT NULL,
  `position` varchar(255) DEFAULT NULL,
  `position_en` varchar(255) DEFAULT NULL,
  `prefix_position_en` varchar(50) DEFAULT NULL,
  `Name_en` varchar(255) DEFAULT NULL,
  `suffix_en` varchar(50) DEFAULT NULL,
  `Email` varchar(255) DEFAULT NULL,
  `TEL` varchar(50) DEFAULT NULL,
  `TELformat` varchar(50) DEFAULT NULL,
  `TEL_ENG` varchar(50) DEFAULT NULL,
  `manage_position_en` varchar(255) DEFAULT NULL,
  `LAB_Name` varchar(255) DEFAULT NULL,
  `Room` varchar(255) DEFAULT NULL,
  `CP_WEB_ID` varchar(255) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `cp_profile`
--

CREATE TABLE `cp_profile` (
  `id` int(11) NOT NULL,
  `user_id` int(11) DEFAULT NULL COMMENT 'fk users table	',
  `name_th` varchar(255) NOT NULL COMMENT 'ชื่อ (ภาษาไทย)',
  `name_en` varchar(255) DEFAULT NULL COMMENT 'Name (English)',
  `position` varchar(255) DEFAULT NULL COMMENT 'ตำแหน่ง',
  `email` varchar(255) DEFAULT NULL COMMENT 'อีเมล',
  `photo_url` varchar(500) DEFAULT NULL COMMENT 'URL รูปโปรไฟล์',
  `info` text DEFAULT NULL COMMENT 'ข้อมูล (แท็บข้อมูล)',
  `education` text DEFAULT NULL COMMENT 'ประวัติการศึกษา',
  `profile_url` varchar(500) DEFAULT NULL COMMENT 'ลิงก์โปรไฟล์ต้นทาง',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `dept_head_assignments`
--

CREATE TABLE `dept_head_assignments` (
  `assignment_id` int(11) NOT NULL,
  `head_user_id` int(11) NOT NULL,
  `restore_role_id` int(11) NOT NULL,
  `effective_from` datetime NOT NULL,
  `effective_to` datetime DEFAULT NULL,
  `changed_by` int(11) DEFAULT NULL,
  `changed_at` datetime NOT NULL DEFAULT current_timestamp(),
  `note` varchar(255) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `document_types`
--

CREATE TABLE `document_types` (
  `document_type_id` int(11) NOT NULL,
  `document_type_name` varchar(255) DEFAULT NULL,
  `code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `category` varchar(50) DEFAULT 'general' COMMENT 'ไม่ได้ใช้',
  `required` tinyint(1) DEFAULT 0,
  `multiple` tinyint(1) DEFAULT 0,
  `document_order` int(11) DEFAULT 0,
  `is_required` enum('yes','no') DEFAULT NULL COMMENT 'ไม่ได้ใช้',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL,
  `fund_types` longtext DEFAULT NULL COMMENT 'ประเภททุนที่ใช้ได้ ["publication_reward", "fund_application"]' CHECK (json_valid(`fund_types`)),
  `subcategory_ids` longtext DEFAULT NULL COMMENT 'รหัส subcategory เฉพาะ [1,2,3] หรือ NULL = ทุก subcategory' CHECK (json_valid(`subcategory_ids`)),
  `subcategory_name` longtext DEFAULT NULL COMMENT 'snapshot ของชื่อทุน ไม่ผูก FK'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `end_of_contract`
--

CREATE TABLE `end_of_contract` (
  `eoc_id` int(11) NOT NULL,
  `content` longtext NOT NULL,
  `display_order` int(11) NOT NULL DEFAULT 1
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

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
  `file_hash` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `is_public` tinyint(1) DEFAULT 0,
  `uploaded_by` int(11) DEFAULT NULL,
  `uploaded_at` datetime DEFAULT current_timestamp(),
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
  `closed_at` datetime DEFAULT NULL,
  `announce_reference_number` varchar(50) DEFAULT NULL,
  `main_annoucement` int(11) DEFAULT NULL,
  `activity_support_announcement` int(11) DEFAULT NULL,
  `author_name_list` varchar(500) DEFAULT NULL
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
  `display_order` int(11) DEFAULT NULL,
  `status` enum('active','inactive','archived') DEFAULT 'active' COMMENT 'สถานะแบบฟอร์ม',
  `year_id` int(11) DEFAULT NULL COMMENT 'ปีของแบบฟอร์ม',
  `created_by` int(11) NOT NULL COMMENT 'ผู้สร้าง (user_id)',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='ตารางเก็บแบบฟอร์มและเอกสารที่เกี่ยวข้องกับการขอทุน';

-- --------------------------------------------------------

--
-- Table structure for table `fund_installment_periods`
--

CREATE TABLE `fund_installment_periods` (
  `installment_period_id` int(11) NOT NULL,
  `fund_level` enum('category','subcategory') NOT NULL DEFAULT 'category',
  `fund_keyword` varchar(255) NOT NULL DEFAULT '',
  `fund_parent_keyword` varchar(255) DEFAULT NULL,
  `year_id` int(11) NOT NULL,
  `installment_number` int(11) NOT NULL,
  `cutoff_date` date NOT NULL,
  `name` varchar(255) DEFAULT NULL,
  `status` enum('active','inactive') DEFAULT 'active',
  `remark` text DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- --------------------------------------------------------

--
-- Table structure for table `fund_subcategories`
--

CREATE TABLE `fund_subcategories` (
  `subcategory_id` int(11) NOT NULL,
  `category_id` int(11) DEFAULT NULL,
  `subcategory_name` varchar(255) DEFAULT NULL,
  `subcategory_code` varchar(100) DEFAULT NULL,
  `fund_condition` text DEFAULT NULL,
  `target_roles` longtext DEFAULT NULL COMMENT 'บทบาทที่สามารถเห็นทุนนี้ได้ (เก็บเป็น JSON array)',
  `form_type` varchar(50) DEFAULT 'download' COMMENT 'ประเภทฟอร์ม: download, publication_reward, research_proposal, etc.',
  `form_url` varchar(255) DEFAULT NULL COMMENT 'URL สำหรับดาวน์โหลดฟอร์ม (ถ้า form_type = download)',
  `year_id` int(255) DEFAULT NULL,
  `status` enum('active','disable') DEFAULT NULL,
  `comment` text DEFAULT NULL,
  `create_at` datetime DEFAULT NULL,
  `update_at` datetime DEFAULT NULL,
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `import_templates`
--

CREATE TABLE `import_templates` (
  `template_id` int(11) NOT NULL,
  `title` varchar(255) NOT NULL COMMENT 'ชื่อเทมเพลตนำเข้า',
  `description` text DEFAULT NULL COMMENT 'รายละเอียดเทมเพลต',
  `file_name` varchar(255) NOT NULL COMMENT 'ชื่อไฟล์ต้นฉบับ',
  `file_path` varchar(500) NOT NULL COMMENT 'path ไฟล์ในระบบ',
  `file_size` bigint(20) DEFAULT NULL COMMENT 'ขนาดไฟล์ (bytes)',
  `mime_type` varchar(100) DEFAULT NULL COMMENT 'ประเภทไฟล์',
  `template_type` enum('user_import','legacy_submission','other') DEFAULT 'other' COMMENT 'ประเภทการนำเข้า',
  `is_required` tinyint(1) DEFAULT 0 COMMENT 'บังคับใช้หรือไม่',
  `display_order` int(11) DEFAULT NULL,
  `status` enum('active','inactive','archived') DEFAULT 'active' COMMENT 'สถานะเทมเพลต',
  `year_id` int(11) DEFAULT NULL COMMENT 'ปีที่เกี่ยวข้อง',
  `created_by` int(11) NOT NULL COMMENT 'ผู้สร้าง (user_id)',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='ตารางเก็บไฟล์เทมเพลตสำหรับการนำเข้า';

-- --------------------------------------------------------

--
-- Table structure for table `innovations`
--

CREATE TABLE `innovations` (
  `id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `title` varchar(500) NOT NULL,
  `innovation_type` varchar(255) DEFAULT NULL,
  `description` text DEFAULT NULL,
  `registered_date` date DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `kku_people_import_runs`
--

CREATE TABLE `kku_people_import_runs` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `trigger_source` varchar(64) NOT NULL,
  `dry_run` tinyint(1) NOT NULL DEFAULT 0,
  `status` enum('running','success','failed') NOT NULL DEFAULT 'running',
  `error_message` text DEFAULT NULL,
  `started_at` datetime(6) NOT NULL DEFAULT current_timestamp(6),
  `finished_at` datetime(6) DEFAULT NULL,
  `duration_seconds` double DEFAULT NULL,
  `fetched_count` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `created_count` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `updated_count` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `failed_count` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `exit_code` int(11) DEFAULT NULL,
  `stdout` longtext DEFAULT NULL,
  `stderr` longtext DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT current_timestamp(6),
  `updated_at` datetime(6) NOT NULL DEFAULT current_timestamp(6) ON UPDATE current_timestamp(6),
  `deleted_at` datetime(6) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `notification_message`
--

CREATE TABLE `notification_message` (
  `id` bigint(20) NOT NULL,
  `event_key` varchar(100) NOT NULL,
  `send_to` enum('user','dept_head','admin') NOT NULL,
  `title_template` text NOT NULL,
  `body_template` text NOT NULL,
  `default_title_template` text NOT NULL,
  `default_body_template` text NOT NULL,
  `description` text DEFAULT NULL,
  `variables` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL CHECK (json_valid(`variables`)),
  `default_variables` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL CHECK (json_valid(`default_variables`)),
  `is_active` tinyint(1) NOT NULL DEFAULT 1,
  `updated_by` bigint(20) DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT current_timestamp(),
  `updated_at` timestamp NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `positions`
--

CREATE TABLE `positions` (
  `position_id` int(11) NOT NULL,
  `position_name` varchar(255) DEFAULT NULL,
  `is_active` enum('yes','no') DEFAULT 'yes',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT NULL ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `projects`
--

CREATE TABLE `projects` (
  `project_id` int(10) UNSIGNED NOT NULL,
  `project_name` varchar(255) NOT NULL COMMENT 'ชื่อโครงการ',
  `type_id` tinyint(3) UNSIGNED NOT NULL COMMENT 'FK -> project_types',
  `event_date` date NOT NULL COMMENT 'วันที่จัด',
  `plan_id` tinyint(3) UNSIGNED NOT NULL COMMENT 'FK -> project_budget_plans',
  `budget_amount` decimal(12,2) UNSIGNED NOT NULL DEFAULT 0.00 COMMENT 'งบประมาณ',
  `participants` int(10) UNSIGNED NOT NULL DEFAULT 0 COMMENT 'จำนวนผู้เข้าร่วม',
  `notes` text DEFAULT NULL COMMENT 'หมายเหตุ',
  `created_by` int(10) UNSIGNED DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT current_timestamp(),
  `updated_at` timestamp NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_general_ci;

-- --------------------------------------------------------

--
-- Table structure for table `project_attachments`
--

CREATE TABLE `project_attachments` (
  `file_id` int(10) UNSIGNED NOT NULL,
  `project_id` int(10) UNSIGNED NOT NULL,
  `original_name` varchar(255) NOT NULL,
  `stored_path` varchar(500) NOT NULL,
  `file_size` bigint(20) UNSIGNED NOT NULL DEFAULT 0,
  `mime_type` varchar(100) NOT NULL,
  `file_hash` varchar(64) DEFAULT NULL,
  `is_public` tinyint(1) NOT NULL DEFAULT 0,
  `uploaded_by` int(10) UNSIGNED DEFAULT NULL,
  `uploaded_at` datetime NOT NULL DEFAULT current_timestamp(),
  `create_at` datetime NOT NULL DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL,
  `display_order` smallint(5) UNSIGNED NOT NULL DEFAULT 1
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `project_budget_plans`
--

CREATE TABLE `project_budget_plans` (
  `plan_id` tinyint(3) UNSIGNED NOT NULL,
  `name_th` varchar(255) NOT NULL,
  `name_en` varchar(255) NOT NULL,
  `display_order` smallint(5) UNSIGNED NOT NULL DEFAULT 1,
  `is_active` tinyint(1) NOT NULL DEFAULT 1
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `project_members`
--

CREATE TABLE `project_members` (
  `member_id` bigint(20) UNSIGNED NOT NULL,
  `project_id` int(10) UNSIGNED NOT NULL,
  `user_id` int(10) UNSIGNED NOT NULL,
  `duty` varchar(255) NOT NULL,
  `workload_hours` decimal(6,2) UNSIGNED NOT NULL DEFAULT 0.00 COMMENT 'ชั่วโมง',
  `display_order` smallint(5) UNSIGNED NOT NULL DEFAULT 1,
  `notes` varchar(255) DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT current_timestamp(),
  `updated_at` timestamp NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `project_types`
--

CREATE TABLE `project_types` (
  `type_id` tinyint(3) UNSIGNED NOT NULL,
  `name_th` varchar(255) NOT NULL,
  `name_en` varchar(255) NOT NULL,
  `display_order` smallint(5) UNSIGNED NOT NULL DEFAULT 1,
  `is_active` tinyint(1) NOT NULL DEFAULT 1
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `publications`
--

CREATE TABLE `publications` (
  `id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `title` varchar(500) NOT NULL,
  `authors` text DEFAULT NULL,
  `journal` varchar(255) DEFAULT NULL,
  `publication_type` enum('journal','conference','book','thesis','other') DEFAULT NULL,
  `publication_date` date DEFAULT NULL,
  `publication_year` smallint(5) UNSIGNED DEFAULT NULL,
  `doi` varchar(255) DEFAULT NULL,
  `url` varchar(512) DEFAULT NULL,
  `cited_by` int(10) UNSIGNED DEFAULT NULL,
  `cited_by_url` varchar(512) DEFAULT NULL,
  `source` enum('scholar','openalex','orcid','crossref') DEFAULT NULL,
  `external_ids` longtext DEFAULT NULL CHECK (json_valid(`external_ids`)),
  `fingerprint` varchar(64) DEFAULT NULL,
  `is_verified` tinyint(1) NOT NULL DEFAULT 0,
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT NULL,
  `citation_history` longtext DEFAULT NULL COMMENT 'citations per year, e.g. {"2018":8,"2019":22}'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
  `approved_amount` decimal(15,2) DEFAULT NULL COMMENT 'ไม่ได้ใช้',
  `create_at` datetime NOT NULL DEFAULT current_timestamp(),
  `update_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL,
  `main_annoucement` int(11) DEFAULT NULL,
  `reward_announcement` int(11) DEFAULT NULL,
  `author_name_list` text DEFAULT NULL,
  `signature` varchar(255) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='ตารางเก็บรายละเอียดการขอรับเงินรางวัลผลงานวิชาการ พร้อมข้อมูลเพิ่มเติม';

-- --------------------------------------------------------

--
-- Table structure for table `publication_reward_external_funds`
--

CREATE TABLE `publication_reward_external_funds` (
  `external_fund_id` int(11) NOT NULL,
  `detail_id` int(11) NOT NULL,
  `submission_id` int(11) NOT NULL,
  `fund_name` varchar(255) DEFAULT NULL,
  `amount` decimal(15,2) DEFAULT 0.00,
  `document_id` int(11) DEFAULT NULL,
  `file_id` int(11) DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='รายละเอียดทุนภายนอกและไฟล์ประกอบของคำร้องขอรางวัลตีพิมพ์';

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `research_fund_admin_events`
--

CREATE TABLE `research_fund_admin_events` (
  `event_id` int(11) NOT NULL,
  `submission_id` int(11) NOT NULL,
  `status_after_id` int(11) DEFAULT NULL,
  `amount` decimal(15,2) DEFAULT NULL,
  `comment` text DEFAULT NULL,
  `created_by` int(11) NOT NULL,
  `created_at` datetime DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `research_fund_event_files`
--

CREATE TABLE `research_fund_event_files` (
  `event_file_id` int(11) NOT NULL,
  `event_id` int(11) NOT NULL,
  `file_id` int(11) NOT NULL,
  `created_at` datetime DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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

-- --------------------------------------------------------

--
-- Table structure for table `roles`
--

CREATE TABLE `roles` (
  `role_id` int(11) NOT NULL,
  `role` varchar(255) DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT NULL ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scholar_import_runs`
--

CREATE TABLE `scholar_import_runs` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `trigger_source` varchar(64) NOT NULL,
  `status` enum('running','success','failed') NOT NULL DEFAULT 'running',
  `error_message` text DEFAULT NULL,
  `started_at` datetime NOT NULL DEFAULT current_timestamp(),
  `finished_at` datetime DEFAULT NULL,
  `users_processed` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `users_with_errors` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `publications_fetched` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `publications_created` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `publications_updated` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `publications_failed` int(10) UNSIGNED NOT NULL DEFAULT 0,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scopus_affiliations`
--

CREATE TABLE `scopus_affiliations` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `afid` varchar(32) NOT NULL,
  `name` text DEFAULT NULL,
  `city` text DEFAULT NULL,
  `country` text DEFAULT NULL,
  `affiliation_url` text DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scopus_api_import_jobs`
--

CREATE TABLE `scopus_api_import_jobs` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `service` varchar(64) NOT NULL DEFAULT 'scopus',
  `job_type` varchar(64) NOT NULL DEFAULT 'author_documents',
  `scopus_author_id` varchar(100) DEFAULT NULL,
  `query_string` text NOT NULL,
  `total_results` int(11) DEFAULT NULL,
  `status` varchar(32) NOT NULL DEFAULT 'running',
  `error_message` text DEFAULT NULL,
  `started_at` datetime NOT NULL DEFAULT current_timestamp(),
  `finished_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scopus_api_requests`
--

CREATE TABLE `scopus_api_requests` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `job_id` bigint(20) UNSIGNED NOT NULL,
  `http_method` varchar(8) NOT NULL DEFAULT 'GET',
  `endpoint` text NOT NULL,
  `query_params` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (json_valid(`query_params`)),
  `request_headers` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (json_valid(`request_headers`)),
  `response_status` int(11) DEFAULT NULL,
  `response_time_ms` int(11) DEFAULT NULL,
  `page_start` int(11) DEFAULT NULL,
  `page_count` int(11) DEFAULT NULL,
  `items_returned` int(11) DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scopus_authors`
--

CREATE TABLE `scopus_authors` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `scopus_author_id` varchar(100) NOT NULL,
  `full_name` text DEFAULT NULL,
  `given_name` text DEFAULT NULL,
  `surname` text DEFAULT NULL,
  `initials` text DEFAULT NULL,
  `orcid` varchar(64) DEFAULT NULL,
  `author_url` text DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scopus_batch_import_runs`
--

CREATE TABLE `scopus_batch_import_runs` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `status` varchar(32) NOT NULL DEFAULT 'running',
  `error_message` text DEFAULT NULL,
  `requested_user_ids` text DEFAULT NULL,
  `limit` int(11) DEFAULT NULL,
  `users_processed` int(11) NOT NULL DEFAULT 0,
  `users_with_errors` int(11) NOT NULL DEFAULT 0,
  `documents_fetched` int(11) NOT NULL DEFAULT 0,
  `documents_created` int(11) NOT NULL DEFAULT 0,
  `documents_updated` int(11) NOT NULL DEFAULT 0,
  `documents_failed` int(11) NOT NULL DEFAULT 0,
  `authors_created` int(11) NOT NULL DEFAULT 0,
  `authors_updated` int(11) NOT NULL DEFAULT 0,
  `affiliations_created` int(11) NOT NULL DEFAULT 0,
  `affiliations_updated` int(11) NOT NULL DEFAULT 0,
  `links_inserted` int(11) NOT NULL DEFAULT 0,
  `links_updated` int(11) NOT NULL DEFAULT 0,
  `started_at` datetime NOT NULL DEFAULT current_timestamp(),
  `finished_at` datetime DEFAULT NULL,
  `duration_seconds` double DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scopus_config`
--

CREATE TABLE `scopus_config` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `key` varchar(128) NOT NULL,
  `value` text NOT NULL,
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scopus_documents`
--

CREATE TABLE `scopus_documents` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `eid` varchar(64) NOT NULL,
  `scopus_id` varchar(64) DEFAULT NULL,
  `scopus_link` text DEFAULT NULL,
  `title` text DEFAULT NULL,
  `abstract` longtext DEFAULT NULL,
  `aggregation_type` varchar(32) DEFAULT NULL,
  `subtype` varchar(32) DEFAULT NULL,
  `subtype_description` text DEFAULT NULL,
  `source_id` varchar(32) DEFAULT NULL,
  `publication_name` text DEFAULT NULL,
  `issn` varchar(32) DEFAULT NULL,
  `eissn` varchar(32) DEFAULT NULL,
  `isbn` varchar(64) DEFAULT NULL,
  `volume` varchar(32) DEFAULT NULL,
  `issue` varchar(32) DEFAULT NULL,
  `page_range` varchar(64) DEFAULT NULL,
  `article_number` varchar(64) DEFAULT NULL,
  `cover_date` date DEFAULT NULL,
  `cover_display_date` text DEFAULT NULL,
  `doi` varchar(255) DEFAULT NULL,
  `pii` varchar(64) DEFAULT NULL,
  `citedby_count` int(11) DEFAULT NULL,
  `openaccess` tinyint(4) DEFAULT NULL,
  `openaccess_flag` tinyint(1) DEFAULT NULL,
  `authkeywords` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (json_valid(`authkeywords`)),
  `fund_acr` text DEFAULT NULL,
  `fund_sponsor` text DEFAULT NULL,
  `raw_json` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (json_valid(`raw_json`)),
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scopus_document_authors`
--

CREATE TABLE `scopus_document_authors` (
  `id` bigint(20) UNSIGNED NOT NULL,
  `document_id` bigint(20) UNSIGNED NOT NULL,
  `author_id` bigint(20) UNSIGNED NOT NULL,
  `author_seq` int(11) DEFAULT NULL,
  `affiliation_id` bigint(20) UNSIGNED DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT current_timestamp(),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `scopus_source_metrics`
--

CREATE TABLE `scopus_source_metrics` (
  `source_metric_id` int(11) NOT NULL,
  `source_id` varchar(32) NOT NULL COMMENT 'Scopus source-id (source-id)',
  `issn` varchar(32) DEFAULT NULL COMMENT 'prism:issn',
  `eissn` varchar(32) DEFAULT NULL COMMENT 'prism:eIssn',
  `metric_year` int(4) NOT NULL COMMENT 'year attribute from citeScoreYearInfo / SNIP / SJR / yearly-data',
  `doc_type` varchar(32) NOT NULL DEFAULT 'all' COMMENT 'citeScoreInfo.docType (usually all, article, review, etc.)',
  `cite_score` decimal(8,3) DEFAULT NULL,
  `cite_score_status` enum('Complete','In-Progress') DEFAULT NULL COMMENT 'status attribute on citeScoreYearInfo',
  `cite_score_scholarly_output` int(11) DEFAULT NULL,
  `cite_score_citation_count` int(11) DEFAULT NULL,
  `cite_score_percent_cited` decimal(5,2) DEFAULT NULL,
  `cite_score_rank` int(11) DEFAULT NULL,
  `cite_score_percentile` decimal(5,2) DEFAULT NULL,
  `cite_score_quartile` varchar(4) DEFAULT NULL COMMENT 'Q1, Q2, Q3, Q4 if present',
  `cite_score_current_metric` decimal(8,3) DEFAULT NULL,
  `cite_score_current_metric_year` int(4) DEFAULT NULL,
  `cite_score_tracker` decimal(8,3) DEFAULT NULL,
  `cite_score_tracker_year` int(4) DEFAULT NULL,
  `sjr` decimal(8,3) DEFAULT NULL,
  `snip` decimal(8,3) DEFAULT NULL,
  `publication_count` int(11) DEFAULT NULL,
  `cite_count_sce` int(11) DEFAULT NULL,
  `zero_cites_sce` decimal(5,2) DEFAULT NULL,
  `rev_percent` decimal(5,2) DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `last_fetched_at` datetime DEFAULT NULL COMMENT 'Last time metrics were fetched from Scopus API'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `subcategory_budgets`
--

CREATE TABLE `subcategory_budgets` (
  `subcategory_budget_id` int(11) NOT NULL,
  `subcategory_id` int(11) NOT NULL,
  `record_scope` enum('overall','rule') NOT NULL DEFAULT 'rule',
  `allocated_amount` decimal(15,2) DEFAULT NULL COMMENT 'จำนวนทุนต่อไป',
  `remaining_budget` decimal(15,2) DEFAULT NULL,
  `used_amount` decimal(15,2) DEFAULT NULL,
  `max_amount_per_grant` decimal(15,2) DEFAULT NULL,
  `max_amount_per_year` decimal(15,2) DEFAULT NULL COMMENT 'Per-user per-year cap; set on OVERALL only',
  `max_grants` int(11) DEFAULT NULL,
  `remaining_grant` int(11) DEFAULT NULL,
  `level` enum('ต้น','กลาง','สูง') DEFAULT NULL,
  `status` enum('active','disable') DEFAULT NULL,
  `fund_description` text DEFAULT NULL,
  `comment` text DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

--
-- Triggers `subcategory_budgets`
--
DELIMITER $$
CREATE TRIGGER `bi_subcat_budget_overall` BEFORE INSERT ON `subcategory_budgets` FOR EACH ROW BEGIN
  IF NEW.record_scope = 'overall' THEN
    IF EXISTS (
      SELECT 1 FROM subcategory_budgets
      WHERE subcategory_id = NEW.subcategory_id AND record_scope = 'overall'
    ) THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Each subcategory can have only one OVERALL row.';
    END IF;
  END IF;

  IF NEW.record_scope = 'rule' THEN
    SET NEW.max_amount_per_year = NULL; 
  END IF;
END
$$
DELIMITER ;
DELIMITER $$
CREATE TRIGGER `bu_subcat_budget_overall` BEFORE UPDATE ON `subcategory_budgets` FOR EACH ROW BEGIN
  IF NEW.record_scope = 'overall'
     AND (OLD.subcategory_id <> NEW.subcategory_id OR OLD.record_scope <> 'overall') THEN
    IF EXISTS (
      SELECT 1 FROM subcategory_budgets
      WHERE subcategory_id = NEW.subcategory_id
        AND record_scope = 'overall'
        AND subcategory_budget_id <> OLD.subcategory_budget_id
    ) THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Each subcategory can have only one OVERALL row.';
    END IF;
  END IF;

  IF NEW.record_scope = 'rule' THEN
    SET NEW.max_amount_per_year = NULL;
  END IF;
END
$$
DELIMITER ;

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
  `category_id` int(11) DEFAULT NULL,
  `subcategory_id` int(11) DEFAULT NULL,
  `subcategory_budget_id` int(11) DEFAULT NULL,
  `status_id` int(11) NOT NULL,
  `submitted_at` datetime DEFAULT NULL,
  `reviewed_at` datetime DEFAULT NULL,
  `head_approved_at` datetime DEFAULT NULL,
  `head_rejected_by` int(11) DEFAULT NULL,
  `head_rejected_at` datetime DEFAULT NULL,
  `head_rejection_reason` text DEFAULT NULL,
  `head_comment` text DEFAULT NULL,
  `head_signature` varchar(255) DEFAULT NULL,
  `head_approved_by` int(11) DEFAULT NULL,
  `admin_approved_by` int(11) DEFAULT NULL,
  `admin_approved_at` datetime DEFAULT NULL,
  `admin_rejected_by` int(11) DEFAULT NULL,
  `admin_rejected_at` datetime DEFAULT NULL,
  `admin_rejection_reason` text DEFAULT NULL,
  `admin_comment` text DEFAULT NULL,
  `contact_phone` varchar(50) DEFAULT NULL,
  `bank_account` varchar(50) DEFAULT NULL,
  `bank_name` varchar(100) DEFAULT NULL,
  `bank_account_name` varchar(150) DEFAULT NULL,
  `rejected_by` int(11) DEFAULT NULL,
  `rejected_at` datetime DEFAULT NULL,
  `approved_at` datetime DEFAULT NULL COMMENT 'ไม่ได้ใช้',
  `approved_by` int(11) DEFAULT NULL COMMENT 'ไม่ได้ใช้',
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `deleted_at` datetime DEFAULT NULL,
  `installment_number_at_submit` int(11) DEFAULT NULL,
  `installment_fund_name_at_submit` varchar(255) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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

   
   SET v_user_id = COALESCE(NEW.admin_approved_by, NEW.head_approved_by, NEW.user_id);

   IF IFNULL(OLD.status_id,0) <> IFNULL(NEW.status_id,0) THEN
      SET v_changed_fields = CONCAT(v_changed_fields,'status,');
   END IF;

   IF OLD.status_id <> NEW.status_id AND NEW.status_id = 2 THEN
      SET v_action = 'approve';
   ELSEIF OLD.status_id <> NEW.status_id AND NEW.status_id = 3 THEN
      SET v_action = 'reject';
   ELSEIF OLD.submitted_at IS NULL AND NEW.submitted_at IS NOT NULL THEN
      SET v_action = 'submit';
   ELSE
      SET v_action = 'update';
   END IF;

   IF v_changed_fields <> '' OR v_action <> 'update' THEN
      INSERT INTO audit_logs (
        user_id, action, entity_type, entity_id, entity_number,
        changed_fields, description, created_at
      ) VALUES (
        v_user_id, v_action, 'submission', NEW.submission_id, NEW.submission_number,
        TRIM(TRAILING ',' FROM v_changed_fields), CONCAT(v_action,' submission'), NOW()
      );
   END IF;
END
$$
DELIMITER ;
DELIMITER $$
CREATE TRIGGER `trg_submissions_sync_legacy` AFTER UPDATE ON `submissions` FOR EACH ROW BEGIN
  
  IF (NEW.admin_approved_by <> OLD.admin_approved_by) OR (NEW.admin_approved_at <> OLD.admin_approved_at) THEN
    UPDATE submissions
      SET approved_by = NEW.admin_approved_by,
          approved_at = NEW.admin_approved_at
    WHERE submission_id = NEW.submission_id;
  END IF;

  
  
  IF (NEW.admin_approved_by <> OLD.admin_approved_by) OR (NEW.admin_approved_at <> OLD.admin_approved_at) THEN
    UPDATE publication_reward_details
      SET approved_by = NEW.admin_approved_by,
          approved_at = NEW.admin_approved_at
    WHERE submission_id = NEW.submission_id;
  END IF;

  IF (NEW.rejected_by <> OLD.rejected_by) OR (NEW.rejected_at <> OLD.rejected_at) THEN
    UPDATE publication_reward_details
      SET rejected_by      = NEW.rejected_by,
          rejected_at      = NEW.rejected_at
    WHERE submission_id = NEW.submission_id;
  END IF;

  
  IF (NEW.admin_approved_by <> OLD.admin_approved_by) OR (NEW.admin_approved_at <> OLD.admin_approved_at) THEN
    UPDATE fund_application_details
      SET approved_by = NEW.admin_approved_by,
          approved_at = NEW.admin_approved_at
    WHERE submission_id = NEW.submission_id;
  END IF;

  IF (NEW.rejected_by <> OLD.rejected_by) OR (NEW.rejected_at <> OLD.rejected_at) THEN
    UPDATE fund_application_details
      SET rejected_by = NEW.rejected_by,
          rejected_at = NEW.rejected_at
    WHERE submission_id = NEW.submission_id;
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
  `original_name` varchar(255) DEFAULT NULL,
  `document_type_id` int(11) NOT NULL,
  `description` text DEFAULT NULL,
  `display_order` int(11) DEFAULT 0,
  `is_required` tinyint(1) DEFAULT 0,
  `is_verified` tinyint(1) DEFAULT 0,
  `verified_by` int(11) DEFAULT NULL,
  `verified_at` datetime DEFAULT NULL,
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

-- --------------------------------------------------------

--
-- Table structure for table `support_fundmapping`
--

CREATE TABLE `support_fundmapping` (
  `req_id` int(11) NOT NULL,
  `req_code` char(10) DEFAULT NULL,
  `name` varchar(300) DEFAULT NULL,
  `description` varchar(1000) DEFAULT NULL,
  `dev_plan` varchar(1000) DEFAULT NULL,
  `deadline` varchar(45) DEFAULT NULL,
  `owner_type` varchar(200) DEFAULT NULL,
  `owner` varchar(45) DEFAULT NULL,
  `owner_contact` varchar(200) DEFAULT NULL,
  `faculty` varchar(45) DEFAULT NULL,
  `matching_status` enum('N','Y','C','D') NOT NULL DEFAULT 'N' COMMENT 'N=ยังไม่ได้จับคู่, Y=จับคู่แล้ว, C=ปิดโครงการแล้ว, D=ยกเลิกความต้องการ',
  `matched_researcher` text DEFAULT NULL COMMENT 'ชื่อนักวิจัยของ CP',
  `tech_comment` varchar(1000) DEFAULT NULL,
  `keywords` varchar(500) DEFAULT NULL,
  `Comment` varchar(1000) DEFAULT NULL,
  `create_date` datetime DEFAULT NULL,
  `update_date` datetime DEFAULT NULL,
  `create_by` varchar(45) DEFAULT NULL,
  `update_by` varchar(45) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='ตารางเก็บข้อมูลงานวิจัยต่างคณะที่ต้องการให้นักวิจัยหรืออาจารย์วิทยาลัยการคอมนำไปเป็นหัวข้อวิจัยร่วมกัน';

-- --------------------------------------------------------

--
-- Table structure for table `system_config`
--

CREATE TABLE `system_config` (
  `config_id` int(11) NOT NULL,
  `system_version` varchar(20) DEFAULT '1.0.0',
  `last_updated` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `updated_by` int(11) DEFAULT NULL,
  `current_year` varchar(250) DEFAULT NULL,
  `start_date` datetime NOT NULL,
  `end_date` datetime NOT NULL,
  `main_annoucement` int(11) DEFAULT NULL,
  `reward_announcement` int(11) DEFAULT NULL,
  `activity_support_announcement` int(11) DEFAULT NULL,
  `conference_announcement` int(11) DEFAULT NULL,
  `service_announcement` int(11) DEFAULT NULL,
  `contact_info` text DEFAULT NULL,
  `kku_report_year` varchar(50) DEFAULT NULL COMMENT 'ปีระเบียบกองทุนมหาวิทยาลัยขอนแก่น',
  `installment` int(11) DEFAULT NULL COMMENT 'เลขที่ใส่ในเอกสาร Publication Reward ในส่วน "งวดที่"',
  `max_submissions_per_year` int(11) NOT NULL DEFAULT 5 COMMENT 'จำนวนครั้งสูงสุดที่ยื่นทุนได้ต่อปี (รวม Publication + Fund Application)'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `temp_authors`
--

CREATE TABLE `temp_authors` (
  `first_name` varchar(255) DEFAULT NULL,
  `last_name` varchar(255) DEFAULT NULL,
  `scopus_id` varchar(30) DEFAULT NULL,
  `scholar_id` varchar(50) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
  `email_notification` varchar(255) DEFAULT NULL,
  `scholar_author_id` varchar(64) DEFAULT NULL,
  `scopus_id` varchar(100) DEFAULT NULL,
  `password` varchar(255) DEFAULT NULL,
  `role_id` int(11) DEFAULT NULL,
  `position_id` int(11) DEFAULT NULL,
  `date_of_employment` date DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT NULL,
  `delete_at` datetime DEFAULT NULL,
  `prefix` varchar(50) DEFAULT NULL,
  `manage_position` varchar(255) DEFAULT NULL,
  `position` varchar(255) DEFAULT NULL,
  `position_en` varchar(255) DEFAULT NULL,
  `prefix_position_en` varchar(50) DEFAULT NULL,
  `Name_en` varchar(255) DEFAULT NULL,
  `suffix_en` varchar(50) DEFAULT NULL,
  `TEL` varchar(50) DEFAULT NULL,
  `TELformat` varchar(50) DEFAULT NULL,
  `TEL_ENG` varchar(50) DEFAULT NULL,
  `manage_position_en` varchar(255) DEFAULT NULL,
  `LAB_Name` varchar(255) DEFAULT NULL,
  `Room` varchar(255) DEFAULT NULL,
  `CP_WEB_ID` varchar(255) DEFAULT NULL,
  `Is_active` char(1) DEFAULT 'A',
  `last_login_at` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Stand-in structure for view `user_innovations_view`
-- (See below for the actual view)
--
CREATE TABLE `user_innovations_view` (
`submission_id` int(11)
,`user_id` int(11)
,`submission_number` varchar(255)
,`title` varchar(255)
,`innovation_type` varchar(255)
,`registered_date` datetime
,`status_name` varchar(255)
,`year_name` varchar(255)
);

-- --------------------------------------------------------

--
-- Table structure for table `user_scholar_metrics`
--

CREATE TABLE `user_scholar_metrics` (
  `user_id` int(11) NOT NULL,
  `hindex` smallint(5) UNSIGNED DEFAULT NULL,
  `hindex5y` smallint(5) UNSIGNED DEFAULT NULL,
  `i10index` smallint(5) UNSIGNED DEFAULT NULL,
  `i10index5y` smallint(5) UNSIGNED DEFAULT NULL,
  `citedby_total` int(10) UNSIGNED DEFAULT NULL,
  `citedby_5y` int(10) UNSIGNED DEFAULT NULL,
  `cites_per_year` longtext DEFAULT NULL CHECK (json_valid(`cites_per_year`)),
  `updated_at` datetime NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `user_sessions`
--

CREATE TABLE `user_sessions` (
  `session_id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `access_token_jti` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `refresh_token` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `device_name` varchar(255) DEFAULT NULL,
  `device_type` varchar(50) DEFAULT NULL,
  `ip_address` varchar(45) DEFAULT NULL,
  `user_agent` text DEFAULT NULL,
  `last_activity` datetime DEFAULT NULL,
  `expires_at` datetime NOT NULL,
  `is_active` tinyint(1) DEFAULT 1,
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `user_tokens`
--

CREATE TABLE `user_tokens` (
  `token_id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `token_type` varchar(64) DEFAULT NULL,
  `token` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `expires_at` datetime NOT NULL,
  `is_revoked` tinyint(1) DEFAULT 0,
  `device_info` varchar(255) DEFAULT NULL,
  `ip_address` varchar(45) DEFAULT NULL,
  `user_agent` text DEFAULT NULL,
  `created_at` datetime DEFAULT current_timestamp(),
  `updated_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
-- Stand-in structure for view `v_approval_records`
-- (See below for the actual view)
--
CREATE TABLE `v_approval_records` (
`submission_id` int(11)
,`submission_number` varchar(255)
,`submission_type` enum('fund_application','publication_reward')
,`user_id` int(11)
,`applicant_name` varchar(511)
,`year_id` int(11)
,`year_th` varchar(255)
,`category_id` int(11)
,`category_name` varchar(255)
,`subcategory_id` int(11)
,`subcategory_name` varchar(255)
,`subcategory_budget_id` int(11)
,`subcategory_budget_label` mediumtext
,`status_id` int(11)
,`approved_by` int(11)
,`approved_at` datetime
,`approved_amount` decimal(37,2)
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_approval_totals_by_teacher`
-- (See below for the actual view)
--
CREATE TABLE `v_approval_totals_by_teacher` (
`user_id` int(11)
,`applicant_name` varchar(511)
,`year_id` int(11)
,`year_th` varchar(255)
,`category_id` int(11)
,`category_name` varchar(255)
,`subcategory_id` int(11)
,`subcategory_name` varchar(255)
,`subcategory_budget_id` int(11)
,`subcategory_budget_label` mediumtext
,`total_approved_amount` decimal(59,2)
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_budget_summary`
-- (See below for the actual view)
--
CREATE TABLE `v_budget_summary` (
`subcategory_id` int(11)
,`allocated_amount` decimal(15,2)
,`used_amount` decimal(37,2)
,`remaining_budget` decimal(38,2)
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_current_dept_head`
-- (See below for the actual view)
--
CREATE TABLE `v_current_dept_head` (
`head_user_id` int(11)
,`effective_from` datetime
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
,`comment` binary(0)
,`create_at` datetime
,`update_at` datetime
,`delete_at` datetime
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
,`action` enum('create','update','delete','login','logout','view','download','approve','reject','submit','review','request_revision')
,`entity_type` varchar(50)
,`entity_number` varchar(50)
,`description` text
,`ip_address` varchar(45)
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_subcategory_policy_rules`
-- (See below for the actual view)
--
CREATE TABLE `v_subcategory_policy_rules` (
`subcategory_id` int(11)
,`subcategory_budget_id` int(11)
,`max_grants` int(11)
,`max_amount_per_grant` decimal(15,2)
,`max_amount_per_year` decimal(15,2)
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_subcategory_user_usage_by_type`
-- (See below for the actual view)
--
CREATE TABLE `v_subcategory_user_usage_by_type` (
`user_id` int(11)
,`year_id` int(11)
,`subcategory_id` int(11)
,`subcategory_budget_id` int(11)
,`submission_type` enum('fund_application','publication_reward')
,`used_grants` bigint(21)
,`used_amount` decimal(37,2)
);

-- --------------------------------------------------------

--
-- Stand-in structure for view `v_subcategory_user_usage_total`
-- (See below for the actual view)
--
CREATE TABLE `v_subcategory_user_usage_total` (
`user_id` int(11)
,`year_id` int(11)
,`subcategory_id` int(11)
,`used_grants_fund` decimal(22,0)
,`used_amount_fund` decimal(37,2)
,`used_grants_pub` decimal(22,0)
,`used_amount_pub` decimal(37,2)
,`used_grants_total` decimal(22,0)
,`used_amount_total` decimal(37,2)
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
,`action` enum('create','update','delete','login','logout','view','download','approve','reject','submit','review','request_revision')
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
-- Stand-in structure for view `v_user_yearly_submission_usage`
-- (See below for the actual view)
--
CREATE TABLE `v_user_yearly_submission_usage` (
`user_id` int(11)
,`year_id` int(11)
,`used_submissions` bigint(21)
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
  ADD KEY `idx_announcements_status_priority_published` (`status`,`priority`,`published_at`),
  ADD KEY `idx_year_id` (`year_id`);

--
-- Indexes for table `announcement_assignments`
--
ALTER TABLE `announcement_assignments`
  ADD PRIMARY KEY (`assignment_id`),
  ADD KEY `idx_aa_slot_code` (`slot_code`),
  ADD KEY `idx_aa_announcement_id` (`announcement_id`),
  ADD KEY `fk_aa_changed_by` (`changed_by`),
  ADD KEY `idx_aa_active` (`end_date`),
  ADD KEY `idx_aa_effective_window` (`slot_code`,`start_date`,`end_date`);

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
-- Indexes for table `auth_identities`
--
ALTER TABLE `auth_identities`
  ADD PRIMARY KEY (`identity_id`),
  ADD UNIQUE KEY `uq_provider_subject` (`provider`,`provider_subject`),
  ADD KEY `idx_user_provider` (`user_id`,`provider`);

--
-- Indexes for table `citescore_metrics_runs`
--
ALTER TABLE `citescore_metrics_runs`
  ADD PRIMARY KEY (`id`),
  ADD KEY `idx_citescore_metrics_runs_type_started` (`run_type`,`started_at`);

--
-- Indexes for table `cp_employee`
--
ALTER TABLE `cp_employee`
  ADD PRIMARY KEY (`ID`);

--
-- Indexes for table `cp_profile`
--
ALTER TABLE `cp_profile`
  ADD PRIMARY KEY (`id`);

--
-- Indexes for table `dept_head_assignments`
--
ALTER TABLE `dept_head_assignments`
  ADD PRIMARY KEY (`assignment_id`),
  ADD KEY `fk_dha_head_user` (`head_user_id`),
  ADD KEY `fk_dha_changed_by` (`changed_by`),
  ADD KEY `fk_dha_restore_role_id` (`restore_role_id`),
  ADD KEY `idx_dha_active` (`effective_to`);

--
-- Indexes for table `document_types`
--
ALTER TABLE `document_types`
  ADD PRIMARY KEY (`document_type_id`);

--
-- Indexes for table `end_of_contract`
--
ALTER TABLE `end_of_contract`
  ADD PRIMARY KEY (`eoc_id`),
  ADD KEY `idx_eoc_order` (`display_order`);

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
  ADD KEY `main_annoucement` (`main_annoucement`),
  ADD KEY `activity_support_announcement` (`activity_support_announcement`);

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
  ADD KEY `idx_fund_forms_status_effective_expiry` (`status`),
  ADD KEY `idx_year_id` (`year_id`);

--
-- Indexes for table `fund_installment_periods`
--
ALTER TABLE `fund_installment_periods`
  ADD PRIMARY KEY (`installment_period_id`),
  ADD UNIQUE KEY `ux_period_year_installment` (`year_id`,`installment_number`,`fund_level`,`fund_keyword`),
  ADD KEY `idx_year_cutoff` (`year_id`,`cutoff_date`),
  ADD KEY `idx_fund_installment_periods_level_keyword` (`fund_level`,`fund_keyword`,`year_id`,`installment_number`),
  ADD KEY `idx_fund_installment_periods_level_keyword_cutoff` (`fund_level`,`fund_keyword`,`year_id`,`cutoff_date`);

--
-- Indexes for table `fund_subcategories`
--
ALTER TABLE `fund_subcategories`
  ADD PRIMARY KEY (`subcategory_id`),
  ADD KEY `category_id` (`category_id`),
  ADD KEY `fund_subcategorie_ibfk_2` (`year_id`),
  ADD KEY `idx_fund_subcategories_code` (`subcategory_code`);

--
-- Indexes for table `import_templates`
--
ALTER TABLE `import_templates`
  ADD PRIMARY KEY (`template_id`),
  ADD KEY `idx_template_type` (`template_type`),
  ADD KEY `idx_status` (`status`),
  ADD KEY `idx_created_by` (`created_by`),
  ADD KEY `idx_delete_at` (`delete_at`),
  ADD KEY `idx_template_type_status` (`template_type`,`status`),
  ADD KEY `idx_year_id` (`year_id`);

--
-- Indexes for table `innovations`
--
ALTER TABLE `innovations`
  ADD PRIMARY KEY (`id`),
  ADD KEY `user_id` (`user_id`);

--
-- Indexes for table `kku_people_import_runs`
--
ALTER TABLE `kku_people_import_runs`
  ADD PRIMARY KEY (`id`),
  ADD KEY `idx_kku_people_import_runs_status` (`status`),
  ADD KEY `idx_kku_people_import_runs_started_at` (`started_at`),
  ADD KEY `idx_kku_people_import_runs_deleted_at` (`deleted_at`);

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
  ADD KEY `fk_notif_submission` (`related_submission_id`),
  ADD KEY `idx_user_created_at` (`user_id`,`create_at`);

--
-- Indexes for table `notification_message`
--
ALTER TABLE `notification_message`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `uk_notification_message_event_audience` (`event_key`,`send_to`);

--
-- Indexes for table `positions`
--
ALTER TABLE `positions`
  ADD PRIMARY KEY (`position_id`);

--
-- Indexes for table `projects`
--
ALTER TABLE `projects`
  ADD PRIMARY KEY (`project_id`),
  ADD KEY `idx_proj_date` (`event_date`),
  ADD KEY `idx_proj_type` (`type_id`),
  ADD KEY `idx_proj_plan` (`plan_id`),
  ADD KEY `idx_proj_created_by` (`created_by`);

--
-- Indexes for table `project_attachments`
--
ALTER TABLE `project_attachments`
  ADD PRIMARY KEY (`file_id`),
  ADD KEY `idx_pa_project` (`project_id`),
  ADD KEY `idx_pa_public_order` (`is_public`,`display_order`),
  ADD KEY `idx_pa_uploader` (`uploaded_by`);

--
-- Indexes for table `project_budget_plans`
--
ALTER TABLE `project_budget_plans`
  ADD PRIMARY KEY (`plan_id`),
  ADD UNIQUE KEY `uq_plans_name` (`name_th`,`name_en`),
  ADD KEY `idx_plans_active_order` (`is_active`,`display_order`);

--
-- Indexes for table `project_members`
--
ALTER TABLE `project_members`
  ADD PRIMARY KEY (`member_id`),
  ADD KEY `idx_pm_project` (`project_id`),
  ADD KEY `idx_pm_user` (`user_id`),
  ADD KEY `idx_pm_order` (`project_id`,`display_order`);

--
-- Indexes for table `project_types`
--
ALTER TABLE `project_types`
  ADD PRIMARY KEY (`type_id`),
  ADD UNIQUE KEY `uq_types_name` (`name_th`,`name_en`),
  ADD KEY `idx_types_active_order` (`is_active`,`display_order`);

--
-- Indexes for table `publications`
--
ALTER TABLE `publications`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `uniq_doi` (`doi`),
  ADD UNIQUE KEY `uniq_fingerprint` (`fingerprint`),
  ADD UNIQUE KEY `ux_pub_user_doi` (`user_id`,`doi`),
  ADD UNIQUE KEY `ux_pub_user_fingerprint` (`user_id`,`fingerprint`),
  ADD KEY `idx_user_year` (`user_id`,`publication_year`);

--
-- Indexes for table `publication_reward_details`
--
ALTER TABLE `publication_reward_details`
  ADD PRIMARY KEY (`detail_id`),
  ADD UNIQUE KEY `submission_id` (`submission_id`),
  ADD UNIQUE KEY `idx_submission` (`submission_id`),
  ADD KEY `idx_publication_date` (`publication_date`),
  ADD KEY `idx_quartile` (`quartile`),
  ADD KEY `idx_prd_submission` (`submission_id`),
  ADD KEY `idx_prd_main_annoucement` (`main_annoucement`),
  ADD KEY `idx_prd_reward_announcement` (`reward_announcement`);

--
-- Indexes for table `publication_reward_external_funds`
--
ALTER TABLE `publication_reward_external_funds`
  ADD PRIMARY KEY (`external_fund_id`),
  ADD KEY `idx_pref_detail_id` (`detail_id`),
  ADD KEY `idx_pref_submission_id` (`submission_id`),
  ADD KEY `idx_pref_document_id` (`document_id`),
  ADD KEY `idx_pref_file_id` (`file_id`);

--
-- Indexes for table `publication_reward_rates`
--
ALTER TABLE `publication_reward_rates`
  ADD PRIMARY KEY (`rate_id`),
  ADD UNIQUE KEY `year_status_quartile` (`year`,`author_status`,`journal_quartile`);

--
-- Indexes for table `research_fund_admin_events`
--
ALTER TABLE `research_fund_admin_events`
  ADD PRIMARY KEY (`event_id`),
  ADD KEY `idx_rfae_submission_created_at` (`submission_id`,`created_at`),
  ADD KEY `idx_rfae_status_after_id` (`status_after_id`),
  ADD KEY `fk_rfae_created_by` (`created_by`);

--
-- Indexes for table `research_fund_event_files`
--
ALTER TABLE `research_fund_event_files`
  ADD PRIMARY KEY (`event_file_id`),
  ADD KEY `idx_rfef_event_id` (`event_id`),
  ADD KEY `idx_rfef_file_id` (`file_id`);

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
-- Indexes for table `scholar_import_runs`
--
ALTER TABLE `scholar_import_runs`
  ADD PRIMARY KEY (`id`),
  ADD KEY `idx_scholar_import_runs_status` (`status`),
  ADD KEY `idx_scholar_import_runs_started_at` (`started_at`);

--
-- Indexes for table `scopus_affiliations`
--
ALTER TABLE `scopus_affiliations`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `uq_scopus_affiliations_afid` (`afid`);

--
-- Indexes for table `scopus_api_import_jobs`
--
ALTER TABLE `scopus_api_import_jobs`
  ADD PRIMARY KEY (`id`),
  ADD KEY `idx_scopus_api_import_jobs_author` (`scopus_author_id`),
  ADD KEY `idx_scopus_api_import_jobs_status` (`status`),
  ADD KEY `idx_scopus_api_import_jobs_started` (`started_at`);

--
-- Indexes for table `scopus_api_requests`
--
ALTER TABLE `scopus_api_requests`
  ADD PRIMARY KEY (`id`),
  ADD KEY `idx_scopus_api_requests_job` (`job_id`),
  ADD KEY `idx_scopus_api_requests_status` (`response_status`),
  ADD KEY `idx_scopus_api_requests_page` (`page_start`,`page_count`);

--
-- Indexes for table `scopus_authors`
--
ALTER TABLE `scopus_authors`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `uq_scopus_authors_scopus_author_id` (`scopus_author_id`);

--
-- Indexes for table `scopus_batch_import_runs`
--
ALTER TABLE `scopus_batch_import_runs`
  ADD PRIMARY KEY (`id`),
  ADD KEY `idx_scopus_batch_import_runs_status_started` (`status`,`started_at`);

--
-- Indexes for table `scopus_config`
--
ALTER TABLE `scopus_config`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `uq_scopus_config_key` (`key`);

--
-- Indexes for table `scopus_documents`
--
ALTER TABLE `scopus_documents`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `uq_scopus_documents_eid` (`eid`),
  ADD KEY `idx_scopus_documents_cover_date` (`cover_date`),
  ADD KEY `idx_scopus_documents_doi` (`doi`),
  ADD KEY `idx_scopus_documents_source_id` (`source_id`);

--
-- Indexes for table `scopus_document_authors`
--
ALTER TABLE `scopus_document_authors`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `uq_scopus_document_authors_doc_author` (`document_id`,`author_id`),
  ADD KEY `fk_scopus_document_authors_affiliation` (`affiliation_id`),
  ADD KEY `idx_scopus_document_authors_author` (`author_id`),
  ADD KEY `idx_scopus_document_authors_document` (`document_id`,`author_seq`);

--
-- Indexes for table `scopus_source_metrics`
--
ALTER TABLE `scopus_source_metrics`
  ADD PRIMARY KEY (`source_metric_id`),
  ADD UNIQUE KEY `uq_scopus_source_year_type` (`source_id`,`metric_year`,`doc_type`),
  ADD KEY `idx_issn_year` (`issn`,`metric_year`),
  ADD KEY `idx_eissn_year` (`eissn`,`metric_year`);

--
-- Indexes for table `subcategory_budgets`
--
ALTER TABLE `subcategory_budgets`
  ADD PRIMARY KEY (`subcategory_budget_id`),
  ADD KEY `subcategories_budgets_ibfk_1` (`subcategory_id`),
  ADD KEY `idx_subcat_scope` (`subcategory_id`,`record_scope`);

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
  ADD KEY `idx_submission_user_year` (`user_id`,`year_id`),
  ADD KEY `idx_submission_category` (`category_id`),
  ADD KEY `idx_submission_subcategory` (`subcategory_id`),
  ADD KEY `idx_submission_budget` (`subcategory_budget_id`),
  ADD KEY `idx_submission_category_subcategory` (`category_id`,`subcategory_id`),
  ADD KEY `idx_submissions_approval` (`status_id`,`user_id`,`year_id`,`category_id`,`subcategory_id`,`subcategory_budget_id`,`approved_at`),
  ADD KEY `head_approved_by` (`head_approved_by`),
  ADD KEY `idx_subm_admin_approved_by` (`admin_approved_by`),
  ADD KEY `idx_subm_rejected_by` (`rejected_by`),
  ADD KEY `idx_subm_rejected_at` (`rejected_at`),
  ADD KEY `idx_head_rejected_by` (`head_rejected_by`),
  ADD KEY `idx_admin_rejected_by` (`admin_rejected_by`),
  ADD KEY `idx_admin_approved_by` (`admin_approved_by`);

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
-- Indexes for table `support_fundmapping`
--
ALTER TABLE `support_fundmapping`
  ADD PRIMARY KEY (`req_id`);

--
-- Indexes for table `system_config`
--
ALTER TABLE `system_config`
  ADD PRIMARY KEY (`config_id`),
  ADD KEY `updated_by` (`updated_by`),
  ADD KEY `main_annoucement` (`main_annoucement`),
  ADD KEY `reward_announcement` (`reward_announcement`),
  ADD KEY `activity_support_announcement` (`activity_support_announcement`),
  ADD KEY `conference_announcement` (`conference_announcement`),
  ADD KEY `service_announcement` (`service_announcement`);

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
-- Indexes for table `user_scholar_metrics`
--
ALTER TABLE `user_scholar_metrics`
  ADD PRIMARY KEY (`user_id`);

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
  MODIFY `announcement_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `announcement_assignments`
--
ALTER TABLE `announcement_assignments`
  MODIFY `assignment_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `application_status`
--
ALTER TABLE `application_status`
  MODIFY `application_status_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `audit_logs`
--
ALTER TABLE `audit_logs`
  MODIFY `log_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `auth_identities`
--
ALTER TABLE `auth_identities`
  MODIFY `identity_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `citescore_metrics_runs`
--
ALTER TABLE `citescore_metrics_runs`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `cp_profile`
--
ALTER TABLE `cp_profile`
  MODIFY `id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `dept_head_assignments`
--
ALTER TABLE `dept_head_assignments`
  MODIFY `assignment_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `document_types`
--
ALTER TABLE `document_types`
  MODIFY `document_type_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `end_of_contract`
--
ALTER TABLE `end_of_contract`
  MODIFY `eoc_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `file_uploads`
--
ALTER TABLE `file_uploads`
  MODIFY `file_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `fund_application_details`
--
ALTER TABLE `fund_application_details`
  MODIFY `detail_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `fund_categories`
--
ALTER TABLE `fund_categories`
  MODIFY `category_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `fund_forms`
--
ALTER TABLE `fund_forms`
  MODIFY `form_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `fund_installment_periods`
--
ALTER TABLE `fund_installment_periods`
  MODIFY `installment_period_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `fund_subcategories`
--
ALTER TABLE `fund_subcategories`
  MODIFY `subcategory_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `import_templates`
--
ALTER TABLE `import_templates`
  MODIFY `template_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `innovations`
--
ALTER TABLE `innovations`
  MODIFY `id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `kku_people_import_runs`
--
ALTER TABLE `kku_people_import_runs`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `notifications`
--
ALTER TABLE `notifications`
  MODIFY `notification_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `positions`
--
ALTER TABLE `positions`
  MODIFY `position_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `projects`
--
ALTER TABLE `projects`
  MODIFY `project_id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `project_attachments`
--
ALTER TABLE `project_attachments`
  MODIFY `file_id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `project_budget_plans`
--
ALTER TABLE `project_budget_plans`
  MODIFY `plan_id` tinyint(3) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `project_members`
--
ALTER TABLE `project_members`
  MODIFY `member_id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `project_types`
--
ALTER TABLE `project_types`
  MODIFY `type_id` tinyint(3) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `publications`
--
ALTER TABLE `publications`
  MODIFY `id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `publication_reward_details`
--
ALTER TABLE `publication_reward_details`
  MODIFY `detail_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `publication_reward_external_funds`
--
ALTER TABLE `publication_reward_external_funds`
  MODIFY `external_fund_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `publication_reward_rates`
--
ALTER TABLE `publication_reward_rates`
  MODIFY `rate_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `research_fund_admin_events`
--
ALTER TABLE `research_fund_admin_events`
  MODIFY `event_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `research_fund_event_files`
--
ALTER TABLE `research_fund_event_files`
  MODIFY `event_file_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `reward_config`
--
ALTER TABLE `reward_config`
  MODIFY `config_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `roles`
--
ALTER TABLE `roles`
  MODIFY `role_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scholar_import_runs`
--
ALTER TABLE `scholar_import_runs`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scopus_affiliations`
--
ALTER TABLE `scopus_affiliations`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scopus_api_import_jobs`
--
ALTER TABLE `scopus_api_import_jobs`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scopus_api_requests`
--
ALTER TABLE `scopus_api_requests`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scopus_authors`
--
ALTER TABLE `scopus_authors`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scopus_batch_import_runs`
--
ALTER TABLE `scopus_batch_import_runs`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scopus_config`
--
ALTER TABLE `scopus_config`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scopus_documents`
--
ALTER TABLE `scopus_documents`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scopus_document_authors`
--
ALTER TABLE `scopus_document_authors`
  MODIFY `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `scopus_source_metrics`
--
ALTER TABLE `scopus_source_metrics`
  MODIFY `source_metric_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `subcategory_budgets`
--
ALTER TABLE `subcategory_budgets`
  MODIFY `subcategory_budget_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `submissions`
--
ALTER TABLE `submissions`
  MODIFY `submission_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `submission_documents`
--
ALTER TABLE `submission_documents`
  MODIFY `document_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `submission_users`
--
ALTER TABLE `submission_users`
  MODIFY `id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `system_config`
--
ALTER TABLE `system_config`
  MODIFY `config_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `users`
--
ALTER TABLE `users`
  MODIFY `user_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `user_fund_eligibilities`
--
ALTER TABLE `user_fund_eligibilities`
  MODIFY `user_fund_eligibility_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `user_sessions`
--
ALTER TABLE `user_sessions`
  MODIFY `session_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `user_tokens`
--
ALTER TABLE `user_tokens`
  MODIFY `token_id` int(11) NOT NULL AUTO_INCREMENT;

--
-- AUTO_INCREMENT for table `years`
--
ALTER TABLE `years`
  MODIFY `year_id` int(11) NOT NULL AUTO_INCREMENT;

-- --------------------------------------------------------

--
-- Structure for view `user_innovations_view`
--
DROP TABLE IF EXISTS `user_innovations_view`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `user_innovations_view`  AS SELECT `s`.`submission_id` AS `submission_id`, `s`.`user_id` AS `user_id`, `s`.`submission_number` AS `submission_number`, coalesce(nullif(`fad`.`project_title`,''),`fs`.`subcategory_name`,`s`.`submission_type`) AS `title`, coalesce(`fs`.`subcategory_name`,`s`.`submission_type`) AS `innovation_type`, `s`.`submitted_at` AS `registered_date`, coalesce(`st`.`status_name`,'') AS `status_name`, `y`.`year` AS `year_name` FROM ((((`submissions` `s` left join `fund_application_details` `fad` on(`fad`.`submission_id` = `s`.`submission_id`)) left join `fund_subcategories` `fs` on(`fs`.`subcategory_id` = `s`.`subcategory_id`)) left join `years` `y` on(`y`.`year_id` = `s`.`year_id`)) left join `application_status` `st` on(`st`.`application_status_id` = `s`.`status_id`)) WHERE `s`.`deleted_at` is null AND `s`.`status_id` = 2 AND (`fs`.`subcategory_name` like '%สิทธิบัตร%' OR `fs`.`subcategory_name` like '%อนุสิทธิบัตร%') ;

-- --------------------------------------------------------

--
-- Structure for view `view_budget_summary`
--
DROP TABLE IF EXISTS `view_budget_summary`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `view_budget_summary`  AS SELECT `y`.`year` AS `year`, `fc`.`category_name` AS `category_name`, `fs`.`subcategory_name` AS `subcategory_name`, `sb`.`allocated_amount` AS `allocated_amount`, `sb`.`used_amount` AS `used_amount`, `sb`.`remaining_budget` AS `remaining_budget`, `sb`.`max_grants` AS `max_grants`, `sb`.`remaining_grant` AS `remaining_grant`, count(`fa`.`application_id`) AS `total_applications`, count(case when `fa`.`application_status_id` = 2 then 1 end) AS `approved_applications` FROM ((((`subcategory_budgets` `sb` left join `fund_subcategories` `fs` on(`sb`.`subcategory_id` = `fs`.`subcategory_id`)) left join `fund_categories` `fc` on(`fs`.`category_id` = `fc`.`category_id`)) left join `years` `y` on(`fc`.`year_id` = `y`.`year_id`)) left join `v_fund_applications` `fa` on(`fs`.`subcategory_id` = `fa`.`subcategory_id` and `fa`.`delete_at` is null)) WHERE `sb`.`delete_at` is null GROUP BY `sb`.`subcategory_budget_id`, `y`.`year`, `fc`.`category_name`, `fs`.`subcategory_name`, `sb`.`allocated_amount`, `sb`.`used_amount`, `sb`.`remaining_budget`, `sb`.`max_grants`, `sb`.`remaining_grant` ;

-- --------------------------------------------------------

--
-- Structure for view `view_fund_applications_summary`
--
DROP TABLE IF EXISTS `view_fund_applications_summary`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `view_fund_applications_summary`  AS SELECT `fa`.`application_id` AS `application_id`, `fa`.`application_number` AS `application_number`, `fa`.`project_title` AS `project_title`, concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `applicant_name`, `u`.`email` AS `email`, `p`.`position_name` AS `position_name`, `fc`.`category_name` AS `category_name`, `fs`.`subcategory_name` AS `subcategory_name`, `y`.`year` AS `year`, `ast`.`status_name` AS `status_name`, `fa`.`requested_amount` AS `requested_amount`, `fa`.`approved_amount` AS `approved_amount`, `fa`.`submitted_at` AS `submitted_at`, `fa`.`approved_at` AS `approved_at` FROM ((((((`v_fund_applications` `fa` left join `users` `u` on(`fa`.`user_id` = `u`.`user_id`)) left join `positions` `p` on(`u`.`position_id` = `p`.`position_id`)) left join `fund_subcategories` `fs` on(`fa`.`subcategory_id` = `fs`.`subcategory_id`)) left join `fund_categories` `fc` on(`fs`.`category_id` = `fc`.`category_id`)) left join `years` `y` on(`fa`.`year_id` = `y`.`year_id`)) left join `application_status` `ast` on(`fa`.`application_status_id` = `ast`.`application_status_id`)) WHERE `fa`.`delete_at` is null ;

-- --------------------------------------------------------

--
-- Structure for view `v_active_reward_config`
--
DROP TABLE IF EXISTS `v_active_reward_config`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_active_reward_config`  AS SELECT `reward_config`.`config_id` AS `config_id`, `reward_config`.`year` AS `year`, `reward_config`.`journal_quartile` AS `journal_quartile`, `reward_config`.`max_amount` AS `max_amount`, `reward_config`.`condition_description` AS `condition_description`, `reward_config`.`create_at` AS `create_at`, `reward_config`.`update_at` AS `update_at` FROM `reward_config` WHERE `reward_config`.`is_active` = 1 AND `reward_config`.`delete_at` is null ORDER BY `reward_config`.`year` DESC, `reward_config`.`journal_quartile` ASC ;

-- --------------------------------------------------------

--
-- Structure for view `v_approval_records`
--
DROP TABLE IF EXISTS `v_approval_records`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_approval_records`  AS SELECT `s`.`submission_id` AS `submission_id`, `s`.`submission_number` AS `submission_number`, `s`.`submission_type` AS `submission_type`, `s`.`user_id` AS `user_id`, concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `applicant_name`, `s`.`year_id` AS `year_id`, `y`.`year` AS `year_th`, `s`.`category_id` AS `category_id`, `fc`.`category_name` AS `category_name`, `s`.`subcategory_id` AS `subcategory_id`, `fsc`.`subcategory_name` AS `subcategory_name`, `s`.`subcategory_budget_id` AS `subcategory_budget_id`, coalesce(nullif(trim(`sb`.`fund_description`),''),nullif(concat('ระดับ ',`sb`.`level`),'ระดับ '),concat('งบ #',`sb`.`subcategory_budget_id`)) AS `subcategory_budget_label`, `s`.`status_id` AS `status_id`, `s`.`approved_by` AS `approved_by`, `s`.`approved_at` AS `approved_at`, CASE WHEN `s`.`submission_type` = 'publication_reward' THEN coalesce(`prd`.`total_approve_amount`,coalesce(`prd`.`reward_approve_amount`,0) + coalesce(`prd`.`revision_fee_approve_amount`,0) + coalesce(`prd`.`publication_fee_approve_amount`,0),0) WHEN `s`.`submission_type` = 'fund_application' THEN coalesce(`fa`.`total_approved_amount`,0) ELSE 0 END AS `approved_amount` FROM (((((((`submissions` `s` join `users` `u` on(`u`.`user_id` = `s`.`user_id` and (`u`.`delete_at` is null or `u`.`delete_at` = 0))) join `years` `y` on(`y`.`year_id` = `s`.`year_id`)) left join `fund_categories` `fc` on(`fc`.`category_id` = `s`.`category_id`)) left join `fund_subcategories` `fsc` on(`fsc`.`subcategory_id` = `s`.`subcategory_id`)) left join `subcategory_budgets` `sb` on(`sb`.`subcategory_budget_id` = `s`.`subcategory_budget_id`)) left join `publication_reward_details` `prd` on(`prd`.`submission_id` = `s`.`submission_id`)) left join (select `fund_application_details`.`submission_id` AS `submission_id`,sum(coalesce(`fund_application_details`.`approved_amount`,0)) AS `total_approved_amount` from `fund_application_details` group by `fund_application_details`.`submission_id`) `fa` on(`fa`.`submission_id` = `s`.`submission_id`)) WHERE `s`.`status_id` = 2 AND `s`.`deleted_at` is null ;

-- --------------------------------------------------------

--
-- Structure for view `v_approval_totals_by_teacher`
--
DROP TABLE IF EXISTS `v_approval_totals_by_teacher`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_approval_totals_by_teacher`  AS SELECT `r`.`user_id` AS `user_id`, `r`.`applicant_name` AS `applicant_name`, `r`.`year_id` AS `year_id`, `r`.`year_th` AS `year_th`, `r`.`category_id` AS `category_id`, `r`.`category_name` AS `category_name`, `r`.`subcategory_id` AS `subcategory_id`, `r`.`subcategory_name` AS `subcategory_name`, `r`.`subcategory_budget_id` AS `subcategory_budget_id`, `r`.`subcategory_budget_label` AS `subcategory_budget_label`, sum(`r`.`approved_amount`) AS `total_approved_amount` FROM `v_approval_records` AS `r` GROUP BY `r`.`user_id`, `r`.`year_id`, `r`.`category_id`, `r`.`subcategory_id`, `r`.`subcategory_budget_id` ;

-- --------------------------------------------------------

--
-- Structure for view `v_budget_summary`
--
DROP TABLE IF EXISTS `v_budget_summary`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_budget_summary`  AS SELECT `sb`.`subcategory_id` AS `subcategory_id`, `sb`.`allocated_amount` AS `allocated_amount`, coalesce(sum(case when `s`.`submission_type` = 'fund_application' then ifnull(`fad`.`approved_amount`,0) when `s`.`submission_type` = 'publication_reward' then ifnull(`prd`.`total_approve_amount`,0) else 0 end),0) AS `used_amount`, `sb`.`allocated_amount`- coalesce(sum(case when `s`.`submission_type` = 'fund_application' then ifnull(`fad`.`approved_amount`,0) when `s`.`submission_type` = 'publication_reward' then ifnull(`prd`.`total_approve_amount`,0) else 0 end),0) AS `remaining_budget` FROM (((`subcategory_budgets` `sb` left join `submissions` `s` on(`s`.`subcategory_id` = `sb`.`subcategory_id` and `s`.`status_id` = 2)) left join `fund_application_details` `fad` on(`fad`.`submission_id` = `s`.`submission_id`)) left join `publication_reward_details` `prd` on(`prd`.`submission_id` = `s`.`submission_id`)) WHERE `sb`.`record_scope` = 'overall' GROUP BY `sb`.`subcategory_id`, `sb`.`allocated_amount` ;

-- --------------------------------------------------------

--
-- Structure for view `v_current_dept_head`
--
DROP TABLE IF EXISTS `v_current_dept_head`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_current_dept_head`  AS SELECT `dept_head_assignments`.`head_user_id` AS `head_user_id`, `dept_head_assignments`.`effective_from` AS `effective_from` FROM `dept_head_assignments` WHERE `dept_head_assignments`.`effective_to` is null ;

-- --------------------------------------------------------

--
-- Structure for view `v_file_uploads_readable`
--
DROP TABLE IF EXISTS `v_file_uploads_readable`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_file_uploads_readable`  AS SELECT `f`.`file_id` AS `file_id`, `f`.`original_name` AS `original_name`, `f`.`stored_path` AS `stored_path`, `f`.`folder_type` AS `folder_type`, `f`.`submission_id` AS `submission_id`, `f`.`file_size` AS `file_size`, `f`.`mime_type` AS `mime_type`, `f`.`file_hash` AS `file_hash`, `f`.`is_public` AS `is_public`, `f`.`uploaded_by` AS `uploaded_by`, `f`.`uploaded_at` AS `uploaded_at`, `f`.`create_at` AS `create_at`, `f`.`update_at` AS `update_at`, `f`.`delete_at` AS `delete_at`, `u`.`user_fname` AS `user_fname`, `u`.`user_lname` AS `user_lname`, concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `uploader_name`, CASE WHEN `f`.`stored_path` like '%/users/%' THEN substring_index(substring_index(`f`.`stored_path`,'/users/',-1),'/',1) ELSE 'unknown' END AS `user_folder`, CASE `f`.`folder_type` WHEN 'temp' THEN 'Temporary Files' WHEN 'submission' THEN 'Submission Files' WHEN 'profile' THEN 'Profile Files' ELSE 'Other Files' END AS `folder_type_name` FROM (`file_uploads` `f` left join `users` `u` on(`f`.`uploaded_by` = `u`.`user_id`)) WHERE `f`.`delete_at` is null ;

-- --------------------------------------------------------

--
-- Structure for view `v_file_usage_stats`
--
DROP TABLE IF EXISTS `v_file_usage_stats`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_file_usage_stats`  AS SELECT `u`.`user_id` AS `user_id`, concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `user_name`, `u`.`email` AS `email`, count(`f`.`file_id`) AS `total_files`, sum(`f`.`file_size`) AS `total_size`, avg(`f`.`file_size`) AS `avg_file_size`, count(case when `f`.`folder_type` = 'temp' then 1 end) AS `temp_files`, count(case when `f`.`folder_type` = 'submission' then 1 end) AS `submission_files`, count(case when `f`.`folder_type` = 'profile' then 1 end) AS `profile_files`, max(`f`.`uploaded_at`) AS `last_upload` FROM (`users` `u` left join `file_uploads` `f` on(`u`.`user_id` = `f`.`uploaded_by` and `f`.`delete_at` is null)) WHERE `u`.`delete_at` is null GROUP BY `u`.`user_id`, `u`.`user_fname`, `u`.`user_lname`, `u`.`email` ORDER BY count(`f`.`file_id`) DESC ;

-- --------------------------------------------------------

--
-- Structure for view `v_fund_applications`
--
DROP TABLE IF EXISTS `v_fund_applications`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_fund_applications`  AS SELECT `s`.`submission_id` AS `application_id`, `s`.`submission_number` AS `application_number`, `s`.`user_id` AS `user_id`, `s`.`year_id` AS `year_id`, `fad`.`subcategory_id` AS `subcategory_id`, `s`.`status_id` AS `application_status_id`, `s`.`approved_by` AS `approved_by`, `fad`.`project_title` AS `project_title`, `fad`.`project_description` AS `project_description`, `fad`.`requested_amount` AS `requested_amount`, `fad`.`approved_amount` AS `approved_amount`, `s`.`submitted_at` AS `submitted_at`, `s`.`approved_at` AS `approved_at`, `fad`.`closed_at` AS `closed_at`, NULL AS `comment`, `s`.`created_at` AS `create_at`, `s`.`updated_at` AS `update_at`, `s`.`deleted_at` AS `delete_at` FROM (`submissions` `s` join `fund_application_details` `fad` on(`s`.`submission_id` = `fad`.`submission_id`)) WHERE `s`.`submission_type` = 'fund_application' ;

-- --------------------------------------------------------

--
-- Structure for view `v_publication_rewards`
--
DROP TABLE IF EXISTS `v_publication_rewards`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_publication_rewards`  AS SELECT `s`.`submission_id` AS `reward_id`, `s`.`submission_number` AS `reward_number`, `s`.`user_id` AS `user_id`, `prd`.`paper_title` AS `paper_title`, `prd`.`journal_name` AS `journal_name`, `prd`.`publication_date` AS `publication_date`, `prd`.`quartile` AS `journal_quartile`, `prd`.`doi` AS `doi`, `prd`.`reward_amount` AS `reward_amount`, `s`.`status_id` AS `status_id`, `s`.`submitted_at` AS `submitted_at`, `s`.`created_at` AS `created_at`, `s`.`updated_at` AS `updated_at`, `s`.`deleted_at` AS `deleted_at` FROM (`submissions` `s` join `publication_reward_details` `prd` on(`s`.`submission_id` = `prd`.`submission_id`)) WHERE `s`.`submission_type` = 'publication_reward' ;

-- --------------------------------------------------------

--
-- Structure for view `v_recent_audit_logs`
--
DROP TABLE IF EXISTS `v_recent_audit_logs`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_recent_audit_logs`  AS SELECT `al`.`log_id` AS `log_id`, `al`.`created_at` AS `created_at`, concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `user_name`, `al`.`action` AS `action`, `al`.`entity_type` AS `entity_type`, `al`.`entity_number` AS `entity_number`, `al`.`description` AS `description`, `al`.`ip_address` AS `ip_address` FROM (`audit_logs` `al` left join `users` `u` on(`al`.`user_id` = `u`.`user_id`)) ORDER BY `al`.`created_at` DESC LIMIT 0, 100 ;

-- --------------------------------------------------------

--
-- Structure for view `v_subcategory_policy_rules`
--
DROP TABLE IF EXISTS `v_subcategory_policy_rules`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_subcategory_policy_rules`  AS SELECT `r`.`subcategory_id` AS `subcategory_id`, `r`.`subcategory_budget_id` AS `subcategory_budget_id`, `r`.`max_grants` AS `max_grants`, `r`.`max_amount_per_grant` AS `max_amount_per_grant`, `r`.`max_amount_per_year` AS `max_amount_per_year` FROM `subcategory_budgets` AS `r` WHERE `r`.`record_scope` = 'rule'union all select `o`.`subcategory_id` AS `subcategory_id`,`o`.`subcategory_budget_id` AS `subcategory_budget_id`,`o`.`max_grants` AS `max_grants`,`o`.`max_amount_per_grant` AS `max_amount_per_grant`,`o`.`max_amount_per_year` AS `max_amount_per_year` from `subcategory_budgets` `o` where `o`.`record_scope` = 'overall' and !exists(select 1 from `subcategory_budgets` `rr` where `rr`.`subcategory_id` = `o`.`subcategory_id` and `rr`.`record_scope` = 'rule' limit 1)  ;

-- --------------------------------------------------------

--
-- Structure for view `v_subcategory_user_usage_by_type`
--
DROP TABLE IF EXISTS `v_subcategory_user_usage_by_type`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_subcategory_user_usage_by_type`  AS SELECT `s`.`user_id` AS `user_id`, `s`.`year_id` AS `year_id`, `s`.`subcategory_id` AS `subcategory_id`, `s`.`subcategory_budget_id` AS `subcategory_budget_id`, `s`.`submission_type` AS `submission_type`, count(0) AS `used_grants`, sum(case when `s`.`submission_type` = 'fund_application' then ifnull(`fad`.`approved_amount`,0) when `s`.`submission_type` = 'publication_reward' then ifnull(`prd`.`total_approve_amount`,0) else 0 end) AS `used_amount` FROM ((`submissions` `s` left join `fund_application_details` `fad` on(`fad`.`submission_id` = `s`.`submission_id`)) left join `publication_reward_details` `prd` on(`prd`.`submission_id` = `s`.`submission_id`)) WHERE `s`.`status_id` = 2 AND `s`.`submission_type` in ('fund_application','publication_reward') GROUP BY `s`.`user_id`, `s`.`year_id`, `s`.`subcategory_id`, `s`.`subcategory_budget_id`, `s`.`submission_type` ;

-- --------------------------------------------------------

--
-- Structure for view `v_subcategory_user_usage_total`
--
DROP TABLE IF EXISTS `v_subcategory_user_usage_total`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_subcategory_user_usage_total`  AS SELECT `s`.`user_id` AS `user_id`, `s`.`year_id` AS `year_id`, `s`.`subcategory_id` AS `subcategory_id`, sum(case when `s`.`submission_type` = 'fund_application' then 1 else 0 end) AS `used_grants_fund`, sum(case when `s`.`submission_type` = 'fund_application' then ifnull(`fad`.`approved_amount`,0) else 0 end) AS `used_amount_fund`, sum(case when `s`.`submission_type` = 'publication_reward' then 1 else 0 end) AS `used_grants_pub`, sum(case when `s`.`submission_type` = 'publication_reward' then ifnull(`prd`.`total_approve_amount`,0) else 0 end) AS `used_amount_pub`, sum(case when `s`.`submission_type` in ('fund_application','publication_reward') then 1 else 0 end) AS `used_grants_total`, sum(case when `s`.`submission_type` = 'fund_application' then ifnull(`fad`.`approved_amount`,0) when `s`.`submission_type` = 'publication_reward' then ifnull(`prd`.`total_approve_amount`,0) else 0 end) AS `used_amount_total` FROM ((`submissions` `s` left join `fund_application_details` `fad` on(`fad`.`submission_id` = `s`.`submission_id`)) left join `publication_reward_details` `prd` on(`prd`.`submission_id` = `s`.`submission_id`)) WHERE `s`.`status_id` = 2 AND `s`.`submission_type` in ('fund_application','publication_reward') GROUP BY `s`.`user_id`, `s`.`year_id`, `s`.`subcategory_id` ;

-- --------------------------------------------------------

--
-- Structure for view `v_submission_audit_trail`
--
DROP TABLE IF EXISTS `v_submission_audit_trail`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_submission_audit_trail`  AS SELECT `s`.`submission_number` AS `submission_number`, `s`.`submission_type` AS `submission_type`, `al`.`created_at` AS `created_at`, concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `action_by`, `al`.`action` AS `action`, `al`.`changed_fields` AS `changed_fields`, `al`.`description` AS `description` FROM ((`submissions` `s` join `audit_logs` `al` on(`al`.`entity_type` = 'submission' and `al`.`entity_id` = `s`.`submission_id`)) left join `users` `u` on(`al`.`user_id` = `u`.`user_id`)) ORDER BY `s`.`submission_id` ASC, `al`.`created_at` ASC ;

-- --------------------------------------------------------

--
-- Structure for view `v_user_activity_summary`
--
DROP TABLE IF EXISTS `v_user_activity_summary`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_user_activity_summary`  AS SELECT `u`.`user_id` AS `user_id`, concat(`u`.`user_fname`,' ',`u`.`user_lname`) AS `user_name`, count(case when `al`.`action` = 'login' then 1 end) AS `login_count`, count(case when `al`.`action` = 'create' then 1 end) AS `create_count`, count(case when `al`.`action` = 'update' then 1 end) AS `update_count`, count(case when `al`.`action` = 'download' then 1 end) AS `download_count`, max(case when `al`.`action` = 'login' then `al`.`created_at` end) AS `last_login`, count(0) AS `total_actions` FROM (`users` `u` left join `audit_logs` `al` on(`u`.`user_id` = `al`.`user_id`)) GROUP BY `u`.`user_id` ;

-- --------------------------------------------------------

--
-- Structure for view `v_user_yearly_submission_usage`
--
DROP TABLE IF EXISTS `v_user_yearly_submission_usage`;

CREATE ALGORITHM=UNDEFINED DEFINER=`drnadech_funddev`@`%` SQL SECURITY DEFINER VIEW `v_user_yearly_submission_usage`  AS SELECT `s`.`user_id` AS `user_id`, `s`.`year_id` AS `year_id`, count(0) AS `used_submissions` FROM (`submissions` `s` join `application_status` `st` on(`st`.`application_status_id` = `s`.`status_id`)) WHERE `s`.`deleted_at` is null AND `s`.`submission_type` in ('fund_application','publication_reward') AND `st`.`status_code` not in ('2','4') GROUP BY `s`.`user_id`, `s`.`year_id` ;

--
-- Constraints for dumped tables
--

--
-- Constraints for table `announcements`
--
ALTER TABLE `announcements`
  ADD CONSTRAINT `fk_announcements_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_announcements_year` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`);

--
-- Constraints for table `announcement_assignments`
--
ALTER TABLE `announcement_assignments`
  ADD CONSTRAINT `fk_aa_announcement` FOREIGN KEY (`announcement_id`) REFERENCES `announcements` (`announcement_id`) ON DELETE SET NULL ON UPDATE CASCADE,
  ADD CONSTRAINT `fk_aa_changed_by` FOREIGN KEY (`changed_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL ON UPDATE CASCADE;

--
-- Constraints for table `audit_logs`
--
ALTER TABLE `audit_logs`
  ADD CONSTRAINT `fk_audit_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `auth_identities`
--
ALTER TABLE `auth_identities`
  ADD CONSTRAINT `fk_auth_identities_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `dept_head_assignments`
--
ALTER TABLE `dept_head_assignments`
  ADD CONSTRAINT `fk_dha_changed_by` FOREIGN KEY (`changed_by`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_dha_head_user` FOREIGN KEY (`head_user_id`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_dha_restore_role_id` FOREIGN KEY (`restore_role_id`) REFERENCES `roles` (`role_id`);

--
-- Constraints for table `file_uploads`
--
ALTER TABLE `file_uploads`
  ADD CONSTRAINT `fk_file_uploads_user` FOREIGN KEY (`uploaded_by`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `fund_application_details`
--
ALTER TABLE `fund_application_details`
  ADD CONSTRAINT `fk_fund_detail_subcategory` FOREIGN KEY (`subcategory_id`) REFERENCES `fund_subcategories` (`subcategory_id`),
  ADD CONSTRAINT `fk_fund_detail_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`);

--
-- Constraints for table `fund_categories`
--
ALTER TABLE `fund_categories`
  ADD CONSTRAINT `fund_categories_ibfk_1` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`);

--
-- Constraints for table `fund_forms`
--
ALTER TABLE `fund_forms`
  ADD CONSTRAINT `fk_fund_forms_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_fund_forms_year` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`);

--
-- Constraints for table `fund_installment_periods`
--
ALTER TABLE `fund_installment_periods`
  ADD CONSTRAINT `fk_fip_year` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`);

--
-- Constraints for table `fund_subcategories`
--
ALTER TABLE `fund_subcategories`
  ADD CONSTRAINT `fund_subcategorie_ibfk_2` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`),
  ADD CONSTRAINT `fund_subcategories_ibfk_1` FOREIGN KEY (`category_id`) REFERENCES `fund_categories` (`category_id`);

--
-- Constraints for table `import_templates`
--
ALTER TABLE `import_templates`
  ADD CONSTRAINT `fk_import_templates_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_import_templates_year` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`);

--
-- Constraints for table `innovations`
--
ALTER TABLE `innovations`
  ADD CONSTRAINT `innovations_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `notifications`
--
ALTER TABLE `notifications`
  ADD CONSTRAINT `fk_notif_submission` FOREIGN KEY (`related_submission_id`) REFERENCES `submissions` (`submission_id`),
  ADD CONSTRAINT `fk_notifications_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `projects`
--
ALTER TABLE `projects`
  ADD CONSTRAINT `fk_projects_plan` FOREIGN KEY (`plan_id`) REFERENCES `project_budget_plans` (`plan_id`),
  ADD CONSTRAINT `fk_projects_type` FOREIGN KEY (`type_id`) REFERENCES `project_types` (`type_id`);

--
-- Constraints for table `project_attachments`
--
ALTER TABLE `project_attachments`
  ADD CONSTRAINT `fk_pa_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`project_id`) ON DELETE CASCADE;

--
-- Constraints for table `project_members`
--
ALTER TABLE `project_members`
  ADD CONSTRAINT `fk_pm_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`project_id`) ON DELETE CASCADE;

--
-- Constraints for table `publications`
--
ALTER TABLE `publications`
  ADD CONSTRAINT `publications_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `publication_reward_details`
--
ALTER TABLE `publication_reward_details`
  ADD CONSTRAINT `fk_prd_main_announcement` FOREIGN KEY (`main_annoucement`) REFERENCES `announcements` (`announcement_id`) ON DELETE SET NULL ON UPDATE CASCADE,
  ADD CONSTRAINT `fk_prd_reward_announcement` FOREIGN KEY (`reward_announcement`) REFERENCES `announcements` (`announcement_id`) ON DELETE SET NULL ON UPDATE CASCADE,
  ADD CONSTRAINT `fk_pub_detail_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`);

--
-- Constraints for table `research_fund_admin_events`
--
ALTER TABLE `research_fund_admin_events`
  ADD CONSTRAINT `fk_rfae_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_rfae_status_after` FOREIGN KEY (`status_after_id`) REFERENCES `application_status` (`application_status_id`),
  ADD CONSTRAINT `fk_rfae_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`) ON DELETE CASCADE;

--
-- Constraints for table `research_fund_event_files`
--
ALTER TABLE `research_fund_event_files`
  ADD CONSTRAINT `fk_rfef_event` FOREIGN KEY (`event_id`) REFERENCES `research_fund_admin_events` (`event_id`) ON DELETE CASCADE,
  ADD CONSTRAINT `fk_rfef_file` FOREIGN KEY (`file_id`) REFERENCES `file_uploads` (`file_id`) ON DELETE CASCADE;

--
-- Constraints for table `scopus_api_requests`
--
ALTER TABLE `scopus_api_requests`
  ADD CONSTRAINT `fk_scopus_api_requests_job` FOREIGN KEY (`job_id`) REFERENCES `scopus_api_import_jobs` (`id`) ON DELETE CASCADE;

--
-- Constraints for table `scopus_document_authors`
--
ALTER TABLE `scopus_document_authors`
  ADD CONSTRAINT `fk_scopus_document_authors_affiliation` FOREIGN KEY (`affiliation_id`) REFERENCES `scopus_affiliations` (`id`) ON DELETE SET NULL,
  ADD CONSTRAINT `fk_scopus_document_authors_author` FOREIGN KEY (`author_id`) REFERENCES `scopus_authors` (`id`) ON DELETE CASCADE,
  ADD CONSTRAINT `fk_scopus_document_authors_document` FOREIGN KEY (`document_id`) REFERENCES `scopus_documents` (`id`) ON DELETE CASCADE;

--
-- Constraints for table `subcategory_budgets`
--
ALTER TABLE `subcategory_budgets`
  ADD CONSTRAINT `subcategories_budgets_ibfk_1` FOREIGN KEY (`subcategory_id`) REFERENCES `fund_subcategories` (`subcategory_id`);

--
-- Constraints for table `submissions`
--
ALTER TABLE `submissions`
  ADD CONSTRAINT `fk_subm_admin_approved_by` FOREIGN KEY (`admin_approved_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  ADD CONSTRAINT `fk_subm_admin_approved_by_v2` FOREIGN KEY (`admin_approved_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  ADD CONSTRAINT `fk_subm_admin_rejected_by_v2` FOREIGN KEY (`admin_rejected_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  ADD CONSTRAINT `fk_subm_head_rejected_by_v2` FOREIGN KEY (`head_rejected_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  ADD CONSTRAINT `fk_subm_rejected_by` FOREIGN KEY (`rejected_by`) REFERENCES `users` (`user_id`) ON DELETE SET NULL,
  ADD CONSTRAINT `fk_submission_approver` FOREIGN KEY (`approved_by`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_submission_category` FOREIGN KEY (`category_id`) REFERENCES `fund_categories` (`category_id`) ON DELETE SET NULL ON UPDATE CASCADE,
  ADD CONSTRAINT `fk_submission_status` FOREIGN KEY (`status_id`) REFERENCES `application_status` (`application_status_id`),
  ADD CONSTRAINT `fk_submission_subcategory` FOREIGN KEY (`subcategory_id`) REFERENCES `fund_subcategories` (`subcategory_id`) ON DELETE SET NULL ON UPDATE CASCADE,
  ADD CONSTRAINT `fk_submission_subcategory_budget` FOREIGN KEY (`subcategory_budget_id`) REFERENCES `subcategory_budgets` (`subcategory_budget_id`) ON DELETE SET NULL ON UPDATE CASCADE,
  ADD CONSTRAINT `fk_submission_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `fk_submission_year` FOREIGN KEY (`year_id`) REFERENCES `years` (`year_id`),
  ADD CONSTRAINT `submissions_ibfk_1` FOREIGN KEY (`head_approved_by`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `submissions_ibfk_2` FOREIGN KEY (`approved_by`) REFERENCES `users` (`user_id`);

--
-- Constraints for table `submission_documents`
--
ALTER TABLE `submission_documents`
  ADD CONSTRAINT `fk_doc_file` FOREIGN KEY (`file_id`) REFERENCES `file_uploads` (`file_id`),
  ADD CONSTRAINT `fk_doc_submission` FOREIGN KEY (`submission_id`) REFERENCES `submissions` (`submission_id`),
  ADD CONSTRAINT `fk_doc_type` FOREIGN KEY (`document_type_id`) REFERENCES `document_types` (`document_type_id`),
  ADD CONSTRAINT `fk_doc_verifier` FOREIGN KEY (`verified_by`) REFERENCES `users` (`user_id`);

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
  ADD CONSTRAINT `system_config_ibfk_1` FOREIGN KEY (`updated_by`) REFERENCES `users` (`user_id`),
  ADD CONSTRAINT `system_config_ibfk_2` FOREIGN KEY (`main_annoucement`) REFERENCES `announcements` (`announcement_id`),
  ADD CONSTRAINT `system_config_ibfk_3` FOREIGN KEY (`reward_announcement`) REFERENCES `announcements` (`announcement_id`),
  ADD CONSTRAINT `system_config_ibfk_4` FOREIGN KEY (`activity_support_announcement`) REFERENCES `announcements` (`announcement_id`),
  ADD CONSTRAINT `system_config_ibfk_5` FOREIGN KEY (`conference_announcement`) REFERENCES `announcements` (`announcement_id`),
  ADD CONSTRAINT `system_config_ibfk_6` FOREIGN KEY (`service_announcement`) REFERENCES `announcements` (`announcement_id`);

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
-- Constraints for table `user_scholar_metrics`
--
ALTER TABLE `user_scholar_metrics`
  ADD CONSTRAINT `fk_user_scholar_metrics_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`) ON DELETE CASCADE;

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
