// Package logx is the Go equivalent of SLF4J's MDC: request-scoped log fields carried in the
// context.Context and added to every slog record by a wrapping handler.
//
// Unlike MDC (a ThreadLocal that is silently LOST on executor threads), a Context travels
// explicitly with the work: pass ctx into the goroutine and the correlation id comes with it.
// That removes the entire TaskDecorator problem - but only if you actually pass the ctx on.
package logx

import (
	"context"
	"log/slog"
	"os"
	"sort"
)

type ctxKey struct{}

func Fields(ctx context.Context) map[string]string {
	m, _ := ctx.Value(ctxKey{}).(map[string]string)
	return m
}

// With returns a ctx carrying one more field (copy-on-write, so parents are never mutated).
func With(ctx context.Context, key, val string) context.Context {
	old := Fields(ctx)
	m := make(map[string]string, len(old)+1)
	for k, v := range old {
		m[k] = v
	}
	m[key] = val
	return context.WithValue(ctx, ctxKey{}, m)
}

// Detach keeps the log fields but drops cancellation - for fire-and-forget background work that
// must outlive the request (the report-submitted event, the manual link check).
func Detach(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

type handler struct{ slog.Handler }

func (h handler) Handle(ctx context.Context, r slog.Record) error {
	f := Fields(ctx)
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r.AddAttrs(slog.String(k, f[k]))
	}
	return h.Handler.Handle(ctx, r)
}
func (h handler) WithAttrs(a []slog.Attr) slog.Handler { return handler{h.Handler.WithAttrs(a)} }
func (h handler) WithGroup(n string) slog.Handler      { return handler{h.Handler.WithGroup(n)} }

// New returns a JSON logger (what Logstash/Loki ingest) that adds ctx fields as top-level keys.
func New(level slog.Level) *slog.Logger {
	return slog.New(handler{slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})})
}

// Wrap decorates any slog.Handler (used by tests that capture output).
func Wrap(h slog.Handler) slog.Handler { return handler{h} }
