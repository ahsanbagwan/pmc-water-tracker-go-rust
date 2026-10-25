// Command api is the Go port of the PMC Water Tracker backend. The geo work (ward lookup,
// distance ranking, link checks, MCP) lives in the Rust service at GEO_URL.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pmc/internal/auth"
	"pmc/internal/cache"
	"pmc/internal/config"
	"pmc/internal/geo"
	"pmc/internal/kv"
	"pmc/internal/lock"
	"pmc/internal/logx"
	"pmc/internal/messaging"
	"pmc/internal/model"
	"pmc/internal/service"
	"pmc/internal/store"
	"pmc/internal/telemetry"
	"pmc/internal/web"
)

const defaultAdminPassword = "change-me-now"

func main() {
	cfg := config.Load(os.Getenv)
	slog.SetDefault(logx.New(cfg.LogLevel))
	if err := run(cfg); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Fail fast on unsafe production config (same guards as the Spring app).
	if cfg.IsProd() && cfg.AdminPassword == defaultAdminPassword {
		return errors.New("refusing to start: prod profile but ADMIN_PASSWORD is unset (still the dev default)")
	}
	jwt, err := auth.NewJWT(cfg.JWTPrivateKeyB64, cfg.JWTPublicKeyB64, cfg.JWTExpiry, cfg.IsProd())
	if err != nil {
		return err
	}

	st, closeStore, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeStore()
	hasher := newHasher()

	var shared kv.KV
	if cfg.RedisHost != "" {
		shared = kv.NewRedis(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword, cfg.RedisTLS)
	} else {
		if cfg.IsProd() {
			slog.Warn("REDIS_HOST is not set: using in-process state. Rate limits, cache, toggle, locks and SSE are NOT shared across replicas.")
		}
		shared = kv.NewMemory()
	}

	audit := service.Auditor{Store: st}
	geoSvc := geo.NewHTTP(cfg.GeoURL)
	ollama := &service.Ollama{KV: shared, StartupDefault: cfg.OllamaEnabled, Host: cfg.OllamaHost, Model: cfg.Model}
	notifier := messaging.Notifier{APIKey: cfg.BrevoAPIKey, From: cfg.MailFrom, To: cfg.AdminNotifyEmail}
	publisher, closeMQ := startMessaging(ctx, cfg, notifier)
	defer closeMQ()

	localities := &service.Locality{
		Store: st, Cache: cache.New(shared, "localities", 30*time.Second), Geo: geoSvc, Analyzer: ollama,
		Events: messaging.Safe{Enabled: cfg.RabbitEnabled, Inner: publisher}, Audit: audit,
	}
	node, _ := os.Hostname()
	links := &service.LinkChecker{Store: st, Geo: geoSvc, Locker: lock.New(shared, node), Audit: audit}

	if err := bootstrapAdmin(ctx, cfg, st, hasher); err != nil {
		return err
	}
	if n, err := store.SeedIfMissing(ctx, st, store.SeedLocalities); err != nil {
		return err
	} else if n > 0 {
		slog.Info("seeded localities", "count", n)
	}

	hub := web.NewHub(cfg.MaxSSE)
	tele := telemetry.New(cfg.OTLPTraces, cfg.OTLPMetrics, cfg.OTLPAuth, "pmc-water-tracker", cfg.OTLPSampling)
	var chaos *web.Chaos
	if cfg.ChaosEnabled {
		chaos = web.NewChaos(true)
		slog.Warn("CHAOS MONKEY IS ENABLED")
	}

	handler := web.New(web.Deps{
		Store: st, KV: shared, Localities: localities, Links: links, Ollama: ollama, Audit: audit, JWT: jwt,
		Hasher: hasher, Geo: geoSvc, GeoURL: cfg.GeoURL, Hub: hub, Chaos: chaos, Telemetry: tele,
		MDCHeaders: cfg.MDCHeaders, CORSOrigins: []string{"http://localhost:*", "https://pmc-water-tracker.vercel.app"},
		TrustProxy: cfg.TrustProxy, SiteURL: cfg.SiteURL,
	})

	go hub.Run(ctx, shared)
	go tele.Run(ctx)
	go links.RunWeekly(ctx)

	srv := &http.Server{
		Addr: ":" + cfg.Port, Handler: handler,
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, // no WriteTimeout: SSE streams are long-lived
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "port", cfg.Port, "profile", cfg.Profile, "redis", cfg.RedisHost != "", "geo", cfg.GeoURL)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	}
}

func bootstrapAdmin(ctx context.Context, cfg config.Config, st store.Store, h auth.Hasher) error {
	n, err := st.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	hash, err := h.Hash(cfg.AdminPassword)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(ctx, model.User{Username: cfg.AdminUsername, PasswordHash: hash, Role: model.RoleAdmin}); err != nil {
		return err
	}
	slog.Info("created initial admin user; set ADMIN_USERNAME/ADMIN_PASSWORD in production", "username", cfg.AdminUsername)
	return nil
}
