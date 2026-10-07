-- The portal grant now authorizes the researcher-management pages and API.
UPDATE permissions
SET description = 'Manage researcher profiles, courses, and settings',
    update_at = current_timestamp()
WHERE code = 'portal.card.researcher_management.access';
