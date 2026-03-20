-- Permission system (RBAC + user overrides)

CREATE TABLE IF NOT EXISTS `permissions` (
  `permission_id` int(11) NOT NULL AUTO_INCREMENT,
  `code` varchar(128) NOT NULL,
  `resource` varchar(64) DEFAULT NULL,
  `action` varchar(64) DEFAULT NULL,
  `description` varchar(255) DEFAULT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL,
  PRIMARY KEY (`permission_id`),
  UNIQUE KEY `uk_permissions_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `role_permissions` (
  `role_permission_id` int(11) NOT NULL AUTO_INCREMENT,
  `role_id` int(11) NOT NULL,
  `permission_id` int(11) NOT NULL,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL,
  PRIMARY KEY (`role_permission_id`),
  UNIQUE KEY `uk_role_permission` (`role_id`,`permission_id`),
  KEY `idx_role_permissions_permission` (`permission_id`),
  CONSTRAINT `fk_role_permissions_role` FOREIGN KEY (`role_id`) REFERENCES `roles` (`role_id`),
  CONSTRAINT `fk_role_permissions_permission` FOREIGN KEY (`permission_id`) REFERENCES `permissions` (`permission_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user_roles` (
  `user_role_id` int(11) NOT NULL AUTO_INCREMENT,
  `user_id` int(11) NOT NULL,
  `role_id` int(11) NOT NULL,
  `is_primary` tinyint(1) DEFAULT 0,
  `is_active` tinyint(1) DEFAULT 1,
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL,
  PRIMARY KEY (`user_role_id`),
  UNIQUE KEY `uk_user_role` (`user_id`,`role_id`),
  KEY `idx_user_roles_role` (`role_id`),
  CONSTRAINT `fk_user_roles_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`),
  CONSTRAINT `fk_user_roles_role` FOREIGN KEY (`role_id`) REFERENCES `roles` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user_permissions` (
  `user_permission_id` int(11) NOT NULL AUTO_INCREMENT,
  `user_id` int(11) NOT NULL,
  `permission_id` int(11) NOT NULL,
  `effect` enum('allow','deny') NOT NULL DEFAULT 'allow',
  `create_at` datetime DEFAULT current_timestamp(),
  `update_at` datetime DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  `delete_at` datetime DEFAULT NULL,
  PRIMARY KEY (`user_permission_id`),
  UNIQUE KEY `uk_user_permission` (`user_id`,`permission_id`),
  KEY `idx_user_permissions_permission` (`permission_id`),
  CONSTRAINT `fk_user_permissions_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`),
  CONSTRAINT `fk_user_permissions_permission` FOREIGN KEY (`permission_id`) REFERENCES `permissions` (`permission_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `permissions` (`code`, `resource`, `action`, `description`)
VALUES
  ('dashboard.view.self', 'dashboard', 'view_self', 'View self dashboard'),
  ('dashboard.view.admin', 'dashboard', 'view_admin', 'View admin dashboard'),
  ('report.export', 'report', 'export', 'Export report data'),
  ('fund.request.create', 'fund_request', 'create', 'Create fund request'),
  ('fund.request.approve', 'fund_request', 'approve', 'Approve fund request'),
  ('submission.read.own', 'submission', 'read_own', 'Read own submissions'),
  ('submission.read.department', 'submission', 'read_department', 'Read department submissions'),
  ('submission.read.all', 'submission', 'read_all', 'Read all submissions'),
  ('scopus.publications.read', 'scopus_publications', 'read', 'Read Scopus publications'),
  ('scopus.publications.read_by_user', 'scopus_publications', 'read_by_user', 'Read Scopus publications grouped by user'),
  ('scopus.publications.export', 'scopus_publications', 'export', 'Export Scopus publications'),
  ('scopus.publications.export_by_user', 'scopus_publications', 'export_by_user', 'Export Scopus publications grouped by user')
ON DUPLICATE KEY UPDATE
  `resource` = VALUES(`resource`),
  `action` = VALUES(`action`),
  `description` = VALUES(`description`),
  `update_at` = current_timestamp();

INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT m.role_id, p.permission_id
FROM (
  SELECT 1 AS role_id, 'dashboard.view.self' AS code UNION ALL
  SELECT 1, 'fund.request.create' UNION ALL
  SELECT 1, 'submission.read.own' UNION ALL
  SELECT 2, 'dashboard.view.self' UNION ALL
  SELECT 2, 'submission.read.own' UNION ALL
  SELECT 3, 'dashboard.view.admin' UNION ALL
  SELECT 3, 'report.export' UNION ALL
  SELECT 3, 'fund.request.create' UNION ALL
  SELECT 3, 'fund.request.approve' UNION ALL
  SELECT 3, 'submission.read.all' UNION ALL
  SELECT 3, 'scopus.publications.read' UNION ALL
  SELECT 3, 'scopus.publications.read_by_user' UNION ALL
  SELECT 3, 'scopus.publications.export' UNION ALL
  SELECT 3, 'scopus.publications.export_by_user' UNION ALL
  SELECT 4, 'dashboard.view.admin' UNION ALL
  SELECT 4, 'submission.read.department' UNION ALL
  SELECT 5, 'dashboard.view.admin' UNION ALL
  SELECT 5, 'submission.read.all'
) AS m
INNER JOIN `permissions` p ON p.code = m.code
ON DUPLICATE KEY UPDATE `update_at` = current_timestamp();
