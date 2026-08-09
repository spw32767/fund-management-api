# External API — Scopus Publications

REST API for external systems to pull faculty members' Scopus publications from the Fund
Management platform. Authentication is by **API key** (stored hashed in our DB), authorized by
**scope**, with per-request **audit logging** and **rate limiting**. The design hosts future
external APIs beyond this first endpoint.

This document has two parts:

- **Part A — Consumer guide** (for the calling system's developers) — how to authenticate and
  call the API.
- **Part B — Operator guide** (internal) — schema, key issuance, deployment.

---

## Part A — Consumer guide

### Base URL

```
https://<host>/api/ext/v1
```

All external endpoints live under `/api/ext/v1`. This is versioned; breaking changes ship
under a new version (`/api/ext/v2`).

### Authentication

Send your API key as a Bearer token on every request:

```
Authorization: Bearer <your-api-key>
```

- The key looks like `fsk_XXXXXXXXXXXXXXXXXXXXXX`. Treat it as a secret — do not embed it in
  front-end code, URLs, or logs.
- Keys may be rotated: you can be given a new key while the old one still works, then the old
  one is revoked. Switch to the new key when you receive it.
- **HTTPS only** in production.

### Endpoint: List Scopus publications

```
GET /api/ext/v1/scopus/publications
```

Returns a **flat, paginated** list — one row per (faculty member, publication). A publication
co-authored by several faculty members in our system appears once per member. Filter and
group on your side as needed.

**Query parameters**

| Param       | Required | Description |
|-------------|----------|-------------|
| `user_ids`  | ✅       | Comma-separated list of faculty `user_id`s (our system's IDs). Duplicates ignored. |
| `year_from` | ✅       | Start year (inclusive), 4 digits. Based on the publication's Scopus cover date. |
| `year_to`   | ❌       | End year (inclusive). Defaults to the current year. |
| `limit`     | ❌       | Page size. Default `50`, max `500`. |
| `offset`    | ❌       | Rows to skip. Default `0`. |

`user_id`s are the platform's stable primary keys; obtain the mapping (person → `user_id`) from
the platform operator.

**Example request**

```bash
curl -H "Authorization: Bearer fsk_XXXXXXXXXXXX" \
  "https://<host>/api/ext/v1/scopus/publications?user_ids=101,102&year_from=2020&year_to=2024&limit=50&offset=0"
```

**Example response** `200 OK`

```json
{
  "success": true,
  "data": [
    {
      "user_id": 101,
      "user_name": "Somchai Jaidee",
      "user_email": "somchai@kku.ac.th",
      "user_scopus_id": "55680000000",
      "document_id": 2201,
      "title": "Deep Learning for X",
      "publication_name": "IEEE Access",
      "publication_year": 2023,
      "cover_date": "2023-05-01T00:00:00Z",
      "cited_by": 12,
      "doi": "10.1109/ACCESS.2023.0000000",
      "eid": "2-s2.0-85000000000",
      "scopus_id": "85000000000",
      "scopus_url": "https://www.scopus.com/record/display.uri?eid=2-s2.0-85000000000&origin=inward",
      "author_names": "Somchai Jaidee | Jane Doe",
      "affiliation_name": "Khon Kaen University",
      "conference_name": null,
      "cite_score_quartile": "Q1"
    }
  ],
  "paging": { "total": 240, "limit": 50, "offset": 0 },
  "filters": { "user_ids": [101, 102], "year_from": 2020, "year_to": 2024 }
}
```

Fields are nullable where the underlying Scopus record is missing them (they are omitted from
the JSON when empty). Additional fields (conference details, ISSN, affiliations JSON, CiteScore
metrics, etc.) may be present.

**Pagination**: use `paging.total` to iterate. Increase `offset` by `limit` until
`offset >= total`.

### Error format

All errors use a consistent envelope:

```json
{ "success": false, "error": "<message>", "code": "<CODE>" }
```

| HTTP | `code`                 | Meaning |
|------|------------------------|---------|
| 401  | `MISSING_API_KEY`      | No `Authorization: Bearer` header. |
| 401  | `INVALID_API_KEY`      | Key unknown, revoked, or expired. |
| 403  | `INSUFFICIENT_SCOPE`   | Key is valid but lacks the required scope. |
| 403  | `HTTPS_REQUIRED`       | Request was not over HTTPS (when enforced). |
| 422  | `INVALID_PARAMETER`    | Missing/invalid `user_ids`, `year_from`, `year_to`, etc. |
| 429  | `RATE_LIMIT_EXCEEDED`  | Too many requests. Respect the `Retry-After` header (seconds). |
| 500  | —                      | Internal error. |

### Rate limiting

Each client is limited to a fixed number of requests per minute (default **100**). Exceeding it
returns `429` with a `Retry-After` header. Design your integration to page steadily rather than
burst.

---

## Part B — Operator guide (internal)

### Architecture

- Same Go binary as the main API (`cmd/api`); the external group hangs off the router directly
  as `/api/ext/v1`, so it does **not** use the internal JWT `AuthMiddleware`.
- Middleware chain on the group (`routes/routes.go`):
  `ExtRequestLog → RequireHTTPS → APIKeyAuthMiddleware → ExtRateLimit → RequireScope → handler`.
- Data comes from `ScopusPublicationService.ListForPartner(...)`, which reuses the same
  (user → scopus_authors via `users.Scopus_id` → scopus_document_authors → scopus_documents)
  join as the internal dashboard, adding a `user_id IN (...)` and inclusive cover-year filter.

### Database schema (migration `035_20260809_create_api_client_tables.sql`)

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

### Managing clients & keys (admin endpoints)

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
  "data": { "raw_key": "fsk_XXXXXXXXXXXX", "key": { "id": 5, "key_prefix": "fsk_XXXXXXXX", "status": "active", ... } }
}
```

**Rotation**: issue a new key, hand it to the consumer, then revoke the old one — no downtime.

### Configuration (env)

| Variable | Default | Purpose |
|---|---|---|
| `EXT_API_RATE_LIMIT_PER_MIN` | `100` | Per-client request budget per minute. |
| `EXT_API_REQUIRE_HTTPS` | `false` | When `true`, reject requests whose `X-Forwarded-Proto` isn't `https`. |

### Deployment notes

- **HTTPS**: terminate TLS at the reverse proxy — that is the primary enforcement point. Set
  `EXT_API_REQUIRE_HTTPS=true` as an in-app backstop once the proxy sets `X-Forwarded-Proto`.
- **CORS**: `Authorization` is already an allowed header. If a browser calls the API directly
  from another origin, add that origin to `ALLOWED_ORIGINS`. Server-to-server callers are
  unaffected by CORS.
- **Rate limiting is in-process**: counters reset on restart and are not shared across
  instances. Fine for a single-process deployment; move to a shared store (e.g. Redis) if the
  API is scaled horizontally.
- **Audit log growth**: `api_request_logs` grows one row per request. Add a retention/rotation
  job if volume is high.
