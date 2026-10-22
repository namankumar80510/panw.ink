# Test report

## Phase 2 · 2026-10-22

| Command | Result |
|---|---|
| `go run ./cmd/migrate up` (00002, 00003 on MariaDB 11.4.13) | applied |
| `go run ./cmd/seed` twice | seeded; second run is a no-op |
| `go test ./...` | pass (adds `internal/render`, `internal/catalog`) |
| `make test-integration` (`-race`) | 26/26 pass (16 from Phases 0–1 + 10 catalogue) |
| `make lint`, `npm test`, `npm run build`, `make security` | pass / 5/5 / built / 0 reachable vulnerabilities |

| ID | Test | Result |
|---|---|---|
| CAT-01 | `TestShortStoryIsOneText` | pass: one entry, no neighbours, story alias 404s for novels |
| CAT-02 | `TestDatabaseFormatInvariants` | pass: each of 9 invalid writes rejected by its named constraint; a control insert proves the statements are valid. The studio command path arrives in Phase 3. |
| CAT-03 | `TestNovelTOC` | pass: ordered, drafts/archived excluded, no bodies |
| CAT-05 | `TestHiddenContentIs404` | pass: draft/archived entries and draft/hidden works return identical 404s with no title/date/body; native API has no content tables |
| CAT-06 | `TestMarkdownSanitized` + `internal/render` unit tests | pass |
| CAT-07 | `TestShortStoryIsOneText` | pass: anonymous 200, `public, max-age=60` |
| PUB-01/02 (partial) | `TestEarlyAccessAndPublicRelease`, `TestUpcomingEntryLocked` | pass: early/upcoming bodies locked for anonymous and verified readers alike (no Patreon yet), `private, no-store` |
| PUB-04 | `TestEarlyAccessAndPublicRelease` | pass: shifting the clock past `public_at` opens the chapter and story with no worker or job |
| — | `TestCatalogueFiltersAndCursor`, `TestWorkDetailAndRedirect`, `TestSearchIsSafe` | pass: tag intersection, length filters, signed keyset cursors (tamper/other-sort rejected), redirects, injection-shaped search input |

Manual (Chrome, desktop): home rails, work overview with volume grouping and
an early badge, chapter reading, → key to the locked chapter (plain-language
early-access page, no body), contents drawer, short story with no chapter
numbering, no console errors. A fixture prose generator that repeated
sentences was fixed for future seeds; the already-seeded dev database keeps
the old text. Mobile widths remain unverified (window resize unavailable).

## Phases 0–1 · 2026-10-22

Environment: Linux, go1.26.10 toolchain, Node 22.22.3 / npm 12.0.2, dev
Compose stack (MariaDB 11.4.13, Redis 7.4.11 with AOF, Mailpit v1.27).

| Command | Result |
|---|---|
| `make lint` (gofmt, `go vet`, `go vet -tags integration`, `tsc --noEmit`) | pass |
| `go test ./...` | pass: auth, config, cryptox, db, httpx, server |
| `cd web && npm test` (Vitest 4.1.12) | 5/5 pass |
| `make test-integration` (`-race`, throwaway `pi_test_*` databases) | 16/16 pass |
| `make security` (govulncheck v1.1.4, npm audit) | 0 reachable vulnerabilities; npm 0 vulnerabilities |
| `npm ci && npm run build` from a clean `node_modules` | pass; install scripts denied (D-011) |
| `go run ./cmd/migrate up && go run ./cmd/collar migrate && go run ./cmd/collar config check` | applied 00001; Collar `ok (0 warnings)` |

### Acceptance items covered

| ID | Test | Result |
|---|---|---|
| AUTH-01 (account part) | `TestRegisterVerifyLogin` | pass. Comment/follow/Patreon gates arrive with those features. |
| AUTH-02 | `TestLoginThrottleAndNoEnumeration` | pass: identical failures for known and unknown emails; throttled within 20 attempts |
| AUTH-03 | `TestPasswordReset` | pass: reuse rejected, sessions revoked, old password dead |
| AUTH-04 | `TestProfileRejectsPrivilegeFields` | pass: `role`/`email`/`state` rejected (422); native `users` PATCH refused |
| SEC-03 | `TestCSRFAndOrigin` | pass: missing/forged token and foreign Origin rejected |
| COL-03 | `TestRolesOnNativeCollections` | pass: owner gets 403 from `/api/v1/admin/jobs` and collection introspection |
| COL-04 | `TestConcurrentRefresh` | pass: 10 concurrent requests on an expired access token → exactly one rotation, no revocation |
| UI-01 | `TestRoutingBoundaries`, `internal/server` `TestSPARouting` | pass |
| UI-02 | Manual: built binary run from an empty directory with no `web/dist` | pass: shells, hashed JS and woff2 served from the embed; no external URLs in the bundle |
| — | `TestNativeAuthBlocked`, `TestLogoutAndLogoutAll`, `TestSuspensionRevokesImmediately`, `TestChangePassword`, `TestRegisterDoesNotEnumerate`, `TestRegisterValidation`, `TestMailJobScrubsLink`, `TestSchedulerBucketOnce` | pass |

### Manual checks

- Server + worker end to end: `POST /auth/register` → `send_email` job → message in Mailpit.
- Chrome (desktop width): home, sign-in with `?next=/settings` redirect, email
  verification link (token removed from the address bar), settings, and
  light/dark/sepia themes. Studio admin Users and Audit pages, as the owner
  promoted with `maintenance owner-bootstrap`, read native Collar collections.
  The second `owner-bootstrap` run was refused.
- Defects found and fixed during these checks: a blank Settings page (mixed
  keyed/unkeyed Mithril fragment), a missing `UTC` option in the time-zone
  picker, scroll and focus reset on every redraw (now only on navigation), and
  a focus ring around `<main>`.
- **Not yet verified:** mobile widths (the browser window could not be
  resized in this session), keyboard-only pass, axe scan, and the Playwright
  suite (Phase 9).
