import { useEffect, useRef, useState } from 'react'

export function useCountUp(targetValue, durationMs = 500) {
  const [displayValue, setDisplayValue] = useState(targetValue)
  const previousValue = useRef(targetValue)

  useEffect(() => {
    const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (prefersReducedMotion) {
      setDisplayValue(targetValue)
      previousValue.current = targetValue
      return
    }

    const startValue = previousValue.current
    const delta = targetValue - startValue
    if (delta === 0) return

    const startTime = performance.now()
    let frameId

    function tick(now) {
      const elapsed = now - startTime
      const progress = Math.min(elapsed / durationMs, 1)
      // Ease-out: fast at first, settling in gently -- reads as smoother than a linear count
      const eased = 1 - Math.pow(1 - progress, 3)
      setDisplayValue(Math.round(startValue + delta * eased))

      if (progress < 1) {
        frameId = requestAnimationFrame(tick)
      } else {
        previousValue.current = targetValue
      }
    }

    frameId = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frameId)
  }, [targetValue, durationMs])

  return displayValue
}
