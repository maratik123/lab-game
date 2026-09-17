package world

import "errors"

// ErrRadiusMismatch is returned when a stored chunk's encoded byte
// length does not equal the maze's own lattice cell count — a world
// file whose chunk radius was edited after chunks were stored.
var ErrRadiusMismatch = errors.New("world: stored map length does not match the lattice cell count")

// ErrMazeMissing is returned when the per-maze lock statement finds no
// row for the maze a handle was opened over — the maze row was deleted
// underneath the handle.
var ErrMazeMissing = errors.New("world: maze row is missing")

// ErrSeedMismatch is returned by Open when a maze row already exists
// for the requested biome and season under a different world seed than
// the one requested: every chunk already stored was generated under
// the row's own seed, so adopting a new one silently would leave a
// world whose stored and freshly generated maps disagree.
var ErrSeedMismatch = errors.New("world: stored seed does not match the requested seed")

// ErrNotAChat is returned by ActivateChat when the given owner id does
// not name a chat.
var ErrNotAChat = errors.New("world: owner is not a chat")

// ErrNotAPlayer is returned by RecordDiscovery when the given owner id
// does not name a player.
var ErrNotAPlayer = errors.New("world: owner is not a player")

// ErrCreateBudget is returned, wrapping the context deadline error,
// when a chunk-creation transaction does not finish within Spec's
// CreateBudget — an exhausted connection pool or a long-held maze row
// lock surfacing as a bounded error instead of a hang.
var ErrCreateBudget = errors.New("world: chunk creation did not finish within its budget")
