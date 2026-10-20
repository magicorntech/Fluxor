import { describe, expect, it } from 'vitest'
import {
  buildBindingInput,
  describeCreator,
  describeScopeRef,
  describeSubject,
  formatSubjectParam,
  formatTime,
  AUDIT_ACTION_GROUPS,
  AUDIT_RESULT_LABELS,
  auditActionLabel,
  auditActor,
  auditSubjectText,
  describeAuditScope,
  formatAuditDetails,
  tokenState,
  TOKEN_LIFETIMES,
  DEFAULT_TOKEN_DAYS,
  hasPermission,
  namesOf,
  permissionSignature,
  roleDescription,
  roleName,
  parseSubjectParam,
  permissionLabel,
  scopeRules,
  sortByLabel,
  toLookup,
  userLabel,
  userStatus,
} from './model'

const lookup = (entries: Record<string, string>) => new Map(Object.entries(entries))

describe('userStatus', () => {
  it('is active for an ordinary account', () => {
    expect(userStatus({ disabled: false, mustChangePassword: false })).toBe('active')
  })
  it('asks for a password change while one is pending', () => {
    expect(userStatus({ disabled: false, mustChangePassword: true })).toBe('mustChange')
  })
  it('shows a disabled account as disabled even if a password change is pending', () => {
    expect(userStatus({ disabled: true, mustChangePassword: true })).toBe('disabled')
    expect(userStatus({ disabled: true, mustChangePassword: false })).toBe('disabled')
  })
})

describe('hasPermission', () => {
  it('finds a listed permission', () => {
    expect(hasPermission(['user:manage', 'rbac:manage'], 'rbac:manage')).toBe(true)
  })
  it('is false for a missing permission, an empty list and no list at all', () => {
    expect(hasPermission(['user:manage'], 'team:manage')).toBe(false)
    expect(hasPermission([], 'user:manage')).toBe(false)
    expect(hasPermission(undefined, 'user:manage')).toBe(false)
  })
})

describe('permissionLabel', () => {
  it('translates known permissions and the wildcard', () => {
    expect(permissionLabel('*')).toBe('Tüm izinler')
    expect(permissionLabel('cluster:view')).toBe("Cluster'ları görüntüle")
  })
  it('falls back to the raw name for unknown permissions', () => {
    expect(permissionLabel('thing:fly')).toBe('thing:fly')
  })
})

describe('describeSubject', () => {
  const users = lookup({ u1: 'Ayşe' })
  const teams = lookup({ t1: 'Ödeme' })
  it('resolves users and teams by their own namespace', () => {
    expect(describeSubject({ kind: 'user', id: 'u1' }, users, teams)).toEqual({ kindLabel: 'Kullanıcı', name: 'Ayşe', found: true })
    expect(describeSubject({ kind: 'team', id: 't1' }, users, teams)).toEqual({ kindLabel: 'Ekip', name: 'Ödeme', found: true })
  })
  it('does not mix up a user id with a team id', () => {
    expect(describeSubject({ kind: 'team', id: 'u1' }, users, teams).found).toBe(false)
  })
  it('marks a deleted subject instead of showing a blank', () => {
    expect(describeSubject({ kind: 'user', id: 'gone' }, users, teams)).toEqual({ kindLabel: 'Kullanıcı', name: 'silinmiş (gone)', found: false })
  })
})

describe('describeScopeRef', () => {
  const clusters = lookup({ c1: 'prod-eu' })
  const groups = lookup({ g1: 'prod' })
  it('words the three kinds of reference', () => {
    expect(describeScopeRef('cluster', { kind: 'any' }, clusters, groups)).toBe("Tüm cluster'lar")
    expect(describeScopeRef('application', { kind: 'any' }, clusters, groups)).toBe('Tüm uygulamalar')
    expect(describeScopeRef('cluster', { kind: 'id', id: 'c1' }, clusters, groups)).toBe('Cluster: prod-eu')
    expect(describeScopeRef('cluster', { kind: 'group', id: 'g1' }, clusters, groups)).toBe('Grup: prod')
  })
  it('flags references to things that no longer exist', () => {
    expect(describeScopeRef('cluster', { kind: 'id', id: 'x' }, clusters, groups)).toBe('Cluster: silinmiş (x)')
    expect(describeScopeRef('cluster', { kind: 'group', id: 'y' }, clusters, groups)).toBe('Grup: silinmiş (y)')
  })
})

