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
**Escalated?** no

### 2026-08-31 — process — a verification that answers an adjacent question launders a wrong list as a checked one
**What happened:** Before handing over a 50-symbol kernel config list, I grepped `Kconfig` for each symbol's existence and reported "все 50 символов существуют", which read as validation of the list. Existence was the wrong question: roughly half the list was unnecessary, and `IP_NF_NAT` depends on `IP_NF_IPTABLES_LEGACY` and would have been silently dropped by `make olddefconfig` — so the list was both bloated and partly inert, under a green check.
**Rule:** State what a check proves and what it does not, in the same breath as its result. A passing check over property A is not evidence for property B, and reporting it bare next to a deliverable transfers unearned confidence — the more so when the check was expensive enough to feel like diligence. For a config list the questions are three and separate: does the symbol exist, are its dependencies satisfiable in this tree, and is it needed at all. Same family as the AGENTS.md pipeline-exit-status axiom: the command answered a different question than the one asked.
**at:** 77696ee
**Kind:** correction
**Escalated?** no
