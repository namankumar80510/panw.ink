# IMPLEMENTATION.md — Pawn.Ink: Reader-First Fiction Library

**Specification version:** 1.1 · **Baseline date:** 2026-10-22 · **Status:** implementation contract

> **For coding agents:** Build the application described here end to end. Do not reinterpret it as an author SaaS, social network, SEO project, or subscription-paywall business. The initial publisher is one person with multiple pen names. Design the data model to *permit* other real authors to be invited later, but do not build public author onboarding. Treat every item marked MUST as an acceptance requirement. Where external provider behavior is not under our control, use the documented API contract plus a tested failure-safe fallback. Do not silently skip features. Document deviations in `docs/DECISIONS.md` and keep deployment non-destructive.

## 0. Product definition and immutable decisions

Build **Pawn.Ink**, a responsive **SPA-only** fiction library for readers of original web novels, stand-alone short stories, and anthologies. The platform operates as a personally owned fiction imprint in version 1, capable of publishing under multiple pen names. Other authors can be invited later, but **actively recruiting writers is not a business goal**.

**Ownership:** Pawn.Ink application code is **proprietary**. Do not add a `LICENSE` file or an open-source license requirement. Respect third-party dependency licenses and retain required notices separately; a dependency notice does not license the application.

**Canonical identity:** app name **Pawn.Ink**; production origin **https://pawn.ink**; Git repository **https://github.com/namankumar80510/panw.ink.git** (the repository spelling is intentional). Use this identity in UI, metadata, deployment examples and documentation.

### 0.1 Mandatory product principles

1. **Free eventual access:** Every narrative entry (novel chapter or complete short story), including side stories, MUST have a finite public-release time. Patreon supporters may read it sooner. No permanently subscriber-only narrative chapters, locked endings, coin purchases, microtransactions, or per-chapter paid unlocks.
2. **Readers first:** fast, distraction-free reading; no login required to read public material; clear completion states; excellent mobile typography and navigation.
3. **Both formats are first-class:** a short story is **one complete text**, with no chapters or serialization; a novel is a sequence of chapters, optionally divided into volumes. An anthology is an ordered collection of independent short-story works, not a disguised novel.
4. **One writer, many identities:** one operator account can own multiple visible pen names. Pen names are identities, not fake user accounts or simulated reader engagement.
5. **SPA frontend:** Parcel-built TypeScript/Mithril client apps, with Bootstrap 5 and Bootstrap Icons installed through npm/bun and bundled locally; reader and author/admin UIs embedded in Go; client-side history routing; API-driven. No server-side HTML rendering, SEO campaigns, schema-markup program, blogging engine, keyword stuffing, or traffic-chasing content infrastructure. Basic shareable links and accessible page titles are still required.
6. **Original, human-authored fiction:** ban mass-produced AI-generated narrative dumps. Editing tools, proofreading, research, and brainstorming assistance are allowed. No unreliable AI-detector-only bans.
7. **No forum, DMs, social feed, follower-of-reader relationships, coin economy, exclusivity contracts, or forced author acquisition.** Reader comments and reviews are allowed and moderated.
8. **Patreon first:** creator's existing Patreon receives payment. This site verifies paid membership to grant *temporary early access*. No payment processing, PayPal membership verification, revenue split, or platform commission in V1. A plain external donation URL can be shown, but no unlock from an unverified payment claim.
9. **Discovery rewards actual reader satisfaction**, with separate short-story and novel statistics, sample-size adjustment, and exposure for under-discovered works. Never reward word count, chapter count, posting volume, review spam, or raw refreshes.
10. **No unfair terms:** authors own their works; any invited contributor accepts only minimal nonexclusive hosting/display terms, necessary for publishing and basic moderation. No copyright transfer, indefinite exclusive rights, or writer employment contract.

### 0.2 Explicit exclusions

Do **not** build: community forums; private messaging; public writing contests; leaderboards based on chapter count; user-generated reading groups; paid promotional slots; author subscription billing; Patreon-like checkout; AI story-generation functionality; automated reposting/scraping from Royal Road/Scribble Hub; native Android/iOS app; SSR/prerendering for search; news/blog CMS; sophisticated personalized ML recommendation models; reader XP/levels; loot-box rewards; public author self-registration; automatic user-to-author conversion; EPUB/PDF exports in initial release; offline premium caching.

### 0.3 Scope boundaries

**Release V1:** complete multi-pen-name publishing CMS, fiction reader, accounts, progress/bookmarks/follows, ratings/reviews/comments, moderation, basic metrics, privacy controls, scheduled free and early publication, Patreon OAuth v2, engagement-based discovery, responsive SPA, repeatable deployment/backup/testing.

**V1.1 (after live validation):** EPUB/PDF generation with author permission; optional email notifications; reading lists; more advanced recommendations; invite-only external writer workflow; PWA/offline *public* chapters. Do not let V1.1 delay V1.

### 0.4 Success metrics (not fabricated targets)

Measure: qualified readers, first-to-second/third chapter continuation, short-story completion, saves/follows per qualified starter, returning readers, cross-work discovery, successful Patreon linking, entitlement correctness, content publishing reliability, and abuse reports. A dashboard MUST not equate pageviews with completed reads. Show unavailable metrics as `insufficient data`, not zero-quality scores.

---

## 1. Technology choices — fixed for this build

- **Backend base:** embed the private **Collar** Go library (`github.com/xmfunew/collar`) in Pawn.Ink. Use Collar's standard `net/http`/Go `ServeMux`, auth, collection rules, generated REST APIs, outbox and job worker. Do not build a parallel chi/CRUD/auth framework. Go toolchain must satisfy the pinned Collar revision (reviewed checkout requires **Go 1.26.0**). Custom reader APIs use `database/sql`, parameterized SQL and typed DTOs for predictable query cost.
- **Database:** MariaDB 11.x or supported LTS, InnoDB, utf8mb4, UTC. **Goose** owns versioned application migrations in `pawn_ink`; **Collar owns its separate `pawn_ink_collar` system database** and migrations. Both databases must be on the same MariaDB server for transactional outbox/job writes. Runtime credentials have CRUD privileges on these two databases, with separate migration credentials. No ORM.
- **Frontend:** **Parcel + TypeScript + Mithril.js**, SPAs with history routing. **Bootstrap 5 CSS/JS and Bootstrap Icons** supply layout, forms, navigation, tables, dialogs and icons. Install `bootstrap`, `@popperjs/core` where needed, and `bootstrap-icons` through npm/bun; Parcel bundles their CSS, scripts and icon-font assets into the embedded build. **No CDN URLs, remote fonts, Tailwind, Vite or Rspack.** Use minimal local CSS for brand, accessibility and reader typography/themes; reuse Bootstrap components instead of creating a new UI framework. Pin dependencies and the chosen package manager; default to npm + committed `package-lock.json` (`npm ci`), or explicitly document bun + its committed lockfile instead, never conflicting lockfiles.
- **State/data:** lightweight typed `fetch` clients: one for custom reader DTOs, one for Collar's native contracts used directly by dashboards. Small Mithril stores; browser storage only for non-sensitive preferences/anonymous progress. Bootstrap widget instances must be initialized/disposed with component lifecycle hooks.
- **Editor:** CodeMirror 6 Markdown editor; **Markdown is the single source of truth**. Render with Goldmark, sanitize with bluemonday; safe links, no raw HTML execution or arbitrary scripts/iframes.
- **Authentication:** use a Collar app auth collection over Pawn.Ink `users`, configured for Argon2id. Collar issues short-lived JWT access tokens and hashed, rotating opaque refresh tokens; it does **not** natively authenticate cookies. A Pawn.Ink server-side browser-session bridge retains encrypted Collar tokens in MariaDB and gives browsers only an opaque `HttpOnly; Secure; SameSite=Lax` cookie plus CSRF protection. No JWT/refresh token in browser storage or browser-visible auth responses. Verification/reset/email flows are application features; do not assume Collar supplies them. See section 1.1.
- **Jobs:** use **Collar's MariaDB-backed queue** and `collar.NewWorker`, `HandleJob`, `Subscribe`, `Jobs().Enqueue`/`EnqueueTx`; at-least-once execution with idempotent handlers. A dedicated `cmd/worker` starts with V1. Repeating schedules are Pawn.Ink scheduler logic, not a presumed Collar cron API.
- **Analytics buffering:** **Redis is required in V1**. View counts and associated validated reading-event data enter Redis first; the background worker flushes to MariaDB **every 120 seconds**. Redis has persistence and a durable volume, shared by API/worker replicas. Process memory, including an Otter-style cache, is not the authoritative buffer: restarting Go must not erase pending data. See section 10.1.
- **Assets:** local development uses MinIO; production uses S3-compatible storage via Collar aliases/presign/confirm endpoints. Application validation decodes/re-encodes uploaded covers/avatars before making them ready/public; Collar MIME/size checks alone do not replace that. Only site images/assets, no arbitrary executable uploads.
- **Email:** SMTP adapter; local Mailpit; production transactional SMTP; no promotional email without opt-in.
- **Testing:** Go `testing`/`httptest`, disposable MariaDB + Redis + MinIO integration environment, Vitest, Playwright, axe and load smoke tests.
- **Distribution:** Parcel builds reader and dashboard entrypoints into `web/dist/`; a Go package at `web/embed.go` embeds the complete output via `embed.FS` (including Bootstrap CSS/JS/fonts/icons). Reader pages and `/studio/*`/`/admin/*` load the appropriate embedded SPA entrypoint. No external static directory, CDN or Node/bun runtime is required in production. **Every `/api/*` route, including unknown routes, bypasses SPA fallback.**
- **Environment:** **bare-metal Linux VPS deployment: Nginx -> systemd-managed Go API binary**, plus a separate systemd-managed Go worker binary. MariaDB and Redis run as host services (or explicitly configured external services); no production containers, Kubernetes, Caddy or Node/bun runtime. Development/test Compose may supply disposable MariaDB, Redis, MinIO and Mailpit. Integrate the Pawn.Ink server block into the existing Nginx installation without overwriting unrelated configuration.

**Important:** Check package API compatibility and stable versions at build time. Pin all versions in `go.mod`, `package-lock.json`, and documented toolchain versions. Never use floating `latest` in a production deploy script. Implementers may substitute a maintained equivalent only with a written decision log and same public behavior.

### 1.1 Collar integration contract (reviewed source, not assumed APIs)

The local checkout at `~/Documents/Code4/collar` was pulled once successfully on 2026-10-22 (already up to date), then reviewed at commit **`e961180bbd79a996cb07e16ef33fe597073d271b`**. Integration evidence: `QUICKSTART.md`, `README.md` sections 6–8, `collar.go`, `types.go`, `worker.go`, `examples/app`, and the routing/config/auth/records/jobs implementation. The module path is `github.com/xmfunew/collar`; treat the dependency as private despite its GitHub URL. Pin a resolvable immutable Go module version for this commit in `go.mod`/`go.sum`; do not commit an absolute local `replace` or vendor private source into this repository. Use scoped `GOPRIVATE=github.com/xmfunew/collar` and authenticated Git access in development/CI; credentials stay outside Git and logs. Record any later Collar upgrade and rerun compatibility tests.

1. Bootstrap with `collar.New(ctx, collar.Options{ConfigPath: "collar.yaml"})`, register custom handlers **before** serving via `app.Handle`/`HandleFunc`, then serve `app.Handler()` through Pawn.Ink's HTTP wrapper. Shut down HTTP and call `app.Close()`. No imports from Collar's `internal/` packages and no duplicate built-in routes.
2. Configure app namespace **`pawn`**, database alias **`content`** -> physical **`pawn_ink`**, `bigintAsString: true`, explicit collection allowlist, auth collection `content.users`, `globalUsers: false`, `hashAlgorithm: argon2id`, `activeField: is_active`. Add `users.is_active` as a generated boolean derived from `state = 'active'`. Disable generic self-registration; custom registration must enforce reader-only defaults and enqueue verification mail. Keep global Collar account registration disabled too.
3. Run Goose on `pawn_ink` and Collar's `migrate` command on `pawn_ink_collar` separately, then `config check`; reload schema with SIGHUP or authorized `POST /api/v1/admin/reload` after migrations. Automatic runtime DDL is disabled. Implement `cmd/collar` using `collar.Main(os.Args[1:], collar.Options{...})`, the library's public CLI entrypoint for migrate/config/superuser operations; application owner bootstrap is separate from Collar superuser bootstrap.
4. **Custom public and reader-account reads** use reserved prefix **`/api/v1/pawn/x`**. Register method-qualified Go ServeMux patterns (not literal `:param` patterns) and test against Collar's wildcard routes. Use `collar.IdentityFrom`, `RequireApp("pawn", ...)`, `Records().Select`/`AccessClause` where appropriate; optimized raw `DB()`/`ReadDB()` queries must apply visibility, published-revision, ownership and Patreon checks themselves. DB pools have no default database: resolve `app.Database("pawn", "content")`, safely quote trusted identifiers and fully qualify SQL. Critical reads following writes and entitlement decisions use the primary.
5. **Author/admin dashboards call Collar endpoints directly**, e.g. `/api/v1/pawn/content/works`, with native filters, sorts, pagination, projections and expansions. Do not build duplicate dashboard listing/CRUD APIs or optimize them as reader hot paths. Collar's list shape, errors and pagination differ from custom reader DTOs; keep separate typed clients. Thin auth/CSRF/session middleware is shared infrastructure, not a CRUD proxy.
6. Collar currently has **no bundled admin UI**. Build the Pawn.Ink author/admin SPA using Bootstrap 5 and Bootstrap Icons, embedded alongside the reader SPA. `collar.ui.yaml` is optional presentation metadata; `GET /api/v1/pawn/ui` and collection-introspection routes require a **Collar superuser**, so ordinary authors/owners must use checked-in typed screen definitions, not rely on those endpoints. Leave `COLLAR_ADMIN_DIR` unset; Pawn.Ink's wrapper owns `/`, `/studio` and `/admin` HTML routing, including Collar's existing root index route.
7. Ingress browser-session middleware wraps `app.Handler()`: validate the opaque cookie, session expiry/revocation and live user state, serialize refresh-token rotation per session, inject the session's Collar bearer access token before Collar authenticates, enforce Origin/CSRF on all cookie mutations, and strip token-bearing responses. Login/refresh/logout dispatch internally to Collar's existing handlers using documented contracts, without calling unavailable internal auth services or minting replacement JWTs. Store refreshed tokens atomically; concurrent tabs must not trigger refresh-reuse revocation. Distinct login/session paths for application users and restricted operator superusers must prevent privilege mixing. Public reading must still work anonymously if a stale browser session cannot refresh.
8. Normal owner/author/moderator accounts are **app users**, never superusers. Collar superusers bypass all collection rules and some field restrictions: reserve them for explicit operator maintenance, with separate re-authenticated sessions and ingress restrictions. The ordinary admin UI must not silently elevate its owner session. Collar `/api/v1/admin/users` manages **global Collar users**, not Pawn.Ink readers; manage Pawn.Ink reader summaries through authorized `content` collections and account actions through custom commands. Collar jobs/reload/hooks admin APIs require the operator session.
9. Define list/view/create/update/delete rules for every exposed collection, ownership relations and protected fields. Collar rules support only one relation-traversal level and no arbitrary SQL functions; use the ownership columns in section 5.6 instead of an unsupported entry -> work -> pen-name traversal. `null`/absent rule means superusers only; empty string means public. Generated body/revision collections stay locked to readers/guests, including expansions, realtime and generated docs. Secrets use `secret: true` (except the configured password column, which Collar automatically protects and rejects if explicitly configured secret), PII/privilege fields use appropriate `hidden`/`readOnly` settings; do not confuse UI hiding with authorization. Keep raw analytics, browser sessions, OAuth states and Patreon secrets out of the collection allowlist entirely.
10. Collar provides generic validation and **asynchronous** subscriptions, not synchronous application write hooks. Allow generic mutations only where field rules and DB constraints enforce every invariant. For revision creation/rendering, publication, reorder, moderation with transactional audit, roles, token handling and other atomic workflows, use the narrow custom commands in section 7.5. Never repair invalid publishing writes later in a subscription. Custom SQL writes pair `EmitChange`/`EmitDelete` with the same transaction when events are required, and `Jobs().EnqueueTx` for dependent work. Generic metadata updates can produce idempotent audit projections from durable outbox subscriptions; security-critical actions audit synchronously.
11. `collar.NewWorker`/`HandleJob`/`Subscribe` run in `cmd/worker`, with `Run(ctx)` and graceful cancellation. Subscriptions are at least once with bounded retries, not a permanent replacement for durable queued work; enqueue idempotent jobs promptly and monitor lag within outbox retention. API and worker use the same pinned Collar/config version.

### 1.2 UI and build contract

Use Parcel HTML entrypoints `web/index.html` (reader) and `web/dashboard.html` (author/admin), one TypeScript project and shared components. Import installed Bootstrap CSS, selected Bootstrap JS components and Bootstrap Icons CSS from source entrypoints; let Parcel rewrite/bundle local font and asset URLs. Build both into `web/dist` with absolute public asset URLs (`/`) so deep-link reloads resolve correctly. `web/embed.go` exposes the embedded FS to `cmd/server`; Go embed paths cannot reach a parent directory. Test the production binary from an empty working directory with no `web/dist` on disk.

Use Bootstrap containers/grid, navbar/offcanvas, forms, cards, alerts, tables, pagination, modals and toasts; reader text keeps its own width/line-height/theme overrides. Use Bootstrap color modes for light/dark with a small sepia extension. Icons must have accessible labels or be decorative alongside text. Bundle all UI dependencies locally: no runtime requests for framework CSS, JS, icon fonts or webfonts. No remote Bootstrap CDN allowances in CSP. Run TypeScript checks separately (`tsc --noEmit`); Parcel transpilation alone is not type checking. Lazy-load editor/dashboard code and measure all transferred CSS/JS/font bytes, including Bootstrap/Icon assets.

## 2. Repository layout

