# Learnings

Append-only corrections log. **Read the boundary rules in [`AGENTS.md` § Learning Log](../AGENTS.md#learning-log) before writing.** The copyable entry skeleton lives in [`templates/learnings-entry.md`](templates/learnings-entry.md) — consult it instead of reverse-engineering the format from this file.

Entries are appended at the END, newest last. Never edit, reorder or delete an existing entry.

### 2026-08-30 — process — "append a line to section X" is not satisfied by appending to the file

**What happened:** Updating the `/task` progress file after subtask 1, I appended the required Decisions-log bullet with a shell `>>`, which landed it after the last line of the whole file — inside the Review-register table — instead of at the end of the `## Decisions log` section the instruction named. Caught on the read-back in the same turn and moved; every later subtask used a splice helper that inserts the bullet before the `## Key discoveries` heading.
**Rule:** When an instruction names a *section* to append to, append inside that section. `>>` appends to the **file**, which is a different place, and the two coincide only when the section happens to be last. Read back the region you wrote, not just confirm the write succeeded.
**at:** bcaffd3
**Kind:** correction
**Escalated?** no

### 2026-08-30 — testing — prove a guard edit is load-bearing by running the same fixtures against the pre-edit body

**What happened:** After extending the `PreToolUse` piped-gate guard's alternation, the 26-row fixture matrix passed against the edited hook body. That alone does not distinguish "the edit works" from "the fixtures were already satisfied" — a matrix can be green for both reasons. Running the identical suite against the pre-edit body, extracted with `git show HEAD:.claude/settings.json`, produced exactly 11 failures — the ten `make` and `golangci-lint fmt` shapes plus the accepted dry-run false positive — and no others.
**Rule:** When a change is supposed to flip specific behaviour, run the new test against the OLD artefact as well. The set of rows that flip, and only that set, is the evidence that the edit is load-bearing *and* that it touches nothing else. A pass against the new artefact alone is equally consistent with a tautological test.
**at:** 05418a8
**Kind:** validation
**Escalated?** no
