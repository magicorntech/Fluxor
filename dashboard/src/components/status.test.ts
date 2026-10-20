import { describe, expect, it } from 'vitest'
import { clusterStatus, healthStatus, syncStatus } from './status'

describe('status maps', () => {
  it('maps cluster statuses', () => {
    expect(clusterStatus('Connected')).toEqual({ tone: 'success', label: 'Bağlı' })
    expect(clusterStatus('Unreachable').tone).toBe('error')
  })

  it('maps health and sync for future screens', () => {
    expect(healthStatus('Degraded').tone).toBe('error')
    expect(syncStatus('OutOfSync').tone).toBe('warning')
  })

  it('falls back for unknown codes', () => {
    expect(clusterStatus('Weird').label).toBe('Weird')
  })
})
