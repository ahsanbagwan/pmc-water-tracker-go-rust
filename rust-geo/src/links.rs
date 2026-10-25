//! Concurrent source-link health checks - replaces the Spring SourceLinkHealthChecker's
//! CompletableFuture + dedicated thread pool with tokio tasks bounded by a semaphore.
//! Same semantics: 8s connect/total timeout, follow redirects, healthy = status < 400.

use futures::stream::{self, StreamExt};
use serde::{Deserialize, Serialize};
use std::time::Duration;

#[derive(Debug, Deserialize)]
pub struct LinkIn {
    pub id: i64,
    pub url: String,
}

#[derive(Debug, Serialize, PartialEq)]
pub struct LinkOut {
    pub id: i64,
    pub healthy: bool,
}

pub fn client() -> reqwest::Client {
    reqwest::Client::builder()
        .connect_timeout(Duration::from_secs(8))
        .timeout(Duration::from_secs(8))
        .redirect(reqwest::redirect::Policy::limited(10))
        .user_agent("pmc-water-tracker-link-checker/1.0")
        .build()
        .expect("http client")
}

pub async fn check_all(client: &reqwest::Client, links: Vec<LinkIn>, concurrency: usize) -> Vec<LinkOut> {
    let mut out: Vec<LinkOut> = stream::iter(links)
        .map(|l| {
            let c = client.clone();
            async move {
                let healthy = match c.get(&l.url).send().await {
                    Ok(resp) => resp.status().as_u16() < 400,
                    Err(_) => false, // invalid URL, timeout, DNS failure... all just "unhealthy"
                };
                LinkOut { id: l.id, healthy }
            }
        })
        .buffer_unordered(concurrency.max(1))
        .collect()
        .await;
    out.sort_by_key(|l| l.id);
    out
}
