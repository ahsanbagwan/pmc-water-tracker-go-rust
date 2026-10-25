import { useEffect, useState } from 'react'

export function useMouseParallax(strength = 10) {
  const [offset, setOffset] = useState({ x: 0, y: 0 })

  useEffect(() => {
    const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (prefersReducedMotion) return

    function handleMouseMove(e) {
      // Normalize to -1..1 based on position within the viewport, then scale by `strength`
      // (max pixels of movement) -- small, subtle, not a dramatic tilt effect.
      const x = (e.clientX / window.innerWidth - 0.5) * 2
      const y = (e.clientY / window.innerHeight - 0.5) * 2
      setOffset({ x: x * strength, y: y * strength })
    }

    window.addEventListener('mousemove', handleMouseMove)
    return () => window.removeEventListener('mousemove', handleMouseMove)
  }, [strength])

  return offset
}
