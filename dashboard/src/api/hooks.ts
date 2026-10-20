import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { toError } from './errors'
import type { components } from './schema'

export type Cluster = components['schemas']['Cluster']
export type ClusterInput = components['schemas']['ClusterInput']

export function useHealth() {
  return useQuery({
    queryKey: ['health'],
    refetchInterval: 10_000,
    retry: false,
    queryFn: async () => {
      const { data, error } = await api.GET('/healthz')
      if (error) throw toError(error)
      return data
    },
  })
}

export function useClusters() {
  return useQuery({
    queryKey: ['clusters'],
    queryFn: async () => {
      const { data, error } = await api.GET('/api/v1/clusters')
      if (error) throw toError(error)
      return data.items
    },
  })
}

export function useCreateCluster() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: ClusterInput) => {
      const { data, error } = await api.POST('/api/v1/clusters', { body: input })
      if (error) throw toError(error)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['clusters'] }),
  })
}

export function useDeleteCluster() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE('/api/v1/clusters/{id}', { params: { path: { id } } })
      if (error) throw toError(error)
    },
    // Deleting a cluster also removes it from its groups and drops the grants that pointed at it.
    onSuccess: () =>
      Promise.all(['clusters', 'groups', 'bindings'].map((key) => qc.invalidateQueries({ queryKey: [key] }))),
  })
}
