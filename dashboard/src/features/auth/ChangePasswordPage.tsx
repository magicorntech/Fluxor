import { Alert, App, Button, Card, Flex, Form, Input } from 'antd'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ApiError } from '../../api/errors'
import { PageHeader } from '../../components/PageHeader'
import { useAuth } from './AuthProvider'

const MIN_LENGTH = 12

interface Values {
  currentPassword: string
  newPassword: string
  confirm: string
}

export function ChangePasswordPage() {
  const { user, changePassword } = useAuth()
  const { message } = App.useApp()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const forced = user?.mustChangePassword ?? false

  const submit = async ({ currentPassword, newPassword }: Values) => {
    setError(null)
    setSubmitting(true)
    try {
      await changePassword(currentPassword, newPassword)
      message.success('Parola değiştirildi')
      navigate('/', { replace: true })
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Parola değiştirilemedi')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Flex vertical gap={16}>
      <PageHeader title="Parolayı değiştir" description="Yeni parola en az 12 karakter olmalıdır." />

      {forced && (
        <Alert
          type="warning"
          showIcon
          title="İlk girişte parolanızı değiştirmeniz gerekiyor"
          description="Parolayı değiştirmeden başka bir sayfayı kullanamazsınız."
        />
      )}

      <Card style={{ maxWidth: 480 }}>
        {error && <Alert type="error" showIcon title={error} style={{ marginBottom: 16 }} />}
        <Form<Values> layout="vertical" onFinish={submit} requiredMark={false}>
          <Form.Item
            name="currentPassword"
            label="Mevcut parola"
            rules={[{ required: true, message: 'Mevcut parola gerekli' }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Form.Item
            name="newPassword"
            label="Yeni parola"
            extra={`En az ${MIN_LENGTH} karakter`}
            rules={[
              { required: true, message: 'Yeni parola gerekli' },
              { min: MIN_LENGTH, message: `En az ${MIN_LENGTH} karakter olmalı` },
              { max: 256, message: 'En fazla 256 karakter olabilir' },
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            name="confirm"
            label="Yeni parola (tekrar)"
            dependencies={['newPassword']}
            rules={[
              { required: true, message: 'Parolayı tekrar girin' },
              ({ getFieldValue }) => ({
                validator: (_, value: string) =>
                  !value || getFieldValue('newPassword') === value
                    ? Promise.resolve()
                    : Promise.reject(new Error('Parolalar eşleşmiyor')),
              }),
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={submitting}>
            Parolayı değiştir
          </Button>
        </Form>
      </Card>
    </Flex>
  )
}
