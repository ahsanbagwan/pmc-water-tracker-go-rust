// Package kv is the shared-state boundary (Redis in production). Everything that must be
// correct across replicas - rate limits, the Ollama toggle, the cache, scheduler locks and the
// SSE fan-out - goes through this interface, never through in-process maps.
package kv

import (
	"context"
	"errors"
	"time"
)

var ErrUnavailable = errors.New("kv unavailable")

type KV interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, val string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	SetNX(ctx context.Context, key, val string, ttl time.Duration) (bool, error)
	Publish(ctx context.Context, channel, msg string) error
	// Subscribe returns a channel of messages that closes when ctx is cancelled.
	Subscribe(ctx context.Context, channel string) (<-chan string, error)
}
