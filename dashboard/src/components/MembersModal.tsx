import { Alert, Modal, Select, Typography } from 'antd'
import { useState } from 'react'

export interface MemberOption {
  value: string
  label: string
}

interface Props {
  title: string
  help: string
  options: MemberOption[]
  initial: string[]
  saving: boolean
  // Shown when the list of candidates could not be loaded.
  optionsError?: string
  onSave: (members: string[]) => void
  onCancel: () => void
}

// Edits the full member list of a team or group. Mount it only while editing: it takes its starting
// selection from `initial` once, when it appears.
export function MembersModal({ title, help, options, initial, saving, optionsError, onSave, onCancel }: Props) {
  const [selected, setSelected] = useState<string[]>(initial)
  return (
    <Modal
      open
      title={title}
      okText="Kaydet"
      cancelText="Vazgeç"
      confirmLoading={saving}
      onOk={() => onSave(selected)}
      onCancel={onCancel}
      width={560}
    >
      <Typography.Paragraph type="secondary">{help}</Typography.Paragraph>
      {optionsError && <Alert type="error" showIcon title={optionsError} style={{ marginBottom: 12 }} />}
      <Select
        mode="multiple"
        showSearch
        optionFilterProp="label"
        style={{ width: '100%' }}
        placeholder="Seçin"
        options={options}
        value={selected}
        onChange={setSelected}
      />
      <Typography.Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
        {selected.length} üye seçili
      </Typography.Text>
    </Modal>
  )
}
