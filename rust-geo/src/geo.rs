//! Geometry: haversine distance, ward boundaries and point-in-polygon.
//!
//! Direct port of the Spring `GeoUtils` and `WardBoundaryService`. Same behaviours kept on
//! purpose: ray casting on exterior rings only (holes ignored), Polygon + MultiPolygon, name
//! looked up under several common property keys, GeoJSON [lng, lat] order, and a missing or
//! broken file degrades to "no wards loaded" instead of failing startup.

use serde::Deserialize;
use serde_json::Value;
use std::path::Path;

const EARTH_RADIUS_KM: f64 = 6371.0;

pub fn distance_km(lat1: f64, lon1: f64, lat2: f64, lon2: f64) -> f64 {
    let d_lat = (lat2 - lat1).to_radians();
    let d_lon = (lon2 - lon1).to_radians();
    let a = (d_lat / 2.0).sin().powi(2)
        + lat1.to_radians().cos() * lat2.to_radians().cos() * (d_lon / 2.0).sin().powi(2);
    let c = 2.0 * a.sqrt().atan2((1.0 - a).sqrt());
    EARTH_RADIUS_KM * c
}

#[derive(Debug, Clone, Deserialize)]
pub struct Point {
    pub id: i64,
    pub lat: f64,
    pub lng: f64,
}

#[derive(Debug, Clone, PartialEq, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Ranked {
    pub id: i64,
    pub distance_km: f64,
}

/// Rank `points` by distance from (lat, lng), nearest first. `radius_km = None` means no cutoff.
pub fn rank_nearby(lat: f64, lng: f64, radius_km: Option<f64>, points: &[Point], limit: Option<usize>) -> Vec<Ranked> {
    let mut out: Vec<Ranked> = points
        .iter()
        .map(|p| Ranked { id: p.id, distance_km: distance_km(lat, lng, p.lat, p.lng) })
        .filter(|r| radius_km.map_or(true, |max| r.distance_km <= max))
        .collect();
    out.sort_by(|a, b| a.distance_km.total_cmp(&b.distance_km));
    if let Some(n) = limit {
        out.truncate(n);
    }
    out
}

#[derive(Debug)]
struct Ring {
    lats: Vec<f64>,
    lngs: Vec<f64>,
}

impl Ring {
    /// Standard ray casting: count crossings of a ray from the point; odd = inside.
    fn contains(&self, lat: f64, lng: f64) -> bool {
        let n = self.lats.len();
        if n == 0 {
            return false;
        }
        let mut inside = false;
        let mut j = n - 1;
        for i in 0..n {
            let (lat_i, lng_i) = (self.lats[i], self.lngs[i]);
            let (lat_j, lng_j) = (self.lats[j], self.lngs[j]);
            let crosses = ((lat_i > lat) != (lat_j > lat))
                && (lng < (lng_j - lng_i) * (lat - lat_i) / (lat_j - lat_i) + lng_i);
            if crosses {
                inside = !inside;
            }
            j = i;
        }
        inside
    }
}

#[derive(Debug)]
struct Ward {
    name: String,
    rings: Vec<Ring>,
}

#[derive(Debug, Default)]
pub struct WardIndex {
    wards: Vec<Ward>,
}

impl WardIndex {
    pub fn empty() -> Self {
        Self::default()
    }

    /// Never fails: a missing/unparseable file just yields an empty index (ward lookup disabled).
    pub fn load(path: &Path) -> (Self, Option<String>) {
        match std::fs::read_to_string(path) {
            Err(_) => (Self::empty(), None),
            Ok(text) => match Self::from_geojson(&text) {
                Ok(idx) => (idx, Some(text)),
                Err(e) => {
                    tracing::error!(error = %e, "failed to parse ward geojson - ward lookup disabled");
                    (Self::empty(), None)
                }
            },
        }
    }

    pub fn from_geojson(text: &str) -> Result<Self, String> {
        let root: Value = serde_json::from_str(text).map_err(|e| e.to_string())?;
        let mut wards = Vec::new();
        for feature in root["features"].as_array().unwrap_or(&vec![]) {
            let name = extract_name(&feature["properties"]);
            let rings = extract_rings(&feature["geometry"]);
            if let (Some(name), false) = (name, rings.is_empty()) {
                wards.push(Ward { name, rings });
            }
        }
        Ok(Self { wards })
    }

