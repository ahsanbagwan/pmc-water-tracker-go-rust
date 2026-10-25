// Package lock is a ShedLock equivalent on the shared KV (SET NX PX): of N replicas hitting the
// same schedule tick, exactly one runs the job.
package lock

import (
	"context"
	"log/slog"
	"time"

	"pmc/internal/kv"
)

type Locker struct {
	kv   kv.KV
	node string
}

func New(k kv.KV, nodeID string) *Locker { return &Locker{kv: k, node: nodeID} }

// TryRun runs fn if the lock is free. atMost is the safety ceiling if this node dies mid-run;
// atLeast keeps the lock held after a fast run so clock skew can't let another node re-run it.
func (l *Locker) TryRun(ctx context.Context, name string, atMost, atLeast time.Duration, fn func()) bool {
	key := "lock:" + name
	ok, err := l.kv.SetNX(ctx, key, l.node, atMost)
	if err != nil {
		slog.WarnContext(ctx, "lock store unreachable - running without a lock (a duplicate run is harmless here)", "job", name, "error", err)
		fn()
		return true
	}
	if !ok {
		slog.InfoContext(ctx, "job already running/locked elsewhere, skipping", "job", name)
		return false
	}
	start := time.Now()
	defer func() {
		if rest := atLeast - time.Since(start); rest > 0 {
			_ = l.kv.Expire(ctx, key, rest)
		} else {
			_ = l.kv.Del(ctx, key)
		}
	}()
	fn()
	return true
}
