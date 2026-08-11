# External Scopus API — Operator Guide (INTERNAL)

Internal companion to `EXTERNAL_API_SCOPUS.md`. **Do not hand this file to external partners** —
it describes our internal schema, key issuance, and deployment. Give partners only the consumer
guide.

## Architecture

- Same Go binary as the main API (`cmd/api`); the external group hangs off the router directly as
  `/api/ext/v1`, so it does **not** use the internal JWT `AuthMiddleware`.
- Middleware chain on the group (`routes/routes.go`):
  `ExtRequestLog → RequireHTTPS → APIKeyAuthMiddleware → ExtRateLimit → RequireScope → handler`.
- Data comes from `ScopusPublicationService.ListForPartner(...)`, which reuses the same
  (user → `scopus_authors` via `users.Scopus_id` → `scopus_document_authors` → `scopus_documents`)
  join as the internal dashboard, adding a `user_id IN (...)` and inclusive cover-year filter.
  Existing service methods are untouched.

## Endpoints & scopes

| Endpoint | Scope | Consumer doc | Notes |
|---|---|---|---|
| `GET /api/ext/v1/scopus/publications` | `scopus.publications.read` | `EXTERNAL_API_SCOPUS.md` | seeded by migration 035 |
| `GET /api/ext/v1/users` | `users.read` | `EXTERNAL_USERS_API.md` | seeded by migration 036; faculty directory sync |

**Adding an external API = new endpoint + a new scope row (seed migration), reusing the
api_clients/api_keys/middleware here — no new tables.**

⚠️ **PII:** the `/users` endpoint returns `email` and `tel` (personal contact data). Grant
`users.read` only to clients authorized to receive it, and record that in the client's contact
notes. The faculty directory (`ListForPartner` in `services/user_directory_service.go`) excludes
soft-deleted users (`delete_at IS NULL`) and never returns passwords.

## Database schema (migration `035_20260809_create_api_client_tables.sql`)

| Table               | Purpose |
|---------------------|---------|
| `api_clients`       | External systems (name, contact, `status` active/disabled). |
| `api_scopes`        | Catalog of grantable scopes (e.g. `scopus.publications.read`). |
| `api_client_scopes` | Which scopes each client holds. |
| `api_keys`          | Keys per client. **Only the SHA-256 hash is stored**, plus a short `key_prefix` for display. Supports many active keys per client (rotation). |
| `api_request_logs`  | One row per external request: client, endpoint, requested `user_ids`, year range, HTTP status, IP, latency. **Never stores the key.** |

The migration also seeds the `scopus.publications.read` scope and an admin permission
`api.clients.manage` (granted to role 3) that gates the management endpoints below.

Apply it like any other migration (see `migrations/README.md`):

```bash
mysql -u <user> -p<password> <database> < migrations/035_20260809_create_api_client_tables.sql
```

## Managing clients & keys (admin endpoints)

All require an admin session (JWT) with permission `api.clients.manage`. Base:
`/api/v1/admin/api-clients`.

| Method & path | Action |
|---|---|
| `POST /api/v1/admin/api-clients` | Create a client. Body: `{ "name", "description?", "contact_email?", "scopes": ["scopus.publications.read"] }` |
| `GET /api/v1/admin/api-clients` | List clients with their scopes. |
| `GET /api/v1/admin/api-clients/:id` | Client detail + its keys (metadata only). |
| `PATCH /api/v1/admin/api-clients/:id` | Update `name`/`description`/`contact_email`/`status`, and/or replace `scopes`. |
| `POST /api/v1/admin/api-clients/:id/keys` | **Issue a key.** Body (optional): `{ "label?", "expires_in_days?" }`. Response returns the plaintext `raw_key` **once** — store it then. |
| `DELETE /api/v1/admin/api-clients/:id/keys/:keyId` | Revoke a key. |
| `GET /api/v1/admin/api-clients/:id/logs?limit=100` | Recent audit-log rows for the client. |

**Issue-key response** (the only time the raw key is shown):

```json
{
  "success": true,
  "message": "Store this key now — it will not be shown again.",
  "data": { "raw_key": "fsk_XXXXXXXXXXXX", "key": { "id": 5, "key_prefix": "fsk_XXXXXXXX", "status": "active" } }
}
```

**Rotation**: issue a new key, hand it to the consumer, then revoke the old one — no downtime.

## Configuration (env)

| Variable | Default | Purpose |
|---|---|---|
| `EXT_API_RATE_LIMIT_PER_MIN` | `100` | Per-client request budget per minute. |
| `EXT_API_REQUIRE_HTTPS` | `false` | When `true`, reject requests whose `X-Forwarded-Proto` isn't `https`. |

## Deployment notes

- **HTTPS**: terminate TLS at the reverse proxy — that is the primary enforcement point. Set
  `EXT_API_REQUIRE_HTTPS=true` as an in-app backstop once the proxy sets `X-Forwarded-Proto`.
- **CORS**: `Authorization` is already an allowed header. If a browser calls the API directly from
  another origin, add that origin to `ALLOWED_ORIGINS`. Server-to-server callers are unaffected by
  CORS.
- **Rate limiting is in-process**: counters reset on restart and are not shared across instances.
  Fine for a single-process deployment; move to a shared store (e.g. Redis) if the API is scaled
  horizontally.
- **Audit log growth**: `api_request_logs` grows one row per request. Add a retention/rotation job
  if volume is high.
