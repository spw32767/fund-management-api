-- Reset role permission baseline for dept_head and executive

-- dept_head (role_id=4): behave as member + dept review scope
UPDATE role_permissions
SET delete_at = current_timestamp(), update_at = current_timestamp()
WHERE role_id = 4
  AND delete_at IS NULL;

INSERT INTO role_permissions (role_id, permission_id, create_at, update_at, delete_at)
SELECT 4, p.permission_id, current_timestamp(), current_timestamp(), NULL
FROM permissions p
WHERE p.code IN (
  'dashboard.view.self',
  'submission.read.department'
)
ON DUPLICATE KEY UPDATE
  delete_at = NULL,
  update_at = current_timestamp();

-- executive (role_id=5): read-only admin dashboard only
UPDATE role_permissions
SET delete_at = current_timestamp(), update_at = current_timestamp()
WHERE role_id = 5
  AND delete_at IS NULL;

INSERT INTO role_permissions (role_id, permission_id, create_at, update_at, delete_at)
SELECT 5, p.permission_id, current_timestamp(), current_timestamp(), NULL
FROM permissions p
WHERE p.code IN (
  'dashboard.view.admin',
  'ui.page.admin.dashboard.view'
)
ON DUPLICATE KEY UPDATE
  delete_at = NULL,
  update_at = current_timestamp();
