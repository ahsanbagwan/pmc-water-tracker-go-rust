//! Link-check benchmark against a LOCAL stub (see ../../bench/README.md). It calls the real
//! `pmc_geo::links::check_all`, and verifies every result so a dead server cannot produce a
//! fast-but-meaningless number. Start the stub first (bench/run.sh does it), then set STUB_URL.

use criterion::{criterion_group, criterion_main, BatchSize, BenchmarkId, Criterion, Throughput};
use pmc_geo::links::{check_all, client, LinkIn};
use std::time::Duration;

const N: i64 = 1000;

// Same data as the Go and Spring benchmarks: every 10th link answers 404, the rest 200.
fn make_links(base: &str, delay: &str) -> Vec<LinkIn> {
    (0..N)
        .map(|i| {
            let code = if i % 10 == 0 { 404 } else { 200 };
            LinkIn { id: i, url: format!("{base}/status/{code}?delay={delay}") }
        })
        .collect()
}

fn bench_link_checker(c: &mut Criterion) {
    let Ok(base) = std::env::var("STUB_URL") else {
        eprintln!("STUB_URL is not set - skipping. Run bench/run.sh, or: go run ./cmd/linkstub & STUB_URL=http://127.0.0.1:9099 cargo bench");
        return;
    };
    let rt = tokio::runtime::Runtime::new().unwrap();
    let http = client();

    let mut group = c.benchmark_group("check_links");
    group.sample_size(10).measurement_time(Duration::from_secs(8)).throughput(Throughput::Elements(N as u64));
    for delay in ["0s", "5ms"] {
        for conc in [10usize, 50, 100] {
            group.bench_with_input(BenchmarkId::new(format!("delay={delay}"), conc), &conc, |b, &conc| {
                b.to_async(&rt).iter_batched(
                    || make_links(&base, delay),
                    |links| {
                        let http = http.clone();
                        async move {
                            let res = check_all(&http, links, conc).await;
                            assert!(
                                res.iter().all(|r| r.healthy == (r.id % 10 != 0)),
                                "wrong results - is the stub reachable at STUB_URL?"
                            );
                        }
                    },
                    BatchSize::SmallInput,
                );
            });
        }
    }
    group.finish();
}

criterion_group!(benches, bench_link_checker);
criterion_main!(benches);
