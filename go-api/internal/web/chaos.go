package web

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"pmc/internal/model"
)

// Chaos is a small Chaos-Monkey: with probability 1/Level per request it injects latency and/or
// a panic (-> 500 via recoverMW). Off unless CHAOS_ENABLED=true; admin-controlled at runtime.
type Chaos struct {
	mu  sync.RWMutex
	cfg ChaosConfig
}

type ChaosConfig struct {
	Enabled          bool `json:"enabled"`
	Level            int  `json:"level"` // 1-in-N
	LatencyActive    bool `json:"latencyActive"`
	LatencyRangeFrom int  `json:"latencyRangeStart"`
	LatencyRangeTo   int  `json:"latencyRangeEnd"`
	ExceptionsActive bool `json:"exceptionsActive"`
}

func NewChaos(enabled bool) *Chaos {
	return &Chaos{cfg: ChaosConfig{Enabled: enabled, Level: 5, LatencyActive: true, LatencyRangeFrom: 1000, LatencyRangeTo: 3000}}
}

func (c *Chaos) Config() ChaosConfig { c.mu.RLock(); defer c.mu.RUnlock(); return c.cfg }

func (c *Chaos) Routes(route func(string, []model.Role, http.HandlerFunc), roles []model.Role) {
	route("GET /actuator/chaosmonkey/status", roles, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, c.Config()) })
	route("POST /actuator/chaosmonkey/enable", roles, func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.cfg.Enabled = true
		c.mu.Unlock()
		writeJSON(w, 200, c.Config())
	})
	route("POST /actuator/chaosmonkey/disable", roles, func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.cfg.Enabled = false
		c.mu.Unlock()
		writeJSON(w, 200, c.Config())
	})
	route("POST /actuator/chaosmonkey/assaults", roles, func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		next := c.cfg
		if err := json.NewDecoder(r.Body).Decode(&next); err != nil || next.Level < 1 {
			c.mu.Unlock()
			writeErr(w, 400, "invalid assault settings")
			return
		}
		next.Enabled = c.cfg.Enabled
		c.cfg = next
		c.mu.Unlock()
		writeJSON(w, 200, c.Config())
	})
}

func (c *Chaos) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := c.Config()
		if !cfg.Enabled || strings.HasPrefix(r.URL.Path, "/actuator") || r.URL.Path == "/healthz" || r.URL.Path == "/api/stream/localities" {
			next.ServeHTTP(w, r)
			return
		}
		if cfg.Level < 1 || rand.IntN(cfg.Level) != 0 {
			next.ServeHTTP(w, r)
			return
		}
		if cfg.LatencyActive && cfg.LatencyRangeTo >= cfg.LatencyRangeFrom {
			d := cfg.LatencyRangeFrom
			if span := cfg.LatencyRangeTo - cfg.LatencyRangeFrom; span > 0 {
				d += rand.IntN(span)
			}
			select {
			case <-time.After(time.Duration(d) * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
		if cfg.ExceptionsActive {
			panic("Chaos Monkey - RuntimeException")
		}
		next.ServeHTTP(w, r)
	})
}
