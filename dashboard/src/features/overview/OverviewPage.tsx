import { ClusterOutlined, CloudServerOutlined, LinkOutlined } from '@ant-design/icons'
import { Alert, Card, Col, Flex, Row, Statistic, theme } from 'antd'
import { PageHeader } from '../../components/PageHeader'
import { useClusters, useHealth } from '../../api/hooks'

export function OverviewPage() {
  const { token } = theme.useToken()
  const health = useHealth()
  const clusters = useClusters()
  const items = clusters.data ?? []
  const connected = items.filter((c) => c.status === 'Connected').length

  return (
    <Flex vertical gap={20}>
      <PageHeader
        title="Genel Bakış"
        description="Filo sağlığı ve senkron grafikleri uygulamalar geldiğinde burada görünecek."
      />

      {health.isError && (
        <Alert
          type="error"
          showIcon
          title="Server'a ulaşılamıyor"
          description="fluxor-server çalışıyor mu? (make server / make dev-server)"
        />
      )}

      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={8}>
          <Card size="small">
            <Statistic
              title="Cluster"
              value={items.length}
              loading={clusters.isLoading}
              prefix={<ClusterOutlined style={{ color: token.colorPrimary }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={8}>
          <Card size="small">
            <Statistic
              title="Bağlı cluster"
              value={connected}
              loading={clusters.isLoading}
              prefix={<LinkOutlined style={{ color: token.colorSuccess }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={8}>
          <Card size="small">
            <Statistic
              title="Server"
              value={health.data ? `v${health.data.version}` : '—'}
              loading={health.isLoading}
              prefix={<CloudServerOutlined style={{ color: token.colorInfo }} />}
            />
          </Card>
        </Col>
      </Row>
    </Flex>
  )
}
