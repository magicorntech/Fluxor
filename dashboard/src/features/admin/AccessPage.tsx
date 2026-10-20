import { PlusOutlined } from '@ant-design/icons'
import { Alert, App, Button, Flex, Form, Modal, Popconfirm, Radio, Select, Tabs, Tag, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useCreateRoleBinding, useDeleteRoleBinding, useRoleBindings, useRoles } from '../../api/access'
import type { Binding, Role } from '../../api/access'
import { useClusters } from '../../api/hooks'
import { useGroups } from '../../api/groups'
import { useTeams } from '../../api/teams'
import { useUsers } from '../../api/users'
import { DataTable } from '../../components/DataTable'
import { PageHeader } from '../../components/PageHeader'
import {
  buildBindingInput,
  describeCreator,
  describeScopeRef,
  describeSubject,
  formatSubjectParam,
  formatTime,
  parseSubjectParam,
  permissionLabel,
  roleDescription,
  roleName,
  scopeRules,
  sortByLabel,
  toLookup,
  userLabel,
} from './model'
import type { GrantForm } from './model'

export function AccessPage() {
  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="Yetkiler"
        description="Rol atamaları: kime, hangi rol, hangi kapsam. Roller sekmesinde yerleşik izin kümeleri."
      />
      <Tabs
        items={[
          { key: 'bindings', label: 'Atamalar', children: <BindingsTab /> },
          { key: 'roles', label: 'Roller', children: <RolesTab /> },
        ]}
      />
    </Flex>
  )
}

interface Option {
  value: string
  label: string
}

function BindingsTab() {
  const { message } = App.useApp()
  const [params, setParams] = useSearchParams()
  const subject = parseSubjectParam(params.get('subject'))
  const roles = useRoles()
  const users = useUsers()
  const teams = useTeams()
  const clusters = useClusters()
  const groups = useGroups()
  const bindings = useRoleBindings(subject ? { subjectKind: subject.kind, subjectId: subject.id } : {})
  const remove = useDeleteRoleBinding()
  const [granting, setGranting] = useState(false)

  const userNames = useMemo(() => toLookup(users.data, userLabel), [users.data])
  const teamNames = useMemo(() => toLookup(teams.data, (t) => t.name), [teams.data])
  const clusterNames = useMemo(() => toLookup(clusters.data, (c) => c.name), [clusters.data])
  const groupNames = useMemo(() => toLookup(groups.data, (g) => g.name), [groups.data])
  const roleNames = useMemo(() => toLookup(roles.data, roleName), [roles.data])

  const userOptions: Option[] = useMemo(() => sortByLabel(users.data ?? [], userLabel).map((u) => ({ value: u.id, label: userLabel(u) })), [users.data])
  const teamOptions: Option[] = useMemo(() => sortByLabel(teams.data ?? [], (t) => t.name).map((t) => ({ value: t.id, label: t.name })), [teams.data])
  const clusterOptions: Option[] = useMemo(() => sortByLabel(clusters.data ?? [], (c) => c.name).map((c) => ({ value: c.id, label: c.name })), [clusters.data])
  const groupOptions: Option[] = useMemo(() => sortByLabel(groups.data ?? [], (g) => g.name).map((g) => ({ value: g.id, label: g.name })), [groups.data])

  const setFilter = (value: string | undefined) => {
    const next = new URLSearchParams(params)
    if (value) next.set('subject', value)
    else next.delete('subject')
    setParams(next, { replace: true })
  }

  return (
    <Flex vertical gap={16}>
      <Flex justify="space-between" align="center" gap={16} wrap>
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          placeholder="Kullanıcı veya ekibe göre süz"
          style={{ minWidth: 320 }}
          value={subject ? formatSubjectParam(subject) : undefined}
          onChange={(v) => setFilter(v)}
          options={[
            { label: 'Kullanıcılar', options: userOptions.map((o) => ({ ...o, value: formatSubjectParam({ kind: 'user', id: o.value }) })) },
            { label: 'Ekipler', options: teamOptions.map((o) => ({ ...o, value: formatSubjectParam({ kind: 'team', id: o.value }) })) },
          ]}
        />
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setGranting(true)}>
          Yetki ver
        </Button>
      </Flex>

      <DataTable<Binding>
        rowKey="id"
        loading={bindings.isLoading}
        dataSource={bindings.data}
        error={bindings.isError ? bindings.error : null}
        emptyText="Henüz yetki ataması yok"
        columns={[
          {
            title: 'Özne',
            render: (_, b) => {
              const s = describeSubject(b.subject, userNames, teamNames)
              return (
                <Flex gap={8} align="center">
                  <Tag>{s.kindLabel}</Tag>
                  <Typography.Text type={s.found ? undefined : 'danger'}>{s.name}</Typography.Text>
                </Flex>
              )
            },
          },
          { title: 'Rol', render: (_, b) => <Tag color={b.roleId === 'admin' ? 'red' : 'blue'}>{roleNames.get(b.roleId) ?? b.roleId}</Tag> },
          { title: "Cluster kapsamı", render: (_, b) => describeScopeRef('cluster', b.scope.cluster, clusterNames, groupNames) },
          { title: 'Uygulama kapsamı', render: (_, b) => describeScopeRef('application', b.scope.application, new Map(), groupNames) },
          { title: 'Veren', render: (_, b) => describeCreator(b.createdBy, userNames) },
          { title: 'Tarih', dataIndex: 'createdAt', render: formatTime },
          {
            title: '',
            key: 'actions',
            render: (_, b) => (
              <Popconfirm
                title="Bu yetki kaldırılsın mı?"
                description="Kullanıcı ya da ekip üyeleri bu yetkiyi hemen kaybeder."
                okText="Kaldır"
                cancelText="Vazgeç"
                onConfirm={() =>
                  remove.mutate(b.id, {
                    onSuccess: () => message.success('Yetki kaldırıldı'),
                    onError: (e) => message.error(e.message),
                  })
                }
              >
                <Button type="link" size="small" danger>
                  Kaldır
                </Button>
              </Popconfirm>
            ),
          },
        ]}
      />

      {granting && (
        <GrantModal
          roles={roles.data ?? []}
          userOptions={userOptions}
          teamOptions={teamOptions}
          clusterOptions={clusterOptions}
          groupOptions={groupOptions}
          initialSubject={subject}
          onClose={() => setGranting(false)}
        />
      )}
    </Flex>
  )
}

