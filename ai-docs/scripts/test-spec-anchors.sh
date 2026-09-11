#!/usr/bin/env bash
# Regression suite for check-spec-anchors.sh.
#
# The interview state below carries, verbatim, part of the issue body and the
# first-round answers of the 2026-09-10 run. The must-hit rows are that run's
# own: the acceptance rows the owner struck as restated rules or prescribed
# mechanisms, the decisions row and the scope item the same mechanism survived
# in, and anchors that quote something the state file holds but that is not
# the owner's words -- a label, a linked number, a question. The must-pass
# rows are the rows the owner kept, anchored the way the charter requires.
# Both directions are asserted: a guard that refuses everything satisfies the
# first half alone.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-spec-anchors.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
guard="$repo_root/ai-docs/scripts/check-spec-anchors.sh"
[ -f "$guard" ] || { echo "FAIL: $guard missing"; exit 1; }

scratch=$(mktemp -d "${TMPDIR:-/tmp}/spec-anchors.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
failures=0
fail() { printf 'FAIL: %s\n' "$*"; failures=$((failures + 1)); }

# The live interview state: the persisted issue, then four answers of round 1.
live_state() {
  cat <<'STATE'
# Interview state — cmd/bot composition root

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-10-cmd-bot-composition-root.spec.md
issue_ref: "#24"
gh_issue:
  title: "cmd/bot composition root: wiring, migration policy, graceful shutdown"
  state: open
  labels: ["mvp", "area:platform"]
  body: |
    ## Scope

    - Start-up sequence with an explicit order and a failure mode per step, each naming what failed.
    - The migration policy decision, implemented and written down.
    - Graceful shutdown on `SIGINT` / `SIGTERM`: stop polling, let in-flight updates and the claimed scheduler batch finish inside a bounded deadline, close the pool.
    - One place where every subsystem is constructed and injected — no package-level singletons, no `init()` wiring.
    - A health/readiness signal distinct from `/metrics` (readiness = migrations applied and the pool answers).

    ## Telemetry obligation

    - Metrics: process start timestamp, build version, readiness state.
    - Events: none.

  comments: []
  linked_issues: ["#18", "#19", "#20", "#22", "#23", "#47"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 2
agent_id: null
prior_qa:
  - round: 1
    question: "Production migration-apply policy: where do pending migrations get applied? (goose applies no lock unless one is configured, so the concurrent-start answer follows from this choice.)"
    answer: "Start-up, opt-out — Applied at start-up by default, with an environment switch turning it off for an operator who runs the separate command instead. Both paths exist and both need covering."
  - round: 1
    question: "DESIGN 12.1 restart hygiene — shifting overdue scheduler run_at forward by the downtime. Does that land in this task or its own issue? No issue in the #47 decomposition owns it today."
    answer: "why we need to shift run_at?"
  - round: 1
    question: "(orchestrator answered from docs/DESIGN.md §12.1 — the rule protects backpack/corpse TTL and raid-session timer edges from firing in one salvo after a home-machine outage — then re-put the same question verbatim)"
    answer: "In this task — The composition root records a liveness heartbeat and, at start-up, shifts overdue pending scheduled_task rows forward by the measured downtime. Grows a forward migration and a start-up step."
  - round: 1
    question: "A subsystem that fails to construct or bind at start-up — fatal, or does the bot come up degraded? (The example that already exists: .env.example's placeholder cloud-canary token refuses the whole canary, own leg included.)"
    answer: "All fatal — Any subsystem failing stops the process, naming it. One rule, no degraded modes — a placeholder canary token keeps the bot down until it is fixed."
```
STATE
}

live="$scratch/live"; mkdir -p "$live"
live_state > "$live/one.spec.md.state.md"

# check <HIT|PASS> <ac|kd|scope> <one row or item>
check() {
  local want=$1 section=$2 row=$3 f="$live/one.spec.md" rc got
  case "$section" in
    ac)    printf '# t\n\n## Acceptance Criteria\n\n| # | Criterion |\n|---|---|\n%s\n' "$row" > "$f" ;;
    kd)    printf '# t\n\n## Key decisions\n\n| Question | Decision |\n|---|---|\n%s\n' "$row" > "$f" ;;
    scope) printf '# t\n\n## Scope\n\n%s\n' "$row" > "$f" ;;
  esac
  bash "$guard" "$f" >/dev/null 2>&1; rc=$?
  if [ "$rc" -eq 1 ]; then got=HIT; else got=PASS; fi
  [ "$got" = "$want" ] || fail "expected $want, got $got (exit $rc), $section: $row"
}

while IFS=$'\t' read -r want section row; do
  case "${want:-}" in ''|'#'*) continue ;; esac
  check "$want" "$section" "$row"
done <<'FIXTURES'
# --- must hit: rows the owner struck at round 2, verbatim, which quote nothing ---
HIT	ac	| AC5 | A single `*slog.Logger` is constructed in the composition root and passed to every subsystem that accepts one, the migration call included; no production file under `cmd/` or `internal/` obtains a logger from a package-level variable or from `slog.Default`. |
HIT	ac	| AC43 | Every exported item added by this task carries a doc comment beginning with its name, and every new package carries a package comment. |
HIT	ac	| AC48 | Module tidiness leaves `go.mod` and `go.sum` unchanged after the change. |
HIT	ac	| AC49 | Every live site in the repository whose claim this diff falsifies is updated in the same PR, per AGENTS.md § *Propagation Rule* step 4. |
HIT	kd	| Process logger | One `*slog.Logger` constructed in the composition root and passed explicitly to every subsystem that takes one, including `store.Migrate`, which rejects a nil logger. Nothing calls `slog.SetDefault`; no package holds a package-level logger. Handler shape is the design's call. |
HIT	scope	12. **Propagation.** This task changes a gate set, a start-up contract and a documented claim about running the binary; every live site in the repository whose claim the diff falsifies is updated in the same PR, per AGENTS.md § *Propagation Rule* step 4.
# --- must hit: an anchor that quotes something the owner never said ---
HIT	ac	| AC5 | One logger reaches every subsystem. [task: "passed to every subsystem that accepts one"] |
HIT	ac	| AC5 | One logger reaches every subsystem. [task: "area:platform"] |
HIT	ac	| AC5 | One logger reaches every subsystem. [task: "#47"] |
HIT	ac	| AC7 | Migrations apply at start-up. [answer 1.1: "goose applies no lock unless one is configured"] |
HIT	ac	| AC7 | Migrations apply at start-up. [task: "Applied at start-up by default"] |
HIT	ac	| AC22 | Start-up failure is fatal. [answer 1.1: "Any subsystem failing stops the process"] |
HIT	ac	| AC22 | Start-up failure is fatal. [answer 1.5: "Any subsystem failing stops the process"] |
HIT	ac	| AC22 | Start-up failure is fatal. [answer 2.1: "Any subsystem failing stops the process"] |
HIT	ac	| AC22 | Start-up failure is fatal. [answer 1.0: "Any subsystem failing stops the process"] |
HIT	ac	| AC22 | Start-up failure is fatal. [answer: 1.4] |
HIT	ac	| AC22 | Start-up failure is fatal. [task: ""] |
HIT	ac	| AC22 | Start-up failure is fatal. [answer 1.4: "All fatal"] [task: "every subsystem is fatal"] |
FIXTURES

while IFS=$'\t' read -r want section row; do
  case "${want:-}" in ''|'#'*) continue ;; esac
  check "$want" "$section" "$row"
done <<'FIXTURES'
# --- must pass: rows the owner kept, anchored ---
PASS	ac	| AC7 | Pending migrations are applied during start-up before readiness can report ready and before the update loop or the scheduler worker starts. [answer 1.1: "Applied at start-up by default"] |
PASS	ac	| AC16 | At start-up the process moves every overdue pending scheduler row forward by the measured downtime. [answer 1.3: "shifts overdue pending scheduled_task rows forward by the measured downtime"] |
PASS	ac	| AC22 | A subsystem that fails to construct, bind or start stops the process with a message naming that subsystem. [answer 1.4: "Any subsystem failing stops the process, naming it"] |
PASS	kd	| Start-up failure policy | **Every failure is fatal.** A subsystem that cannot be constructed, bound or started stops the process, naming it. [answer 1.4: "One rule, no degraded modes"] |
PASS	scope	1. **One composition root.** A single place assembles every subsystem and injects it explicitly. [task: "One place where every subsystem is constructed and injected"]
PASS	scope	4. **Readiness**, distinct from the metrics endpoint. [task: "A health/readiness signal distinct from `/metrics`"]
PASS	ac	| AC1 | The bot is assembled in one place. [task: "composition root: wiring, migration policy"] |
# --- must pass: whitespace is normalised, on both sides, and nothing else is ---
PASS	ac	| AC23 | The registry carries the process start instant. [task: "Telemetry obligation - Metrics: process start timestamp"] |
PASS	ac	| AC3 | The migration policy is written down. [task: "The migration policy    decision"] |
PASS	ac	| AC22 | Start-up failure is fatal. [answer 1.4: "All fatal"] [task: "One place where every subsystem is constructed"] |
# --- must pass: an open question, and rows the guard does not own ---
PASS	kd	| Shutdown budget shape | TBD — asked in round 2 |
PASS	ac	| AC9 | **TBD** — waits on the round-2 answer |
FIXTURES

spec="$live/one.spec.md"

# --- an answer id past the recorded answers says so, not only "unresolved" ---
printf '%s\n' '# t' '' '## Acceptance Criteria' '' '| # | Criterion |' '|---|---|' \
  '| AC22 | Start-up failure is fatal. [answer 1.5: "All fatal"] |' > "$spec"
out=$(bash "$guard" "$spec" 2>&1)
grep -q 'no such answer' <<< "$out" || fail "an answer id past the recorded answers was not named as missing"

# --- only the issue's words are task text, whatever layout the other fields take ---
live_state | sed 's/^  linked_prs: \[\]$/  linked_prs:\n    - "#100"/' > "$live/one.spec.md.state.md"
grep -q '^    - "#100"$' "$live/one.spec.md.state.md" || fail "instrument: the block-list fixture did not take"
printf '%s\n' '# t' '' '## Acceptance Criteria' '' '| # | Criterion |' '|---|---|' \
  '| AC1 | x [task: "#100"] |' > "$spec"
bash "$guard" "$spec" >/dev/null 2>&1 && fail "a linked number in block-list form was read as task text"
live_state > "$live/one.spec.md.state.md"

# --- a multi-line scope item: the anchor on a continuation line counts ---
cat > "$spec" <<'SPEC'
# t

## Scope

2. **Graceful shutdown on `SIGINT` and `SIGTERM`**, bounded by a single whole-shutdown
   deadline. [task: "Graceful shutdown on `SIGINT` / `SIGTERM`: stop polling"]

   A second paragraph of the same item, indented, carries no anchor of its own.
3. **A migrate-only entry point.** It applies pending migrations and exits.
SPEC
out=$(bash "$guard" "$spec" 2>&1); rc=$?
[ "$rc" -eq 1 ] || fail "an unanchored item after an anchored multi-line one should hit (exit $rc)"
grep -q 'Scope item "2\.' <<< "$out" && fail "the anchored multi-line item was judged without its continuation line"
grep -q 'Scope item "3\.' <<< "$out" || fail "the unanchored item was not the one reported"

# --- a table header is the row before its separator, whatever it says ---
printf '%s\n' '# t' '' '## Key decisions' '' '| Asked | Settled |' '|:---|---:|' \
  '| Start-up failure policy | Every failure is fatal. [answer 1.4: "All fatal"] |' > "$spec"
bash "$guard" "$spec" >/dev/null 2>&1 || fail "a header row with its own titles was judged as a decision"

# --- sections the guard does not own, and fenced code, are skipped ---
printf '%s\n' '# t' '' '## Out of scope' '' '- Container images — the infrastructure pass.' '' \
  '## Deferred' '' '| what | why | separate issue needed? |' '|---|---|---|' '| goleak | a leak detector | yes |' '' \
  '## Acceptance Criteria' '' '```' '| AC1 | an example row, no anchor |' '```' > "$spec"
bash "$guard" "$spec" >/dev/null 2>&1 || fail "out-of-scope, deferred or fenced rows were judged"

# --- a superseded issue body is history: no task anchor resolves against it ---
superseded_state() {
  live_state | sed 's/issue_body_status: current/issue_body_status: superseded/'
}
superseded_state > "$live/one.spec.md.state.md"
printf '%s\n' '# t' '' '## Acceptance Criteria' '' '| # | Criterion |' '|---|---|' \
  '| AC3 | The policy is written down. [task: "The migration policy decision"] |' > "$spec"
out=$(bash "$guard" "$spec" 2>&1); rc=$?
if [ "$rc" -ne 1 ] || ! grep -q 'no-task-text' <<< "$out"; then
  fail "a task anchor resolved against a superseded body with no task text (exit $rc)"
fi

# ... and the task text the interview persisted beside it is the source.
{ superseded_state | sed '/^round_cap:/,$d'
  printf '%s\n' 'task_description: |' '  Wire the bot: one place builds every subsystem, and a signal drains it.' \
    'round_cap: 4' 'prior_qa: []' '```'
} > "$live/one.spec.md.state.md"
printf '%s\n' '# t' '' '## Acceptance Criteria' '' '| # | Criterion |' '|---|---|' \
  '| AC1 | A signal drains the bot. [task: "a signal drains it"] |' > "$spec"
bash "$guard" "$spec" >/dev/null 2>&1 || fail "a task anchor did not resolve against the persisted task text"
printf '%s\n' '# t' '' '## Acceptance Criteria' '' '| # | Criterion |' '|---|---|' \
  '| AC3 | The policy is written down. [task: "The migration policy decision"] |' > "$spec"
bash "$guard" "$spec" >/dev/null 2>&1 && fail "a task anchor resolved against the superseded body instead of the task text"

# --- free-text mode, and a pipe written the way a table cell must write it ---
printf '%s\n' '# Interview state — t' '' '```yaml' 'schema_version: 1' 'task_description: |' \
  '  Split the log into learnings | gaps views.' 'round: 1' 'prior_qa: []' '```' > "$live/one.spec.md.state.md"
printf '%s\n' '# t' '' '## Acceptance Criteria' '' '| # | Criterion |' '|---|---|' \
  '| AC1 | The log has two views. [task: "learnings \| gaps views"] |' > "$spec"
bash "$guard" "$spec" >/dev/null 2>&1 || fail "an escaped pipe in a task fragment did not resolve"

# --- no state file on disk: presence only, and it says so ---
rm -f "$live/one.spec.md.state.md"
printf '%s\n' '# t' '' '## Acceptance Criteria' '' '| # | Criterion |' '|---|---|' \
  '| AC1 | x [task: "a fragment nobody can check here"] |' > "$spec"
out=$(bash "$guard" "$spec" 2>&1); rc=$?
[ "$rc" -eq 0 ] || fail "with no state file an anchored row should pass on presence (exit $rc)"
grep -q 'presence only' <<< "$out" || fail "with no state file the skipped resolution was not announced"
printf '%s\n' '# t' '' '## Acceptance Criteria' '' '| # | Criterion |' '|---|---|' '| AC1 | x |' > "$spec"
bash "$guard" "$spec" >/dev/null 2>&1 && fail "with no state file an unanchored row passed"

# --- default mode: only what this branch added or changed is judged ---
repo="$scratch/repo"; mkdir -p "$repo"
git -C "$repo" init -q -b main . && git -C "$repo" config user.email t@t && git -C "$repo" config user.name t
mkdir -p "$repo/ai-docs/plans/done" "$repo/ai-docs/plans/ignored"
cat > "$repo/ai-docs/plans/done/old.spec.md" <<'OLD'
# old

## Scope

1. **An old item** that predates anchors and wraps onto
   a second line.

## Out of scope

- Something another issue owns.

## Acceptance Criteria

| # | Criterion |
|---|---|
| AC1 | An old criterion with no anchor. |
OLD
git -C "$repo" add -A && git -C "$repo" commit -q -m base
git -C "$repo" checkout -q -b feat
cp "$guard" "$repo/guard.sh"
run_default() { (cd "$repo" && bash guard.sh) >/dev/null 2>&1; }

printf '| AC2 | A new criterion. [task: "anything, checked for presence here"] |\n' >> "$repo/ai-docs/plans/done/old.spec.md"
git -C "$repo" commit -q -am anchored
run_default || fail "default mode judged a legacy row nobody touched"

printf '| AC3 | A new criterion with no anchor. |\n' >> "$repo/ai-docs/plans/done/old.spec.md"
git -C "$repo" commit -q -am unanchored
run_default && fail "default mode missed an unanchored row this branch added"

git -C "$repo" reset -q --hard HEAD~2
sed -i 's/a second line\./a second, reworded line./' "$repo/ai-docs/plans/done/old.spec.md"
git -C "$repo" commit -q -am touch-continuation
run_default && fail "an item this branch edited on its continuation line was not judged"

git -C "$repo" reset -q --hard HEAD~1
sed -i 's/Something another issue owns\./Something another issue now owns./' "$repo/ai-docs/plans/done/old.spec.md"
git -C "$repo" commit -q -am touch-out-of-scope
run_default || fail "an out-of-scope item was judged"

# --- a retired state file is found and resolves ---
git -C "$repo" reset -q --hard HEAD~1
{ live_state | sed 's#^spec_path: .*#spec_path: ai-docs/plans/old.spec.md#'; } > "$repo/ai-docs/plans/ignored/old.spec.md.state.md"
printf '| AC2 | A new criterion. [task: "a fragment the retired state does not hold"] |\n' >> "$repo/ai-docs/plans/done/old.spec.md"
git -C "$repo" add ai-docs/plans/done/old.spec.md && git -C "$repo" commit -q -m retired
run_default && fail "the retired state file was not consulted"

if [ "$failures" -gt 0 ]; then
  printf '%s regression(s) in check-spec-anchors.sh\n' "$failures"
  exit 1
fi
printf 'spec-anchor guard: all fixtures behave as specified\n'
exit 0
