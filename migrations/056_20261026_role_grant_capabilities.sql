-- Capabilities previously enforced through primary role IDs. No user role or
-- user override is changed. Apply this migration before deploying the API.
INSERT INTO permissions (code, `module`, resource, action, description)
VALUES
 ('fund.settings.read', 'research', 'fund_settings', 'read', 'Read fund settings and reference data'),
 ('fund.settings.manage', 'research', 'fund_settings', 'manage', 'Manage fund settings and reference data'),
 ('projects.manage', 'research', 'projects', 'manage', 'Create, update and delete projects and project metadata'),
 ('academic.import.manage', 'research', 'academic_import', 'manage', 'Run imports, classifications and synchronization jobs'),
 ('submission.manage.all', 'research', 'submission', 'manage_all', 'Manage submissions, participants and documents for all users'),
 ('fund.visibility.all', 'research', 'fund', 'view_all', 'View funds regardless of target role'),
 ('submission.staff.read', 'research', 'submission', 'read_staff', 'Read the staff submission review queue')
ON DUPLICATE KEY UPDATE description = VALUES(description), update_at = current_timestamp();

INSERT INTO role_permissions (role_id, permission_id, create_at, update_at, delete_at)
SELECT role_grants.role_id, p.permission_id, current_timestamp(), current_timestamp(), NULL
FROM (
 SELECT 3 AS role_id, 'fund.settings.read' AS code UNION ALL
 SELECT 3, 'fund.settings.manage' UNION ALL
 SELECT 3, 'projects.manage' UNION ALL
 SELECT 3, 'academic.import.manage' UNION ALL
 SELECT 3, 'submission.manage.all' UNION ALL
 SELECT 3, 'fund.visibility.all' UNION ALL
 SELECT 5, 'fund.settings.read' UNION ALL
 SELECT 5, 'submission.read.all' UNION ALL
 SELECT 2, 'submission.staff.read'
) role_grants JOIN permissions p ON p.code = role_grants.code AND p.delete_at IS NULL
ON DUPLICATE KEY UPDATE update_at = role_permissions.update_at;
