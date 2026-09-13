// Package maze is the deterministic generation core: seed derivation,
// the pinned draw reductions, the generation inputs, island selection,
// the per-chunk algorithm draw, the five spanning-structure algorithms,
// the extra-passage pass, border portals, the prefab hook, and the
// per-coordinate entry point Cell. Every value it yields for a
// coordinate is a pure function of the world seed, the generation
// inputs, and the coordinate itself — no clock, no unseeded random
// source, and no floating-point arithmetic anywhere on the path, so the
// same three inputs yield the same result across processes and Go
// versions.
package maze