```text
pawn.ink/
├── IMPLEMENTATION.md
├── README.md
├── .env.example
├── .gitignore
├── Makefile
├── go.mod
├── go.sum
├── collar.yaml             # app/database aliases, explicit collections, auth/rules
├── collar.ui.yaml          # optional operator UI metadata, no secrets
├── cmd/
│   ├── server/main.go       # embed Collar + session wrapper + embedded SPA
│   ├── worker/main.go       # Collar worker + recurring scheduler + Redis flusher
│   ├── collar/main.go       # public Collar CLI entrypoint
│   ├── migrate/main.go
│   ├── seed/main.go
│   └── maintenance/main.go
├── internal/
│   ├── config/
│   ├── server/              # routes, middlewares, JSON errors
│   ├── auth/                # Collar cookie bridge + verification/reset features
│   ├── users/
│   ├── authors/
│   ├── works/
│   ├── editor/
│   ├── publishing/          # state machine, scheduler, entitlements
│   ├── patreon/             # OAuth, API v2, webhook, sync
│   ├── library/             # progress, save, follow
│   ├── reviews/
│   ├── comments/
│   ├── collections/
│   ├── discovery/           # ranking + candidate pools
│   ├── analytics/           # Redis-first events/views -> 120s flush -> aggregates
│   ├── redis/               # shared durable buffer, streams, bounded operations
│   ├── moderation/
│   ├── uploads/
│   ├── jobs/
│   ├── mail/
│   ├── export/              # V1.1 only
│   └── db/                  # explicit SQL repositories/queries
├── migrations/
│   ├── 00001_core.sql
│   ├── 00002_publishing.sql
│   ├── 00003_community.sql
│   ├── 00004_patreon.sql
│   ├── 00005_analytics.sql
│   └── ...                   # actual up/down layout per Goose
├── web/
│   ├── package.json
│   ├── package-lock.json
│   ├── index.html           # Parcel reader entrypoint
│   ├── dashboard.html       # Parcel author/admin entrypoint
│   ├── embed.go             # //go:embed dist; exported embedded FS
│   ├── .parcelrc            # only if defaults need adjustment
│   ├── tsconfig.json
│   ├── src/
│   │   ├── main.ts
│   │   ├── router.ts
│   │   ├── app.ts
│   │   ├── api/                # separate custom-reader and native-Collar clients
│   │   ├── components/         # Layout, Reader, Modals, Toasts
│   │   ├── pages/              # each route in section 4
│   │   ├── admin/              # author/owner dashboard
│   │   ├── styles/             # Bootstrap imports, minimal reader/brand overrides
│   │   ├── stores/             # auth, prefs, reader state
│   │   └── utils/
│   ├── public/
│   └── tests/
├── tests/
│   ├── integration/
│   ├── e2e/
│   ├── fixtures/
│   └── load/
├── deployments/
│   ├── docker-compose.dev.yml
│   ├── nginx-pawn-ink.conf.example
│   ├── pawn-ink.service.example
│   ├── pawn-ink-worker.service.example
│   ├── backup.sh
│   └── restore-check.sh
├── docs/
│   ├── DECISIONS.md
│   ├── API.md
│   ├── DB.md
│   ├── RANKING.md
│   ├── PRIVACY.md
│   ├── RUNBOOK.md
│   ├── MODERATION.md
│   └── TEST_REPORT.md
└── scripts/
    ├── dev.sh
    ├── test.sh
    ├── build.sh
    └── smoke.sh
```

The application MUST run locally without Patreon or a production SMTP credential. The dev Patreon adapter may simulate entitlements only when `ENV=development` and a visible non-production banner is present. Never ship fake entitlements in production.

---

## 3. Domain concepts and publishing rules

### 3.1 Domain objects

- **User:** authentic reader account; role flags: `reader`, `author`, `moderator`, `owner` (roles controlled by DB, not browser).
- **Author/Pen name:** public identity with bio, avatar, slug, owner user ID. Multiple pen names may share the same owner. Do not expose legal identity or automatically disclose shared account ownership.
- **Work:** `novel` or `short_story` (title, synopsis, tags, visibility, cover, status). A short story is never chapter-serialized.
- **Entry:** one novel chapter OR the one full short-story text; same rendering, revision and release infrastructure. A story MUST have exactly one entry with `ordinal=1`; a novel can have many.
- **Volume:** optional grouping of consecutive novel chapters, per-work sorted order; no requirement that every novel has volumes.
- **Collection/Anthology:** curated ordered list of independent short-story work IDs. A collection never changes those stories' standalone reading URLs.
- **Revision:** immutable snapshot of an entry's Markdown after an edit; one current draft revision and one published revision pointer; revision history and rollback.
- **Release:** `early_at` (nullable) and `public_at` (mandatory for every scheduled/published entry). A scheduled early entry becomes readable to authorized supporters at `early_at`, and to all visitors at `public_at`.
- **Entitlement:** verified access to Patreon tier(s) associated with the creator whose work is requested; NOT inferred from browser state or amount of payment alone.
- **Reader library:** progress, follows, saves, statuses, and preferences.
- **Community:** comments on entries; reviews/ratings on works; moderation and reports.
- **Analytics:** privacy-aware read sessions and aggregates; separate from raw HTTP logs.

### 3.2 Invariants — enforce in service AND database where possible

1. `work.kind in {novel, short_story}`. `short_story` has exactly one entry; adding a second is forbidden. It is presented as **Read story**, not Chapter 1.
2. A published/scheduled entry always has `public_at` and immutable public release deadline. `early_at <= public_at` if present; no perpetual premium state. **MAX_EARLY_DAYS=30** between early and public release in V1 (configurable downward, never unlimited).
3. An early entry's content MUST be committed and frozen as a published revision before `early_at` begins. Scheduled revisions may be edited before release; edits after release produce a new published revision with an audit entry.
4. At `now >= public_at`, access is free regardless of Patreon, job execution, login, or external API outages. **Public access is computed from timestamps at request time**, not dependent on scheduler running.
5. At `early_at <= now < public_at`, content is returned only to a verified current entitled user. Return `403` with `EARLY_ACCESS_REQUIRED` otherwise; work metadata and entry title/scheduled public date remain public.
6. Before `early_at` and `public_at`, no reader (paid or free) can retrieve the entry body. Author and owner can preview unpublished revisions through admin-only endpoints.
7. Work and entry must be published/visible. A hidden, archived, or moderation-removed work cannot be bypassed using a known ID or Patreon tier.
8. Owner may unpublish for rights/safety; this is an exceptional, audited action, not a recurring paywall. Reader progress references remain coherent.
9. Only work owners/moderators/owner edit, hide, or remove content as permitted. Author X cannot read private drafts of author Y.
10. Never count an auto-refresh, preview, admin visit, blocked bot, or repeated same-reader event as additional qualified readers.
11. Reviews are one per `(user_id, work_id)`; ratings between 1 and 5, can be edited; no anonymous reviews.
12. Public reading doesn't require an account. Commenting/reviewing/following and Patreon linking require authenticated verified email.
13. Do not implement fake author profiles implying distinct real people with invented biographies; pen names can be distinct brands without deceptive endorsements or fake engagement.
14. All SQL transactions and release operations use UTC; display reader's chosen timezone only for UI.

### 3.3 Editorial workflow

`draft -> scheduled -> published -> archived` (and `hidden` for moderation visibility). `scheduled` may have an early and/or public phase; **access must be derived from dates**, not a mutable `premium_only` flag. Author creates work -> adds entries -> Markdown autosaves draft -> previews -> checks word count / policy -> sets public and optional earlier release -> schedules -> server exposes according to clock -> work can be marked complete. Completion is author-declared when last entry has a public date; warn if declaring complete with missing public releases.

Edits to released entries must preserve public URLs and reading progress. Keep revision audit trail and optionally display 'Updated on' for significant edits. Deleting an entry in an active serialized novel requires confirmation and chapter-order validation; prefer archiving/unpublishing with redirects within the SPA rather than silently renumbering chapters. Store stable `entry_id` and `slug`; `ordinal` for ordering, not identity. No readers should lose their position because the title changes.

---

## 4. Information architecture, frontend routes and interactions

All URLs are **normal history routes**. Direct browser navigation, refresh, and back/forward MUST work. Route matching is in the SPAs; GET reader document paths return embedded `index.html`, and `/studio`, `/studio/*`, `/admin`, `/admin/*` return embedded `dashboard.html`. Missing asset/API requests never return an HTML shell. No hash-only routing. Use `aria-*`, keyboard focus, and accessible headings; target WCAG 2.2 AA for essential flows.

### 4.1 Public routes

| Route | Page | Required behavior |
|---|---|---|
| `/` | Home | Continue reading when available, featured fiction, short reads, ongoing novels, recently updated, discovery picks; no fake stats when catalogue small |
| `/discover` | Explore | Trending, Top Rated, New & Underexplored, Latest, genre filters; tab between Short Stories / Novels / All |
| `/novels` | Novel catalogue | Query, tags, genre, status, length, sort, pagination |
| `/stories` | Short-story catalogue | Query, genre, estimated reading minutes, completed-only (all stories are complete once public), pagination |
| `/collections` | Anthologies | Curated collections, story counts, descriptions |
| `/collections/:slug` | Collection | Ordered independent linked short stories; reader can jump to any |
| `/authors/:slug` | Pen-name profile | Bio, completed and ongoing works, follow button, optional external links |
| `/fiction/:workSlug` | Work overview | Title, cover, synopsis, tags, warnings, author, reviews, public entries, early schedule, continue/start, status |
| `/fiction/:workSlug/ch/:entrySlug` | Novel chapter | Distraction-free reader, previous/next, TOC, save progress, comments; early access message if locked |
| `/stories/:workSlug/read` | Full short story | One uninterrupted story, no chapter list, completion, comments, reactions via save/review only |
| `/reviews/:workSlug` | Reviews | Paginated reviews, rating summary, review composer if eligible |
| `/search` | Search | Works/authors/collections, fuzzy title search optional; no open-web crawler |
| `/support` | Support | Explains free eventual access, Patreon link, tier benefits, connect steps |
| `/policies/{terms,privacy,content,community,copyright}` | Policies | Plain-language policies and reporting contact |
| `/about` | About | Platform promise, short-versus-serial formats, no AI-dump approach |

### 4.2 Reader-account routes

`/login`, `/register`, `/verify-email`, `/forgot-password`, `/reset-password`, `/library`, `/history`, `/settings`, `/settings/patreon`, `/notifications` (in-app follow/update markers; email delivery later). Sign-in from a chapter returns to that chapter afterwards. Continue Reading list stores both novels and partially read short stories. Anonymous local progress is offered for import/merge at registration with user confirmation (never overwrite newer server progress).

### 4.3 Owner/author routes

`/studio`, `/studio/works`, `/studio/works/new`, `/studio/works/:id/edit`, `/studio/works/:id/entries`, `/studio/entries/:id/edit`, `/studio/entries/:id/schedule`, `/studio/collections`, `/studio/authors`, `/studio/analytics`, `/studio/analytics/:workId`, `/studio/comments`, `/studio/reviews`, `/studio/media`, `/studio/integrations/patreon`, `/studio/settings`, `/admin/users`, `/admin/reports`, `/admin/moderation`, `/admin/discovery`, `/admin/jobs`, `/admin/audit`, `/admin/system`.

Studio/admin screens use the shared Bootstrap 5 components and native Collar data client (sections 1.1 and 7.5); both reader and dashboard entrypoints are embedded in the Go application. Studio navigation is hidden from non-authors; API MUST independently enforce permissions. **No public `Become an author` CTA** in V1.

### 4.4 Standard page components

- `Header`: platform brand, discover, short stories, novels, search, library/account. Mobile collapses to simple menu.
- `WorkCard`: cover, title, author, format, genres, status, word count or time, content-warning flag; no invented review score.
- `ReadNow`: resume last unfinished entry; chapter name/progress.
- `ReadingShell`: maximum text width (default 720px), light/dark/sepia, font size 14–26px, font family (serif/sans), line height 1.45–2.0, paragraphs, keyboard shortcuts, chapter nav, TOC drawer, scroll restoration, accessible settings.
- `EntryStatus`: available/free, early-access with public date, upcoming, hidden; avoid confusing 'paywall' language.
- `Discussion`: comments, replies limited to one nested level, spoiler masking, report and author moderation.
- `RatingPanel`: star aggregate, distribution, confidence/sample size when useful, review list, user's editable rating.
- `DiscoveryTabs`: Recently Updated, Trending, Top Rated, New & Underexplored; separate format pivots.
- `Notice`: reader-friendly errors, Patreon outage warnings, login prompts, scheduled date details.

### 4.5 Reader-specific interaction rules

