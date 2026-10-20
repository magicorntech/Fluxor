/** Shared status vocabulary for badges across clusters, apps, sync, health. */

export type StatusTone = 'success' | 'processing' | 'error' | 'warning' | 'default'

export interface StatusSpec {
  tone: StatusTone
  label: string
}

const CLUSTER: Record<string, StatusSpec> = {
  Connected: { tone: 'success', label: 'Bağlı' },
  Unreachable: { tone: 'error', label: 'Ulaşılamıyor' },
  Unknown: { tone: 'default', label: 'Bilinmiyor' },
}

const HEALTH: Record<string, StatusSpec> = {
  Healthy: { tone: 'success', label: 'Sağlıklı' },
  Progressing: { tone: 'processing', label: 'İlerliyor' },
  Degraded: { tone: 'error', label: 'Bozuk' },
  Missing: { tone: 'default', label: 'Eksik' },
  Unknown: { tone: 'default', label: 'Bilinmiyor' },
}

const SYNC: Record<string, StatusSpec> = {
  Synced: { tone: 'success', label: 'Senkron' },
  OutOfSync: { tone: 'warning', label: 'Senkron dışı' },
  Unknown: { tone: 'default', label: 'Bilinmiyor' },
}

export function clusterStatus(status: string): StatusSpec {
  return CLUSTER[status] ?? { tone: 'default', label: status }
}

export function healthStatus(status: string): StatusSpec {
  return HEALTH[status] ?? { tone: 'default', label: status }
}

export function syncStatus(status: string): StatusSpec {
  return SYNC[status] ?? { tone: 'default', label: status }
}
