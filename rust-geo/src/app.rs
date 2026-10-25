use crate::{geo, links, mcp};
use axum::{
    extract::{Query, Request, State},
    http::{header, HeaderValue, StatusCode},
    middleware::{self, Next},
    response::{IntoResponse, Response},
    routing::{get, post},
    Json, Router,
};
use serde::Deserialize;
use serde_json::{json, Value};
use std::{sync::Arc, time::Instant};

#[derive(Clone)]
pub struct AppState {
    pub wards: Arc<geo::WardIndex>,
    pub raw_geojson: Option<Arc<String>>,
    pub http: reqwest::Client,
    /// Base URL of the Go API, e.g. http://go-api:8080 (used by the MCP tools).
    pub api_url: String,
    pub link_concurrency: usize,
}

pub const CORRELATION_HEADER: &str = "x-correlation-id";

pub fn router(state: AppState) -> Router {
    Router::new()
        .route("/healthz", get(healthz))
        .route("/geo/ward", get(ward_lookup))
        .route("/geo/boundaries.geojson", get(boundaries))
        .route("/geo/nearby", post(nearby))
        .route("/links/check", post(check_links))
        .route("/mcp", post(mcp_handler))
        .layer(middleware::from_fn(correlation_and_logging))
        .with_state(state)
}

/// Same job as the Spring CorrelationIdFilter + RequestLoggingFilter: reuse X-Correlation-Id
/// or generate one, echo it on the response, and log one line per request carrying it.
async fn correlation_and_logging(req: Request, next: Next) -> Response {
    let start = Instant::now();
    let method = req.method().clone();
    let path = req.uri().path().to_owned();
    let cid = req
        .headers()
        .get(CORRELATION_HEADER)
        .and_then(|v| v.to_str().ok())
        .filter(|s| !s.trim().is_empty())
        .map(str::to_owned)
        .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());

    let mut resp = next.run(req).await;
    if let Ok(v) = HeaderValue::from_str(&cid) {
        resp.headers_mut().insert(CORRELATION_HEADER, v);
    }
    tracing::info!(
        correlation_id = %cid, method = %method, path = %path,
        status = resp.status().as_u16(), duration_ms = start.elapsed().as_millis() as u64,
        "request"
    );
    resp
}

async fn healthz() -> Json<Value> {
    Json(json!({ "status": "UP" }))
}

#[derive(Deserialize)]
struct LatLng {
    lat: f64,
    lng: f64,
}

async fn ward_lookup(State(s): State<AppState>, Query(q): Query<LatLng>) -> Json<Value> {
    let ward = s.wards.find_ward(q.lat, q.lng);
    Json(json!({
        "ward": ward.unwrap_or(""),
        "found": ward.is_some(),
        "loadedWardCount": s.wards.count(),
    }))
}

async fn boundaries(State(s): State<AppState>) -> Response {
    match &s.raw_geojson {
        Some(text) => (
            [(header::CONTENT_TYPE, "application/geo+json")],
            text.as_str().to_owned(),
        )
            .into_response(),
        None => StatusCode::NOT_FOUND.into_response(),
    }
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct NearbyReq {
    lat: f64,
    lng: f64,
    radius_km: Option<f64>,
    limit: Option<usize>,
    points: Vec<geo::Point>,
}

async fn nearby(Json(req): Json<NearbyReq>) -> Json<Vec<geo::Ranked>> {
    Json(geo::rank_nearby(req.lat, req.lng, req.radius_km, &req.points, req.limit))
}

#[derive(Deserialize)]
struct CheckReq {
    links: Vec<links::LinkIn>,
}

async fn check_links(State(s): State<AppState>, Json(req): Json<CheckReq>) -> Json<Vec<links::LinkOut>> {
    Json(links::check_all(&s.http, req.links, s.link_concurrency).await)
}

async fn fetch_localities(s: &AppState, query: &[(&str, &str)]) -> Result<Vec<Value>, String> {
    let url = format!("{}/api/localities", s.api_url.trim_end_matches('/'));
    let resp = s.http.get(url).query(query).send().await.map_err(|e| e.to_string())?;
    if !resp.status().is_success() {
        return Err(format!("API returned {}", resp.status()));
    }
    resp.json::<Vec<Value>>().await.map_err(|e| e.to_string())
}

async fn call_tool(s: &AppState, name: &str, args: &Value) -> Value {
    match name {
        "get_locality_status" => {
            let q = args["name"].as_str().unwrap_or("");
            match fetch_localities(s, &[("search", q)]).await {
                Ok(list) => mcp::text_result(mcp::summarize_matches(q, &list)),
                Err(e) => mcp::text_result(format!("Could not reach the locality API: {e}")),
            }
        }
        "list_localities_by_status" => {
            let status = args["status"].as_str().unwrap_or("");
            if !mcp::STATUSES.contains(&status) {
                return mcp::text_result(format!(
                    "Unknown status \"{status}\". Valid values: MUNICIPAL, TANKER_DEPENDENT, MIXED, PIPELINE_IN_PROGRESS."
                ));
            }
            match fetch_localities(s, &[("status", status)]).await {
                Ok(list) => mcp::text_result(mcp::summarize_status_list(status, &list)),
                Err(e) => mcp::text_result(format!("Could not reach the locality API: {e}")),
            }
        }
        _ => mcp::text_result(format!("Unknown tool \"{name}\".")),
    }
}

async fn mcp_handler(State(s): State<AppState>, Json(msg): Json<Value>) -> Response {
    let id = msg.get("id").cloned().unwrap_or(Value::Null);
    let method = msg["method"].as_str().unwrap_or("");

    // Notifications have no id and get no JSON-RPC response body.
    if msg.get("id").is_none() {
        return StatusCode::ACCEPTED.into_response();
    }

    let body = match method {
        "initialize" => mcp::rpc_ok(
            &id,
            json!({
                "protocolVersion": "2025-03-26",
                "capabilities": { "tools": { "listChanged": false } },
                "serverInfo": { "name": "pmc-water-tracker", "version": "1.0.0" }
            }),
        ),
        "ping" => mcp::rpc_ok(&id, json!({})),
        "tools/list" => mcp::rpc_ok(&id, mcp::tools_list()),
        "tools/call" => {
            let name = msg["params"]["name"].as_str().unwrap_or("");
            let result = call_tool(&s, name, &msg["params"]["arguments"]).await;
            mcp::rpc_ok(&id, result)
        }
        other => mcp::rpc_err(&id, -32601, &format!("Method not found: {other}")),
    };
    Json(body).into_response()
}
