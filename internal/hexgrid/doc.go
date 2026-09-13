// Package hexgrid is the hex-world topology vocabulary: the axial cell
// coordinate, the six face directions and the opposite relation, the
// canonical face two neighbouring cells share, the chunk coordinate, the
// chunk grid's dimensions, the coordinate-to-chunk mapping, and the
// chunk-grid distance. It knows nothing about seeds, mazes, or content —
// every consumer that only needs a distance metric or a coordinate
// vocabulary depends on this package alone, never on a generator.
package hexgrid