describe('scopeRules', () => {
  it('lets an ordinary role be limited to clusters, but not (yet) to applications', () => {
    const rules = scopeRules({ permissions: ['cluster:view', 'app:view', 'app:sync'] })
    expect(rules.clusterScope).toBe(true)
    expect(rules.applicationScope).toBe(false)
    expect(rules.reason).toContain('Uygulama kapsamı')
  })
  it('never lets the administrator role be limited', () => {
    const rules = scopeRules({ permissions: ['*'] })
    expect(rules).toMatchObject({ clusterScope: false, applicationScope: false })
    expect(rules.reason).toContain('sistem izinleri')
  })
  it('treats any role with a system permission like the administrator role', () => {
    expect(scopeRules({ permissions: ['cluster:view', 'user:manage'] }).clusterScope).toBe(false)
    expect(scopeRules({ permissions: ['rbac:manage'] }).clusterScope).toBe(false)
  })
  it('allows nothing before a role is chosen', () => {
    expect(scopeRules(undefined)).toEqual({ clusterScope: false, applicationScope: false })
  })
})

describe('buildBindingInput', () => {
  const free = { clusterScope: true, applicationScope: false }
  const locked = { clusterScope: false, applicationScope: false }
  const base = { subjectKind: 'team' as const, subjectId: 't1', roleId: 'deployer' }

  it('grants everywhere by default', () => {
    expect(buildBindingInput({ ...base, clusterKind: 'any' }, free)).toEqual({
      subject: { kind: 'team', id: 't1' },
      roleId: 'deployer',
      scope: { cluster: { kind: 'any' } },
    })
  })
  it('limits to one cluster or to a cluster group', () => {
    expect(buildBindingInput({ ...base, clusterKind: 'id', clusterId: 'c1' }, free).scope).toEqual({ cluster: { kind: 'id', id: 'c1' } })
    expect(buildBindingInput({ ...base, clusterKind: 'group', clusterId: 'g1' }, free).scope).toEqual({ cluster: { kind: 'group', id: 'g1' } })
  })
  it('falls back to "all" when a narrow scope was chosen but no resource picked', () => {
    expect(buildBindingInput({ ...base, clusterKind: 'id' }, free).scope).toEqual({ cluster: { kind: 'any' } })
  })
  it('drops a scope the role does not allow instead of sending it', () => {
    expect(buildBindingInput({ ...base, roleId: 'admin', clusterKind: 'id', clusterId: 'c1' }, locked).scope).toEqual({ cluster: { kind: 'any' } })
  })
})

describe('subject URL parameter', () => {
  it('round-trips', () => {
    expect(parseSubjectParam(formatSubjectParam({ kind: 'user', id: 'abc' }))).toEqual({ kind: 'user', id: 'abc' })
    expect(parseSubjectParam(formatSubjectParam({ kind: 'team', id: 'x:y' }))).toEqual({ kind: 'team', id: 'x:y' })
  })
  it('ignores anything malformed', () => {
    for (const bad of [null, '', 'user', 'user:', ':abc', 'robot:abc']) {
      expect(parseSubjectParam(bad)).toBeUndefined()
    }
  })
})

describe('describeCreator', () => {
  const users = lookup({ u1: 'Ayşe' })
  it('names the creator, the setup, and a creator that no longer exists', () => {
    expect(describeCreator('u1', users)).toBe('Ayşe')
    expect(describeCreator('bootstrap', users)).toBe('İlk kurulum')
    expect(describeCreator('gone', users)).toBe('silinmiş (gone)')
    expect(describeCreator(undefined, users)).toBe('—')
  })
})

describe('small helpers', () => {
  it('userLabel shows the display name, the login and a disabled marker', () => {
    expect(userLabel({ username: 'ayse' })).toBe('ayse')
    expect(userLabel({ username: 'ayse', displayName: 'Ayşe' })).toBe('Ayşe (ayse)')
    expect(userLabel({ username: 'ayse', disabled: true })).toBe('ayse — devre dışı')
  })
  it('sortByLabel sorts with Turkish rules and leaves the input alone', () => {
    const input = [{ n: 'Zeynep' }, { n: 'Çağla' }, { n: 'Ali' }]
    expect(sortByLabel(input, (x) => x.n).map((x) => x.n)).toEqual(['Ali', 'Çağla', 'Zeynep'])
    expect(input[0].n).toBe('Zeynep')
  })
  it('toLookup maps ids to names and tolerates a missing list', () => {
    expect(toLookup([{ id: 'a', n: 'A' }], (x) => x.n).get('a')).toBe('A')
    expect(toLookup(undefined, () => '').size).toBe(0)
  })
  it('formatTime handles missing and invalid input', () => {
    expect(formatTime(undefined)).toBe('—')
    expect(formatTime('not a date')).toBe('—')
    expect(formatTime('2026-10-20T10:00:00Z')).not.toBe('—')
  })
})

