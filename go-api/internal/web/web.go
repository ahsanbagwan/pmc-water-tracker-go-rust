// Package web is the HTTP layer: router, security rules and the filter chain.
//
// Chain (outermost first), mirroring the Spring filter order:
//
//	recover -> correlation id -> header->log fields -> request log -> CORS -> login rate limit
//	-> chaos monkey -> mux (per-route auth + handler timing)
package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"pmc/internal/auth"
	"pmc/internal/geo"
	"pmc/internal/kv"
	"pmc/internal/logx"
	"pmc/internal/model"
	"pmc/internal/service"
	"pmc/internal/store"
	"pmc/internal/telemetry"
)

type Deps struct {
	Store       store.Store
	KV          kv.KV
	Localities  *service.Locality
	Links       *service.LinkChecker
	Ollama      *service.Ollama
	Audit       service.Auditor
	JWT         *auth.JWT
	Hasher      auth.Hasher
	Geo         geo.Service
	GeoURL      string // base URL of the Rust service, for the /mcp reverse proxy
	Hub         *Hub
	Chaos       *Chaos
	Telemetry   *telemetry.Exporter
	MDCHeaders  []string
	CORSOrigins []string // exact origins; entries like "http://localhost:*" match any port
	TrustProxy  bool
	SiteURL     string
}

type ctxKey int

const principalKey ctxKey = 1

func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{Deps: d}

	any_ := func(string) []model.Role { return nil }
	_ = any_
	adminEditor := []model.Role{model.RoleAdmin, model.RoleEditor}
	adminOnly := []model.Role{model.RoleAdmin}

	route := func(pattern string, roles []model.Role, fn http.HandlerFunc) {
		var hd http.Handler = fn
		if roles != nil {
			hd = d.require(roles, hd)
		}
		mux.Handle(pattern, timing(pattern, hd))
	}

	// public
	route("GET /healthz", nil, h.health)
	route("GET /actuator/health", nil, h.health)
	route("GET /rss.xml", nil, h.rss)
	route("GET /pune-admin-wards.geojson", nil, h.boundaries)
	route("POST /api/auth/login", nil, h.login)
	route("GET /api/localities", nil, h.listLocalities)
	route("GET /api/localities/nearby", nil, h.nearby)
	route("GET /api/localities/reports/pending", adminEditor, h.pendingReports)
	route("GET /api/localities/{id}", nil, h.getLocality)
	route("GET /api/localities/{id}/nearest-reliable", nil, h.nearestReliable)
	route("POST /api/localities/reports", nil, h.submitReport)
	route("GET /api/wards/lookup", nil, h.wardLookup)
	route("GET /api/stream/localities", nil, h.stream)

	// admin / editor
	route("POST /api/localities", adminEditor, h.createLocality)
	route("POST /api/localities/reports/{id}/approve", adminEditor, h.approve)
	route("POST /api/localities/check-source-links", adminEditor, h.checkSourceLinks)
	route("PUT /api/localities/{id}", adminEditor, h.updateLocality)
	route("PUT /api/wards/{id}/recompute", adminEditor, h.recomputeWard)

	// admin only
	route("DELETE /api/localities/{id}", adminOnly, h.deleteLocality)
	route("GET /api/users", adminOnly, h.listUsers)
	route("POST /api/users", adminOnly, h.createUser)
	route("DELETE /api/users/{id}", adminOnly, h.deleteUser)
	route("GET /api/audit-log", adminOnly, h.auditLog)
	route("GET /api/admin/features/ollama", adminOnly, h.ollamaStatus)
	route("POST /api/admin/features/ollama/enable", adminOnly, h.ollamaSet(true))
	route("POST /api/admin/features/ollama/disable", adminOnly, h.ollamaSet(false))
	if d.Chaos != nil {
		d.Chaos.Routes(route, adminOnly)
	}

	// MCP lives in the Rust service; expose it through the same public origin.
	if d.GeoURL != "" {
		if u, err := url.Parse(d.GeoURL); err == nil {
			mux.Handle("/mcp", httputil.NewSingleHostReverseProxy(u))
		}
	}

	var chain http.Handler = mux
	if d.Chaos != nil {
		chain = d.Chaos.Middleware(chain)
	}
	chain = d.rateLimitLogin(chain)
	chain = d.cors(chain)
	chain = requestLog(d.Telemetry, chain)
	chain = d.headerFields(chain)
	chain = correlationID(chain)
	return recoverMW(chain)
}