function GrantModal({
  roles,
  userOptions,
  teamOptions,
  clusterOptions,
  groupOptions,
  initialSubject,
  onClose,
}: {
  roles: Role[]
  userOptions: Option[]
  teamOptions: Option[]
  clusterOptions: Option[]
  groupOptions: Option[]
  initialSubject?: { kind: 'user' | 'team'; id: string }
  onClose: () => void
}) {
  const { message } = App.useApp()
  const create = useCreateRoleBinding()
  const [form] = Form.useForm<GrantForm>()
  const subjectKind = Form.useWatch('subjectKind', form) ?? initialSubject?.kind ?? 'user'
  const roleId = Form.useWatch('roleId', form)
  const clusterKind = Form.useWatch('clusterKind', form) ?? 'any'

  const role = roles.find((r) => r.id === roleId)
  const rules = scopeRules(role)

  return (
    <Modal open title="Yetki ver" okText="Yetki ver" cancelText="Vazgeç" confirmLoading={create.isPending} onOk={() => form.submit()} onCancel={onClose} width={560}>
      <Form<GrantForm>
        form={form}
        layout="vertical"
        initialValues={{
          subjectKind: initialSubject?.kind ?? 'user',
          subjectId: initialSubject?.id,
          clusterKind: 'any',
        }}
        onValuesChange={(changed, all) => {
          // Another kind of subject makes the chosen one meaningless.
          if ('subjectKind' in changed) form.setFieldValue('subjectId', undefined)
          // A role that cannot be narrowed always applies everywhere.
          if ('roleId' in changed && !scopeRules(roles.find((r) => r.id === all.roleId)).clusterScope) {
            form.setFieldsValue({ clusterKind: 'any', clusterId: undefined })
          }
          if ('clusterKind' in changed) form.setFieldValue('clusterId', undefined)
        }}
        onFinish={(v) =>
          create.mutate(buildBindingInput(v, rules), {
            onSuccess: () => {
              message.success('Yetki verildi')
              onClose()
            },
            onError: (e) => message.error(e.message),
          })
        }
      >
        <Form.Item name="subjectKind" label="Kime">
          <Radio.Group
            optionType="button"
            options={[
              { label: 'Kullanıcı', value: 'user' },
              { label: 'Ekip', value: 'team' },
            ]}
          />
        </Form.Item>
        <Form.Item name="subjectId" rules={[{ required: true, message: 'Seçim yapın' }]}>
          <Select
            key={subjectKind}
            showSearch
            optionFilterProp="label"
            placeholder={subjectKind === 'user' ? 'Kullanıcı seçin' : 'Ekip seçin'}
            options={subjectKind === 'user' ? userOptions : teamOptions}
          />
        </Form.Item>

        <Form.Item name="roleId" label="Rol" rules={[{ required: true, message: 'Rol seçin' }]}>
          <Select placeholder="Rol seçin" options={roles.map((r) => ({ value: r.id, label: `${roleName(r)} — ${roleDescription(r)}` }))} />
        </Form.Item>
        {role && (
          <Flex gap={4} wrap style={{ marginBottom: 16 }}>
            {role.permissions.map((p) => (
              <Tag key={p} color={p === '*' ? 'red' : undefined}>
                {permissionLabel(p)}
              </Tag>
            ))}
          </Flex>
        )}

        <Form.Item name="clusterKind" label="Cluster kapsamı">
          <Radio.Group
            disabled={!rules.clusterScope}
            options={[
              { label: "Tüm cluster'lar", value: 'any' },
              { label: 'Tek cluster', value: 'id' },
              { label: 'Cluster grubu', value: 'group' },
            ]}
          />
        </Form.Item>
        {rules.clusterScope && clusterKind !== 'any' && (
          <Form.Item name="clusterId" rules={[{ required: true, message: clusterKind === 'id' ? 'Cluster seçin' : 'Grup seçin' }]}>
            <Select
              showSearch
              optionFilterProp="label"
              placeholder={clusterKind === 'id' ? 'Cluster seçin' : 'Grup seçin'}
              options={clusterKind === 'id' ? clusterOptions : groupOptions}
            />
          </Form.Item>
        )}
        <Flex vertical gap={4}>
          <Typography.Text type="secondary">Uygulama kapsamı: tüm uygulamalar</Typography.Text>
          {rules.reason && <Alert type="info" showIcon title={rules.reason} />}
        </Flex>
      </Form>
    </Modal>
  )
}

function RolesTab() {
  const roles = useRoles()
  return (
    <Flex vertical gap={12}>
      <Typography.Text type="secondary">
        Roller hazır gelir ve değiştirilemez. Yetki, bir rolün bir kullanıcıya ya da ekibe, bir kapsamda verilmesiyle oluşur.
      </Typography.Text>
      <DataTable<Role>
        rowKey="id"
        loading={roles.isLoading}
        dataSource={roles.data}
        error={roles.isError ? roles.error : null}
        emptyText="Rol yok"
        columns={[
          {
            title: 'Rol',
            render: (_, r) => (
              <Flex vertical>
                <Typography.Text strong>{roleName(r)}</Typography.Text>
                <Typography.Text type="secondary" code>
                  {r.id}
                </Typography.Text>
              </Flex>
            ),
          },
          { title: 'Açıklama', render: (_, r) => roleDescription(r) },
          {
            title: 'İzinler',
            render: (_, r) => (
              <Flex gap={4} wrap>
                {r.permissions.map((p) => (
                  <Tag key={p} color={p === '*' ? 'red' : undefined}>
                    {permissionLabel(p)}
                  </Tag>
                ))}
              </Flex>
            ),
          },
        ]}
      />
    </Flex>
  )
}
