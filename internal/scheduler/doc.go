// Package scheduler implements a Postgres-backed task scheduler: one
// worker over the scheduled_task table, claiming due rows with
// FOR NO KEY UPDATE ... SKIP LOCKED and executing each in its own
// transaction. A task type is a Go registry entry,
// not a schema value — the database stores it as text and validates
// nothing about it, because the registry is the sole authority on which
// types exist and how they run.
//
// Every persisted instant is read from the database, never from
// time.Now: due-ness, the execution instant and the settlement instant
// are all server-supplied, so backoff and cadence are pure functions of
// instants the database returned. Go time survives in this package only
// as intervals — the poll interval, the per-task execution deadline, and
// the loop's observed duration — none of which is ever compared against
// a persisted instant.
//
// This package never imports the ledger package: it moves no balance of
// its own, and the code that posts a mechanic's effects is that
// mechanic's own handler, given the worker's own *pgx.Tx.
//
// A scheduled task's payload is a data contract — the API-stability
// carve-out for live data: a payload key, once shipped, is added to or left
// alone, never renamed or repurposed, because the payload of an
// already-scheduled row must still decode after every future deploy.
package scheduler
