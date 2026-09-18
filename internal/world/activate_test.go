package world

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/maratik123/lab-game/internal/gate"
	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
)

func TestActivateChat_FirstActivation(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 100)
	chat := createOwner(t, pool, store.OwnerChat, -2001)

	got, err := m.ActivateChat(ctx, chat, nil)
	if err != nil {
		t.Fatalf("ActivateChat: %v", err)
	}
	if got != (hexgrid.Chunk{}) {
		t.Fatalf("first activation's gate chunk = %v, want the centre chunk", got)
	}

	var chunkType, cause string
	var gateChatID *int64
	var spiralIndex *int64
	if err := pool.QueryRow(ctx,
		`SELECT chunk_type::text, creation_cause::text, gate_chat_id, spiral_index FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`,
		m.id, got.Q, got.R,
	).Scan(&chunkType, &cause, &gateChatID, &spiralIndex); err != nil {
		t.Fatalf("read chunk row: %v", err)
	}
	if chunkType != string(ChunkTypeGate) || cause != string(CreationCauseChatActivation) {
		t.Fatalf("stored (type, cause) = (%s, %s), want (gate, chat_activation)", chunkType, cause)
	}
	if gateChatID == nil || *gateChatID != int64(chat) {
		t.Fatalf("stored gate_chat_id = %v, want %d", gateChatID, chat)
	}
	wantSpiral := gate.SpiralIndex(got)
	if spiralIndex == nil || *spiralIndex != wantSpiral {
		t.Fatalf("stored spiral_index = %v, want %d", spiralIndex, wantSpiral)
	}
}

func TestActivateChat_MatchesGateNext(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 101)

	// Create some chunks first through EnsureChunkAt, so the world holds
	// state beyond the empty maze.
	for _, cell := range []hexgrid.Coord{{Q: 0, R: 0}, {Q: 20, R: 0}, {Q: 0, R: 20}} {
		if _, err := m.EnsureChunkAt(ctx, cell, Actor{}); err != nil {
			t.Fatalf("EnsureChunkAt(%v): %v", cell, err)
		}
	}

	var created, gates []hexgrid.Chunk
	rows, err := pool.Query(ctx, readMazeChunksSQL, m.id)
	if err != nil {
		t.Fatalf("read created chunks: %v", err)
	}
	for rows.Next() {
		var ch hexgrid.Chunk
		if err := rows.Scan(&ch.Q, &ch.R); err != nil {
			t.Fatalf("scan: %v", err)
		}
		created = append(created, ch)
	}
	rows.Close()
	want, err := gate.Next(created, gates, m.gateSpacing)
	if err != nil {
		t.Fatalf("gate.Next: %v", err)
	}

	chat := createOwner(t, pool, store.OwnerChat, -2002)
	got, err := m.ActivateChat(ctx, chat, nil)
	if err != nil {
		t.Fatalf("ActivateChat: %v", err)
	}
	if got != want {
		t.Fatalf("gate chunk = %v, want gate.Next's own answer %v", got, want)
	}

	// The fixture's three prior EnsureChunkAt calls occupy the centre
	// chunk, so the placement rule's own independently computed answer
	// here is off-centre: unlike a fresh maze's first activation, want's
	// spiral index and ring are not both zero, so a stored or emitted
	// value that was pinned to zero would be caught here instead of
	// coinciding with the true answer by construction. wantSpiral and
	// wantRing are computed from want, not from got.
	wantSpiral := gate.SpiralIndex(want)
	wantRing := hexgrid.ChunkDistance(hexgrid.Chunk{}, want)
	if wantSpiral == 0 || wantRing == 0 {
		t.Fatalf("fixture's gate chunk %v is at the centre (spiral=%d, ring=%d); this test needs an off-centre gate to discriminate a pinned zero from the true value", want, wantSpiral, wantRing)
	}

	var storedSpiral *int64
	if err := pool.QueryRow(ctx,
		`SELECT spiral_index FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`,
		m.id, got.Q, got.R,
	).Scan(&storedSpiral); err != nil {
		t.Fatalf("read stored spiral_index: %v", err)
	}
	if storedSpiral == nil || *storedSpiral != wantSpiral {
		t.Fatalf("stored spiral_index = %v, want %d", storedSpiral, wantSpiral)
	}

	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM event WHERE type = $1 AND chat_id = $2`, store.EventChunkCreated, chat).Scan(&payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	var decoded struct {
		SpiralIndex *int64 `json:"spiral_index"`
		Ring        *int64 `json:"ring"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded.SpiralIndex == nil || *decoded.SpiralIndex != wantSpiral {
		t.Fatalf("payload spiral_index = %v, want %d", decoded.SpiralIndex, wantSpiral)
	}
	if decoded.Ring == nil || *decoded.Ring != wantRing {
		t.Fatalf("payload ring = %v, want %d", decoded.Ring, wantRing)
	}
}

