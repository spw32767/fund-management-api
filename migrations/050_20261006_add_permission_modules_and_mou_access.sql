-- Keep the existing permission IDs and grants; classify the catalog by application.
ALTER TABLE permissions
  ADD COLUMN `module` varchar(32) NOT NULL DEFAULT 'research' AFTER `code`;

UPDATE permissions SET `module` = 'research' WHERE `module` <> 'research';

INSERT INTO permissions (code, `module`, resource, action, description)
VALUES
  ('mou.read', 'mou', 'mou', 'read', 'View MOU records and activities'),
  ('mou.manage', 'mou', 'mou', 'manage', 'Create, update, renew and delete MOU records and activities'),
  ('portal.card.research_fund.access', 'portal', 'portal_card', 'research_fund_access', 'Open research fund from the portal'),
  ('portal.card.external_fund.access', 'portal', 'portal_card', 'external_fund_access', 'Open external fund from the portal'),
  ('portal.card.links.access', 'portal', 'portal_card', 'links_access', 'Open links from the portal'),
  ('portal.card.researcher_management.access', 'portal', 'portal_card', 'researcher_management_access', 'Open researcher management from the portal')
ON DUPLICATE KEY UPDATE
  `module` = VALUES(`module`),
  resource = VALUES(resource),
  action = VALUES(action),
  description = VALUES(description),
  delete_at = NULL,
  update_at = current_timestamp();

INSERT INTO role_permissions (role_id, permission_id, create_at, update_at, delete_at)
SELECT permission_grants.role_id, p.permission_id, current_timestamp(), current_timestamp(), NULL
FROM (
  SELECT 3 AS role_id, 'mou.read' AS code UNION ALL
  SELECT 3, 'mou.manage' UNION ALL
  SELECT 1, 'portal.card.research_fund.access' UNION ALL
  SELECT 2, 'portal.card.research_fund.access' UNION ALL
  SELECT 3, 'portal.card.research_fund.access' UNION ALL
  SELECT 4, 'portal.card.research_fund.access' UNION ALL
  SELECT 5, 'portal.card.research_fund.access' UNION ALL
  SELECT 1, 'portal.card.external_fund.access' UNION ALL
  SELECT 2, 'portal.card.external_fund.access' UNION ALL
  SELECT 3, 'portal.card.external_fund.access' UNION ALL
  SELECT 4, 'portal.card.external_fund.access' UNION ALL
  SELECT 5, 'portal.card.external_fund.access' UNION ALL
  SELECT 6, 'portal.card.external_fund.access' UNION ALL
  SELECT 1, 'portal.card.links.access' UNION ALL
  SELECT 2, 'portal.card.links.access' UNION ALL
  SELECT 3, 'portal.card.links.access' UNION ALL
  SELECT 4, 'portal.card.links.access' UNION ALL
  SELECT 5, 'portal.card.links.access' UNION ALL
  SELECT 6, 'portal.card.links.access' UNION ALL
  SELECT 3, 'portal.card.researcher_management.access' UNION ALL
  SELECT 6, 'portal.card.researcher_management.access'
) AS permission_grants
INNER JOIN permissions p ON p.code = permission_grants.code
ON DUPLICATE KEY UPDATE delete_at = NULL, update_at = current_timestamp();
