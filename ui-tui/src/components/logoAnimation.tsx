import { Box, Text } from '@hermes/ink'
import { useEffect, useState } from 'react'

import { LOGO_WIDTH, logo } from '../banner.js'
import type { Theme } from '../theme.js'

/**
 * Animated REAMES logo with a shimmer wave sweeping left to right.
 *
 * The animation divides the logo into "phases" — each tick advances the
 * wave, and characters within the active band get highlighted with a
 * brighter color.
 */

// How many animation frames across the logo width
const PHASES = 24
const TICK_MS = 80

// The shimmer profile: a wave with a bright peak, trailing off behind
const buildBrightnessProfile = (phase: number, totalPhases: number): number[] => {
  const profile: number[] = []

  for (let i = 0; i < totalPhases; i++) {
    // Distance from wave center (phase)
    let dist = (i - phase + totalPhases) % totalPhases

    if (dist > totalPhases / 2) {
      dist = totalPhases - dist
    }

    // Normalized distance [0..1]
    const t = dist / (totalPhases / 2)
    // Gaussian-like falloff
    const brightness = Math.exp(-t * t * 3) * 0.4 + 0.6

    profile.push(Math.min(1, brightness))
  }

  return profile
}

// Interpolate between two hex colors
const mixColor = (a: string, b: string, t: number): string => {
  const ha = a.replace('#', '')
  const hb = b.replace('#', '')
  const ra = parseInt(ha.substring(0, 2), 16)
  const ga = parseInt(ha.substring(2, 4), 16)
  const ba = parseInt(ha.substring(4, 6), 16)
  const rb = parseInt(hb.substring(0, 2), 16)
  const gb = parseInt(hb.substring(2, 4), 16)
  const bb = parseInt(hb.substring(4, 6), 16)

  const r = Math.round(ra + (rb - ra) * t)
  const g = Math.round(ga + (gb - ga) * t)
  const blue = Math.round(ba + (bb - ba) * t)

  return `#${r.toString(16).padStart(2, '0')}${g.toString(16).padStart(2, '0')}${blue.toString(16).padStart(2, '0')}`
}

export function AnimatedLogo({ t }: { t: Theme }) {
  const lines = logo(t.color)
  const [phase, setPhase] = useState(0)

  useEffect(() => {
    const id = setInterval(() => {
      setPhase(p => (p + 1) % PHASES)
    }, TICK_MS)

    return () => clearInterval(id)
  }, [])

  const brightnessProfile = buildBrightnessProfile(phase, PHASES)
  const columnsPerPhase = Math.ceil(LOGO_WIDTH / PHASES)

  return (
    <Box flexDirection="column" alignItems="center" width={LOGO_WIDTH + 4}>
      {lines.map(([baseColor, text], rowIdx) => {
        const cols = Math.max(1, Math.ceil(text.length / columnsPerPhase))

        return (
          <Box key={rowIdx} height={1}>
            {Array.from({ length: cols }, (_, colIdx) => {
              const phaseIdx = (colIdx + Math.floor(phase * columnsPerPhase / 2)) % PHASES
              const brightness = brightnessProfile[phaseIdx] ?? 0.6
              const segmentStart = colIdx * columnsPerPhase
              const segment = text.slice(segmentStart, segmentStart + columnsPerPhase)

              if (!segment) {
                return null
              }

              // Mix base color toward white based on brightness
              const lit = brightness > 0.85
                ? mixColor(baseColor, '#ffffff', (brightness - 0.85) * 6)
                : baseColor

              return (
                <Text key={colIdx} color={lit} bold={brightness > 0.9}>
                  {segment}
                </Text>
              )
            })}
          </Box>
        )
      })}
    </Box>
  )
}
