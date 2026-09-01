#!/usr/bin/env bash
#
# Citation-namespace guard.
#
# INVARIANT: every citation in the harness must resolve for its reader.
#   A bare `#N` / `PR #N` is repo-relative -> must resolve in THIS repo.
#   This harness was ported from graphite-gp, which had ported it from
#   quartzite, so most inherited citations point at one of those two repos.
#   Such a citation MUST name its namespace (`maratik123/graphite-gp#N`,
#   `maratik123/quartzite#N`, "graphite-gp's `ai-docs/learnings.md` <date>",
#   or a full path under that project's memory namespace).
#
# Run by `/ai-audit` Phase 2 (checklist item P) against the full instruction
# surface. Also runnable standalone: `bash .claude/skills/ai-audit/scripts/check-citations.sh`.
#
# On FAIL: qualify the offending citation with its namespace. NEVER drop a
# citation to make this pass -- dropping destroys verified history and is
# the bug this guard exists to catch, not a valid fix.
#
# Exclusions carried over from the guard's original red test:
#   - ai-docs/learnings.md itself (append-only local log; not a citation site)
#   - ai-docs/bugfix/** (deleted-on-resolution trace files, not durable)
#   - illustrative example filenames matching YYYY-MM-DD-slug.ext (no authority
#     claim -- e.g. a sample spec filename in a template or table row)
#   - already-namespace-qualified forms (naming quartzite or graphite-gp)
#   - scripts/test-*.sh fixture payloads (test data, not citations)
# One additional targeted exclusion added when this guard was promoted:
#   - corrections-log.md's `Superseded by:` field-spec row is FORBIDDEN to
#     touch -- it is a format-spec example
#     illustrating that field's date-ref syntax, not a citation with a real
#     referent. Excluded by matching the ROW'S OWN TEXT within that one file,
#     so any *other* date citation later added to corrections-log.md still
#     gets checked.
#     NOT by line number: this exclusion was originally pinned to
#     `corrections-log.md:47`, an unrelated commit inserted rows above it, and
#     the pin silently re-pointed at a dateless neighbour -- un-excluding the
#     example (guard RED on a clean tree) while appearing to still work.
#     Locked by case 2 of scripts/test-check-citations.sh.
#
# SCOPE -- what this guard does NOT cover, and why. State this openly: a
# silent exclusion reads as "covered everything" when it did not.
#   - ai-docs/plans/**  : frozen historical records. A merged spec/design
#     documents what was decided AT THE TIME; rewriting its references
#     edits history rather than an instruction.
#   - ai-docs/deferred/**: `_inbox.jsonl` is AXIOM-protected -- AGENTS.md
#     § Workflow: written ONLY by /task Step 12 and /triage, never by hand.
#     A guard that fails on a file nobody may hand-edit is a guard that
#     teaches people to ignore it.
set -uo pipefail

# The high-water mark is an INSTRUMENT reading. An empty or non-numeric value
# (gh auth/network hiccup, a lock collision with a concurrent gh call) must
# fail loudly as an instrument error -- never leak into the comparison, where
# it would flag every citation as unresolvable and dress the outage up as a
# finding. Retry briefly, then stop.
LOCAL_MAX=""
for attempt in 1 2 3; do
  LOCAL_MAX=$(gh pr list --state all --limit 1 --json number --jq '.[0].number // 0' 2>/dev/null)
  case "$LOCAL_MAX" in ''|*[!0-9]*) LOCAL_MAX=""; sleep "$attempt" ;; *) break ;; esac
done
if [ -z "$LOCAL_MAX" ]; then
  echo "ERROR: could not read the local PR high-water mark (gh pr list returned nothing numeric after 3 attempts)." >&2
  echo "       Instrument failure, not a citation finding -- check gh auth / network and re-run." >&2
  exit 1
fi
fail=0

