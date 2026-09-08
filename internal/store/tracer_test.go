package store

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
)

// recordedStmt is one statement observed by queryRecorder.
type recordedStmt struct {
	SQL          string
	Args         []any
	RowsAffected int64
}

// queryRecorder is a pgx.QueryTracer that records every statement issued on
// connections it is attached to (the phase/capture-order assertions and
// every "recorder empty after rec.Reset()" pre-SQL assertion elsewhere in
// this suite). Safe for concurrent use.
type queryRecorder struct {
	mu    sync.Mutex
	stmts []recordedStmt
}

type recorderIndexKey struct{}

func (r *queryRecorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	r.mu.Lock()
	idx := len(r.stmts)
	r.stmts = append(r.stmts, recordedStmt{SQL: data.SQL, Args: data.Args})
	r.mu.Unlock()
	return context.WithValue(ctx, recorderIndexKey{}, idx)
}

func (r *queryRecorder) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	idx, ok := ctx.Value(recorderIndexKey{}).(int)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if idx < len(r.stmts) {
		r.stmts[idx].RowsAffected = data.CommandTag.RowsAffected()
	}
}

// Reset discards every recorded statement.
func (r *queryRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stmts = nil
}

// Statements returns a snapshot of every statement recorded since the last
// Reset.
func (r *queryRecorder) Statements() []recordedStmt {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedStmt(nil), r.stmts...)
}
