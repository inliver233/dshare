import { useEffect, useState } from 'react'
import {
  ActionIcon,
  Badge,
  Box,
  Button,
  Card,
  Code,
  Group,
  NumberInput,
  Select,
  SimpleGrid,
  Stack,
  Switch,
  Table,
  Text,
  TextInput,
  Textarea,
  ThemeIcon,
  Title,
} from '@mantine/core'
import { Activity, CheckCircle2, Copy, Gauge, KeyRound, Shield, Trash2, Upload, Users, Search } from 'lucide-react'
import type { APIKey, AdminStats, Contribution, ImportResponse, MeResponse, ServiceConfig, User } from '../types/api'
import { api } from '../services/api'
import { CopyBox, CopyButton } from '../components/CopyButton'
import { EmptyState, ErrorText } from '../components/Feedback'
import { copyText } from '../utils/clipboard'
import { displayName, formatDate, formatNumber } from '../utils/format'

export function DashboardPage({ me, onChanged }: { me: MeResponse; onChanged: () => void }) {
  const [contributionsVersion, setContributionsVersion] = useState(0)
  const refreshAfterImport = () => {
    onChanged()
    setContributionsVersion((version) => version + 1)
  }
  return (
    <Stack gap="xl">
      <Stats me={me} />
      <KeyManager keys={me.keys} onChanged={onChanged} />
      <ImportPanel onDone={refreshAfterImport} />
      <ContributionList version={contributionsVersion} onChanged={onChanged} />
    </Stack>
  )
}

export function AdminSections() {
  return (
    <Stack gap="xl">
      <ServiceConfigPanel />
      <AdminPanel />
    </Stack>
  )
}

function SectionTitle({ icon: Icon, title }: { icon: typeof KeyRound; title: string }) {
  return (
    <Group gap="xs" mb="md">
      <ThemeIcon variant="light" color="brandBlue" radius="md" size={32}><Icon size={18} /></ThemeIcon>
      <Title order={2}>{title}</Title>
    </Group>
  )
}

type MetricCardProps = {
  label: string
  value: string
  icon: typeof KeyRound
}

function MetricCard({ label, value, icon: Icon }: MetricCardProps) {
  const slashIndex = value.indexOf('/')
  const hasSplitValue = slashIndex > 0
  const mainValue = hasSplitValue ? value.slice(0, slashIndex) : value
  const suffixValue = hasSplitValue ? value.slice(slashIndex) : ''

  return (
    <Card className="metricCard">
      <Group justify="space-between" align="flex-start" gap="sm" className="metricHeader">
        <Text className="metricLabel">{label}</Text>
        <ThemeIcon variant="light" color="brandBlue" radius="sm" size={32}><Icon size={18} /></ThemeIcon>
      </Group>
      <Group className="metricValueGroup" gap={4} align="baseline" wrap="nowrap">
        <Text className="metricValue">{mainValue}</Text>
        {suffixValue && <Text className="metricSuffix">{suffixValue.replace('/', '/ ')}</Text>}
      </Group>
    </Card>
  )
}

function Stats({ me }: { me: MeResponse }) {
  const dailyLimit = me.user.requests_per_day
  const dailyUsage = dailyLimit > 0 ? `${formatNumber(me.stats.requests_today)}/${formatNumber(dailyLimit)}` : `${formatNumber(me.stats.requests_today)}/不限`
  const remaining = dailyLimit > 0 ? formatNumber(me.stats.requests_remaining) : '不限'
  const cards = [
    ['有效上传', formatNumber(me.stats.valid_uploads), CheckCircle2],
    ['总调用量', formatNumber(me.stats.total_requests), Activity],
    ['今日已用', dailyUsage, Gauge],
    ['剩余额度', remaining, Gauge],
    ['每分钟限制', me.user.requests_per_minute ? formatNumber(me.user.requests_per_minute) : '不限', Gauge],
  ] as const

  return (
    <SimpleGrid cols={{ base: 1, xs: 2, md: 5 }} spacing="md">
      {cards.map(([label, value, Icon]) => (
        <MetricCard label={label} value={value} icon={Icon} key={label} />
      ))}
    </SimpleGrid>
  )
}

