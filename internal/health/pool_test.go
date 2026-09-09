package health

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// closedPortDSN returns a DSN pointing at a TCP port on the loopback
// interface that nothing is listening on: it binds a listener, reads its
// port, and closes it immediately, so building a pool against it needs
// no real server and no container.
func closedPortDSN(t *testing.T) string {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return fmt.Sprintf("postgres://user:pass@%s/db?sslmode=disable", addr)
}

// realStat builds a real *pgxpool.Pool against an unreachable DSN, with
// the given configured maximum connection count, and returns its Stat
// snapshot — a pool built this way still answers Stat with no server,
// which is what keeps this package's test binary database-free.
func realStat(t *testing.T, maxConns int32) *pgxpool.Stat {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(closedPortDSN(t))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	cfg.MaxConns = maxConns
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool.Stat()
}

func TestPoolCollector_RealPoolCarriesEveryFamily(t *testing.T) {
	t.Parallel()
	stat := realStat(t, 4)
	reg := NewRegistry()
	if _, err := NewPoolCollector(reg, func() *pgxpool.Stat { return stat }); err != nil {
		t.Fatalf("NewPoolCollector: %v", err)
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	wantFamilies := []string{
		familyPoolConns, familyPoolTotalConns, familyPoolMaxConns, familyPoolAcquires,
		familyPoolAcquireDuration, familyPoolEmptyAcquires, familyPoolEmptyAcquireWait,
		familyPoolCanceledAcquires, familyPoolNewConns, familyPoolMaxLifetimeDestroys,
		familyPoolMaxIdleDestroys,
	}
	got := map[string]bool{}
	for _, mf := range mfs {
		got[mf.GetName()] = true
	}
	for _, want := range wantFamilies {
		if !got[want] {
			t.Errorf("scrape missing family %s", want)
		}
	}

	values := observedLabelValues(mfs, labelState)
	for _, want := range []string{poolStateIdle, poolStateAcquired, poolStateConstructing} {
		if !values[want] {
			t.Errorf("labgame_pgxpool_conns missing state=%q", want)
		}
	}
}

func TestPoolCollector_NilAccessorEmitsNothing(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if _, err := NewPoolCollector(reg, func() *pgxpool.Stat { return nil }); err != nil {
		t.Fatalf("NewPoolCollector: %v", err)
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if len(mf.GetMetric()) > 0 {
			t.Errorf("family %s carries %d metrics with a nil accessor, want none", mf.GetName(), len(mf.GetMetric()))
		}
	}
}

func TestPoolCollector_AccessorCalledOncePerScrapeNotCached(t *testing.T) {
	t.Parallel()
	stat1 := realStat(t, 4)
	stat2 := realStat(t, 9)
	var calls int32
	var toggle bool
	reg := NewRegistry()
	if _, err := NewPoolCollector(reg, func() *pgxpool.Stat {
		atomic.AddInt32(&calls, 1)
		toggle = !toggle
		if toggle {
			return stat1
		}
		return stat2
	}); err != nil {
		t.Fatalf("NewPoolCollector: %v", err)
	}

	mfs1, err := reg.Gather()
	if err != nil {
		t.Fatalf("first Gather: %v", err)
	}
	mfs2, err := reg.Gather()
	if err != nil {
		t.Fatalf("second Gather: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("accessor called %d times across two scrapes, want 2 (never cached)", got)
	}
	if got, want := gatherText(t, mfs1), gatherText(t, mfs2); got == want {
		t.Error("two scrapes with a different MaxConns produced identical text — the accessor result may be cached")
	}
}
