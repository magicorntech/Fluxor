import { PlusOutlined } from '@ant-design/icons'
import { App, Button, Flex, Form, Input, Modal, Select, Space, Tag, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useCreateTeam, useDeleteTeam, useSetTeamMembers, useTeams, useUpdateTeam } from '../../api/teams'
import type { Team } from '../../api/teams'
import { useUsers } from '../../api/users'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { DataTable } from '../../components/DataTable'
import { MembersModal } from '../../components/MembersModal'
import { PageHeader } from '../../components/PageHeader'
import { formatSubjectParam, formatTime, namesOf, sortByLabel, toLookup, userLabel } from './model'

const MAX_TAGS = 4

export function TeamsPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const teams = useTeams()
  const users = useUsers()
  const remove = useDeleteTeam()
  const setMembers = useSetTeamMembers()
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<Team | null>(null)
  const [membersOf, setMembersOf] = useState<Team | null>(null)

  const userNames = useMemo(() => toLookup(users.data, userLabel), [users.data])
  const userOptions = useMemo(
    () => sortByLabel(users.data ?? [], userLabel).map((u) => ({ value: u.id, label: userLabel(u) })),
    [users.data],
  )

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="Ekipler"
        description="Bir ekibe verilen yetki, ekibin tüm üyeleri için geçerlidir. Üyelik değişiklikleri hemen etkili olur."
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
            Yeni ekip
          </Button>
        }
      />

      <DataTable<Team>
        rowKey="id"
        loading={teams.isLoading}
        dataSource={teams.data}
        error={teams.isError ? teams.error : null}
        emptyText="Henüz ekip yok"
        columns={[
          { title: 'Ad', dataIndex: 'name', render: (v: string) => <Typography.Text strong>{v}</Typography.Text> },
          { title: 'Açıklama', dataIndex: 'description', render: (v?: string) => v || '—' },
          {
            title: 'Üyeler',
            render: (_, t) =>
              t.members.length === 0 ? (
                <Typography.Text type="secondary">Üye yok</Typography.Text>
              ) : (
                <Space size={[0, 4]} wrap>
                  {namesOf(t.members, userNames).slice(0, MAX_TAGS).map((name) => (
                    <Tag key={name}>{name}</Tag>
                  ))}
                  {t.members.length > MAX_TAGS && <Tag>+{t.members.length - MAX_TAGS}</Tag>}
                </Space>
              ),
          },
          { title: 'Oluşturulma', dataIndex: 'createdAt', render: formatTime },
          {
            title: '',
            key: 'actions',
            render: (_, t) => (
              <Space size={0} wrap>
                <Button type="link" size="small" onClick={() => setMembersOf(t)}>
                  Üyeler
                </Button>
                <Button type="link" size="small" onClick={() => setEditing(t)}>
                  Düzenle
                </Button>
                <Button
                  type="link"
                  size="small"
                  onClick={() => navigate(`/admin/access?subject=${encodeURIComponent(formatSubjectParam({ kind: 'team', id: t.id }))}`)}
                >
                  Yetkiler
                </Button>
                <ConfirmDelete
                  name={t.name}
                  description="Ekibe verilen tüm yetkiler de silinir."
                  onConfirm={() =>
                    remove.mutate(t.id, {
                      onSuccess: () => message.success('Ekip silindi'),
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

      {creating && <CreateTeamModal options={userOptions} onClose={() => setCreating(false)} />}
      {editing && <EditTeamModal team={editing} onClose={() => setEditing(null)} />}
      {membersOf && (
        <MembersModal
          title={`${membersOf.name} — üyeler`}
          help="Ekibin üye listesinin tamamı burada belirlenir; listeden çıkarılan kullanıcılar ekibin yetkilerini hemen kaybeder."
          options={userOptions}
          initial={membersOf.members}
          saving={setMembers.isPending}
          optionsError={users.isError ? `Kullanıcı listesi alınamadı: ${users.error.message}` : undefined}
          onCancel={() => setMembersOf(null)}
          onSave={(members) =>
            setMembers.mutate(
              { id: membersOf.id, members },
              {
                onSuccess: () => {
                  message.success('Üyeler güncellendi')
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

function CreateTeamModal({ options, onClose }: { options: { value: string; label: string }[]; onClose: () => void }) {
  const { message } = App.useApp()
  const create = useCreateTeam()
  const [form] = Form.useForm<{ name: string; description?: string; members?: string[] }>()
  return (
    <Modal open title="Yeni ekip" okText="Oluştur" cancelText="Vazgeç" confirmLoading={create.isPending} onOk={() => form.submit()} onCancel={onClose}>
      <Form
        form={form}
        layout="vertical"
        onFinish={(v) =>
          create.mutate(
            { name: v.name.trim(), description: v.description ?? '', members: v.members ?? [] },
            {
              onSuccess: () => {
                message.success('Ekip oluşturuldu')
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
        <Form.Item name="members" label="Üyeler (isteğe bağlı)">
          <Select mode="multiple" showSearch optionFilterProp="label" options={options} placeholder="Kullanıcı seçin" />
        </Form.Item>
      </Form>
    </Modal>
  )
}

function EditTeamModal({ team, onClose }: { team: Team; onClose: () => void }) {
  const { message } = App.useApp()
  const update = useUpdateTeam()
  const [form] = Form.useForm<{ name: string; description?: string }>()
  return (
    <Modal open title={`${team.name} — düzenle`} okText="Kaydet" cancelText="Vazgeç" confirmLoading={update.isPending} onOk={() => form.submit()} onCancel={onClose}>
      <Form
        form={form}
        layout="vertical"
        initialValues={{ name: team.name, description: team.description ?? '' }}
        onFinish={(v) =>
          update.mutate(
            { id: team.id, name: v.name.trim(), description: v.description ?? '' },
            {
              onSuccess: () => {
                message.success('Ekip güncellendi')
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
