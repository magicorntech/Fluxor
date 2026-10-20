import { api } from './client'
import { toError } from './errors'
import { tokenStore } from './tokenStore'
import type { components } from './schema'

export type Session = components['schemas']['Session']
export type User = components['schemas']['User']

// Required on cookie-authenticated calls (CSRF defence); the server only checks it is present.
const csrf = { 'X-Fluxor-Request': '1' } as const

export async function login(username: string, password: string): Promise<Session> {
  const { data, error } = await api.POST('/api/v1/auth/login', { body: { username, password } })
  if (error) throw toError(error)
  return data
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<Session> {
  const { data, error } = await api.PUT('/api/v1/auth/password', { body: { currentPassword, newPassword } })
  if (error) throw toError(error)
  return data
}

export async function logout(): Promise<void> {
  // Best effort: the local session is dropped even if the server cannot be reached.
  await api.POST('/api/v1/auth/logout', { params: { header: csrf } }).catch(() => undefined)
}

// A refresh token works only once, so concurrent callers (React StrictMode's double effect,
// the expiry timer, a 401 handler) must share one request instead of racing each other.
let inflight: Promise<Session | null> | null = null

// Returns the new session, or null when there is no valid refresh cookie (= signed out).
export function refresh(): Promise<Session | null> {
  inflight ??= doRefresh().finally(() => {
    inflight = null
  })
  return inflight
}

const RETRY_DELAY_MS = 400

async function doRefresh(): Promise<Session | null> {
  // Without an access token there was no session to begin with (a fresh page load), so a 401 is final.
  // With one, a 401 may only mean that another tab of this browser exchanged the shared cookie a moment
  // ago and our request carried the old value: the cookie has been replaced since, so try once more.
  const hadSession = tokenStore.get() !== null
  for (let attempt = 0; ; attempt++) {
    const { data, response, error } = await api.POST('/api/v1/auth/refresh', { params: { header: csrf } })
    if (response.status !== 401) {
      if (error) throw toError(error)
      return data
    }
    if (!hadSession || attempt > 0) return null
    await new Promise((resolve) => setTimeout(resolve, RETRY_DELAY_MS))
  }
}
