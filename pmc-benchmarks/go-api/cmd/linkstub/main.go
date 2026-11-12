// Command linkstub runs the benchmark stub server: go run ./cmd/linkstub -addr 127.0.0.1:9099
package main

import (
	"flag"
	"log"
	"net/http"

	"pmc/internal/benchstub"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9099", "listen address")
	flag.Parse()
	log.Printf("link stub listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, benchstub.Handler()))
}
