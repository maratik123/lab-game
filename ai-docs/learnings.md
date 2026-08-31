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

### 2026-08-31 — process — evidence gathered and then not read is not evidence
**What happened:** Reconstructing a `genkernel` command line for the user's host, I emitted `--lvm --mdadm` — after having already inspected the live initramfs and seen `usr/lib/udev/{probe-bcache,bcache-register}`, `69-bcache.rules`, and a root filesystem on `/dev/bcache0`, with LVM present nowhere on the machine. The correct flags were `--mdadm --bcache`. The advice would have produced an unbootable system, and the refuting observation was already in my own tool output.
**Rule:** Every flag in a command line reconstructed for a specific host must trace to a named observation from this session. A flag with no observation behind it (`--lvm`) is fabrication, and a missing flag whose evidence you already printed (`--bcache`) is worse than an unchecked guess — the tool output was read for one question and never re-read for the one that mattered. Before emitting such a command, walk its flags against the gathered evidence one by one.
**at:** 77696ee
**Kind:** correction
**Escalated?** no

### 2026-08-31 — tooling — an ebuild's postinst is not a substitute for reading the machine's active selection
**What happened:** Told the user `emerge app-containers/podman` would be sufficient for the container network stack, assuming `net-firewall/iptables`'s `pkg_postinst` would point the iptables backend at nftables. It does so only when no selection exists; this host had chosen `xtables-legacy-multi` in 2022, so `netavark` would have called `iptables-legacy` and the kernel would have answered "Table does not exist". `eselect iptables set xtables-nft-multi` was required and absent from the plan. I never ran `eselect iptables list`.
**Rule:** A package manager's install-time default is a claim about a *fresh* system, never about *this* one. Where a Gentoo package has an `eselect` module (iptables, kernel, python, java-vm, editor), read the current selection with `eselect <mod> list` before asserting what an `emerge` leaves behind. Generalises the AGENTS.md § Dependency Versions axiom to a sixth category: a system-wide alternatives/selection state.
**at:** 77696ee
**Kind:** correction
**Escalated?** AGENTS.md

### 2026-08-31 — process — a verification that answers an adjacent question launders a wrong list as a checked one
**What happened:** Before handing over a 50-symbol kernel config list, I grepped `Kconfig` for each symbol's existence and reported "все 50 символов существуют", which read as validation of the list. Existence was the wrong question: roughly half the list was unnecessary, and `IP_NF_NAT` depends on `IP_NF_IPTABLES_LEGACY` and would have been silently dropped by `make olddefconfig` — so the list was both bloated and partly inert, under a green check.
**Rule:** State what a check proves and what it does not, in the same breath as its result. A passing check over property A is not evidence for property B, and reporting it bare next to a deliverable transfers unearned confidence — the more so when the check was expensive enough to feel like diligence. For a config list the questions are three and separate: does the symbol exist, are its dependencies satisfiable in this tree, and is it needed at all. Same family as the AGENTS.md pipeline-exit-status axiom: the command answered a different question than the one asked.
**at:** 77696ee
**Kind:** correction
**Escalated?** no

### 2026-08-31 — process — followed a skill's restated ordering instead of opening the contract it cited
**What happened:** Running `/improve`, I took `.claude/skills/improve/SKILL.md` item 6 at its word — "apply the approved proposals to the working tree first, then dispatch the clean-context reproducers" — and applied three proposals before any baseline. That same item points at `self-improve.md § Step 6` for the eval contract, and the canonical page it leads to (`ai-docs/improve-eval-contract.md` § *The RED baseline*) fixes the opposite sequence: baseline batch first, because the pre-change state "is obtained by not having applied anything yet". I opened the contract only after applying, when I went looking for how to run the baseline. Recovered by cp-backing-up the applied files and restoring HEAD for the baseline window, so no proposal reached history unevaluated.
**Rule:** A pointer to a canonical source is an instruction to open it, and it outranks the pointing document's own summary of what it says. When one instruction file both restates a procedure and names another as canonical for it, the restatement is the stale copy by default — read the named source before the first irreversible step, not when the restatement runs out. Tell: any step whose cost is asymmetric (applying is cheap, un-applying mid-flow is not) is the step that must be preceded by the read.
**at:** bd89550
**Kind:** correction
**Escalated?** no

