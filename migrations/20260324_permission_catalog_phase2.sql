-- Phase 2 permission catalog and role baseline

INSERT INTO permissions (code, resource, action, description)
VALUES
  ('portal.member.access', 'portal', 'member_access', 'Access member portal'),
  ('portal.admin.access', 'portal', 'admin_access', 'Access admin portal'),
  ('portal.executive.access', 'portal', 'executive_access', 'Access executive portal'),

  ('ui.page.member.dashboard.view', 'ui_page_member', 'dashboard_view', 'View member dashboard page'),
  ('ui.page.member.profile.view', 'ui_page_member', 'profile_view', 'View member profile page'),
  ('ui.page.member.research_fund.view', 'ui_page_member', 'research_fund_view', 'View member research fund page'),
  ('ui.page.member.promotion_fund.view', 'ui_page_member', 'promotion_fund_view', 'View member promotion fund page'),
  ('ui.page.member.applications.view', 'ui_page_member', 'applications_view', 'View member applications page'),
  ('ui.page.member.received_funds.view', 'ui_page_member', 'received_funds_view', 'View member received funds page'),
  ('ui.page.member.announcements.view', 'ui_page_member', 'announcements_view', 'View member announcements page'),
  ('ui.page.member.projects.view', 'ui_page_member', 'projects_view', 'View member projects page'),
  ('ui.page.member.notifications.view', 'ui_page_member', 'notifications_view', 'View member notifications page'),
  ('ui.page.member.dept_review.view', 'ui_page_member', 'dept_review_view', 'View department head review page'),

  ('fund.request.update', 'fund_request', 'update', 'Update own fund request'),
  ('fund.request.delete', 'fund_request', 'delete', 'Delete own fund request'),
  ('publication.reward.manage_own', 'publication_reward', 'manage_own', 'Create and manage own publication reward requests'),
  ('publication.reward.approve', 'publication_reward', 'approve', 'Approve or reject publication reward requests'),
  ('publication.reward.rate.manage', 'publication_reward_rate', 'manage', 'Manage publication reward rates'),
  ('access.view', 'access_control', 'view', 'View access control roles and permissions'),
  ('announcement.manage', 'announcement', 'manage', 'Manage announcements'),
  ('fund.form.manage', 'fund_form', 'manage', 'Manage fund forms'),
  ('dept_head.review.recommend', 'dept_head_review', 'recommend', 'Recommend submissions as department head'),
  ('dept_head.review.reject', 'dept_head_review', 'reject', 'Reject submissions as department head'),
  ('dept_head.review.request_revision', 'dept_head_review', 'request_revision', 'Request revision as department head')
ON DUPLICATE KEY UPDATE
  resource = VALUES(resource),
  action = VALUES(action),
  description = VALUES(description),
  update_at = current_timestamp();

-- reset current active role permissions and apply new baseline
UPDATE role_permissions
SET delete_at = current_timestamp(), update_at = current_timestamp()
WHERE role_id IN (1, 2, 3, 4, 5)
  AND delete_at IS NULL;

