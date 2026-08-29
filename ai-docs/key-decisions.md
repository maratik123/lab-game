# Key decisions

Decisions with the reasoning that produced them, so a later reader does not re-litigate a settled trade-off. Game-design decisions live in [`docs/DESIGN.md`](../docs/DESIGN.md) and are **not** duplicated here; this page carries the engineering and repository decisions, each pointing at its source.

Format: **KD-N — decision** · *why* · *consequence* · *source*.

## Stack

**KD-1 — Go.** Compilation and static types, a cheap instance (native binary), concurrency and networking in the standard library. Rust with `teloxide` was evaluated and rejected: incomplete Bot API surface plus framework opinionation. *Consequence:* the harness gates are `go build` / `go test -race` / `golangci-lint`. *Source:* `docs/DESIGN.md` §11.

**KD-2 — `telego` (mymmrac/telego), low-level layer only.** Types and methods are generated one-to-one from the official documentation, so coverage is complete by construction. The helper layer is not used; a thin project-owned layer adds retries, idempotency and rate limiting. Defaults `fasthttp`/`go-json` are replaced with `net/http` and `encoding/json` (supported swap). *Fallback if it disappoints:* `go-telegram/bot`. **Never** `go-telegram-bot-api` (frozen). *Source:* §11.

**KD-3 — Postgres + `pgx` + a migration tool.** One database holds the ledger, the world, sessions, the event log and the scheduler queue — which is what makes a self-check like "opening + turnover == closing" meaningful at all. *Source:* §11.

**KD-4 — A self-written scheduler on Postgres, not a job framework.** `scheduled_tasks` plus a worker on `SELECT … WHERE run_at <= now() FOR UPDATE SKIP LOCKED`, each task executing in one transaction with its effects. ~100 lines, canonical pattern, horizontally scalable by adding a worker. `gocron` rejected (in-memory; one-shot tasks must survive restarts), Temporal rejected (overkill). *Escape hatch if it grows features:* River. *Source:* §11.

**KD-5 — Double-entry ledger with a global World account.** Append-only postings, zero-sum per kind enforced at write time, balances materialised only for controlled accounts, daily close for point-in-time balances. Pure event sourcing was rejected (a tax on every read); materialising a balance for *every* account was rejected (a hot row on World would be a global mutex on the economy). *Source:* §11, [`domain-invariants.md`](domain-invariants.md).

**KD-6 — A second machine for items with identity.** Instance-carrying items are never ledger kinds; ownership is a chain of movements. *Source:* §11.

**KD-7 — Lazy ticks instead of cron.** State is computed from timestamps at read time, so the bot is a stateless update handler over Postgres and restarts are trivial. The single true cron is the daily ledger close; the standing timer is the designed exception and rides the scheduler. *Source:* §11.

**KD-8 — Raid session as an explicit FSM row.** No goroutines or in-memory timers; a restart is invisible to sessions; every transition is a basis document, so the transition log is a ready-made raid trace. *Source:* §3.5.

## Infrastructure

**KD-9 — Self-hosted `telegram-bot-api` from day one.** Motivation is a lived incident: the shared `api.telegram.org` frontend can wedge a specific bot for hours with no 429 and no error, and reissuing the token does not help because the state hangs on the bot id. A local instance talks MTProto to the DC directly and exposes `/stats`. It does **not** relax message rate limits — those are enforced by Telegram's core. *Source:* §12.2.

**KD-10 — Long polling, not webhooks.** Only outbound traffic is needed: no public IP, no domain, no TLS — so the whole thing can live on a home machine until a Mini App (radar) forces a public HTTPS endpoint. *Source:* §12.1.

**KD-11 — The test bot runs against cloud `api.telegram.org` while production runs on our instance.** Two independent paths to Telegram: "test bot works, production hangs" localises an incident instantly. Isolation is by configuration, never by code — same image, different env. *Source:* §12.5.

**KD-12 — A restore into testing is always sanitised.** Chat/user ids rewritten, outbound queue drained, updates offset reset — otherwise the test bot messages real people on the first tick. The same script doubles as the backup-restorability check, so that duty stops being a separate task. *Source:* §12.3, §12.5.

## Repository and harness

**KD-13 — Module path `github.com/maratik123/lab-game`.** The repository is named for the project directory, not for the game's future name (which is an open question, §16.6), so finding the name later costs no import rewrite. *Decided:* 2026-08-29.

**KD-14 — Private repository, no server-side protection.** GitHub refuses branch protection and rulesets on a private repository on the free plan (`403: Upgrade to GitHub Pro or make this repository public`). The design document's economy and anti-abuse rules are not published while the repository is private. *Consequence:* `main` is protected only by the local commit hook and honour-system rules — see `AGENTS.md` § Permissions. Revisit when the repository goes public or the plan changes. *Decided:* 2026-08-29.

**KD-15 — The harness is ported from `graphite-gp` (itself evolved from `quartzite`), not reinvented.** What travels is the process — spec-driven `/task` flow, the learning loop, the propagation rule, the review agents. What is rewritten is every language-bound surface (gates, style, test and doc conventions, hooks) plus new domain rules for the ledger, telemetry and Telegram safety. Citations inherited from those projects are namespaced (`graphite-gp`'s log, `maratik123/quartzite#N`) rather than left as bare `#N`, which would resolve against *this* repository and lie. *Decided:* 2026-08-29.

**KD-16 — Strict `golangci-lint` from the first commit.** Enabled beyond the default set: `exhaustive` (an FSM switch must be total), `rowserrcheck` / `sqlclosecheck` (database correctness), `errorlint`, `nilerr`, `bodyclose`, `noctx`, `gosec`, `revive` with exported-doc rules. A finding is fixed, not silenced; `nolintlint` requires a specific linter and a stated reason on any exception. *Decided:* 2026-08-29.
