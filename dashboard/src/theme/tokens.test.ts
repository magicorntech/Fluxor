import { describe, expect, it } from 'vitest'
import { buildTheme, resolveMode } from './tokens'

describe('resolveMode', () => {
  it('follows system when preference is system', () => {
    expect(resolveMode('system', true)).toBe('dark')
    expect(resolveMode('system', false)).toBe('light')
  })

  it('honours an explicit preference', () => {
    expect(resolveMode('dark', false)).toBe('dark')
    expect(resolveMode('light', true)).toBe('light')
  })
})

describe('buildTheme', () => {
  it('uses teal primary and 6px radius in both modes', () => {
    for (const mode of ['light', 'dark'] as const) {
      const t = buildTheme(mode)
      expect(t.token?.colorPrimary).toBe('#0F766E')
      expect(t.token?.borderRadius).toBe(6)
    }
  })
})