describe('role text', () => {
  it('shows the built-in roles in Turkish', () => {
    expect(roleName({ id: 'admin', name: 'Administrator' })).toBe('Yönetici')
    expect(roleName({ id: 'cluster-admin', name: 'Cluster admin' })).toBe('Cluster yöneticisi')
    expect(roleDescription({ id: 'viewer', description: 'See clusters' })).toContain('görür')
  })
  it('falls back to the server text for a role it does not know', () => {
    expect(roleName({ id: 'custom', name: 'Auditor' })).toBe('Auditor')
    expect(roleDescription({ id: 'custom', description: 'Reads audit logs' })).toBe('Reads audit logs')
  })
})

describe('permissionSignature', () => {
  const base = { permissions: ['user:manage', 'rbac:manage'], canCreateClusters: true, teams: [{ id: 't1', name: 'A' }, { id: 't2', name: 'B' }] }
  it('is the same for the same summary, whatever the order', () => {
    const shuffled = { permissions: ['rbac:manage', 'user:manage'], canCreateClusters: true, teams: [{ id: 't2', name: 'B' }, { id: 't1', name: 'A' }] }
    expect(permissionSignature(shuffled)).toBe(permissionSignature(base))
  })
  it('ignores a team being renamed, since that does not change what the user may see', () => {
    expect(permissionSignature({ ...base, teams: [{ id: 't1', name: 'Renamed' }, { id: 't2', name: 'B' }] })).toBe(permissionSignature(base))
  })
  it('changes when a permission is gained or lost', () => {
    expect(permissionSignature({ ...base, permissions: ['user:manage'] })).not.toBe(permissionSignature(base))
    expect(permissionSignature({ ...base, permissions: [...base.permissions, 'team:manage'] })).not.toBe(permissionSignature(base))
  })
  it('changes when the right to create clusters changes', () => {
    expect(permissionSignature({ ...base, canCreateClusters: false })).not.toBe(permissionSignature(base))
  })
  it('changes when the user joins or leaves a team', () => {
    expect(permissionSignature({ ...base, teams: [{ id: 't1', name: 'A' }] })).not.toBe(permissionSignature(base))
    expect(permissionSignature({ ...base, teams: [] })).not.toBe(permissionSignature(base))
  })
})

describe('namesOf', () => {
  it('sorts names with Turkish rules and shows unknown ids as they are', () => {
    const names = new Map([['a', 'Zeynep'], ['b', 'Çağla']])
    expect(namesOf(['a', 'b', 'x'], names)).toEqual(['Çağla', 'x', 'Zeynep'])
  })
  it('copes with an empty list', () => {
    expect(namesOf([], new Map())).toEqual([])
  })
})

describe('audit actions', () => {
  // The names the server writes (api: s.handle(...) and the refresh-reuse event). If the server gains an
  // action, add it here and to AUDIT_ACTION_GROUPS together, so the log never shows a raw code by accident.
  const serverActions = [
    'auth.login', 'auth.password_change', 'auth.refresh_reuse',
    'token.create', 'token.revoke',
    'user.create', 'user.update', 'user.reset_password',
    'team.create', 'team.update', 'team.delete', 'team.set_members',
    'group.create', 'group.update', 'group.delete', 'group.set_members',
    'role_binding.create', 'role_binding.delete',
    'cluster.create', 'cluster.delete',
  ]
  it('has a Turkish label for every action the server writes, and no others', () => {
    const known = AUDIT_ACTION_GROUPS.flatMap((g) => g.actions.map((a) => a.value))
    expect([...known].sort()).toEqual([...serverActions].sort())
    for (const action of serverActions) expect(auditActionLabel(action)).not.toBe(action)
  })
  it('lists each action once', () => {
    const known = AUDIT_ACTION_GROUPS.flatMap((g) => g.actions.map((a) => a.value))
    expect(new Set(known).size).toBe(known.length)
  })
  it('shows an unknown action by its own name', () => {
    expect(auditActionLabel('thing.explode')).toBe('thing.explode')
  })
  it('has a label for every result', () => {
    for (const r of ['success', 'denied', 'failure', 'error'] as const) expect(AUDIT_RESULT_LABELS[r].text).toBeTruthy()
  })
})

