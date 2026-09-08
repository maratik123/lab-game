# Go Agent Rules

**CRITICALLY**
1) **English for every durable artefact** — code, comments, commit messages, PR bodies, instruction files, specs, designs, `learnings.md`. **Russian for two surfaces only:** conversation with the product owner, and `docs/**` (the game-design corpus is Russian by decision — do not translate it).

## Project

**lab-game** — a Telegram bot game ("Лабиринты", working title): a group chat becomes a settlement in a shared, infinite hex maze; raids, instant auto-combat, stamina, extraction (backpacks / corpses / looting / rescue raids), asynchronous PvP by trails, seasonal world rotation. Go + Postgres, Telegram Bot API via long polling against a self-hosted `telegram-bot-api` instance.

> **AXIOM — `docs/DESIGN.md` is DECISIONS; `docs/IDEAS.md` is NOT.**
> The boundary between the two files is *designed / not designed*, **not** *MVP / post-MVP*. The test the corpus itself states: *"can an agent implement this without making design decisions?"*
>
> | Surface | Standing |
> |---|---|
> | `docs/DESIGN.md` | **Implement from it. Never redesign it** without an explicit user request. §14 is the MVP scope; §15 is the deferred backlog with reasons; §16 is the open-question list. |
> | `docs/DESIGN.md` outside §14 | Still binding as a **constraint**: MVP code must not hard-wire assumptions that contradict designed post-MVP behaviour (the canonical example: a raid session is modelled for many participants even while raids are solo). |
> | `docs/IDEAS.md` | **Never implement, never encode in a schema.** Directions without mechanics. A file promotes to `DESIGN.md` only through an explicit user decision. |
> | An open question in §16 | Not a licence to decide it silently. Surface it; balance numbers go to config, never to code. |

> Read [`ai-docs/context.md`](ai-docs/context.md) for orientation, architecture, and the standing decisions — on demand. The canonical spec is [`docs/DESIGN.md`](docs/DESIGN.md); its idea backlog is [`docs/IDEAS.md`](docs/IDEAS.md).

## Permissions

Machine-enforced rules live in `.claude/settings.json` (allow/deny entries, hooks). Read that file for the authoritative list — duplicating it here lets the two sources drift.

> **`origin` enforces NOTHING.** `maratik123/lab-game` is a **private repository on a free plan**, where GitHub refuses both branch protection and rulesets (`403: Upgrade to GitHub Pro or make this repository public`). CI runs on every PR and every push to `main`, but **no check is required** and nothing blocks a merge or a force-push. Every `main`-protection rule below is therefore enforced by the local `PreToolUse` commit hook plus honour-system discipline — treat them as *harder* obligations, not softer ones, because a red check will not stop you.

Honor-system rules (no machine check; still binding):

- **DENY:** `git push --force` to feature branches — prefer `--force-with-lease`, and only after explicit user approval. Never force-push `main`.
- **DENY:** `git push` to `main`. Every change reaches `main` through a merged PR.
- **DENY:** files outside project root.
- **DENY:** a real bot token, DSN, or `api_id`/`api_hash` in any tracked file — including a test fixture, an example, a commit message, and a PR body. Secrets live in `.env` (gitignored) and in the deploy environment. A leaked token is rotated through BotFather, not edited out of history.
- **ASK:** any tool not allow-listed in `settings.json`; if denied — suggest an alternative.

On session start: read `.gitignore`, treat matched paths as a read blacklist.

## Build & Test

```bash
make verify                                             # every gate below, in one run
go build ./...                                          # whole module
go test ./...                                           # all tests
go test ./internal/raid/ -run TestName                  # filter
go test -race ./...                                     # race gate (required for concurrent code)
make test                                               # the whole suite against ONE shared Postgres
make test-race                                          # the race gate on that same route
make test-db-up                                         # bring up a long-lived shared test server (CLIENTS=N sizes it)
make test-db-down                                       # remove it — nothing else can
make test-fallback                                      # the per-binary container path's own gate
make test-contention                                    # the race gate under induced cross-package load
go vet ./...                                            # vet (also inside golangci-lint)
golangci-lint run                                       # strict lint gate
golangci-lint fmt                                       # apply every enabled formatter
golangci-lint fmt -d                                    # format check — non-zero exit = dirty
go mod tidy && git diff --exit-code go.mod go.sum       # module hygiene gate
make comment-refs                                       # the comment-reference ban, over the whole tracked gated set
actionlint .github/workflows/<file>.yml                 # required gate for any new/modified workflow file
shellcheck <script>.sh                                  # required gate for any new/modified shell script
go run ./cmd/bot                                        # run the bot (exits non-zero unless .env.example's variables are exported)
```

> **`make test` and `make test-race` provision ONE Postgres for the whole run**, through
> `cmd/testpg`: it uses a `LAB_GAME_TEST_DSN` you exported as it stands, otherwise a
> long-lived server from `make test-db-up` if that one answers and admits the run, otherwise
> an anonymous container it sizes, starts and removes itself on every exit path. The
> coverage ratchet takes the same route, after its skip decision. A **bare** `go test ./...`
> is unchanged and still starts one container per database-backed test binary — that is the
> designed fallback, and `make test-fallback` is the gate that keeps it executed now that no
> default gate reaches it. The connection arithmetic, the contention rule for wall-clock
> constants and the `make test-contention` probe:
> [`ai-docs/go-test-conventions.md`](ai-docs/go-test-conventions.md) and
> [`ai-docs/key-decisions.md`](ai-docs/key-decisions.md) KD-20.

