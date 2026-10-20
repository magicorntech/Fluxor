import { Flex } from 'antd'
import { EmptyState } from '../../components/EmptyState'
import { PageHeader } from '../../components/PageHeader'

export function EventsPage() {
  return (
    <Flex vertical gap={16}>
      <PageHeader title="Olaylar" description="Canlı etkinlik akışı (SSE) Adım E'de gelecek." />
      <EmptyState description="Canlı olay akışı henüz yok" />
    </Flex>
  )
}
