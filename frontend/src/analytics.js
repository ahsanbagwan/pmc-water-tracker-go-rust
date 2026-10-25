const MEASUREMENT_ID = import.meta.env.VITE_GA_MEASUREMENT_ID

/** No-op if VITE_GA_MEASUREMENT_ID isn't set -- e.g. local dev, where you almost certainly
 *  don't want your own testing traffic mixed into real analytics. */
export function initAnalytics() {
  if (!MEASUREMENT_ID) {
    return
  }

  const script = document.createElement('script')
  script.async = true
  script.src = `https://www.googletagmanager.com/gtag/js?id=${MEASUREMENT_ID}`
  document.head.appendChild(script)

  window.dataLayer = window.dataLayer || []
  function gtag() {
    window.dataLayer.push(arguments)
  }
  window.gtag = gtag

  gtag('js', new Date())
  gtag('config', MEASUREMENT_ID)
}
