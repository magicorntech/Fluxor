import { PlusOutlined } from '@ant-design/icons'
import { App, Button, Flex, Form, Input, Modal, Popconfirm, Space, Tag, Typography } from 'antd'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useCreateUser, useResetPassword, useUpdateUser, useUsers } from '../../api/users'
import type { AdminUser } from '../../api/users'
import { DataTable } from '../../components/DataTable'
import { PageHeader } from '../../components/PageHeader'
import { SecretModal } from '../../components/SecretModal'
import { usePermissions } from '../auth/usePermissions'
import { USER_STATUS_LABELS, formatSubjectParam, formatTime, userStatus } from './model'

interface Secret {
  title: string
  username: string
  password: string
}

export function UsersPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const { user: me } = usePermissions()
  const users = useUsers()
  const update = useUpdateUser()
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<AdminUser | null>(null)
  const [resetting, setResetting] = useState<AdminUser | null>(null)
  const [secret, setSecret] = useState<Secret | null>(null)

  const toggleDisabled = (u: AdminUser) =>
    update.mutate(
      { id: u.id, disabled: !u.disabled },
      {
        onSuccess: () => message.success(u.disabled ? 'Hesap etkinleştirildi' : 'Hesap devre dışı bırakıldı'),
        onError: (e) => message.error(e.message),
      },
    )

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="Kullanıcılar"
        description="Hesap oluşturma, parola sıfırlama ve hesap durumu."
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
            Yeni kullanıcı
          </Button>
        }
      />

      <DataTable<AdminUser>
        rowKey="id"
        loading={users.isLoading}
        dataSource={users.data}
        error={users.isError ? users.error : null}
        emptyText="Henüz kullanıcı yok"
        columns={[
          {
            title: 'Kullanıcı',
            render: (_, u) => (
              <Flex vertical>
                <Typography.Text strong>{u.username}</Typography.Text>
                {u.displayName && <Typography.Text type="secondary">{u.displayName}</Typography.Text>}
              </Flex>
            ),
          },
          { title: 'E-posta', dataIndex: 'email', render: (v?: string) => v || '—' },
          {
            title: 'Durum',
            render: (_, u) => {
              const s = USER_STATUS_LABELS[userStatus(u)]
              return <Tag color={s.color}>{s.text}</Tag>
            },
          },
          { title: 'Son giriş', dataIndex: 'lastLoginAt', render: formatTime },
          { title: 'Oluşturulma', dataIndex: 'createdAt', render: formatTime },
          {
            title: '',
            key: 'actions',
            render: (_, u) => {
              const self = u.id === me?.id
              return (
                <Space size={0} wrap>
                  <Button type="link" size="small" onClick={() => setEditing(u)}>
                    Düzenle
                  </Button>
                  <Button
                    type="link"
                    size="small"
                    onClick={() => navigate(`/admin/access?subject=${encodeURIComponent(formatSubjectParam({ kind: 'user', id: u.id }))}`)}
                  >
                    Yetkiler
                  </Button>
                  {!self && (
                    <Button type="link" size="small" onClick={() => setResetting(u)}>
                      Parola sıfırla
                    </Button>
                  )}
                  {!self && (
                    <Popconfirm
                      title={u.disabled ? `${u.username} etkinleştirilsin mi?` : `${u.username} devre dışı bırakılsın mı?`}
                      description={u.disabled ? undefined : 'Kullanıcının oturumları hemen sona erer.'}
                      okText={u.disabled ? 'Etkinleştir' : 'Devre dışı bırak'}
                      cancelText="Vazgeç"
                      onConfirm={() => toggleDisabled(u)}
                    >
                      <Button type="link" size="small" danger={!u.disabled}>
                        {u.disabled ? 'Etkinleştir' : 'Devre dışı bırak'}
                      </Button>
                    </Popconfirm>
                  )}
                </Space>
              )
            },
          },
        ]}
      />

      {creating && (
        <CreateUserModal
          onClose={() => setCreating(false)}
          onCreated={(username, temporaryPassword) => {
            setCreating(false)
            if (temporaryPassword) setSecret({ title: 'Kullanıcı oluşturuldu', username, password: temporaryPassword })
            else message.success('Kullanıcı oluşturuldu')
          }}
        />
      )}
      {editing && <EditUserModal user={editing} onClose={() => setEditing(null)} />}
      {resetting && (
        <ResetPasswordModal
          user={resetting}
          onClose={() => setResetting(null)}
          onDone={(temporaryPassword) => {
            const username = resetting.username
            setResetting(null)
            if (temporaryPassword) setSecret({ title: 'Parola sıfırlandı', username, password: temporaryPassword })
            else message.success('Parola sıfırlandı')
          }}
        />
      )}
      {secret && (
        <SecretModal
          title={secret.title}
          warning={{
            title: 'Bu parola yalnızca bir kez gösterilir',
            description: 'Kapattıktan sonra tekrar görüntülenemez. Kullanıcı ilk girişte parolasını değiştirmek zorunda.',
          }}
          rows={[
            { label: 'Kullanıcı adı', value: secret.username },
            { label: 'Geçici parola', value: secret.password, code: true },
          ]}
          onClose={() => setSecret(null)}
        />
      )}
    </Flex>
  )
}

