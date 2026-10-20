import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { toError } from './errors'
import type { components } from './schema'

export type Team = components['schemas']['Team']
export type TeamInput = components['schemas']['TeamInput']

export function useTeams() {
  return useQuery({
    queryKey: ['teams'],
    queryFn: async () => {
      const { data, error } = await api.GET('/api/v1/teams')
      if (error) throw toError(error)
      return data.items
    },
  })
}

// Changing teams can change what the signed-in user, and the role bindings listing, may see.
function useRefreshAfterTeamChange() {
  const qc = useQueryClient()
  return () =>
    Promise.all(
      [['teams'], ['bindings'], ['me'], ['clusters']].map((queryKey) => qc.invalidateQueries({ queryKey })),
    )
}

export function useCreateTeam() {
  const refresh = useRefreshAfterTeamChange()
  return useMutation({
    mutationFn: async (input: TeamInput) => {
      const { data, error } = await api.POST('/api/v1/teams', { body: input })
      if (error) throw toError(error)
      return data
    },
    onSuccess: refresh,
  })
}

export function useUpdateTeam() {
  const refresh = useRefreshAfterTeamChange()
  return useMutation({
    mutationFn: async ({ id, name, description }: { id: string; name?: string; description?: string }) => {
      const { data, error } = await api.PATCH('/api/v1/teams/{id}', {
        params: { path: { id } },
        body: { name, description },
      })
      if (error) throw toError(error)
      return data
    },
    onSuccess: refresh,
  })
}

export function useSetTeamMembers() {
  const refresh = useRefreshAfterTeamChange()
  return useMutation({
    mutationFn: async ({ id, members }: { id: string; members: string[] }) => {
      const { data, error } = await api.PUT('/api/v1/teams/{id}/members', {
        params: { path: { id } },
        body: { members },
      })
      if (error) throw toError(error)
      return data
    },
    onSuccess: refresh,
  })
}

export function useDeleteTeam() {
  const refresh = useRefreshAfterTeamChange()
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE('/api/v1/teams/{id}', { params: { path: { id } } })
      if (error) throw toError(error)
    },
    onSuccess: refresh,
  })
}
