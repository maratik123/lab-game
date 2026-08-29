# Panic index

Every intentional panicking call (`panic`, `log.Fatal*`, `log.Panic*`, `must…` helpers, a deliberate nil-map write) in **production** code (outside `_test.go`), each with a one-line justification that it is genuinely unreachable or unrecoverable. Kept in sync by review and by the `panic-gate` hook in `.claude/settings.json`.

**The project targets zero production panics and currently holds it** — the table below is empty. `main` may exit non-zero on a startup failure; that is not a panic and needs no row here. Treat any new panicking call on a request path, a scheduler task, or a ledger write as a red flag: those paths must degrade into an error the caller can render or retry, because a panic in a handler drops a player's action and a panic in a task can leave the session's `seq` un-advanced.

| File:line | Call | Why it cannot fire (or is unrecoverable) |
|---|---|---|
| — | — | — |
