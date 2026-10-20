import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { App, Button, Flex, Form, Input, Modal, Tag } from 'antd'
import { useState } from 'react'
import { useClusters, useCreateCluster, useDeleteCluster } from '../../api/hooks'
import type { Cluster, ClusterInput } from '../../api/hooks'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { DataTable } from '../../components/DataTable'
import { PageHeader } from '../../components/PageHeader'
import { StatusTag } from '../../components/StatusBadge'
import { usePermissions } from '../auth/usePermissions'

export function ClustersPage() {
  const { message } = App.useApp()
  const clusters = useClusters()
  const create = useCreateCluster()
  const remove = useDeleteCluster()
  const { canCreateClusters } = usePermissions()
  const [open, setOpen] = useState(false)
  const [form] = Form.useForm<ClusterInput>()

  const submit = (values: ClusterInput) => {
    create.mutate(values, {
      onSuccess: () => {
        message.success('Cluster eklendi')
        setOpen(false)
        form.resetFields()
      },
      onError: (e) => message.error(e.message),
    })
  }

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="Cluster'lar"
        description="Kayıtlı Kubernetes cluster'ları. Canlı bağlantı ve detay sayfası sonraki adımda gelecek."
        extra={
          canCreateClusters ? (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>
              Cluster ekle
            </Button>
          ) : undefined
        }
      />

      <DataTable<Cluster>
        rowKey="id"
        loading={clusters.isLoading}
        dataSource={clusters.data}
        error={clusters.isError ? clusters.error : null}
        emptyText="Henüz cluster yok"
        emptyAction={
          canCreateClusters ? (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>
              Cluster ekle
            </Button>
          ) : undefined
        }
        columns={[
          { title: 'Ad', dataIndex: 'name', render: (v: string) => <strong>{v}</strong> },
          { title: 'API Server', dataIndex: 'apiServer' },
          {
            title: 'Durum',
            dataIndex: 'status',
            render: (s: Cluster['status']) => <StatusTag status={s} />,
          },
          { title: 'Sürüm', dataIndex: 'version', render: (v?: string) => v || '—' },
          {
            title: 'Etiketler',
            dataIndex: 'labels',
            render: (labels: Cluster['labels']) =>
              Object.keys(labels).length === 0
                ? '—'
                : Object.entries(labels).map(([k, v]) => (
                    <Tag key={k} style={{ marginBottom: 2 }}>
                      {`${k}=${v}`}
                    </Tag>
                  )),
          },
          {
            title: '',
            key: 'actions',
            width: 56,
            render: (_, c) =>
              c.permissions.includes('cluster:manage') ? (
                <ConfirmDelete
                  name={c.name}
                  onConfirm={() =>
                    remove.mutate(c.id, {
                      onSuccess: () => message.success('Cluster silindi'),
                      onError: (e) => message.error(e.message),
                    })
                  }
                >
                  <Button type="text" danger icon={<DeleteOutlined />} aria-label="Sil" />
                </ConfirmDelete>
              ) : null,
          },
        ]}
      />

      <Modal
        title="Cluster ekle"
        open={open}
        okText="Ekle"
        cancelText="Vazgeç"
        confirmLoading={create.isPending}
        destroyOnHidden
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
      >
        <Form form={form} layout="vertical" onFinish={submit} preserve={false}>
          <Form.Item
            name="name"
            label="Ad"
            rules={[
              { required: true, message: 'Ad gerekli' },
              { pattern: /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/, message: 'Küçük harf, rakam ve "-" kullanın' },
            ]}
          >
            <Input placeholder="prod-eu" />
          </Form.Item>
          <Form.Item
            name="apiServer"
            label="API Server URL"
            rules={[{ required: true, type: 'url', message: 'Geçerli bir URL girin' }]}
          >
            <Input placeholder="https://10.0.0.1:6443" />
          </Form.Item>
        </Form>
      </Modal>
    </Flex>
  )
}
