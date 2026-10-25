package logx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
)

func TestFieldsFollowContextIntoGoroutines(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	log := slog.New(Wrap(slog.NewJSONHandler(&lockedWriter{&buf, &mu}, nil)))

	ctx := With(With(context.Background(), "correlationId", "abc-123"), "header.x-tenant-id", "t1")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // a "worker thread": the id arrives because ctx was passed, no decorator needed
		defer wg.Done()
		log.InfoContext(ctx, "in worker")
	}()
	wg.Wait()
	log.InfoContext(context.Background(), "no context")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	var first, second map[string]any
	_ = json.Unmarshal(lines[0], &first)
	_ = json.Unmarshal(lines[1], &second)
	if first["correlationId"] != "abc-123" || first["header.x-tenant-id"] != "t1" {
		t.Fatalf("fields missing: %v", first)
	}
	if _, ok := second["correlationId"]; ok {
		t.Fatalf("leaked into unrelated context: %v", second)
	}
}

func TestWithDoesNotMutateParent(t *testing.T) {
	parent := With(context.Background(), "a", "1")
	_ = With(parent, "b", "2")
	if _, ok := Fields(parent)["b"]; ok {
		t.Fatal("parent mutated")
	}
}

func TestDetachKeepsFieldsDropsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(With(context.Background(), "correlationId", "x"))
	d := Detach(ctx)
	cancel()
	if d.Err() != nil || Fields(d)["correlationId"] != "x" {
		t.Fatal("detach wrong")
	}
}

type lockedWriter struct {
	b  *bytes.Buffer
	mu *sync.Mutex
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}
