-- User management permissions

INSERT INTO `permissions` (`code`, `resource`, `action`, `description`, `create_at`, `update_at`, `delete_at`)
VALUES
  ('users.view', 'users', 'view', 'View user management records', current_timestamp(), current_timestamp(), NULL),
  ('users.manage', 'users', 'manage', 'Create and update user management records', current_timestamp(), current_timestamp(), NULL)
ON DUPLICATE KEY UPDATE
  `resource` = VALUES(`resource`),
  `action` = VALUES(`action`),
  `description` = VALUES(`description`),
  `update_at` = current_timestamp(),
  `delete_at` = NULL;

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `create_at`, `update_at`, `delete_at`)
SELECT 3, p.permission_id, current_timestamp(), current_timestamp(), NULL
FROM `permissions` p
WHERE p.code IN ('users.view', 'users.manage')
ON DUPLICATE KEY UPDATE
  `update_at` = current_timestamp(),
  `delete_at` = NULL;
