// Package telemetry exports request traces and metrics over OTLP/HTTP JSON (no SDK dependency).
// Works with Grafana Cloud's OTLP gateway: endpoints end in /v1/traces and /v1/metrics and the
// Authorization header is "Basic base64(instanceID:token)".
package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Exporter struct {
	TracesURL, MetricsURL, AuthHeader, Service string
	Sampling                                   float64
	HC                                         *http.Client

	mu    sync.Mutex
	spans []map[string]any
	agg   map[string]*bucket
	start time.Time
}

type bucket struct {
	count  int64
	sumMs  float64
	method string
	route  string
	status int
}

func New(tracesURL, metricsURL, auth, service string, sampling float64) *Exporter {
	if tracesURL == "" && metricsURL == "" {
		return nil
	}
	return &Exporter{TracesURL: tracesURL, MetricsURL: metricsURL, AuthHeader: auth, Service: service,
		Sampling: sampling, HC: &http.Client{Timeout: 10 * time.Second}, agg: map[string]*bucket{}, start: time.Now()}
}

func rid(n int) string { b := make([]byte, n); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func attr(k, v string) map[string]any {
	return map[string]any{"key": k, "value": map[string]any{"stringValue": v}}
}
func attrI(k string, v int) map[string]any {
	return map[string]any{"key": k, "value": map[string]any{"intValue": strconv.Itoa(v)}}
}

// Record is called once per request. The route is the mux pattern (low cardinality), which is
// what you want as the "API name" dimension on a dashboard.
func (e *Exporter) Record(method, route string, status int, start time.Time, d time.Duration) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	key := fmt.Sprintf("%s|%s|%d", method, route, status)
	b := e.agg[key]
	if b == nil {
		b = &bucket{method: method, route: route, status: status}
		e.agg[key] = b
	}
	b.count++
	b.sumMs += float64(d.Microseconds()) / 1000

	if e.TracesURL == "" || (e.Sampling < 1 && float64(time.Now().UnixNano()%1000)/1000 >= e.Sampling) {
		return
	}
	code := 1
	if status >= 500 {
		code = 2
	}
	e.spans = append(e.spans, map[string]any{
		"traceId": rid(16), "spanId": rid(8), "name": route, "kind": 2,
		"startTimeUnixNano": strconv.FormatInt(start.UnixNano(), 10),
		"endTimeUnixNano":   strconv.FormatInt(start.Add(d).UnixNano(), 10),
		"attributes":        []any{attr("http.request.method", method), attr("http.route", route), attrI("http.response.status_code", status)},
		"status":            map[string]any{"code": code},
	})
}

func (e *Exporter) resource() map[string]any {
	return map[string]any{"attributes": []any{attr("service.name", e.Service)}}
}

func (e *Exporter) post(ctx context.Context, url string, body any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.AuthHeader != "" {
		req.Header.Set("Authorization", e.AuthHeader)
	}
	resp, err := e.HC.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("OTLP endpoint returned %d", resp.StatusCode)
	}
	return nil
}

func (e *Exporter) FlushTraces(ctx context.Context) error {
	e.mu.Lock()
	spans := e.spans
	e.spans = nil
	e.mu.Unlock()
	if len(spans) == 0 || e.TracesURL == "" {
		return nil
	}
	return e.post(ctx, e.TracesURL, map[string]any{"resourceSpans": []any{map[string]any{
		"resource": e.resource(), "scopeSpans": []any{map[string]any{"spans": spans}}}}})
}

// FlushMetrics sends cumulative http_server_requests counters (count and total duration) per
// method/route/status - "total calls whether error or success", per API.
func (e *Exporter) FlushMetrics(ctx context.Context) error {
	e.mu.Lock()
	var count, dur []any
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	start := strconv.FormatInt(e.start.UnixNano(), 10)
	for _, b := range e.agg {
		attrs := []any{attr("http.request.method", b.method), attr("http.route", b.route), attrI("http.response.status_code", b.status)}
		count = append(count, map[string]any{"attributes": attrs, "startTimeUnixNano": start, "timeUnixNano": now, "asInt": strconv.FormatInt(b.count, 10)})
		dur = append(dur, map[string]any{"attributes": attrs, "startTimeUnixNano": start, "timeUnixNano": now, "asDouble": b.sumMs})
	}
	e.mu.Unlock()
	if len(count) == 0 || e.MetricsURL == "" {
		return nil
	}
	sum := func(name, unit string, pts []any) map[string]any {
		return map[string]any{"name": name, "unit": unit, "sum": map[string]any{"aggregationTemporality": 2, "isMonotonic": true, "dataPoints": pts}}
	}
	return e.post(ctx, e.MetricsURL, map[string]any{"resourceMetrics": []any{map[string]any{
		"resource": e.resource(), "scopeMetrics": []any{map[string]any{"metrics": []any{
			sum("http.server.requests", "{request}", count),
			sum("http.server.request.duration.total", "ms", dur)}}}}}})
}

// Run flushes traces every 5s and metrics every 15s until ctx ends.
func (e *Exporter) Run(ctx context.Context) {
	if e == nil {
		return
	}
	tt, mt := time.NewTicker(5*time.Second), time.NewTicker(15*time.Second)
	defer tt.Stop()
	defer mt.Stop()
	for {
		select {
		case <-tt.C:
			if err := e.FlushTraces(ctx); err != nil {
				slog.Warn("OTLP trace export failed", "error", err)
			}
		case <-mt.C:
			if err := e.FlushMetrics(ctx); err != nil {
				slog.Warn("OTLP metric export failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
