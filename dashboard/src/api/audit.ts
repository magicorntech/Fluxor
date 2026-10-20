import { useInfiniteQuery } from '@tanstack/react-query'
import { api } from './client'
import { toError } from './errors'
import type { components } from './schema'

export type AuditEvent = components['schemas']['AuditEvent']
export type AuditResult = components['schemas']['AuditResult']

export interface AuditFilter {
  actor?: string
  action?: string
  result?: AuditResult
  from?: string // RFC 3339
  to?: string
}

const PAGE_SIZE = 50

// The audit log, newest first, fetched a page at a time. The server's nextCursor is the cursor of the
// following page; when it is absent there is nothing older.
export function useAuditEvents(filter: AuditFilter) {
  return useInfiniteQuery({
    queryKey: ['audit', filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET('/api/v1/audit', {
        params: { query: { ...filter, limit: PAGE_SIZE, cursor: pageParam } },
      })
      if (error) throw toError(error)
      return data
    },
    getNextPageParam: (last) => last.nextCursor,
  })
}
