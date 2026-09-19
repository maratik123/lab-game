package world

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/store"
)

// chunkCreatedPayload is the chunk_created event's payload: the
// dimensions the event dictionary names beyond the universal ones. SpiralIndex and Ring
// are present only for a gate chunk — a fabric chunk's payload carries
// neither key, via the pointer fields' own omitempty.
type chunkCreatedPayload struct {
	ChunkType         ChunkType     `json:"chunk_type"`
	CreationCause     CreationCause `json:"creation_cause"`
	GenerationVersion int32         `json:"generation_version"`
	Q                 int32         `json:"q"`
	R                 int32         `json:"r"`
	SpiralIndex       *int64        `json:"spiral_index,omitempty"`
	Ring              *int64        `json:"ring,omitempty"`
}

// appendChunkCreated appends one chunk_created event on tx for a chunk
// just created at ch: maze_id always, chat_id and player_id from by,
// depth null. It moves no balance, so it appends the bare event with
// no accompanying posting.
func appendChunkCreated(ctx context.Context, tx pgx.Tx, mazeID int64, by Actor, ch hexgrid.Chunk, typ ChunkType, cause CreationCause, version int32, spiralIndex, ring *int64) error {
	payload, err := json.Marshal(chunkCreatedPayload{
		ChunkType:         typ,
		CreationCause:     cause,
		GenerationVersion: version,
		Q:                 ch.Q,
		R:                 ch.R,
		SpiralIndex:       spiralIndex,
		Ring:              ring,
	})
	if err != nil {
		return fmt.Errorf("world: marshal chunk_created payload: %w", err)
	}

	ev := store.Event{
		Type:     store.EventChunkCreated,
		PlayerID: by.PlayerID,
		ChatID:   by.ChatID,
		MazeID:   &mazeID,
		Payload:  payload,
	}
	if _, err := store.AppendEvent(ctx, tx, ev); err != nil {
		return fmt.Errorf("world: append chunk_created event: %w", err)
	}
	return nil
}
