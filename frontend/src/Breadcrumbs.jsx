import { STATUS_CONFIG } from './statusConfig.js'

/**
 * This app has no client-side routing (no React Router, the URL never changes) -- its real
 * "navigation" is filter/selection STATE instead. This breadcrumb reflects that state as a
 * trail, rather than a URL hierarchy: All Localities > [status filter] > [selected locality].
 * Each segment except the last is clickable and jumps back exactly one level, same UX
 * convention as a normal breadcrumb -- the last segment represents "where you are now" and is
 * deliberately not a link.
 */
export default function Breadcrumbs({ status, selectedId, localities, onSelectStatus, onSelectLocality }) {
  const selectedLocality = selectedId ? localities.find((loc) => loc.id === selectedId) : null
  const statusConfig = status !== 'all' ? STATUS_CONFIG[status] : null

  return (
    <nav className="breadcrumbs" aria-label="Breadcrumb">
      <button
        className="breadcrumb-segment breadcrumb-link"
        onClick={() => {
          onSelectStatus('all')
          onSelectLocality(null)
        }}
      >
        All Localities
      </button>

      {statusConfig && (
        <>
          <span className="breadcrumb-separator">/</span>
          {selectedLocality ? (
            <button
              className="breadcrumb-segment breadcrumb-link"
              onClick={() => onSelectLocality(null)}
            >
              {statusConfig.label}
            </button>
          ) : (
            <span className="breadcrumb-segment breadcrumb-current">{statusConfig.label}</span>
          )}
        </>
      )}

      {selectedLocality && (
        <>
          <span className="breadcrumb-separator">/</span>
          <span className="breadcrumb-segment breadcrumb-current">{selectedLocality.name}</span>
        </>
      )}
    </nav>
  )
}
