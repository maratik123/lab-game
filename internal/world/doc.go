// Package world persists mazes, their chunks, chat gates and each
// player's own cell discoveries, and answers the nearest-gate depth
// query on read. Open resolves or creates a maze row and refuses a
// seed that disagrees with a stored one. EnsureChunkAt returns a
// chunk's stored map, creating it whole under a short, per-maze locked
// transaction on a genuine miss; a created chunk's map is stored with
// the generation version it was built under and is never regenerated.
// ActivateChat allocates a chat's own gate under the same lock and the
// same transaction shape. Depth reads the maze's gate rows and answers
// the nearest one's cell distance, computed fresh on every call. A
// discovery rides the caller's own transaction and is personal to the
// player who made it. A stored map's byte layout is a data contract:
// changing it is a forward migration that rewrites every stored blob,
// never a refactor.
package world