- Public chapters load without cookie or identity; avoid modal login walls.
- Preserve scroll position when returning from comments. Save local scroll/progress throttled, sync server when logged in (on 10–15 sec intervals / meaningful percent changes; don't fire on every scroll pixel).
- Pressing Next opens the next *accessible* chapter, not an inaccessible body; a locked-next page shows release time and Patreon connect action.
- Reader can choose reduced motion, line spacing, width and font; store settings locally and optionally sync per account.
- Mark a short story completed after the user reaches near end *and* passes minimal active-reading sanity conditions; provide explicit `Mark finished` override. Never fake a completion solely due to page render.
- Ratings are on works, comments on entries; short stories can be rated as a whole and commented on as their single entry.
- Do not bury completed fiction under endless ongoing-content widgets. Home must display at least one completed/short-story feature rail.

---
## 5. Database schema (canonical logical contract)

**Conventions:** Every primary key is `BIGINT UNSIGNED AUTO_INCREMENT` unless otherwise stated; foreign keys use matching `BIGINT UNSIGNED`; `created_at`/`updated_at` are `DATETIME(6)` in UTC. Soft deletion uses `deleted_at` only where specified. Use transactions for linked writes. All text uses `utf8mb4`, `VARCHAR` appropriate sizes, InnoDB. Back references, indexes, and check constraints are compulsory. MariaDB CHECK support and syntax must be confirmed against the deployed version, and validated in the application even when CHECK exists. Use `NULL` for unknown/not-applicable, never zero timestamps.

**Schema ownership:** this section defines application tables in `pawn_ink`. Collar system tables (auth sessions, jobs, outbox, cursors, hooks, etc.) live in `pawn_ink_collar` and follow Collar migrations, not copied application schemas. Only an explicit subset of application tables/views is exposed to generated APIs.

### 5.1 Identity, sessions, publisher

**`users`**: `id PK`, `email VARCHAR(254) UNIQUE NOT NULL` (store normalized lowercased), `handle VARCHAR(40) UNIQUE`, `display_name VARCHAR(80)`, `password_hash VARCHAR(255) NOT NULL`, `email_verified_at DATETIME(6) NULL`, `state ENUM('active','suspended','deleted')`, `is_active BOOLEAN GENERATED ALWAYS AS (state = 'active') STORED` (Collar active field), `role ENUM('reader','author','moderator','owner') DEFAULT 'reader'`, `locale VARCHAR(20)`, `timezone VARCHAR(60) DEFAULT 'UTC'`, `reader_settings_json JSON NULL`, `created_at`, `updated_at`, `deleted_at NULL`. Never return password hash. 'Owner' set by one-time CLI bootstrap, never user input.

**`browser_sessions`**: Pawn.Ink's cookie-to-Collar bridge, not a replacement Collar token/session authority. `id PK`, `user_id FK users`, `token_hash BINARY(32) UNIQUE`, `csrf_token_hash BINARY(32)`, `collar_session_id VARCHAR(80)`, `access_token_ciphertext VARBINARY(4096)`, `access_expires_at`, `refresh_token_ciphertext VARBINARY(4096)`, `refresh_version BIGINT UNSIGNED`, `expires_at`, `revoked_at NULL`, `last_seen_at`, `created_at`, optional minimized short-lived security metadata; index `(user_id,expires_at)`. Browser cookie is an opaque random token of at least 256 bits; encrypted Collar credentials never leave the server. Session lifetime is bounded by Collar refresh expiry (configure a maximum 30 days), rotated on login/change-password. Serialize refresh and atomically save replacement tokens. Logout/reset/suspension revoke the bridge immediately and reconcile Collar sessions; do not rely solely on JWT expiry or Collar's short-TTL identity cache. No generated collection exposure. Keep separate operator bridge records/subject identifiers for Collar superusers; never coerce their UUIDs into app-user BIGINT foreign keys.

**`auth_tokens`**: `id PK`, `user_id FK`, `type ENUM('verify_email','reset_password')`, `token_hash BINARY(32) UNIQUE`, `expires_at`, `consumed_at NULL`, `created_at`, index `(user_id,type,expires_at)`; token bytes never logged.

**`pen_names`**: `id PK`, `owner_user_id FK users`, `slug VARCHAR(100) UNIQUE`, `name VARCHAR(140)`, `bio TEXT`, `avatar_asset_id FK assets NULL`, `website_url VARCHAR(500) NULL`, `patreon_creator_connection_id FK creator_connections NULL`, `state ENUM('active','hidden')`, `created_at`, `updated_at`. FK for `creator_connections` may be added by later migration to avoid cycles. Multiple pen names per owner.

**`author_collaborators`** (for invited real people/possible future shared projects): `pen_name_id FK`, `user_id FK`, `permission ENUM('editor','manager')`, `created_at`, composite PK `(pen_name_id,user_id)`. V1 studio creates no public invites; owner may create private pen names. Invited writers in a later release must explicitly accept terms.

**`assets`**: `id PK`, `owner_user_id FK users`, `storage_key VARCHAR(400) UNIQUE`, `mime VARCHAR(100)`, `byte_size BIGINT UNSIGNED`, `width INT NULL`, `height INT NULL`, `sha256 BINARY(32)`, `alt_text VARCHAR(300)`, `state ENUM('pending','ready','quarantined','deleted')`, `created_at`, `deleted_at NULL`; FK references enforce access; never accept remote hotlinked covers as an implicit internal asset.

### 5.2 Fiction catalogue and content

**`works`**: `id PK`, `pen_name_id FK`, `kind ENUM('novel','short_story')`, `slug VARCHAR(160) UNIQUE`, `title VARCHAR(240)`, `subtitle VARCHAR(240) NULL`, `synopsis TEXT`, `cover_asset_id FK assets NULL`, `primary_genre_id FK genres`, `publication_status ENUM('ongoing','completed','hiatus','cancelled')`, `visibility ENUM('draft','listed','unlisted','hidden') DEFAULT 'draft'`, `age_rating ENUM('general','teen','mature') DEFAULT 'teen'`, `language_code VARCHAR(16) DEFAULT 'en'`, `content_warnings_json JSON NULL`, `first_public_at DATETIME(6) NULL`, `last_public_update_at DATETIME(6) NULL`, `completed_at DATETIME(6) NULL`, `is_featured BOOLEAN DEFAULT FALSE`, `created_at`, `updated_at`, `deleted_at NULL`. Indexes `(kind,visibility,last_public_update_at)`, `(primary_genre_id,publication_status)`, `(pen_name_id,created_at)`. `slug` globally unique across works; immutable stable link after first publication unless redirect row created.

**`genres`**: `id PK`, `slug VARCHAR(80) UNIQUE`, `name VARCHAR(100)`, `enabled BOOLEAN`, `display_order INT`. Seed fantasy, progression fantasy, cultivation, isekai, romance, mystery, psychological thriller, crime, horror, sci-fi, literary, comedy, adventure, drama, historical and other. One primary genre per work; secondary tags supply nuance.

**`tags`**: `id PK`, `slug VARCHAR(80) UNIQUE`, `label VARCHAR(100)`, `category ENUM('theme','trope','setting','content','other')`, `is_sensitive BOOLEAN DEFAULT FALSE`; `work_tags(work_id FK,tag_id FK, PRIMARY KEY(work_id,tag_id))`, index `tag_id`. Author may propose tags but moderators maintain canonical tag list; no near-duplicate auto-tag farms.

**`volumes`**: `id PK`, `work_id FK works`, `number SMALLINT UNSIGNED`, `title VARCHAR(160)`, `description TEXT NULL`, `created_at`, `updated_at`, UNIQUE `(work_id,number)`; assert parent is novel. May be empty drafts but do not expose empty volumes publicly.

**`entries`**: `id PK`, `work_id FK works`, `volume_id FK volumes NULL`, `ordinal INT UNSIGNED NOT NULL`, `slug VARCHAR(160)`, `title VARCHAR(240)`, `summary TEXT NULL`, `workflow ENUM('draft','scheduled','published','archived')`, `early_at DATETIME(6) NULL`, `public_at DATETIME(6) NULL`, `current_draft_revision_id FK entry_revisions NULL`, `published_revision_id FK entry_revisions NULL`, `last_edited_at`, `created_at`, `updated_at`, UNIQUE `(work_id,ordinal)`, UNIQUE `(work_id,slug)`, index `(work_id,workflow,public_at)`, index `(workflow,early_at)`, index `(workflow,public_at)`. `ordinal=1` for short stories; reject ordinal>1 for short-story works. `volume_id` only for novels; ensure volume belongs to same work in service. Entries must not render even titles in a hidden work as 'public' content if author visibility is draft.

**`entry_revisions`**: `id PK`, `entry_id FK entries`, `version INT UNSIGNED`, `markdown LONGTEXT NOT NULL`, `rendered_html LONGTEXT NOT NULL` (sanitized at write time), `plain_text LONGTEXT NULL` (optional index source), `word_count INT UNSIGNED`, `estimated_read_seconds INT UNSIGNED`, `checksum BINARY(32)`, `editor_user_id FK users`, `change_note VARCHAR(250) NULL`, `created_at`, UNIQUE `(entry_id,version)`. Revisions immutable. Published entry references a published revision; reading APIs ONLY return that revision, never an unreviewed draft. Enforce maximum entry body size (e.g. 2 MB UTF-8) and editor request limit.

**`collections`**: `id PK`, `pen_name_id FK`, `slug VARCHAR(160) UNIQUE`, `title VARCHAR(240)`, `description TEXT`, `cover_asset_id FK NULL`, `visibility ENUM('draft','listed','unlisted','hidden')`, `created_at`, `updated_at`.

**`collection_items`**: `collection_id FK`, `work_id FK`, `ordinal INT UNSIGNED`, `created_at`, PRIMARY KEY `(collection_id,work_id)`, UNIQUE `(collection_id,ordinal)`. Reject novel work IDs from collections for V1 anthology semantics. A short story can belong to multiple collections.

**`slug_redirects`**: `id PK`, `entity_type ENUM('work','entry','author','collection')`, `old_path VARCHAR(500) UNIQUE`, `new_path VARCHAR(500)`, `created_at`. API returns canonical target; SPA `replaceState` redirect; prevent cyclic mapping.

### 5.3 Reader engagement and community

**`library_items`**: `user_id FK`, `work_id FK`, `status ENUM('saved','reading','completed','dropped')`, `is_favorite BOOLEAN DEFAULT FALSE`, `last_opened_at NULL`, `created_at`, `updated_at`, PK `(user_id,work_id)`; index `(user_id,status,updated_at)`.

**`work_follows`**: `user_id FK`, `work_id FK`, `created_at`, PK `(user_id,work_id)`, index `(work_id,created_at)`.

**`author_follows`**: `user_id FK`, `pen_name_id FK`, `created_at`, PK `(user_id,pen_name_id)`.

**`reading_progress`**: `user_id FK`, `work_id FK`, `entry_id FK`, `fraction DECIMAL(5,4) DEFAULT 0`, `approx_paragraph INT UNSIGNED NULL`, `finished BOOLEAN DEFAULT FALSE`, `finished_at NULL`, `last_read_at`, `updated_at`, PK `(user_id,entry_id)`, index `(user_id,work_id,last_read_at)`; verify entry belongs to work. Note: finishing a short story does not imply auto-follow.

**`reviews`**: `id PK`, `user_id FK`, `work_id FK`, `stars TINYINT UNSIGNED` 1–5, `title VARCHAR(160) NULL`, `body TEXT NULL`, `spoiler BOOLEAN DEFAULT FALSE`, `state ENUM('visible','hidden','removed')`, `qualified_reader_at_review BOOLEAN`, `created_at`, `updated_at`, UNIQUE `(user_id,work_id)`, index `(work_id,state,created_at)`.

**`comments`**: `id PK`, `entry_id FK`, `user_id FK`, `parent_id FK comments NULL`, `body TEXT` (plain/limited Markdown, sanitized), `spoiler BOOLEAN DEFAULT FALSE`, `state ENUM('visible','hidden','removed')`, `created_at`, `updated_at`, `deleted_at NULL`, indexes `(entry_id,state,created_at)`, `(parent_id,created_at)`. Max one reply level (`parent.parent_id` must be null). Soft delete replaces body in public output with `Comment removed` if replies exist.

**`reports`**: `id PK`, `reporter_user_id FK`, `target_type ENUM('work','entry','comment','review','user')`, `target_id BIGINT UNSIGNED`, `reason ENUM('spam','harassment','copyright','plagiarism','ai_dump','unsafe','other')`, `details TEXT`, `state ENUM('open','reviewing','resolved','dismissed')`, `assigned_user_id FK NULL`, `resolution_notes TEXT NULL`, `created_at`, `updated_at`, index `(state,created_at)`, index `(target_type,target_id)`. Check target existence in service since polymorphic FK cannot enforce.

**`moderation_actions`**: `id PK`, `actor_user_id FK`, `target_type`, `target_id`, `action ENUM('hide','restore','remove','warn','suspend','unsuspend')`, `reason TEXT`, `created_at`, index `(target_type,target_id,created_at)`.

**`audit_logs`**: `id PK`, `actor_user_id FK NULL`, `action VARCHAR(100)`, `object_type VARCHAR(60)`, `object_id BIGINT UNSIGNED NULL`, `before_json JSON NULL`, `after_json JSON NULL`, `request_id VARCHAR(60)`, `created_at`, index `(object_type,object_id,created_at)`; no passwords, tokens, private raw reader content or session cookies in logs.

### 5.4 Patreon integration

**`creator_connections`**: `id PK`, `owner_user_id FK`, `provider ENUM('patreon')`, `patreon_campaign_id VARCHAR(64) UNIQUE`, `display_name VARCHAR(160)`, `access_token_ciphertext VARBINARY(4096)`, `refresh_token_ciphertext VARBINARY(4096)`, `token_expires_at NULL`, `webhook_id VARCHAR(80) NULL`, `webhook_secret_ciphertext VARBINARY(4096) NULL`, `status ENUM('active','reauthorize','disabled')`, `last_synced_at NULL`, `created_at`, `updated_at`. V1 configure only **one** creator campaign for all owner pen names; the relationship supports multiple campaigns later. Secrets encrypted with separate encryption key not in DB.

**`patreon_tiers`**: `id PK`, `creator_connection_id FK`, `patreon_tier_id VARCHAR(80)`, `title VARCHAR(160)`, `access_rank SMALLINT UNSIGNED` (0 = no early access; positive ranks ascend), `active BOOLEAN`, `created_at`, `updated_at`, UNIQUE `(creator_connection_id,patreon_tier_id)`; ranks are operator-configured, never inferred solely from currency amount. Higher rank grants lower-rank entitlements on same creator campaign.

**`entry_access`**: `entry_id FK entries PRIMARY KEY`, `creator_connection_id FK`, `minimum_access_rank SMALLINT UNSIGNED`, `created_at`, `updated_at`. Only meaningful before public release. For zero-rank entries, no early locking; if `early_at` exists and is sooner than public, minimum rank must be >=1. Deny ambiguous missing mappings rather than granting unknown tiers.

**`patreon_links`**: `id PK`, `user_id FK users UNIQUE`, `patreon_user_id VARCHAR(80) UNIQUE NOT NULL`, `access_token_ciphertext VARBINARY(4096)`, `refresh_token_ciphertext VARBINARY(4096)`, `expires_at DATETIME(6) NULL`, `status ENUM('active','reauthorize','unlinked')`, `last_verified_at NULL`, `created_at`, `updated_at`. One Patreon ID linked to one site user; unlink erases tokens and entitlement state (do not alter historical reading progress). Tokens encrypted at rest.

**`patreon_memberships`**: `id PK`, `patreon_link_id FK`, `creator_connection_id FK`, `patreon_member_id VARCHAR(100)`, `patreon_campaign_id VARCHAR(80)`, `patron_status VARCHAR(50) NULL`, `current_tier_ids_json JSON`, `verified_access_rank SMALLINT UNSIGNED DEFAULT 0`, `last_charge_status VARCHAR(80) NULL`, `entitlement_expires_at DATETIME(6) NULL` (only if reliably available), `last_checked_at`, `next_check_at`, `verification_state ENUM('valid','unknown','revoked')`, `created_at`, `updated_at`, UNIQUE `(patreon_link_id,creator_connection_id)`; store only fields necessary for entitlement. Patreon v2 `currently_entitled_tiers` is the authority, not historical lifetime pledge totals.

**`patreon_webhook_events`**: `id PK`, `creator_connection_id FK`, `event_key CHAR(64) UNIQUE` (SHA-256 over normalized event type + authenticated raw body; replay protection), `trigger VARCHAR(100)`, `raw_hash BINARY(32)`, `processing_status ENUM('queued','processed','failed')`, `received_at`, `processed_at NULL`, `error_code VARCHAR(100) NULL`. Never retain full raw webhook payload indefinitely; log only the fields needed to reconcile.

### 5.5 Analytics, discovery and jobs

**`read_sessions`**: `id PK`, `reader_key_hash BINARY(32)` (rotating pseudonymous identifier; optionally user-derived), `user_id FK NULL`, `work_id FK`, `entry_id FK`, `first_seen_at`, `last_seen_at`, `active_seconds INT UNSIGNED`, `max_scroll_fraction DECIMAL(5,4)`, `completed BOOLEAN`, `qualified BOOLEAN`, `device_class ENUM('mobile','desktop','tablet','unknown')`, `source_type ENUM('direct','internal','external','other')`, `ingestion_state ENUM('normal','suspicious','ignored')`, `created_at`, `updated_at`; indexes `(work_id,first_seen_at,qualified)`, `(entry_id,first_seen_at)`, `(reader_key_hash,entry_id,first_seen_at)`. **Data minimization:** retention 30 days max for raw sessions; aggregation survives after deletion.

**`analytics_events`**: `id PK`, `client_event_id CHAR(36) UNIQUE`, `reader_key_hash BINARY(32)`, `user_id FK NULL`, `work_id FK NULL`, `entry_id FK NULL`, `event_type ENUM('open','heartbeat','progress','completed','continue','save','follow','review','discover_impression','discover_click')`, `event_at`, `active_seconds_delta SMALLINT UNSIGNED DEFAULT 0`, `scroll_fraction DECIMAL(5,4) NULL`, `source VARCHAR(40)`, `payload_json JSON NULL`, `received_at`; indexes `(event_at,event_type)`, `(work_id,event_at)`, `(reader_key_hash,work_id,event_at)`. Data accepted only via restricted schema, max batch size and event size; retention 30 days. Aggregator deduplicates and filters; not every HTTP request is a reading event.

**`work_metrics_daily`**: `(work_id, day_utc DATE, format ENUM('novel','short_story'), discovery_impressions INT, discovery_clicks INT, raw_views BIGINT UNSIGNED, unique_starters INT, qualified_readers INT, qualified_completers INT, continuation_qualifiers INT, return_qualifiers INT, new_saves INT, new_follows INT, valid_reviews INT, rating_sum INT, suspicious_events INT, public_reads INT, updated_at)`, PK `(work_id,day_utc)`. Counting distinct users across days needs a separate rolling dedupe key/cache — summing daily uniques alone is NOT a 28-day unique audience. Recompute rolling windows from sanitized session data or time-bucket dedupe table. Counts are populated by the Redis flush/aggregate pipeline, never by generic dashboard writes.

**View-count storage:** `work_metrics_daily.raw_views` records validated raw opens separately from qualified engagement; add `entry_metrics_daily(entry_id FK, day_utc DATE, raw_views BIGINT UNSIGNED, updated_at, PRIMARY KEY(entry_id,day_utc))` for chapter/story-entry counts. Both are flushed from newly inserted stream events every 120 seconds; raw views never count as unique readers or ranking votes. Metadata (work/entry, event UUID, timestamp, pseudonymous reader/session and eligibility flags) travels with the Redis event, not only a counter.

**`qualification_ledger`**: `window_start DATE`, `work_id FK`, `reader_key_hash BINARY(32)`, `qualified_start BOOLEAN`, `qualified_completion BOOLEAN`, `qualified_continue BOOLEAN`, `qualified_return BOOLEAN`, `saved BOOLEAN`, `followed BOOLEAN`, `updated_at`, PK `(window_start,work_id,reader_key_hash)`; TTL/partition deletes after 60 days; use rolling 28-day boundaries. Needed so rate denominators use unique humans, not summed daily counts. Use account ID when logged in, rotated anon key when not, with conservative dedupe when merging. Guest-to-user merging never increases counts.

**`discovery_impressions`**: `id PK`, `viewer_key_hash BINARY(32)`, `work_id FK`, `surface ENUM('home','discover','genre','search','related')`, `rank_position INT`, `algorithm_version VARCHAR(40)`, `shown_at`, `clicked_at NULL`, `qualified_click BOOLEAN`, index `(work_id,surface,shown_at)`, short TTL 30 days. Record impression only when visible >=50% for >=1 second, batched; not merely sent by the API. Do not expose individual viewer identities to author dashboard.

**`ranking_snapshots`**: `id PK`, `work_id FK`, `format ENUM('novel','short_story')`, `category VARCHAR(60)` (trending/top_rated/discover), `window_start`, `window_end`, `algorithm_version`, `score DECIMAL(8,4)`, `sample_size INT`, `parts_json JSON`, `generated_at`, UNIQUE `(work_id,category,window_end,algorithm_version)`; index `(format,category,generated_at,score)`.

**Collar system `jobs`**: reuse Collar's actual queue in `pawn_ink_collar`; do not create the previously prescribed application `jobs` table. At the reviewed revision, states are `pending|running|done|failed`; `EnqueueOptions` provides `Queue`, `RunAt`, `Delay`, `MaxAttempts`, `UniqueKey`. Unique keys dedupe pending/running work and are reusable after completion; permanent business dedupe belongs in application constraints/receipts. Recovery/reaping, retry backoff and admin operations are supplied by Collar.

**`analytics_flush_batches`**: `batch_id VARCHAR(100) PRIMARY KEY`, `first_stream_id VARCHAR(80)`, `last_stream_id VARCHAR(80)`, `event_count INT UNSIGNED`, `committed_at`; receipts for transactionally applied Redis batches. Per-event `client_event_id` uniqueness remains the durable replay protection across different batch groupings. Counters are derived only from newly inserted eligible events (not blindly incremented again on replay). Receipts/dedupe records cannot expire before the permitted Redis replay horizon. These tables are not writable through generated dashboard APIs.

**`notifications`**: `id PK`, `user_id FK`, `kind ENUM('chapter_public','work_update','reply','moderation','system')`, `reference_type VARCHAR(50)`, `reference_id BIGINT`, `read_at NULL`, `created_at`, `dedupe_key VARCHAR(160) UNIQUE`, index `(user_id,read_at,created_at)`.

**`email_outbox`**: `id PK`, `user_id FK NULL`, `recipient VARCHAR(254)`, `template VARCHAR(100)`, `template_data_json JSON`, `state ENUM('queued','sent','failed')`, `attempts SMALLINT`, `next_attempt_at`, `created_at`, `sent_at NULL`; use for verification/reset, not unsolicited marketing.

**`site_settings`**: `key VARCHAR(120) PRIMARY KEY`, `value_json JSON`, `updated_by FK users`, `updated_at`. Only whitelisted mutable keys; secrets ALWAYS in environment/secrets manager.

### 5.6 Schema implementation notes

- **Collar ownership projection:** add `owner_user_id FK users` to `works`, `entries`, `entry_revisions`, `volumes`, `collections` and other nested author-owned collections as needed. Maintain consistency with parent ownership using composite UNIQUE/FK constraints and DB validation; reject owner changes through generic updates. Native creates may supply the caller's owner ID only where a create rule checks it and the parent relationship atomically; complex creates use commands. Rules can then use `owner_user_id = @request.auth.id` plus explicit permitted roles, or a single parent relation such as `work_id.owner_user_id`. Read-only analytics views project work ownership without exposing reader keys. Do not assume multi-hop relation traversal works. Test author/editor collaborator access; use an explicit permission projection/view where direct ownership is insufficient.
- Because `entries` ↔ `entry_revisions` cross-reference, create tables first without revision-pointer FKs and add those constraints in a subsequent migration. Validate revision points to same entry inside the service transaction.
- Use `ON DELETE RESTRICT` for valuable content and legal audit references; use explicit safe cleanup commands and soft deletion rather than cascading away all analytics and work history.
- Include indexes needed by queries; inspect production slow query logs and `EXPLAIN` on catalogue/discovery and TOC queries.
- Use MySQL/MariaDB FULLTEXT on work title + synopsis for V1 search; implement relevance with safe bounded queries. Later consider a dedicated search engine only with evidence.
- JSON columns are not an excuse to avoid normalization of tiers, works or readers; JSON for settings/warnings/metadata only.
- Add a database migration guard that refuses to apply destructive DDL during ordinary deploy without explicit backup/checklist.

---

## 6. Roles, permissions, content ownership

| Action | Anonymous | Reader | Author (own only) | Moderator | Owner |
|---|---|---|---|---|---|
| Read public fiction | Yes | Yes | Yes | Yes | Yes |
| Read Patreon-early fiction | No | With verified matching entitlement | With verified entitlement, or own preview | Only if personally entitled; moderation exception audited | Yes (administrative preview) |
| Save/follow/progress sync | Local only | Yes | Yes | Yes | Yes |
| Post review/comment | No | Verified email | Verified email | Verified email | Verified email |
| Create work/entry/collection | No | No | Own pen names | No, unless also author | Yes |
| Edit others' work | No | No | No | Hide pending review only | Yes with audit |
| Moderate comments/reports | No | No | Own work discussions, constrained | Yes | Yes |
| Modify ranking weights | No | No | No | No | Yes, audited, versioned |
| Manage Patreon creator integration | No | No | Own connected creator only (future) | No | Yes |
| Read analytics | No | Own library only | Aggregated own work | Abuse-only views | All aggregates |
| Invite author / change role | No | No | No | No | Owner-only CLI/admin |

**Authorization implementation:** enforce `user -> pen_name owner` or collaborator permission -> work -> entry ownership in one repository/service check; never trust `author_id` from client. Author analytic reports must enforce work ownership. Admin privilege operations write `audit_logs` records. A moderator must not be given Patreon tokens or individual reader histories.

---

## 7. HTTP/JSON API specification

**Custom base:** `/api/v1/pawn/x` (all relative paths below use this base unless an absolute Collar path is explicitly stated). **Native Collar base:** `/api/v1/pawn/content/{table}` plus its documented auth/storage/admin routes. Native Collar endpoints retain their own contracts; the uniform envelopes, decimal IDs, status mapping and cursor conventions below apply to custom endpoints only. `Content-Type: application/json; charset=utf-8`, ISO 8601 UTC timestamps, UTF-8 strings, stable error codes. IDs encoded as decimal strings in JSON to avoid JS integer precision issues; `next_cursor` is opaque server-signed cursor. GET requests must not mutate state. Query sizes bounded (`limit <= 50`, default 20). All write endpoints require session + CSRF + appropriate role. Mutations use `Idempotency-Key` where duplicate submissions are costly (publishing, moderation, Patreon link callback).

### 7.1 Custom API envelopes

```json
{"data":{"id":"123","title":"Holunkai"},"meta":{"request_id":"uuid","next_cursor":null}}
```

```json
{"error":{"code":"EARLY_ACCESS_REQUIRED","message":"Available publicly on 2026-11-15","details":{"public_at":"2026-11-15T00:00:00Z","min_access_rank":1}},"request_id":"uuid"}
```

Custom domain errors must use the custom response writer (or `app.WriteJSON` with the explicit envelope/status); do not pass unknown uppercase application codes to `collar.NewError`/`WriteError`, which map unknown codes to 500.

HTTP status mapping: `200` read/update, `201` create, `202` queued, `204` delete/empty, `400` malformed, `401` not authenticated, `403` permission/early access, `404` not found/hidden, `409` conflict/version mismatch, `422` validation, `429` rate limited, `500` internal, `503` provider outage if live sync explicitly requested. All errors must be renderable in SPA.

### 7.2 Public read endpoints

| Method/path | Parameters | Returns |
|---|---|---|
| `GET /healthz` | none | Process alive; no sensitive build details |
| `GET /readyz` | none | DB/schema ready; Redis/worker degradation reported separately and does not stop public reading; Patreon not required |
| `GET /api/v1/pawn/x/home` | optional `format` | Featured, completed, latest, short reads, safe discovery selections |
| `GET /api/v1/pawn/x/discover` | `surface=trending|top_rated|new|latest`, `format`, `genre`, `tags`, `status`, `limit`,`cursor` | Eligible work cards and source labels |
| `GET /api/v1/pawn/x/works` | `kind`, `genre`, `tags`, `status`, `q`, `sort`, `min_words`, `max_words`, `max_minutes`, `cursor` | Filtered published works |
| `GET /api/v1/pawn/x/works/:slug` | none | Public work metadata, author, tag list, rating summary, reader-specific status if logged in |
| `GET /api/v1/pawn/x/works/:slug/toc` | none | Public + early-entry title/ordinal/volume/release dates/access flags; never drafts |
| `GET /api/v1/pawn/x/works/:slug/entries/:entrySlug` | none | One published revision body if public/entitled; otherwise typed lock response |
| `GET /api/v1/pawn/x/stories/:slug/read` | none | Convenience alias for single-entry short-story body; same entitlement checks |
| `GET /api/v1/pawn/x/authors/:slug` | none | Public pen name and listed work cards |
| `GET /api/v1/pawn/x/collections` | filters | Published anthology collections |
| `GET /api/v1/pawn/x/collections/:slug` | none | Collection and public story card list |
| `GET /api/v1/pawn/x/search` | `q`, `type=all|works|authors|collections`, `limit`, `cursor` | Sanitized search results |
| `GET /api/v1/pawn/x/works/:slug/reviews` | `sort=newest|helpful|high|low`, `cursor` | Visible review list and rating distribution |
| `GET /api/v1/pawn/x/entries/:entryId/comments` | `cursor`, `sort=oldest|newest` | Visible comments + single-level replies |
| `GET /api/v1/pawn/x/genres` | none | Genre metadata |
| `GET /api/v1/pawn/x/tags` | optional `q` | Approved tags only |
| `GET /api/v1/pawn/x/public-config` | none | Brand, public feature flags, limits, Patreon campaign URL, never secrets |

Sample public entry `data`: `{id,work_id,work_kind,title,slug,ordinal,volume,public_at,early_at,access:{state:'public'|'early'|'upcoming'},body_html,word_count,read_seconds,previous,next,published_revision}`. If not entitled, remove `body_html` entirely. Cache-control: `public, max-age=60` or CDN-specific for completely public content, `private, no-store` for entitlement-dependent or user-personalized responses. No CDN caching for early bodies.

### 7.3 Authentication and profile

Browser-facing routes here implement the bridge/application features. They must not collide with or pretend to replace native `POST /api/v1/pawn/auth/content/users/login`, `POST /api/v1/auth/refresh`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/me`, `GET /api/v1/auth/sessions`, `DELETE /api/v1/auth/sessions/{id}` and `POST /api/v1/auth/change-password`. Internal dispatch uses Collar contracts; browser ingress denies direct token-issuing login/refresh endpoints so responses cannot expose credentials. Generated user registration and user mutations are locked; custom registration/verification/reset applies safe fields and application policy.

| Method/path | Request fields | Behavior |
|---|---|---|
| `POST /auth/register` | `email,password,handle?,display_name` | Creates unverified reader, sends verification; rate-limited |
| `POST /auth/login` | `email,password` | Internally calls Collar app login with `identity,password`; stores encrypted tokens in bridge, sets opaque cookie, returns only public user DTO |
| `POST /auth/logout` | none | Revokes session and clears cookie |
| `POST /auth/logout-all` | none | Revokes all sessions for user |
| `GET /auth/me` | none | Current reader and permissions; `401` if not logged in |
| `POST /auth/verify-email` | `token` | One-time verify; logs safe audit event |
| `POST /auth/resend-verification` | `email` | Non-enumerating response, rate-limited |
| `POST /auth/password/forgot` | `email` | Non-enumerating response; email token |
| `POST /auth/password/reset` | `token,new_password` | Rotates sessions and consumes token |
| `POST /auth/password/change` | `old_password,new_password` | Changes password, rotate/revoke sessions |
| `PATCH /users/me` | `display_name,timezone,settings` | Allowed profile fields only |
| `DELETE /users/me` | confirmation | Queue GDPR-style account deletion/anonymization with safety/legal retention policy |
| `GET /users/me/sessions` | none | Session list; sensitive details redacted |

### 7.4 Reader library and social actions

| Method/path | Request | Behavior |
|---|---|---|
| `GET /me/library` | `status,sort,cursor` | Saved/reading/completed/dropped works |
| `PUT /me/library/:workId` | `status,is_favorite` | Idempotent upsert |
| `DELETE /me/library/:workId` | none | Removes library item |
| `PUT /me/works/:workId/follow` | none | Follows work (unique) |
| `DELETE /me/works/:workId/follow` | none | Unfollow |
| `PUT /me/authors/:authorId/follow` | none | Follows pen name |
| `DELETE /me/authors/:authorId/follow` | none | Unfollow |
| `GET /me/progress` | cursor | Last reading state |
| `PUT /me/progress/:entryId` | `fraction,approx_paragraph,finished,occurred_at` | Validate range and work/entry relationship; monotonic within one session unless explicit rewind; secure merge |
| `POST /works/:workId/reviews` | `stars,title?,body?,spoiler` | Creates one user review; email verified |
| `PATCH /reviews/:reviewId` | allowed review fields | Owner edit or moderator action |
| `DELETE /reviews/:reviewId` | none | Soft remove user review |
| `POST /entries/:entryId/comments` | `body,parent_id?,spoiler` | Sanitized, one level deep, rate-limited |
| `PATCH /comments/:commentId` | `body,spoiler` | Own comment editable within configurable period |
| `DELETE /comments/:commentId` | none | Soft-delete own comment |
| `POST /reports` | `target_type,target_id,reason,details` | Reports inappropriate material; deduped per reporter/target/window |
| `GET /me/notifications` | `cursor,unread_only` | In-app recent notices |
| `POST /me/notifications/:id/read` | none | Marks read |

### 7.5 Author/admin dashboards: direct Collar endpoints

Dashboards use these **native absolute paths**, with the shared cookie bridge injecting the caller's own bearer token on the server. Preserve Collar request/response shapes, `filter`, `sort`, `fields`, `expand`, `page`/`perPage`, `cursor`, `skipTotal` and composite-ID encoding; do not wrap them in custom reader envelopes. Bound page sizes and query complexity. Every collection has ownership/role rules; client filters are convenience, never security.

| Dashboard function | Native endpoint / access |
|---|---|
| Pen names | `GET/POST /api/v1/pawn/content/pen_names`; `GET/PATCH .../pen_names/{id}` for allowed metadata |
| Works | `GET/POST /api/v1/pawn/content/works`; `GET/PATCH .../works/{id}` for safe metadata only |
| Entries, volumes and revisions | `GET /api/v1/pawn/content/entries`, `.../volumes`, `.../entry_revisions`, and their ID routes; owned drafts visible only to permitted authors/staff; creation/scheduling/version changes use commands below |
| Collections | `GET/POST /api/v1/pawn/content/collections`; `GET/PATCH .../collections/{id}`; `GET .../collection_items`; ordered replacement uses a command |
| Taxonomy | `GET /api/v1/pawn/content/genres`, `.../tags`, `.../work_tags`; approved staff maintain taxonomy through permitted native mutations |
| Analytics | `GET /api/v1/pawn/content/work_metrics_daily`, `.../ranking_snapshots`, and read-only aggregate views with explicit configured primary keys; authors see only their works, no raw reader keys |
| Community moderation inbox | `GET /api/v1/pawn/content/comments`, `.../reviews`, `.../reports`; author visibility limited to permitted own-work context, reporter identity excluded |
| Audit and moderation history | `GET /api/v1/pawn/content/audit_logs`, `.../moderation_actions`, role restricted; no generic mutation |
| Account management | `GET /api/v1/pawn/content/admin_user_summaries` (read-only staff-safe view); role/suspension/deletion through commands, never unrestricted user PATCH |
| Feature/ranking/settings | Native reads of allowlisted `site_settings`, `ranking_snapshots` and permitted safe settings writes; ranking activation stays audited command |
| Asset metadata/storage | `GET /api/v1/pawn/content/assets`; Collar `/api/v1/pawn/storage/{alias}/presign`, `/confirm`, `/url`, `/object` as appropriate; uploaded objects remain quarantined until application validation finishes |
| Operator job management | `GET /api/v1/admin/jobs`, `POST /api/v1/admin/jobs/{id}/retry`, `DELETE /api/v1/admin/jobs/{id}` require a distinct operator superuser session; ordinary app owner has no implicit access |

Collar generic writes cannot implement arbitrary multi-table transactions, conditional revision swaps or sanitization hooks. Only the following **business commands** remain custom under `/api/v1/pawn/x`; they supplement the native APIs rather than recreating dashboard CRUD:

| Method/path | Required atomic behavior |
|---|---|
| `POST /studio/works/:id/initialize-story` | Create exactly one short-story entry under lock, idempotently; native work creation leaves it draft until initialized |
| `POST /studio/works/:id/complete`, `/archive` | Validate completion/archive, preserve references, audit |
| `POST /studio/works/:id/volumes`, `/entries` | Create novel-only volume/entry with locked ordering and story invariants |
| `POST /studio/volumes/:id/reorder` | Validate parent and serialize order changes |
| `PUT /studio/entries/:id/draft` | `markdown,editor_version,expected_revision`; create immutable sanitized revision and atomically swap draft pointer, `409` on conflict |
| `POST /studio/entries/:id/preview` | Sanitized own-work preview, no public body URL |
| `POST /studio/entries/:id/restore` | New revision from a prior revision, never mutate history |
| `POST /studio/entries/:id/schedule`, `/publish-now`, `/unpublish`, `/reorder` | Enforce dates, ownership, frozen revision, tier map, stable IDs, audit and job enqueue in one transaction |
| `PUT /studio/collections/:id/items` | Atomic ordered story-only replacement |
| `POST /studio/assets/:id/validate` | Decode/re-encode quarantined upload, check owner/type/dimensions, set ready only on success; bounded local upload command allowed for tests |
| `POST /studio/comments/:id/hide` | Restricted own-work moderation with reason and synchronous audit |

Unknown/unsafe fields are rejected; server-managed ownership, revision pointers, computed fields, release/privilege/secret fields are protected against native PATCH as well. Generic reads must never provide an alternative path to locked text. Upload limit 5 MB, JPEG/PNG/WebP only, decode/re-encode, no SVG. MinIO development follows the same storage lifecycle as production.

### 7.6 Owner/moderator commands and operator boundary

Standard reports/audit/users/settings/snapshot listings use the native endpoints above. Custom `/admin/actions`, `/admin/users/:id/suspend`, `/restore`, `/admin/ranking/config`, `/admin/ranking/rebuild`, `/admin/featured` and account-role/deletion commands perform domain validation, confirmation/re-authentication where needed, transactions and audit. `/studio/patreon/tier-map`, `/studio/patreon/status` and creator connection commands are owner-only custom integration operations; no generic token/secret collections. `/admin/system/status` reports sanitized application/Redis/worker health.

For direct generic metadata updates, use Collar rules + field/DB validation and durable outbox-driven audit projections. For protected publishing/moderation/role operations, ingress blocks the equivalent native mutations, including operator sessions in the normal dashboard; commands are the only application write path. Explicit CLI/break-glass maintenance remains operator controlled. Test direct HTTP requests, not just hidden UI controls. Collar superuser bypasses cannot be made safe merely by `readOnly`/`hidden` flags.

### 7.7 Analytics ingestion API

`POST /api/v1/pawn/x/events` accepts `[{client_event_id,event_type,work_id,entry_id?,active_seconds_delta?,scroll_fraction?,occurred_at,source?}]` up to 25 per batch, max 16 KB total. Write accepted events and their view/session metadata to durable Redis first, then respond `202` with count accepted/ignored only after Redis acceptance. An unavailable/full buffer returns bounded `503 ANALYTICS_UNAVAILABLE` with retry advice; never claim persistence, silently use process memory or synchronously fall back to per-view SQL. Public reading remains available. Duplicate retries preserve client UUIDs. The worker flushes every 120 seconds (section 10.1). Backend validates session, timestamp skew, eligibility, IDs, sequence plausibility and rate limits; do not trust completion events without supporting activity; use opaque anonymous cookie/session token on guests, not fingerprinting. `GET /api/v1/pawn/x/me/analytics-optout` and `PUT /api/v1/pawn/x/me/analytics-optout` for optional metrics preference. Essential progress synchronization is separate from analytics consent.

---
## 8. Ranking and discovery — exact V1 mathematics

**Objectives:** A great 2,500-word story may rank alongside a long novel; a 5-chapter novel is not penalized merely for being shorter than a 400-chapter epic; a story with 3 enthusiastic readers should not unfairly dominate a story with 500 satisfied readers; no direct points for update spam, total words, raw pageviews, or comment volume. **Separate format-specific scoring and pools**. All weights/version parameters live in versioned `discovery/config.go` and `docs/RANKING.md`; changes must create new `algorithm_version` and a side-by-side comparison before activation.

### 8.1 Basic definitions

- **Reader key:** logged-in user ID (pseudonymously HMACed for analytics) OR a site-issued first-party random anonymous ID persisted as a limited-duration cookie. Never use permanent fingerprinting. Anonymous guests with blocked storage can read but cannot reliably contribute a unique-engagement vote; this is acceptable.
- **Qualified start (QS):** a unique reader opened a work and accrued at least `min(45 seconds active foreground, 15% of estimated text reading time)` **and** at least 15% content scroll progress for a >=300-word entry, OR manually advanced meaningfully to a subsequent chapter. For shorter entries use a proportional threshold. JS heartbeat capped at 15 seconds/request and cross-validated against real elapsed time; event claimed active time never exceeds session wall time. This is anti-spam friction, not a hard claim of human identification.
- **Qualified completion (QC), short stories:** reader reached >=90% text position, accrued >=35% expected reading time (estimated at 230 wpm with reasonable lower bound), and reading session appears plausible; a reader pressing `Mark finished` is respected as personal progress but not automatically a ranking-qualified completion. Count at most once per reader/work/window.
- **Qualified continuation (QCont), novels:** qualified starter subsequently opened and meaningfully read Chapter 2 or a later chapter within 14 days, when Chapter 2 is publicly available to that reader. For novels with only one public chapter, **continuation is unavailable**, not zero; do not rank the work as low-quality because it has not published Chapter 2. This avoids weighting 300 chapters over 3.
- **Qualified return (QR), novels:** qualified starter returned to that work on a distinct UTC date within 14 days after first start and made meaningful progress. Restrict eligible cohort to readers with sufficient elapsed time and a follow-up chapter opportunity. Do not penalize a finished very short novel if no new chapters exist.
- **Save/follow conversion (SF):** unique qualified starters that saved, favorited, or followed the work within 14 days, counted once per reader; remove reversals if they occur before window closes. A reader cannot supply three separate votes by saving + following + favoriting.
- **Quality (Q):** average 1–5 star reviews, **one unique verified user review per work**, with stronger treatment for qualified readers. Rating uses Bayesian smoothing to avoid five-star bombing.
- **Discovery impression:** item >=50% visible within viewport for >=1 second; dedup one impression per `(viewer,work,surface,day)` for exposure accounting. This does not itself boost ranking.

Timing: use UTC, windows of 28 days for quality/engagement and trailing 7 days for trend momentum; compute only mature cohorts (allow 14 days to judge follow-up outcomes) or explicitly model pending denominators. Works with <3 qualifying readers in a cohort display `Insufficient data`; they are eligible for exploration and Latest, not shown as deceptively perfect 100% engagement.

### 8.2 Bayesian/shrinkage adjustment

For every proportion signal `p = successes / eligible`, use:

```text
smoothed(successes, eligible, prior, k)
  = (successes + prior * k) / (eligible + k)
```

`prior` is the historical mean for comparable **format** (novel/short), optionally genre if cohort >=1000 eligible readers. Initially use `prior=0.50` for engagement/conversion only as a neutral placeholder; replace with empirical aggregate once volume permits. `k=40` for short-story metrics and `k=80` for novel metrics. Return `null` for metrics without an opportunity/eligible denominator; **redistribute remaining weights**, don't pretend absent=0 or perfect=1. Persist component numerators, denominators, priors and adjusted values for audit. The algorithm MUST be tested against hand-calculated fixtures.

For ratings: Bayesian rating `R_raw=(sum_stars + 3.5 * 10)/(review_count + 10)` and `Q=(R_raw-1)/4`, clamped 0..1. Unqualified accounts' public reviews remain visible but suspicious/abusive ratings are excluded from ranking pending moderation. Avoid hiding legitimate criticism just for reducing a work's score. A rating is not considered statistically stable until 5 independent non-suspicious reviews; before that weight is redistributed to engagement.

### 8.3 Format-specific primary scores

**Short stories** (available as complete single texts):

```text
E_short = smoothed(unique qualified completions, eligible qualified starts, p_completion, 40)
F_short = smoothed(unique saves/follows, eligible qualified starts, p_save, 40)
Q_short = Bayesian confidence-adjusted normalized rating

Score_short = 100 * weighted_mean({E_short: 0.55, F_short: 0.30, Q_short: 0.15},
                                  omit unavailable components)
```

**Novels** (only eligible starts with continuation opportunities):

```text
E_novel = smoothed(qualified continuers, eligible starters, p_continuation, 80)
F_novel = smoothed(unique saves/follows, eligible starters, p_save, 80)
R_novel = smoothed(qualified returners, eligible mature starters, p_return, 80)
Q_novel = Bayesian confidence-adjusted normalized rating

Score_novel = 100 * weighted_mean({E_novel: 0.40, F_novel: 0.25,
                                  R_novel: 0.20, Q_novel: 0.15},
                                  omit unavailable components)
```

**Do not rank two formats on raw scores without format balancing.** Each category creates `short_story` and `novel` lists separately; `/discover?format=all` interleaves them with an adjustable quota (default 50/50 where supply exists), preserving within-format order. For initial small catalogue, gracefully allocate unused positions to whichever format exists.

### 8.4 Trending, Top Rated, New & Underexplored, Latest

- **Top Rated:** sort descending format-specific primary score over recent 90-day eligible cohorts (stable older works may use lifetime aggregate after verified unique-reader dedupe); require >=20 qualified starters OR >=5 distinct qualified reviewer accounts. Small-sample works go to exploration, not falsely displayed as 'top rated'. Tie break by unique qualified readers then stable work ID.
- **Trending:** use primary quality score **plus measured change in genuine discovery activity**, without giving raw views an unbounded vote:

```text
q7 = qualified unique starters in last 7 days
qprev = qualified unique starters in prior 7 days
momentum = clamp( ln(1 + q7) / ln(1 + 200), 0, 1 )
trend_change = clamp( (q7-qprev) / max(20, qprev), -1, 1 )
TrendScore = 0.70 * PrimaryScore
           + 20 * momentum
           + 10 * ((trend_change + 1) / 2)
```

  To prevent giant incumbent-only feeds, `q7` is capped by the log function; ensure `q7 >= 5` for displayed Trending, with small-catalogue fallback to curated and New. Require the underlying quality evidence or label as Emerging rather than Trending. The above function is a deliberate product heuristic, not empirical proof of optimal rankings; include fixture tests to catch surprising effects.
- **New & Underexplored:** eligible published work with >=1 publicly readable entry, initially published within 60 days **or** lifetime qualified impressions below the 25th percentile, and no policy penalty. Prioritize low verified exposure and recent publication, with a **deterministic randomized order by UTC day + anonymized session seed**. Do not expose raw random randomness to the author. Show at least 20% of discovery slots from this pool when enough works exist.
- **Latest:** work last **publicly** updated (not draft save, metadata edit, early-only publish, or minor typo correction). One work can appear at most once in a Latest page, even if the author posts 10 chapters that day. No added scoring for update frequency.
- **Featured:** manually curated owner selection tagged as editorial, not disguised as algorithmic. Default home when <30 eligible works or <1000 qualified starts across 28 days. Works cannot pay to enter.

### 8.5 Final feed building

For each request: determine format/genre/safety filters -> load cached candidate snapshots -> remove hidden/unavailable and excluded content -> select up to 80% ranked (quality/trend/top according to surface) + at least 20% eligible underexplored -> interleave format pools -> apply author diversity cap of 2 per 10 recommendations **only when >=3 distinct actual authors exist** (the initial one-author catalogue must not be artificially restricted) -> stable seeded tie breaking -> return cursor + `algorithm_version`. Do not personalize ranking with sensitive inferred traits. Pagination cursor signs `surface,filter_hash,last_score,last_id,algo_version,expiry` with HMAC.

### 8.6 Fraud and robustness

1. Deduplicate unique reader keys per work and scoring window, including account/guest merges.
2. Never count bot UA from known obvious bots, suspicious rapid reads, impossible active time, same IP floods or replayed event IDs as qualified engagement. IP is a short-lived abuse signal, **not** a permanent identity fingerprint.
3. Rate-limit analytics events and reviews, flag coordinated five-star spikes and high-volume suspicious completions for manual review, and *do not* punish organic traffic spikes automatically.
4. **Never score author-owned reading sessions on own works**, owner/staff QA sessions, page reloads, or admin previews.
5. Do not directly score comment counts, total chapters, chapter update frequency, word count, or raw total views.
6. Run offline simulations: 3-reader 100%-completion short versus 500-reader 80%-completion short; 5-chapter high-retention novel versus 200-chapter mediocre novel; brigaded 20-review story; zero-review story with high completion; new work with zero exposure. Verify fairness and no division by zero.
7. Owner may quarantine manipulated entries with documented reason; score adjustment not a secret punitive setting.
8. Recompute rankings every 6 hours or after enough new data; 1-hour cache for Top Rated/Trending is acceptable. Latest can update within one minute. Do not recompute full corpus for each page request.
9. Analytics and ranking outputs must carry `algorithm_version`, snapshot date and statistical minimums; author dashboard labels sample sizes.

---

## 9. Patreon — definitive V1 access specification

**As of 2026-10-22:** Patreon Public API v1 retired on Oct 7, 2026 for ordinary integrations; use **API v2** only. The API uses explicit `include` and `fields` and `currently_entitled_tiers` is recommended for determining a member's live benefits. Before coding, confirm developer app approval, client scopes, and API documentation in `https://docs.patreon.com/`. Never invent a reliable access level from a browser claim or a screenshot of an invoice.

### 9.1 Initial configuration

The owner configures **one** Patreon campaign for the entire original catalogue regardless of pen name. Maintain mappings:

```text
Patreon campaign = creator_connections.patreon_campaign_id
Tier ID 'tier_a' -> access_rank 1
Tier ID 'tier_b' -> access_rank 2
All early entries specify minimum_access_rank = 1 or 2
```

Rank is platform-local; Patreon tier IDs are opaque strings and MUST be matched explicitly. `currently_entitled_tiers` is decisive, including legitimate trial/gift states when Patreon grants current entitlement. Do not substitute `patron_status` alone or compare pledge dollar amounts with the threshold. If a user belongs to several tiers from the same campaign, select the **maximum configured rank among currently entitled tiers**. Unknown tier IDs map to rank 0 until explicitly configured. Unknown campaign IDs never grant access.

### 9.2 Reader Patreon OAuth flow

1. Verified logged-in reader clicks **Connect Patreon** in `/settings/patreon` or from the chapter's early-access notice. Website creates a server-side one-use random `state` bound to user ID + session ID + 10-minute expiry (store in a dedicated `oauth_states` table with hashed state and consumed-at; migration required). Include authorization `code` flow, HTTPS redirect URI configured exactly in Patreon, and the smallest scopes required: `identity` and `identity.memberships` for access to membership relationships; request more only if API needs it. Do **not** need Patreon email to verify entitlement.
2. Redirect to `GET https://www.patreon.com/oauth2/authorize?response_type=code&client_id=...&redirect_uri=...&scope=...&state=...`.
3. On backend `GET /api/v1/pawn/x/patreon/callback`, validate caller's session, returned `state`, expiry, one-use constraint; exchange code server-to-server via form `POST https://www.patreon.com/api/oauth2/token`; never put client secret or refresh token in SPA.
4. Call `GET https://www.patreon.com/api/oauth2/v2/identity?include=memberships,...` with explicit `fields[user]`, `fields[member]` as needed, including tier and campaign relationships. The exact `include` expression and resource fields must be confirmed with a real sandbox/test response. Match memberships by **campaign ID**, not creator name/email, then identify current entitled tier IDs. API may return member relationships through `included`; write parser against JSON:API resource IDs and relationships, not array position.
5. Reject linking if Patreon ID already linked to another user (show safe account-recovery guidance); do not silently steal/reassign the connection. Store encrypted tokens, expiry, membership snapshot, verification timestamp. Redirect to `/settings/patreon?connected=1` with no tokens in URL.
6. If Patreon denies authorization, state expires, account is unlinked, or provider is down, show a clear non-destructive error. Never create entitlement from failure.

**Endpoints:** `GET /api/v1/pawn/x/patreon/connect` -> 302 authorization; `GET /api/v1/pawn/x/patreon/callback` -> validation/exchange and redirect; `GET /api/v1/pawn/x/patreon/status` -> connection and mapped access rank only; `POST /api/v1/pawn/x/patreon/refresh` -> rate-limited forced verification; `DELETE /api/v1/pawn/x/patreon/disconnect` -> remove linked token and entitlement; `POST /api/v1/pawn/x/webhooks/patreon` -> webhook, no session cookie but signed. Creator connection setup only in `/studio/integrations/patreon` with owner privilege; display actual campaign/tier names from API after successful authorized handshake.

### 9.3 Entitlement check (server, every early request)

```text
function canReadEntry(entry, work, viewer, now):
    if work.hidden or work.draft or entry.archived: deny(404)
    if viewer has explicit author/owner preview permission and request is an admin preview: allow
    if entry.published_revision_id is NULL: deny(404)
    if entry.public_at is not NULL and now >= entry.public_at: allow_public
    if entry.early_at is NULL or now < entry.early_at: deny(UPCOMING)
    if viewer not logged in: deny(EARLY_ACCESS_REQUIRED)
    snapshot = verifiedPatreonEntitlement(viewer, entry.creator_campaign)
    if snapshot.verified AND snapshot.rank >= entry.min_access_rank: allow_private
    deny(EARLY_ACCESS_REQUIRED)
```

- Cache verified Patreon entitlement <=10 minutes under normal operation, with immediate invalidation when a valid membership webhook reports change. For a previously verified member during a temporary Patreon outage, optionally allow **no more than 10 additional minutes** of stale access, configurable and logged; after that fail closed for **early** content. Public content continues regardless of provider outage.
- Review payment status only insofar as needed to honor Patreon `currently_entitled_tiers`. Cancelled-but-covered periods may still be entitled until Patreon removes them.
- No cached shared responses for early chapters, no content body in HTML shell, service-worker cache, source maps, analytics events, metadata lists, or unauthenticated API responses.
- Chapter public availability overrides everything (even a broken webhook or expired auth token). No scheduled-release job is required for this transition.
- On disconnect, clear current entitlement immediately and stop member checks; on cancellation webhook, invalidate and re-fetch before any further early access.

### 9.4 Creator connection and webhooks

Creator authorizes required creator scopes (`campaigns`, `campaigns.members`, `w:campaigns.webhook` if using API-managed webhooks). Create or configure webhook for campaign with V2 `members:create`, `members:update`, `members:delete`, `members:pledge:create`, `members:pledge:update`, `members:pledge:delete`, depending on approval and volume. Do not register `pledges:*` legacy triggers.

- Patreon currently documents `X-Patreon-Signature` as hex-encoded **HMAC-MD5 over the exact raw HTTP body** with webhook secret. Implement constant-time signature comparison on decoded digest (MD5 here is provider protocol compatibility, **not** for storing user passwords or custom security hashes). Verify signature **before** parsing or queuing. Limit payload size; reject missing/bad signatures with 401/400.
- Extract campaign and member IDs, deduplicate repeated deliveries, enqueue reconciliation; never trust raw webhook alone to grant a paid tier without confirming campaign and membership details. Out-of-order messages must not overwrite a newer verified provider snapshot.
- Store webhook secrets encrypted, rotate safely; logs must not contain tokens, webhook body secrets, or patron personal information.
- Use scheduled reconciliation for active linked accounts (at least daily) and when requests encounter stale snapshots; page through Patreon v2 cursor pagination only as necessary and respect rate limits/backoff.
- If Patreon webhook registration fails or is unavailable, polling reconciliation is an explicit degraded mode, visible in owner dashboard. Site must not falsely claim real-time entitlement.
- If Patreon changes its signature or fields in a later API version, update adapter tests and provider version docs before deploying. Never silently disable verification.

### 9.5 Patron-facing language and UX

Explain: "All main fiction eventually releases free. Supporting us on Patreon provides early access while your membership remains entitled." Early chapter card shows publicly scheduled date, eligible tiers, creator Patreon link, `Connect Patreon`, and help for `I already subscribed`. The reading account and Patreon account are linked explicitly; buying a Patreon membership alone doesn't automatically create a site account. Show connected state and last-check time. Do not collect or display payment card details, pledge history, addresses or subscriber emails from Patreon when unnecessary.

### 9.6 Patreon integration tests (with fixtures + optional live test)

- Not linked -> locked before public.
- Free Patreon tier -> locked for paid early access.
- Authorized rank 1 -> rank 1 works only; rank 2 -> rank 1 and 2.
- Entitled gifted/trial tier -> access only if current entitled relationship reports it.
- Cancelled but still entitled until period end -> access follows provider's currently entitled tiers.
- Revoked, invalidated tier, disconnected, creator mismatch -> denied.
- Webhook duplicate / replay -> idempotent; invalid HMAC -> rejected; out-of-order -> fresh provider state wins.
- OAuth CSRF attempt -> rejected; code replay -> rejected; same Patreon account linked elsewhere -> conflict.
- Early chapter fetched from unauthenticated endpoint / CDN -> body never leaks.
- Public release timestamp passes during Patreon outage -> anonymous free access immediately.

---

## 10. Privacy-aware reading analytics

### 10.1 Event pipeline

Browser sends small batched events -> custom API validates schema/consent/rates/eligibility -> **Redis stream plus atomic dedupe/view counters and associated data** -> **worker flush every 120 seconds** -> MariaDB `analytics_events`/`read_sessions`/qualification ledger/daily aggregates -> ranking snapshots. Essential progress remains a separate direct MariaDB write and must work with analytics disabled or Redis unavailable. No per-view MariaDB write on the request path.

**Required buffering and flush semantics:**

1. Redis is a separately running shared service with a persistent volume, AOF enabled and a documented fsync policy. Default `appendfsync everysec` preserves buffers across Go restarts while acknowledging an approximately one-second Redis host-crash loss window; require stronger persistence if zero acknowledged loss is needed. Do not claim cross-system exactly-once delivery or zero-loss from AOF alone. Otter and other in-process caches lose contents on process restart and cannot replace this buffer.
2. Validate IDs, consent, event age/size and plausibility before append. Use an atomic Redis operation/script to append the event, record a bounded-lived UUID dedupe marker and update provisional view counters/associated metadata together. Raw opens/page loads are labeled separately from qualified unique starts/completions; never infer completion from an HTTP GET. Counter keys include work/entry/UTC day as appropriate, using server-derived reader pseudonyms. The stream event is the recoverable source for counters; don't flush counters and the same event as two independent increments.
3. Namespace keys by environment/app (e.g. `pawn:prod:analytics:*`); one Redis stream per initial deployment is sufficient. Use a consumer group and pending entries so an API/worker restart leaves accepted events recoverable. A single scheduler lease/leader enqueues `flush_analytics` in Collar's queue on UTC **120-second boundaries** using `analytics:flush:{bucket}`; it scans pending backlog on startup. This is Pawn.Ink scheduling around Collar, not a built-in periodic-job API. The handler drains bounded batches repeatedly through a captured watermark and does not wait another interval per batch. Never self-enqueue a running job with its same pending unique key expecting another run.
4. Each flush claims pending/unprocessed events (including abandoned claims), assigns a stable batch ID and uses a MariaDB transaction to record its receipt, insert only new UUID events, update sessions/qualification and add only newly eligible metric contributions. Concurrent workers/replays must not double-count; enforce unique keys and row locking for event and reader/work/window transitions. Persist cumulative session state needed for heartbeat plausibility across flush boundaries. Source event time determines the UTC day, not flush time. Invalid/quarantined events get an explicit terminal disposition/metric, not an infinite poison-message loop.
5. **Commit MariaDB before acknowledging/removing Redis entries.** A crash before commit leaves events pending; a crash after commit before ACK replays them with no new contributions. Never `GETDEL`/delete/reset the buffer before SQL commit. Bound processing transactions and renew/reclaim leases safely. ACKed entries may be trimmed with a safe watermark; never trim pending/unpersisted entries merely to enforce stream length. Provisional counters reconcile from committed totals plus remaining stream state; normal dashboards read flushed MariaDB aggregates through Collar and show `last_flushed_at`/lag, not misleading real-time claims.
6. Run every 120 seconds under normal conditions (the production interval is fixed at `120s`; validate configuration accordingly), allow best-effort final flush on graceful shutdown, and drain/recover backlog at worker startup. An app restart must not lose the last interval. Retry MariaDB failures using Collar's queue/backoff while retaining Redis data. Provisional view-counter/dedupe metadata can expire only under a documented policy compatible with pending replay; never expire unflushed stream data. Keep durable SQL replay protection at least as long as the maximum retained backlog/client retry horizon. Ingested-on-time backlog is not rejected simply because a DB outage delays flushing beyond the client's 24-hour ingestion age limit.
7. Redis uses a capacity budget and `noeviction` for this buffer. Bound ingestion time/memory; if full/down, reject optional events explicitly, track dropped/rejected counts and preserve reading/publishing. Do not silently switch to process memory. Monitor persistence errors, stream depth/oldest age, pending claims, last successful flush, flush latency, retries and dedupe counts; alert when a 120-second cycle is missed materially. Redis outage is a degraded analytics dependency, not a public-reader outage.
8. Pending Redis payloads obey the same privacy/consent controls as persisted rows: no story bodies/tokens, no permanent fingerprinting. Account deletion/opt-out purges or suppresses associated pending events using a durable deletion cutoff/tombstone rechecked during flush, so delayed data cannot recreate deleted histories. Normal raw-data retention starts from event time; use aggregate-only handling or explicit suppression for backlog beyond raw retention. Backups/restores include a plan for pending analytics and dedupe watermarks to prevent replay inflation.

First-party cookies only, no third-party advertising trackers or hidden cross-site identity matching. Default to minimal functional analytics and offer opt-out for nonessential detailed behavior tracking. Client heartbeat only while visible/active/not idle; cap claimed deltas against elapsed time. Client event UUIDs support retries; reject newly ingested events older than 24 hours for ranking. Repeated opens without activity are not qualified reads. Rotate pseudonymous reader keys according to the privacy policy.

### 10.2 Dashboards

Read flushed aggregates directly through authorized Collar collections/views; show the last successful flush and processing lag (normally up to 120 seconds plus processing time). No request-time raw-event scans.

Owner overview: traffic (page loads separately labeled), unique qualified starters, median chapter continuation, story completion by genre and reading time, follow/save conversion, current publications, top referring *categories*, Patreon link success, errors, moderation queue. Work-level dashboard: per-entry funnel (`opened -> qualified -> continued -> completed`), cohort curves, traffic/referral categories, audience retention and read duration distributions (aggregated), discovery impressions/clickthrough, reading width/device class distribution. Never show individual named-reader behavior or individual engagement histories to authors.

If a metric has <20 distinct contributing readers, suppress granular cohort breakdown to reduce re-identification and label `Insufficient data`. Always show number of qualified readers and date range next to rates. Provide CSV export of aggregates for owner, excluding pseudonymous raw keys. Data retention: raw analytics events and sessions 30 days, temporary qualification ledgers up to 60 days, aggregate daily work metrics 24 months or until reset, security logs 30–90 days; make policies configurable. Account deletion removes or anonymizes user-specific progress and history within documented retention windows, except legally required audit records.

### 10.3 Performance budgets

API events endpoint must return quickly (`202` only after Redis acceptance) without ranking calculation on request path. Batch up to 25 events. Redis append is bounded and durable under the configured persistence policy; 120-second worker flushing is idempotent and retryable. Keep event traffic under control: heartbeat at most once every 15 seconds per active entry; max one unique qualified start per reader/work/day for aggregates. Backpressure drops nonessential analytics before it affects reading or publishing.

---
## 11. Editorial tools and publishing behavior

### 11.1 Work wizard (implement exact steps)

1. **Choose format:** Novel or Short Story. Cannot change after publication.
2. **Choose pen name:** from identities owned by active user.
3. **Metadata:** title, slug auto-suggested/validated, one primary genre, optional approved tags, synopsis (max 4,000 chars), language, age rating, content warnings, cover. Show preview card in dashboard.
4. **Create content:** Novel -> add Chapter 1, optional Volume 1; Short Story -> immediately create its single entry and open editor. Short-story UI MUST NOT show `Add chapter`.
5. **Draft:** Markdown CodeMirror with autosave after 1.5 sec of idle, manual Save, visible dirty state and revision version. Support headings, emphasis, separators, blockquote, dialogue formatting, links; don't support arbitrary HTML, executable embed code, scripts, frames or remote tracking pixels. Let users paste Word/Google Docs rich text as *plain Markdown conversion* (avoid injecting pasted HTML).
6. **Preview:** sanitized HTML using exact public reading styles and selected theme; preview must not create a publicly accessible URL.
7. **Schedule:** choose public release date, optional early access date and tier, time zone preview but save UTC; validate not beyond 30-day maximum early lead; show accurate statuses and public countdown.
8. **Publish:** capture immutable published revision and timeline in one DB transaction; enqueue optional in-app follower notification when first public, not when draft edited.
9. **After release:** editing published text creates a new revision and audited swap; notifying followers is optional for significant revisions only; avoid gaming Latest via corrections.
10. **Complete:** mark finished novel only after all intended chapters exist and have public dates; warn of unlisted drafted chapters. Completed short stories are automatically complete when public.

### 11.2 Editorial ergonomics

- Work list filters draft/ongoing/completed; pen name, format, release date.
- Bulk scheduling novel chapters: given anchor date, interval in days/week and optional early lead, show full preview of future dates; apply transactionally after confirmation. Per-entry overrides allowed. Never schedule draft with empty body.
- Simple chapter reordering with drag/drop and accessible keyboard alternative; serialize renumbering under DB lock so UNIQUE ordinal doesn't conflict; use temporary range offsets or reorder table transaction.
- Bulk text import from author-owned `.md` or `.txt` files only; 1 file = 1 story entry, or author-confirmed delimiter-based chapter splits; no scraping other services or importing third-party copyrighted material. Show split preview before commit.
- Scheduled content entry should display `Free on [date]` even if currently early. No fake countdown for data that is not actually scheduled.
- `story` estimated minutes = `ceil(word_count / 230)` with minimum 1 minute; novel work estimate = sum of public chapters (include optional total author-declared word count separately, labeled).
- `Word count` = count tokens separated by Unicode-aware boundaries (handle apostrophes and numbers reasonably). Unit tests for mixed punctuation and Unicode; never use bytes/characters as words.
- No draft autosave requests after user loses permission, logouts or navigates away; show recoverable local unsaved edit copy with explicit user action, not quiet write to unrelated work.

### 11.3 User review/comments UX

- Reviews accept 1–5 stars, optional title and body; distinguish star-only ratings from written reviews. Flag spoilers; collapse spoilers until clicked. Show anonymous handle only (user display name), not email.
- Comments on individual entries; short story single entry only. Supports text/limited Markdown, one reply level, author badge for genuine author identities, time, edit/delete, spoiler, flag/report. Never add like/dislike/reaction farming in V1.
- Rate limit comments to 5 per 10 minutes/user and reviews to 5 new/edited submissions per hour/user (configurable). Additional automatic moderation by manual word blocklist, link-spam threshold, repeat-body hash, and user report review. Do not block harmless quotes of banned words in fiction itself.
- Author can hide offensive comments on their work with reason; all actions audit logged and owner can restore. Author cannot edit a reader's comment text.
- A submitted report is private; do not expose reporter identity to reported author. Report abuse can also be rate limited.

### 11.4 Content policy

Publish guidelines in `/policies/content`: original works or properly authorized adaptations; no plagiarized content; substantial AI-generated narrative farms prohibited; use of AI for outlines, proofreading, translation assistance, brainstorming permitted provided human remains responsible for narrative authorship and rights. If a prospective author's status is unclear, request evidence/examples and use a human review process; do not make unappealable automated detector decisions. Report/copyright contacts and takedown process required before invite-based expansion. Content tags and age labels inform readers; exclude unlawful content and conduct; apply current hosting/payment-provider restrictions after reviewing terms. Legal policy wording is a draft for local legal review, not a representation that the application is compliant with every jurisdiction by default.

---

## 12. Backend service design and concurrency

### 12.1 Server stack organization

Embed Collar as described in section 1.1. Native dashboard reads/allowed CRUD stay in Collar. Custom reader and business-command handlers parse/bound JSON and call services; services enforce invariants, authorize, transact and write audit; repositories handle SQL; middleware handles request IDs, tracing, sessions, CSRF, rate limiting, body limits, and panic recovery. Avoid business rules duplicated in both handlers and frontend. Pass `context.Context` to every DB and Patreon request with bounded timeouts. Never hold DB transaction while calling Patreon, SMTP, or object storage.

Use interfaces only at real seams (`Store`, `PatreonClient`, `AssetStore`, `Mailer`, `Clock`, `RandomSource`), not speculative abstraction for every small function. `Clock` injectable for release tests. URL slugs validated using `[a-z0-9]+(?:-[a-z0-9]+)*`, length 3–160 for work slug, reserved path words blocked. Store canonical slugs, allow name changes with redirect records. JSON body max 1 MB for normal API, 2 MB for editor bodies; 5 MB image upload.

### 12.2 Publishing concurrency

- On editor autosave, send `expected_revision`; server checks current draft version inside transaction. If mismatch return `409 REVISION_CONFLICT` with server revision summary; show side-by-side choices, never silently overwrite a newer draft.
- When scheduling, transaction checks own work, current draft revision, entry uniqueness, correct dates and tier map, materializes immutable sanitized published revision, updates release pointers and writes audit; job enqueue occurs transactionally.
- Release notifications job can run late, but reader content publication cannot run late because it is always computed from `public_at`. This is mandatory.
- Two concurrent admin reorders or publishes must serialize per work and return conflict instead of leaving duplicate or missing ordinals.
- Assets are initially uploaded pending, validated then marked ready; failed uploads cleaned asynchronously. Avoid orphaned content references.

### 12.3 Job definitions

| Job kind | Cadence/trigger | Idempotency key |
|---|---|---|
| `notify_public_entry` | At public time (or startup catch-up) | `entry:{id}:public:{public_at}` |
| `update_work_rollups` | After new public release/edit | `work:{id}:rev:{revision}` |
| `flush_analytics` | Every **120 seconds**, startup backlog recovery, best-effort shutdown | `analytics:flush:{120_second_utc_bucket}` plus durable event/batch receipts |
| `aggregate_analytics` | During/after successful flush; rebuild on demand | `aggregate:{committed_batch_id}` |
| `rank_discovery` | Every 6 hours; owner manual | `rank:{algorithm_version}:{window_end}` |
| `cleanup_raw_analytics` | Daily | `cleanup:{date}` |
| `patreon_reconcile` | Periodic + webhook-triggered | `member:{member_id}:{time_bucket}` |
| `refresh_patron_tokens` | Before token expiry | `patreon:token:{link_id}:{expiry_version}` |
| `cleanup_expired_auth` | Daily | `authcleanup:{date}` |
| `send_email` | On verified account actions | `mail:{outbox_id}` |
| `cover_variant_generation` | New asset | `image:{asset_id}:variant:{size}` |
| `deletion_cleanup` | After retention deadlines | `deletion:{entity}:{id}` |

Use Collar job claiming, running-job recovery and retry/backoff; do not depend on the discarded application `locked_until`/`dead` schema. Limit per-kind parallelism. Exhausted jobs remain `failed`, visible through Collar admin and application alerts; safe retry is explicit. The Pawn.Ink recurring scheduler recreates missed runs after restart using durable state/UTC buckets, not an in-memory timer alone. No job may send duplicate email notifications after execution timeout; use unique notification and mail outbox keys. On startup, schedule missed jobs using current data, not just in-memory timers.

### 12.4 Search and sorting

Use MariaDB FULLTEXT on public `works` title+synopsis with prefix fallback for short titles and normalized exact-slug matches. Limit search text to 100 chars, no user-supplied SQL operators interpolated. Normalize punctuation, Unicode and whitespace; exclude drafts/hidden; optional mature-content filter. Sort options for story and novel catalogues: recent public release, engagement/trending, rating, alphabetical, newest work. Use parameterized keyset pagination to avoid deep OFFSET. Genre/tag filter is intersection; make filter semantics clear in UI.

### 12.5 Caching rules

- Parcel-built locally bundled assets (including Bootstrap and icon fonts): immutable one-year if hashed filenames; `index.html` no-cache (SPA deploys should update).
- Public work metadata/chapters: short shared caching permitted **only when response is identical for all visitors** and content is already public. Stale public content acceptable briefly after an edit; purge on content update. Never cache hidden works after moderation withdrawal.
- Early entry bodies, custom `/me/*` and `/studio/*`, all authenticated native Collar collections/admin endpoints, Patreon status, auth: `Cache-Control: private, no-store`; `Vary: Cookie` alone is not sufficient protection in an incorrectly configured CDN.
- Public TOC may contain only public metadata/timestamps, never hidden text, and may be cached briefly; entitlement-specific access booleans must not be cached publicly.
- Redis is mandatory for pending view/event data with 120-second flushes; it is not a shared cache for early bodies or credentials. Keep public read optimization based on indexes and precomputed snapshots, with optional disposable caches only where safe.

---

## 13. Security model and production guardrails

### 13.1 Threat model

Protect against: stolen sessions, CSRF, XSS via fiction or comments, SQL injection, path traversal, OAuth callback forgery, Patreon token theft, webhook spoof/replay, unauthorized draft access, leaked paid content from CDN/API, account enumeration, password spraying, upload malware, report spam, analytics manipulation, role escalation, accidental SQL schema destruction and production data loss.

### 13.2 Controls — implement before public release

1. HTTPS only in production; strict host validation; use trusted proxy headers only from configured proxy IP; HSTS after HTTPS verified. Set `HttpOnly; Secure; SameSite=Lax` on auth cookies; no sensitive bearer tokens in browser storage.
2. Argon2id with resource parameters calibrated for deployment (document them); random salts from cryptographic RNG; password length min 12 max 128; block common compromised passwords if practical; per-IP+email login throttling and temporary cooldown. Never log password or reset tokens.
3. CSRF protection for all cookie-authenticated mutations, same-origin Origin/Referer check and explicit CSRF token sent via header; OAuth `state` is separate one-time check. CSP default self, local Parcel/Bootstrap/Icon assets only (no CDN allowlist), no inline unsafe scripts, no arbitrary frames, no third-party analytics; ensure Markdown images can't inject active content.
4. Markdown sanitized server side. Link protocols allow `https`, `http` (optional), `mailto`; reject `javascript:`, `data:` links, handlers, raw scripts, iframe, style blocks. Display text via safe DOM rendering; do not use unsanitized `m.trust`.
5. Fine-grained authorization on every work/entry/studio/admin endpoint and asset edit. Use request-owned user identity from validated session only. Account bootstrap privileges through CLI, not public endpoint.
6. OAuth client secrets and token encryption key in environment/service secrets (0600 root/pawn-ink user readable as necessary), encrypted DB Patreon tokens with AEAD (AES-256-GCM or XChaCha20-Poly1305), unique nonce and key version; no key in git or logs.
7. Webhook signature verified on exact raw bytes with provider-prescribed algorithm and constant-time compare; unknown event types ignored safely after verification; replay/idempotency protection and fresh reconcile.
8. Uploaded images: verify actual decoded format, re-encode, strip metadata, cap dimensions (e.g. 5000x5000), disallow SVG and executable extensions, use generated random storage keys, serve `X-Content-Type-Options: nosniff`, `Content-Disposition: inline` only for safe images.
9. API rate limits: registration 5/hour/IP prefix, login 10/15min/IP/email combo, reset 3/hour/address+IP prefix, comments as above, reports 10/day, event ingestion 120 requests/min/session max. Limits are initial knobs, not fixed scientific values. Avoid using IP as a persistent analytics identity.
10. Return security headers: Content-Security-Policy, Referrer-Policy strict-origin-when-cross-origin, Permissions-Policy minimal, X-Content-Type-Options, appropriate frame ancestors. Test with CSP report-only before enforcement.
11. Public API never exposes draft Markdown, Patreon credentials, reader emails, exact individual progress, unpublished entry bodies, provider webhook payloads, security log contents.
12. DB account has least privileges: runtime account CRUD only on designated Pawn.Ink application and Collar system databases; migration account owns DDL; backup account read-only as appropriate. Do not run Go process as root.
13. Data exports / account deletions require re-authentication and audit. Separate account deletion from content takedown to avoid accidental destruction of published fiction.
14. System logs structured and scrubbed of secrets. Persistent audit tables restricted to owner. Error responses do not reveal SQL queries or stack traces.
15. CI uses dependency scanning, `govulncheck`, `go vet`, frontend audit, and tests; block shipping known critical exploitable flaws unless documented mitigation.

### 13.3 Site privacy policy specifics

No third-party ads/tracking in V1; reading analytics for aggregate product quality; anonymous reading allowed; users may request export/delete of account progress; explain retention, Patreon scopes and data sharing. Social reviews/comments are public; email and Patreon identity are private. No selling of identifiable reading histories. Obtain consent for optional email newsletter when implemented. Content ratings and moderation policy should be clear, not moralizing.

### 13.4 Non-destructive VPS instructions — mandatory for agents

This project may be developed on a production machine. **Never assume the VPS is disposable.** Before *any* deployment or DB change: inspect `pwd`, current Git status, listening ports, proxy configuration, databases and systemd services. Do not overwrite or delete unrelated services, volumes, buckets, directories or certificates. Never run `rm -rf` on unknown paths, wildcard destructive SQL, `docker compose down -v` on production, `DROP DATABASE`, `TRUNCATE` user data, or alter an existing reverse-proxy root configuration without explicit operator approval.

- Use a project-specific Linux user and directory (suggest `/srv/pawn-ink`), project-specific DB (`pawn_ink`) and credentials, a **new verified-free bind port** chosen via config; avoid binding public interface directly. The environment's prior reserved port ranges, if any, take precedence; do not invent that 140xx ports are always available.
- Print/record intended changes and backup points before execution. Use `ss -ltnp` to inspect ports and `systemctl` to inspect services; if insufficient privileges or ambiguous ownership, STOP before applying that change and report blocker.
- Database migration runs separately, with backup and dry-run/inspection; never automatically call down migrations on deploy. Test restoration on a separate database before major schema changes.
- Secrets live outside repository in restrictive files; use `.env.example` with placeholders only.
- Restart only the dedicated service being installed. If existing reverse proxy is shared, supply proposed site block and request approval before modifying it.
- Build and test in a project directory; do not modify global npm, Go, firewall, DNS or PHP/application settings without need and explicit approval.
- If only local development was requested, do not deploy or expose the app publicly.

---

## 14. Environment configuration and commands

Example `.env.example` (all secrets replaced by safe placeholders):

```dotenv
APP_ENV=development
APP_NAME=Pawn.Ink
APP_BASE_URL=http://localhost:14021
HTTP_ADDR=127.0.0.1:14021
TRUSTED_PROXY_CIDRS=
COLLAR_CONFIG=./collar.yaml
COLLAR_ADDR=127.0.0.1:14021
COLLAR_DSN=pawn_app:CHANGE_ME@tcp(127.0.0.1:3306)/?parseTime=true&loc=UTC
COLLAR_ADMIN_DSN=pawn_migrator:CHANGE_ME@tcp(127.0.0.1:3306)/?parseTime=true&loc=UTC
COLLAR_AUTO_MIGRATE=false
COLLAR_GLOBAL_REGISTRATION=false
COLLAR_ACCESS_TTL=15m
COLLAR_REFRESH_TTL=720h
COLLAR_TRUST_PROXY=false
COLLAR_ADMIN_DIR=
COLLAR_JWT_SECRET=CHANGE_ME_LONG_RANDOM_HEX
APP_DATABASE=pawn_ink
COLLAR_SYSTEM_DATABASE=pawn_ink_collar
REDIS_URL=redis://127.0.0.1:6379/0
REDIS_KEY_PREFIX=pawn:dev:analytics
ANALYTICS_FLUSH_INTERVAL=120s
SESSION_HMAC_KEY=CHANGE_ME_LONG_RANDOM_HEX
CSRF_HMAC_KEY=CHANGE_ME_LONG_RANDOM_HEX
TOKEN_ENCRYPTION_KEY=CHANGE_ME_32_BYTE_SECRET
RANKING_HMAC_KEY=CHANGE_ME_LONG_RANDOM_HEX
ANALYTICS_HMAC_KEY=CHANGE_ME_LONG_RANDOM_HEX
OWNER_BOOTSTRAP_EMAIL=owner@example.invalid
SMTP_HOST=localhost
SMTP_PORT=1025
SMTP_USERNAME=
SMTP_PASSWORD=
MAIL_FROM=no-reply@example.invalid
OBJECT_STORE_KIND=s3
# Local development uses MinIO with Collar storage configuration.
S3_ENDPOINT=
S3_BUCKET=
S3_ACCESS_KEY=
S3_SECRET_KEY=
PATREON_ENABLED=false
PATREON_CLIENT_ID=
PATREON_CLIENT_SECRET=
PATREON_REDIRECT_URL=http://localhost:14021/api/v1/pawn/x/patreon/callback
PATREON_CREATOR_CAMPAIGN_ID=
MAX_EARLY_DAYS=30
ANALYTICS_ENABLED=true
ALLOW_TEST_ENTITLEMENTS=true
LOG_LEVEL=info
```

**Production deployment:** bare metal, **Nginx -> systemd -> Go API binary**, with a separate systemd worker. Install versioned binaries under `/srv/pawn-ink`, configure `/etc/pawn-ink` environment/secrets, and proxy Nginx only to the approved loopback listener. No mandatory application `LICENSE` file: the code is proprietary.

**Production identity:** `APP_NAME=Pawn.Ink`, `APP_BASE_URL=https://pawn.ink`, `PATREON_REDIRECT_URL=https://pawn.ink/api/v1/pawn/x/patreon/callback`. Configure Collar YAML database aliases/storage from environment explicitly; `APP_DATABASE`, `COLLAR_SYSTEM_DATABASE`, Redis and app HTTP settings are Pawn.Ink configuration, not automatically recognized Collar variables. Keep the wrapper listen address and `COLLAR_ADDR` consistent. Private module access uses scoped `GOPRIVATE` and credential helpers/CI secrets, never token-bearing URLs. Bootstrap/Icons are installed local packages, not remote runtime resources. Redis AOF/volume/noeviction settings live in infrastructure configuration.

**Note:** `ALLOW_TEST_ENTITLEMENTS` MUST be rejected at boot unless `APP_ENV=development` and host is bound to loopback. Use different random secrets per environment. In production secure cookie logic requires HTTPS behind properly configured trusted proxy.

### 14.1 Developer commands (create these Make targets)

```bash
make help          # print targets
make deps          # go mod download + npm ci
make dev-db        # start disposable MariaDB + Redis + MinIO + Mailpit, no -v deletion
make migrate       # Goose app migrations + separate Collar system migrate; then config check
make seed-dev      # dev-only fixture seed; MUST refuse on production
make owner         # interactive/safe owner bootstrap (never public HTTP)
make dev-api       # run Go API; local SPA fallback if built
make dev-web       # Parcel watch/build; Go serves rebuilt reader/dashboard assets in dev
make dev-worker    # run Collar worker + 120-second analytics scheduler/flusher
make dev           # documented API + Parcel watch + worker process manager/terminals
make test          # Go + frontend unit and integration tests
make test-e2e      # Playwright against disposable test environment
make lint          # gofmt/go vet/TS ESLint/typecheck
make security      # govulncheck + dependency checks
make build         # npm ci, tsc --noEmit, Parcel build both HTML entries; Go API/worker/CLI binaries
make smoke         # health and critical published/early access checks
make backup        # both app/Collar DBs + assets + coordinated Redis pending-data recovery
make release-check # verify migrations, tests, config, smoke preflight
```

Default development uses Parcel watch with Go serving emitted assets from disk; production always uses embed.FS. If a Parcel dev server is used, bind it to loopback and configure an explicit same-origin API proxy/cookie flow. Example Go port 14021 is illustrative only. Choose safe ports by inspection. Ensure Makefile does not run `docker compose down -v` or destructive cleanup by default. Provide instructions for Linux, with Bash scripts that use `set -euo pipefail` and quote paths.

### 14.2 Reverse proxy and deployment

- **Bare-metal request path:** Nginx for **pawn.ink** HTTPS -> the systemd-managed Go API binary bound `127.0.0.1:<approved_port>`, preserve secure headers, timeout 15 sec default for API, longer only bounded uploads. HTTP to HTTPS redirect. Enable compression for JSON/assets/content after verifying content safety; serve static images and hashed JS/CSS aggressively with correct cache controls.
- One Go API process hosts embedded Collar, the browser-session wrapper and both embedded SPA entrypoints. A separate `pawn-ink-worker` process runs Collar jobs/subscriptions and the 120-second Redis flush scheduler from V1. Deploy/upgrade both together. `SIGTERM` stops accepting requests, completes bounded in-flight work, cancels worker cleanly and attempts a final bounded analytics flush; pending Redis data survives forced termination.
- Define separate `pawn-ink.service` and `pawn-ink-worker.service` systemd units with a dedicated user, `Restart=on-failure`, environment file, `WorkingDirectory`, restrictive filesystem access, security hardening options that do not conflict with required asset storage.
- Health: `/healthz` = process alive; `/readyz` = application/Collar DB reachable + schema compatibility. Report Redis persistence/ingestion and worker/flush lag as degraded health separately; Redis failure must not prevent public chapter reads. Provider failure should show integration warning, not make entire fiction library unready.
- Nginx: provide a dedicated `server` block for `pawn.ink`, TLS certificate paths, proxy headers and loopback upstream; check `nginx -t` before an authorized reload. Preserve unrelated sites. Nginx fronts the Go binary; the Go binary serves both embedded UIs and all API routes. Production has no separate frontend server or asset build process.
- Rolling release on one bare-metal VPS: preflight backup -> migration from reviewed version -> build new binary -> swap via dedicated service restart -> smoke checks -> rollback app binary if failed. **DB down migration is not part of automatic rollback**; write backward-compatible migrations where feasible.
- Backups: coordinated application + Collar system DB backups (sessions, jobs, outbox/cursors), assets, Redis AOF/pending-stream recovery and encryption-key recovery plan; 7 daily + 4 weekly snapshots as starting policy; encrypt backup and keep off-VPS copy. Test full restore at least monthly on staging. Replacing an encryption key without re-encrypting token data will orphan Patreon credentials; document rotation plan.
- Observability: structured request logs with request IDs, Go process metrics, HTTP latency, DB pool stats, Collar worker queue depth, failed jobs, Redis stream/pending depth and oldest age, persistence health, 120-second flush lag/retries, Patreon refresh errors, ranking generation age, storage consumption, asset upload failures, spam/report spikes. Health alerts should avoid disclosing reader information.

### 14.3 Performance and accessibility requirements

Target on a normal smartphone and average network: initial SPA JS/CSS gzip <= 250 KB where achievable (include local Bootstrap/Popper/Icon assets and report icon-font transfer separately) (measure build), lazy-load editor/admin code, instant cached route transitions, API reads p95 < 250 ms on warm small dataset (excluding network) and public chapter body < 500 ms on reasonable VPS load; measure before making claims. Support 20k public works/2M entries architecturally through indexes/keyset pagination, but do not prematurely provision that much infrastructure. No ranking per request. Automated accessibility: forms label, tab order, visible focus, aria live statuses, contrast, reduced motion, keyboard TOC. Reader must work without hover/touch gestures.

---
## 15. Precise implementation sequence for an autonomous coding agent

**Execution rule:** Complete phases in order. After every phase run relevant unit/integration tests, update `docs/TEST_REPORT.md` with commands/results, and commit a coherent change. Do **not** advance with knowingly failing tests. If an external credential/approval is absent, implement a clearly labeled dev adapter and mark live integration blocked; build everything else and describe exact operator action required. Do not claim live Patreon works merely because fixtures pass.

### Phase 0 — Safe inspection and scaffold

- [ ] Inspect repository, host OS/toolchain, available ports, existing DB/proxy/services; record findings. Do not change production settings.
- [ ] Create repo layout, README, env example, Makefile, CI workflow, gitignore excluding secrets, `.env`, uploads, build caches, binary outputs. Application is proprietary; do not create or require a `LICENSE` file. Add Nginx server-block and separate API/worker systemd examples for bare-metal deployment.
- [ ] Initialize module for `github.com/namankumar80510/panw.ink`; configure authenticated private Collar resolution, pin reviewed revision/toolchain, embed Collar and establish the native/custom route boundary. Scaffold Parcel/Mithril/TypeScript reader and dashboard entrypoints with npm/bun-installed Bootstrap 5/Bootstrap Icons; no CDN, Vite or Rspack.
- [ ] Create disposable MariaDB/Redis/MinIO/Mailpit Compose setup with persistent Redis AOF volume/noeviction; separate Goose app and Collar system migrations; commit and check `collar.yaml` allowlist/rules/auth configuration.
- [ ] Build Go server embedding both Parcel entrypoints and all local Bootstrap/Icon assets; verify reader/studio/admin deep links, root precedence, missing-asset behavior and `/api/*` JSON 404s from an empty working directory. Add worker binary/service from V1.
- [ ] Use Bootstrap responsive layouts, components/color modes, accessible toasts and Icons; minimal brand/reader CSS. Check Parcel rewrites fonts/assets and `tsc --noEmit` passes independently.

**Exit:** `make dev` shows homepage; `GET /api/v1/pawn/x/public-config` responds JSON; direct page reload `/stories` works; no global services disrupted; no secrets committed.

### Phase 1 — Authentication and account safety

- [ ] Configure Collar app auth over users/derived `is_active`; implement durable encrypted browser-session bridge, single-writer refresh rotation, auth tokens and CSRF. Keep native token-issuing paths inaccessible to browser clients.
- [ ] Reuse Collar login/refresh/logout/password/session APIs internally; build application registration/verify-email/reset flows and immediate session revocation checks. Do not duplicate JWT/refresh-token authority.
- [ ] Enforce Argon2id, rate limits, cookie flags, generic anti-enumeration responses and role middleware.
- [ ] Bootstrap application owner **through CLI**, separately from restricted Collar operator superuser; no hardcoded passwords or automatic app-owner elevation.
- [ ] Build signup/signin/settings UI and tested redirect-back behavior.
- [ ] Add DB constraints and integration tests for sessions and concurrent login/logout.

**Exit:** logged-out visitor reads public pages; verified reader can log in; unverified account cannot comment or link Patreon; no role escalation via edited requests; forgotten-password requests reveal no account existence.

### Phase 2 — Catalogue model + reader foundation

- [ ] Implement pen names, works, genres/tags, entries, volumes, immutable revisions, collections, redirect slugs and asset metadata.
- [ ] Implement public work/author/collection/catalogue/search routes with filters, proper pagination and hidden/draft enforcement.
- [ ] Build Home, Discover placeholder rails, Novels, Short Stories, Work Overview, Pen Name and Anthology pages with empty states.
- [ ] Build Markdown Goldmark sanitization and reading-shell component.
- [ ] Add reader settings, toc, previous/next, back/forward and deep-linked route refresh.
- [ ] Validate short story one-entry invariant and novel chapter ordinal rules in DB/service.

**Exit:** seed one novel with 3 chapters, one standalone short story, and one collection; reading the short story shows a single uninterrupted text with no chapter numbering. Hidden drafts cannot be fetched by public API.

### Phase 3 — Publishing studio and schedules

- [ ] Build embedded Bootstrap author/admin dashboards using native Collar collections for listings/allowed CRUD. Add ownership/field rules and read-only aggregate views; custom commands only for atomic publishing/validation workflows. Verify raw native mutation/expand routes cannot bypass protections.
- [ ] Add CodeMirror Markdown editor with autosave, preview, revision versions, conflict recovery, revision history, restore.
- [ ] Add image upload/variant generation with safe format validation and asset permissions.
- [ ] Implement scheduling with UTC conversion, bulk novel chapter scheduling, immutable published revision, max 30-day lead, tier requirement for early release and immediate-public release mode.
- [ ] Implement release status calculated from `now`, not job progress. Public list updates use `public_at` only.
- [ ] Add editorial audit events; article correction does not bump Latest artificially.

**Exit:** author schedules tomorrow-public chapter and supporter-early chapter; before proper time anonymous reader cannot read it; simulated clock passing public time makes it available without a worker. Draft edits remain private until explicit publish/swap.

### Phase 4 — Reader library and comments/reviews

- [ ] Add guest local reading progress, authenticated sync/merge, library status/favorites, follows for works and pen names.
- [ ] Add review CRUD (one review per account/work), star distribution, spoiler handling, review moderation state.
- [ ] Add entry comments and one reply level, edit/delete/spoiler/report, rate limits and moderation tools.
- [ ] Build reading settings controls, keyboard navigation and near-end finish affordance.
- [ ] Implement notifications table and in-app unread views for releases/replies; no unsolicited emails.

**Exit:** progress continues across devices for logged-in account; a short story can be marked finished without generating chapters; duplicate ratings/likes cannot create duplicate rows; hidden comment isn't rendered; a non-owner cannot edit another reader's text.

### Phase 5 — Patreon production integration

- [ ] Implement creator connection owner UI and secure token storage; Patreon API v2 client with explicit scopes/fields, refresh retries and bounded HTTP timeouts.
- [ ] Implement user Patreon link OAuth code/state flow, conflict handling, disconnect, status and force refresh.
- [ ] Implement tier mapping, rank-based entry access, grant/revoke logic and short-lived cache.
- [ ] Add HMAC-verified v2 membership webhooks, dedupe/reconciliation, provider outage handling, delayed job retry, admin integration status.
- [ ] Write mock server fixtures reproducing actual documented v2 JSON:API relationships, including member/campaign/tier IDs and error statuses. Include live-test checklist for operator-run real Patreon account.
- [ ] Apply `no-store` rules, protect API cache and remove early bodies from TOCs/search/previews.

**Exit:** verified Tier A reader sees Tier A early chapter but not Tier B; unrelated campaign patron cannot read; invalid webhook gives 401; public release happens correctly during simulated Patreon outage. Production launch with early-access feature **requires** successful real-provider test; otherwise disable early access feature flag and launch free catalogue only.

### Phase 6 — Analytics collection and work dashboards

- [ ] Implement Redis-first view counters and associated validated events/session data, atomic UUID dedupe, opt-out, plausibility checks and bounded rate limits; no process-memory authority or per-view SQL writes.
- [ ] Implement Collar-queued flush every **120 seconds**, startup backlog reclaim, SQL event/batch dedupe and commit-before-ACK; prove multi-worker/concurrent ingestion correctness and bounded shutdown behavior.
- [ ] Verify AOF/volume persistence, Redis capacity/outage behavior, SQL outage/backlog retry, cross-flush session qualification, pending-data deletion suppression and health/lag reporting.
- [ ] Implement privacy-aware qualification ledger, daily aggregates, expiry cleanup and export/suppression.
- [ ] Implement studio analytics dashboards through authorized native Collar aggregate endpoints, showing last-flush time/lag and correct story/novel denominators.
- [ ] Add source impression events and unique-reader protections; exclude own-author/admin reads.
- [ ] Run abuse simulation and verify metrics from seeded deterministic events.

**Exit:** refreshing 100 times doesn't count as 100 readers; a 15-minute background tab with no active reading does not count as 15 minutes; opted-out user can still save progress; analytics for tiny groups doesn't expose individual behavior; pending views survive Go restart; repeated SQL-commit/Redis-ACK crashes and concurrent flushes do not inflate counts; healthy flushing runs every 120 seconds.

### Phase 7 — Ranking and discovery

- [ ] Implement typed ranking config + version, format-specific Bayesian components and fallback for unavailable denominators.
- [ ] Add Trending, Top Rated, New & Underexplored, Latest and Featured rails; load snapshots rather than calculating from events in requests.
- [ ] Implement format interleaving, genre filters, basic exploration quota, deterministic daily seed, cursor signing and small-catalogue graceful fallback.
- [ ] Build owner rank snapshot inspector showing component values, sample size and previous algorithm version; changes audited.
- [ ] Unit tests with fixtures including short story vs giant novel, new author, one reader all-positive, review brigade and different release schedules.

**Exit:** 100 posted chapters without reads offers no score bonus; two works with equal reader satisfaction but vastly different word counts are comparable within their formats; exploration remains available for new short fiction; public discovery lists never expose drafts.

### Phase 8 — Content moderation and administrative safety

- [ ] Implement report queue, comment/review hide/restore, work hiding, copyright/contact route, audit log and admin work controls.
- [ ] Publish terms/privacy/content/community/copyright pages; no contract promising exclusivity, no AI-generated dump publication, no secret paywall.
- [ ] Implement protected user suspensions, safe unpublish, account deletion request and restricted staff visibility.
- [ ] Add abuse rate limits, CSP, security headers, upload validation and end-to-end unauthorized API tests.

**Exit:** reported comment can be moderated; hidden story is removed from catalogue and API reads; moderator cannot view Patreon secrets; author cannot permanently remove another author's novel; user deletion has documented handling.

### Phase 9 — QA, documentation and production readiness

- [ ] Create complete seeded demo library (see section 16), screenshot mobile/tablet/desktop routes and correct visual defects.
- [ ] Run backend/frontend tests, integrated DB migration checks, k6 smoke, Playwright including auth/reading/studio/Patreon simulated and accessibility scans.
- [ ] Verify API/worker graceful shutdown, Collar job/outbox recovery, Redis pending/AOF recovery, coordinated backup/restore without replay inflation, cold start, email retry, release timestamps and non-destructive deployment.
- [ ] Confirm both embedded UIs work without runtime Node/bun or external CSS/JS/font requests; test native Collar contracts/ownership and custom DTO contracts separately.
- [ ] Generate and review docs (`README`, `API`, `DB`, `RANKING`, `PRIVACY`, `RUNBOOK`, `TEST_REPORT`, `DECISIONS`).
- [ ] Build binary, inspect deployment plan, **ask explicit operator approval before touching production**; stage and smoke test, then production deploy only upon authorization.

**Exit:** all V1 acceptance tests pass and artifacts exist; no test entitlements or example secrets enabled in production; app publicly exposes only intended novel/story bodies.

---

## 16. Fixtures and deterministic test scenarios

Create test fixtures (fictional metadata, synthetic text, no copyrighted external novels) in `tests/fixtures`, seeded only under development/test guards:

```yaml
users:
  - reader_verified:  reader1@example.invalid
  - reader_unverified: reader2@example.invalid
  - author_owner: owner@example.invalid
pen_names:
  - slug: raven-vale
    title: Raven Vale
  - slug: a-n-other
    title: A. N. Other
works:
  - slug: holunkai
    kind: novel
    pen_name: raven-vale
    status: ongoing
    entries:
      - slug: the-last-breath
        ordinal: 1
        release: public
      - slug: the-wrong-savior
        ordinal: 2
        release: public
      - slug: the-first-specimen
        ordinal: 3
        release: early_rank_1_future_public
  - slug: the-locked-study
    kind: short_story
    pen_name: a-n-other
    entries:
      - slug: full-story
        ordinal: 1
        release: public
  - slug: beyond-the-door
    kind: short_story
    pen_name: a-n-other
    entries:
      - slug: full-story
        ordinal: 1
        release: early_rank_2_future_public
collections:
  - slug: unsettling-tales
    items: [the-locked-study, beyond-the-door]
```

Fixtures must also include: a hidden unpublished draft, an archived entry, reviews with a spoiled comment, a moderator-removed comment, tier mapping Rank 1/2, fake member and canceled-member fixture, 500-reader synthetic ranking sample, guest progress and account-merge conflict. Write clock-driven test fixtures with exact UTC dates relative to fake clock, not real wall-clock test flakiness.

### 16.1 Required automated test matrix

| ID | Scenario | Expected result |
|---|---|---|
| AUTH-01 | Reader registers and verifies email | Can comment, follow and link Patreon after verification |
| AUTH-02 | User logs in with wrong password 20 times | Throttled; no user-enumerating errors |
| AUTH-03 | Reset token reused | First succeeds, second rejected |
| AUTH-04 | Reader passes `{role:'owner'}` in PATCH | Rejected, role unchanged |
| CAT-01 | Create short story | Exactly one entry, reader sees no chapter UI |
| CAT-02 | Add second entry to short story | Validation rejects even through API |
| CAT-03 | Add chapters to novel | Stable ordered table of contents |
| CAT-04 | Reorder existing chapter | URLs, bookmarks, progress IDs remain valid |
| CAT-05 | Request hidden unpublished body by ID | `404`, no body bytes or metadata leak |
| CAT-06 | HTML script in Markdown/comments | Escaped/removed; no script execution |
| CAT-07 | Anonymous reads full public story | `200`, no login required |
| PUB-01 | Future scheduled chapter pre-early | Nobody except authorized draft preview can read body |
| PUB-02 | Rank 1 early date passed | Rank 1 and 2 readers can read, anonymous cannot |
| PUB-03 | Rank 2 early date passed | Only Rank 2 can read |
| PUB-04 | Public date passed and worker stopped | Everyone can read immediately |
| PUB-05 | Early-public gap >30 days | Rejected |
| PUB-06 | Scheduled entry missing `public_at` | Rejected |
| PUB-07 | Draft autosave conflicts | `409`, both versions recoverable |
| PUB-08 | Correct typo after release | No fresh-public-update ranking bump |
| PUB-09 | Author clicks complete with unreleased drafts | Warning; no silent deletion or public exposure |
| LIB-01 | Guest progress imported after login | Safe merge, no overwrite of newer server progress |
| LIB-02 | Reader on laptop resumes on phone | Same entry/fraction after sync |
| LIB-03 | Save/unsave repeatedly | No duplicate follow/engagement counts |
| COM-01 | Same reader reviews twice | Second update, not duplicate |
| COM-02 | Reader attempts to edit other's comment | Forbidden |
| COM-03 | Third-level reply | Rejected |
| COM-04 | Moderator hides reported review | Removed from public aggregate after rebuild |
| PAT-01 | Patreon not connected | Early body inaccessible |
| PAT-02 | Foreign campaign membership | Early body inaccessible |
| PAT-03 | Correct entitled Rank 1 | Rank 1 yes; Rank 2 no |
| PAT-04 | Correct entitled Rank 2 | Rank 1 and 2 yes |
| PAT-05 | Membership revoked by signed webhook | Cache invalidated, access reconciled/removed |
| PAT-06 | Forged webhook and duplicate replay | Forged rejected; duplicate idempotent |
| PAT-07 | Patreon API outage | Public story still works; early access follows bounded cache/fail-closed policy |
| PAT-08 | Account A Patreon linked on B | Safe conflict, never shared entitlement |
| ANA-01 | 100 refreshes by one reader | One or fewer qualified unique starts, not 100 |
| ANA-02 | Idle/background page sends false heartbeat | No falsely qualified completion |
| ANA-03 | Author reads own work | Excluded from rank engagement |
| ANA-04 | Reader opts out | No optional analytics; progress still works |
| ANA-05 | Accept views, restart API before flush | Pending counts/data survive in Redis and flush on recovery |
| ANA-06 | Healthy worker crosses repeated 120-second boundaries | Accepted events flush within interval plus bounded processing; dashboard shows last flush/lag |
| ANA-07 | Crash before SQL commit or after commit before Redis ACK | Retry/reclaim loses no pending events and applies each eligible contribution once |
| ANA-08 | Two flush workers while new events arrive | No dropped new events, duplicate batches or inflated unique counts |
| ANA-09 | MariaDB unavailable for multiple intervals | Redis retains pending data; recovery drains backlog without re-aging accepted events |
| ANA-10 | Redis unavailable/full | No success acknowledgment for unbuffered events; bounded failure, reading/progress/publishing continue |
| ANA-11 | Redis restarts with persisted AOF/volume | Pending data recovers according to documented fsync policy; host-crash loss window stated |
| ANA-12 | Opt-out/account deletion before delayed flush | Pending data suppressed/purged; flush cannot recreate deleted histories |
| COL-01 | Author lists another author's draft through native API/expand/realtime | No draft/body exposure |
| COL-02 | Direct native PATCH of role/release/revision/protected fields | Rejected; custom atomic command required |
| COL-03 | Normal app owner calls Collar superuser admin APIs | Denied; operator session required, no silent elevation |
| COL-04 | Two tabs refresh one browser session concurrently | One serialized rotation; no reuse revocation or token exposure |
| COL-05 | Native dashboard list and custom reader API | Each matches its own documented envelopes, pagination and errors |
| UI-01 | Reload reader, studio and admin deep links in built binary | Correct embedded entrypoint/assets; unknown API/asset paths never HTML |
| UI-02 | Run built binary from empty working directory, block external network | Both UIs retain Bootstrap CSS/JS/Icons from embedded assets, no CDN dependencies |
| RANK-01 | Only 3 perfect readers vs 500 good readers | Bayesian confidence prevents dominance from tiny sample |
| RANK-02 | 2,000-word story vs 200-chapter novel | Fair separate format categories; no length bonus |
| RANK-03 | Author publishes 10 low-read chapters | Latest one entry/work, rank has no volume bonus |
| RANK-04 | New zero-exposure work | Eligible for exploration, not fake Top Rated |
| RANK-05 | Missing opportunity to continue novel | No zero retention penalty; component omitted |
| RANK-06 | Suspicious coordinated clicks | Excluded/flagged without deleting legitimate reviews automatically |
| SEC-01 | XSS via cover metadata/Markdown/comment | No script execution |
| SEC-02 | Direct studio API as another author | `403`/`404`, no draft body |
| SEC-03 | CSRF attempt from other origin | Rejected |
| SEC-04 | Uploaded script disguised as image | Rejected |
| OPS-01 | Server restarts with pending release | Public access clock-correct; notification job recovers |
| OPS-02 | DB backup restores on test DB | Counts/relations/content intact |
| OPS-03 | SPA direct reload `/fiction/holunkai` | Route loads, API returns independently |
| OPS-04 | App deploy on shared production VPS | No unrelated services touched |

### 16.2 API contract verification

Generate OpenAPI 3.1 for custom reader/command APIs from typed handlers/spec or maintain a checked-in equivalent. Snapshot/verify Collar-generated `/api/v1/pawn/openapi.json` separately using an authorized identity; preserve native endpoint contracts instead of relabeling them as custom DTOs. Test route registration against the pinned Collar library and the ingress browser-session/CSRF restrictions. Every route must have: auth level, request/response schema, status codes, cache classification, errors, rate limits, tests and owner. Frontend DTO types generated from OpenAPI where possible. Test schema validation for unknown fields, oversized strings, invalid email, duplicate slugs, malicious Markdown, impossible dates, conflicting chapter ordinals, missing tier config and empty responses. Include browser automation for one happy-path and one failure-path per feature family.

### 16.3 Manual UX verification (must actually inspect)

- Mobile widths 320/375/430px, tablet 768px, desktop 1280/1920px; no horizontal overflow or clipped reader menus.
- Light, dark and sepia; font 14/26px; long novel paragraphs; non-Latin names; low vision/high contrast; focus and keyboard navigation.
- On mobile, search filter, story card, TOC, chapter next, comments, review composer and Patreon connection must be operable without hover.
- Reader pages retain readable width; don't render giant app-dashboard chrome while reading.
- Draft/editor pages clearly distinguish `Save`, `Schedule`, `Publish now`; publishing destructive consequences confirmed.
- Countdown uses correct reader timezone, server UTC authoritative; DST in reader-selected timezone handled correctly.
- Anonymous private links must never show early content even if browser cache contains an authenticated response.
- Every empty-state screen tells the reader what they can do next without fake sample stories in production.

---

## 17. Production launch criteria and rollback plan

### 17.1 Go-live checklist

- [ ] Bare-metal **Nginx -> systemd -> Go binary** deployment for **pawn.ink**; TLS valid, `nginx -t` passes and traffic routes only to the app's approved loopback port; dedicated API/worker units healthy.
- [ ] Pawn.Ink and Collar system databases isolated/backed up; migrations recorded; runtime DB user non-DDL privileged.
- [ ] Redis AOF/volume/noeviction and recovery verified; worker flushes every 120 seconds, lag observable; both embedded Parcel/Bootstrap/Icon UIs pass offline-asset smoke checks.
- [ ] Production secrets random, unique, outside Git; all development/test data and entitlements disabled.
- [ ] `APP_BASE_URL` and Patreon OAuth redirect match the live domain exactly.
- [ ] No real users created using seed fixtures; owner bootstrap secure.
- [ ] At least one complete short story and one public novel chapter published using actual CMS, not manual SQL.
- [ ] Reader signup, verification, login, review, comment, save, follow and progress all work.
- [ ] Public and early timestamps verified using controlled integration clock fixtures plus real server time.
- [ ] Patreon API v2 creator+patron real-account integration validated **if feature enabled**.
- [ ] Webhook signature verified with documented provider secret; callbacks not logging tokens.
- [ ] Discovery cards and ranking snapshot show real/insufficient data, no fabricated engagement.
- [ ] Abuse reports and copyright contact work; community/content/privacy policies published.
- [ ] XSS/CSRF/authz tests pass; no early bodies cached publicly.
- [ ] Backups tested by restoring to separate test environment; alerting and runbook in place.
- [ ] `make smoke` passes after deploy; any failure triggers documented dedicated-app rollback only.

### 17.2 Restore plan

1. Stop only this app service (or turn on maintenance mode); never stop an unrelated service.
2. Confirm backup integrity and encryption key availability. Verify backup timestamp and schema version.
3. Restore to **new isolated database first**, sanity check users, works, entries, revisions and links.
4. If recovery of production necessary and explicitly approved, take an additional emergency backup then switch only this app's database credentials/config to recovered dataset.
5. Restore assets using keys referenced by DB, ensure access policies correct; restart dedicated service, run tests and check release timestamps.
6. If Patreon token decryption fails, require safe reauthorization rather than exposing secrets; public chapters must remain readable.

### 17.3 Maintenance operations

Document how to: invite an author manually using owner-controlled admin/CLI (only if desired), add pen names, reset lost owner password from CLI with operator verification, rotate secrets and Patreon tokens, archive a work, redact a comment, resolve a report, regenerate rankings, restore backups, regenerate cover thumbnails, inspect stalled jobs, export aggregate analytics, change release dates with audit, and deploy/rollback binary. Keep these instructions inside `docs/RUNBOOK.md`.

---

## 18. Completion deliverables — mandatory, not aspirational

A coding agent may say **IMPLEMENTED** only if all of the following exist:

1. Runnable Collar-based `cmd/server`, `cmd/worker` and CLI binaries; Parcel-built reader and author/admin SPAs embedded with locally bundled Bootstrap 5/Bootstrap Icons, all core routes, custom reader APIs and direct native Collar dashboard APIs.
2. Database migrations covering all V1 tables with indexes and integrity constraints; safe DB bootstrap and dev fixtures.
3. Working pen names, novels, *single-entry* short stories, collections, novel volume optionality, cover assets, content editor, revisions, preview and publication schedules.
4. Working login/register/email verification/reset/session security, reader library, anonymous and registered progress, follows, ratings, reviews, comments, spoilers, report and moderation flows.
5. Public/early content access enforcement with server-side Patreon API v2 integration, owner tier mapping, encrypted tokens, webhook verification, and real-provider validation when credentials available.
6. Persistent Redis-first views/events with 120-second idempotent worker flush, restart/outage/replay tests, analytics/ranking fixtures, native Collar aggregate dashboards and fair discovery with underexposed works.
7. Responsive, accessible mobile/desktop reader including themes, font settings and next/chapter navigation.
8. Tests for every acceptance matrix item; CI or reproducible test script; vulnerabilities documented and addressed.
9. Documentation: deployment, backup/restore, API and DB references, decision log, privacy/moderation, error/incident runbook, test report.
10. Safe operator-approved production deployment **only when explicitly requested**. Otherwise leave tested application runnable locally/staging and report deployment as pending, not completed.

**Every missing capability must be declared explicitly** in a final status table `Implemented | Tested | Blocked/Deferred`, with evidence commands. Never represent mocked Patreon credentials or a non-running UI as a finished integration. Work through all phases without unnecessary clarification; stop only for destructive action authorization, missing credentials/approvals required for a real external connection, or genuinely irreversible/legal decisions. Preserve user data and ship useful completed functionality whenever externally blocked.

---

## 19. Final product rules (quick agent reference)

```text
PRODUCT                 Pawn.Ink — reader-first fiction library / independent publishing imprint
PRODUCTION ORIGIN       https://pawn.ink
REPOSITORY              https://github.com/namankumar80510/panw.ink.git
INITIAL PUBLISHER       One real owner, multiple pen names, optional invite-only authors later
PUBLIC READING          Free, no account needed
PREMIUM                 Patreon-verified temporary early access only
PERMANENT PAYWALL       Forbidden for narrative content
STORY FORMAT            Exactly one complete entry, no chapters
NOVEL FORMAT            Ordered chapters, optional volumes
ANTHOLOGY FORMAT        Collection of separate short-story works
FRONTEND                Parcel + Mithril.js + TypeScript SPAs, Go-embedded, no SSR
UI                      Bootstrap 5 + Bootstrap Icons via npm/bun, bundled locally; no CDN
BACKEND                 Private Collar library + custom reader/atomic-command handlers
DASHBOARDS              Direct native Collar APIs; embedded author/admin UI
DATABASE                MariaDB app (Goose) + separate Collar-managed system DB
VIEW / EVENT BUFFER     Persistent Redis first; worker flush every 120 seconds, no in-memory authority
DISCOVERY               Separate format scores, Bayesian sample correction, exploration quota
ENGAGEMENT              Unique qualified readers, continuation/completion, save/follow, rating
AI DUMP STORIES         Prohibited, human editorial review; assistive editing permitted
FORUMS / DMs            None
READERS FOLLOW          Works and pen names, not other readers
AUTHOR CONTRACTS        No exclusivity or copyright transfer; minimum hosting TOS if invited
PUBLIC AUTHOR SIGNUP    None in V1; invitation/admin approval only
PAYMENTS                External Patreon (not platform checkout), no coins
EPUB / PDF              Deferred to V1.1, with author controls
SEO CAMPAIGN / SSR      None; shareable paths and basic accessibility only
CODE OWNERSHIP          Proprietary application; no LICENSE file required
DEPLOYMENT              Bare metal: Nginx -> systemd -> Go binary; separate Go worker unit
OPERATIONAL RULE        Never modify/destroy unrelated production state
```

## 20. Technical references for implementation verification

- Patreon API reference (OAuth v2, members, entitlements, webhook scopes): https://docs.patreon.com/
- Patreon OAuth authorization + exchange: https://www.patreon.com/oauth2/authorize and https://www.patreon.com/api/oauth2/token (use documented parameters and securely registered redirects).
- Patreon webhook events: documented v2 `members:*` events; current `X-Patreon-Signature` uses HMAC-MD5 over raw payload, verify according to up-to-date docs.
- Go modules and dependency pinning: https://go.dev/ref/mod
- Parcel documentation: https://parceljs.org/
- Bootstrap 5 documentation (install as local packages): https://getbootstrap.com/docs/5.3/getting-started/download/
- Bootstrap Icons (local package): https://icons.getbootstrap.com/
- Redis persistence and streams (confirm pinned version behavior at implementation): https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/ and https://redis.io/docs/latest/develop/data-types/streams/
- Private Collar source/module: `github.com/xmfunew/collar`; reviewed local sources/commit in section 1.1.
- Mithril documentation: https://mithril.js.org/
- MariaDB official docs: https://mariadb.com/kb/en/documentation/
- OWASP cheat sheets: https://cheatsheetseries.owasp.org/
- WCAG 2.2: https://www.w3.org/TR/WCAG22/

**End of specification.** Build V1 exactly as defined, with tests and honest reporting of external integration blockers. Do not silently expand into an author marketplace, social network, SEO publisher, permanent paywall, or AI-content repository.
