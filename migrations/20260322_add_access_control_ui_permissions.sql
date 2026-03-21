-- Access control management + admin page visibility permissions

INSERT INTO `permissions` (`code`, `resource`, `action`, `description`)
VALUES
  ('access.manage', 'access_control', 'manage', 'Manage role permissions and user permission overrides'),
  ('ui.page.admin.dashboard.view', 'ui_page_admin', 'dashboard_view', 'View admin dashboard page'),
  ('ui.page.admin.research_fund.view', 'ui_page_admin', 'research_fund_view', 'View research fund management page'),
  ('ui.page.admin.promotion_fund.view', 'ui_page_admin', 'promotion_fund_view', 'View promotion fund management page'),
  ('ui.page.admin.applications.view', 'ui_page_admin', 'applications_view', 'View applications management page'),
  ('ui.page.admin.scopus.view', 'ui_page_admin', 'scopus_view', 'View Scopus research page'),
  ('ui.page.admin.fund_settings.view', 'ui_page_admin', 'fund_settings_view', 'View fund settings page'),
  ('ui.page.admin.projects.view', 'ui_page_admin', 'projects_view', 'View projects management page'),
  ('ui.page.admin.approval_records.view', 'ui_page_admin', 'approval_records_view', 'View approval records page'),
  ('ui.page.admin.import_export.view', 'ui_page_admin', 'import_export_view', 'View import/export page'),
  ('ui.page.admin.academic_imports.view', 'ui_page_admin', 'academic_imports_view', 'View academic imports page'),
  ('ui.page.admin.access_control.view', 'ui_page_admin', 'access_control_view', 'View access control management page')
ON DUPLICATE KEY UPDATE
  `resource` = VALUES(`resource`),
  `action` = VALUES(`action`),
  `description` = VALUES(`description`),
  `update_at` = current_timestamp();

INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT 3, p.permission_id
FROM `permissions` p
WHERE p.code IN (
  'access.manage',
  'ui.page.admin.dashboard.view',
  'ui.page.admin.research_fund.view',
  'ui.page.admin.promotion_fund.view',
  'ui.page.admin.applications.view',
  'ui.page.admin.scopus.view',
  'ui.page.admin.fund_settings.view',
  'ui.page.admin.projects.view',
  'ui.page.admin.approval_records.view',
  'ui.page.admin.import_export.view',
  'ui.page.admin.academic_imports.view',
  'ui.page.admin.access_control.view'
)
ON DUPLICATE KEY UPDATE `update_at` = current_timestamp(), `delete_at` = NULL;
