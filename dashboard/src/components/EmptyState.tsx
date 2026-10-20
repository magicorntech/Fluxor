import { Empty, Flex } from 'antd'
import type { ReactNode } from 'react'

export function EmptyState({
  description,
  action,
  image,
}: {
  description: ReactNode
  action?: ReactNode
  image?: ReactNode
}) {
  return (
    <Flex vertical align="center" justify="center" style={{ paddingBlock: 48 }}>
      <Empty image={image ?? Empty.PRESENTED_IMAGE_SIMPLE} description={description}>
        {action}
      </Empty>
    </Flex>
  )
}
