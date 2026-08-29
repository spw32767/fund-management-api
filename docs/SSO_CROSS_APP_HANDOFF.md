# SSO Cross-App Handoff — Internal Scope & Design

> **Audience:** our own team / AI agents working in `fund-management-api` (the "fs" system).
> This document describes the whole feature end-to-end so an agent reading it understands
> **both** what our side does **and** what the sibling "hrd" system (built by the intern)
> must do. The intern-facing spec is a separate file: [`SSO_HANDOFF_FOR_ACADEMIC.md`](SSO_HANDOFF_FOR_ACADEMIC.md).

---

## 1. Goal

Let a **second application** ("hrd", `https://hrd.computing.kku.ac.th`, built by an
intern, running on a **different port of the same VM**) reuse **our** KKU SSO login, so:

- The user logs in through **one** SSO integration (ours). The hrd app does **not**
  register its own SSO app with the university.
- The hrd app has its own normal (username/password) login page and just adds a
  **"Login with KKU SSO"** button. That button sends the user through **our** SSO and comes
  back authenticated — the user never sees our login UI.
- Identity (who the user is) is passed to hrd; **authorization** (what they can do,
  roles) is decided by hrd on its own.

## 2. Why a handoff is needed (the cookie boundary)

Our auth cookie (`auth_token`) is set **host-only** — see `controllers/sso_auth.go`,
`c.SetCookie(name, token, maxAge, "/", "", true, true)` where the 5th arg (domain) is `""`.

- Host-only ⇒ the cookie is sent **only** to `fs.computing.kku.ac.th`.
- `hrd.computing.kku.ac.th` is a **different host**, so the browser never sends our
  cookie there. HRD cannot read our session even right after login.
- The cookie is also `httpOnly` (JS cannot read it) + `Secure` + `SameSite=Lax`.

So a plain link to hrd is not enough. We cross the subdomain boundary with a **one-time
ticket** (like an OAuth authorization code), never by widening or sharing the cookie.

We deliberately chose **not** to set the cookie on the parent domain `.computing.kku.ac.th`,
because that would expose our session cookie to every service under the faculty domain.

## 3. Architecture — our app is a lightweight identity broker

```mermaid
sequenceDiagram
    autonumber
    participant U as User (browser)
    participant A as hrd (intern app)<br/>hrd.computing.kku.ac.th
    participant F as fs (our app)<br/>fs.computing.kku.ac.th
    participant S as KKU SSONext

    U->>A: Open protected page / click "Login with KKU SSO"
    A-->>U: 302 → fs /api/auth/sso/login?return_to=<hrd callback>
    U->>F: GET /api/auth/sso/login?return_to=...
    Note over F: validate return_to against allowlist,<br/>store in short-lived cookie sso_return_to
    F-->>U: 302 → KKU SSO login (?app=APP_ID)
    U->>S: Enter KKU username / password
    S-->>U: 302 → fs /api/auth/sso/callback?code=...
    U->>F: GET /api/auth/sso/callback?code=...
    Note over F: exchange code → email;<br/>allowlist check (users table);<br/>set our own auth_token cookie
    Note over F: return_to cookie present →<br/>mint one-time ticket (60s, user_tokens)
    F-->>U: 302 → hrd callback?ticket=XYZ
    U->>A: GET /auth/sso/callback?ticket=XYZ
    A->>F: POST /api/auth/sso/handoff/verify {ticket}<br/>X-Handoff-Client-Secret (server-to-server)
    F-->>A: 200 {ok, user_id, email, first_name, last_name}
    Note over A: create hrd's OWN session cookie;<br/>apply hrd's OWN roles/permissions
    A-->>U: 302 → into hrd app (logged in)
```

## 4. What OUR side (`fund-management-api`) provides — IMPLEMENTED

All changes are **opt-in**. When `SSO_HANDOFF_ALLOWED_ORIGINS` is empty the feature is off and
SSO behaves exactly as before.

| Piece | Location |
|---|---|
| `return_to` handling + open-redirect guard, ticket minting, verify endpoint | `controllers/sso_handoff.go` |
| `SSOLoginRedirect` stores validated `return_to` cookie | `controllers/sso_auth.go` |
| `SSOCallback` mints ticket + redirects to `return_to?ticket=` on success | `controllers/sso_auth.go` |
| Route `POST /api/auth/sso/handoff/verify` | `routes/routes.go` |
| Env vars | `.env.example` |
| Unit tests (allowlist / URL building) | `controllers/sso_handoff_test.go` |

### 4.1 Endpoints (our contract)

**A) Initiate login** — browser redirect, entry point for hrd:
```
GET https://fs.computing.kku.ac.th/api/auth/sso/login?return_to=<url-encoded hrd callback>
```
- `return_to` must be HTTPS and its origin (`scheme://host[:port]`) must be listed in
  `SSO_HANDOFF_ALLOWED_ORIGINS`. Invalid/disallowed `return_to` is silently ignored (falls back
  to normal login → `/`).

