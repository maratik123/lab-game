# Progress: World and biome config: generation inputs, lexicon, bestiary — ACTIVE
_Updated: 2026-09-16 04:30_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-16-world-biome-config
**base_commit:** da3e6a8e755fb91129f795e2b73aa4cccf828b19
**Last build:** PASS

**Issue:** #28
**Spec:** ai-docs/plans/2026-09-16-world-biome-config.spec.md

**current_step:** Step 8 — subtask 6 of 8 complete
**last_passed_gate:** go build ./... ; go test ./... (whole repo) ; golangci-lint run ./... ; golangci-lint fmt -d ; go vet ./... ; make comment-refs ; make file-limits ; make import-guard | 2026-09-16 | (pending commit)
**entry_args:** 28

## Next action

**Do this immediately:** spawn Group A (subtasks 1–8) through `/context-reset` into `code-writer`, per the design's `## Handoff plan`; no inline model override — the subagent's sonnet/medium is frontmatter-pinned.

## Subtasks

- [x] 1. Biome resource kinds in the ledger — migration 00008, Go mirror, member accessor
- [x] 2. Lift the YAML schema walker into its own file; add the non-scalar leaf kind
- [x] 3. World value types + scalar half of the world schema (id, seed, generation inputs, k)
- [x] 4. Content half of the world schema (resource profile, naming style, lexicon, bestiary)
- [x] 5. Author the MVP world; delete the placeholder; re-word both falsified `.env.example` clauses
- [x] 6. World-set loader and wiring; `resolveWorldPath` to directory-only; `Config.WorldPath` removed
- [ ] 7. Remove the chunk radius from the balance side  ← CURRENT
- [ ] 8. Tracked-set gates + the CODE half of the AC3 propagation sweep
- [ ] 9. Design corpus (`~/lab-private`): promote the sketch, amend the sketch-count sentence, re-file §16 item 5
- [ ] 10. Key decisions: new entries and the KD amendments
- [ ] 11. Documentation half of the AC3 sweep over `ai-docs/**`

## Decisions log