// ---- middleware -----------------------------------------------------------------------

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(c int)           { s.status = c; s.ResponseWriter.WriteHeader(c) }
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.ErrorContext(r.Context(), "unhandled exception while processing request", "panic", rec)
				writeErr(w, http.StatusInternalServerError, "An unexpected error occurred.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

const CorrelationHeader = "X-Correlation-Id"

func correlationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(CorrelationHeader))
		if id == "" {
			id = newID()
		}
		w.Header().Set(CorrelationHeader, id)
		next.ServeHTTP(w, r.WithContext(logx.With(r.Context(), "correlationId", id)))
	})
}

// headerFields copies an allowlist of request headers into log fields ("header.x-tenant-id") so
// they become filterable keys in Kibana/Loki. Allowlist only - never Authorization or Cookie.
func (d Deps) headerFields(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		for _, name := range d.MDCHeaders {
			if v := r.Header.Get(name); v != "" {
				ctx = logx.With(ctx, "header."+strings.ToLower(name), v)
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestLog(t *telemetry.Exporter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		dur := time.Since(start)
		slog.InfoContext(r.Context(), "request", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "durationMs", dur.Milliseconds())
		if t != nil {
			route := r.Pattern
			if route == "" {
				route = "UNMATCHED"
			}
			t.Record(r.Method, route, rec.status, start, dur)
		}
	})
}

// timing is the HandlerTimingInterceptor: times only the matched handler, excluding the chain.
func timing(pattern string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.DebugContext(r.Context(), "handler timing", "handler", pattern, "durationMs", time.Since(start).Milliseconds())
	})
}

func (d Deps) cors(next http.Handler) http.Handler {
	allowed := func(origin string) bool {
		for _, a := range d.CORSOrigins {
			if a == origin {
				return true
			}
			if strings.HasSuffix(a, ":*") && strings.HasPrefix(origin, strings.TrimSuffix(a, "*")) {
				return true
			}
		}
		return false
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && allowed(o) {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Expose-Headers", CorrelationHeader)
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", firstNonEmpty(r.Header.Get("Access-Control-Request-Headers"), "*"))
				w.Header().Set("Access-Control-Max-Age", "1800")
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

const (
	loginPath        = "/api/auth/login"
	maxLoginAttempts = 5
	loginWindow      = time.Minute
)

// rateLimitLogin: Redis INCR + EXPIRE shared by every replica; fails OPEN if Redis is down.
func (d Deps) rateLimitLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != loginPath {
			next.ServeHTTP(w, r)
			return
		}
		ip := d.clientIP(r)
		key := "rate-limit:login:" + ip
		n, err := d.KV.Incr(r.Context(), key)
		if err != nil {
			slog.WarnContext(r.Context(), "KV unreachable for rate limiting - failing OPEN", "error", err)
		} else {
			if n == 1 { // only the request that created the key sets the expiry
				_ = d.KV.Expire(r.Context(), key, loginWindow)
			}
			if n > maxLoginAttempts {
				slog.WarnContext(r.Context(), "rate limit exceeded for login attempts", "ip", ip)
				writeErr(w, http.StatusTooManyRequests, "Too many login attempts. Try again in a minute.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (d Deps) clientIP(r *http.Request) string {
	if d.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// require is the per-route authorisation rule. Missing/invalid token and wrong role are both 403,
// as in the Spring app (no 401 entry point was configured there).
func (d Deps) require(roles []model.Role, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			writeErr(w, http.StatusForbidden, "Access denied")
			return
		}
		user, role, err := d.JWT.Parse(strings.TrimPrefix(hdr, "Bearer "))
		if err != nil {
			writeErr(w, http.StatusForbidden, "Access denied")
			return
		}
		ok := false
		for _, want := range roles {
			if string(want) == role {
				ok = true
			}
		}
		if !ok {
			writeErr(w, http.StatusForbidden, "Access denied")
			return
		}
		ctx := auth.WithPrincipal(r.Context(), user, role)
		ctx = context.WithValue(ctx, principalKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ---- helpers ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }

func decode(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return validationError{"Malformed JSON request: " + err.Error()}
	}
	return nil
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	var nf service.NotFoundError
	var ve validationError
	switch {
	case errors.As(err, &nf):
		slog.DebugContext(r.Context(), "404", "msg", nf.Msg)
		writeErr(w, http.StatusNotFound, nf.Msg)
	case errors.As(err, &ve):
		slog.DebugContext(r.Context(), "400", "msg", ve.msg)
		writeErr(w, http.StatusBadRequest, ve.msg)
	default:
		slog.ErrorContext(r.Context(), "unhandled exception while processing request", "error", err)
		writeErr(w, http.StatusInternalServerError, "An unexpected error occurred.")
	}
}
