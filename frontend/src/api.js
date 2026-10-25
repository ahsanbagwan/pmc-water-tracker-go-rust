const API_BASE = import.meta.env.VITE_API_BASE ?? 'http://localhost:8080/api'
const BACKEND_ORIGIN = API_BASE.replace(/\/api\/?$/, '')

export async function fetchLocalities({ status, search } = {}) {
  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (search) params.set('search', search)
  const query = params.toString() ? `?${params.toString()}` : ''

  const res = await fetch(`${API_BASE}/localities${query}`)
  if (!res.ok) {
    throw new Error(`Request failed (${res.status})`)
  }
  return res.json()
}

export async function fetchNearby(lat, lng, radiusKm) {
  const params = new URLSearchParams({ lat, lng })
  if (radiusKm != null) params.set('radiusKm', radiusKm)

  const res = await fetch(`${API_BASE}/localities/nearby?${params.toString()}`)
  if (!res.ok) {
    throw new Error(`Request failed (${res.status})`)
  }
  return res.json()
}

/** Returns null (not an error) if the ward boundary file hasn't been added to the backend yet
 *  -- see WardBoundaryService's class comment for where to get it. */
export async function fetchWardBoundaries() {
  const res = await fetch(`${BACKEND_ORIGIN}/pune-admin-wards.geojson`)
  if (!res.ok) {
    return null
  }
  return res.json()
}

export async function submitReport(payload) {
  const res = await fetch(`${API_BASE}/localities/reports`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error ?? `Request failed (${res.status})`)
  }
  return res.json()
}
