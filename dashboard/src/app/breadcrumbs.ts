import { NAV } from './nav'

const EXTRA: Record<string, string> = {
  '/account/tokens': "API token'larım",
  '/account/password': 'Parolayı değiştir',
  '/login': 'Giriş',
}

/** Breadcrumb trail for the current path (home is always first when not on `/`). */
export function breadcrumbsFor(pathname: string): { path: string; label: string }[] {
  const crumbs: { path: string; label: string }[] = [{ path: '/', label: 'Genel Bakış' }]
  if (pathname === '/' || pathname === '') return crumbs

  const known = [...NAV.map((e) => ({ path: e.key, label: e.label })), ...Object.entries(EXTRA).map(([path, label]) => ({ path, label }))]
  let best: { path: string; label: string } | undefined
  for (const e of known) {
    if (e.path === '/') continue
    const matches = pathname === e.path || pathname.startsWith(e.path + '/')
    if (matches && (!best || e.path.length > best.path.length)) best = e
  }
  if (best) crumbs.push(best)
  return crumbs
}
