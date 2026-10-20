import { ReloadOutlined } from '@ant-design/icons'
import { Button, DatePicker, Flex, Select, Space, Tag, Tooltip, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useAuditEvents } from '../../api/audit'
import type { AuditEvent, AuditFilter, AuditResult } from '../../api/audit'
import { useTeams } from '../../api/teams'
import { useUsers } from '../../api/users'
import { DataTable } from '../../components/DataTable'
import { PageHeader } from '../../components/PageHeader'
import {
  AUDIT_ACTION_GROUPS,
  AUDIT_RESULT_LABELS,
  auditActionLabel,
  auditActor,
  auditSubjectText,
  formatAuditDetails,
  sortByLabel,
  toLookup,
  userLabel,
} from './model'

const { RangePicker } = DatePicker

export function AuditPage() {
  const [filter, setFilter] = useState<AuditFilter>({})
  const audit = useAuditEvents(filter)
  const users = useUsers()
  const teams = useTeams()

  const events = useMemo(() => audit.data?.pages.flatMap((p) => p.items) ?? [], [audit.data])
  const userNames = useMemo(() => toLookup(users.data, userLabel), [users.data])
  const teamNames = useMemo(() => toLookup(teams.data, (t) => t.name), [teams.data])
  const userOptions = useMemo(
    () => sortByLabel(users.data ?? [], userLabel).map((u) => ({ value: u.id, label: userLabel(u) })),
    [users.data],
  )

  const update = (patch: Partial<AuditFilter>) => setFilter((f) => ({ ...f, ...patch }))

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="Denetim"
        description="Değişiklikler ve reddedilen denemeler. Okumalar kaydedilmez; sır asla yazılmaz."
        extra={
          <Button icon={<ReloadOutlined />} loading={audit.isRefetching} onClick={() => void audit.refetch()}>
            Yenile
          </Button>
        }
      />

      <Flex gap={12} wrap>
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          placeholder="Kullanıcı"
          style={{ width: 240 }}
          options={userOptions}
          onChange={(v) => update({ actor: v })}
        />
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          placeholder="İşlem"
          style={{ width: 280 }}
          options={AUDIT_ACTION_GROUPS.map((g) => ({ label: g.label, options: g.actions }))}
          onChange={(v) => update({ action: v })}
        />
        <Select<AuditResult>
          allowClear
          placeholder="Sonuç"
          style={{ width: 160 }}
          options={(Object.keys(AUDIT_RESULT_LABELS) as AuditResult[]).map((r) => ({ value: r, label: AUDIT_RESULT_LABELS[r].text }))}
          onChange={(v) => update({ result: v })}
        />
        <RangePicker
          showTime
          allowClear
          placeholder={['Başlangıç', 'Bitiş']}
          onChange={(range) => update({ from: range?.[0]?.toISOString(), to: range?.[1]?.toISOString() })}
        />
      </Flex>

      <DataTable<AuditEvent>
        rowKey="id"
        size="small"
        loading={audit.isLoading}
        dataSource={events}
        error={audit.isError ? audit.error : null}
        emptyText="Kayıt yok"
        columns={[
          {
            title: 'Zaman',
            dataIndex: 'time',
            width: 170,
            render: (t: string) => new Date(t).toLocaleString('tr-TR'),
          },
          {
            title: 'Kullanıcı',
            render: (_, ev) => {
              const a = auditActor(ev)
              return a.signedIn ? a.name : <Tooltip title="Oturum açılmamıştı; denenen ad">{a.name} <Tag>oturumsuz</Tag></Tooltip>
            },
          },
          {
            title: 'İşlem',
            render: (_, ev) => (
              <Flex vertical>
                <Typography.Text>{auditActionLabel(ev.action)}</Typography.Text>
                <Typography.Text type="secondary" code>
                  {ev.action}
                </Typography.Text>
              </Flex>
            ),
          },
          { title: 'Konu', render: (_, ev) => auditSubjectText(ev) },
          {
            title: 'Sonuç',
            width: 140,
            render: (_, ev) => {
              const r = AUDIT_RESULT_LABELS[ev.result]
              return (
                <Tag color={r.color}>
                  {r.text} · {ev.status}
                </Tag>
              )
            },
          },
          { title: 'Adres', dataIndex: 'ip', width: 130, render: (v?: string) => v || '—' },
          {
            title: 'Ayrıntı',
            render: (_, ev) => (
              <Space size={[4, 4]} wrap>
                {formatAuditDetails(ev.details, { users: userNames, teams: teamNames }).map((d) => (
                  <Tag key={d.key}>
                    {d.label}: {d.value}
                  </Tag>
                ))}
              </Space>
            ),
          },
        ]}
      />

      {audit.hasNextPage && (
        <Flex justify="center">
          <Button loading={audit.isFetchingNextPage} onClick={() => void audit.fetchNextPage()}>
            Daha eski kayıtları yükle
          </Button>
        </Flex>
      )}
    </Flex>
  )
}