INSERT INTO role_permissions (role_id, permission_id, create_at, update_at, delete_at)
SELECT mapping.role_id, p.permission_id, current_timestamp(), current_timestamp(), NULL
FROM (
  -- teacher
  SELECT 1 AS role_id, 'portal.member.access' AS code UNION ALL
  SELECT 1, 'dashboard.view.self' UNION ALL
  SELECT 1, 'fund.request.create' UNION ALL
  SELECT 1, 'fund.request.update' UNION ALL
  SELECT 1, 'fund.request.delete' UNION ALL
  SELECT 1, 'publication.reward.manage_own' UNION ALL
  SELECT 1, 'submission.read.own' UNION ALL
  SELECT 1, 'ui.page.member.dashboard.view' UNION ALL
  SELECT 1, 'ui.page.member.profile.view' UNION ALL
  SELECT 1, 'ui.page.member.research_fund.view' UNION ALL
  SELECT 1, 'ui.page.member.promotion_fund.view' UNION ALL
  SELECT 1, 'ui.page.member.applications.view' UNION ALL
  SELECT 1, 'ui.page.member.received_funds.view' UNION ALL
  SELECT 1, 'ui.page.member.announcements.view' UNION ALL
  SELECT 1, 'ui.page.member.projects.view' UNION ALL
  SELECT 1, 'ui.page.member.notifications.view' UNION ALL

  -- staff
  SELECT 2, 'portal.member.access' UNION ALL
  SELECT 2, 'dashboard.view.self' UNION ALL
  SELECT 2, 'fund.request.create' UNION ALL
  SELECT 2, 'fund.request.update' UNION ALL
  SELECT 2, 'fund.request.delete' UNION ALL
  SELECT 2, 'publication.reward.manage_own' UNION ALL
  SELECT 2, 'submission.read.own' UNION ALL
  SELECT 2, 'ui.page.member.dashboard.view' UNION ALL
  SELECT 2, 'ui.page.member.profile.view' UNION ALL
  SELECT 2, 'ui.page.member.research_fund.view' UNION ALL
  SELECT 2, 'ui.page.member.promotion_fund.view' UNION ALL
  SELECT 2, 'ui.page.member.applications.view' UNION ALL
  SELECT 2, 'ui.page.member.received_funds.view' UNION ALL
  SELECT 2, 'ui.page.member.announcements.view' UNION ALL
  SELECT 2, 'ui.page.member.projects.view' UNION ALL
  SELECT 2, 'ui.page.member.notifications.view' UNION ALL

  -- admin
  SELECT 3, 'portal.admin.access' UNION ALL
  SELECT 3, 'access.view' UNION ALL
  SELECT 3, 'access.manage' UNION ALL
  SELECT 3, 'dashboard.view.admin' UNION ALL
  SELECT 3, 'report.export' UNION ALL
  SELECT 3, 'fund.request.create' UNION ALL
  SELECT 3, 'fund.request.update' UNION ALL
  SELECT 3, 'fund.request.delete' UNION ALL
  SELECT 3, 'fund.request.approve' UNION ALL
  SELECT 3, 'publication.reward.manage_own' UNION ALL
  SELECT 3, 'publication.reward.approve' UNION ALL
  SELECT 3, 'publication.reward.rate.manage' UNION ALL
  SELECT 3, 'announcement.manage' UNION ALL
  SELECT 3, 'fund.form.manage' UNION ALL
  SELECT 3, 'submission.read.all' UNION ALL
  SELECT 3, 'scopus.publications.read' UNION ALL
  SELECT 3, 'scopus.publications.read_by_user' UNION ALL
  SELECT 3, 'scopus.publications.export' UNION ALL
  SELECT 3, 'scopus.publications.export_by_user' UNION ALL
  SELECT 3, 'ui.page.admin.dashboard.view' UNION ALL
  SELECT 3, 'ui.page.admin.research_fund.view' UNION ALL
  SELECT 3, 'ui.page.admin.promotion_fund.view' UNION ALL
  SELECT 3, 'ui.page.admin.applications.view' UNION ALL
  SELECT 3, 'ui.page.admin.scopus.view' UNION ALL
  SELECT 3, 'ui.page.admin.fund_settings.view' UNION ALL
  SELECT 3, 'ui.page.admin.projects.view' UNION ALL
  SELECT 3, 'ui.page.admin.approval_records.view' UNION ALL
  SELECT 3, 'ui.page.admin.import_export.view' UNION ALL
  SELECT 3, 'ui.page.admin.academic_imports.view' UNION ALL
  SELECT 3, 'ui.page.admin.access_control.view' UNION ALL

  -- dept head = member + review scope
  SELECT 4, 'portal.member.access' UNION ALL
  SELECT 4, 'dashboard.view.self' UNION ALL
  SELECT 4, 'fund.request.create' UNION ALL
  SELECT 4, 'fund.request.update' UNION ALL
  SELECT 4, 'fund.request.delete' UNION ALL
  SELECT 4, 'publication.reward.manage_own' UNION ALL
  SELECT 4, 'submission.read.own' UNION ALL
  SELECT 4, 'submission.read.department' UNION ALL
  SELECT 4, 'dept_head.review.recommend' UNION ALL
  SELECT 4, 'dept_head.review.reject' UNION ALL
  SELECT 4, 'dept_head.review.request_revision' UNION ALL
  SELECT 4, 'ui.page.member.dashboard.view' UNION ALL
  SELECT 4, 'ui.page.member.profile.view' UNION ALL
  SELECT 4, 'ui.page.member.research_fund.view' UNION ALL
  SELECT 4, 'ui.page.member.promotion_fund.view' UNION ALL
  SELECT 4, 'ui.page.member.applications.view' UNION ALL
  SELECT 4, 'ui.page.member.received_funds.view' UNION ALL
  SELECT 4, 'ui.page.member.announcements.view' UNION ALL
  SELECT 4, 'ui.page.member.projects.view' UNION ALL
  SELECT 4, 'ui.page.member.notifications.view' UNION ALL
  SELECT 4, 'ui.page.member.dept_review.view' UNION ALL

  -- executive = admin dashboard readonly
  SELECT 5, 'portal.executive.access' UNION ALL
  SELECT 5, 'dashboard.view.admin' UNION ALL
  SELECT 5, 'ui.page.admin.dashboard.view'
) AS mapping
INNER JOIN permissions p ON p.code = mapping.code
ON DUPLICATE KEY UPDATE
  delete_at = NULL,
  update_at = current_timestamp();