### 2026-08-31 — testing — a control that comes back uniformly clean is a claim about the instrument first
**What happened:** Four eval baselines dispatched against the pre-change tree — one proposal probed in four different scenario shapes — all came back GREEN, which under the contract meant no proposal could commit. The available reading was "the model already does this, the rules are unnecessary". Instead of taking it, I looked for a channel that could produce that result independent of the rules, and found one: `AGENTS.md` § *Agent Docs* heads its table "Read on nearly every task" and lists `ai-docs/learnings.md`, so the pre-change tree instructs every baseline agent to read the correction the rule was derived from. The confirming detail was a date — a baseline answer asserted "this host carried a 2022 selection", and `eselect iptables list` prints no dates; that fact exists only in the source entry.
**Rule:** When a control, a negative test, or a baseline comes back clean across every variation you try, spend the next step on the instrument rather than on the conclusion — a uniformly clean control and a genuinely absent effect look identical from the result alone, and only one of them is worth acting on. Look for a specific in the output that could only have come from the channel you meant to close; a date, a count, or a proper noun the subject had no other way to know is the cheapest such probe. Same shape as the pipeline-exit-status axiom one level up: the run answered a different question than the one asked.
**at:** bd89550
**Kind:** validation
**Escalated?** no

### 2026-08-31 — process — ran the next phase's read-only work while the previous phase's question was still unasked
**What happened:** `/ai-audit` Phase 1 says: after the subagent reports, if it left any entry as *needs user judgment*, present each one and ask how to resolve **before continuing to Phase 2**. The subagent was still running, so I started Phase 2's Step 2.2 inventory to fill the wait — then kept going through the guard runs and frontmatter sweeps after it returned with exactly such an item, and only asked once I had most of Phase 2's mechanical work in hand. My reasoning was that the work was read-only and the answer could not change it, which was true here and is not the test the skill states. I noticed mid-flight, argued myself into "the risk being guarded against is compounding, and I'm not compounding", then asked anyway — but by then the ordering was already spent.
**Rule:** A skill's "ask before continuing to X" is a **sequencing** instruction, not a data-dependency hint. "My next step is read-only / cannot be invalidated by the answer" is not an exemption the skill offers, and it is a predictably self-serving reading — it always licenses proceeding. Filling a subagent's runtime with the *next* phase's work is the specific temptation: legitimate before the delegate returns, a violation the moment it returns with an open question. When a delegate lands a needs-judgment item, stop the phase there and ask; resume the pipeline after the answer, not around it.
**at:** e97768c
**Kind:** correction
**Escalated?** no

### 2026-08-31 — testing — counted the instrument's own output before reading the check it fed
**What happened:** `/ai-audit` Checklist O ended with `comm -12` returning empty, which the checklist's own table reads as *"no flag — clash-scan baseline holds"*. The line above it said `embedded names: 0`. Taking the pass would have been the whole of the check. Reading the zero instead: the checklist cited `claude-tools-hierarchy.md §§1a/1b/2a/3a/3b` as its corpus, and `git show` over every commit that ever touched that file returned 0 such headings in all five — the sections never existed, and the file inventories *project* names rather than embedded ones anyway. So the extractor had always returned nothing and the intersection had always been empty, on any tree. Its cited authority was absent too: `grep -rni clash AGENTS.md` → no match, though the checklist opened by claiming to enforce an "AGENTS.md `## Propagation Rule` clash-rename AXIOM". Rewritten with a fail-closed precondition; it now finds 26 project names against 31 embedded ones.
**Rule:** For any check shaped as *intersect two sets* / *diff against a baseline* / *grep a corpus*, read the **cardinality of each input** before reading the verdict — an empty right-hand side makes `comm -12`, `grep -f`, and `diff` all report the clean answer for every possible left-hand side. Make it structural where the check is written down: give the recipe an explicit `inconclusive` outcome for the empty-corpus case, so a future reader cannot record a pass the instrument never earned. Second, cheap and separate: a checklist that names a `§`-anchor or an AXIOM as its authority is making a citation, so resolve it — both of this one's were fabricated at import and survived because nothing downstream ever needed them to be real. Recurrence of the 2026-08-31 *a control that comes back uniformly clean* entry, one layer down: there the clean control was the eval baseline, here it is the guard itself.
**at:** a8191a9
**Kind:** validation
**Escalated?** no
