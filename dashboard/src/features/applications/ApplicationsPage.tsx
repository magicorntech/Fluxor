import { Flex } from 'antd'
import { EmptyState } from '../../components/EmptyState'
import { PageHeader } from '../../components/PageHeader'

export function ApplicationsPage() {
  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="Uygulamalar"
        description="Git kaynağı + cluster hedefleri. Oluşturma sihirbazı Adım D'de gelecek."
      />
      <EmptyState description="Uygulama yönetimi sonraki adımda (D) gelecek" />
    </Flex>
  )
}
