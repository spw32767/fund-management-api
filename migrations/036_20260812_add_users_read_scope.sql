-- External API: add the `users.read` scope for the faculty (users) directory endpoint.
-- Reuses the api_* subsystem from migration 035; no new tables.

INSERT INTO api_scopes (code, description)
VALUES ('users.read', 'Read faculty (users) directory via the external API')
ON DUPLICATE KEY UPDATE description = VALUES(description);
