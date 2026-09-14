// Package maze is the deterministic generation core: seed derivation,
// the pinned draw reductions, the generation inputs, island selection,
// the per-chunk algorithm draw, the five spanning-structure algorithms,
// the extra-passage pass, border portals, chunk types, and the
// chunk-level entry point Generate, whose result is a Map. Every value
// it yields for a chunk is a pure function of the world seed, the
// generation inputs, the chunk coordinate, its type, and the neighbour
// maps it is generated against — no clock, no unseeded random source,
// and no floating-point arithmetic anywhere on the path, so the same
// inputs yield the same result across processes and Go versions.
package maze
