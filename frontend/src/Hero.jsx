import { useABTest, trackConversion } from './useABTest.js'

/**
 * Full-viewport video-background hero. Uses the app's own established palette
 * (--color-header/--color-municipal) rather than a mismatched new one, and civic-register
 * copy rather than marketing language -- "explore the map," not "shop now."
 *
 * The CTA smooth-scrolls straight to #main-content (the real search/map/register UI) --
 * this is the deliberate fix for the structural cost of adding a big hero at all: the actual
 * tool a visitor came for should never be more than one click away, even below the fold.
 *
 * A/B TEST: two CTA copy variants, testing whether a more direct "see your ward" framing
 * out-clicks the more general "explore the map" framing. See useABTest.js -- results land in
 * GA4 as experiment_exposure / experiment_conversion events, filterable by experiment_name
 * and variant.
 */
const CTA_COPY = {
  A: 'Explore the water map \u2193',
  B: 'See your ward\u2019s status \u2192',
}

export default function Hero({ parallax = { x: 0, y: 0 } }) {
  const ctaVariant = useABTest('hero-cta-copy', ['A', 'B'])

  const scrollToContent = () => {
    trackConversion('hero-cta-copy', ctaVariant)
    document.getElementById('main-content')?.scrollIntoView({ behavior: 'smooth' })
  }

  return (
    <section className="hero">
      {/*
        REPLACE THIS with real footage when you have it:
        <video className="hero-video" autoplay muted loop playsinline poster="hero-poster.jpg">
          <source src="/hero-water.mp4" type="video/mp4" />
        </video>
        All four attributes (autoplay, muted, loop, playsinline) are required TOGETHER for
        autoplay to work across browsers -- drop any one and it silently fails on mobile Safari.
      */}
      <div className="hero-fallback-bg" />
      <div className="hero-overlay" />

      <div className="hero-content" style={{ transform: `translate(${parallax.x}px, ${parallax.y}px)` }}>
        <p className="hero-eyebrow">Pune Municipal Corporation &middot; Ward Register</p>
        <h1 className="hero-title">
          Every ward has a story about <em>water</em>
        </h1>
        <p className="hero-subtitle">
          A citizen-tracked record of piped municipal supply, tanker dependency, and pipeline
          work across Pune &mdash; kept honest by the people who live it.
        </p>
        <button className="hero-cta" onClick={scrollToContent}>
          {CTA_COPY[ctaVariant]}
        </button>
      </div>

      <button className="hero-scroll-hint" onClick={scrollToContent} aria-label="Scroll to map">
        <span>Scroll</span>
      </button>
    </section>
  )
}
