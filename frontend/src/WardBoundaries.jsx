import { useEffect, useState } from 'react'
import { GeoJSON } from 'react-leaflet'
import { fetchWardBoundaries } from './api.js'

const WARD_STYLE = {
  color: '#6b4e9e',
  weight: 1.5,
  fillOpacity: 0,
  dashArray: '4 3',
}

export default function WardBoundaries() {
  const [geoJson, setGeoJson] = useState(null)

  useEffect(() => {
    let cancelled = false
    fetchWardBoundaries().then((data) => {
      if (!cancelled) setGeoJson(data)
    })
    return () => {
      cancelled = true
    }
  }, [])

  if (!geoJson) {
    return null // file not present yet, or still loading -- either way, nothing to render
  }

  return (
    <GeoJSON
      data={geoJson}
      style={WARD_STYLE}
      onEachFeature={(feature, layer) => {
        const name = feature.properties?.name ?? feature.properties?.Name ?? feature.properties?.NAME
        if (name) layer.bindTooltip(name, { sticky: true })
      }}
    />
  )
}
