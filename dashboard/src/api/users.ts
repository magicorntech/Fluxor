import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { toError } from './errors'
import type { components } from './schema'

export type AdminUser = components['schemas']['AdminUser']
export type CreateUserInput = components['schemas']['CreateUserInput']
export type CreateUserResponse = components['schemas']['CreateUserResponse']
export type UpdateUserInput = components['schemas']['UpdateUserInput']

export function useUsers() {
  return useQuery({
    queryKey: ['users'],
    queryFn: async () => {
      const { data, error } = await api.GET('/api/v1/users')
      if (error) throw toError(error)
      return data.items
    },
  })
}

export function useCreateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: CreateUserInput) => {
      const { data, error } = await api.POST('/api/v1/users', { body: input })
      if (error) throw toError(error)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useUpdateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, ...body }: UpdateUserInput & { id: string }) => {
      const { data, error } = await api.PATCH('/api/v1/users/{id}', { params: { path: { id } }, body })
      if (error) throw toError(error)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useResetPassword() {
  return useMutation({
    mutationFn: async ({ id, password }: { id: string; password?: string }) => {
      const { data, error } = await api.POST('/api/v1/users/{id}/reset-password', {
        params: { path: { id } },
        body: { password },
      })
      if (error) throw toError(error)
      return data
    },
  })
}
