import { Alert, Button, Descriptions, Modal, Typography } from 'antd'
import type { ReactNode } from 'react'

export interface SecretRow {
  label: string
  value: string
  // Show the value in a code style (passwords, tokens).
  code?: boolean
}

interface Props {
  title: string
  rows: SecretRow[]
  // The warning above the values. The default says the secret is shown only once.
  warning?: { title: string; description?: string }
  // Extra help below the values, such as how to use a token.
  hint?: ReactNode
  onClose: () => void
}

const DEFAULT_WARNING = {
  title: 'Bu bilgi yalnızca bir kez gösterilir',
  description: 'Kapattıktan sonra tekrar görüntülenemez.',
}

// Shows a secret that the server hands out exactly once (a generated password, an API token). It lives
// only in this dialog: the server keeps just a hash, so it cannot be shown again.
export function SecretModal({ title, rows, warning = DEFAULT_WARNING, hint, onClose }: Props) {
  return (
    <Modal
      open
      title={title}
      onCancel={onClose}
      mask={{ closable: false }}
      width={620}
      footer={
        <Button type="primary" onClick={onClose}>
          Kapat
        </Button>
      }
    >
      <Alert type="warning" showIcon title={warning.title} description={warning.description} style={{ marginBottom: 16 }} />
      <Descriptions column={1} size="small" bordered>
        {rows.map((row) => (
          <Descriptions.Item key={row.label} label={row.label}>
            <Typography.Text code={row.code} copyable>
              {row.value}
            </Typography.Text>
          </Descriptions.Item>
        ))}
      </Descriptions>
      {hint && <div style={{ marginTop: 16 }}>{hint}</div>}
    </Modal>
  )
}
