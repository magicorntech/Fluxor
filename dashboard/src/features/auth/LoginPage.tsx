import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Flex, Form, Input, Typography, theme } from 'antd'
import { useState } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { ApiError } from '../../api/errors'
import { useAuth } from './AuthProvider'

interface LoginValues {
  username: string
  password: string
}

export function LoginPage() {
  const { status, login } = useAuth()
  const location = useLocation()
  const { token } = theme.useToken()
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  if (status === 'authenticated') {
    const from = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname
    return <Navigate to={from && from !== '/login' ? from : '/'} replace />
  }

  const submit = async ({ username, password }: LoginValues) => {
    setError(null)
    setSubmitting(true)
    try {
      await login(username, password)
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Giriş yapılamadı')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Flex
      justify="center"
      align="center"
      style={{
        minHeight: '100vh',
        padding: 24,
        background: `linear-gradient(160deg, ${token.colorBgLayout} 0%, ${token.colorPrimaryBg} 100%)`,
      }}
    >
      <Card style={{ width: 400, maxWidth: '100%' }} styles={{ body: { padding: 28 } }}>
        <Flex vertical gap={8} style={{ marginBottom: 28 }} align="flex-start">
          <Flex
            align="center"
            justify="center"
            style={{
              width: 40,
              height: 40,
              borderRadius: 8,
              background: token.colorPrimary,
              color: '#fff',
              fontWeight: 700,
              fontSize: 18,
            }}
          >
            F
          </Flex>
          <Typography.Title level={3} style={{ margin: 0 }}>
            Fluxor
          </Typography.Title>
          <Typography.Text type="secondary">Devam etmek için giriş yapın</Typography.Text>
        </Flex>

        {error && <Alert type="error" showIcon title={error} style={{ marginBottom: 16 }} />}

        <Form<LoginValues> layout="vertical" onFinish={submit} requiredMark={false} size="large">
          <Form.Item name="username" label="Kullanıcı adı" rules={[{ required: true, message: 'Kullanıcı adı gerekli' }]}>
            <Input prefix={<UserOutlined />} autoComplete="username" autoFocus />
          </Form.Item>
          <Form.Item name="password" label="Parola" rules={[{ required: true, message: 'Parola gerekli' }]}>
            <Input.Password prefix={<LockOutlined />} autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={submitting}>
            Giriş yap
          </Button>
        </Form>
      </Card>
    </Flex>
  )
}
