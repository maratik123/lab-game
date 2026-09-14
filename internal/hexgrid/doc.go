// Package hexgrid is the hex-world topology vocabulary: the axial cell
// coordinate, the six face directions and the opposite relation, the
// canonical face two neighbouring cells share, the chunk coordinate, and
// the hexagonal super-lattice of chunks (Lattice) — the mapping between
// a cell and the chunk holding it, a chunk's local cells, and the faces
// on a chunk's border with each of its six neighbours. It knows nothing
// about seeds, mazes, or content — every consumer that only needs a
// distance metric or a coordinate vocabulary depends on this package
// alone, never on a generator.
package hexgrid
