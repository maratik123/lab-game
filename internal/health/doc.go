// Package health exposes lab-game's whole health-metrics and canary
// surface: a Prometheus registry, one adapter per consumer-declared
// Observer interface, a pgx connection-pool collector, the process-
// identity and restart-hygiene collector, the /metrics and /readyz
// endpoints' shared server, and the two-leg Telegram canary. Every
// constructor takes a prometheus.Registerer and registers only its own
// family on it — there is no aggregate constructor and no global
// default registerer.
//
// Readiness is one definition, a ReadyFunc, shared by the /readyz HTTP
// path and the labgame_ready gauge: the composition root supplies it
// once, at NewServer/NewProcess construction, and both consumers call
// it with an internally-bounded context so neither a probe nor a scrape
// can hang on it.
//
// # Observation-field register
//
// Every field of a consumer-declared observation struct must reach either
// a metric family or a named exemption. The transport observation carries
// no exemption: every one of its fields feeds a family. The scheduler's
// two observation structs, and the update-ingest loop's two observation
// structs, carry the exemptions below. A package-level table binds this
// prose to those structs by name; a reflection-based guard
// asserts, in both directions, that the table's field set matches each
// struct's own, and that every exempt field named here also appears in
// this table.
//
//   - A task observation's BatchSize field is an alias, not an omission:
//     it is the discovery cardinality of the cycle the task came from, so
//     it repeats once per task in a batch. The claim-batch-size family
//     already takes the same number once per cycle from the loop
//     observation's own BatchSize field; exporting the task-level one as
//     well would weight the distribution by batch size.
//   - A task observation's ConsecutiveFailures field is a per-row
//     property, not a health series: a gauge of it is last-write-wins
//     across concurrently executing tasks and means nothing at scrape
//     time, and a histogram of it would double-count a row that fails
//     repeatedly. "Tasks are retrying, and by kind" is already the
//     failure-labelled counter; "this specific row is stuck" is a per-row
//     question a dead-task listing answers, not a fleet-wide one this
//     package exports.
//   - An update observation's Attempt field (its attempt ordinal) is
//     bounded only by an operator-set retry cap — a per-update property,
//     not a fleet-wide one. The retry volume it would describe is
//     already the failed- and panic-labelled outcome counts.
package health
