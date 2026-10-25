import { useEffect } from 'react'

/**
 * Deliberately the OPPOSITE default of the landing-page demo this was adapted from: there,
 * .reveal elements start hidden via plain CSS, which is fine for a marketing page but risky
 * here -- if IntersectionObserver ever failed to run, the app's actual water-status data
 * would stay invisible forever. Instead: nothing is hidden until this hook confirms it can
 * actually observe and reveal elements, by adding a "js-reveal-ready" class to <body> first.
 * CSS only applies the hidden-until-revealed treatment when that class is present.
 */
export function useScrollReveal() {
  useEffect(() => {
    if (!('IntersectionObserver' in window)) {
      return // browser can't support it -- leave everything at its default visible state
    }

    const revealElements = document.querySelectorAll('.reveal')
    if (revealElements.length === 0) return

    document.body.classList.add('js-reveal-ready')

    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            entry.target.classList.add('is-visible')
            observer.unobserve(entry.target)
          }
        })
      },
      { threshold: 0.1 },
    )

    revealElements.forEach((el) => observer.observe(el))

    return () => observer.disconnect()
  }, [])
}
