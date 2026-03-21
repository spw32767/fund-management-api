-- Separate access control permissions: view vs manage

INSERT INTO permissions (code, resource, action, description)
VALUES
  ('access.view', 'access_control', 'view', 'View access control roles, permissions, and effective rights')
ON DUPLICATE KEY UPDATE
  resource = VALUES(resource),
  action = VALUES(action),
  description = VALUES(description),
  update_at = current_timestamp();

INSERT INTO role_permissions (role_id, permission_id, create_at, update_at, delete_at)
SELECT 3, p.permission_id, current_timestamp(), current_timestamp(), NULL
FROM permissions p
WHERE p.code IN ('access.view', 'access.manage')
ON DUPLICATE KEY UPDATE
  delete_at = NULL,
  update_at = current_timestamp();
