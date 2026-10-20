// The access token lives only in memory: it is never written to localStorage or a cookie, so a
// script injected into the page cannot read it back from storage. A page reload starts without a
// token and gets a new one from the HttpOnly refresh cookie (see AuthProvider).
let accessToken: string | null = null
let onUnauthorized: (() => void) | null = null

export const tokenStore = {
  get: () => accessToken,
  set: (token: string) => {
    accessToken = token
  },
  clear: () => {
    accessToken = null
  },
  // Called when an authenticated request comes back 401.
  setUnauthorizedHandler: (fn: (() => void) | null) => {
    onUnauthorized = fn
  },
  notifyUnauthorized: () => onUnauthorized?.(),
}
