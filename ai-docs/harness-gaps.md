# Harness gaps

Append-only log of **harness diagnoses**: gaps, ambiguities, and defects in instruction files, queued for `/improve`. This is the designated parking surface `AGENTS.md` § *Learning Log* names — a harness diagnosis written here is NOT an instruction-file edit under Boundary rule 2, and does NOT belong in `ai-docs/learnings.md` (that log holds conduct corrections only).

Boundary rule 1 (append-only) applies here verbatim: never edit, reorder, summarise, or delete an existing entry; supersession is a new entry plus the old entry's `Superseded by:` field.

## Entry skeleton

```
### YYYY-MM-DD — [short description of the gap]
**target:** [harness file the fix belongs in — e.g. `.claude/agents/self-review.md`]
**Observed:** [what happened that the harness permitted or failed to prevent]
**Gap:** [the property of the harness that allowed it]
**Proposed edit:** [minimal protocol-shaped change — a gate, a required field, a closed list; not a disposition]
**at:** [commit SHA | `main`]    (required for any numeric claim)
**Superseded by:** [ref] — [reason]    (optional)
```

Entries are appended at the END, newest last.

---
