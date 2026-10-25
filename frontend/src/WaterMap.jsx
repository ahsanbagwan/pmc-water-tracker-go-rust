import { MapContainer, TileLayer, CircleMarker, Popup } from 'react-leaflet'
import { STATUS_CONFIG } from './statusConfig'
import WaterHeatmap from './WaterHeatmap.jsx'
import WardBoundaries from './WardBoundaries.jsx'

const PUNE_CENTER = [18.5204, 73.8567]

export default function WaterMap({ localities, selectedId, onSelect, viewMode = 'markers', showWards = false }) {
  return (
    <MapContainer
      center={PUNE_CENTER}
      zoom={11}
      scrollWheelZoom={true}
      style={{ height: '100%', width: '100%' }}
    >
      <TileLayer
        attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
        url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
      />
      {showWards && <WardBoundaries />}
      {viewMode === 'heatmap' ? (
        <WaterHeatmap localities={localities} />
      ) : (
        localities.map((loc) => {
          const config = STATUS_CONFIG[loc.status]
          const isSelected = loc.id === selectedId
          return (
            <CircleMarker
              key={loc.id}
              center={[loc.latitude, loc.longitude]}
              radius={isSelected ? 12 : 8}
              pathOptions={{
                color: config.color,
                fillColor: config.color,
                fillOpacity: isSelected ? 0.9 : 0.65,
                weight: isSelected ? 3 : 1.5,
              }}
              eventHandlers={{ click: () => onSelect(loc.id) }}
            >
              <Popup>
                <strong>{loc.name}</strong>
                <br />
                {config.label}
                <br />
                <span style={{ fontSize: '0.85em', color: '#4a5c5a' }}>{loc.notes}</span>
              </Popup>
            </CircleMarker>
          )
        })
      )}
    </MapContainer>
  )
}
