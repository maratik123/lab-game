---
name: interview
description: "Requirements interview with the product owner. Output: spec saved to ai-docs/plans/, cross-linked with a tracking GitHub issue. Invoked by `/task` for Steps 1–5, or run standalone for spec-only work that defers implementation."
argument-hint: "[issue-number | task description]"
allowed-tools: Bash(gh issue view *) Bash(gh issue list *) Bash(gh issue create *) Bash(gh issue comment *)
---

Orchestrator for the spec-drafting interview. Drives the round loop, surfaces the subagent's questions to the user, and applies the user's answers — but does **not** draft the spec itself. Spec drafting and question generation live in `.claude/agents/spec-writer.md` (subagent on `model: inherit`).

> **MUST run before:** code investigation, `design-writer` Subagent, or writing code.
> Run standalone when you want a spec without committing to implementation (defer it to `ai-docs/plans/deferred/` afterward).
> For the full task workflow use `/task` — it delegates Steps 1–5 to this skill, then continues with design → implementation → PR.

> **⚡ Compaction recovery check — read FIRST on every invocation.**
> If you are re-entering this skill after auto-compaction (a
> summary/compaction block appears at the top of context, or workflow
> context feels thin), STOP before any tool call and:
>
> 1. **Locate the durable-state file** — list `ls ai-docs/plans/*.spec.md.state.md 2>/dev/null` (then read both the matched `.state.md` AND its sibling `<spec_path>`).
>    If exactly one in-flight artefact exists, that's the durable state.
>    If none exists, this is a fresh invocation. (Multiple matches:
>    surface to the user before continuing.)
> 2. Read it **top-to-bottom in one pass** — every line, including older
>    sections. Do not skim. The recorded `round` (from the `.state.md` YAML block) is a
>    cross-check, never an instruction to skip the read.
> 3. **Then re-enter this skill from the top of its body.** The body's
>    re-entry logic uses `round` (after the full read) to skip
>    user-confirmed checkpoints that need not be redone — resume from the round recorded in `.state.md`'s `round:` field; do NOT restart at round 1, and do NOT re-create the state file.
>
> If `ls ai-docs/plans/*.spec.md.state.md 2>/dev/null` returns no matches, this is a fresh invocation —
> proceed normally.
>
> See `.claude/skills/context-reset/SKILL.md` § **Compaction recovery
> (re-entry)** for the canonical handoff rationale.

## Architecture

Two pieces:

1. **This file** (orchestrator) — plumbing only. Detects entry mode, manages state, runs the round loop, parses the subagent's YAML status block, surfaces questions via `AskUserQuestion`, executes action handlers on `unresolvable`, posts the cross-link comment on `ready`.
2. **`.claude/agents/spec-writer.md`** (subagent, `model: inherit`) — owns scope extraction, question drafting, AGENTS.md preflight, the Rule-5 substring blacklist, the optimization-target enforcement, and the spec write itself.

## Round / question caps

| Constant | Value | Where |
|---|---|---|
| `round_cap` | 4 | Hard-coded in this skill; passed to subagent every invocation |
| `questions_per_round_cap` | 3 | Hard-coded in this skill; passed to subagent every invocation |

