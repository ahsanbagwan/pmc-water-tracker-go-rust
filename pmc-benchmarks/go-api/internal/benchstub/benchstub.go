// Package benchstub is a tiny local HTTP server for benchmarks. The Go, Rust and Spring link-check
// benchmarks all hit the SAME stub, so none of them depends on the public internet and the
// comparison is fair.
//
//	GET /status/{code}?delay=5ms   -> responds with that status after the delay (default 200, no delay)
package benchstub

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status/", func(w http.ResponseWriter, r *http.Request) {
		code, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/status/"))
		if err != nil || code < 100 || code > 599 {
			code = 400
		}
		if d, err := time.ParseDuration(r.URL.Query().Get("delay")); err == nil && d > 0 {
			time.Sleep(d)
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
