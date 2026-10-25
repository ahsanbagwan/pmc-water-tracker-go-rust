// Package geo talks to the Rust geo service and carries the local fallbacks, so a geo-service
// outage degrades features instead of breaking the API (same philosophy as the rest of the app).
package geo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"pmc/internal/logx"
)

type Point struct {
	ID  int64   `json:"id"`
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Ranked struct {
	ID         int64   `json:"id"`
	DistanceKm float64 `json:"distanceKm"`
}

type WardResult struct {
	Ward            string `json:"ward"`
	Found           bool   `json:"found"`
	LoadedWardCount int    `json:"loadedWardCount"`
}

type LinkIn struct {
	ID  int64  `json:"id"`
	URL string `json:"url"`
}

type LinkOut struct {
	ID      int64 `json:"id"`
	Healthy bool  `json:"healthy"`
}

type Service interface {
	Nearby(ctx context.Context, lat, lng float64, radiusKm *float64, pts []Point, limit int) []Ranked
	Ward(ctx context.Context, lat, lng float64) WardResult
	CheckLinks(ctx context.Context, links []LinkIn) []LinkOut
	Boundaries(ctx context.Context) ([]byte, bool)
}

const earthRadiusKm = 6371.0

func DistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// RankLocal is the in-process fallback with the same semantics as the Rust endpoint.
func RankLocal(lat, lng float64, radiusKm *float64, pts []Point, limit int) []Ranked {
	out := make([]Ranked, 0, len(pts))
	for _, p := range pts {
		d := DistanceKm(lat, lng, p.Lat, p.Lng)
		if radiusKm == nil || d <= *radiusKm {
			out = append(out, Ranked{p.ID, d})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DistanceKm < out[j].DistanceKm })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

type HTTP struct {
	base string
	hc   *http.Client
}

func NewHTTP(base string) *HTTP {
	return &HTTP{base: base, hc: &http.Client{Timeout: 15 * time.Second}}
}

func (h *HTTP) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// carry the correlation id across the service boundary
	if cid := logx.Fields(ctx)["correlationId"]; cid != "" {
		req.Header.Set("X-Correlation-Id", cid)
	}
	resp, err := h.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("geo service returned %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if b, ok := out.(*[]byte); ok {
		*b, err = io.ReadAll(resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (h *HTTP) Nearby(ctx context.Context, lat, lng float64, radiusKm *float64, pts []Point, limit int) []Ranked {
	req := map[string]any{"lat": lat, "lng": lng, "points": pts}
	if radiusKm != nil {
		req["radiusKm"] = *radiusKm
	}
	if limit > 0 {
		req["limit"] = limit
	}
	var out []Ranked
	if err := h.do(ctx, http.MethodPost, "/geo/nearby", req, &out); err != nil {
		slog.WarnContext(ctx, "geo service unavailable, ranking locally", "error", err)
		return RankLocal(lat, lng, radiusKm, pts, limit)
	}
	return out
}

func (h *HTTP) Ward(ctx context.Context, lat, lng float64) WardResult {
	var out WardResult
	q := url.Values{"lat": {strconv.FormatFloat(lat, 'f', -1, 64)}, "lng": {strconv.FormatFloat(lng, 'f', -1, 64)}}
	if err := h.do(ctx, http.MethodGet, "/geo/ward?"+q.Encode(), nil, &out); err != nil {
		slog.WarnContext(ctx, "geo service unavailable, ward lookup disabled", "error", err)
		return WardResult{}
	}
	return out
}

func (h *HTTP) CheckLinks(ctx context.Context, links []LinkIn) []LinkOut {
	var out []LinkOut
	if err := h.do(ctx, http.MethodPost, "/links/check", map[string]any{"links": links}, &out); err != nil {
		slog.WarnContext(ctx, "geo service unavailable, checking links locally", "error", err)
		return CheckLinksLocal(ctx, links, 10)
	}
	return out
}

func (h *HTTP) Boundaries(ctx context.Context) ([]byte, bool) {
	var b []byte
	if err := h.do(ctx, http.MethodGet, "/geo/boundaries.geojson", nil, &b); err != nil {
		return nil, false
	}
	return b, true
}

// CheckLinksLocal: fallback with the same rules (8s timeout, redirects followed, healthy < 400).
func CheckLinksLocal(ctx context.Context, links []LinkIn, concurrency int) []LinkOut {
	hc := &http.Client{Timeout: 8 * time.Second}
	out := make([]LinkOut, len(links))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, l := range links {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			healthy := false
			if req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL, nil); err == nil {
				if resp, err := hc.Do(req); err == nil {
					healthy = resp.StatusCode < 400
					resp.Body.Close()
				}
			}
			out[i] = LinkOut{l.ID, healthy}
		}()
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
