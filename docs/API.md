# API reference (Phases 0–1)

There are two contracts; never mix them:

- **Custom API**: `/api/v1/pawn/x/*`. Envelopes `{"data":…,"meta":{"request_id","next_cursor"}}`
  and `{"error":{"code","message","details?"},"request_id"}`. IDs are decimal
  strings and times are ISO-8601 UTC.
- **Native Collar API**: `/api/v1/pawn/content/{table}`. Collar's own list
  shape (`items`, `page`, `perPage`, `totalItems`…), filters and error envelope.
  The dashboards call it directly.

## Ingress rules (all `/api/*`)

| Rule | Behaviour |
|---|---|
| Credentials | Only the HttpOnly session cookie. Incoming `Authorization` headers and realtime `?token=` are stripped. |
| Native auth routes | `/api/v1/auth/*` and `/api/v1/{app}/auth/*` → `404 NOT_FOUND` (reached only by internal dispatch). |
| Unsafe methods | Same-origin `Origin` (or `Referer`) required, else `403 CSRF_REJECTED`. With a live session, `X-CSRF-Token` must match too, except on the anonymous-capable auth routes. |
| Unknown custom routes | `404 NOT_FOUND`, or `405 METHOD_NOT_ALLOWED` with `Allow`. |
| Caching | `Cache-Control: private, no-store` unless a route opts into public caching. |

## Health

| Route | Response |
|---|---|
| `GET /healthz` | `200 {"status":"ok"}` |
| `GET /readyz` | `200` when the app database is reachable and migrated to this binary's schema, else `503`. `checks.redis` reports `ok` or `degraded: …` without failing readiness. |

## Custom routes

| Method · path | Auth | Request | Success | Errors | Rate limit | Cache |
|---|---|---|---|---|---|---|
| `GET /public-config` | none | – | 200 brand, flags, limits | – | – | public 60 s |
| `POST /auth/register` | none | `email, password, display_name, handle?` | 202 (same for existing emails) | 422 fields | 5/h/IP prefix | no-store |
| `POST /auth/login` | none | `email, password` | 200 `{user, csrf_token}` + cookie | 401 `INVALID_CREDENTIALS`, 429 | 10/15 min/IP+email, plus Collar throttle | no-store |
| `POST /auth/logout` | cookie | – | 204, cookie cleared | – | – | no-store |
| `POST /auth/logout-all` | session | – | 204 | 401 | – | no-store |
| `GET /auth/me` | session | – | 200 `{user, csrf_token}` | 401 `UNAUTHENTICATED` | – | no-store |
| `POST /auth/verify-email` | none | `token` | 200 | 400 `TOKEN_INVALID` | 30/h/IP prefix | no-store |
| `POST /auth/resend-verification` | none | `email` | 202 (generic) | – | 3/h/IP+email | no-store |
| `POST /auth/password/forgot` | none | `email` | 202 (generic) | – | 3/h/IP+email | no-store |
| `POST /auth/password/reset` | none | `token, new_password` | 200, all sessions revoked | 400 `TOKEN_INVALID`, 422 | 30/h/IP prefix | no-store |
| `POST /auth/password/change` | session+CSRF | `old_password, new_password` | 200 `{csrf_token}` + rotated cookie; other sessions revoked | 422 fields, 429 | login limiter | no-store |
| `PATCH /users/me` | session+CSRF | `display_name?, timezone?, settings?` (unknown fields → 422) | 200 user | 422 | – | no-store |
| `GET /users/me/sessions` | session | – | 200 list (device label, times, current) | 401 | – | no-store |
| `DELETE /users/me/sessions/{id}` | session+CSRF | – | 204 | 404 | – | no-store |

`user` = `{id, email, handle, display_name, role, email_verified, timezone, settings, permissions{can_comment, can_review, studio, admin}}`.
Permissions are UI hints; every endpoint enforces its own.

## Native collections exposed so far (`collar.yaml`)

| Collection | list/view | writes |
|---|---|---|
| `users` | locked (superuser only) | locked |
| `admin_user_summaries` (view) | owner, moderator | none |
| `audit_logs` | owner | none |
| `site_settings` | owner | locked (owner commands later) |

Collar superuser/admin APIs (`/api/v1/admin/*`, `/api/v1/pawn/collections`,
`/api/v1/pawn/ui`) return 403 to every app user, including the owner.
