import { PlusOutlined } from '@ant-design/icons'
import { App, Button, Flex, Form, Input, Modal, Select, Tag, Typography } from 'antd'
import { useState } from 'react'
import { useCreateToken, useDeleteToken, useTokens } from '../../api/tokens'
import type { ApiToken } from '../../api/tokens'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { DataTable } from '../../components/DataTable'
import { PageHeader } from '../../components/PageHeader'
import { SecretModal } from '../../components/SecretModal'
import { DEFAULT_TOKEN_DAYS, MAX_TOKENS, TOKEN_LIFETIMES, formatTime, tokenState } from '../admin/model'

export function TokensPage() {
  const { message } = App.useApp()
  const tokens = useTokens()
  const remove = useDeleteToken()
  const [creating, setCreating] = useState(false)
  const [created, setCreated] = useState<{ name: string; secret: string } | null>(null)
  const count = tokens.data?.length ?? 0

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="API token'larım"
        description={`Betik ve otomasyon için. Sahibinin yetkileriyle çalışır. En fazla ${MAX_TOKENS} (${count} kullanılıyor).`}
        extra={
          <Button type="primary" icon={<PlusOutlined />} disabled={count >= MAX_TOKENS} onClick={() => setCreating(true)}>
            Yeni token
          </Button>
        }
      />

      <DataTable<ApiToken>
        rowKey="id"
        loading={tokens.isLoading}
        dataSource={tokens.data}
        error={tokens.isError ? tokens.error : null}
        emptyText="Henüz token yok"
        columns={[
          { title: 'Ad', dataIndex: 'name', render: (v: string) => <Typography.Text strong>{v}</Typography.Text> },
          { title: 'Başlangıç', dataIndex: 'prefix', render: (v: string) => <Typography.Text code>{v}…</Typography.Text> },
          { title: 'Oluşturulma', dataIndex: 'createdAt', render: formatTime },
          { title: 'Son kullanım', dataIndex: 'lastUsedAt', render: (v?: string) => (v ? formatTime(v) : 'Hiç kullanılmadı') },
          {
            title: 'Bitiş',
            dataIndex: 'expiresAt',
            render: (v: string, t) => (
              <Flex gap={8} align="center">
                {formatTime(v)}
                {tokenState(t) === 'expired' && <Tag color="error">Süresi doldu</Tag>}
              </Flex>
            ),
          },
          {
            title: '',
            key: 'actions',
            render: (_, t) => (
              <ConfirmDelete
                name={t.name}
                description="Bu token'ı kullanan her şey hemen erişimini kaybeder."
                onConfirm={() =>
                  remove.mutate(t.id, {
                    onSuccess: () => message.success('Token silindi'),
                    onError: (e) => message.error(e.message),
                  })
                }
              >
                <Button type="link" size="small" danger>
                  Sil
                </Button>
              </ConfirmDelete>
            ),
          },
        ]}
      />

      {creating && (
        <CreateTokenModal
          onClose={() => setCreating(false)}
          onCreated={(name, secret) => {
            setCreating(false)
            setCreated({ name, secret })
          }}
        />
      )}
      {created && (
        <SecretModal
          title="Token oluşturuldu"
          warning={{
            title: 'Bu token yalnızca şimdi gösteriliyor',
            description: 'Kapattıktan sonra tekrar görüntülenemez; kaybederseniz silip yenisini oluşturmanız gerekir. Parola gibi saklayın.',
          }}
          rows={[
            { label: 'Ad', value: created.name },
            { label: 'Token', value: created.secret, code: true },
          ]}
          hint={
            <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
              Kullanım: her istekte <Typography.Text code>Authorization: Bearer {'<token>'}</Typography.Text> başlığını gönderin.
            </Typography.Paragraph>
          }
          onClose={() => setCreated(null)}
        />
      )}
    </Flex>
  )
}

function CreateTokenModal({ onClose, onCreated }: { onClose: () => void; onCreated: (name: string, secret: string) => void }) {
  const { message } = App.useApp()
  const create = useCreateToken()
  const [form] = Form.useForm<{ name: string; expiresInDays: number }>()
  return (
    <Modal open title="Yeni API token" okText="Oluştur" cancelText="Vazgeç" confirmLoading={create.isPending} onOk={() => form.submit()} onCancel={onClose}>
      <Form
        form={form}
        layout="vertical"
        initialValues={{ expiresInDays: DEFAULT_TOKEN_DAYS }}
        onFinish={(v) =>
          create.mutate(
            { name: v.name.trim(), expiresInDays: v.expiresInDays },
            { onSuccess: (data) => onCreated(data.name, data.token), onError: (e) => message.error(e.message) },
          )
        }
      >
        <Form.Item
          name="name"
          label="Ad"
          extra="Token'ın ne için kullanıldığını hatırlatacak bir ad, örneğin: CI hattı"
          rules={[{ required: true, whitespace: true, message: 'Ad gerekli' }, { max: 64, message: 'En fazla 64 karakter' }]}
        >
          <Input autoFocus autoComplete="off" />
        </Form.Item>
        <Form.Item name="expiresInDays" label="Geçerlilik süresi">
          <Select options={TOKEN_LIFETIMES.map((l) => ({ value: l.days, label: l.label }))} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
