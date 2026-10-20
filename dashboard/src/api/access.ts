import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { toError } from './errors'
import type { components } from './schema'

export type Role = components['schemas']['Role']
export type Binding = components['schemas']['Binding']
export type BindingInput = components['schemas']['BindingInput']
export type Ref = components['schemas']['Ref']

export function useRoles() {
  return useQuery({
    queryKey: ['roles'],
    staleTime: Infinity, // the built-in roles never change while the server runs
    queryFn: async () => {
      const { data, error } = await api.GET('/api/v1/roles')
      if (error) throw toError(error)
      return data.items
    },
  })
}

export interface BindingFilter {
  subjectKind?: components['schemas']['SubjectKind']
  subjectId?: string
}

export function useRoleBindings(filter: BindingFilter = {}) {
  return useQuery({
    queryKey: ['bindings', filter.subjectKind ?? '', filter.subjectId ?? ''],
    queryFn: async () => {
      const query = filter.subjectKind && filter.subjectId ? { subjectKind: filter.subjectKind, subjectId: filter.subjectId } : {}
      const { data, error } = await api.GET('/api/v1/role-bindings', { params: { query } })
      if (error) throw toError(error)
      return data.items
    },
  })
}

// Changing grants can change what the signed-in user may see or do.
function useRefreshAfterBindingChange() {
  const qc = useQueryClient()
  return () => Promise.all([['bindings'], ['me'], ['clusters']].map((queryKey) => qc.invalidateQueries({ queryKey })))
}

export function useCreateRoleBinding() {
  const refresh = useRefreshAfterBindingChange()
  return useMutation({
    mutationFn: async (input: BindingInput) => {
      const { data, error } = await api.POST('/api/v1/role-bindings', { body: input })
      if (error) throw toError(error)
      return data
    },
    onSuccess: refresh,
  })
}

export function useDeleteRoleBinding() {
  const refresh = useRefreshAfterBindingChange()
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE('/api/v1/role-bindings/{id}', { params: { path: { id } } })
      if (error) throw toError(error)
    },
    onSuccess: refresh,
  })
}
