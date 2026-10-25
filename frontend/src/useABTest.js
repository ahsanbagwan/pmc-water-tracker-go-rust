import { useEffect, useState } from 'react'

/**
 * Minimal A/B testing: assigns a variant once per visitor (persisted in localStorage, so
 * repeat visits stay consistent -- a user seeing a DIFFERENT variant every reload would make
 * any result meaningless), and reports exposure/conversion through the GA4 setup already
 * wired in analytics.js, rather than building a separate tracking backend.
 *
 * No server involvement at all -- appropriate for this project's scale. A server-assigned
 * split would matter if you needed the SAME visitor to see the same variant across different
 * devices, which isn't a real requirement for an anonymous, no-login civic tool.
 */
export function useABTest(testName, variants = ['A', 'B']) {
  const [variant] = useState(() => {
    const storageKey = `ab-test:${testName}`
    const existing = localStorage.getItem(storageKey)
    if (existing && variants.includes(existing)) {
      return existing
    }

    const assigned = variants[Math.floor(Math.random() * variants.length)]
    localStorage.setItem(storageKey, assigned)
    return assigned
  })

  useEffect(() => {
    if (window.gtag) {
      window.gtag('event', 'experiment_exposure', {
        experiment_name: testName,
        variant,
      })
    }
  }, [testName, variant])

  return variant
}

/** Call when the user performs the action you're actually testing for (e.g. clicking the
 *  CTA) -- separate from the automatic exposure event above, since "was shown" and "took the
 *  action" are the two numbers you actually need to compare between variants. */
export function trackConversion(testName, variant) {
  if (window.gtag) {
    window.gtag('event', 'experiment_conversion', {
      experiment_name: testName,
      variant,
    })
  }
}
