import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { toError } from './errors'
import type { components } from './schema'

export type ApiToken = components['schemas']['ApiToken']
export type CreatedApiToken = components['schemas']['CreatedApiToken']

export function useTokens() {
  return useQuery({
    queryKey: ['tokens'],
    queryFn: async () => {
      const { data, error } = await api.GET('/api/v1/tokens')
      if (error) throw toError(error)
      return data.items
    },
  })
}

export function useCreateToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { name: string; expiresInDays: number }) => {
      const { data, error } = await api.POST('/api/v1/tokens', { body: input })
      if (error) throw toError(error)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tokens'] }),
  })
}

export function useDeleteToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE('/api/v1/tokens/{id}', { params: { path: { id } } })
      if (error) throw toError(error)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tokens'] }),
  })
}
