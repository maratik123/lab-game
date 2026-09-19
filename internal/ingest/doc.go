// Package ingest implements lab-game's whole update-ingestion front door:
// the long-poll loop, the kind-based
// router, the per-update transaction with its bounded retry, the
// persisted offset, the give-up record, the observation seam and the
// Gate implementation the chat allowlist and the bot's own presence ride
// on. Its structural
// model is this module's task scheduler — a Postgres-backed loop with an
// immutable registry, one transaction per attempt handed to a
// consumer-declared handler, a terminal give-up state, and an observer
// interface with no exporter.
//
// This package imports the Telegram client (for the client and the
// Gate/Call types), the configuration package, the ledger package (the
// ErrAlreadyPosted sentinel and the owner read) and the shared backoff
// package, plus telego. The Telegram client gains no import of the
// ledger package and no pool: the gate is consumer-declared here, in the
// package that needs it.
//
// An update's operation_id — the idempotency key the ledger's Post
// requires for its basis documents — is this package's to define: the
// ledger package treats it as opaque. The grammar is "<space>:<id>",
// built by an unexported constructor so a handler cannot assemble one
// from raw Telegram fields.
//
// A Handler MUST NOT issue an outbound Bot API call whose permission
// rests on a row its own uncommitted transaction created: a
// Telegram send is not rollback-able, so a message justified by a row the
// transaction then rolls back has already reached a real person. The
// designed route for a post-commit send is the outbound notification
// queue, out of this package's scope.
package ingest
