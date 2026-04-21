-- Add dedicated admin permission for research dashboard page

INSERT INTO permissions (code, resource, action, description)
VALUES
  ('ui.page.admin.research_dashboard.view', 'ui_page_admin', 'research_dashboard_view', 'View research dashboard page')
ON DUPLICATE KEY UPDATE
  resource = VALUES(resource),
  action = VALUES(action),
  description = VALUES(description),
  update_at = current_timestamp();

INSERT INTO role_permissions (role_id, permission_id, create_at, update_at, delete_at)
SELECT 3, p.permission_id, current_timestamp(), current_timestamp(), NULL
FROM permissions p
WHERE p.code IN ('ui.page.admin.research_dashboard.view')
ON DUPLICATE KEY UPDATE
  delete_at = NULL,
  update_at = current_timestamp();
