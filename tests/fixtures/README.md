# Development fixtures

`make seed-dev` (or `go run ./cmd/seed`) loads the deterministic catalogue
of IMPLEMENTATION.md §16 into a **development or test** database. The command
refuses any other `APP_ENV`. The data lives in `internal/seed`. All prose is
synthetic text written for this repository, and release dates are relative
to the moment of seeding.

| Account | Role | Verified |
|---|---|---|
| `owner@example.invalid` | owner (author if an owner already exists) | yes |
| `reader1@example.invalid` | reader | yes |
| `reader2@example.invalid` | reader | no |

Development-only password for all three: `pawn-dev-fixture-passphrase`
(override with `SEED_PASSWORD`). Never seed a production database.

| Fixture | Purpose |
|---|---|
| `holunkai` (novel, Raven Vale) | ch 1–2 public, ch 3 early-access (public in 6 days), ch 4 draft, ch 5 archived; one volume; ch 2 contains a `<script>`/`javascript:` sanitization probe |
| `the-locked-study` (short story, A. N. Other) | complete public story |
| `beyond-the-door` (short story) | early-access story (public in 5 days) |
| `the-quiet-manuscript` | work in draft visibility with a released-looking entry: must 404 |
| `a-withdrawn-tale` | moderation-hidden work: must 404 |
| `unsettling-tales` | anthology of the two A. N. Other stories |
| `/fiction/holunkai-old` | slug redirect to `/fiction/holunkai` |
| Tag `proposed-tag` | unapproved: never shown publicly |

Fixtures for reviews, comments, Patreon tiers, ranking samples and progress
merges arrive with their phases.
