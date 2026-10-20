// Pure helpers behind the administration pages: describing grants in words, deciding which scope a
// role allows, building API payloads. Kept free of React so they can be unit tested.
import type { BindingInput, Ref, Role } from '../../api/access'
import type { AuditEvent, AuditResult } from '../../api/audit'
import type { Me } from '../../api/me'
import type { ApiToken } from '../../api/tokens'

export const SYSTEM_PERMISSIONS = ['user:manage', 'team:manage', 'group:manage', 'rbac:manage', 'audit:view'] as const
export type SystemPermission = (typeof SYSTEM_PERMISSIONS)[number]

// Application scopes (and application groups) are accepted by the server only once applications exist.
export const APPLICATION_SCOPE_AVAILABLE = false

const PERMISSION_LABELS: Record<string, string> = {
  '*': 'Tüm izinler',
  'cluster:view': "Cluster'ları görüntüle",
  'cluster:manage': "Cluster ekle, düzenle ve sil",
  'app:view': 'Uygulamaları görüntüle',
  'app:sync': 'Uygulamaları senkronize et',
  'app:rollback': 'Uygulamaları geri al',
  'app:manage': 'Uygulama tanımlarını yönet',
  'user:manage': 'Kullanıcıları yönet',
  'team:manage': 'Ekipleri yönet',
  'group:manage': 'Grupları yönet',
  'rbac:manage': 'Yetkileri yönet',
  'audit:view': 'Denetim kayıtlarını görüntüle',
}

export function permissionLabel(p: string): string {
  return PERMISSION_LABELS[p] ?? p
}

export function hasPermission(perms: readonly string[] | undefined, p: string): boolean {
  return perms?.includes(p) ?? false
}

export type UserStatus = 'active' | 'disabled' | 'mustChange'

// A disabled account is shown as disabled even if it also still has to change its password.
export function userStatus(u: { disabled: boolean; mustChangePassword: boolean }): UserStatus {
  if (u.disabled) return 'disabled'
  return u.mustChangePassword ? 'mustChange' : 'active'
}

export const USER_STATUS_LABELS: Record<UserStatus, { text: string; color: string }> = {
  active: { text: 'Aktif', color: 'success' },
  disabled: { text: 'Devre dışı', color: 'default' },
  mustChange: { text: 'Parola değişimi bekliyor', color: 'warning' },
}

export function formatTime(iso: string | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString('tr-TR')
}

export function sortByLabel<T>(items: readonly T[], label: (t: T) => string): T[] {
  return [...items].sort((a, b) => label(a).localeCompare(label(b), 'tr'))
}

export function userLabel(u: { username: string; displayName?: string; disabled?: boolean }): string {
  const base = u.displayName ? `${u.displayName} (${u.username})` : u.username
  return u.disabled ? `${base} — devre dışı` : base
}

export type Lookup = ReadonlyMap<string, string>

export function toLookup<T extends { id: string }>(items: readonly T[] | undefined, name: (t: T) => string): Lookup {
  return new Map((items ?? []).map((t) => [t.id, name(t)]))
}

const MISSING = (id: string) => `silinmiş (${id})`

export interface SubjectView {
  kindLabel: 'Kullanıcı' | 'Ekip'
  name: string
  found: boolean
}

export function describeSubject(subject: { kind: 'user' | 'team'; id: string }, users: Lookup, teams: Lookup): SubjectView {
  const lookup = subject.kind === 'user' ? users : teams
  const name = lookup.get(subject.id)
  return {
    kindLabel: subject.kind === 'user' ? 'Kullanıcı' : 'Ekip',
    name: name ?? MISSING(subject.id),
    found: name !== undefined,
  }
}

