import { hasPermission } from '../features/admin/model'
import type { SystemPermission } from '../features/admin/model'

export interface NavEntry {
  key: string // the route path
  label: string
  section: 'main' | 'admin'
  // The entry is shown only to users who hold this permission.
  permission?: SystemPermission
}

export const NAV: readonly NavEntry[] = [
  { key: '/', label: 'Genel Bakış', section: 'main' },
  { key: '/clusters', label: "Cluster'lar", section: 'main' },
  { key: '/applications', label: 'Uygulamalar', section: 'main' },
  { key: '/events', label: 'Olaylar', section: 'main' },
  { key: '/admin/users', label: 'Kullanıcılar', section: 'admin', permission: 'user:manage' },
  { key: '/admin/teams', label: 'Ekipler', section: 'admin', permission: 'team:manage' },
  { key: '/admin/groups', label: 'Gruplar', section: 'admin', permission: 'group:manage' },
  { key: '/admin/access', label: 'Yetkiler', section: 'admin', permission: 'rbac:manage' },
  { key: '/admin/audit', label: 'Denetim', section: 'admin', permission: 'audit:view' },
]

export function visibleNav(perms: readonly string[] | undefined): { main: NavEntry[]; admin: NavEntry[] } {
  const shown = NAV.filter((e) => !e.permission || hasPermission(perms, e.permission))
  return { main: shown.filter((e) => e.section === 'main'), admin: shown.filter((e) => e.section === 'admin') }
}

// The menu entry that matches the current path: the longest entry whose path is a prefix on a path
// boundary, so "/admin/users/x" selects "/admin/users" and "/clustersX" selects nothing.
export function selectedNavKey(pathname: string): string {
  let best = ''
  for (const { key } of NAV) {
    const matches = key === '/' ? pathname === '/' : pathname === key || pathname.startsWith(key + '/')
    if (matches && key.length > best.length) best = key
  }
  return best
}
