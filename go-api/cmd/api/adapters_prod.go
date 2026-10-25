//go:build prod

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/crypto/bcrypt"

	"pmc/internal/auth"
	"pmc/internal/config"
	"pmc/internal/messaging"
	"pmc/internal/model"
	"pmc/internal/store"
)

// ---- MySQL ----------------------------------------------------------------------------

func openStore(ctx context.Context, cfg config.Config) (store.Store, func(), error) {
	if cfg.DBHost == "" {
		return nil, nil, fmt.Errorf("DB_HOST/DB_PORT/DB_NAME/DB_USERNAME/DB_PASSWORD are required with -tags prod")
	}
	// tls=skip-verify == the Spring URL's sslMode=REQUIRED (encrypted, CA not verified). Switch to
	// a registered tls config for full verification.
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?tls=skip-verify&parseTime=true&loc=UTC&charset=utf8mb4",
		url.QueryEscape(cfg.DBUser), cfg.DBPass, cfg.DBHost, cfg.DBPort, cfg.DBName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		return nil, nil, fmt.Errorf("mysql ping: %w", err)
	}
	s := &store.SQL{DB: db}
	if err := s.Migrate(ctx); err != nil {
		return nil, nil, err
	}
	return s, func() { db.Close() }, nil
}

// ---- bcrypt (compatible with hashes written by Spring's BCryptPasswordEncoder) ---------

type bcryptHasher struct{ fallback auth.PBKDF2 }

func newHasher() auth.Hasher { return bcryptHasher{} }

func (bcryptHasher) Hash(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b), err
}

func (h bcryptHasher) Verify(p, hash string) bool {
	if strings.HasPrefix(hash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
	}
	return h.fallback.Verify(p, hash)
}

// ---- RabbitMQ -------------------------------------------------------------------------

type amqpPublisher struct{ ch *amqp.Channel }

func (p amqpPublisher) Publish(ctx context.Context, ev model.ReportSubmittedEvent) error {
	body, _ := json.Marshal(ev)
	return p.ch.PublishWithContext(ctx, messaging.Exchange, messaging.ReportRoutingKey, false, false,
		amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: body})
}

func startMessaging(ctx context.Context, cfg config.Config, notifier messaging.Notifier) (messaging.Publisher, func()) {
	if !cfg.RabbitEnabled {
		return messaging.Noop{}, func() {}
	}
	scheme := "amqp"
	if cfg.RabbitTLS {
		scheme = "amqps"
	}
	u := fmt.Sprintf("%s://%s:%s@%s:%s/%s", scheme, url.QueryEscape(cfg.RabbitUser), url.QueryEscape(cfg.RabbitPass),
		cfg.RabbitHost, cfg.RabbitPort, url.PathEscape(strings.TrimPrefix(cfg.RabbitVHost, "/")))
	conn, err := amqp.Dial(u)
	if err != nil {
		slog.Warn("RabbitMQ unreachable at startup; report events disabled until restart", "error", err)
		return messaging.Noop{}, func() {}
	}
	ch, err := conn.Channel()
	if err == nil {
		err = ch.ExchangeDeclare(messaging.Exchange, "topic", true, false, false, false, nil)
	}
	if err == nil {
		_, err = ch.QueueDeclare(messaging.ReportQueue, true, false, false, false, nil)
	}
	if err == nil {
		err = ch.QueueBind(messaging.ReportQueue, messaging.ReportRoutingKey, messaging.Exchange, false, nil)
	}
	if err != nil {
		slog.Warn("RabbitMQ setup failed; report events disabled", "error", err)
		conn.Close()
		return messaging.Noop{}, func() {}
	}
	msgs, err := ch.Consume(messaging.ReportQueue, "", false, false, false, false, nil)
	if err == nil {
		go func() {
			for d := range msgs {
				var ev model.ReportSubmittedEvent
				if json.Unmarshal(d.Body, &ev) == nil {
					notifier.Handle(ctx, ev) // never errors: a mail problem must not cause redelivery loops
				}
				_ = d.Ack(false)
			}
		}()
	}
	return amqpPublisher{ch}, func() { conn.Close() }
}
