import { useEffect } from 'react'
import { useMap } from 'react-leaflet'
import L from 'leaflet'
import 'leaflet.heat'

// Higher weight = shows up "hotter" on the heatmap. Weights the worst water-access
// situations most heavily, so this reads as "where problems concentrate," not just
// "where any locality happens to be" -- a plain density map of all localities would be
// far less useful than one weighted toward severity.
const SEVERITY_WEIGHT = {
  TANKER_DEPENDENT: 1.0,
  MIXED: 0.6,
  PIPELINE_IN_PROGRESS: 0.5,
  MUNICIPAL: 0.15,
}

export default function WaterHeatmap({ localities }) {
  const map = useMap()

  useEffect(() => {
    const points = localities
      .filter((loc) => loc.latitude != null && loc.longitude != null)
      .map((loc) => [loc.latitude, loc.longitude, SEVERITY_WEIGHT[loc.status] ?? 0.3])

    const heatLayer = L.heatLayer(points, {
      radius: 35,
      blur: 25,
      maxZoom: 14,
      minOpacity: 0.35,
      // Same palette as the pin markers/status chips, low-to-high severity: municipal teal
      // at the cool end, through mixed/in-progress amber, to tanker-dependent rust at the peak.
      gradient: {
        0.2: '#1d7a8c',
        0.5: '#b98a1f',
        0.8: '#b5541f',
      },
    })

    heatLayer.addTo(map)

    // Cleanup on unmount or when localities change -- otherwise switching data (or toggling
    // the view off and back on) would stack duplicate heat layers on the same map instance.
    return () => {
      map.removeLayer(heatLayer)
    }
  }, [map, localities])

  return null // manages an imperative Leaflet layer only; renders no JSX of its own
}
