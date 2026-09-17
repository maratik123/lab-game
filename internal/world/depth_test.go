package world

import (
	"testing"

	"github.com/maratik123/lab-game/internal/gate"
	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
)

func TestDepth_GateCentreIsZero(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 200)
	chat := createOwner(t, pool, store.OwnerChat, -5001)

	gateCh, err := m.ActivateChat(ctx, chat, nil)
	if err != nil {
		t.Fatalf("ActivateChat: %v", err)
	}
	centre := m.Lattice().Center(gateCh)

	depth, found, err := m.Depth(ctx, pool, centre)
	if err != nil {
		t.Fatalf("Depth: %v", err)
	}
	if !found {
		t.Fatal("Depth: found = false, want true")
	}
	if depth != 0 {
		t.Fatalf("Depth(gate centre) = %d, want 0", depth)
	}

	probe := hexgrid.Coord{Q: centre.Q + 3, R: centre.R - 1}
	want := hexgrid.Distance(probe, centre)
	got, found, err := m.Depth(ctx, pool, probe)
	if err != nil {
		t.Fatalf("Depth: %v", err)
	}
	if !found {
		t.Fatal("Depth: found = false, want true")
	}
	if got != want {
		t.Fatalf("Depth(probe) = %d, want %d (hexgrid.Distance)", got, want)
	}
}

func TestDepth_NoGate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 201)

	_, found, err := m.Depth(ctx, pool, hexgrid.Coord{})
	if err != nil {
		t.Fatalf("Depth: %v", err)
	}
	if found {
		t.Fatal("Depth on a maze with no gate: found = true, want false")
	}
}

func TestDepth_FabricChunksDoNotReachIt(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 202)
	chat := createOwner(t, pool, store.OwnerChat, -5002)

	if _, err := m.ActivateChat(ctx, chat, nil); err != nil {
		t.Fatalf("ActivateChat: %v", err)
	}
	probe := hexgrid.Coord{Q: 30, R: 30}

	before, foundBefore, err := m.Depth(ctx, pool, probe)
	if err != nil {
		t.Fatalf("Depth (before): %v", err)
	}
	if !foundBefore {
		t.Fatal("Depth (before): found = false, want true")
	}

	// Create fabric chunks nearer to probe than the gate is.
	nearCh, _ := m.Lattice().Locate(probe)
	for _, cell := range []hexgrid.Coord{m.Lattice().Center(nearCh), m.Lattice().Center(nearCh.Neighbor(hexgrid.DirE))} {
		if _, err := m.EnsureChunkAt(ctx, cell, Actor{}); err != nil {
			t.Fatalf("EnsureChunkAt: %v", err)
		}
	}

	after, foundAfter, err := m.Depth(ctx, pool, probe)
	if err != nil {
		t.Fatalf("Depth (after): %v", err)
	}
	if !foundAfter || after != before {
		t.Fatalf("Depth changed after creating fabric chunks near probe: before=%d after=%d (found=%v)", before, after, foundAfter)
	}
}

// TestDepth_LaterGateLowersEarlierDepth drives both gates through
// ActivateChat rather than inserting rows by hand, so the case covers
// the wiring. The second gate's own chunk is predicted through the
// placement package's own scan directly — the first activation of a
// fresh maze always takes the centre chunk, so the second is a
// deterministic function of that alone — and the probe is that
// predicted chunk's own centre cell, so "before" (one gate) is strictly
// positive and "after" (two gates) is exactly zero, with no dependence
// on the spiral's actual geometry.
func TestDepth_LaterGateLowersEarlierDepth(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 203)
	chatA := createOwner(t, pool, store.OwnerChat, -5003)

	gateA, err := m.ActivateChat(ctx, chatA, nil)
	if err != nil {
		t.Fatalf("ActivateChat(A): %v", err)
	}

	predictedB, err := gate.Next([]hexgrid.Chunk{gateA}, []hexgrid.Chunk{gateA}, m.gateSpacing)
	if err != nil {
		t.Fatalf("gate.Next: %v", err)
	}
	probe := m.Lattice().Center(predictedB)

	before, found, err := m.Depth(ctx, pool, probe)
	if err != nil {
		t.Fatalf("Depth (before): %v", err)
	}
	if !found {
		t.Fatal("Depth (before): found = false, want true")
	}
	if before <= 0 {
		t.Fatalf("Depth (before, one gate at the centre chunk) = %d, want strictly positive", before)
	}

	chatB := createOwner(t, pool, store.OwnerChat, -5004)
	gateB, err := m.ActivateChat(ctx, chatB, nil)
	if err != nil {
		t.Fatalf("ActivateChat(B): %v", err)
	}
	if gateB != predictedB {
		t.Fatalf("ActivateChat(B) chose %v, want gate.Next's own prediction %v", gateB, predictedB)
	}

	after, found, err := m.Depth(ctx, pool, probe)
	if err != nil {
		t.Fatalf("Depth (after): %v", err)
	}
	if !found {
		t.Fatal("Depth (after): found = false, want true")
	}
	if after != 0 {
		t.Fatalf("Depth (after, probe at the second gate's own centre) = %d, want 0", after)
	}
	if after >= before {
		t.Fatalf("Depth after a nearer gate = %d, want strictly less than %d", after, before)
	}
}
