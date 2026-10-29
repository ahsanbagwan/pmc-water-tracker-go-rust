use criterion::{black_box, criterion_group, criterion_main, Criterion, BenchmarkId};
use futures::stream::{self, StreamExt};
use reqwest::Client;
use std::time::Duration;

// Simulate your LinkIn struct
#[derive(Clone)]
struct LinkIn { id: i64, url: String }

async fn check_links_concurrent(client: &Client, links: Vec<LinkIn>, concurrency: usize) {
    let _: Vec<bool> = stream::iter(links)
        .map(|l| {
            let c = client.clone();
            async move {
                match c.get(&l.url).send().await {
                    Ok(resp) => resp.status().as_u16() < 400,
                    Err(_) => false,
                }
            }
        })
        .buffer_unordered(concurrency)
        .collect()
        .await;
}

fn bench_link_checker(c: &mut Criterion) {
    // Setup async runtime for the benchmark
    let rt = tokio::runtime::Runtime::new().unwrap();
    let client = Client::builder()
        .timeout(Duration::from_secs(5))
        .build()
        .unwrap();

    // Create a mock server (using wiremock or similar in real life,
    // here we point to a fast public endpoint or assume local mock)
    // For this example, we'll use a fast, reliable endpoint.
    let links: Vec<LinkIn> = (0..1000)
        .map(|i| LinkIn {
            id: i,
            url: "http://httpbin.org/status/200".to_string()
        })
        .collect();

    let mut group = c.benchmark_group("concurrent_link_check");
    group.measurement_time(Duration::from_secs(10)); // Run longer for stable stats

    for concurrency in [10, 50, 100, 500].iter() {
        group.bench_with_input(BenchmarkId::new("buffer_unordered", concurrency), concurrency, |b, &conc| {
            b.to_async(&rt).iter(|| {
                check_links_concurrent(black_box(&client), black_box(links.clone()), conc)
            });
        });
    }
    group.finish();
}

criterion_group!(benches, bench_link_checker);
criterion_main!(benches);