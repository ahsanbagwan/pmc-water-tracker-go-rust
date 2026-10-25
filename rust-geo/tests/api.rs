use axum::{body::Body, http::{Request, StatusCode}, routing::get, Json, Router};
use http_body_util::BodyExt;
use pmc_geo::{app, geo, links};
use serde_json::{json, Value};
use std::sync::Arc;
use tower::ServiceExt;

const WARDS: &str = r#"{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"name":"Ward A"},
 "geometry":{"type":"Polygon","coordinates":[[[73.0,18.0],[74.0,18.0],[74.0,19.0],[73.0,19.0],[73.0,18.0]]]}}]}"#;

fn state(api_url: &str, with_wards: bool) -> app::AppState {
    let (wards, raw) = if with_wards {
        (geo::WardIndex::from_geojson(WARDS).unwrap(), Some(Arc::new(WARDS.to_string())))
    } else {
        (geo::WardIndex::empty(), None)
    };
    app::AppState { wards: Arc::new(wards), raw_geojson: raw, http: links::client(), api_url: api_url.into(), link_concurrency: 4 }
}

async fn send(router: Router, req: Request<Body>) -> (StatusCode, axum::http::HeaderMap, Value) {
    let resp = router.oneshot(req).await.unwrap();
    let (status, headers) = (resp.status(), resp.headers().clone());
    let bytes = resp.into_body().collect().await.unwrap().to_bytes();
    (status, headers, serde_json::from_slice(&bytes).unwrap_or(Value::Null))
}

fn post_json(uri: &str, body: Value) -> Request<Body> {
    Request::post(uri).header("content-type", "application/json").body(Body::from(body.to_string())).unwrap()
}

#[tokio::test]
async fn ward_lookup_found_and_not_found() {
    let r = app::router(state("http://x", true));
    let (st, _, b) = send(r.clone(), Request::get("/geo/ward?lat=18.5&lng=73.5").body(Body::empty()).unwrap()).await;
    assert_eq!(st, StatusCode::OK);
    assert_eq!(b, json!({"ward":"Ward A","found":true,"loadedWardCount":1}));
    let (_, _, b) = send(r, Request::get("/geo/ward?lat=10&lng=10").body(Body::empty()).unwrap()).await;
    assert_eq!(b, json!({"ward":"","found":false,"loadedWardCount":1}));
}

#[tokio::test]
async fn boundaries_404_without_file_and_served_with_it() {
    let (st, _, _) = send(app::router(state("http://x", false)), Request::get("/geo/boundaries.geojson").body(Body::empty()).unwrap()).await;
    assert_eq!(st, StatusCode::NOT_FOUND);
    let (st, _, b) = send(app::router(state("http://x", true)), Request::get("/geo/boundaries.geojson").body(Body::empty()).unwrap()).await;
    assert_eq!(st, StatusCode::OK);
    assert_eq!(b["type"], "FeatureCollection");
}

#[tokio::test]
async fn nearby_ranks_and_limits() {
    let body = json!({"lat":18.5,"lng":73.8,"limit":2,"points":[
        {"id":1,"lat":18.6,"lng":73.8},{"id":2,"lat":18.51,"lng":73.8},{"id":3,"lat":18.55,"lng":73.8}]});
    let (st, _, b) = send(app::router(state("http://x", false)), post_json("/geo/nearby", body)).await;
    assert_eq!(st, StatusCode::OK);
    let ids: Vec<i64> = b.as_array().unwrap().iter().map(|r| r["id"].as_i64().unwrap()).collect();
    assert_eq!(ids, vec![2, 3]);
    assert!(b[0]["distanceKm"].as_f64().unwrap() < b[1]["distanceKm"].as_f64().unwrap());
}

#[tokio::test]
async fn correlation_id_is_echoed_or_generated() {
    let r = app::router(state("http://x", false));
    let (_, h, _) = send(r.clone(), Request::get("/healthz").header("X-Correlation-Id", "abc-123").body(Body::empty()).unwrap()).await;
    assert_eq!(h["x-correlation-id"], "abc-123");
    let (_, h, _) = send(r, Request::get("/healthz").body(Body::empty()).unwrap()).await;
    assert_eq!(h["x-correlation-id"].to_str().unwrap().len(), 36);
}