> **AXIOM — `actionlint` MUST pass before `git add` on any modified `.github/workflows/*.yml`; `shellcheck` MUST pass before `git add` on any modified `*.sh`.**
> Required gates, **same status as `go build ./...` and `golangci-lint run`.** Never bypass.
>
> | If you see... | Action |
> |---|---|
> | `M .github/workflows/<name>.yml` in `git status` | Run `actionlint <file>` (pass every changed workflow file in one invocation) **before** `git add` |
> | `M <any>.sh` in `git status` | Run `shellcheck <file>` **before** `git add` |
> | Either tool reports an error | Fix it. **NEVER** bypass. |
>
> What `actionlint` catches that `go` cannot: runner-version mismatches, deprecated action versions, expression-syntax errors, shell-quoting issues. Harness scripts are executable code and get the same treatment as `.go` files.

> **A zero exit status is evidence about the LAST pipeline stage, not about your question.** Never pipe a gate whose exit code is load-bearing — `go test ./... | tail -6` reports `tail`'s status (always 0), so a RED gate records as green, and `tail -N` can truncate away the `FAIL` line you needed. Capture to a file **under `tmp/`** and grep the saved log: `mkdir -p tmp && go test ./... > tmp/gate.log 2>&1 && echo GATE-GREEN || echo GATE-RED`, then `grep -E "^(FAIL|ok|---)" tmp/gate.log`. `tmp/` is the one ignored scratch directory — a gate log, a mutation backup or a throwaway probe written to the repository ROOT is refused by a `PreToolUse` hook, because 60 of them accumulated there unseen while this sentence named a single filename. (`set -o pipefail` also works.) A `PreToolUse` hook blocks the `go test … | tail/head` form; the principle is broader than what the hook matches — the same silent-success shape covers a `jq` filter printing `null` from an error body, and a mutating flag (`rg -r`) rewriting output while exiting 0.

> **AXIOM — statement coverage may rise and may not fall.** `.githooks/pre-commit` is a symbolic
> link to `.githooks/pre-commit.sh`, which runs the comment-reference gate over the staged set and
> then the ratchet.
> `.githooks/coverage-ratchet.sh` measures `go test -coverprofile ./...`, compares it with
> the value recorded in [`ai-docs/coverage-ratchet.txt`](ai-docs/coverage-ratchet.txt), refuses a
> drop past the tolerance, and records a new high-water mark in the same commit. `make cover-ratchet`
> and CI's Test job run the identical script with `--check` — it never writes there.
>
> | State | What happens |
> |---|---|
> | No `.go` / `.sql` / `go.mod` / `go.sum` staged | Skipped, silently — coverage cannot have moved. Most commits in a `/task` run cost nothing. |
> | Unstaged edits to such files | **Blocked.** The measurement is taken on the working tree, so with them present it describes neither the commit nor the tree. |
> | Suite not green | **Blocked** — coverage is not measurable. The measurement provisions its own shared server through `cmd/testpg`. Container runtime missing? `LAB_GAME_TEST_DSN` points the suite at a server you already run — `make test-db-up` needs the same runtime, so it is the answer to a slow suite, not to an absent runtime. |
> | `go` not on `$PATH` | Skipped, loud. Named fail direction: a machine with no Go toolchain cannot measure Go coverage. |
> | Coverage fell past the tolerance | **Blocked**, with the uncovered functions listed. |
> | Ratchet file absent | Initialised at the measured value and staged. There is no separate setup step. |
>
> **The two legitimate exits from a block are: cover what the change added, or lower the recorded
> value IN THE SAME COMMIT and say in the message why the drop is correct.** `--no-verify` is not a
> third one — a `PreToolUse` hook refuses it, because a gate an agent can switch off is not a gate.
> Lowering the file is deliberately easy and deliberately visible: it lands in the diff a reviewer
> reads.
>
> The tolerance is **0.60 pp**, and it is not zero because a few timing-dependent error paths flip
> between runs — 6 statements today. Two consequences worth carrying: **a recorded mark is one
> draw, not a property of the tree** (the ratchet only ever raises, so it converges on the luckiest
> run), and the measurement runs with the Go test cache on — **but a replay is no longer
> guaranteed, and which way it falls depends on the DSN.** `testdb.Main` consults
> `LAB_GAME_TEST_DSN`, which puts it in the cache key, so under the wrapper's own anonymous
> container — a fresh ephemeral port every invocation — the database-backed packages are a
> cache miss on every commit and their timing-dependent statements are re-drawn each time.
> A re-run at an unchanged commit replays the previous profile only while the DSN is stable:
> one you exported, or `make test-db-up`'s reused named container. Re-measure with `-count=1`, in **both**
> environments, before changing the number; the script's header carries the recipe and `git log -p`
> on it carries why the number is what it is.

**CI runs the same gates** (`.github/workflows/ci.yml`, Go via `make`): Format · Build (build + vet + `go mod tidy` delta) · Test (incl. `-race`) · Lint · Harness guards (shellcheck on every script and hook body, the citation guard, the guard suites, the link check) · Actionlint. Each job is `paths-filter`-gated, so **a job that did not run is not a passing job** — read the run, not the absence of red.

Search: `ast-index` first (see [`.claude/rules/ast-index.md`](.claude/rules/ast-index.md)); fall back to `rg <pattern> --type go [-l | -C 3]` when `ast-index` returns empty.

