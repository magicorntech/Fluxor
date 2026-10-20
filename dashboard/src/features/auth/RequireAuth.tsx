import { Flex, Spin } from 'antd'
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from './AuthProvider'

export const PASSWORD_PATH = '/account/password'

// Guards the routes inside it: anonymous visitors go to the login page, and a user who still has
// to change the initial password can only reach the password page.
export function RequireAuth() {
  const { status, user } = useAuth()
  const location = useLocation()

  if (status === 'loading') {
    return (
      <Flex justify="center" align="center" style={{ minHeight: '100vh' }}>
        <Spin size="large" />
      </Flex>
    )
  }
  if (status === 'anonymous' || !user) {
    return <Navigate to="/login" replace state={{ from: location }} />
  }
  if (user.mustChangePassword && location.pathname !== PASSWORD_PATH) {
    return <Navigate to={PASSWORD_PATH} replace />
  }
  return <Outlet />
}