func TestActivateChat_Reactivation(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 102)
	chat := createOwner(t, pool, store.OwnerChat, -2003)

	first, err := m.ActivateChat(ctx, chat, nil)
	if err != nil {
		t.Fatalf("first ActivateChat: %v", err)
	}
	second, err := m.ActivateChat(ctx, chat, nil)
	if err != nil {
		t.Fatalf("second ActivateChat: %v", err)
	}
	if first != second {
		t.Fatalf("second activation's gate = %v, want the same %v", second, first)
	}

	var chunkCount, eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunk WHERE maze_id = $1 AND gate_chat_id = $2`, m.id, chat).Scan(&chunkCount); err != nil {
		t.Fatalf("count chunk: %v", err)
	}
	if chunkCount != 1 {
		t.Fatalf("gate chunk rows for chat = %d, want 1", chunkCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event WHERE type = $1 AND chat_id = $2`, store.EventChunkCreated, chat).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("chunk_created events for chat = %d, want 1", eventCount)
	}
}

func TestActivateChat_TwoChatsRespectSpacing(t *testing.T) {
	t.Parallel()

	for _, k := range []int{0, 1, 3} {
		t.Run(fmtK(k), func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			pool := storetest.Pool(t)
			m, err := Open(ctx, pool, Spec{
				Biome: "test_biome", Season: 1, Seed: int64(200 + k),
				Generation: testParams(), GateSpacing: k, CreateBudget: testCreateBudget,
			})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			chatA := createOwner(t, pool, store.OwnerChat, int64(-3000-k))
			chatB := createOwner(t, pool, store.OwnerChat, int64(-4000-k))

			gateA, err := m.ActivateChat(ctx, chatA, nil)
			if err != nil {
				t.Fatalf("ActivateChat(A): %v", err)
			}
			gateB, err := m.ActivateChat(ctx, chatB, nil)
			if err != nil {
				t.Fatalf("ActivateChat(B): %v", err)
			}
			if gateA == gateB {
				t.Fatalf("two chats took the same gate chunk %v", gateA)
			}
			if d := hexgrid.ChunkDistance(gateA, gateB); d < int64(k)+1 {
				t.Fatalf("gate distance = %d, want at least k+1 = %d", d, k+1)
			}
		})
	}
}

func fmtK(k int) string {
	return "k_" + strconv.Itoa(k)
}

func TestActivateChat_NotAChat(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 103)
	player := createOwner(t, pool, store.OwnerPlayer, 3001)

	_, err := m.ActivateChat(ctx, player, nil)
	if !errors.Is(err, ErrNotAChat) {
		t.Fatalf("err = %v, want ErrNotAChat", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunk WHERE maze_id = $1`, m.id).Scan(&count); err != nil {
		t.Fatalf("count chunk: %v", err)
	}
	if count != 0 {
		t.Fatalf("chunk rows = %d, want 0 (ActivateChat with a non-chat owner must write nothing)", count)
	}
}

func TestActivateChat_BorderAgreementOnGatePath(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 104)

	fabricCh := hexgrid.Chunk{Q: 1, R: 0}
	fabricMap, err := m.EnsureChunkAt(ctx, m.Lattice().Center(fabricCh), Actor{})
	if err != nil {
		t.Fatalf("EnsureChunkAt: %v", err)
	}

	chat := createOwner(t, pool, store.OwnerChat, -2004)
	gateCh, err := m.ActivateChat(ctx, chat, nil)
	if err != nil {
		t.Fatalf("ActivateChat: %v", err)
	}

	// No gate chunk exists yet, so the placement rule yields the centre.
	// fabricCh is on ring 1, which is chunk distance 1 from the centre,
	// so the two are neighbours and the shared border exists. The loop
	// below is required to find that border, not merely opportunistic.
	for d := hexgrid.DirE; d <= hexgrid.DirSE; d++ {
		if fabricCh.Neighbor(d) == gateCh {
			gateMap, ok, err := m.readChunk(ctx, pool, gateCh)
			if err != nil || !ok {
				t.Fatalf("readChunk(gate): ok=%v err=%v", ok, err)
			}
			borderAgrees(t, m.Lattice(), fabricMap, gateMap, d)
			return
		}
	}
	t.Fatalf("fixture's gate %v did not land adjacent to the pre-created chunk %v; the fixture no longer forces adjacency, so this case has nothing to walk", gateCh, fabricCh)
}

func TestActivateChat_Event(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 105)
	chat := createOwner(t, pool, store.OwnerChat, -2005)

	got, err := m.ActivateChat(ctx, chat, nil)
	if err != nil {
		t.Fatalf("ActivateChat: %v", err)
	}

	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM event WHERE type = $1 AND chat_id = $2`, store.EventChunkCreated, chat).Scan(&payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	var decoded struct {
		SpiralIndex *int64 `json:"spiral_index"`
		Ring        *int64 `json:"ring"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	wantSpiral := gate.SpiralIndex(got)
	wantRing := hexgrid.ChunkDistance(hexgrid.Chunk{}, got)
	if decoded.SpiralIndex == nil || *decoded.SpiralIndex != wantSpiral {
		t.Fatalf("payload spiral_index = %v, want %d", decoded.SpiralIndex, wantSpiral)
	}
	if decoded.Ring == nil || *decoded.Ring != wantRing {
		t.Fatalf("payload ring = %v, want %d", decoded.Ring, wantRing)
	}
}