const PASSWORD_HINT = 'Boş bırakırsanız rastgele bir parola üretilir. Kullanıcı ilk girişte değiştirmek zorunda.'

function CreateUserModal({ onClose, onCreated }: { onClose: () => void; onCreated: (username: string, temporaryPassword?: string) => void }) {
  const { message } = App.useApp()
  const create = useCreateUser()
  const [form] = Form.useForm<{ username: string; displayName?: string; email?: string; password?: string }>()
  return (
    <Modal open title="Yeni kullanıcı" okText="Oluştur" cancelText="Vazgeç" confirmLoading={create.isPending} onOk={() => form.submit()} onCancel={onClose}>
      <Form
        form={form}
        layout="vertical"
        onFinish={(v) =>
          create.mutate(
            { username: v.username, displayName: v.displayName || undefined, email: v.email || undefined, password: v.password || undefined },
            { onSuccess: (data) => onCreated(data.username, data.temporaryPassword), onError: (e) => message.error(e.message) },
          )
        }
      >
        <Form.Item
          name="username"
          label="Kullanıcı adı"
          extra="3-64 karakter: küçük harf, rakam, '.', '_' ve '-'"
          rules={[
            { required: true, message: 'Kullanıcı adı gerekli' },
            { pattern: /^[a-z0-9][a-z0-9._-]{1,62}[a-z0-9]$/, message: 'Geçerli bir kullanıcı adı girin' },
          ]}
        >
          <Input autoFocus autoComplete="off" />
        </Form.Item>
        <Form.Item name="displayName" label="Görünen ad" rules={[{ max: 100, message: 'En fazla 100 karakter' }]}>
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="email" label="E-posta" rules={[{ type: 'email', message: 'Geçerli bir e-posta girin' }]}>
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="password" label="Geçici parola (isteğe bağlı)" extra={PASSWORD_HINT} rules={[{ min: 12, message: 'En az 12 karakter' }, { max: 256, message: 'En fazla 256 karakter' }]}>
          <Input.Password autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  )
}

function EditUserModal({ user, onClose }: { user: AdminUser; onClose: () => void }) {
  const { message } = App.useApp()
  const update = useUpdateUser()
  const [form] = Form.useForm<{ displayName: string; email: string }>()
  return (
    <Modal open title={`${user.username} — düzenle`} okText="Kaydet" cancelText="Vazgeç" confirmLoading={update.isPending} onOk={() => form.submit()} onCancel={onClose}>
      <Form
        form={form}
        layout="vertical"
        initialValues={{ displayName: user.displayName ?? '', email: user.email ?? '' }}
        onFinish={(v) =>
          update.mutate(
            { id: user.id, displayName: v.displayName ?? '', email: v.email ?? '' },
            {
              onSuccess: () => {
                message.success('Kullanıcı güncellendi')
                onClose()
              },
              onError: (e) => message.error(e.message),
            },
          )
        }
      >
        <Form.Item name="displayName" label="Görünen ad" rules={[{ max: 100, message: 'En fazla 100 karakter' }]}>
          <Input autoFocus autoComplete="off" />
        </Form.Item>
        <Form.Item name="email" label="E-posta" extra="Boş bırakırsanız e-posta kaldırılır." rules={[{ type: 'email', message: 'Geçerli bir e-posta girin' }]}>
          <Input autoComplete="off" />
        </Form.Item>
      </Form>
    </Modal>
  )
}

function ResetPasswordModal({ user, onClose, onDone }: { user: AdminUser; onClose: () => void; onDone: (temporaryPassword?: string) => void }) {
  const { message } = App.useApp()
  const reset = useResetPassword()
  const [form] = Form.useForm<{ password?: string }>()
  return (
    <Modal open title={`${user.username} — parola sıfırla`} okText="Sıfırla" cancelText="Vazgeç" confirmLoading={reset.isPending} onOk={() => form.submit()} onCancel={onClose}>
      <Typography.Paragraph type="secondary">
        Kullanıcının tüm oturumları sona erer, hesap kilidi kalkar ve bir sonraki girişte parolasını değiştirmesi gerekir.
      </Typography.Paragraph>
      <Form
        form={form}
        layout="vertical"
        onFinish={(v) =>
          reset.mutate(
            { id: user.id, password: v.password || undefined },
            { onSuccess: (data) => onDone(data.temporaryPassword), onError: (e) => message.error(e.message) },
          )
        }
      >
        <Form.Item name="password" label="Geçici parola (isteğe bağlı)" extra={PASSWORD_HINT} rules={[{ min: 12, message: 'En az 12 karakter' }, { max: 256, message: 'En fazla 256 karakter' }]}>
          <Input.Password autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
