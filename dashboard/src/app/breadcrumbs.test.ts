import { describe, expect, it } from 'vitest'
import { breadcrumbsFor } from './breadcrumbs'

describe('breadcrumbsFor', () => {
  it('is only home on overview', () => {
    expect(breadcrumbsFor('/')).toEqual([{ path: '/', label: 'Genel Bakış' }])
  })

  it('adds the matching nav entry', () => {
    expect(breadcrumbsFor('/clusters')).toEqual([
      { path: '/', label: 'Genel Bakış' },
      { path: '/clusters', label: "Cluster'lar" },
    ])
  })

  it('matches nested admin paths', () => {
    const crumbs = breadcrumbsFor('/admin/users')
    expect(crumbs.at(-1)).toEqual({ path: '/admin/users', label: 'Kullanıcılar' })
  })
})