echo "== local high-water mark: PR #${LOCAL_MAX} =="
echo
echo "--- (1) unqualified 'PR #N' / bare '#N' claiming to be local but exceeding it ---"
# Matches BOTH `PR #N` and a bare `#N`. Bare `#N` is the same defect: GitHub
# resolves it repo-relative, so an imported bare ref silently rebinds. A
# `PR #N`-only regex is blind to it -- that blind spot shipped once already
# (self-review Round 1), because the sweep inherited AC3's `PR #N` command
# and inherited its aim. Match the DEFECT, not the phrasing that first
# surfaced it.
#
# EXAMPLE sites carry no authority claim and MUST stay green -- see (a)/(b)
# below. `#N <= LOCAL_MAX` is skipped too: it may be a real local ref.
# Hex colours need TWO defences, not one -- see (c). Do not "simplify" that
# to `\b` alone; it looks sufficient and is not.
while IFS=: read -r file line cite; do
  n=${cite##*#}
  [ "$n" -le "$LOCAL_MAX" ] 2>/dev/null && continue
  txt=$(sed -n "${line}p" "$file")
  # CONTEXT-QUALIFIED: the line already names the sibling namespace, so a bare
  # `#N` beside it inherits that scope and a reader can resolve it. Required for
  # quoted PR-body syntax (`Closes #289` on maratik123/quartzite#295) and for
  # run-on refs (`maratik123/quartzite#168`, `#169`) -- rewriting either to
  # spell the qualifier per-number would falsify the quote or bloat the prose.
  case "$txt" in *quartzite*|*graphite-gp*) continue ;; esac
  # EXAMPLE shapes carry no authority claim. Two kinds, handled differently:
  #
  # (a) TEMPLATE FIELD lines -- ANCHORED to the line start, so the key must BE
  #     the field, not merely appear in prose. Unanchored, `detail:` matched
  #     mid-sentence and silently swallowed real citations: any prose line
  #     opening "In detail: ..." and then citing a high-numbered ref was
  #     skipped. That is the substring-blacklist trap this guard warns about;
  #     anchoring closes it.
  echo "$txt" | grep -qE '^[[:space:]]*[-*]?[[:space:]]*(issue_ref|linked_prs|tracked_in|detail):' && continue
  # (b) PROSE specimens -- a line that quotes an illustrative `#N` inside
  #     example text where no field key exists to anchor to. Excluded by a
  #     phrase that sits ON THE LINE ITSELF, never by file:line. This started
  #     as a pair of file:line pins. The spec-writer.md pin had drifted onto an
  #     empty line without the guard noticing (the same rot the header records
  #     for check (2)) -- and its specimen turned out to be covered by the
  #     anchored template-field rule in (a) anyway. The task/reference.md pin
  #     was still accurate; the phrase match has been in place since the
  #     learning-loop import, and the commit that rewrote this comment moved
  #     that line by 57 rows -- which would have broken a pin, had the code
  #     still carried one. Live specimen: the entry_args format demo in task/reference.md (bare-vs-plain
  #     issue-ref argument forms). A phrase match is broader than a pin -- every
  #     line carrying the phrase is exempt -- so keep it specific to the demo's
  #     wording and re-run test-check-citations.sh after touching it.
  #     (Described, not spelled: a comment that quotes the bad shape IS the bad
  #     shape, and this script would flag its own source. See reference.md
  #     Checklist P -- "describe the bad shape; spell it only alongside its fix.")
  case "$txt" in
    *entry_args*) continue ;;
  esac
  # (c) HEX COLOUR shape. `\b` alone is NOT sufficient: it excludes #93A2B8
  #     (letters break the digit run) but an ALL-NUMERIC hex like #123456 or
  #     #000000 DOES match it. Exclude the 6-digit shape explicitly -- no
  #     plausible issue/PR number here is 6 digits (local max 78; quartzite
  #     max 628). Verified, not assumed: an earlier comment here claimed \b
  #     made hex unmatchable, which is false.
  case "$n" in [0-9][0-9][0-9][0-9][0-9][0-9]) continue ;; esac
  printf '  RED  %s:%s  -> %s (local max %s; does not resolve here)\n' "$file" "$line" "$cite" "$LOCAL_MAX"
  fail=$((fail + 1))
done < <(grep -rnoE '(^|[^a-zA-Z0-9/_-])#[0-9]+\b' .claude/ AGENTS.md ai-docs/ 2>/dev/null \
           | grep -v learnings.md | grep -v '^ai-docs/bugfix/' \
           | grep -v '^ai-docs/plans/' | grep -v '^ai-docs/deferred/' \
           | grep -v '/scripts/test-[a-z-]*\.sh:' \
           | sed -E 's/:([^:]*)(#[0-9]+)$/:\2/')

echo
echo "--- (2) 'learnings.md <date>' citations outside this log's range ---"
# This repo's log starts 2026-08-29. Any earlier date cannot be a local entry.
# CITATION only: require a 'see|entry|validated|recurrence|added' cue on the
# line, so illustrative example FILENAMES (2026-05-01-paint-style.spec.md) do
# not fire.
while IFS=: read -r file line _; do
  txt=$(sed -n "${line}p" "$file")
  # Targeted exclusion: the `Superseded by:` field-spec row illustrates the
  # same-date disambiguation syntax — `YYYY-MM-DD ("slug")` — so its date is a
  # format example, not a citation.
  # KEEP THIS COMMENT FREE OF LITERAL IN-RANGE DATES AND CUE WORDS: this file
  # is itself scanned by check (2) below, so writing the shape concretely here
  # would make the guard flag its own source.
  # Addressed by CONTENT, never by line number: this exclusion was previously
  # pinned to `corrections-log.md:47`, and an unrelated commit inserting rows
  # above it moved the row to :49, which both un-excluded the example (RED on a
  # clean tree) and silently re-pointed the pin at a dateless neighbour.
  # Locked by case 2 of test-check-citations.sh.
  # shellcheck disable=SC2016  # literal backticks are the pattern, not an expansion
  [ "$file" = "ai-docs/corrections-log.md" ] &&
    printf '%s' "$txt" | grep -q '^> `Superseded by:`' && continue
  echo "$txt" | grep -qiE 'see |entry|validated|recurrence|added ' || continue
  echo "$txt" | grep -qiE 'quartzite|graphite-gp' && continue   # already qualified
  echo "$txt" | grep -qE '[0-9]{4}-[0-9]{2}-[0-9]{2}-[a-z]' && continue  # example filename
  printf '  RED  %s:%s  -> cites a learnings.md date this log never had\n' "$file" "$line"
  fail=$((fail + 1))
done < <(grep -rnoE '2026-(0[1-7]-[0-9]{2}|08-([01][0-9]|2[0-8]))' .claude/ AGENTS.md ai-docs/ 2>/dev/null | grep -v learnings.md | grep -v '^ai-docs/bugfix/')

echo
echo "--- (3) 'feedback_*.md' cited without its owning namespace ---"
while IFS=: read -r file line _; do
  txt=$(sed -n "${line}p" "$file")
  echo "$txt" | grep -qE 'projects/-home-syt-RustroverProjects-(quartzite|graphite-gp)' && continue
  printf '  RED  %s:%s  -> cites a memory file without naming whose namespace holds it\n' "$file" "$line"
  fail=$((fail + 1))
done < <(grep -rnoE 'feedback_[a-z_]+\.md' .claude/ AGENTS.md 2>/dev/null)

echo
if [ "$fail" -gt 0 ]; then
  echo "FAIL: ${fail} unresolvable citation(s). Each says 'here' but means graphite-gp or quartzite."
  exit 1
fi
echo "PASS: every citation resolves for its reader."
