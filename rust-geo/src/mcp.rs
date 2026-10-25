//! Minimal MCP server (Streamable HTTP, JSON-RPC 2.0) exposing the same two read-only tools
//! as the Spring version: get_locality_status and list_localities_by_status. It does not touch
//! the database - it calls the Go API's public GET endpoints, same trust level as any citizen.

use serde_json::{json, Value};

pub const STATUSES: [&str; 4] = ["MUNICIPAL", "TANKER_DEPENDENT", "MIXED", "PIPELINE_IN_PROGRESS"];

pub fn tools_list() -> Value {
    json!({ "tools": [
        {
            "name": "get_locality_status",
            "description": "Look up the current water supply status of a PMC (Pune) locality by name, e.g. 'Wagholi' or 'Kothrud'. Returns status, notes, and source citation.",
            "inputSchema": {
                "type": "object",
                "properties": { "name": { "type": "string", "description": "Locality name to search for" } },
                "required": ["name"]
            }
        },
        {
            "name": "list_localities_by_status",
            "description": "List PMC localities matching a water status. Valid status values: MUNICIPAL (reliable piped supply), TANKER_DEPENDENT (no piped connection), MIXED (partial piped coverage), PIPELINE_IN_PROGRESS (pipeline work underway).",
            "inputSchema": {
                "type": "object",
                "properties": { "status": { "type": "string", "enum": STATUSES } },
                "required": ["status"]
            }
        }
    ]})
}

pub fn text_result(text: impl Into<String>) -> Value {
    json!({ "content": [ { "type": "text", "text": text.into() } ] })
}

pub fn summarize_matches(name: &str, localities: &[Value]) -> String {
    if localities.is_empty() {
        return format!("No locality found matching \"{name}\".");
    }
    localities
        .iter()
        .map(|l| {
            let mut line = format!(
                "{}: {}",
                l["name"].as_str().unwrap_or("?"),
                l["status"].as_str().unwrap_or("?")
            );
            if let Some(n) = l["notes"].as_str() {
                line.push_str(&format!(" -- {n}"));
            }
            if let Some(u) = l["sourceUrl"].as_str() {
                line.push_str(&format!(" (source: {u})"));
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

pub fn summarize_status_list(status: &str, localities: &[Value]) -> String {
    if localities.is_empty() {
        format!("No localities currently have status {status}.")
    } else {
        localities.iter().filter_map(|l| l["name"].as_str()).collect::<Vec<_>>().join(", ")
    }
}

pub fn rpc_ok(id: &Value, result: Value) -> Value {
    json!({ "jsonrpc": "2.0", "id": id, "result": result })
}

pub fn rpc_err(id: &Value, code: i32, message: &str) -> Value {
    json!({ "jsonrpc": "2.0", "id": id, "error": { "code": code, "message": message } })
}
