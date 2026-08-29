# Documentation conventions — lab-game

Rules for Go doc comments. `revive`'s `exported` and `package-comments` rules enforce the mechanical half; the rest is review.

## Scope

Applies to every `.go` file under `cmd/` and `internal/`. Test files document only what is non-obvious about the fixture.

## DOC-1 — Summary sentence

A doc comment starts with the identifier's name and reads as a sentence:

```go
// Post writes one balanced group of postings under a single basis document.
func Post(ctx context.Context, tx pgx.Tx, basis Basis, ps ...Posting) error
```

Not `// This function writes…`, not `// Writes…`. Name first, then a verb in the third person.

## DOC-2 — Package comment

Every package has exactly one package comment, on the file named after the package (or `doc.go` when the package is large). It states what the package owns and what it deliberately does not:

```go
// Package ledger implements the double-entry accounting machine: postings,
// basis documents, and the daily close. It owns balance mutation entirely —
// no other package writes to the balance tables.
package ledger
```

## DOC-3 — Contract sections

Document the contract, not the implementation. Where relevant, in this order: what it does · what it assumes (preconditions) · what it returns · which errors it can return and what they mean · concurrency safety.

- A function returning a sentinel error names it: *"Returns [ErrNoStamina] when the player's balance would go negative."*
- A `…Unchecked` variant states its precondition and the guard that guarantees it (see [`go-api-naming.md`](go-api-naming.md)).
- A function safe for concurrent use says so; the default assumption is that it is not.

## DOC-4 — Design citations

Anything implementing a designed mechanic cites its section, so a reader can find the reasoning without hunting:

```go
// Depth is the distance to the nearest chat entrance in any chat's maze,
// measured on the chunk grid. Crafted doors deliberately do not count
// (docs/DESIGN.md §2.2.4) — a door shortens the road, never the danger.
```

Cite the section number, never a line number: the design document is edited, and `§2.2.4` survives what `:118` does not.

## DOC-5 — What not to write

- No comment that restates the code (`// increment i`).
- No commented-out code — the history holds it.
- No `TODO` without an issue reference: `// TODO(#42): …`. A `TODO` without an owner is a lie about future work.
- No stale comment: changing behaviour without updating the comment above it is the same defect class as a broken test.

## DOC-6 — Examples

An `Example…` function in `_test.go` is the preferred documentation for anything with a non-obvious call sequence — it compiles, it runs in CI, and it cannot rot silently.
