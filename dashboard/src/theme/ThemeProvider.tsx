import { ConfigProvider, App as AntApp } from 'antd'
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import {
  buildTheme,
  readThemePreference,
  resolveMode,
  writeThemePreference,
  type ThemeMode,
  type ThemePreference,
} from './tokens'

interface ThemeContextValue {
  preference: ThemePreference
  mode: ThemeMode
  setPreference: (p: ThemePreference) => void
  cyclePreference: () => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

function systemPrefersDark(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(() => readThemePreference())
  const [systemDark, setSystemDark] = useState(systemPrefersDark)

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => setSystemDark(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const mode = resolveMode(preference, systemDark)

  useEffect(() => {
    document.documentElement.dataset.theme = mode
    document.documentElement.style.colorScheme = mode
  }, [mode])

  const setPreference = (p: ThemePreference) => {
    writeThemePreference(p)
    setPreferenceState(p)
  }

  const cyclePreference = () => {
    const order: ThemePreference[] = ['system', 'light', 'dark']
    const i = order.indexOf(preference)
    setPreference(order[(i + 1) % order.length]!)
  }

  return (
    <ThemeContext.Provider value={{ preference, mode, setPreference, cyclePreference }}>
      <ConfigProvider theme={buildTheme(mode)}>
        <AntApp>{children}</AntApp>
      </ConfigProvider>
    </ThemeContext.Provider>
  )
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider')
  return ctx
}
