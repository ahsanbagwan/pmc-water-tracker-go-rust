package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"pmc/internal/kv"
)

func TestLoadCachesAndEvictAllInvalidates(t *testing.T) {
	c := New(kv.NewMemory(), "localities", 30*time.Second)
	ctx := context.Background()
	calls := 0
	loader := func() ([]string, error) { calls++; return []string{"a"}, nil }

	for i := 0; i < 3; i++ {
		if v, _ := Load(ctx, c, "list", loader); len(v) != 1 {
			t.Fatal(v)
		}
	}
	if calls != 1 {
		t.Fatalf("loader ran %d times, want 1", calls)
	}
	c.EvictAll(ctx)
	_, _ = Load(ctx, c, "list", loader)
	if calls != 2 {
		t.Fatalf("after evict loader ran %d times, want 2", calls)
	}
}

func TestLoaderErrorsAreNotCached(t *testing.T) {
	c := New(kv.NewMemory(), "x", time.Minute)
	n := 0
	f := func() (int, error) { n++; return 0, errors.New("boom") }
	_, _ = Load(context.Background(), c, "k", f)
	_, _ = Load(context.Background(), c, "k", f)
	if n != 2 {
		t.Fatal("error was cached")
	}
}

func TestEvictVisibleAcrossReplicasSharingKV(t *testing.T) {
	shared := kv.NewMemory()
	podA, podB := New(shared, "l", time.Minute), New(shared, "l", time.Minute)
	ctx := context.Background()
	v := 1
	load := func() (int, error) { return v, nil }
	_, _ = Load(ctx, podA, "k", load)
	v = 2
	if got, _ := Load(ctx, podB, "k", load); got != 1 {
		t.Fatalf("pod B should see pod A's cached value, got %d", got)
	}
	podA.EvictAll(ctx) // admin update handled by pod A
	if got, _ := Load(ctx, podB, "k", load); got != 2 {
		t.Fatalf("pod B still stale after pod A evicted: %d", got)
	}
}
