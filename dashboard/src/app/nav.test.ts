import { describe, expect, it } from 'vitest'
import { selectedNavKey, visibleNav } from './nav'

const keys = (entries: { key: string }[]) => entries.map((e) => e.key)

describe('visibleNav', () => {
  it('shows only the general pages to a user without system permissions', () => {
    const nav = visibleNav([])
    expect(keys(nav.main)).toEqual(['/', '/clusters', '/applications', '/events'])
    expect(nav.admin).toEqual([])
  })
  it('shows exactly the administration pages the user holds the permission for', () => {
    expect(keys(visibleNav(['team:manage', 'audit:view']).admin)).toEqual(['/admin/teams', '/admin/audit'])
    expect(keys(visibleNav(['group:manage']).admin)).toEqual(['/admin/groups'])
    expect(keys(visibleNav(['user:manage', 'rbac:manage']).admin)).toEqual(['/admin/users', '/admin/access'])
  })
  it('shows all administration pages to an administrator', () => {
    const all = ['user:manage', 'team:manage', 'group:manage', 'rbac:manage', 'audit:view']
    expect(keys(visibleNav(all).admin)).toEqual(['/admin/users', '/admin/teams', '/admin/groups', '/admin/access', '/admin/audit'])
  })
  it('copes with no permission list at all', () => {
    expect(visibleNav(undefined).admin).toEqual([])
  })
})

describe('selectedNavKey', () => {
  it('matches the overview only on the root path', () => {
    expect(selectedNavKey('/')).toBe('/')
    expect(selectedNavKey('/nope')).toBe('')
  })
  it('matches an entry and its sub-paths', () => {
    expect(selectedNavKey('/clusters')).toBe('/clusters')
    expect(selectedNavKey('/clusters/abc')).toBe('/clusters')
    expect(selectedNavKey('/admin/users')).toBe('/admin/users')
    expect(selectedNavKey('/admin/users/42')).toBe('/admin/users')
  })
  it('does not match a path that merely starts with the same letters', () => {
    expect(selectedNavKey('/clustersX')).toBe('')
    expect(selectedNavKey('/admin/usersX')).toBe('')
  })
  it('selects nothing on the account page, which has no menu entry', () => {
    expect(selectedNavKey('/account/password')).toBe('')
  })
})