    pub fn find_ward(&self, lat: f64, lng: f64) -> Option<&str> {
        self.wards
            .iter()
            .find(|w| w.rings.iter().any(|r| r.contains(lat, lng)))
            .map(|w| w.name.as_str())
    }

    pub fn count(&self) -> usize {
        self.wards.len()
    }
}

fn extract_name(props: &Value) -> Option<String> {
    ["name", "Name", "NAME", "ward_name", "WARD_NAME"]
        .iter()
        .filter_map(|k| props.get(*k).and_then(Value::as_str))
        .find(|s| !s.trim().is_empty())
        .map(str::to_owned)
}

fn extract_rings(geometry: &Value) -> Vec<Ring> {
    let coords = &geometry["coordinates"];
    match geometry["type"].as_str() {
        Some("Polygon") => exterior(coords).into_iter().collect(),
        Some("MultiPolygon") => coords
            .as_array()
            .map(|polys| polys.iter().filter_map(exterior).collect())
            .unwrap_or_default(),
        _ => vec![],
    }
}

fn exterior(polygon: &Value) -> Option<Ring> {
    let ring = polygon.get(0)?.as_array()?;
    let mut lats = Vec::with_capacity(ring.len());
    let mut lngs = Vec::with_capacity(ring.len());
    for c in ring {
        // GeoJSON order is [longitude, latitude]
        lngs.push(c.get(0)?.as_f64()?);
        lats.push(c.get(1)?.as_f64()?);
    }
    Some(Ring { lats, lngs })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn haversine_known_distance() {
        // Kothrud -> Shivajinagar is a few km; Pune -> Mumbai is ~120 km
        let d = distance_km(18.5204, 73.8567, 19.0760, 72.8777);
        assert!((d - 119.0).abs() < 5.0, "got {d}");
        assert_eq!(distance_km(18.5, 73.8, 18.5, 73.8), 0.0);
    }

    #[test]
    fn rank_sorts_filters_and_limits() {
        let pts = vec![
            Point { id: 1, lat: 18.60, lng: 73.80 },
            Point { id: 2, lat: 18.51, lng: 73.80 },
            Point { id: 3, lat: 18.55, lng: 73.80 },
        ];
        let all = rank_nearby(18.50, 73.80, None, &pts, None);
        assert_eq!(all.iter().map(|r| r.id).collect::<Vec<_>>(), vec![2, 3, 1]);
        let near = rank_nearby(18.50, 73.80, Some(8.0), &pts, None);
        assert_eq!(near.iter().map(|r| r.id).collect::<Vec<_>>(), vec![2, 3]);
        assert_eq!(rank_nearby(18.50, 73.80, None, &pts, Some(1)).len(), 1);
    }

    const WARDS: &str = r#"{"type":"FeatureCollection","features":[
      {"type":"Feature","properties":{"name":"Square"},
       "geometry":{"type":"Polygon","coordinates":[[[73.0,18.0],[74.0,18.0],[74.0,19.0],[73.0,19.0],[73.0,18.0]]]}},
      {"type":"Feature","properties":{"Name":"Multi"},
       "geometry":{"type":"MultiPolygon","coordinates":[
         [[[75.0,18.0],[76.0,18.0],[76.0,19.0],[75.0,19.0],[75.0,18.0]]],
         [[[77.0,18.0],[78.0,18.0],[78.0,19.0],[77.0,19.0],[77.0,18.0]]]]}},
      {"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}}
    ]}"#;

    #[test]
    fn ward_lookup_polygon_multipolygon_and_outside() {
        let idx = WardIndex::from_geojson(WARDS).unwrap();
        assert_eq!(idx.count(), 2, "feature without a name is skipped");
        // lat, lng order for the API; GeoJSON is [lng, lat]
        assert_eq!(idx.find_ward(18.5, 73.5), Some("Square"));
        assert_eq!(idx.find_ward(18.5, 77.5), Some("Multi"));
        assert_eq!(idx.find_ward(18.5, 75.5), Some("Multi"));
        assert_eq!(idx.find_ward(18.5, 74.5), None);
        assert_eq!(idx.find_ward(10.0, 73.5), None);
    }

    #[test]
    fn missing_or_bad_file_degrades_to_empty() {
        let (idx, raw) = WardIndex::load(Path::new("/definitely/not/here.geojson"));
        assert_eq!(idx.count(), 0);
        assert!(raw.is_none());
        assert_eq!(idx.find_ward(18.5, 73.5), None);
        assert!(WardIndex::from_geojson("not json").is_err());
    }
}