## API Stability

> **AXIOM — No Go API-stability contract. Clean breaks, always. No compat shims.**
> lab-game is an **application**, not a library — it is never imported by anyone outside this module and has **no** downstream clients. Exported Go API may be freely renamed, removed, or restructured at any time without deprecation layers or aliases.
>
> | If you're tempted to... | Do this instead |
> |---|---|
> | Keep `func OldName(...)` delegating to `NewName` "for compat" | **DELETE** it — call sites update directly |
> | Keep both old and new APIs side-by-side temporarily | Pick one — old is gone |
> | Add an interface solely to preserve an old signature | Remove the old signature |

> **CARVE-OUT — data contracts are the opposite, and the asymmetry is the point.** The Postgres schema, the append-only `posting` / `item_movements` ledgers, the basis-document tables, the scheduler's `scheduled_task` payloads, and any persisted enum value are **live data that outlives every deploy**. They change by **forward migration**, never by redefinition: a posting written last season must still parse and still balance. A renamed state string, a re-numbered enum, or a repurposed column is a data-corruption bug wearing a refactor's clothes. Combat is the designed exception the other way (`docs/DESIGN.md` §4): `combat()` is a pure function whose internals may be rewritten wholesale — the **version of the combat system is part of every stored log and PvP-trail snapshot** precisely so that freedom stays safe.

## API Naming

Full rules — including the **`…Unchecked` AXIOM** (a function skipping a precondition check carries the suffix, and its doc comment names both the precondition and the caller that guarantees it) — live in [`ai-docs/go-api-naming.md`](ai-docs/go-api-naming.md). The three that get violated most: no stutter (`raid.Session`, not `raid.RaidSession`); interfaces named for behaviour and declared **by the consumer**; `ctx context.Context` first and never stored in a struct.

## Code Style

Thin by design — this project grows its own style rules through the learning loop (`/improve`). Start with:

- **Source files:** Go only under `cmd/*` and `internal/*`; format via `golangci-lint fmt`, never by hand.
- **Linter posture:** strict `golangci-lint run` (config in `.golangci.yml`); no `//nolint` without a specific linter and a stated reason (`nolintlint` enforces both).
- **Errors:** every returned error is handled or deliberately wrapped with `%w` and context (`fmt.Errorf("materialize node %s: %w", coord, err)`). Never `_ = err`. Never `panic` in production code — see the panic rule in *Go Test Conventions*.
- **Magic numbers:** a literal with semantic meaning becomes a named constant. **Balance constants are different and stronger: they belong in configuration, not in Go source** (`docs/DESIGN.md` §16.5 — stamina cap, step cost, timers, shop rates, door price curve, `budget(dist)`, combat dice). A tuning value hard-coded in a `.go` file is a defect even when it is named.
- **Determinism:** world generation, combat, and any PvP-trail replay are pure functions of `(seed, input)`. No `time.Now()`, no map-iteration order, and no un-seeded `math/rand` on those paths.
- **Documentation:** every exported item carries a doc comment starting with its name; every package has a package comment. See [`ai-docs/doc-convention.md`](ai-docs/doc-convention.md).
- **Comments point at nothing outside themselves.** A comment says what the thing is, states its call contract, and carries what the linter requires; it carries no markdown path, design-section number, acceptance-criterion id, decision anchor, issue number outside `TODO(#…)`, repository path, URL, or package-qualified symbol of this module named outside its own package. The reason is rot: the thing pointed at is edited and the comment becomes a claim nothing checks. `make comment-refs` gates the lexical half over `*.go`, `*.sh`, `*.sql`, `*.yml`, `*.yaml`, `.gitignore`, `.env.example`, `Makefile` and `.githooks/**`; narration and the bare unqualified name are review-judged. Full rule, exemptions and the two review-judged halves: [`ai-docs/doc-convention.md`](ai-docs/doc-convention.md) § DOC-4.
- **File size:** soft 500/800; hard 1000, and 1500 for `_test.go` — both gated; exemptions and the don't-over-split counter-rule are in `code-style.md`.

See [`ai-docs/code-style.md`](ai-docs/code-style.md) for the canonical (growing) reference.

## Domain Rules

Project invariants that outrank convenience. Full detail: [`ai-docs/domain-invariants.md`](ai-docs/domain-invariants.md).

> **AXIOM — Balances move only through the ledger, never by an ad-hoc UPDATE.**
> Every change to stamina, resources, money, or items is a set of postings written by `store.Post` under exactly one basis document, and the postings in one transaction sum to zero per kind. Item instances move through `item_movements` with an unbroken holder chain. A handler that mutates a balance column directly is rejected in review, however small the change.
>
> The action table — what to do instead of each ad-hoc write — lives with the mechanics: [`ai-docs/domain-invariants.md` § 1 — The ledger](ai-docs/domain-invariants.md).

> **AXIOM — A new mechanic declares its telemetry in the same PR that implements it** (`docs/DESIGN.md` §13.4). Events go in the event dictionary; a mechanic that moves balances additionally declares its **posting signature**, and the contract test checks actual postings against it. Telemetry never lags code.
>
> | If the PR... | It also carries |
> |---|---|
> | Adds or changes a mechanic | That mechanic's events, in the event dictionary |
> | Moves any balance | The basis document's **posting signature**, plus the contract test that checks actual postings against it |

