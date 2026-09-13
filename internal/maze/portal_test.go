package maze

import (
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

var refDims16 = hexgrid.Dims{Cols: 16, Rows: 16}

func TestBorderCandidates_AgreeFromEitherSide(t *testing.T) {
	t.Parallel()
	a := hexgrid.Chunk{Q: 0, R: 0}
	b := hexgrid.Chunk{Q: 1, R: 0}
	fromA := borderCandidates(refDims16, a, b)
	fromB := borderCandidates(refDims16, b, a)
	if len(fromA) != len(fromB) {
		t.Fatalf("borderCandidates(a,b) has %d entries, borderCandidates(b,a) has %d", len(fromA), len(fromB))
	}
	for i := range fromA {
		if fromA[i] != fromB[i] {
			t.Errorf("entry %d differs: %v vs %v", i, fromA[i], fromB[i])
		}
	}
}

func TestBorderCandidates_DiagonalBorderHasExactlyOneCandidateAtEveryDimension(t *testing.T) {
	t.Parallel()
	dimsList := []hexgrid.Dims{{Cols: 16, Rows: 16}, {Cols: 4, Rows: 4}, {Cols: 8, Rows: 20}}
	for _, d := range dimsList {
		origin := hexgrid.Chunk{Q: 0, R: 0}
		diag := hexgrid.Chunk{Q: 1, R: -1}
		candidates := borderCandidates(d, origin, diag)
		if len(candidates) != 1 {
			t.Errorf("dims %+v: diagonal border (0,0)-(1,-1) has %d candidates, want exactly 1", d, len(candidates))
		}
		mirror := hexgrid.Chunk{Q: -1, R: 1}
		mirrorCandidates := borderCandidates(d, origin, mirror)
		if len(mirrorCandidates) != 1 {
			t.Errorf("dims %+v: diagonal border (0,0)-(-1,1) has %d candidates, want exactly 1", d, len(mirrorCandidates))
		}
	}
}

func TestBorderCandidates_ManyCandidateBordersHaveSeveral(t *testing.T) {
	t.Parallel()
	origin := hexgrid.Chunk{Q: 0, R: 0}
	east := hexgrid.Chunk{Q: 1, R: 0}
	candidates := borderCandidates(refDims16, origin, east)
	if len(candidates) < 2 {
		t.Errorf("east border has %d candidates, want several", len(candidates))
	}
}

func TestSelectPortals_OneOrTwoCappedByCandidates(t *testing.T) {
	t.Parallel()
	origin := hexgrid.Chunk{Q: 0, R: 0}
	east := hexgrid.Chunk{Q: 1, R: 0}
	candidates := borderCandidates(refDims16, origin, east)

	sawOne, sawTwo := false, false
	for seedByte := range 50 {
		s := newStream([32]byte{byte(seedByte), 9})
		portals := selectPortals(candidates, s)
		if len(portals) < 1 || len(portals) > 2 {
			t.Fatalf("seed %d: portal count = %d, want 1 or 2", seedByte, len(portals))
		}
		for f := range portals {
			found := false
			for _, c := range candidates {
				if c == f {
					found = true
				}
			}
			if !found {
				t.Errorf("seed %d: portal %v is not among the candidates", seedByte, f)
			}
		}
		if len(portals) == 1 {
			sawOne = true
		}
		if len(portals) == 2 {
			sawTwo = true
		}
	}
	if !sawOne || !sawTwo {
		t.Errorf("over 50 seeds, sawOne=%v sawTwo=%v, want both to occur", sawOne, sawTwo)
	}
}

func TestSelectPortals_DiagonalBorderAlwaysExactlyOne(t *testing.T) {
	t.Parallel()
	origin := hexgrid.Chunk{Q: 0, R: 0}
	diag := hexgrid.Chunk{Q: 1, R: -1}
	candidates := borderCandidates(refDims16, origin, diag)
	for seedByte := range 20 {
		s := newStream([32]byte{byte(seedByte), 3, 3})
		portals := selectPortals(candidates, s)
		if len(portals) != 1 {
			t.Errorf("seed %d: diagonal-border portal count = %d, want exactly 1", seedByte, len(portals))
		}
	}
}

func TestSelectPortals_EmptyCandidatesYieldsNoPortal(t *testing.T) {
	t.Parallel()
	if portals := selectPortals(nil, newStream([32]byte{1})); len(portals) != 0 {
		t.Errorf("selectPortals(nil, _) = %v, want none", portals)
	}
}

func TestSelectPortals_ReproducibleFromTheSameKey(t *testing.T) {
	t.Parallel()
	origin := hexgrid.Chunk{Q: 0, R: 0}
	east := hexgrid.Chunk{Q: 1, R: 0}
	candidates := borderCandidates(refDims16, origin, east)
	key := [32]byte{4, 4, 4}
	a := selectPortals(candidates, newStream(key))
	b := selectPortals(candidates, newStream(key))
	if len(a) != len(b) {
		t.Fatalf("portal count differs across identical keys: %d vs %d", len(a), len(b))
	}
	for f := range a {
		if !b[f] {
			t.Errorf("portal sets differ across identical keys: %v vs %v", a, b)
		}
	}
}
