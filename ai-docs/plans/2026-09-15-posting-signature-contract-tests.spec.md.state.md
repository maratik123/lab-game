# Interview state — posting-signature contract tests

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-15-posting-signature-contract-tests.spec.md
issue_ref: "#26"
gh_issue:
  title: "Posting-signature contract-test framework"
  state: open
  labels: ["mvp", "area:ledger"]
  body: |
    ## What

    The design promises that every basis-document type has an expected posting set, and that a contract test checks actual postings against it. That check does not exist. This issue builds the framework and applies it to the document types that exist by then.

    ## Design refs

    - `docs/DESIGN.md` §11 — **posting signatures per document type**: `craft_succeeded` writes resource debits plus an item credit; `shop_sale` is resource against money; a manual correction is any balanced set. The structural check "actual postings match the signature" is the economy's contract test. The §13.4 event dictionary extends into signatures: a new mechanic declares its event **and** its signature, and the harness verifier checks it.
    - `docs/DESIGN.md` §13.4 + `AGENTS.md` § Domain Rules — **a mechanic that moves balances declares its posting signature in the same PR**, with the contract test that checks it. This is an axiom, not a nicety.
    - `ai-docs/deferred/_inbox.jsonl` — recorded as deferred from the ledger-core task with the note "needs a mechanic with a signature to check — with the first posting mechanic". This is that follow-up.
    - `docs/DESIGN.md` §11 — item movements are part of the same contract: one document produces postings in both machines, and **both** signatures are checked.

    ## Depends on

    #25

    ## Scope

    - A signature declaration form: per basis-document type, the expected postings (which account definitions, which kinds, which signs, which cardinality) and the expected item movements.
    - A test harness that, given a document type and a real transaction, asserts the actual postings and movements conform — including that nothing unexpected was written.
    - A registry completeness check: a basis-document type with no declared signature fails the suite. **It runs in CI as part of the test gate `make verify` already carries, and it fails the build** — `docs/DESIGN.md:430` (§13.4) states the declaration as a process invariant and `:328` (§11) defines what is declared, so this is a gate, not a disposition. A check that runs only when someone remembers is the disposition.
    - Application to every document type that exists when this lands.

    ## Out of scope

    - Signatures for mechanics that do not exist yet — each ships with its mechanic, which is the point.
    - The event-dictionary half of the obligation — #21 owns the registry; this issue owns the postings half.

    ## Telemetry obligation

    This issue is the enforcement mechanism for the posting half of the telemetry axiom.

    ## Open questions to close in the spec

    - Whether a signature is Go data or a declarative file, and how a reviewer sees a signature change in a diff.
    - How "any balanced set" (manual correction) is expressed without weakening the check for everything else.

    ---

    Part of #47 (MVP roadmap).
  comments: []
  linked_issues: ["#25", "#21", "#47"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: null
prior_qa: []
```
