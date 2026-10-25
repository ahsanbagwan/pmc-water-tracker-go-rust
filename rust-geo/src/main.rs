use pmc_geo::{app, geo, links};
use std::{path::PathBuf, sync::Arc};

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .json()
        .with_env_filter(tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()))
        .init();

    let port = std::env::var("PORT").unwrap_or_else(|_| "8090".into());
    let ward_path = PathBuf::from(
        std::env::var("WARD_GEOJSON").unwrap_or_else(|_| "static/pune-admin-wards.geojson".into()),
    );
    let (wards, raw) = geo::WardIndex::load(&ward_path);
    if wards.count() == 0 {
        tracing::info!(path = %ward_path.display(), "no ward boundaries loaded - ward lookup disabled until the geojson is added");
    } else {
        tracing::info!(count = wards.count(), "loaded ward boundaries");
    }

    let state = app::AppState {
        wards: Arc::new(wards),
        raw_geojson: raw.map(Arc::new),
        http: links::client(),
        api_url: std::env::var("GO_API_URL").unwrap_or_else(|_| "http://localhost:8080".into()),
        link_concurrency: std::env::var("LINK_CHECK_CONCURRENCY").ok().and_then(|v| v.parse().ok()).unwrap_or(20),
    };

    let listener = tokio::net::TcpListener::bind(format!("0.0.0.0:{port}")).await.expect("bind");
    tracing::info!(%port, "pmc-geo listening");
    axum::serve(listener, app::router(state))
        .with_graceful_shutdown(async {
            let _ = tokio::signal::ctrl_c().await;
        })
        .await
        .expect("server");
}