#[tokio::test]
async fn link_checker_marks_good_and_bad_urls() {
    let upstream = Router::new()
        .route("/ok", get(|| async { "fine" }))
        .route("/gone", get(|| async { StatusCode::NOT_FOUND }));
    let l = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = l.local_addr().unwrap();
    tokio::spawn(async move { axum::serve(l, upstream).await.unwrap() });

    let body = json!({"links":[
        {"id":2,"url":format!("http://{addr}/gone")},
        {"id":1,"url":format!("http://{addr}/ok")},
        {"id":3,"url":"http://127.0.0.1:1/nothing-listens"},
        {"id":4,"url":"not a url"}]});
    let (st, _, b) = send(app::router(state("http://x", false)), post_json("/links/check", body)).await;
    assert_eq!(st, StatusCode::OK);
    assert_eq!(b, json!([{"id":1,"healthy":true},{"id":2,"healthy":false},{"id":3,"healthy":false},{"id":4,"healthy":false}]));
}

#[tokio::test]
async fn mcp_protocol_and_tools() {
    // fake Go API
    let api = Router::new().route("/api/localities", get(|q: axum::extract::Query<std::collections::HashMap<String, String>>| async move {
        if q.get("search").map(String::as_str) == Some("wagholi") {
            Json(json!([{"name":"Wagholi","status":"TANKER_DEPENDENT","notes":"tankers","sourceUrl":"http://s"}]))
        } else if q.get("status").map(String::as_str) == Some("MIXED") {
            Json(json!([{"name":"Undri"},{"name":"Katraj"}]))
        } else {
            Json(json!([]))
        }
    }));
    let l = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = l.local_addr().unwrap();
    tokio::spawn(async move { axum::serve(l, api).await.unwrap() });
    let r = app::router(state(&format!("http://{addr}"), false));

    let (_, _, b) = send(r.clone(), post_json("/mcp", json!({"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}))).await;
    assert_eq!(b["result"]["serverInfo"]["name"], "pmc-water-tracker");

    let (_, _, b) = send(r.clone(), post_json("/mcp", json!({"jsonrpc":"2.0","id":2,"method":"tools/list"}))).await;
    let names: Vec<&str> = b["result"]["tools"].as_array().unwrap().iter().map(|t| t["name"].as_str().unwrap()).collect();
    assert_eq!(names, vec!["get_locality_status", "list_localities_by_status"]);

    let call = |name: &str, args: Value| post_json("/mcp", json!({"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":name,"arguments":args}}));
    let (_, _, b) = send(r.clone(), call("get_locality_status", json!({"name":"wagholi"}))).await;
    assert_eq!(b["result"]["content"][0]["text"], "Wagholi: TANKER_DEPENDENT -- tankers (source: http://s)");
    let (_, _, b) = send(r.clone(), call("get_locality_status", json!({"name":"nowhere"}))).await;
    assert_eq!(b["result"]["content"][0]["text"], "No locality found matching \"nowhere\".");
    let (_, _, b) = send(r.clone(), call("list_localities_by_status", json!({"status":"MIXED"}))).await;
    assert_eq!(b["result"]["content"][0]["text"], "Undri, Katraj");
    let (_, _, b) = send(r.clone(), call("list_localities_by_status", json!({"status":"BOGUS"}))).await;
    assert!(b["result"]["content"][0]["text"].as_str().unwrap().starts_with("Unknown status \"BOGUS\""));

    let (_, _, b) = send(r.clone(), post_json("/mcp", json!({"jsonrpc":"2.0","id":9,"method":"nope"}))).await;
    assert_eq!(b["error"]["code"], -32601);
    let resp = r.oneshot(post_json("/mcp", json!({"jsonrpc":"2.0","method":"notifications/initialized"}))).await.unwrap();
    assert_eq!(resp.status(), StatusCode::ACCEPTED);
}
