import { useQueryClient } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import * as authApi from '../../api/auth'
import type { Session, User } from '../../api/auth'
import { tokenStore } from '../../api/tokenStore'

type Status = 'loading' | 'authenticated' | 'anonymous'

interface AuthContextValue {
  status: Status
  user: User | null
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  changePassword: (currentPassword: string, newPassword: string) => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

// Refresh this long before the access token expires.
const REFRESH_MARGIN_MS = 60_000
const MIN_REFRESH_DELAY_MS = 5_000

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<Status>('loading')
  const [user, setUser] = useState<User | null>(null)
  const timer = useRef<number | undefined>(undefined)

  const signedOut = useCallback(() => {
    window.clearTimeout(timer.current)
    tokenStore.clear()
    setUser(null)
    setStatus('anonymous')
    queryClient.clear()
  }, [queryClient])

  const apply = useCallback(
    function applySession(session: Session) {
      tokenStore.set(session.accessToken)
      setUser(session.user)
      setStatus('authenticated')

      window.clearTimeout(timer.current)
      const delay = Math.max(
        new Date(session.expiresAt).getTime() - Date.now() - REFRESH_MARGIN_MS,
        MIN_REFRESH_DELAY_MS,
      )
      timer.current = window.setTimeout(() => {
        authApi
          .refresh()
          .then((next) => (next ? applySession(next) : signedOut()))
          // A network hiccup must not sign the user out; the next 401 triggers another attempt.
          .catch(() => undefined)
      }, delay)
    },
    [signedOut],
  )

  // Restore the session after a page load from the refresh cookie.
  useEffect(() => {
    let cancelled = false
    authApi
      .refresh()
      .then((session) => {
        if (cancelled) return
        if (session) apply(session)
        else signedOut()
      })
      .catch(() => {
        if (!cancelled) signedOut()
      })
    return () => {
      cancelled = true
    }
  }, [apply, signedOut])

  // An authenticated request got a 401 (expired token, disabled account): try once to get a new
  // token, otherwise fall back to the login page.
  useEffect(() => {
    tokenStore.setUnauthorizedHandler(() => {
      authApi
        .refresh()
        .then((session) => {
          if (session) {
            apply(session)
            void queryClient.invalidateQueries()
          } else {
            signedOut()
          }
        })
        .catch(() => undefined)
    })
    return () => tokenStore.setUnauthorizedHandler(null)
  }, [apply, signedOut, queryClient])

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      login: async (username, password) => apply(await authApi.login(username, password)),
      logout: async () => {
        await authApi.logout()
        signedOut()
      },
      changePassword: async (currentPassword, newPassword) =>
        apply(await authApi.changePassword(currentPassword, newPassword)),
    }),
    [status, user, apply, signedOut],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>')
  return ctx
}