- **Step 7**: design-review reached GO on round 3; rounds 1 and 2 returned ITERATE. All four GO notes and both recommendations are design-internal — none met a spec-amending trigger — so they were folded by `design-writer` and design-review did not run again, per the Step 7 table.
- **Step 7**: the owner's three answers (round 3 of `prior_qa`) were recorded in the design under the decision each question's stated route named — D8, D13, D3 — and added no spec row. Verified independently: the spec is byte-unchanged across all five design rounds.
- **Step 7**: the orchestrator's fourth `design-writer` pass carried the owner's decisions rather than review findings, and was not counted against the design-review round cap; the cap is attached to design-review cycles. Surfaced to the owner as an interpretation open to correction.
- **Subtask 1**: `migrate_test.go` and `migrate_process_test.go` hard-code the migration count (`want 8`) and `migrate_test.go` hard-codes the `ledger_kind` member set as an exact list — both needed updating for migration 00008 (count 9, three new members). Not called out by the design's decomposition table; found by running the store package's tests after adding the migration and fixed in the same commit, since it is a mechanical consequence of D8/D9, not a design deviation.
- **Subtask 1**: named the exported member-list accessor `store.Kinds()` (design leaves the name open — decomposition subtask 1 says only "the exported member-list accessor"). Returns a fresh copy each call, matching the existing `stringsOf`/copy-out convention already used for the DB comparison in `enums_test.go`.
- **Subtask 3**: decoded every integer destination (int32 radius, int64 seed, uint64 weights) via `n.Decode(dst)` directly into the typed pointer rather than decoding into a wider int and narrowing — this is what makes go.yaml.in/yaml/v3 itself refuse an out-of-range value (D4's own point) and avoids a gosec G115 conversion entirely, so no `//nolint:gosec` is needed anywhere in `world_schema.go`.
- **Subtask 3**: adding `bindInt32`/`bindInt64`/`bindUint64` pushed the repo-wide count of the `"!!int"` string literal to 5, tripping `goconst`. Fixed by naming `tagInt`/`tagFloat` constants in `schema.go` and using them from both files — a minimal, non-behavioural touch to the subtask-2 lift, not a design deviation.
- **Subtask 3**: `World`'s doc comment on the `Generation maze.Params` field states that a later file adds further fields on this same struct — flagging ahead of time that subtask 4 necessarily also touches `world_schema.go` (to add those fields to `World`), even though the design's subtask-4 file list names only `world_content.go`/`world_content_test.go`. `World` is a single struct declaration; Go does not allow splitting a struct's field list across files.
- **Subtask 3**: `make comment-refs` (run before staging, per the per-subtask gate list) initially failed on `world_schema.go`/`world_schema_test.go`: several doc comments named package-qualified symbols of this module's own other packages (`maze.New`, `maze.Params`, `maze.Algorithm`, `gate.Next`) and one comment carried a decision anchor (`D3`) and a repo path (`world_content.go`) — all banned by D12/DOC-4. Reworded every comment to describe the contract in prose with no qualified symbol, anchor, or path; re-ran `make comment-refs` clean before committing.
- **Subtask 4**: the naming-style slot cross-check (`checkNamingStyleSlots`) is implemented as a separate function over the already-decoded `NamingStyle`, called after a successful `worldContentSchema` walk — not inside either leaf's own binder, since a single leaf's binder only ever sees its own subtree and the check needs both `templates` and `parts` populated first. This mirrors how the generator's own cross-field validation runs after its scalar inputs are bound (design's own "single source of every cross-field refusal" shape for the generation inputs), and keeps the two leaf binders independently testable. The world-set loader (subtask 6) is where this function is wired into the full per-file decode; this subtask's own test file calls it directly against `worldContentSchema`'s output.
- **Subtask 4**: resource-profile weights and the compound leaves are validated with a manual per-element decode (checking each field's YAML tag before `Decode`) rather than a single `n.Decode` into a tagged Go struct — preserving the same "absent key vs. authored zero" and "no quoted-string number" distinctions the scalar binders enforce, which a naive whole-node `Decode` would silently lose.
- **Subtask 4**: `make comment-refs` initially failed on `world_content.go` for two decision-anchor references (`D7`) left in doc comments — reworded to prose with no anchor and re-ran clean before committing.
- **Subtask 5**: verified the authored `config/world/cotton_candy.yaml` end to end before committing it — a throwaway, unstaged `_test.go` in `internal/config` (deleted before commit, never staged) decoded it through `worldScalarSchema`/`worldContentSchema`/`checkNamingStyleSlots` and called the generator's own constructor on the decoded seed and generation params; all green, so the tracked file is known to satisfy the schema and the generator's cross-field checks even though the loader wiring (subtask 6) does not exist yet to exercise it end-to-end itself.
- **Subtask 5**: chose `k: 0` deliberately for the MVP world — the design's own Approach section names `k = 0` as the "concentric rings from the centre" case the whole node-tree/absent-vs-zero property exists to keep distinguishable from an absent key; the tracked file now carries that exact case live.
- **Subtask 5**: `make comment-refs` initially failed on the reworded `.env.example` line for a repo-path finding on the token `*.yaml` (the gate's extension-based repo-path check, not a markdown-path finding) — reworded to say "YAML" instead of `*.yaml`; re-ran clean.
- **Subtask 5**: resource-profile weights (spun_sugar 3, pastel_fleece 2, glitter_dust 1) and the bestiary/naming-style/lexicon content are placeholder-shaped like the tracked balance file's own numbers — loadable and internally consistent, not balance-tuned; the design supplies no numeric guidance for these (D8/D10 fix only the resource-kind tokens and the shape, not weights or rates).
- **Subtask 6**: per-file schema/cross-check errors (`loadWorldFile`) are joined flat via `errors.Join`, never re-wrapped in an outer `fmt.Errorf("...%w", …)` around that join — only the loader-level duplicate-id and empty-set refusals, and the read/parse failures (not `*KeyError`s), get path context added directly in their own message. A single `%w` wrap around an already-joined error breaks `containsKeyError`'s tree walk (it only descends via `Unwrap() []error`, not through a further single `Unwrap() error` layer), so nesting was avoided by construction rather than by extending that test helper.
- **Subtask 6**: the generation-inputs cross-field fold (the generator's own constructor) is reported as a `*KeyError` keyed `"generation"` — the design's own wording is "naming the generation subtree" — carrying the generator's own error text verbatim via `%s` so the message states the actual bound/reason.
- **Subtask 6**: `TestLoadWorldSet_TwoMalformedWorldsProduceBothFailuresInFileOrder` deliberately uses two *different* failure classes (a missing `id` in one file, an invalid `seed` in the other) rather than the same key twice, since two `*KeyError`s sharing one `Key` are indistinguishable to `containsKeyError` — this is a test-design choice, not a loader limitation.
- **Subtask 6**: `make comment-refs` initially failed on `world_load_test.go` for a repo-path finding on the literal filename `b.yaml` in a comment — reworded to avoid naming a bare `*.yaml` token; re-ran clean.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 3 | Decomposition subtask 8: "Two sites it reaches are assessed still-true" — a count of grep-reached sites | design-internal | folded | design § Decomposition subtask 8 @ da3e6a8 |
| G2 | 3 | Subtask 11 names two sweep sites I could not resolve to a falsified claim | design-internal | folded | design § Decomposition subtask 11 @ da3e6a8 |
| G3 | 3 | D6's byte-for-byte lift list omits `schemaNode` | design-internal | folded | design § Key decisions D6 @ da3e6a8 |
| G4 | 3 | Subtask 6 says "the new `Config` field" without stating whether `Config.WorldPath` survives | design-internal | folded | design § Key decisions D1 and Decomposition subtask 6 @ da3e6a8 |
| G5 | 3 | Note 2 is a decomposition-accuracy item, not a scope change | design-internal | folded | design § Decomposition subtask 11 @ da3e6a8 |
| G6 | 3 | `ai-docs/deferred/_inbox.jsonl:31` is answered by this task, but that file is written only by Step 12 and `/triage` | design-internal | folded | design § Decomposition subtask 11 @ da3e6a8 |

## Key discoveries (don't re-investigate)

- The spec-anchor gate resolves `[task: …]` and `[answer N.n: …]` by CONTENT against the interview state file, not by presence — proven by four controls inside the Acceptance Criteria table. Probes placed in `## Open questions` are invisible: the gate's parser judges only `scope`, `kd` and `ac` sections.
- The design corpus `~/lab-private` is a separate repository, outside the PR and outside CI. Subtask 9 edits it in place with `git -C ~/lab-private`. Verified untouched by every design round so far.
- `go.yaml.in/yaml/v3 v3.0.5` refuses an out-of-range value into an `int32` destination and accepts it silently into `int` — the reason D4 binds the radius through `int32` and no G115 conversion exists.
- The owner's three round-3 answers are in the interview state file's `prior_qa` as round 3; design tags `[answer 3.1]`–`[answer 3.3]` resolve against them.

## AC Status

| AC | Status |
|----|--------|
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |
| AC5 | NOT_TESTED |
| AC6 | NOT_TESTED |
| AC7 | NOT_TESTED |
| AC8 | NOT_TESTED |
| AC9 | NOT_TESTED |
| AC10 | NOT_TESTED |
| AC11 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- Subtask 1: `internal/store/migrations/00008_biome_resource_kinds.sql` (new), `internal/store/enums.go`, `internal/store/enums_test.go`, `internal/store/migrate_test.go`, `internal/store/migrate_process_test.go`
- Subtask 2: `internal/config/schema.go` (new, lifted), `internal/config/balance_load.go` (trimmed to `balanceSchema`/`loadBalance`), `internal/config/schema_test.go` (new)
- Subtask 3: `internal/config/world_schema.go` (new: `World` struct, `bindInt32`/`bindInt64`/`bindUint64`/`bindLowerSnakeCaseString`, `worldScalarSchema`), `internal/config/world_schema_test.go` (new); `internal/config/schema.go` touched to add the shared `tagInt`/`tagFloat` constants (goconst, five `"!!int"` literals across the two files)
- Subtask 4: `internal/config/world_content.go` (new: `ResourceEntry`, `NamingStyle`, `BestiaryEntry`, the compound-leaf binders, `worldContentSchema`, `checkNamingStyleSlots`), `internal/config/world_content_test.go` (new); `internal/config/world_schema.go` touched to add `ResourceProfile`/`NamingStyle`/`Lexicon`/`Bestiary` fields to `World` (flagged as a necessary deviation in subtask 3's own decisions-log entry); `internal/config/schema.go` touched to add the shared `tagStr` constant (goconst, three `"!!str"` literals across three files)
- Subtask 5: `config/world/cotton_candy.yaml` (new), `config/world/.gitkeep` (deleted), `.env.example` (both falsified world-path clauses reworded)
- Subtask 6: `internal/config/world_load.go` (new: `loadWorldSet`, `loadWorldFile`), `internal/config/world_load_test.go` (new); `internal/config/world.go` (`resolveWorldPath` rewritten to directory-only via `os.Stat`); `internal/config/world_test.go` (regular-file-succeeds case removed, regular-file-refused case added); `internal/config/config.go` (`Config.WorldPath` removed, `Config.Worlds []World` added, `Load` wired through `loadWorldSet`); `internal/config/config_test.go` (`validConfigEnv` now writes one valid world file instead of an empty directory; `WorldPath` assertion replaced with a `Worlds` assertion); `internal/config/env.go` (`envKeys` doc comment reworded); `internal/config/doc.go` (package comment reworded — the world set is now decoded, not merely probed)
