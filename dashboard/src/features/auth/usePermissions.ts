import { useMe } from '../../api/me'
import { hasPermission } from '../admin/model'
import { useAuth } from './AuthProvider'

// What the signed-in user may do, for showing or hiding parts of the UI. It starts from the user in
// the session and is kept fresh by useMe(). This is a convenience only: the server checks every request.
export function usePermissions() {
  const { user: sessionUser } = useAuth()
  const me = useMe()
  const user = me.data ?? sessionUser
  return {
    user,
    can: (permission: string) => hasPermission(user?.permissions, permission),
    canCreateClusters: user?.canCreateClusters ?? false,
  }
}
