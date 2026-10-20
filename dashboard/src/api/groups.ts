import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { toError } from './errors'
import type { components } from './schema'

export type ResourceGroup = components['schemas']['ResourceGroup']
export type ResourceGroupInput = components['schemas']['ResourceGroupInput']

export function useGroups() {
  return useQuery({
    queryKey: ['groups'],
    queryFn: async () => {
      const { data, error } = await api.GET('/api/v1/resource-groups')
      if (error) throw toError(error)
      return data.items
    },
  })
}

// Changing groups can change what the signed-in user may see, and which bindings still exist.
function useRefreshAfterGroupChange() {
  const qc = useQueryClient()
  return () =>
    Promise.all(
      [['groups'], ['bindings'], ['me'], ['clusters']].map((queryKey) => qc.invalidateQueries({ queryKey })),
    )
}

export function useCreateGroup() {
  const refresh = useRefreshAfterGroupChange()
  return useMutation({
    mutationFn: async (input: ResourceGroupInput) => {
      const { data, error } = await api.POST('/api/v1/resource-groups', { body: input })
      if (error) throw toError(error)
      return data
    },
    onSuccess: refresh,
  })
}

export function useUpdateGroup() {
  const refresh = useRefreshAfterGroupChange()
  return useMutation({
    mutationFn: async ({ id, name, description }: { id: string; name?: string; description?: string }) => {
      const { data, error } = await api.PATCH('/api/v1/resource-groups/{id}', {
        params: { path: { id } },
        body: { name, description },
      })
      if (error) throw toError(error)
      return data
    },
    onSuccess: refresh,
  })
}

export function useSetGroupMembers() {
  const refresh = useRefreshAfterGroupChange()
  return useMutation({
    mutationFn: async ({ id, members }: { id: string; members: string[] }) => {
      const { data, error } = await api.PUT('/api/v1/resource-groups/{id}/members', {
        params: { path: { id } },
        body: { members },
      })
      if (error) throw toError(error)
      return data
    },
    onSuccess: refresh,
  })
}

export function useDeleteGroup() {
  const refresh = useRefreshAfterGroupChange()
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE('/api/v1/resource-groups/{id}', { params: { path: { id } } })
      if (error) throw toError(error)
    },
    onSuccess: refresh,
  })
}