// Words for one dimension of a grant's scope.
export function describeScopeRef(dimension: 'cluster' | 'application', ref: Ref, items: Lookup, groups: Lookup): string {
  if (ref.kind === 'any') return dimension === 'cluster' ? "Tüm cluster'lar" : 'Tüm uygulamalar'
  const id = ref.id ?? ''
  if (ref.kind === 'group') return `Grup: ${groups.get(id) ?? MISSING(id)}`
  return `${dimension === 'cluster' ? 'Cluster' : 'Uygulama'}: ${items.get(id) ?? MISSING(id)}`
}

export interface ScopeRules {
  // The cluster dimension may be narrowed to one cluster or a cluster group.
  clusterScope: boolean
  // The application dimension may be narrowed.
  applicationScope: boolean
  // Why something is not allowed, to show next to the disabled control.
  reason?: string
}

// Roles with system permissions (the administrator role) apply everywhere. Everything else may be
// limited to clusters; narrowing by application waits until applications exist.
export function scopeRules(role: Pick<Role, 'permissions'> | undefined): ScopeRules {
  if (!role) return { clusterScope: false, applicationScope: false }
  const isSystem = role.permissions.some((p) => p === '*' || (SYSTEM_PERMISSIONS as readonly string[]).includes(p))
  if (isSystem) {
    return { clusterScope: false, applicationScope: false, reason: 'Bu rol sistem izinleri içerir, bu yüzden kapsam daraltılamaz.' }
  }
  return {
    clusterScope: true,
    applicationScope: APPLICATION_SCOPE_AVAILABLE,
    reason: APPLICATION_SCOPE_AVAILABLE ? undefined : 'Uygulama kapsamı, uygulamalar eklenince açılacak.',
  }
}

export interface GrantForm {
  subjectKind: 'user' | 'team'
  subjectId: string
  roleId: string
  clusterKind: 'any' | 'id' | 'group'
  clusterId?: string
}

// Builds the request body for a new grant. A scope the role does not allow is dropped rather than sent.
export function buildBindingInput(form: GrantForm, rules: ScopeRules): BindingInput {
  const narrowed = rules.clusterScope && form.clusterKind !== 'any' && !!form.clusterId
  return {
    subject: { kind: form.subjectKind, id: form.subjectId },
    roleId: form.roleId,
    scope: { cluster: narrowed ? { kind: form.clusterKind, id: form.clusterId } : { kind: 'any' } },
  }
}

// The subject filter lives in the URL as "user:<id>" or "team:<id>".
export function parseSubjectParam(value: string | null): { kind: 'user' | 'team'; id: string } | undefined {
  if (!value) return undefined
  const i = value.indexOf(':')
  if (i < 0) return undefined
  const kind = value.slice(0, i)
  const id = value.slice(i + 1)
  if ((kind !== 'user' && kind !== 'team') || !id) return undefined
  return { kind, id }
}

export function formatSubjectParam(subject: { kind: 'user' | 'team'; id: string }): string {
  return `${subject.kind}:${subject.id}`
}

// Who created a grant, in words.
export function describeCreator(createdBy: string | undefined, users: Lookup): string {
  if (!createdBy) return '—'
  if (createdBy === 'bootstrap') return 'İlk kurulum'
  return users.get(createdBy) ?? MISSING(createdBy)
}

// The built-in roles come from the server in English; the dashboard shows them in Turkish. An
// unknown role (a future custom one) falls back to the server's own text.
const ROLE_TEXT: Record<string, { name: string; description: string }> = {
  viewer: { name: 'Görüntüleyici', description: "Cluster'ları ve uygulamaları görür" },
  deployer: { name: 'Dağıtımcı', description: 'Görüntüleyici yetkileri, ayrıca uygulamaları senkronize eder ve geri alır' },
  maintainer: { name: 'Bakımcı', description: 'Dağıtımcı yetkileri, ayrıca uygulama tanımlarını oluşturur, düzenler ve siler' },
  'cluster-admin': { name: 'Cluster yöneticisi', description: "Cluster'ları görür ve yönetir" },
  admin: { name: 'Yönetici', description: 'Her şey: kullanıcılar, ekipler ve yetki yönetimi dahil' },
}

