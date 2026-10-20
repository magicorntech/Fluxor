import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { useMe } from '../../api/me'
import { permissionSignature } from '../admin/model'

// Mount once, high in the tree. When the signed-in user's permissions or teams change (an
// administrator granted or revoked something), everything else on screen may show data the user can
// no longer see, or miss data they now can, so all other queries are fetched again.
export function usePermissionSync() {
  const qc = useQueryClient()
  const { data } = useMe()
  const signature = data ? permissionSignature(data) : undefined
  const previous = useRef<string | undefined>(undefined)

  useEffect(() => {
    if (signature === undefined) return
    if (previous.current !== undefined && previous.current !== signature) {
      void qc.invalidateQueries({ predicate: (q) => q.queryKey[0] !== 'me' })
    }
    previous.current = signature
  }, [signature, qc])
}
