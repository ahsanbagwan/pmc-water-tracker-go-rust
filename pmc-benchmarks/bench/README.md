# Benchmarks: Go vs Rust vs Spring Boot

All three implementations run the **same two workloads** on the **same local stub server**, so nothing
depends on the internet (the old benchmarks called `httpbin.org`, which made the numbers meaningless).

| Workload | Go | Rust | Spring |
|---|---|---|---|
| Check 1,000 source links (every 10th answers 404), delay 0 ms and 5 ms, concurrency 10/50/100 | `go-api/internal/geo/bench_test.go` `BenchmarkCheckLinksLocal` | `rust-geo/benches/link_check_bench.rs` | `legacy-spring-backend/src/test/java/.../benchmark/LinkCheckBenchmark.java` |
| Rank 1,000 points by distance (all / 5 km + top 10) | `BenchmarkRankLocal` | `rust-geo/benches/geo_bench.rs` | `.../benchmark/RankingBenchmark.java` |

The stub is `go-api/cmd/linkstub`: `GET /status/{code}?delay=5ms`.

## Run everything
```bash
bench/run.sh            # or: bench/run.sh go | rust | spring
```
On Windows use Git Bash. Close other heavy programs first, and run it on an idle machine.

## Run one by hand
```bash
cd go-api && go run ./cmd/linkstub &                      # terminal 1 (stub on :9099)
export STUB_URL=http://127.0.0.1:9099
cd go-api && go test ./internal/geo -run '^$' -bench . -benchtime 5x
cd rust-geo && cargo bench --bench link_check_bench --bench geo_bench
cd legacy-spring-backend && BENCH=1 mvn test -Dtest='LinkCheckBenchmark,RankingBenchmark'
```
`cargo bench --bench NAME -- <criterion flags>`: put the `--bench` name before `--`, otherwise Cargo hands
criterion's flags to the library test harness and fails with "Unrecognized option".

## Reading the results
* Each benchmark **verifies every result**. If the stub is not reachable they fail with a clear message instead of
  printing a fast number.
* Go reports `ns/op` per 1,000 links and `links/s`. Criterion prints `time` per 1,000 links and `thrpt`. Spring prints mean
  and best ms per 1,000 links and `links/s` (JVM warm-up iterations are run first and discarded).
* `delay=0s` measures client overhead (connections, parsing, scheduling). `delay=5ms` measures how well each
  runtime overlaps waiting; with 5 ms per request, 1,000 links at concurrency 10 cannot finish faster than 500 ms.
* What is **not** compared: database writes (the Spring benchmark uses a mock repository), the Go-to-Rust HTTP hop
  that the real Go API uses for link checks, and JSON encoding. Also, the Go benchmark measures the Go *fallback*
  (`CheckLinksLocal`), which is what runs without the Rust service.
* The stub runs on the same machine, so it competes for CPU with the benchmark. That affects all three equally,
  but treat small differences with suspicion.