export function roleName(role: { id: string; name: string }): string {
  return ROLE_TEXT[role.id]?.name ?? role.name
}

export function roleDescription(role: { id: string; description: string }): string {
  return ROLE_TEXT[role.id]?.description ?? role.description
}

// Everything in the user's summary that decides what the UI offers or which data is visible. When it
// changes, the data already on screen may be stale and must be fetched again.
export function permissionSignature(me: Pick<Me, 'permissions' | 'canCreateClusters' | 'teams'>): string {
  return JSON.stringify([
    [...me.permissions].sort(),
    me.canCreateClusters,
    me.teams.map((t) => t.id).sort(),
  ])
}

// Names for a list of ids, sorted for display. Ids with no known name are shown as the id.
export function namesOf(ids: readonly string[], names: Lookup): string[] {
  return ids.map((id) => names.get(id) ?? id).sort((a, b) => a.localeCompare(b, 'tr'))
}

// --- audit log ---

export interface AuditActionOption {
  value: string
  label: string
}

// Every action name the server writes, grouped for the filter. The server's names are stable; the
// Turkish wording lives here.
export const AUDIT_ACTION_GROUPS: { label: string; actions: AuditActionOption[] }[] = [
  {
    label: 'Oturum',
    actions: [
      { value: 'auth.login', label: 'Giriş' },
      { value: 'auth.password_change', label: 'Parola değişikliği' },
      { value: 'auth.refresh_reuse', label: 'Oturum belirteci yeniden kullanıldı' },
    ],
  },
  {
    label: 'Kullanıcılar',
    actions: [
      { value: 'user.create', label: 'Kullanıcı oluşturuldu' },
      { value: 'user.update', label: 'Kullanıcı güncellendi' },
      { value: 'user.reset_password', label: 'Parola sıfırlandı' },
    ],
  },
  {
    label: 'Ekipler',
    actions: [
      { value: 'team.create', label: 'Ekip oluşturuldu' },
      { value: 'team.update', label: 'Ekip güncellendi' },
      { value: 'team.delete', label: 'Ekip silindi' },
      { value: 'team.set_members', label: 'Ekip üyeleri değişti' },
    ],
  },
  {
    label: 'Gruplar',
    actions: [
      { value: 'group.create', label: 'Grup oluşturuldu' },
      { value: 'group.update', label: 'Grup güncellendi' },
      { value: 'group.delete', label: 'Grup silindi' },
      { value: 'group.set_members', label: 'Grup üyeleri değişti' },
    ],
  },
  {
    label: 'Yetkiler',
    actions: [
      { value: 'role_binding.create', label: 'Yetki verildi' },
      { value: 'role_binding.delete', label: 'Yetki kaldırıldı' },
    ],
  },
  {
    label: "Cluster'lar",
    actions: [
      { value: 'cluster.create', label: 'Cluster eklendi' },
      { value: 'cluster.delete', label: 'Cluster silindi' },
    ],
  },
  {
    label: "API token'ları",
    actions: [
      { value: 'token.create', label: 'Token oluşturuldu' },
      { value: 'token.revoke', label: 'Token silindi' },
    ],
  },
]

const ACTION_LABELS = new Map(AUDIT_ACTION_GROUPS.flatMap((g) => g.actions.map((a) => [a.value, a.label] as const)))

// An action the dashboard does not know (a newer server) is shown by its own name.
export function auditActionLabel(action: string): string {
  return ACTION_LABELS.get(action) ?? action
}

export const AUDIT_RESULT_LABELS: Record<AuditResult, { text: string; color: string }> = {
  success: { text: 'Başarılı', color: 'success' },
  denied: { text: 'Reddedildi', color: 'warning' },
  failure: { text: 'Başarısız', color: 'default' },
  error: { text: 'Hata', color: 'error' },
}

