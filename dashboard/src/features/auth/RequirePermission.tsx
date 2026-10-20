import { Button, Result } from 'antd'
import type { ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import type { SystemPermission } from '../admin/model'
import { usePermissions } from './usePermissions'

// Shows the page only to users who hold the permission. Direct visits to a hidden page by URL end here.
export function RequirePermission({ permission, children }: { permission: SystemPermission; children: ReactNode }) {
  const { can } = usePermissions()
  const navigate = useNavigate()
  if (!can(permission)) {
    return (
      <Result
        status="403"
        title="Bu sayfaya erişim yetkiniz yok"
        subTitle="Gerekli izne sahip olmadığınız için bu sayfayı kullanamazsınız."
        extra={<Button onClick={() => navigate('/')}>Genel Bakış'a dön</Button>}
      />
    )
  }
  return children
}