**B) Callback** — existing route, our side only. On success **with** a valid `return_to` cookie
it redirects the browser to `return_to?ticket=<ticket>` (preserving any existing query params).
Without it, it redirects to `/` as before.

**C) Verify ticket** — server-to-server, called by hrd's backend:
```
POST https://fs.computing.kku.ac.th/api/auth/sso/handoff/verify
Content-Type: application/json
X-Handoff-Client-Secret: <SSO_HANDOFF_CLIENT_SECRET>   # required only if the env var is set
{ "ticket": "<ticket from the redirect>" }
```
Responses:
- `200 {"ok":true,"user_id":123,"email":"...","first_name":"...","last_name":"..."}`
- `400 {"ok":false,"error":"missing ticket"}`
- `401 {"ok":false,"error":"invalid or used ticket" | "expired ticket" | "user not allowed" | "invalid client secret"}`

### 4.2 Ticket properties

- Opaque 256-bit random string (`crypto/rand`), **not** a JWT → no signing secret shared.
- Persisted in existing `user_tokens` table with `token_type = "sso_handoff"` → **no DB migration**.
- **Single-use**: consumed (revoked) on first verify, inside a transaction, before the expiry
  check, so a leaked ticket cannot be replayed even within its TTL.
- **TTL 60s** (`handoffTicketTTL`).

### 4.3 Allowlist / identity semantics (say this in the meeting)

- The **email allowlist is checked against OUR `users` table** (`delete_at IS NULL`) — same rule
  as normal SSO login (`controllers/sso_auth.go`). Only people who already exist in our DB (e.g.
  faculty) get a ticket at all.
- We return **identity only** (`user_id`, `email`, `first_name`, `last_name`). We do **not** tell
  hrd what the user may do. HRD maps identity → its own roles/permissions.

## 5. What the HRD side (intern) must build — NOT in this repo

See [`SSO_HANDOFF_FOR_ACADEMIC.md`](SSO_HANDOFF_FOR_ACADEMIC.md) for the full spec to hand over.
Summary:

1. A **"Login with KKU SSO"** button on their existing login page → redirects browser to our
   endpoint **A** with `return_to` = their own callback URL.
2. A **callback route** (e.g. `GET /auth/sso/callback?ticket=...`) that takes the ticket and calls
   our endpoint **C** from **their backend** (with the client secret).
3. On success: create **their own** session/cookie on `hrd.*` (httpOnly) and apply their own
   roles. On failure: show an error / send back to their login.
4. Their own logout (optionally chained to our `/api/auth/logout`).

They must **never** try to read our `auth_token` cookie or forge our JWT — identity only ever
comes through the verified ticket.

## 6. Configuration

### 6.1 Our `.env` (production, `fs`)
```
SSO_HANDOFF_ALLOWED_ORIGINS=https://hrd.computing.kku.ac.th
SSO_HANDOFF_CLIENT_SECRET=<long random shared secret, give the same value to the intern>
```
(Existing `SSO_*`, `JWT_SECRET`, `AUTH_COOKIE_NAME` stay as they are.)

### 6.2 What to REQUEST from the university (we don't control these)
- **New subdomain `hrd.computing.kku.ac.th`**: DNS record pointing to the **same VM**, a
  **TLS certificate** for it (or a wildcard `*.computing.kku.ac.th`), and a **reverse-proxy** rule
  routing it to the intern app's port (same treatment `fs.*` already gets).
- **SSO: no change required.** The SSO callback stays `https://fs.computing.kku.ac.th/api/auth/sso/callback`.
  We are **not** registering a new SSO app and **not** changing the redirect/logout URLs.

## 7. Security notes / decisions

- **Open-redirect guard**: tickets are only ever appended to an origin explicitly listed in
  `SSO_HANDOFF_ALLOWED_ORIGINS`, over HTTPS. Anything else is ignored.
- **No secret sharing for identity**: the ticket is opaque and verified server-side; hrd
  never needs our `JWT_SECRET`.
- **Client authentication**: `SSO_HANDOFF_CLIENT_SECRET` (constant-time compared) authenticates the
  hrd backend on the verify call — set it in production.
- **Cookie stays host-only** for `fs.*`; we did not weaken it.
- **Blast radius**: if hrd is compromised, the attacker still only obtains identities of users
  who complete an interactive KKU SSO login and are in our allowlist — no bulk access to our data.

## 8. Known unrelated test noise

Three pre-existing tests (`TestSSOCallbackRejectsUnknownUser`, `...MergesExistingUserByEmail`,
`...RejectsSoftDeletedUser` in `controllers/sso_auth_test.go`) fail in the current tree because
their scripted-DB regex expects `LIMIT 1` while the installed GORM emits a parameterized `LIMIT ?`.
This is **independent of the handoff feature** (confirmed by stashing the handoff changes and
re-running). The new handoff tests pass. Fix the regex separately if desired.
