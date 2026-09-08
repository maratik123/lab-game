# API naming

Rules behind `AGENTS.md` § *API Naming*.

## The `…Unchecked` AXIOM

> **A function that skips a validity check its sibling performs MUST carry the `Unchecked` suffix, and its doc comment MUST state (a) the precondition, and (b) who guarantees it.**
>
> | Shape | Verdict |
> |---|---|
> | `func StepUnchecked(s Session, e Edge) Session` with a doc comment naming the guard that already ran | Correct |
> | `func Step(...)` that silently assumes the caller validated | **Wrong** — rename or check |
> | `…Unchecked` whose doc comment does not name the guarantor | **Wrong** — the comment is the contract |
>
> The suffix is a warning to the reader, not an optimisation badge. If no measurement showed the check mattered, delete the unchecked variant and keep one honest function.
>
> **Naming the guarantor under the comment-reference ban.** The guarantor is named the way the ban allows: a symbol in the comment's own package is written as the symbol, because a symbol that *is* the contract is not an outward reference. A guarantor in **another package of this module** is described rather than written as `<pkg>.<Ident>` — "the caller that has already taken the session lock", not the qualified name. The AXIOM is unchanged by this: the comment still states the precondition and still says who guarantees it. See [`doc-convention.md`](doc-convention.md) § DOC-4.

## Naming rules

- **No stutter.** `raid.Session`, not `raid.RaidSession`. `ledger.Post`, not `ledger.PostPosting`. The package name is part of the identifier at the call site.
- **Constructors** are `New…`, returning a concrete type; a constructor that can fail returns `(T, error)`.
- **`Must…` is the only licence to panic**, and only when its input is a compile-time constant (`MustParseTemplate` at package init). Anything reading configuration, user input or a database row returns an error.
- **Errors:** sentinel values are `ErrSomething`; error types are `SomethingError`; every wrap uses `%w`.
- **Interfaces are named for behaviour** (`Poster`, `Notifier`, `Scheduler`) and **declared by the consumer**, in the package that calls them — not shipped beside the implementation. One-method interfaces get the `-er` suffix.
- **Getters carry no `Get`**: `s.Leader()`, not `s.GetLeader()`. Setters do carry `Set`.
- **Abbreviations keep their case**: `ID`, `URL`, `HTTP`, `DB` — `chatID`, not `chatId`.
- **`ctx` is first, `error` is last.** `func (s *Store) Post(ctx context.Context, tx pgx.Tx, basis Basis, ps ...Posting) error`.
- **Domain vocabulary is the design document's vocabulary.** The design is Russian; the code is English, and the mapping is fixed once in the glossary of the package that owns the concept — `raid`, `node`, `chunk`, `prefab`, `door`, `backpack`, `corpse`, `trail`, `stamina`, `posting`, `basis`. Do not invent a second English word for a concept that already has one in the code.
