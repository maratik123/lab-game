package store

import (
	"context"
	"testing"
)

// chatKnowledgeRow is one row of the chat_knowledge view.
type chatKnowledgeRow struct {
	chatID OwnerID
	mazeID int64
	q, r   int32
}

func TestChatKnowledge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	newOwner := func(kind OwnerKind) OwnerID {
		tg := nextTelegramID.Add(1)
		o, err := CreateOwner(ctx, tx, kind, &tg)
		if err != nil {
			t.Fatalf("CreateOwner(%s): %v", kind, err)
		}
		return o.ID
	}
	newMaze := func(biome string) int64 {
		var id int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO maze (biome, world_seed, season) VALUES ($1, 1, 1) RETURNING id`, biome,
		).Scan(&id); err != nil {
			t.Fatalf("insert maze: %v", err)
		}
		return id
	}
	discover := func(mazeID int64, q, r int32, playerID OwnerID) {
		if _, err := tx.Exec(ctx,
			`INSERT INTO node_discovery (maze_id, q, r, player_id) VALUES ($1, $2, $3, $4)`,
			mazeID, q, r, playerID,
		); err != nil {
			t.Fatalf("insert node_discovery: %v", err)
		}
	}
	member := func(chatID, playerID OwnerID) {
		if _, err := tx.Exec(ctx,
			`INSERT INTO chat_membership (chat_id, player_id) VALUES ($1, $2)`, chatID, playerID,
		); err != nil {
			t.Fatalf("insert chat_membership: %v", err)
		}
	}
	knowledgeOf := func(chatID OwnerID) []chatKnowledgeRow {
		rows, err := tx.Query(ctx,
			`SELECT chat_id, maze_id, q, r FROM chat_knowledge WHERE chat_id = $1 ORDER BY maze_id, q, r`, chatID)
		if err != nil {
			t.Fatalf("query chat_knowledge: %v", err)
		}
		var got []chatKnowledgeRow
		for rows.Next() {
			var r chatKnowledgeRow
			if err := rows.Scan(&r.chatID, &r.mazeID, &r.q, &r.r); err != nil {
				t.Fatalf("scan chat_knowledge row: %v", err)
			}
			got = append(got, r)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		return got
	}

	maze := newMaze("chat-knowledge-fixture")

	// (a) two members of chatA each discover a distinct cell, and the chat
	// knows both; a cell discovered by both appears once.
	chatA := newOwner(OwnerChat)
	playerX := newOwner(OwnerPlayer)
	playerY := newOwner(OwnerPlayer)
	member(chatA, playerX)
	member(chatA, playerY)
	discover(maze, 1, 0, playerX)
	discover(maze, 2, 0, playerY)
	discover(maze, 3, 0, playerX)
	discover(maze, 3, 0, playerY) // both discover the same cell

	gotA := knowledgeOf(chatA)
	wantA := []chatKnowledgeRow{
		{chatA, maze, 1, 0},
		{chatA, maze, 2, 0},
		{chatA, maze, 3, 0},
	}
	if !chatKnowledgeRowsEqual(gotA, wantA) {
		t.Fatalf("chat A knowledge = %+v, want %+v (AC10: union of members, a shared cell once)", gotA, wantA)
	}

	// (b) a player who is a member of a second chat discovers a cell there,
	// and the first chat does not know it.
	chatB := newOwner(OwnerChat)
	playerZ := newOwner(OwnerPlayer)
	member(chatB, playerZ)
	discover(maze, 9, 9, playerZ)

	gotAAfterB := knowledgeOf(chatA)
	if !chatKnowledgeRowsEqual(gotAAfterB, wantA) {
		t.Fatalf("chat A knowledge after chat B's discovery = %+v, want unchanged %+v (AC11)", gotAAfterB, wantA)
	}
	for _, r := range knowledgeOf(chatB) {
		if r.q == 1 || r.q == 2 { // any of chat A's own cells
			t.Fatalf("chat B knowledge leaked chat A's cell: %+v", r)
		}
	}

	// (c) a discovery recorded BEFORE the membership is in the chat's
	// knowledge as soon as the membership row exists.
	chatC := newOwner(OwnerChat)
	playerW := newOwner(OwnerPlayer)
	discover(maze, 5, 5, playerW) // discovered first, no membership yet
	if got := knowledgeOf(chatC); len(got) != 0 {
		t.Fatalf("chat C knowledge before membership = %+v, want empty", got)
	}
	member(chatC, playerW)
	gotC := knowledgeOf(chatC)
	wantC := []chatKnowledgeRow{{chatC, maze, 5, 5}}
	if !chatKnowledgeRowsEqual(gotC, wantC) {
		t.Fatalf("chat C knowledge after membership = %+v, want %+v (AC12 first clause)", gotC, wantC)
	}

	// (c') one player who is a member of chats D and E discovers one cell,
	// and that cell is in D's knowledge and in E's.
	chatD := newOwner(OwnerChat)
	chatE := newOwner(OwnerChat)
	playerV := newOwner(OwnerPlayer)
	member(chatD, playerV)
	member(chatE, playerV)
	discover(maze, 7, 7, playerV)
	wantDE := []chatKnowledgeRow{{0, maze, 7, 7}}
	gotD := knowledgeOf(chatD)
	wantDE[0].chatID = chatD
	if !chatKnowledgeRowsEqual(gotD, wantDE) {
		t.Fatalf("chat D knowledge = %+v, want %+v (AC12 second clause)", gotD, wantDE)
	}
	gotE := knowledgeOf(chatE)
	wantDE[0].chatID = chatE
	if !chatKnowledgeRowsEqual(gotE, wantDE) {
		t.Fatalf("chat E knowledge = %+v, want %+v (AC12 second clause)", gotE, wantDE)
	}

	// (d) a chat with no members knows nothing.
	chatEmpty := newOwner(OwnerChat)
	if got := knowledgeOf(chatEmpty); len(got) != 0 {
		t.Fatalf("empty chat knowledge = %+v, want empty", got)
	}
}

func chatKnowledgeRowsEqual(a, b []chatKnowledgeRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
