package lock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pmc/internal/kv"
)

func TestOnlyOneReplicaRunsTheJob(t *testing.T) {
	shared := kv.NewMemory()
	var runs atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			New(shared, "pod").TryRun(context.Background(), "job", time.Minute, 0, func() {
				runs.Add(1)
				time.Sleep(50 * time.Millisecond)
			})
		}()
	}
	close(start)
	wg.Wait()
	if runs.Load() != 1 {
		t.Fatalf("job ran %d times, want 1", runs.Load())
	}
}

func TestLockReleasedAfterRunAndHeldForAtLeast(t *testing.T) {
	shared := kv.NewMemory()
	l := New(shared, "pod")
	ctx := context.Background()
	if !l.TryRun(ctx, "a", time.Minute, 0, func() {}) || !l.TryRun(ctx, "a", time.Minute, 0, func() {}) {
		t.Fatal("lock not released after run")
	}
	l.TryRun(ctx, "b", time.Minute, time.Minute, func() {})
	if l.TryRun(ctx, "b", time.Minute, time.Minute, func() { t.Fatal("ran inside atLeast window") }) {
		t.Fatal("should be locked for atLeast")
	}
}
