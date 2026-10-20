import { Flex, Typography } from 'antd'
import type { ReactNode } from 'react'

export function PageHeader({
  title,
  description,
  extra,
}: {
  title: ReactNode
  description?: ReactNode
  extra?: ReactNode
}) {
  return (
    <Flex justify="space-between" align="flex-start" gap={16} wrap="wrap" style={{ marginBottom: 4 }}>
      <Flex vertical gap={4} style={{ minWidth: 0, flex: 1 }}>
        <Typography.Title level={3} style={{ margin: 0 }}>
          {title}
        </Typography.Title>
        {description ? (
          <Typography.Text type="secondary" style={{ maxWidth: 720 }}>
            {description}
          </Typography.Text>
        ) : null}
      </Flex>
      {extra ? <Flex gap={8} wrap="wrap" align="center">{extra}</Flex> : null}
    </Flex>
  )
}
