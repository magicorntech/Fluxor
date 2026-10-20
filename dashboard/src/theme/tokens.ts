import type { ThemeConfig } from 'antd'
import { theme as antdTheme } from 'antd'

/** Shared Fluxor look: slate neutrals + teal accent (not default antd blue / purple). */
const brand = {
  colorPrimary: '#0F766E',
  colorSuccess: '#15803D',
  colorWarning: '#C2410C',
  colorError: '#B91C1C',
  colorInfo: '#0369A1',
  borderRadius: 6,
  fontSize: 14,
  controlHeight: 32,
  fontFamily:
    '"IBM Plex Sans", "Segoe UI", system-ui, -apple-system, sans-serif',
  fontFamilyCode: '"IBM Plex Mono", "SF Mono", ui-monospace, monospace',
}

export type ThemeMode = 'light' | 'dark'

export function buildTheme(mode: ThemeMode): ThemeConfig {
  const dark = mode === 'dark'
  return {
    algorithm: dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    token: {
      ...brand,
      colorBgLayout: dark ? '#0B1220' : '#F1F5F9',
      colorBgContainer: dark ? '#111827' : '#FFFFFF',
      colorBorder: dark ? '#1F2937' : '#E2E8F0',
      colorText: dark ? '#E5E7EB' : '#0F172A',
      colorTextSecondary: dark ? '#9CA3AF' : '#64748B',
    },
    components: {
      Layout: {
        headerBg: dark ? '#111827' : '#FFFFFF',
        siderBg: dark ? '#0F172A' : '#FFFFFF',
        bodyBg: dark ? '#0B1220' : '#F1F5F9',
        triggerBg: dark ? '#1E293B' : '#E2E8F0',
      },
      Menu: {
        itemBorderRadius: 6,
        itemMarginInline: 8,
        iconSize: 16,
      },
      Table: {
        cellPaddingBlock: 10,
        cellPaddingInline: 12,
        headerBorderRadius: 6,
      },
      Card: {
        paddingLG: 20,
      },
      Button: {
        borderRadius: 6,
      },
    },
  }
}

export type ThemePreference = 'light' | 'dark' | 'system'

export const THEME_STORAGE_KEY = 'fluxor-theme'

export function resolveMode(pref: ThemePreference, systemDark: boolean): ThemeMode {
  if (pref === 'system') return systemDark ? 'dark' : 'light'
  return pref
}

export function readThemePreference(): ThemePreference {
  try {
    const v = localStorage.getItem(THEME_STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* ignore */
  }
  return 'system'
}

export function writeThemePreference(pref: ThemePreference) {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, pref)
  } catch {
    /* ignore */
  }
}
