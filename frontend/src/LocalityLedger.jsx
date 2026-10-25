import { STATUS_CONFIG } from './statusConfig'
import './LocalityLedger.css'

export default function LocalityLedger({ localities, selectedId, onSelect }) {
  if (localities.length === 0) {
    return <p className="ledger-empty">No localities match this filter yet.</p>
  }

  return (
    <ol className="ledger">
      {localities.map((loc, index) => {
        const config = STATUS_CONFIG[loc.status]
        return (
          <li
            key={loc.id}
            id={`locality-row-${loc.id}`}
            className={`ledger-row ${loc.id === selectedId ? 'is-selected' : ''}`}
            onClick={() => onSelect(loc.id)}
          >
            <span className="ledger-index">{String(index + 1).padStart(2, '0')}</span>
            <div className="ledger-body">
              <div className="ledger-headline">
                <span className="ledger-name">{loc.name}</span>
                {loc.ward && <span className="ledger-ward">{loc.ward}</span>}
              </div>
              {loc.notes && <p className="ledger-notes">{loc.notes}</p>}
              <div className="ledger-meta">
                {loc.sourceUrl ? (
                  <a href={loc.sourceUrl} target="_blank" rel="noreferrer">
                    {loc.sourceName ?? 'Source'}
                  </a>
                ) : (
                  <span>{loc.sourceName ?? 'Source not specified'}</span>
                )}
                {loc.lastVerified && <span> &middot; checked {loc.lastVerified}</span>}
              </div>
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
  )
}
