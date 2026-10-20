import { useQuery } from '@tanstack/react-query'
import { api } from './client'
import { toError } from './errors'
import type { components } from './schema'

export type Me = components['schemas']['User']

// The signed-in user with their permission summary. It is refetched regularly and when the window
// regains focus, so that a change made by an administrator reaches the UI without a reload. The
// server still checks every request; this only decides what the UI offers.
export function useMe() {
  return useQuery({
    queryKey: ['me'],
    staleTime: 30_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: 'always',
    queryFn: async () => {
      const { data, error } = await api.GET('/api/v1/auth/me')
      if (error) throw toError(error)
      return data
    },
  })
}