These constants are **not** configurable via skill arguments in this iteration. Future configurability is a separate concern (deferred — see the sibling quartzite project's `maratik123/quartzite#188` spec).

## State file

Path: `<spec_path>.state.md` — e.g. `ai-docs/plans/2026-05-09-name.spec.md` ↔ `ai-docs/plans/2026-05-09-name.spec.md.state.md`.

Created at the start of round 1 and **committed with the spec after every delegate return — Step 3b item 0 is the command, and 3a refuses to spawn round ≥ 2 while the spec is untracked.** This file and `<spec_path>` are the only record of an interview that runs for hours. Both were untracked for their whole life until a delegate's truncating edit proved that unrecoverable (`ai-docs/learnings.md` 2026-09-02); the sentence you are reading then stated the spec was tracked, and a second truncation (2026-09-08) landed on a spec three rounds untracked, because a stated fact is read as background and only a step with a command is read as a step.

**Lifecycle by exit:**

| Exit | State file |
|---|---|
| `ready` | **KEPT.** A later step re-enters `spec-writer` — a Step 7 GO-with-notes spec amendment, a Step 9 finding, a Step 11 amendment trigger, a `/pr-commented` round — and this file is what makes that a continuation instead of a cold start: it carries the round counter and the original `gh_issue:` / `task_description:` inputs. `/task` Step 12 `mv`s it to `ai-docs/plans/ignored/` before the PR. |
| `abort` | Deleted with the spec (`git rm` both, commit) — there is nothing to return to. |
| `defer_to_deferred` | Deleted; the parked spec is re-entered through `⚡ Second`, which starts its own interview. |

Format: markdown header + a single fenced YAML block.

```markdown
# Interview state — <task name>

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/YYYY-MM-DD-name.spec.md
issue_ref: "#188"
gh_issue:                          # present only when issue_ref resolves to a real gh issue; omitted for free-text entry mode
  title: "<verbatim from gh issue view>"
  state: open                      # open | closed
  labels: ["enhancement"]          # may be empty
  body: |
    <verbatim issue body block-scalar>
  comments:
    - author: "..."
      body: "..."
  linked_issues: ["#<N>"]           # extracted from body / comments via #\d+ regex; may be empty — spec-writer READS each one (gh issue view) before round 1
  issue_body_status: current         # or `superseded` — Step 1's answer when TASK differs from the persisted body
  linked_prs: ["#100"]             # same; may be empty
task_description: |                # free-text entry mode; or beside gh_issue: when issue_body_status is superseded (Step 1)
  <user's free-text task description, or the TASK section verbatim>
round_cap: 4
questions_per_round_cap: 3
round: 2
agent_id: <captured from round-1 Agent invocation; null if cold-Agent path>
prior_qa:
  - round: 1
    question: "..."
    answer: "..."
  - round: 1
    question: "..."
    answer: "..."
```
```

The `gh_issue:` / `task_description:` blocks are the durable home for the spec-writer's original inputs. The round prompts (Step 3a) carry the same content inline today, but persistence to `.state.md` survives auto-compaction and cold re-spawns where the round prompt has been dropped — the orchestrator can rebuild the prompt from `.state.md` on re-entry without re-issuing `gh issue view`.

**This file is also what every spec row's anchor resolves against** (`spec-writer.md` Rule 11): `[task: "…"]` against the task text — the `task_description:` block when there is one, else the issue's title, body and comments — and `[answer <round>.<n>: "…"]` against the n-th `prior_qa` entry of that round. So `prior_qa` is **append-only**: an entry is never edited, reordered or removed, and an answer is the owner's words verbatim, never a summary of them.

## Workflow

### Step 1: Detect entry mode

Inspect `$ARGUMENTS`. **First, apply the hand-off contract** (defined at the `/task` call site, `.claude/skills/task/SKILL.md` Steps 1–5):

- **Bare text** (no `## TASK` / `## RECON` / `## DELTA` headers) — the whole of `$ARGUMENTS` is the task description. Always legal.
- **Sectioned hand-off** — `## TASK (verbatim)` is the task description; `## RECON (unverified claims)` is carried into the spec-writer prompt as the `recon` field, verbatim, and is NEVER merged into `issue_body`; `## DELTA` lines are constraints the ORCHESTRATOR added — surface them to the user for confirmation in round 1 (they are not the user's words until confirmed).
- **Malformed hand-off** — sectioned but missing `NOT READ:` inside RECON, missing the DELTA section, carrying instructions or applicability verdicts inside RECON, or restating TASK in changed words: **STOP. Return it to the caller naming the violated clause; do not start the round loop.** A malformed hand-off processed anyway is how a compression step's inventions become the task.

Then detect entry mode:

- **Issue ref** — matches `^#?\d+$`: load `gh issue view <N> --json title,body,state,labels,comments` once. Record `tracking_issue = <N>`. **When the hand-off is sectioned and its `## TASK (verbatim)` is not the issue body** (the owner reframed the task in conversation — the shape of 2026-09-08, where the persisted body prescribed the opposite of TASK and the delegate spent its `## Source conflicts` on a source the spec itself declared void), ask before anything else — `AskUserQuestion`, single-select: **Update the issue now** (`gh issue edit <N> --body-file <path>` with TASK, recommended) / **TASK supersedes the body** / **Stop**. Record the answer as `issue_body_status: current | superseded` in the state file's `gh_issue:` block; `spec-writer` reads it (§ *Read before drafting* item 2 there) and treats a superseded body as history, not as a source. **On `superseded`, also write TASK verbatim into the state file as a `task_description:` block** — the one state file that carries both blocks. Without it the task text lives only in a round prompt, which compaction drops, and no spec anchor can resolve against it. Extract `#\d+` references from the body + comments → `linked_issues` / `linked_prs` (split on whether the referenced number is an issue or a PR; cheap heuristic — running `gh pr view <M>` once per match is acceptable, or treating ambiguous refs as `linked_issues` is acceptable until a downstream consumer needs the precise split).
- **Free text / empty**: use as task description, or ask "What do you want to plan?" if empty. `tracking_issue` is unset until Step 5.

### Step 2: Compute paths and seed state

1. Derive a kebab-case spec slug from the issue title (or task description), ≤ 5 words.
2. `spec_path = ai-docs/plans/<TODAY>-<slug>.spec.md`
3. `state_path = <spec_path>.state.md`
3a. **Create the feature branch now — the first commit of this flow happens in this skill, not in `/task` Step 8.** Run `git branch --show-current`; if it is `main`, `git checkout -b <prefix>/<TODAY>-<slug>` using the same date-slug as `spec_path` (AGENTS.md § Workflow AXIOM 1 — the branch exists before the first edit, and from here on there are edits to commit). `/task` Step 8 finds the branch already created and verifies it instead of creating it.
4. Write the initial state file, then `git add ai-docs/plans/<TODAY>-<slug>.spec.md.state.md` and commit it (no `-f` needed — the state path matches no ignore rule). Contents: `round: 1`, `prior_qa: []`, `agent_id: null`, **plus the Step 1 payload**: issue-ref mode emits a `gh_issue:` block populated with `title` / `state` / `labels` / `body` / `comments` / `linked_issues` / `linked_prs`; free-text mode emits a `task_description:` block carrying the user's description verbatim. Exactly one block is present per state file, with the single exception Step 1 makes for a superseded issue body, which adds `task_description:` beside `gh_issue:`.

### Step 3: Round loop

For each round (1..=`round_cap`):

#### 3a. Invoke the subagent

**Round 1 — cold spawn (primary path):**

```
Agent(
  subagent_type="spec-writer",
  prompt="""
    Read .claude/agents/spec-writer.md and follow it.

    issue_ref: <#N | "free-text">
    issue_body: |
      <verbatim from gh issue view, OR the TASK section / bare text of the hand-off>
    recon: |
      <the RECON section verbatim, when present; omit the field otherwise>
    round: 1
    round_cap: 4
    questions_per_round_cap: 3
    prior_qa: []
    spec_path: <spec_path>
  """
)
```

`issue_body` and `recon` never mix: the first is the task, the second is claims.

Capture the returned `agentId` into the state file's `agent_id`. If the harness does not return a usable `agentId`, leave it null — rounds 2+ will use the cold-spawn fallback.

**Rounds 2..cap — two gates first, then warm reuse if possible, cold fallback otherwise:**

0. `git ls-files --error-unmatch <spec_path>` exits 0, or STOP: the spec is untracked and 3b item 0 was skipped — run it now, then continue. A delegate edits this file with scripts; an untracked file has no copy to restore from.
0a. Every message the owner sent since the previous round is already a `prior_qa` entry (its verbatim text as `answer`, the question it answers or `(unprompted)` as `question`). A message that arrived **while the delegate was live** was forwarded to it verbatim by `SendMessage` **as the first action of the turn that saw it** — before any log entry, memory write or verification, because a delegate drafting against a ruling the owner has already reversed costs a whole round (measured 2026-09-08: a ruling seen at 00:00 and relayed at 00:09 arrived after the round it should have shaped had returned `ready`). `AGENTS.md` § *Communication* already says corrections propagate in the same turn; this is the same rule with its position in the turn fixed.


- If `agent_id` is set in state: `SendMessage(to=agent_id, prompt="""<same fields with updated round + prior_qa>""")`. Capture the response.
- If `agent_id` is null OR the `SendMessage` call fails: cold spawn a fresh `Agent(subagent_type="spec-writer", prompt=...)` (no `model=` — the frontmatter `inherit` governs) with the full state in the prompt (the Subagent definition mandates re-derivation from prompt anyway). Update state file's `agent_id` from the new spawn (may again be null).

> The cold-spawn path is the **default contract**; warm reuse is an opportunistic optimization conditional on the harness returning a usable `agentId` and `SendMessage` succeeding.
>
> **Cold-spawn is the contract, not a fallback.** Do **NOT** probe `ToolSearch` for `SendMessage` per round — its absence is stable for the whole session, so re-probing each round is wasted overhead. Do **NOT** emit fallback-framed status lines such as *"SendMessage not available — using cold spawn"* (they read as a regression). If a status line is emitted at all, phrase it neutrally — *"Spawning round N spec-writer."* Treat warm reuse as a silent optimization, never as the headline.

#### 3b. Parse the YAML status block

0. **Track what came back before reading what it says.** `git add <spec_path> <state_path> && git commit -q -m "chore(plans): interview round <N> — spec draft"` (the file may be new or modified; both are fine — `git add` on an unchanged path is a no-op). This is the copy the next delegate edit can be restored from, and the one `doc-edit-guard.sh` cannot replace: the guard undoes a truncating edit in the same command, git undoes everything else.

The subagent's response ends with a fenced YAML block. Extract it; parse `status`, `round`, and `questions` / `reason` as applicable.

**On parse failure** (malformed YAML, missing required fields):

1. **One-shot retry** — `SendMessage(to=agent_id, ...)` (or fresh `Agent` if cold) with prompt: `"Re-emit only the YAML status block, exact schema. Your previous response did not contain a parseable status block at the end."` Parse again.
2. **On second failure** — orchestrator-injects a synthetic `unresolvable` and proceeds to 3d:
   ```yaml
   status: unresolvable
   round: <current>
   reason:
     category: logically_unresolvable
     detail: "Spec-writer subagent emitted unparseable YAML status twice."
     suggested_action: abort
   ```

#### 3c. Branch on status

- **`ready`** → go to **Step 4**.
- **`ask`** → go to **3d** (surface questions).
- **`unresolvable`** → go to **3e** (action chooser).

#### 3d. Surface questions to the user

Validate before forwarding:

- `len(questions) <= questions_per_round_cap` — if exceeded, send the Subagent a one-shot trim instruction (`"Trim to <cap> highest-leverage questions; emit only the YAML status block."`).
- Each `header` ≤ 12 chars; each `options` list has 2..=4 entries (`AskUserQuestion` constraints).
- No question contains a Rule-5 blacklisted substring (final defence — the Subagent should have caught it). On violation: one-shot Subagent re-spawn with explicit instruction to re-read AGENTS.md and the Rule-5 blacklist.

Then call `AskUserQuestion(questions=[...])` with the entire list — the tool supports up to 4 questions per call, so 1..=3 fit cleanly. The user answers all in a single UI exchange.

Append each `(question, answer)` pair to state's `prior_qa` with `round: <current>`. Increment `round`. Loop to 3a.

#### 3e. Action chooser on `unresolvable`

Build an `AskUserQuestion` with:

- The Subagent's `reason.detail` as the question prose.
- Options: the Subagent's `suggested_action` first (recommended), plus the other applicable actions per the table below.

| Category | Actions to offer (recommended first) |
|---|---|
| `cap_reached` | `extend_cap` (recommended), `defer_to_deferred`, `abort` |
| `logically_unresolvable` | `defer_to_deferred` (recommended), `abort`, `request_external_info` |
| `external_dependency` | `request_external_info` (recommended), `defer_to_deferred`, `abort` |
| `empty_scope` | `abort` (recommended), `request_external_info`, `defer_to_deferred` |
| `user_loop` | `defer_to_deferred` (recommended), `abort`, `request_external_info` |

Execute the chosen action:

- **`extend_cap`** — bump `round_cap += 1` in state; loop to 3a with `round: <current> + 1`. The Subagent receives the new `round_cap` and may now `ask` if it has questions.
- **`defer_to_deferred`** — `git mv <spec_path> ai-docs/plans/deferred/` (the spec is tracked from 3b item 0 of round 1); update `INDEX.md` (move row to **Deferred plans**, status `🟡 spec-only`); `git rm` the state file; commit; exit. Skip Step 4.
- **`abort`** — `git rm <spec_path>` and the state file (both tracked from 3b item 0 of round 1), commit, exit. Skip Step 4. The branch is left for the user to delete.
- **`request_external_info`** — prompt the user via `AskUserQuestion` (single free-form question option) for the additional context; loop to 3a with `extra_context: <user paste>` injected into the next round's prompt.

### Step 4: Cross-link and exit (on `ready`)

0. **Run the spec gates yourself before the owner sees the spec** — `bash ai-docs/scripts/check-spec-anchors.sh <spec_path>`, `bash ai-docs/scripts/check-spec-shape.sh <spec_path>` and `bash ai-docs/scripts/check-ac-shape.sh <spec_path>`, each read by its exit code, never piped. The delegate ran them before `ready`; its green is a claim (AGENTS.md § *Patterns* 1). Red → re-spawn the round (cold, same fields) with `extra_context` carrying the gate output verbatim and nothing else; red twice → surface the output to the owner via `AskUserQuestion`, quoted. **This is the orchestrator's whole check of the spec's zone: an exit code, not a reading.** Neither add, reword nor strike a row yourself, and do not audit the rows by judgment in place of the gates — the audit an orchestrator ran by judgment on 2026-09-10 hunted added scope and passed every row that restated a standing rule or prescribed a mechanism (`ai-docs/learnings.md` 2026-09-10). A row the owner wants changed after reading goes back to `spec-writer` as their answer, recorded in `prior_qa`.
1. Show the user the final spec at `<spec_path>` (last 80 lines if long).
2. Confirm — `AskUserQuestion`: "Approve and post cross-link comment?" / { Approve, Tweak first }.
3. On Approve:
   - Resolve the tracking issue if not already pinned (issue-ref mode = already pinned; free-text mode = run the issue-search / propose-new flow):
     ```bash
     gh issue list --state open --search "<keyword>"
     ```
     If a candidate exists, ask user. Otherwise propose a new issue (title from spec name; body from spec scope) and run `gh issue create` after user approval. Capture the number into the spec's `**Tracked in:**` field if it wasn't there.
   - Post the cross-link:
     ```bash
     gh issue comment <N> --body "Spec: \`<spec_path>\`"
     ```
   - **Do NOT delete the state file.** It is kept for every later return to `spec-writer` (§ *State file* → lifecycle table); `/task` Step 12 retires it. The final spec and state file are already committed by 3b item 0; `git status --porcelain -- <spec_path> <state_path>` is empty before exiting, or commit what it lists.
4. Skill exits. `/task` (the caller) resumes at Step 6 (`design-writer` Subagent).

> **Skip the tracking-issue resolution only if the user explicitly states "no tracking issue".** Note the reason in the spec header (`**Tracked in:** none — <reason>`) and skip the cross-link comment.

## Spec-only run

If the user wants to stop after the interview ("just draft the spec, defer the implementation"):

1. Move the spec to `ai-docs/plans/deferred/`
2. Update `INDEX.md` (move row to **Deferred plans**, status `🟡 spec-only`)
3. Delete the state file
4. Do NOT proceed to Step 6 of `/task`. The spec can be picked up later via `/task`'s deferred-plan-activation preamble.

## Patterns

### 1. Delegate every question and every spec write to the `spec-writer` Subagent

**Default to** delegating every question and every spec write to the `spec-writer` Subagent. The orchestrator's role is plumbing — surface the Subagent's questions via `AskUserQuestion` and forward the user's answers as `prior_qa`; never draft a clarifying question yourself, even when the next question feels "obvious" from the user's last answer. Same for the spec body: never edit `*.spec.md` directly — even when the change feels like "just a typo" or "just a tweak the user asked for after `status: ready`". The `spec-writer` Subagent owns ALL writes to `*.spec.md` (mirrors the AXIOM in `.claude/skills/task/SKILL.md` above the Design Amendment header).

_Validated by repeated user correction across multiple rounds: "from now and for future — don't ask by yourself, delegate to subagent". Recorded in the sibling **quartzite** project's memory namespace — `~/.claude/projects/-home-syt-RustroverProjects-quartzite/memory/feedback_interview_delegate_to_subagent.md` — and in quartzite's `ai-docs/learnings.md` 2026-05-24 entries on orchestrator-side direct spec edits. Both corrections happened **in quartzite**, not here; this repo's log begins 2026-08-29. The rule also stands locally on the AXIOM cross-referenced above._

## Anti-patterns

- Drafting questions yourself in the orchestrator. The subagent owns question authorship; you forward the questions verbatim.
- Mutating the spec yourself. The subagent owns spec writes; the orchestrator only reads it.
- Passing the spec to the owner on the delegate's word that its gates are green, or judging its rows in place of running them (Step 4 item 0).
- Editing, reordering or summarising a `prior_qa` entry — spec anchors index into it (§ *State file*).
- Skipping the YAML status parse and inferring intent from prose. The status block is the contract; treat parse failure as a defect.
- Embedding the Rule-5 substring blacklist in this file. It lives in the Subagent definition; this orchestrator's only Rule-5 role is the validation gate at 3d (defence in depth).
- Deleting the state file on `ready`. It is the re-entry point for every later `spec-writer` round (§ *State file*); only `abort` and `defer_to_deferred` remove it. An orphaned state file from an abandoned run is caught by `⚡ First`'s validation sequence, not by destroying the record of a live one.
- Saving the spec without `**Tracked in:**` (unless user explicitly opted out).
- Skipping the cross-link comment on the tracking issue.
- **Silently switching to implementation mid-interview.** If the Subagent's first round suggests the task is trivially small (< ~20 lines, no design decisions), the Subagent should still emit `ready` with a complete spec; the orchestrator surfaces it normally and the user can choose to spec-only-defer if they want a one-shot edit instead.