function KeyManager({ keys, onChanged }: { keys: APIKey[]; onChanged: () => void }) {
  const [name, setName] = useState('default')
  const [created, setCreated] = useState<APIKey | null>(null)
  const [deleting, setDeleting] = useState<number | null>(null)
  const create = async () => {
    const key = await api<APIKey>('/api/keys', { method: 'POST', body: JSON.stringify({ name }) })
    setCreated(key)
    onChanged()
  }
  const revoke = async (id: number) => {
    if (!window.confirm('确认删除这个 API Key？删除后无法恢复。')) return
    setDeleting(id)
    try {
      await api(`/api/keys/${id}`, { method: 'DELETE' })
      if (created?.id === id) setCreated(null)
      onChanged()
    } finally {
      setDeleting(null)
    }
  }

  return (
    <Card>
      <SectionTitle icon={KeyRound} title="项目 API Key" />
      <Group gap="sm" align="flex-end" mb="md">
        <TextInput className="keyNameInput" value={name} onChange={(e) => setName(e.currentTarget.value)} placeholder="Key 名称" />
        <Button leftSection={<KeyRound size={18} />} onClick={create}>创建</Button>
      </Group>
      {created?.key && <CopyBox value={created.key} />}
      {keys.length === 0 ? <EmptyState>还没有 API Key</EmptyState> : (
        <Table className="dataTable keyTable" highlightOnHover={false}>
          <Table.Tbody>
            {keys.map((key) => (
              <Table.Tr key={key.id}>
                <Table.Td>
                  <Text fw={500} size="sm" truncate>{key.name || 'unnamed'}</Text>
                  <Text size="xs" c="var(--color-text-muted)">创建 {formatDate(key.created_at)}</Text>
                </Table.Td>
                <Table.Td><Code className="machineCode">{key.masked_key}</Code></Table.Td>
                <Table.Td className="keyUsageCell">
                  <Text size="sm" c="var(--color-text-body)">今日 {formatNumber(key.requests_today)}</Text>
                  <Text size="sm" c="var(--color-text-body)">总计 {formatNumber(key.total_requests)}</Text>
                </Table.Td>
                <Table.Td><Text size="xs" c="var(--color-text-muted)">{key.last_used_at ? `最后使用 ${formatDate(key.last_used_at)}` : '尚未使用'}</Text></Table.Td>
                <Table.Td>
                  {key.key ? <CopyButton value={key.key} className="copyAction" /> : <Text size="xs" c="var(--color-text-muted)">旧 Key 不可复制</Text>}
                </Table.Td>
                <Table.Td>
                  <ActionIcon
                    className="deleteAction"
                    disabled={deleting === key.id}
                    onClick={() => revoke(key.id)}
                    title="删除"
                    aria-label="删除"
                  >
                    <Trash2 size={18} />
                  </ActionIcon>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </Card>
  )
}

function ImportPanel({ onDone }: { onDone: () => void }) {
  const [lines, setLines] = useState('')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<ImportResponse | null>(null)
  const [error, setError] = useState('')
  const submit = async () => {
    setLoading(true)
    setError('')
    try {
      const normalized = lines.replace(/：/g, ':')
      const data = await api<ImportResponse>('/api/ds2api/import', { method: 'POST', body: JSON.stringify({ lines: normalized }) })
      setResult(data)
      onDone()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }
  return (
    <Card>
      <SectionTitle icon={Upload} title="DeepSeek 账号贡献" />
      <Box className="importCanvas">
        <Textarea
          autosize
          minRows={8}
          value={lines}
          onChange={(e) => setLines(e.currentTarget.value)}
          placeholder={'账号:密码\nuser@example.com:password'}
          className="monoInput importTextarea"
        />
        <Button className="importButton" variant="default" leftSection={<Upload size={18} />} onClick={submit} disabled={loading || !lines.trim()}>
          {loading ? '验证中' : '导入并验证'}
        </Button>
      </Box>
      <ErrorText>{error}</ErrorText>
      {result && (
        <Stack gap={6} mt="md">
          <Text fw={600} size="sm">总数 {result.total}，有效 {result.valid}，无效 {result.invalid}，重复 {result.duplicate}</Text>
          {result.results?.slice(0, 20).map((r) => <Text className={`mini ${r.status}`} key={r.account}>{r.account} · {r.status} · {r.message}</Text>)}
        </Stack>
      )}
    </Card>
  )
}

function ContributionList({ version, onChanged }: { version: number; onChanged: () => void }) {
  const [items, setItems] = useState<Contribution[]>([])
  const [deleting, setDeleting] = useState<number | null>(null)
  const load = async () => {
    const data = await api<{ items: Contribution[] }>('/api/contributions')
    setItems(data.items)
  }
  useEffect(() => {
    load().catch(() => undefined)
  }, [version])
  const remove = async (item: Contribution) => {
    if (!window.confirm(`确认从 ds2api 删除账号 ${item.account}？删除后该贡献额度会被回收。`)) return
    setDeleting(item.id)
    try {
      await api(`/api/contributions/${item.id}`, { method: 'DELETE' })
      await load()
      onChanged()
    } finally {
      setDeleting(null)
    }
  }

  return (
    <Card>
      <SectionTitle icon={CheckCircle2} title="最近贡献" />
      {items.length === 0 ? <EmptyState>还没有贡献记录</EmptyState> : (
        <Table className="dataTable" verticalSpacing="md">
          <Table.Tbody>
            {items.map((item) => (
              <Table.Tr key={item.id}>
                <Table.Td><Text size="sm" truncate>{item.account}</Text></Table.Td>
                <Table.Td><Badge className={item.status} variant="light">{item.status}</Badge></Table.Td>
                <Table.Td><Text size="xs" c="var(--color-text-muted)">{item.message}</Text></Table.Td>
                <Table.Td><Text size="xs" c="var(--color-text-muted)">{formatDate(item.created_at)}</Text></Table.Td>
                <Table.Td>
                  {item.status === 'valid' && (
                    <ActionIcon className="deleteAction" disabled={deleting === item.id} onClick={() => remove(item)} title="从 ds2api 删除" aria-label="从 ds2api 删除">
                      <Trash2 size={18} />
                    </ActionIcon>
                  )}
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </Card>
  )
}

function ServiceConfigPanel() {
  const [config, setConfig] = useState<ServiceConfig | null>(null)
  const [draft, setDraft] = useState<ServiceConfig | null>(null)
  const [message, setMessage] = useState('')
  const load = async () => {
    const data = await api<ServiceConfig>('/api/admin/service-config')
    setConfig(data)
    setDraft({
      ...data,
      new_api_key: '',
      ds2api_admin_key: '',
      discord_client_secret: '',
      ds2api_auto_proxy: { ...data.ds2api_auto_proxy, password: '' },
    })
  }
  useEffect(() => { load().catch((err) => setMessage(err.message)) }, [])
  const update = (patch: Partial<ServiceConfig>) => {
    if (draft) setDraft({ ...draft, ...patch })
  }
  const save = async () => {
    if (!draft) return
    const saved = await api<ServiceConfig>('/api/admin/service-config', {
      method: 'PUT',
      body: JSON.stringify(draft),
    })
    setConfig(saved)
    setDraft({
      ...saved,
      new_api_key: '',
      ds2api_admin_key: '',
      discord_client_secret: '',
      ds2api_auto_proxy: { ...saved.ds2api_auto_proxy, password: '' },
    })
    setMessage('已保存')
  }
  const updateAutoProxy = (patch: Partial<ServiceConfig['ds2api_auto_proxy']>) => {
    if (draft) update({ ds2api_auto_proxy: { ...draft.ds2api_auto_proxy, ...patch } })
  }
  if (!draft) return null

  return (
    <Card>
      <SectionTitle icon={Shield} title="服务配置" />
      <SimpleGrid cols={{ base: 1, md: 2 }} spacing="sm">
        <TextInput label="new-api 地址" value={draft.new_api_base_url} onChange={(e) => update({ new_api_base_url: e.currentTarget.value })} />
        <TextInput label="new-api 后端 Key" type="password" value={draft.new_api_key || ''} placeholder={config?.new_api_key_preview || '留空保留'} onChange={(e) => update({ new_api_key: e.currentTarget.value })} />
        <TextInput label="ds2api 地址" value={draft.ds2api_base_url} onChange={(e) => update({ ds2api_base_url: e.currentTarget.value })} />
        <TextInput label="ds2api 管理 Key" type="password" value={draft.ds2api_admin_key || ''} placeholder={config?.ds2api_admin_key_preview || '留空保留'} onChange={(e) => update({ ds2api_admin_key: e.currentTarget.value })} />
        <Switch className="configSwitch" label="ds2api 自动代理" checked={draft.ds2api_auto_proxy.enabled} onChange={(e) => updateAutoProxy({ enabled: e.currentTarget.checked })} />
        <Select label="代理类型" data={['socks5', 'socks5h']} value={draft.ds2api_auto_proxy.type} onChange={(value) => updateAutoProxy({ type: value || 'socks5' })} />
        <TextInput label="代理 Host" value={draft.ds2api_auto_proxy.host} onChange={(e) => updateAutoProxy({ host: e.currentTarget.value })} />
        <NumberInput label="代理端口" value={draft.ds2api_auto_proxy.port || 0} onChange={(value) => updateAutoProxy({ port: Number(value) })} />
        <TextInput label="代理用户名模板" value={draft.ds2api_auto_proxy.username_template} onChange={(e) => updateAutoProxy({ username_template: e.currentTarget.value })} />
        <TextInput label="代理密码" type="password" value={draft.ds2api_auto_proxy.password || ''} placeholder={config?.ds2api_auto_proxy.password_preview || '留空保留'} onChange={(e) => updateAutoProxy({ password: e.currentTarget.value })} />
        <TextInput label="代理名称模板" value={draft.ds2api_auto_proxy.name_template} onChange={(e) => updateAutoProxy({ name_template: e.currentTarget.value })} />
        <TextInput label="APP_BASE_URL" value={draft.app_base_url} onChange={(e) => update({ app_base_url: e.currentTarget.value })} />
        <TextInput label="Discord Client ID" value={draft.discord_client_id} onChange={(e) => update({ discord_client_id: e.currentTarget.value })} />
        <TextInput label="Discord Client Secret" type="password" value={draft.discord_client_secret || ''} placeholder={config?.discord_secret_preview || '留空保留'} onChange={(e) => update({ discord_client_secret: e.currentTarget.value })} />
        <TextInput label="Discord 回调地址" value={draft.discord_redirect_url} onChange={(e) => update({ discord_redirect_url: e.currentTarget.value })} />
      </SimpleGrid>
      <Group mt="md">
        <Button leftSection={<Shield size={18} />} onClick={save}>保存配置</Button>
        {message && <Text c="green.7" size="sm">{message}</Text>}
      </Group>
    </Card>
  )
}

function AdminPanel() {
  const [stats, setStats] = useState<AdminStats | null>(null)
  const [users, setUsers] = useState<User[]>([])
  const [q, setQ] = useState('')
  const load = async () => {
    const [s, u] = await Promise.all([
      api<AdminStats>('/api/admin/stats'),
      api<{ items: User[] }>(`/api/admin/users?q=${encodeURIComponent(q)}`),
    ])
    setStats(s)
    setUsers(u.items)
  }
  useEffect(() => { load().catch(() => undefined) }, [])
  const update = async (user: User) => {
    await api(`/api/admin/users/${user.id}/limits`, {
      method: 'PUT',
      body: JSON.stringify({
        requests_per_minute: Number(user.requests_per_minute),
        requests_per_day: Number(user.requests_per_day),
        max_concurrent_requests: Number(user.max_concurrent_requests),
      }),
    })
    load()
  }
  return (
    <Card>
      <SectionTitle icon={Users} title="管理员" />
      {stats && (
        <SimpleGrid cols={{ base: 1, xs: 2, md: 4 }} spacing="md" mb="md">
          <MetricCard label="用户" value={formatNumber(stats.users)} icon={Users} />
          <MetricCard label="有效贡献" value={formatNumber(stats.valid_uploads)} icon={CheckCircle2} />
          <MetricCard label="调用" value={formatNumber(stats.total_requests)} icon={Activity} />
          <MetricCard label="Key" value={formatNumber(stats.active_api_keys)} icon={KeyRound} />
        </SimpleGrid>
      )}
      <Group gap="sm" align="flex-end" mb="md">
        <TextInput value={q} onChange={(e) => setQ(e.currentTarget.value)} placeholder="搜索 Discord ID / 用户名" />
        <Button leftSection={<Search size={18} />} onClick={load}>搜索</Button>
      </Group>
      <Table className="dataTable adminUsersTable">
        <Table.Thead>
          <Table.Tr>
            <Table.Th>用户</Table.Th>
            <Table.Th>贡献</Table.Th>
            <Table.Th>调用</Table.Th>
            <Table.Th>分钟</Table.Th>
            <Table.Th>每日</Table.Th>
            <Table.Th>并发</Table.Th>
            <Table.Th>操作</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {users.map((user) => <EditableUser key={user.id} user={user} onSave={update} />)}
        </Table.Tbody>
      </Table>
    </Card>
  )
}

function EditableUser({ user, onSave }: { user: User; onSave: (user: User) => void }) {
  const [draft, setDraft] = useState(user)
  useEffect(() => setDraft(user), [user])
  return (
    <Table.Tr>
      <Table.Td><Box className="userIdentity"><Text fw={500} size="sm" truncate>{displayName(user)}</Text><Text size="xs" c="var(--color-text-muted)">{user.discord_id}</Text></Box></Table.Td>
      <Table.Td><Text size="sm">{formatNumber(user.valid_uploads)}</Text></Table.Td>
      <Table.Td><Text size="sm">{formatNumber(user.total_requests)}</Text></Table.Td>
      <Table.Td><NumberInput className="compactNumber" value={draft.requests_per_minute} onChange={(value) => setDraft({ ...draft, requests_per_minute: Number(value) })} /></Table.Td>
      <Table.Td><NumberInput className="compactNumber" value={draft.requests_per_day} onChange={(value) => setDraft({ ...draft, requests_per_day: Number(value) })} /></Table.Td>
      <Table.Td><NumberInput className="compactNumber" value={draft.max_concurrent_requests} onChange={(value) => setDraft({ ...draft, max_concurrent_requests: Number(value) })} /></Table.Td>
      <Table.Td><Button size="xs" variant="light" onClick={() => onSave(draft)}>保存</Button></Table.Td>
    </Table.Tr>
  )
}
