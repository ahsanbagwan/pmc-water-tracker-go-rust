package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExporterSendsTracesAndPerRouteMetricsWithAuth(t *testing.T) {
	var mu sync.Mutex
	got := map[string]string{}
	auth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := new(strings.Builder)
		_ = json.NewEncoder(b).Encode(json.RawMessage(mustRead(r)))
		mu.Lock()
		got[r.URL.Path], auth = b.String(), r.Header.Get("Authorization")
		mu.Unlock()
	}))
	defer srv.Close()
	e := New(srv.URL+"/otlp/v1/traces", srv.URL+"/otlp/v1/metrics", "Basic abc", "pmc", 1)
	now := time.Now()
	e.Record("GET", "GET /api/localities", 200, now, 5*time.Millisecond)
	e.Record("GET", "GET /api/localities", 200, now, 7*time.Millisecond)
	e.Record("POST", "POST /api/auth/login", 401, now, 1*time.Millisecond)
	if err := e.FlushTraces(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.FlushMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if auth != "Basic abc" {
		t.Fatalf("auth header: %q", auth)
	}
	tr, me := got["/otlp/v1/traces"], got["/otlp/v1/metrics"]
	if !strings.Contains(tr, "GET /api/localities") || !strings.Contains(tr, `"service.name"`) {
		t.Fatalf("trace payload: %s", tr)
	}
	// 2 successful calls and 1 error call, kept as separate series per route+status
	if !strings.Contains(me, `"asInt":"2"`) || !strings.Contains(me, `"asInt":"1"`) || !strings.Contains(me, "POST /api/auth/login") {
		t.Fatalf("metric payload: %s", me)
	}
}

func TestNilExporterIsSafe(t *testing.T) {
	var e *Exporter
	e.Record("GET", "x", 200, time.Now(), 0)
	if New("", "", "", "s", 1) != nil {
		t.Fatal("no endpoints should disable telemetry")
	}
}

func mustRead(r *http.Request) []byte {
	b := make([]byte, 0, 1024)
	buf := make([]byte, 512)
	for {
		n, err := r.Body.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil {
			return b
		}
	}
}