Three more, each with its mechanics on that page: **never write to a chat that is not the intended one** (`ALLOWED_CHAT_IDS`, plus snapshot sanitisation as part of restore — §12.5); **respect Telegram limits by construction** (honour `retry_after`, back off exponentially, never a tight retry loop — a flood ban attaches to the bot id and survives token reissue); **scheduler tasks are idempotent and guard-checked** on `state`/`seq`, because a stale task firing late is normal operation, not an error (§3.5).

## Dependency Versions

> **AXIOM — Query live state BEFORE asserting any claim about an external dependency, this module's own dependency graph, an external tool's flags/behaviour, this repo's VCS state, or an upstream issue's status. Memory is stale — and so is any tool blind to the category you are asking about.**
>
> **Six categories, each with its own recipe — the command must reach the CATEGORY, or its exit 0 is about a different question than yours:** a module's published versions (`go list -m -versions <module>`, or `https://proxy.golang.org/<escaped-module>/@v/list`); whether `X` is a dependency here (`grep <module> go.mod` **AND** `go mod why -m <module>` for transitive reach — `go.mod` lists direct requirements only); an external tool's flag (`<tool> --help` or run it — **never** from memory); a file's **tracked / ignored / on-disk** status (category-matched `git` command — `git status` is **blind to ignored files**, so empty output is never proof of absence); an upstream issue's state (`gh issue view <N> --json state,comments` — the body is frozen, the **closing comment** carries the resolution); a host's **current system-wide selection** where one exists (`eselect <module> list` — the install-time default of a package describes a *fresh* system, never one that already chose).
>
> **Full per-category recipes: [`ai-docs/dependency-versions.md`](ai-docs/dependency-versions.md).**
>
> If your draft contains substrings like *"would add"*, *"introduce X as a dep"*, *"pull in X"*, *"avoid X as a dep"*, *"X is not currently a dependency"*, *"supports `--flag`"*, *"takes `--flag`"*, *"is committed"*, *"is tracked"*, *"is gitignored"*, *"there are no"*, *"still affects"*, *"is unfixed"* — **STOP**, run the relevant check, and either rewrite with the verified fact or drop the claim.

> **AXIOM — Established Go packages and the standard library first. Hand-rolling is a decision that must be ARGUED, and two arguments are refused outright.**
>
> Fewer dependencies is better, and that cuts against writing your own just as hard: code you hand-rolled is a dependency this project owns, tests and carries forever, while a settled ecosystem package is one the ecosystem already tests. **"Prefer the standard library" means stdlib over a third-party package — never *stdlib plus your own implementation* over an established one.** Where the stdlib does not cover the requirement, the next step is a settled package, not a bespoke one.
>
> | Argument offered for hand-rolling | Standing |
> |---|---|
> | *"It's only 10–20 lines — cheaper than writing the import"* | **REFUSED.** Line count is not the cost. Ownership is: the edge cases not hit yet, plus every test and review round each of them buys. |
> | *"Better to write our own than to pull in an established dependency"* | **REFUSED.** Dependency aversion is not a reason by itself. A maintained, widely-used package is the default, not the concession. |
> | The package is unmaintained or abandoned, or its API cannot express the requirement | **A real argument** — make it in the design document, naming the package and the specific mismatch. |
> | A rejected-alternatives comparison: what was evaluated, why each lost, what the escape hatch is | **The standard an argued wheel meets.** Model: **KD-4**, the self-written scheduler — `gocron` rejected as in-memory, Temporal as overkill, River named as the escape hatch. That decision stands. |

When changing dependencies: **never hand-edit a version in `go.mod`** — `go get <module>@<version>`, then `go mod tidy`, then `go build ./...`, then read `git diff go.mod go.sum` **before staging** (tidy also prunes and adds transitive lines). A new dependency needs a stated reason in the design document, and so does hand-rolling in place of one — the AXIOM above governs both directions (`docs/DESIGN.md` §11 already fixes the load-bearing ones).

## Workflow

> **AXIOM 1 — NEVER edit on local `main` when work is intended for a PR.**
> Create a feature branch (`git checkout -b feat/...` or `chore/...`) **before** any file edit — not before commit, **before edit**.
>
> | If `git branch --show-current` returns... | Action |
> |---|---|
> | `main` AND you're about to make a PR-targeted edit | **STOP**. Run `git checkout -b <prefix>/<descriptive-name>` first. Only then edit. |
> | A feature branch | Proceed with edits |
> | `main` AND you've already made commits (recovery) | `git stash` → `git checkout -b <feature>` → `git checkout main && git reset --soft origin/main && git restore --staged .` → push feature branch → open PR. Pop stash on feature branch if needed. |
>
> The first action of any skill/workflow that produces commits is `git branch --show-current`; if `main`, switch **before** any `Edit`/`Write`. Before any `git push`, confirm again — if it is `main`, stop and apply recovery. **`origin` will not stop you** (§ Permissions) — this hook and this rule are the whole enforcement.

