import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  LoadingOutlined,
  QuestionCircleOutlined,
} from '@ant-design/icons'
import { Tag } from 'antd'
import type { ReactNode } from 'react'
import { clusterStatus, type StatusSpec, type StatusTone } from './status'

const toneColor: Record<StatusTone, string> = {
  success: 'success',
  processing: 'processing',
  error: 'error',
  warning: 'warning',
  default: 'default',
}

const toneIcon: Record<StatusTone, ReactNode> = {
  success: <CheckCircleOutlined />,
  processing: <LoadingOutlined spin />,
  error: <CloseCircleOutlined />,
  warning: <ExclamationCircleOutlined />,
  default: <QuestionCircleOutlined />,
}

export function StatusBadge({
  tone,
  label,
  icon = true,
}: StatusSpec & { icon?: boolean }) {
  return (
    <Tag color={toneColor[tone]} icon={icon ? toneIcon[tone] : undefined} style={{ marginInlineEnd: 0 }}>
      {label}
    </Tag>
  )
}

/** Cluster connection badge — keeps the old StatusTag call sites working. */
export function StatusTag({ status }: { status: string }) {
  return <StatusBadge {...clusterStatus(status)} />
}
