//go:build !prod

package main

import (
	"context"
	"log/slog"

	"pmc/internal/auth"
	"pmc/internal/config"
	"pmc/internal/messaging"
	"pmc/internal/store"
)

// Default build: zero third-party dependencies. In-memory store (the old `dev` profile's H2),
// stdlib PBKDF2 hashing, no broker. Build with `-tags prod` for MySQL + bcrypt + RabbitMQ.

func openStore(_ context.Context, cfg config.Config) (store.Store, func(), error) {
	if cfg.IsProd() {
		slog.Warn("built WITHOUT -tags prod: using the in-memory store, data is lost on restart")
	}
	return store.NewMemory(), func() {}, nil
}

func newHasher() auth.Hasher { return auth.PBKDF2{} }

func startMessaging(_ context.Context, cfg config.Config, _ messaging.Notifier) (messaging.Publisher, func()) {
	if cfg.RabbitEnabled {
		slog.Warn("RABBITMQ_ENABLED=true but this binary was built without -tags prod; events are dropped")
	}
	return messaging.Noop{}, func() {}
}
