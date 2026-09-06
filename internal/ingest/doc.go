// Package ingest implements lab-game's whole update-ingestion front door
// (issue #22, docs/DESIGN.md §22): the long-poll loop, the kind-based
// router, the per-update transaction with its bounded retry, the
// persisted offset, the give-up record, the observation seam and the
// tg.Gate implementation the chat allowlist rides on. Its structural
// model is internal/scheduler — a Postgres-backed loop with an immutable
// registry, one transaction per attempt handed to a consumer-declared
// handler, a terminal give-up state, and an observer interface with no
// exporter.
//
// This package imports internal/tg (the client and the Gate/Call types),
// internal/config, internal/store (the ErrAlreadyPosted sentinel and the
// owner read) and internal/backoff, plus telego. internal/tg gains no
// import of internal/store and no pool: the gate is consumer-declared
// here, in the package that needs it.
//
// An update's operation_id — the idempotency key store.Post's basis
// documents require — is this package's to define: internal/store treats
// it as opaque. The grammar is "<space>:<id>", built by an unexported
// constructor so a handler cannot assemble one from raw Telegram fields
// (design D9).
//
// A Handler MUST NOT issue an outbound Bot API call whose permission
// rests on a row its own uncommitted transaction created (design D18): a
// Telegram send is not rollback-able, so a message justified by a row the
// transaction then rolls back has already reached a real person. The
// designed route for a post-commit send is the outbound notification
// queue (#43, out of this task's scope).
package ingest
