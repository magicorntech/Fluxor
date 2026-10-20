import { PlusOutlined } from '@ant-design/icons'
import { App, Button, Flex, Form, Input, Modal, Select, Space, Tag, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useClusters } from '../../api/hooks'
import { useCreateGroup, useDeleteGroup, useGroups, useSetGroupMembers, useUpdateGroup } from '../../api/groups'
import type { ResourceGroup } from '../../api/groups'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { DataTable } from '../../components/DataTable'
import { MembersModal } from '../../components/MembersModal'
import { PageHeader } from '../../components/PageHeader'
import { formatTime, namesOf, sortByLabel, toLookup } from './model'

const MAX_TAGS = 4

export function GroupsPage() {
  const { message } = App.useApp()
  const groups = useGroups()
  const clusters = useClusters()
  const remove = useDeleteGroup()
  const setMembers = useSetGroupMembers()
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<ResourceGroup | null>(null)
  const [membersOf, setMembersOf] = useState<ResourceGroup | null>(null)

  const clusterNames = useMemo(() => toLookup(clusters.data, (c) => c.name), [clusters.data])
  const clusterOptions = useMemo(
    () => sortByLabel(clusters.data ?? [], (c) => c.name).map((c) => ({ value: c.id, label: c.name })),
    [clusters.data],
  )

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="Gruplar"
        description="Bir gruba yetki verildiğinde gruptaki tüm cluster'lar kapsanır. Uygulama grupları uygulamalarla gelecek."
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
            Yeni grup
          </Button>
        }
      />

      <DataTable<ResourceGroup>
        rowKey="id"
        loading={groups.isLoading}
        dataSource={groups.data}
        error={groups.isError ? groups.error : null}
        emptyText="Henüz grup yok"
        columns={[
          { title: 'Ad', dataIndex: 'name', render: (v: string) => <Typography.Text strong>{v}</Typography.Text> },
          { title: 'Tür', render: () => <Tag>Cluster grubu</Tag> },
          { title: 'Açıklama', dataIndex: 'description', render: (v?: string) => v || '—' },
          {
            title: "Cluster'lar",
            render: (_, g) =>
              g.members.length === 0 ? (
                <Typography.Text type="secondary">Cluster yok</Typography.Text>
              ) : (
                <Space size={[0, 4]} wrap>
                  {namesOf(g.members, clusterNames).slice(0, MAX_TAGS).map((name) => (
                    <Tag key={name}>{name}</Tag>
                  ))}
                  {g.members.length > MAX_TAGS && <Tag>+{g.members.length - MAX_TAGS}</Tag>}
                </Space>
              ),
          },
          { title: 'Oluşturulma', dataIndex: 'createdAt', render: formatTime },
          {
            title: '',
            key: 'actions',
            render: (_, g) => (
              <Space size={0} wrap>
                <Button type="link" size="small" onClick={() => setMembersOf(g)}>
                  Cluster'lar
                </Button>
                <Button type="link" size="small" onClick={() => setEditing(g)}>
                  Düzenle
                </Button>
                <ConfirmDelete
                  name={g.name}
                  description="Bu gruba bağlı yetkiler de silinir."
                  onConfirm={() =>
                    remove.mutate(g.id, {
                      onSuccess: () => message.success('Grup silindi'),
                      onError: (e) => message.error(e.message),
                    })
                  }
                >
                  <Button type="link" size="small" danger>
                    Sil
                  </Button>
                </ConfirmDelete>
              </Space>
            ),
          },
        ]}
      />

      {creating && <CreateGroupModal options={clusterOptions} onClose={() => setCreating(false)} />}
      {editing && <EditGroupModal group={editing} onClose={() => setEditing(null)} />}
      {membersOf && (
        <MembersModal
          title={`${membersOf.name} — cluster'lar`}
          help="Grubun cluster listesinin tamamı burada belirlenir."
          options={clusterOptions}
          initial={membersOf.members}
          saving={setMembers.isPending}
          optionsError={clusters.isError ? `Cluster listesi alınamadı: ${clusters.error.message}` : undefined}
          onCancel={() => setMembersOf(null)}
          onSave={(members) =>
            setMembers.mutate(
              { id: membersOf.id, members },
              {
                onSuccess: () => {
                  message.success('Grup güncellendi')
                  setMembersOf(null)
                },
                onError: (e) => message.error(e.message),
              },
            )
          }
        />
      )}
    </Flex>
  )
}

function CreateGroupModal({ options, onClose }: { options: { value: string; label: string }[]; onClose: () => void }) {
  const { message } = App.useApp()
  const create = useCreateGroup()
  const [form] = Form.useForm<{ name: string; description?: string; members?: string[] }>()
  return (
    <Modal open title="Yeni cluster grubu" okText="Oluştur" cancelText="Vazgeç" confirmLoading={create.isPending} onOk={() => form.submit()} onCancel={onClose}>
      <Form
        form={form}
        layout="vertical"
        onFinish={(v) =>
          create.mutate(
            { kind: 'cluster', name: v.name.trim(), description: v.description ?? '', members: v.members ?? [] },
            {
              onSuccess: () => {
                message.success('Grup oluşturuldu')
                onClose()
              },
              onError: (e) => message.error(e.message),
            },
          )
        }
      >
        <Form.Item name="name" label="Ad" rules={[{ required: true, whitespace: true, message: 'Ad gerekli' }, { max: 64, message: 'En fazla 64 karakter' }]}>
          <Input autoFocus autoComplete="off" />
        </Form.Item>
        <Form.Item name="description" label="Açıklama" rules={[{ max: 500, message: 'En fazla 500 karakter' }]}>
          <Input.TextArea rows={2} />
        </Form.Item>
        <Form.Item name="members" label="Cluster'lar (isteğe bağlı)">
          <Select mode="multiple" showSearch optionFilterProp="label" options={options} placeholder="Cluster seçin" />
        </Form.Item>
      </Form>
    </Modal>
  )
}

function EditGroupModal({ group, onClose }: { group: ResourceGroup; onClose: () => void }) {
  const { message } = App.useApp()
  const update = useUpdateGroup()
  const [form] = Form.useForm<{ name: string; description?: string }>()
  return (
    <Modal open title={`${group.name} — düzenle`} okText="Kaydet" cancelText="Vazgeç" confirmLoading={update.isPending} onOk={() => form.submit()} onCancel={onClose}>
      <Form
        form={form}
        layout="vertical"
        initialValues={{ name: group.name, description: group.description ?? '' }}
        onFinish={(v) =>
          update.mutate(
            { id: group.id, name: v.name.trim(), description: v.description ?? '' },
            {
              onSuccess: () => {
                message.success('Grup güncellendi')
                onClose()
              },
              onError: (e) => message.error(e.message),
            },
          )
        }
      >
        <Form.Item name="name" label="Ad" rules={[{ required: true, whitespace: true, message: 'Ad gerekli' }, { max: 64, message: 'En fazla 64 karakter' }]}>
          <Input autoFocus autoComplete="off" />
        </Form.Item>
        <Form.Item name="description" label="Açıklama" rules={[{ max: 500, message: 'En fazla 500 karakter' }]}>
          <Input.TextArea rows={2} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
