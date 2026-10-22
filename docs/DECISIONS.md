# Decision log

Deviations from, and interpretations of, `IMPLEMENTATION.md`. Newest last.

## D-001 · Collar pinned at v0.1.1 (2026-10-22)
`github.com/xmfunew/collar v0.1.1` is the tag on the reviewed commit
`e961180bbd79a996cb07e16ef33fe597073d271b`. It resolves through authenticated
Git with `GOPRIVATE=github.com/xmfunew/collar`; no `replace` directive, no
vendored source. CI reads it with a `COLLAR_READ_TOKEN` repository secret
written to a credential store at runtime (never committed, never echoed).

## D-002 · Browser-session bridge design
- The browser gets one opaque cookie (`__Host-pawn_session` over HTTPS,
  `pawn_session` in plain-HTTP development): 256 random bits, stored only as
  `HMAC-SHA256(SESSION_HMAC_KEY, "session", token)`.
- Collar access/refresh tokens are sealed with AES-256-GCM
  (`TOKEN_ENCRYPTION_KEY`, version byte + random nonce, AD bound to the
  user id and token kind).
- The CSRF token is derived, not stored: `HMAC-SHA256(CSRF_HMAC_KEY, "csrf",
  cookie)`. The SPA receives it from `/auth/login` and `/auth/me` and keeps
  it in memory only; `csrf_token_hash` stores its SHA-256 for audit parity
  with the schema. Every unsafe `/api` request needs a same-origin `Origin`
  (or `Referer`); with a live session it also needs `X-CSRF-Token`.
  Anonymous-capable auth routes (login, register, verify, forgot, reset,
  logout) only need the Origin check.
- Login, refresh, logout, `me` and change-password are internal dispatches to
  Collar's documented routes through `App.Handler()`. Ingress returns 404 for
  every `/api/v1/auth/*` and `/api/v1/{app}/auth/*` path, strips incoming
  `Authorization` headers and realtime `?token=` parameters, so browsers can
  neither obtain nor present Collar tokens.
- Refresh serializes per session with an in-process mutex plus
  `SELECT … FOR UPDATE` on the session row; a waiter that finds a fresh token
  reuses it (proved by `TestConcurrentRefresh`). If the process dies after
  Collar rotated the refresh token but before the new ciphertext commits, the
  next refresh triggers Collar's reuse detection and the user signs in again.
  That failure is accepted: it is rare, and it fails safe.
- Revocation is immediate in the bridge (`revoked_at`), then best-effort in
  Collar (logout with the session's token, refreshing once if needed). The
  CLI password reset revokes bridge rows only; the Collar sessions behind
  them are unreachable and expire.

## D-003 · Live user state on every request
The bridge joins `users` on each cookie lookup, so suspension, deletion and
role changes apply on the next request. Collar's own identity cache
(`COLLAR_AUTH_CACHE_TTL`, default 5 s) still applies to Collar rules
(`@request.auth.role`). A role change can take up to 5 s to reach native
collection rules.

## D-004 · Pawn.Ink hashes registration passwords itself
Collar's hashing package is internal, and generic registration stays off. So
`internal/auth.HashPassword` produces PHC argon2id strings with Collar's exact
parameters (m=64 MiB, t=2, p=1, 16-byte salt, 32-byte key). Collar verifies
them on login (integration test `TestRegisterVerifyLogin`). These parameters
cost about 60–100 ms per hash on the development machine. Recalibrate them
for production hardware together with Collar, never separately, or Collar's
rehash-on-login would diverge.

## D-005 · Collar pools do not parse times
Collar forces `parseTime=false` on its pools, so raw SQL through
`App.DB()` reads `DATETIME` as text. `internal/db.Time` scans these values as
UTC. DSN examples therefore omit `parseTime`; `cmd/migrate` and
`cmd/maintenance` open their own pools and set it explicitly.

## D-006 · Unknown custom routes answered by the wrapper
Collar would route `/api/v1/pawn/x/<anything>` to its generic
`/{app}/{db}/{table}` handler. That handler's 404 lists the collection names.
`server.Routes` records every custom pattern, and ingress answers unknown
`/api/v1/pawn/x/*` paths with the custom 404/405 envelope.

## D-007 · Recurring scheduler buckets
Collar's unique keys only dedupe pending/running jobs. The scheduler inserts
`scheduler_runs(name, bucket)` and enqueues the job in the same transaction
(`INSERT IGNORE`). Each UTC bucket is enqueued at most once across restarts
and multiple workers (`TestSchedulerBucketOnce`).

## D-008 · Per-process rate limits
Auth rate limits are in-process fixed windows (§13.2 knobs). V1 runs one API
process per host. Collar adds its own per-process login throttling. Move the
limiters to Redis or MariaDB if more API replicas are added.

## D-009 · Mail outbox holds one-time links until sent
`email_outbox.template_data_json` holds the verification or reset link until
delivery. Then the worker replaces it with `{}`, and does the same when it
marks the row failed. Token hashes, not tokens, live in `auth_tokens`.
Delivery is at least once: a crash between SMTP acceptance and the `sent`
update can resend one message.

## D-010 · Dev services on non-default ports
The development host already runs MariaDB 10.11 (3306), Redis (6379),
PostgreSQL (5432) and a web server on 80/443 with a Caddy-style admin port
(2019). The disposable Compose stack therefore uses 13306 (MariaDB 11.4.13),
16379 (Redis 7.4.11, AOF everysec, noeviction), 19000/19001 (MinIO) and
11025/18025 (Mailpit), all bound to 127.0.0.1. The API dev port 14021 was
verified free with `ss -ltnp`. MinIO uses the `pgsty/minio` community build
because upstream no longer publishes images.

## D-011 · npm 12 install scripts denied explicitly
npm 12 blocks dependency install scripts by default. Parcel's `@parcel/watcher`,
`lmdb` and `msgpackr-extract`, `@swc/core` and `esbuild` ship prebuilt platform
binaries. Their scripts are only fallback compilers or verifiers. They are
listed as `false` in `web/package.json` `allowScripts`, and a clean
`npm ci && npm run build && npm test` works without them.

## D-012 · Frontend toolchain pins
TypeScript 5.9.3, not 7.x: the native-compiler major is too new to rely on
for `tsc --noEmit`. Vitest 4.1.12 is the first release that clears the
tinypool/@vitest/mocker advisories (dev-only tooling). Parcel empties
`web/dist`, so the build restores the tracked `web/dist/.gitkeep` that
`//go:embed all:dist` needs on a fresh clone.

## D-013 · `collar.ui.yaml` omitted
It is optional, and Collar serves it only to superusers. The Pawn.Ink
dashboards use checked-in typed screen definitions (§1.1 item 6), so there is
no file yet.

## D-014 · Owner bootstrap and roles via CLI
`cmd/maintenance owner-bootstrap` runs once: it refuses when an owner exists.
It creates the owner (or promotes an existing account) as a verified Collar
app user with `role=owner`, never a Collar superuser, and audits the action.
`set-role` grants `author`/`moderator`/`reader` and refuses to touch the
owner. Operator superusers are created separately with
`cmd/collar superuser create`.

## D-015 · Go toolchain go1.26.10
`go.mod` keeps `go 1.26.0`, the language version Collar requires. It adds
`toolchain go1.26.10` because govulncheck reported 32 reachable standard-library
vulnerabilities in 1.26.0 and none in 1.26.10. Raise the toolchain line for
each security patch release.
