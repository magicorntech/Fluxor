import createClient from 'openapi-fetch'
import type { paths } from './schema'
import { tokenStore } from './tokenStore'

// Empty base URL = same origin (Vite proxy in dev, reverse proxy in prod).
// Set VITE_API_URL when the dashboard is served from a different origin than the server
// (the server must then list the dashboard origin in FLUXOR_CORS_ORIGINS).
// `credentials: 'include'` lets the browser send the HttpOnly refresh cookie in both setups.
export const api = createClient<paths>({
  baseUrl: import.meta.env.VITE_API_URL ?? '',
  credentials: 'include',
})

// These endpoints answer 401 for ordinary reasons (wrong password, no/expired refresh cookie),
// which must not be mistaken for an expired session.
const AUTH_FLOW_PATHS = new Set([
  '/api/v1/auth/login',
  '/api/v1/auth/refresh',
  '/api/v1/auth/logout',
  '/api/v1/auth/password',
])

api.use({
  onRequest({ request }) {
    const token = tokenStore.get()
    if (token) request.headers.set('Authorization', `Bearer ${token}`)
    return request
  },
  onResponse({ response, schemaPath }) {
    if (response.status === 401 && !AUTH_FLOW_PATHS.has(schemaPath)) tokenStore.notifyUnauthorized()
    return response
  },
})