describe('auditActor and auditSubjectText', () => {
  it('names the user, or the name that was tried when nobody was signed in', () => {
    expect(auditActor({ actorId: 'u1', actorName: 'ayse' })).toEqual({ name: 'ayse', signedIn: true })
    expect(auditActor({ actorId: undefined, actorName: 'ghost' })).toEqual({ name: 'ghost', signedIn: false })
    expect(auditActor({})).toEqual({ name: '—', signedIn: false })
  })
  it('prefers the recorded name, since a deleted subject cannot be looked up', () => {
    expect(auditSubjectText({ subject: 'cluster:c1', subjectName: 'alpha' })).toBe('alpha')
    expect(auditSubjectText({ subject: 'cluster:c1' })).toBe('cluster:c1')
    expect(auditSubjectText({})).toBe('—')
  })
})

describe('describeAuditScope', () => {
  it('words the scope', () => {
    expect(describeAuditScope('cluster=any application=any')).toBe('Cluster: tümü · Uygulama: tümü')
    expect(describeAuditScope('cluster=id:c1 application=any')).toBe('Cluster: c1 · Uygulama: tümü')
    expect(describeAuditScope('cluster=group:g1 application=any')).toBe('Cluster: grup g1 · Uygulama: tümü')
  })
})

describe('formatAuditDetails', () => {
  it('translates known keys and values', () => {
    const got = formatAuditDetails({ reason: 'bad_password', via: 'api_token', disabled: 'true' })
    expect(got).toEqual([
      { key: 'reason', label: 'Neden', value: 'Hatalı parola' },
      { key: 'via', label: 'Yöntem', value: 'API token' },
      { key: 'disabled', label: 'Devre dışı', value: 'evet' },
    ])
  })
  it('puts known keys in a fixed order and unknown ones last, sorted', () => {
    const keys = formatAuditDetails({ zzz: '1', members: '3', aaa: '2', role: 'viewer' }).map((d) => d.key)
    expect(keys).toEqual(['role', 'members', 'aaa', 'zzz'])
  })
  it('resolves who a grant went to when the name is known, and shows the raw value otherwise', () => {
    const users = new Map([['u1', 'Ayşe']])
    const teams = new Map([['t1', 'Ödeme']])
    expect(formatAuditDetails({ to: 'user:u1' }, { users, teams })[0].value).toBe('Kullanıcı: Ayşe')
    expect(formatAuditDetails({ to: 'team:t1' }, { users, teams })[0].value).toBe('Ekip: Ödeme')
    expect(formatAuditDetails({ to: 'user:gone' }, { users, teams })[0].value).toBe('user:gone')
    expect(formatAuditDetails({ to: 'user:u1' })[0].value).toBe('user:u1')
  })
  it('keeps unknown reasons and unknown keys as they are', () => {
    expect(formatAuditDetails({ reason: 'new_reason' })[0].value).toBe('new_reason')
    expect(formatAuditDetails({ shiny: 'x' })[0].label).toBe('shiny')
  })
  it('copes with no details', () => {
    expect(formatAuditDetails(undefined)).toEqual([])
    expect(formatAuditDetails({})).toEqual([])
  })
})

describe('tokens', () => {
  it('knows whether a token has expired', () => {
    const now = new Date('2026-10-20T12:00:00Z')
    expect(tokenState({ expiresAt: '2026-10-21T00:00:00Z' }, now)).toBe('active')
    expect(tokenState({ expiresAt: '2026-10-20T11:59:59Z' }, now)).toBe('expired')
    expect(tokenState({ expiresAt: '2026-10-20T12:00:00Z' }, now)).toBe('expired')
  })
  it('offers lifetimes the server accepts, including the default', () => {
    for (const l of TOKEN_LIFETIMES) expect(l.days >= 1 && l.days <= 365).toBe(true)
    expect(TOKEN_LIFETIMES.map((l) => l.days)).toContain(DEFAULT_TOKEN_DAYS)
  })
})
