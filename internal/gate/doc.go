// Package gate places gates along the spiral order and answers the
// nearest-gate depth query the danger formulas read: Spiral is the
// deterministic placement order over the chunk super-lattice, Next
// scans that order for a chunk far enough from every existing gate, and
// Set answers how many cells a cell lies from the nearest gate. It
// imports hexgrid and the standard library alone, so a consumer that
// needs only a distance metric or the chunk vocabulary gains no
// generator.
package gate
