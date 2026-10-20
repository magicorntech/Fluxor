import { Alert, Table } from 'antd'
import type { TableProps } from 'antd'
import type { ReactNode } from 'react'
import { EmptyState } from './EmptyState'

export function DataTable<T extends object>({
  error,
  emptyText = 'Kayıt yok',
  emptyAction,
  ...props
}: TableProps<T> & {
  error?: Error | null
  emptyText?: string
  emptyAction?: ReactNode
}) {
  if (error) {
    return <Alert type="error" showIcon title="Veri yüklenemedi" description={error.message} />
  }

  return (
    <Table<T>
      size="middle"
      pagination={false}
      {...props}
      locale={{
        ...props.locale,
        emptyText: <EmptyState description={emptyText} action={emptyAction} />,
      }}
    />
  )
}