// Who acted. A failed login has no user, only the name that was tried.
export function auditActor(ev: Pick<AuditEvent, 'actorId' | 'actorName'>): { name: string; signedIn: boolean } {
  return { name: ev.actorName || '—', signedIn: !!ev.actorId }
}

// What the event was about: the name if there is one (it survives deletion), else the "kind:id".
export function auditSubjectText(ev: Pick<AuditEvent, 'subject' | 'subjectName'>): string {
  return ev.subjectName || ev.subject || '—'
}

const LOGIN_REASONS: Record<string, string> = {
  unknown_user: 'Bilinmeyen kullanıcı',
  bad_password: 'Hatalı parola',
  locked: 'Hesap kilitli',
  disabled: 'Hesap devre dışı',
}

const DETAIL_LABELS: Record<string, string> = {
  reason: 'Neden',
  via: 'Yöntem',
  role: 'Rol',
  to: 'Kime',
  scope: 'Kapsam',
  fields: 'Alanlar',
  disabled: 'Devre dışı',
  members: 'Üye sayısı',
  kind: 'Tür',
  newName: 'Yeni ad',
  expiresInDays: 'Süre (gün)',
  effect: 'Etki',
}
const DETAIL_ORDER = Object.keys(DETAIL_LABELS)

// "cluster=id:ab12 application=any" in words. IDs are kept as they are.
export function describeAuditScope(text: string): string {
  const part = (name: string) => new RegExp(`${name}=(\\S+)`).exec(text)?.[1]
  const word = (ref: string | undefined) => {
    if (!ref || ref === 'any') return 'tümü'
    if (ref.startsWith('group:')) return `grup ${ref.slice(6)}`
    if (ref.startsWith('id:')) return ref.slice(3)
    return ref
  }
  return `Cluster: ${word(part('cluster'))} · Uygulama: ${word(part('application'))}`
}

export interface DetailView {
  key: string
  label: string
  value: string
}

// The facts the server recorded about an event, in words. Known keys come first in a fixed order,
// unknown ones after them, sorted. `names` resolves "user:ID" / "team:ID" to a name when it can.
export function formatAuditDetails(
  details: Record<string, string> | undefined,
  names?: { users?: Lookup; teams?: Lookup },
): DetailView[] {
  const entries = Object.entries(details ?? {})
  const rank = (key: string) => {
    const i = DETAIL_ORDER.indexOf(key)
    return i < 0 ? DETAIL_ORDER.length : i
  }
  entries.sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
  return entries.map(([key, raw]) => {
    let value = raw
    if (key === 'reason') value = LOGIN_REASONS[raw] ?? raw
    else if (key === 'via') value = raw === 'api_token' ? 'API token' : raw
    else if (key === 'scope') value = describeAuditScope(raw)
    else if (key === 'disabled') value = raw === 'true' ? 'evet' : 'hayır'
    else if (key === 'to') {
      const [kind, id] = [raw.slice(0, raw.indexOf(':')), raw.slice(raw.indexOf(':') + 1)]
      const found = kind === 'user' ? names?.users?.get(id) : kind === 'team' ? names?.teams?.get(id) : undefined
      value = found ? `${kind === 'user' ? 'Kullanıcı' : 'Ekip'}: ${found}` : raw
    }
    return { key, label: DETAIL_LABELS[key] ?? key, value }
  })
}

// --- API tokens ---

export const TOKEN_LIFETIMES = [
  { days: 7, label: '7 gün' },
  { days: 30, label: '30 gün' },
  { days: 90, label: '90 gün' },
  { days: 180, label: '180 gün' },
  { days: 365, label: '1 yıl' },
]
export const DEFAULT_TOKEN_DAYS = 30
export const MAX_TOKENS = 20

export type TokenState = 'active' | 'expired'

export function tokenState(token: Pick<ApiToken, 'expiresAt'>, now: Date = new Date()): TokenState {
  return new Date(token.expiresAt).getTime() > now.getTime() ? 'active' : 'expired'
}
