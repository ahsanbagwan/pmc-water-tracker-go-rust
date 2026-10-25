// Package config reads the same environment variables the Spring app used, so existing
// deployment config (Render, K8s Secret, Concourse) carries over unchanged.
package config

import (
	"log/slog"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port    string
	Profile string // "dev" | "prod"  (SPRING_PROFILES_ACTIVE is honoured too)

	AdminUsername, AdminPassword string

	JWTPrivateKeyB64, JWTPublicKeyB64 string
	JWTExpiry                         time.Duration

	OllamaEnabled     bool
	OllamaHost, Model string

	MDCHeaders []string

	RedisHost, RedisPort, RedisPassword string
	RedisTLS                            bool

	RabbitEnabled                                  bool
	RabbitHost, RabbitPort, RabbitUser, RabbitPass string
	RabbitVHost                                    string
	RabbitTLS                                      bool

	BrevoAPIKey, MailFrom, AdminNotifyEmail string

	DBHost, DBPort, DBName, DBUser, DBPass string

	GeoURL string

	OTLPTraces, OTLPMetrics, OTLPAuth string
	OTLPSampling                      float64

	ChaosEnabled bool
	TrustProxy   bool
	SiteURL      string
	LogLevel     slog.Level
	MaxSSE       int
}

func (c Config) IsProd() bool { return c.Profile == "prod" }

func Load(env func(string) string) Config {
	get := func(k, def string) string {
		if v := env(k); v != "" {
			return v
		}
		return def
	}
	b := func(k string) bool { v, _ := strconv.ParseBool(env(k)); return v }

	profile := get("SPRING_PROFILES_ACTIVE", get("APP_PROFILE", "dev"))
	if strings.Contains(profile, "prod") {
		profile = "prod"
	} else {
		profile = "dev"
	}

	ms, err := strconv.ParseInt(get("JWT_EXPIRATION_MS", "43200000"), 10, 64)
	if err != nil {
		ms = 43200000
	}
	sampling, err := strconv.ParseFloat(get("OTLP_SAMPLING_PROBABILITY", "1.0"), 64)
	if err != nil {
		sampling = 1
	}
	maxSSE, _ := strconv.Atoi(get("MAX_SSE_CLIENTS", "5000"))

	var headers []string
	for _, h := range strings.Split(env("MDC_HEADERS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			headers = append(headers, h)
		}
	}
	level := slog.LevelInfo
	if err := level.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		level = slog.LevelInfo
	}

	return Config{
		Port: get("PORT", "8080"), Profile: profile,
		AdminUsername: get("ADMIN_USERNAME", "admin"), AdminPassword: get("ADMIN_PASSWORD", "change-me-now"),
		JWTPrivateKeyB64: env("JWT_PRIVATE_KEY_B64"), JWTPublicKeyB64: env("JWT_PUBLIC_KEY_B64"),
		JWTExpiry:     time.Duration(ms) * time.Millisecond,
		OllamaEnabled: b("OLLAMA_ENABLED"), OllamaHost: get("OLLAMA_HOST", "http://localhost:11434"), Model: get("OLLAMA_MODEL", "llama3.2"),
		MDCHeaders: headers,
		RedisHost:  env("REDIS_HOST"), RedisPort: get("REDIS_PORT", "6379"), RedisPassword: env("REDIS_PASSWORD"), RedisTLS: b("REDIS_SSL_ENABLED"),
		RabbitEnabled: b("RABBITMQ_ENABLED"), RabbitHost: get("RABBITMQ_HOST", "localhost"), RabbitPort: get("RABBITMQ_PORT", "5672"),
		RabbitUser: get("RABBITMQ_USERNAME", "guest"), RabbitPass: get("RABBITMQ_PASSWORD", "guest"), RabbitVHost: get("RABBITMQ_VHOST", "/"), RabbitTLS: b("RABBITMQ_SSL_ENABLED"),
		BrevoAPIKey: env("BREVO_API_KEY"), MailFrom: env("MAIL_FROM_ADDRESS"), AdminNotifyEmail: env("ADMIN_NOTIFICATION_EMAIL"),
		DBHost: env("DB_HOST"), DBPort: get("DB_PORT", "3306"), DBName: env("DB_NAME"), DBUser: env("DB_USERNAME"), DBPass: env("DB_PASSWORD"),
		GeoURL:     get("GEO_URL", "http://localhost:8090"),
		OTLPTraces: env("OTLP_TRACES_ENDPOINT"), OTLPMetrics: env("OTLP_METRICS_ENDPOINT"), OTLPAuth: env("OTLP_AUTH_HEADER"), OTLPSampling: sampling,
		ChaosEnabled: b("CHAOS_ENABLED"), TrustProxy: b("TRUST_PROXY"),
		SiteURL: get("SITE_URL", "https://pmc-water-tracker.vercel.app"), LogLevel: level, MaxSSE: maxSSE,
	}
}
