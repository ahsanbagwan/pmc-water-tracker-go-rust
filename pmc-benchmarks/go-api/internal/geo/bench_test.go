package geo

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"

	"pmc/internal/benchstub"
)

// stubBase returns the shared stub (STUB_URL, as started by bench/run.sh) or starts an in-process one.
func stubBase(b *testing.B) string {
	b.Helper()
	if u := os.Getenv("STUB_URL"); u != "" {
		return u
	}
	s := httptest.NewServer(benchstub.Handler())
	b.Cleanup(s.Close)
	return s.URL
}

// Same data in the Go, Rust and Spring benchmarks: every 10th link answers 404, the rest 200.
func makeLinks(base string, n int, delay string) []LinkIn {
	links := make([]LinkIn, n)
	for i := range links {
		code := 200
		if i%10 == 0 {
			code = 404
		}
		links[i] = LinkIn{ID: int64(i), URL: fmt.Sprintf("%s/status/%d?delay=%s", base, code, delay)}
	}
	return links
}

// BenchmarkCheckLinksLocal times the real fallback link checker against a local stub. Every run
// also verifies the results, so a failing server can never produce a fast-but-meaningless number.
func BenchmarkCheckLinksLocal(b *testing.B) {
	base := stubBase(b)
	ctx := context.Background()
	for _, delay := range []string{"0s", "5ms"} {
		for _, conc := range []int{10, 50, 100} {
			b.Run(fmt.Sprintf("delay=%s/conc=%d", delay, conc), func(b *testing.B) {
				links := makeLinks(base, 1000, delay)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					res := CheckLinksLocal(ctx, links, conc)
					for _, r := range res {
						if r.Healthy != (r.ID%10 != 0) {
							b.Fatalf("link %d: healthy=%v (is the stub reachable?)", r.ID, r.Healthy)
						}
					}
				}
				b.ReportMetric(float64(len(links))*float64(b.N)/b.Elapsed().Seconds(), "links/s")
			})
		}
	}
}

// Same 1,000 points in the Go, Rust and Spring benchmarks (deterministic, scattered around Pune).
func benchPoints() []Point {
	pts := make([]Point, 1000)
	for i := range pts {
		pts[i] = Point{ID: int64(i), Lat: 18.40 + float64(i*7919%1000)/4000, Lng: 73.70 + float64(i*104729%1000)/4000}
	}
	return pts
}

func BenchmarkRankLocal(b *testing.B) {
	pts := benchPoints()
	radius := 5.0
	b.Run("all", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			RankLocal(18.5204, 73.8567, nil, pts, 0)
		}
	})
	b.Run("radius5km_limit10", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			RankLocal(18.5204, 73.8567, &radius, pts, 10)
		}
	})
}
