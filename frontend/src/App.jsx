import { useEffect, useMemo, useState } from 'react'
import WaterMap from './WaterMap.jsx'
import LocalityLedger from './LocalityLedger.jsx'
import Breadcrumbs from './Breadcrumbs.jsx'
import Hero from './Hero.jsx'
import { fetchLocalities, fetchNearby } from './api.js'
import { STATUS_CONFIG, STATUS_ORDER } from './statusConfig.js'
import { useCountUp } from './useCountUp.js'
import { useMouseParallax } from './useMouseParallax.js'
import { useScrollReveal } from './useScrollReveal.js'
import './App.css'

/** Its own component specifically so useCountUp's hook call happens at THIS component's top
 *  level, once per chip instance -- calling a hook inside the .map() loop that renders these
 *  would violate the Rules of Hooks (hooks must be unconditional, top-level calls). */
function StatusChip({ label, count, isActive, chipColor, chipBg, onClick }) {
  const animatedCount = useCountUp(count)
  return (
    <button
      className={`chip ${isActive ? 'is-active' : ''}`}
      style={chipColor ? { '--chip-color': chipColor, '--chip-bg': chipBg } : undefined}
      onClick={onClick}
    >
      {label} ({animatedCount})
    </button>
  )
}

export default function App() {
  const [localities, setLocalities] = useState([])
  const [status, setStatus] = useState('all')
  const [search, setSearch] = useState('')
  const [selectedId, setSelectedId] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [viewMode, setViewMode] = useState('markers')
  const [showWards, setShowWards] = useState(false)
  const [nearbyResults, setNearbyResults] = useState(null)
  const [locating, setLocating] = useState(false)
  const [locationError, setLocationError] = useState(null)

  const headerParallax = useMouseParallax(8)
  useScrollReveal()

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    fetchLocalities({ status: status === 'all' ? undefined : status, search: search || undefined })
      .then((data) => {
        if (!cancelled) setLocalities(data)
      })
      .catch((err) => {
        if (!cancelled) setError(err.message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [status, search])

  const counts = useMemo(() => {
    const tally = { MUNICIPAL: 0, TANKER_DEPENDENT: 0, MIXED: 0, PIPELINE_IN_PROGRESS: 0 }
    for (const loc of localities) tally[loc.status] = (tally[loc.status] ?? 0) + 1
    return tally
  }, [localities])

  // Scrolls the selected locality's register row into view and briefly flashes it -- useful
  // specifically because the map and register sit side-by-side, so selecting something on the
  // map can leave the matching row scrolled out of view in a long list.
  useEffect(() => {
    if (!selectedId) return
    const row = document.getElementById(`locality-row-${selectedId}`)
    if (!row) return

    row.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    row.classList.add('flash-highlight')
    const timeout = setTimeout(() => row.classList.remove('flash-highlight'), 900)
    return () => clearTimeout(timeout)
  }, [selectedId])

  const findNearMe = () => {
    if (!navigator.geolocation) {
      setLocationError('Your browser doesn\'t support geolocation.')
      return
    }
    setLocating(true)
    setLocationError(null)
    navigator.geolocation.getCurrentPosition(
      (position) => {
        const { latitude, longitude } = position.coords
        fetchNearby(latitude, longitude, 5) // within 5km
          .then((results) => setNearbyResults(results))
          .catch((err) => setLocationError(err.message))
          .finally(() => setLocating(false))
      },
      () => {
        setLocationError('Could not get your location -- check your browser\'s location permission.')
        setLocating(false)
      },
    )
  }

  const clearNearMe = () => {
    setNearbyResults(null)
    setLocationError(null)
  }

  return (
    <div className="app">
      <Hero parallax={headerParallax} />

      <div className="controls reveal" id="main-content">
        <input
          type="search"
          className="search-input"
          placeholder="Search a locality, e.g. Wagholi"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Search localities"
        />
        <button className="near-me-btn" onClick={findNearMe} disabled={locating}>
          {locating ? 'Locating...' : '📍 Near me'}
        </button>
        <div className="filter-chips" role="group" aria-label="Filter by status">
          <StatusChip
            label="All"
            count={localities.length}
            isActive={status === 'all'}
            onClick={() => setStatus('all')}
          />
          {STATUS_ORDER.map((key) => {
            const config = STATUS_CONFIG[key]
            return (
              <StatusChip
                key={key}
                label={config.short}
                count={counts[key] ?? 0}
                isActive={status === key}
                chipColor={config.color}
                chipBg={config.bg}
                onClick={() => setStatus(key)}
              />
            )
          })}
        </div>
      </div>

      {!nearbyResults && (
        <Breadcrumbs
          status={status}
          selectedId={selectedId}
          localities={localities}
          onSelectStatus={setStatus}
          onSelectLocality={setSelectedId}
        />
      )}

      <main className="layout reveal">
        <section className="map-pane">
          <div className="view-toggle" role="group" aria-label="Map view">
            <button
              className={`view-toggle-btn ${viewMode === 'markers' ? 'is-active' : ''}`}
              onClick={() => setViewMode('markers')}
            >
              Pins
            </button>
            <button
              className={`view-toggle-btn ${viewMode === 'heatmap' ? 'is-active' : ''}`}
              onClick={() => setViewMode('heatmap')}
            >
              Heatmap
            </button>
          </div>
          <label className="wards-toggle">
            <input
              type="checkbox"
              checked={showWards}
              onChange={(e) => setShowWards(e.target.checked)}
            />
            Ward boundaries
          </label>
          {error ? (
            <div className="empty-state">
              <p>
                <strong>Can't reach the API.</strong> Make sure the Spring Boot backend is running
                on <code>localhost:8080</code>.
              </p>
              <p className="empty-state-detail">{error}</p>
            </div>
          ) : (
            <WaterMap
              localities={localities}
              selectedId={selectedId}
              onSelect={setSelectedId}
              viewMode={viewMode}
              showWards={showWards}
            />
          )}
        </section>

        <section className="ledger-pane">
          {nearbyResults ? (
            <>
              <div className="ledger-pane-header">
                <h2 className="ledger-pane-title">Nearest to you</h2>
                <button className="clear-near-me-btn" onClick={clearNearMe}>
                  Back to full register
                </button>
              </div>
              {locationError && <p className="ledger-empty">{locationError}</p>}
              {nearbyResults.length === 0 ? (
                <p className="ledger-empty">Nothing found within 5km.</p>
              ) : (
                <ol className="ledger">
                  {nearbyResults.map(({ locality, distanceKm }) => {
                    const config = STATUS_CONFIG[locality.status]
                    return (
                      <li
                        key={locality.id}
                        className={`ledger-row ${locality.id === selectedId ? 'is-selected' : ''}`}
                        onClick={() => setSelectedId(locality.id)}
                      >
                        <span className="ledger-index">{distanceKm.toFixed(1)}km</span>
                        <div className="ledger-body">
                          <div className="ledger-headline">
                            <span className="ledger-name">{locality.name}</span>
                          </div>
                          {locality.notes && <p className="ledger-notes">{locality.notes}</p>}
                        </div>
                        <span
                          className="ledger-stamp"
                          style={{ color: config.color, borderColor: config.color, background: config.bg }}
                        >
                          {config.short}
                        </span>
                      </li>
                    )
                  })}
                </ol>
              )}
            </>
          ) : (
            <>
              <h2 className="ledger-pane-title">Register</h2>
              {locationError && <p className="ledger-empty">{locationError}</p>}
              {loading ? (
                <div className="skeleton-list" aria-label="Loading">
                  {[1, 2, 3, 4, 5].map((i) => (
                    <div key={i} className="skeleton-row">
                      <div className="skeleton-block skeleton-index" />
                      <div className="skeleton-block skeleton-line" />
                      <div className="skeleton-block skeleton-stamp" />
                    </div>
                  ))}
                </div>
              ) : (
                <LocalityLedger localities={localities} selectedId={selectedId} onSelect={setSelectedId} />
              )}
            </>
          )}
        </section>
      </main>

      <footer className="app-footer">
        <p>
          Status entries are compiled from public reporting, not a live PMC feed &mdash; each row
          cites its source and the date it was last checked. Spot something outdated? The API
          accepts citizen reports at <code>POST /api/localities/reports</code> for review.
        </p>
      </footer>
    </div>
  )
}
