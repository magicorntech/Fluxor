import { Popconfirm } from 'antd'
import type { ReactNode } from 'react'

export function ConfirmDelete({
  name,
  onConfirm,
  children,
  okText = 'Sil',
  description,
}: {
  name: string
  onConfirm: () => void
  children: ReactNode
  okText?: string
  description?: ReactNode
}) {
  return (
    <Popconfirm
      title={`${name} silinsin mi?`}
      description={description}
      okText={okText}
      cancelText="Vazgeç"
      okButtonProps={{ danger: true }}
      onConfirm={onConfirm}
    >
      {children}
    </Popconfirm>
  )
}