- Merge PRs via merge commit (`gh pr merge --merge`); never squash/rebase-merge.
- Run `go build ./...` before commit; run `go mod tidy` and check `git diff go.mod go.sum` whenever dependencies moved.
- Stage explicitly; **never** `git add -A` / `git add .` — the working tree holds gitignored local state and gate logs.
- **Never** `git commit --no-verify` (or any hook-skip flag) — fix the hook.
- **`gh … --body` vs the commit-block hook.** The commit-block hook matches `git[[:space:]]+commit`, so a `gh issue create` / `gh pr create` / `gh pr comment` invocation whose `--body` argument *contains* that substring is falsely blocked — use `--body-file <path>` instead of inlining the body.
- **NEVER** batch a `git commit` / data-dependent `AskUserQuestion` in the same turn as the `Edit`/subagent call producing its inputs; verify with `git diff --cached --stat` first.
- **Before every `git commit` during a PR task**, stage `ai-docs/learnings.md` with the related change — learnings are part of the deliverable and must be visible in the PR diff. **After a push**, a new learning entry gets its own commit.
- **Delegation has FOUR phases, and failures land in the middle two** — fit (charter *and* environment), hand-off (leave the index CLEAN, or your staged work lands in the delegate's commit), while-it-runs (a delegate waiting on a long job is waiting, not stuck), and return (**a return summary is a claim, not a record** — verify every gate/PASS against the durable artefact). Read [`ai-docs/delegation-rules.md`](ai-docs/delegation-rules.md) before any spawn that commits, edits protected files, or runs long.
- **No "too simple" step-skip in `/task`.** Steps 6 / 7 / 10 are MANDATORY; user authorisation is the only bypass.
- **CI-fix commits get self-review too** — `/pr-ci-failed` and `/main-ci-failed` each run it before their push.
- **NEVER** `git reset --hard` — discards uncommitted work. The same hazard applies to `git checkout -- <file>` and `git restore <file>`: both restore the *whole* working-tree file to HEAD, silently dropping every uncommitted edit to it — safe **only** when you mean to discard the file's entire delta, **hazardous** when the file mixes an edit you keep with one you drop. To undo a test/injection edit on such a file, use a cp-backup (`cp f bak; …; cp bak f`) or a scratch file, then re-verify with `git diff --name-only <base>`. (`git checkout <branch>`/`-b` and `git restore --staged` are unaffected.)
- Plan first. Tests before production code (TDD). Lint changed files.
- Any file with ~50+ lines of substantial logic MUST have tests (`_test.go` beside it).
- After generating/moving a markdown file with relative links, trace one link via `realpath` before committing.
- **PR review comment resolution:** resolve only comments fixed by code; objections stay open for the reviewer.

> **AXIOM 2 — Read the PR body via `gh pr view <N>` after EVERY `git push` to a feature branch with an open PR. Unconditional.**
> The READ is mandatory even for a routine typo/format/nit push. The EDIT is conditional — only when the body contradicts the new commits.
>
> | After... | Required action |
> |---|---|
> | `git push` to a feature branch with an open PR | Run `gh pr view <N> --json title,body` immediately. Read the body. |
> | The body still describes the diff accurately | No `gh pr edit` needed — read complete |
> | The body contradicts the new commits (renames, scope drift, AC flips, cited counts) | Run `gh pr edit` to sync |
> | `gh pr create` immediately preceded the push (first push that opened the PR) | **Skip** the read — the body is what you just authored. The rule fires on the **next** push. |

> **AXIOM — Every code-producing commit on a feature branch with an open (or about-to-be-opened) PR must pass a self-review before `git push`.**
> **Carve-out — Step-8 branch pushes are visibility, not presentation.** During `/task` Step 8 the branch is pushed from the first group return onward with **no PR existing yet**; self-review gates **PR creation** (Step 12, after APPROVE), and CI on the PR is the FINAL gate — it confirms the locally-green tree is green in the reference environment, it does not hunt defects the loop should have found.
> Named instances: `/task` Step 10, `/bugfix` Step 6, `/project-review` Step 5. The enumeration is a list of *named* instances, **never** the only covered surfaces — the obligation is a workspace rule, not a per-skill courtesy. Full matrix: [`.claude/agents/self-review.md` § When self-review applies](.claude/agents/self-review.md). An unnamed surface is covered when it **either** ships executable code (a hook body, a script, Go code) **or** changes an instruction-file rule that other surfaces must obey — "no `.go` diff" is never the test.
>
> APPROVE = push. REJECT = fix on the same branch and re-run; after 3 REJECTs in a row, surface and stop without pushing.

> **AXIOM — `ai-docs/deferred/_inbox.jsonl` is written ONLY by `/task` Step 12 and `/triage`.**
> A hand-edit hides rows from the parser and collides with future appends; one malformed line breaks the whole `jq` read. Row shape: [`ai-docs/templates/inbox-row.md`](ai-docs/templates/inbox-row.md).
>
> | If you need to... | Action |
> |---|---|
> | Record a deferred / out-of-scope / open-question item | Let `/task` Step 12 parse it from the finalised spec — never append by hand |
> | Promote, dedupe or move a row | Run `/triage` — `triage-runner` owns every mutation under `ai-docs/deferred/**` |

## Propagation Rule

> **AXIOM — Edits to one instruction file MUST propagate to its sync-group siblings in the SAME PR.**
> The Propagation Rule fires whenever you edit an instruction file. Sister files in the same sync group must receive the corresponding change before the PR is opened.
>
> | If you edit... | You MUST also check / update... |
> |---|---|
> | Any `.claude/agents/*.md` or `.claude/skills/**` file in a declared sync group | Apply the same change to its siblings — the group table lives in [`ai-docs/propagation-groups.md`](ai-docs/propagation-groups.md). |
> | `AGENTS.md` (rule add / exemption) | Run `grep -rni "<changed-keyword>" .claude/ AGENTS.md ai-docs/` and apply the same change to every match. |
> | Any edit that changes a Tool/Subagent/Skill/Hook contract | Update [`ai-docs/claude-tools-hierarchy.md`](ai-docs/claude-tools-hierarchy.md) in the same PR. |
> | `AGENTS.md` § *Learning Log* (boundary rules, entry format, `Kind:` / `Escalated?` semantics) | `.claude/agents/self-improve.md` AND `.claude/agents/learnings-escalation-audit.md` (Learning-Log group) |
| A hook body in `.claude/settings.json` | Re-verify it fires per [`ai-docs/hook-verification.md`](ai-docs/hook-verification.md), and update the rule text in `AGENTS.md` that the hook backs. |
> | Any other instruction file | Run the same grep — the Procedure below catches lingering references. |
>
> Sync groups are declared in this table as their files land; the learning-loop and CI groups arrive with those skills.

**Procedure:**
1. Before closing the edit, `grep -rni "<changed-keyword>" .claude/ AGENTS.md ai-docs/` for any file referencing the same rule/terminology. **`-i` is not optional** — a sweep over prose is case-insensitive or it under-reports. Corollary: **a file you have already edited is not thereby done** — re-grep it whole, after the edit; one file holding both the fix and the surviving falsehood is the likeliest shape, not the least.
2. Apply the same change (or the corresponding enforcement adjustment) in every match.
3. Rule exemptions must propagate to the checklists that enforce the rule.
4. When the change propagates a **factual / policy claim** (a version, a CI-gate status, a "the repo does X" statement) rather than a rule keyword, the step-1 grep set is necessary but not sufficient — also sweep repo-root user-facing docs (`README.md`, `docs/**`) for the same claim. Completeness test: every LIVE doc must agree; history surfaces (`ai-docs/learnings.md`, `ai-docs/plans/done/**`) are left untouched.

Do not refer to a skill as an "agent" or vice versa — the distinction matters for spawning. (`project-review` is a skill; `review-findings` and `self-review` are agents it spawns.)

## Communication

Interpret user phrasing literally and conservatively. When uncertain — ask, don't guess.

- **"Submit / push to PR"** = `git push` the branch to remote so commits appear in the open PR. **NOT** `gh pr merge`. Only merge when the user explicitly says "merge".
- **"wtf?" / "what?" / "huh?"** (or similar surprise/frustration) = the previous action was the opposite of what the user wanted. **Stop immediately**, do not retry, ask what was wrong before doing anything else.
- **IDE files** (`.idea/`, `*.iml`, `.vscode/`, `*.swp`) — never add, remove, modify, stage, or `.gitignore` them unless the user explicitly asks.
- **A verbal acknowledgement is not a fix.** When the user corrects a fact — especially one already written into a file — the correction is a **work item**, not a conversational beat. Reply **and**, in the same turn, `grep` the artefact for the wrong claim and edit it. Tell: any reply containing *"fair"*, *"good point"*, *"you're right"*, *"that closes it"* that is **not accompanied by an `Edit`** to whatever asserts the now-refuted thing.
- **A recorded result is a claim, not a completion.** A sentence asserting your *own* work-state — "verified", "confirmed", "gate PASSed", "done" — written to any durable surface (a PR body, a progress log, a trace field) is a **timestamped claim**, not a standing fact. Re-run the underlying check *after the LAST edit of the turn*, immediately before recording — never record-then-edit. After fixing a claim-class defect, re-scan the **whole section**, not just the fixed line.
- **A citation offered as authority is itself a claim — open it.** Before invoking an in-repo rule, table row, learnings entry, date, or `file:line` as the reason for an action *or an inaction*, resolve it and confirm it says what you are citing it for. Three failure shapes, all observed upstream: a **premise** attached to a correct rule (it propagates further than the rule); a **banded rule** where you quote a neighbouring row's action instead of the row your own measurement falls in — and that error is predictably self-serving, because the half-remembered row is the one that lets you skip work; and a **date or `file:line`** attached to supporting evidence, where a thematically *adjacent* entry makes the misattribution feel checked. Special force when the cited rule lives in the file you are editing. A reviewer's reading of a rule is an argument, not the rule: verify a **permissive** reading harder than a restrictive one.
- **A bound is not a target; a limit that is not a gate is a limit.** "Never weaker" (`>=`) authorises staying put, not movement. Tightening a rule nobody asked to tighten is unapproved scope exactly as loosening one is.
- **A correction propagates to every delegate that received the original — in the same turn.** When the user narrows, reverses, or carves out a prior instruction, the acknowledging turn also `SendMessage`s the correction verbatim to every live delegate that received the original; the reply names the delegates messaged or states `no live recipients`.
- **Deviating from user-approved scope requires an ask, not a notification.** *"I also did X — say the word if you'd rather I revert"* puts the burden of catching scope drift on the user. Ask **before** widening scope, even when the argument is compelling — and especially when the argument comes from a reviewer whose premise you have not run.

## Patterns

### 1. Verify an intermediary's claims — findings, retractions, premises, wave-throughs — as skeptically as each other

*Default to* verifying a reviewer's *retraction*, *salvage suggestion*, and *"leave it / harmless / follow-up"* call with the same command you would run against its original finding. A retraction is an assertion; a proposed fix is a claim that the fix works; a "harmless" ruling is a claim about harm.

**The asymmetry to resist.** A finding feels like a challenge and invites checking, while a withdrawal or a wave-through feels like *relief* and invites acceptance — which is exactly when an unverified claim slips through, because agreeing costs nothing in the moment. *Prefer* overriding a reviewer only in the direction of **more** verification: declining a suggested fix because you tested it and it fails is sound; accepting one because it sounds right is not.

**Not only reviewers — any intermediary.** The same posture applies to a *delegate's* claims. *Default to* verifying a delegate's design-blocking STOP ("this primitive can't satisfy its AC") with a command — compile a reduced repro, read the cited code — before amending a design; it is a real finding, but a finding, not a fact. And *prefer* checking a delegate's "this AC clause is untestable on the fixture I used, so I generalised/skipped it": build the missing coverage rather than accepting a PARTIAL.

Validated in the sibling **graphite-gp** project (`ai-docs/learnings.md`, 2026-07-16 — *treating a reviewer's retractions and suggestions as skeptically as its findings*, `Kind: validation`, escalated there to `AGENTS.md`; this harness was adapted from that project, which is where the pattern was earned). Six `self-review` rounds in which the reviewer withdrew a finding built on an unrun premise, offered a salvage anchor that false-positived on the prose it was written to spare, and waved through a false claim as a "harmless lay gloss" that compiling refuted — every override went toward *more* verification. The delegate half is the same log's 2026-07-23 pair: a design-blocking `code-writer` STOP whose premise verification confirmed, and a Step-9 per-AC sweep that refused a delegate's "untestable judgment call" wave-through and built the missing fixture instead.

### 2. A green instrument is a claim about the instrument until you have seen it go red

*Default to* treating a clean result from any verification apparatus — a control, a negative test, a baseline, a guard suite, a set intersection — as evidence about the **apparatus** first and the **subject** second. A genuinely absent effect and an instrument that cannot detect one look identical from the result alone, and only one of them is worth acting on.

**Three shapes, each cheap to rule out.** *Prefer* running a changed guard's own fixtures against the **pre-change** artefact (`git show HEAD:<file>`) — the set of rows that flip, and only that set, is what distinguishes a load-bearing edit from a tautological test. *Prefer* reading the **cardinality of every input** before the verdict of any check shaped as *intersect two sets* / *diff against a baseline* / *grep a corpus* — an empty right-hand side makes `comm -12`, `grep -f` and `diff` report the clean answer for every possible left-hand side. And where a control comes back clean across **every** variation tried, *default to* spending the next step on the channel rather than the conclusion: look for a specific in the output that could only have come from the channel you meant to close — a date, a count, or a proper noun the subject had no other way to know is the cheapest such probe.

**Where the check is written down, make it structural.** *Prefer* giving a recipe an explicit `inconclusive` outcome for the empty-corpus case, so a later reader cannot record a pass the instrument never earned. And a checklist naming a `§`-anchor or an AXIOM as its authority is making a citation — resolve it (§ *Communication*); both of one checklist's were fabricated at import, surviving because nothing downstream had ever needed them to be real.

Validated in this project (`ai-docs/learnings.md`, 2026-08-30 and 2026-08-31 ×2, all `Kind: validation`): a piped-gate guard whose pre-edit re-run flipped exactly the eleven expected rows; four `/improve` eval baselines whose uniform GREEN was traced to the pre-change tree instructing every baseline agent to read the source correction — caught by a date in a returned answer that the cited command does not print; and an `/ai-audit` Checklist O whose `comm -12` had always been empty because its extractor had never matched anything.

## Agent Docs

Read on nearly every task:

| Path | Purpose |
|------|---------|
| [`ai-docs/context.md`](ai-docs/context.md) | Project orientation |
| [`ai-docs/domain-invariants.md`](ai-docs/domain-invariants.md) | Ledger, telemetry, scheduler and Telegram-safety invariants |
| [`ai-docs/code-style.md`](ai-docs/code-style.md) | Go code-style reference |
| [`ai-docs/go-test-conventions.md`](ai-docs/go-test-conventions.md) | Table tests, `-race`, golden logs, Postgres fixtures |
| [`ai-docs/learnings.md`](ai-docs/learnings.md) | Corrections log — feed for `/improve` |

**Every other page — key decisions, naming, doc convention, dependency recipes, delegation, hook verification, writing style, tool inventory, propagation groups, templates, plans, telemetry schema — is indexed in [`ai-docs/agent-docs-index.md`](ai-docs/agent-docs-index.md).**

## Learning Log

On **ANY** instruction violation, write a new entry to `ai-docs/learnings.md` — there is no "obvious", "minor", "trivial", "already-known", or "duplicate" disposition. The history (including recurrences and superseded entries) is the artefact `/improve` audits to decide escalation fan-out. See [`ai-docs/corrections-log.md`](ai-docs/corrections-log.md) for the enumerated skip-reasons that are explicitly disallowed. **Read the two boundary rules below before you write.**

**Two logs, one genre each.** `ai-docs/learnings.md` holds **conduct corrections and validations**. A harness diagnosis (a gap or defect in an instruction file, addressed to `/improve`) goes to [`ai-docs/harness-gaps.md`](ai-docs/harness-gaps.md) — same skeleton plus a required `target:` field. An entry about **another entry** belongs in neither: use the original's `Superseded by:` field (Boundary rule 1's exception). `/improve` reads both. Misfiled genre = violation.

### Boundary rule 1 — `ai-docs/learnings.md` is APPEND-ONLY

> **NEVER** edit, rewrite, reorder, summarise, or delete an existing entry in `ai-docs/learnings.md`. Only append new entries at the end. This applies even when:
> - a newer correction supersedes an older one — write a NEW entry that says so, leave the old one intact
> - an entry turns out to be wrong, redundant, or poorly worded — write a NEW entry that corrects it
> - you are tempted to "tidy up" or "consolidate" the file
>
> The history of corrections (including superseded and wrong ones) is itself the artefact `/improve` audits. Editing past entries destroys that history.
>
> **Exception — `Escalated?` and `Superseded by:` fields, subagent-driven only.** Both fields MAY be updated in-place by the `self-improve` subagent (`/improve`) and the `learnings-escalation-audit` subagent (`/ai-audit` Phase 1). All other lines of an entry remain immutable.

### Boundary rule 2 — writing to `learnings.md` triggers NO other rule-file edits in the same turn

> When you write to `ai-docs/learnings.md`, you **MUST NOT** also edit `AGENTS.md`, `CLAUDE.md`, `.claude/**`, `ai-docs/code-style.md`, or `ai-docs/doc-convention.md` in the same conversation turn.
>
> Writing a learning entry is **NOT** authorisation to escalate the rule into instruction files. Set `Escalated? no` and stop. Project-level escalation happens only when the user runs `/improve`, or explicitly asks ("escalate this", "add to AGENTS.md").
>
> **Carve-out:** appending to `ai-docs/harness-gaps.md` is NOT an instruction-file edit — it is the designated parking surface for harness diagnoses. Appending to both logs in one turn is legal; editing an instruction file in that turn is still not.
>
> **Machine-enforced while an interview is live.** A `PreToolUse` hook (`.claude/settings.json`, suite `ai-docs/scripts/test-instruction-edit-guard.sh`) blocks any `Edit`/`Write` to the files this rule names, and any `Bash` command that writes into one, while an `ai-docs/plans/*.spec.md.state.md` exists on the branch and `ai-docs/plans/.task-inflight` does not — i.e. between round 1 of `/interview` and `/task` Step 8. The owner's *"the instruction should say X"* is a diagnosis for `harness-gaps.md`; it was read as a licence once (`ai-docs/learnings.md` 2026-09-08), and the hook is what stops the second time. Steps 8–12 carry the marker and are exempt; `/improve` runs on a branch with no state file and is untouched.
>
> **Two narrow exceptions** — `/improve` + `/ai-audit` updating `Escalated?` / `Superseded by:` on **existing** entries, and in-flow capture of an in-task insight during `/task` Steps 8–12. Conditions and rationale: [`ai-docs/corrections-log.md`](ai-docs/corrections-log.md).

### Entry format

**Copyable skeleton + a filled example: [`ai-docs/templates/learnings-entry.md`](ai-docs/templates/learnings-entry.md) — consult that template to inspect the format, NOT the live log.** For orientation, an entry is a `### YYYY-MM-DD — [category] — [short description]` heading followed by `**What happened:**`, `**Rule:**`, optional `**Kind:**`, `**Escalated?**`, and optional `**Superseded by:**`.

`Kind:` defaults to `correction` (a violation to stop doing); write `Kind: validation` for a working protocol to keep doing. `Escalated?` records **project-level** persistence only — user-local auto-memory and `settings.local.json` do **not** count → stay `no`.

Categories: `code-style` | `process` | `architecture` | `testing` | `documentation` | `tooling` | `search` | `other`

Run `/improve` when **≥3 unescalated correction entries**, **≥2 unescalated validation entries**, or a stale-validation flag from `/ai-audit` accumulates.

## Go Test Conventions

- **Table-driven subtests** are the default shape: a `[]struct{name string; …}` slice, `t.Run(tc.name, …)`, `t.Parallel()` where the test is independent. Test names describe behaviour: `returns_error_when_stamina_exhausted`.
- **`go test -race ./...` is a required gate for any change touching goroutines, the scheduler, or shared state.** A race is a defect, never a flake.
- **No `panic` / `log.Fatal` in production code.** A `PostToolUse` hook flags them on write; every surviving instance is justified in a doc comment **and** recorded in [`ai-docs/panic-index.md`](ai-docs/panic-index.md). The comment states the justification itself and does not point at the index — a comment naming a markdown path is what the reference ban forbids, and the index is found by name, not by a pointer from the code. `main` may exit non-zero; libraries return errors.
- **Determinism is testable, so test it exactly.** Generation, combat, and trail replays take an explicit seed: assert exact outputs, and keep the golden log of a combat in the repository (`combat()` is a pure function by design — `docs/DESIGN.md` §4 — so a snapshot test is free and the freedom to rewrite combat depends on it).
- **Postgres is tested against Postgres**, not a mock: the ledger's invariants (zero-sum per kind, the `CHECK` constraints, the capture order under concurrency) are database behaviour. Concurrency stress tests run under `-race`.
- **Assert on behaviour, transitions, errors, and edge cases** — for the raid FSM that means every edge, including the timer edges whose guard fails (a stale task firing late is expected traffic, `docs/DESIGN.md` §3.5).
- A search miss on a construct that SHOULD exist is a **search-method failure first** ([`.claude/rules/ast-index.md`](.claude/rules/ast-index.md) → *Negative results are NOT evidence*).

Detail and worked examples: [`ai-docs/go-test-conventions.md`](ai-docs/go-test-conventions.md).
