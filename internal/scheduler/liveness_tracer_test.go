package scheduler

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// livenessWriteCounter is a pgx.QueryTracer that counts every statement
// whose SQL writes process_liveness's seen_at — the "how many
// heartbeats landed" instrument T4's Run/interval scenario needs, since
// the assertion is "writes once per interval", not a timing measurement
// on one call. Safe for concurrent use.
type livenessWriteCounter struct {
	n atomic.Int64
}

func (c *livenessWriteCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "UPDATE process_liveness SET seen_at") {
		c.n.Add(1)
	}
	return ctx
}

func (c *livenessWriteCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Count returns the number of process_liveness writes observed so far.
func (c *livenessWriteCounter) Count() int64 {
	return c.n.Load()
}
