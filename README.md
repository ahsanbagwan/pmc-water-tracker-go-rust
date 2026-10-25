# PMC Water Tracker - Go + Rust backend

The Spring Boot backend rewritten as **two services**. The React frontend is unchanged and talks to the
same `/api/...` contract.

```
 browser ─► Go API :8080 ──HTTP/JSON──► Rust geo service :8090
              │  auth, REST, cache,        ward point-in-polygon, nearby ranking,
              │  rate limit, SSE, audit,   concurrent source-link checks,
              │  Ollama, RabbitMQ, OTLP    MCP server (/mcp, reached through the Go API)
              ▼
        MySQL (same tables as the Spring app)  +  Redis (shared state across replicas)
```

| Spring piece | Now |
|---|---|
| Controllers, Security, JWT RS256, audit aspect | Go `internal/web`, `internal/auth`, `internal/service/audit.go` (JWT is stdlib RS256, token-compatible; same `JWT_*_B64` env vars) |
| `@Cacheable` + Redis | `internal/cache` (generation counter: evicting on one pod is visible to all) |
| Redis rate limiter, Ollama toggle | same Redis keys (`rate-limit:login:<ip>`, `feature-toggle:ollama-enabled`) |
| ShedLock | `internal/lock` (`SET NX PX` on Redis) |
| MDC + `MdcTaskDecorator` | `context.Context` log fields (`internal/logx`): the correlation id travels with the work, no decorator needed - pass `ctx` into goroutines |
| `WardBoundaryService`, `GeoUtils`, `NearbyReportFinder` math | Rust `rust-geo/src/geo.rs` |
| `SourceLinkHealthChecker` thread pool | Rust `links.rs` (tokio + bounded concurrency); Go stores the results |
| MCP server (Java SDK) | Rust `mcp.rs`, JSON-RPC over HTTP |
| RabbitMQ publisher/listener, Brevo mail | `internal/messaging` + `cmd/api/adapters_prod.go` |
| Micrometer/OTLP | `internal/telemetry` (OTLP/HTTP JSON; per-route request counts incl. errors) |
| Chaos Monkey | `internal/web/chaos.go` (`CHAOS_ENABLED=true`, admin-only `/actuator/chaosmonkey/*`) |
| *new* | `GET /api/stream/localities` Server-Sent Events, fanned out through Redis pub/sub to every replica |

The old code is kept in `legacy-spring-backend/` for side-by-side comparison.

## Run it

```bash
# 1. Rust geo service
cd rust-geo && cargo run --release                 # :8090   (optional: WARD_GEOJSON=path/to/pune-admin-wards.geojson)

# 2. Go API (default build = stdlib only, in-memory store; zero setup)
cd go-api && go run ./cmd/api                      # :8080   admin / change-me-now in dev
```

Production build (MySQL + bcrypt + RabbitMQ adapters): `cd go-api && go mod tidy && go build -tags prod ./cmd/api`.
Environment variables are the same names the Spring app used (`DB_HOST`, `REDIS_HOST`, `ADMIN_PASSWORD`,
`JWT_PRIVATE_KEY_B64`, `OLLAMA_ENABLED`, `RABBITMQ_*`, `BREVO_API_KEY`, `OTLP_*`, `MDC_HEADERS` ...) plus
`GEO_URL` (default `http://localhost:8090`), `TRUST_PROXY`, `CHAOS_ENABLED`, `MAX_SSE_CLIENTS`, `LOG_LEVEL`.
`SPRING_PROFILES_ACTIVE=prod` still switches on the production guards (refuses to start without a real admin
password and JWT keypair). `docker compose up --build` runs everything with MySQL and Redis.

Tests: `cd go-api && go test -race ./...` and `cd rust-geo && cargo test`.

## What was and was not verified (be aware before deploying)

**Verified here (automated + live):** all Rust code (10 tests); all Go code in the default build (`go vet`, `go test -race`,
46 tests); both binaries run together and the endpoints were exercised with curl: listing/filtering/search,
nearby ranking through Rust, JWT login, 401/403/429 rules, citizen report -> approve, audit log, RSS,
cache invalidation after a write, SSE push after a write, ward lookup/recompute, MCP through the Go proxy,
link checker, correlation id appearing in both services' logs.

**Written but NOT compiled or run in the authoring sandbox** (its network policy blocked `proxy.golang.org`, so
third-party Go modules could not be downloaded):
- `go-api/cmd/api/adapters_prod.go` (MySQL driver import, bcrypt, RabbitMQ via amqp091-go)
- `go-api/internal/store/sql.go` compiles, but has never run against a real MySQL (column mapping follows
  Hibernate's naming so it should work on your existing Aiven tables; BIT(1) handling is the thing to watch)
- `docker-compose.yml` and the Dockerfiles, the K8s manifest edits, and the CI workflow
- Redis: the client is hand-written RESP and was tested against a fake server, not a real Redis
First build with `-tags prod` needs `go mod tidy` to create `go.sum`.

## Behaviour differences from the Spring app (deliberate)

- Wrong login / duplicate username / unknown user id return **401 / 409 / 404** with `{"error": ...}`. The Spring
  `@ExceptionHandler(Exception.class)` was swallowing `ResponseStatusException` and returning 500.
- Bad enum or missing numeric query params return **400** (Spring returned 500 for the same reason).
- `GET /api/localities/reports/pending` now needs ADMIN/EDITOR (it was public through the `GET /api/localities/**` rule,
  exposing unverified citizen reports). The frontend does not call it.
- Anonymous and wrong-role requests return 403 with a JSON body (status code unchanged from Spring).
- Report approval/update read from the database, not the 30s cache, so they cannot act on a stale copy.
- Login rate limiting uses `X-Forwarded-For` only when `TRUST_PROXY=true`; otherwise the socket address (as before).
- Seed data: 39 localities, generated from the Java seed list.

## Not ported 1:1

Spring Boot Actuator (`/actuator/health` and `/healthz` exist; `/metrics`, `/prometheus` do not - metrics go out over OTLP),
the H2 console, `PropertyPrecedenceDebugger`/`BeanInitializationTimingPostProcessor` (Spring-specific teaching code),
and the Sonar/JaCoCo workflow (still targets the Java code in `legacy-spring-backend/`).
`pune-admin-wards.geojson` was never in the repo; ward lookup stays disabled until you place the file for the Rust service.
