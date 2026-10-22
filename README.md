# Pawn.Ink

A reader-first fiction library for original web novels, complete short
stories and anthologies, published by one imprint under several pen names.
Production origin: **https://pawn.ink**. The contract is
[`IMPLEMENTATION.md`](IMPLEMENTATION.md); deviations are logged in
[`docs/DECISIONS.md`](docs/DECISIONS.md).

Proprietary application code. There is no open-source license.

## Status

| Phase | Scope | State |
|---|---|---|
| 0 | Scaffold: Collar embedding, route boundary, embedded Parcel SPAs, dev stack, CI, deploy examples | done |
| 1 | Auth: Collar app auth, encrypted cookie bridge, CSRF, registration/verify/reset, CLI owner bootstrap | done |
| 2 | Catalogue model, sanitized rendering, public reader API, reading shell, fixtures | done |
| 3+ | Studio, library, Patreon, analytics, ranking, moderation, QA | not started |

Test evidence: [`docs/TEST_REPORT.md`](docs/TEST_REPORT.md).

## Stack

- **Go 1.26** (toolchain go1.26.10). It embeds the private
  [Collar](https://github.com/xmfunew/collar) library (v0.1.1) for auth,
  rule-protected native REST, the outbox and the job queue. Custom reader and
  command routes live under `/api/v1/pawn/x`.
- **MariaDB** with two databases: `pawn_ink`, migrated by Goose from
  `migrations/`, and `pawn_ink_collar`, migrated by `collar migrate`.
- **Redis**: the persistent analytics buffer (AOF, noeviction), flushed every
  120 s. It joins the pipeline in Phase 6; `/readyz` already reports it.
- **Parcel + TypeScript + Mithril + Bootstrap 5 + Bootstrap Icons**, bundled
  locally and embedded into the Go binary. There is no CDN and no Node runtime
  in production.

```
browser ──cookie──▶ cmd/server ─┬─ /healthz /readyz
                                ├─ /, /stories, … ──▶ embedded index.html (reader SPA)
                                ├─ /studio/*, /admin/* ──▶ embedded dashboard.html
                                └─ /api/* ──▶ session bridge (cookie → Collar bearer, CSRF, Origin)
                                              └─▶ Collar handler: native /api/v1/pawn/content/* + custom /api/v1/pawn/x/*
cmd/worker ── Collar worker (jobs, outbox) + Pawn.Ink scheduler (+ 120 s Redis flush, Phase 6)
```

## Local development

Prerequisites: Go ≥ 1.26, Node 22 with npm, Docker with Compose, `openssl`.
The private Collar module needs authenticated Git access and
`GOPRIVATE=github.com/xmfunew/collar`, which the Makefile exports.

```bash
make dev-db        # disposable MariaDB 11.4 / Redis 7.4 / MinIO / Mailpit on 127.0.0.1:13306/16379/19000/11025
make dev-env       # writes .env (0600) with fresh random development secrets
make deps          # go mod download + npm ci
make migrate       # Goose (pawn_ink) + collar migrate (pawn_ink_collar) + collar config check
make build-web     # Parcel build into web/dist (Go serves it from disk via WEB_DIST_DIR)
make dev           # Parcel watch + API on http://localhost:14021 + worker
make seed-dev      # optional: §16 demo catalogue + fixture accounts (tests/fixtures/README.md)
make owner OWNER_EMAIL=you@example.com   # one-time owner bootstrap (prompts for a password)
```

Mail goes to Mailpit at http://127.0.0.1:18025. The dev stack never deletes
volumes. Stop it with `make dev-db-stop`.

## Tests

```bash
make lint              # gofmt, go vet, tsc --noEmit
make test              # Go unit + Vitest
make test-integration  # real API against throwaway pi_test_* databases (needs make dev-db)
make security          # govulncheck + npm audit
```

## Production

The deployment is bare metal: **Nginx → systemd → Go binary**, with a separate
worker unit. See `deployments/` for the Nginx server block, the systemd units
and the backup/restore scripts, and [`docs/RUNBOOK.md`](docs/RUNBOOK.md) for
procedures. Nothing is deployed without explicit operator approval.
