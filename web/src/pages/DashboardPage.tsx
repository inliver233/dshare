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
import { motion, Variants } from 'framer-motion'
import CountUp from 'react-countup'
import type { APIKey, AdminStats, Contribution, ImportResponse, MeResponse, ServiceConfig, User } from '../types/api'
import { api } from '../services/api'
import { CopyBox, CopyButton } from '../components/CopyButton'
import { EmptyState, ErrorText } from '../components/Feedback'
import { copyText } from '../utils/clipboard'
import { displayName, formatDate, formatNumber } from '../utils/format'

const containerVariants: Variants = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: { staggerChildren: 0.1 }
  }
}

const itemVariants: Variants = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0, transition: { type: 'spring', stiffness: 300, damping: 24 } }
}

export function DashboardPage({ me, onChanged }: { me: MeResponse; onChanged: () => void }) {
  const [contributionsVersion, setContributionsVersion] = useState(0)
  const refreshAfterImport = () => {
    onChanged()
    setContributionsVersion((version) => version + 1)
  }
  return (
    <motion.div variants={containerVariants} initial="hidden" animate="show">
      <Stack gap="xl">
        <motion.div variants={itemVariants}><Stats me={me} /></motion.div>
        <motion.div variants={itemVariants}><KeyManager keys={me.keys} onChanged={onChanged} /></motion.div>
        <motion.div variants={itemVariants}><ImportPanel onDone={refreshAfterImport} /></motion.div>
        <motion.div variants={itemVariants}><ContributionList version={contributionsVersion} onChanged={onChanged} /></motion.div>
      </Stack>
    </motion.div>
  )
}

export function AdminSections() {
  return (
    <motion.div variants={containerVariants} initial="hidden" animate="show">
      <Stack gap="xl">
        <motion.div variants={itemVariants}><ServiceConfigPanel /></motion.div>
        <motion.div variants={itemVariants}><AdminPanel /></motion.div>
      </Stack>
    </motion.div>
  )
}

function SectionTitle({ icon: Icon, title }: { icon: typeof KeyRound; title: string }) {
  return (
    <Group gap="xs" mb="xl">
      <ThemeIcon variant="light" color="brandBlue" radius="xl" size={36}><Icon size={20} /></ThemeIcon>
      <Title order={2}>{title}</Title>
    </Group>
  )
}

type MetricCardProps = {
  label: string
  value: number
  prefix?: string
  suffix?: string
  suffixValue?: string
  icon: typeof KeyRound
}

function MetricCard({ label, value, prefix = '', suffix = '', suffixValue = '', icon: Icon }: MetricCardProps) {
  return (
    <div className="metricCard">
      <Icon className="watermarkIcon" fill="currentColor" strokeWidth={0} />
      <Text className="metricLabel">{label}</Text>
      <div className="metricValueGroup">
        <Text className="metricValue">
          {prefix}
          <CountUp end={value} duration={1.5} separator="," />
          {suffix}
        </Text>
        {suffixValue && <Text className="metricSuffix">{suffixValue}</Text>}
      </div>
    </div>
  )
}

function Stats({ me }: { me: MeResponse }) {
  const dailyLimit = me.user.requests_per_day
  
  return (
    <Box>
      <div className="statsGrid">
        <MetricCard label="总调用量" value={me.stats.total_requests} icon={Activity} />
        <MetricCard label="有效上传" value={me.stats.valid_uploads} icon={CheckCircle2} />
      </div>
      <div className="statsGrid-secondary">
        <MetricCard 
          label="今日已用" 
          value={me.stats.requests_today} 
          suffixValue={dailyLimit > 0 ? `/ ${formatNumber(dailyLimit)}` : '/ 不限'} 
          icon={Gauge} 
        />
        <MetricCard 
          label="剩余额度" 
          value={dailyLimit > 0 ? me.stats.requests_remaining : 0} 
          prefix={dailyLimit > 0 ? '' : '∞ '}
          icon={Shield} 
        />
        <MetricCard 
          label="每分钟限制" 
          value={me.user.requests_per_minute || 0} 
          prefix={me.user.requests_per_minute ? '' : '∞ '}
          icon={Activity} 
        />
      </div>
    </Box>
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
    <Box>
      <SectionTitle icon={KeyRound} title="项目 API Key" />
      <Group gap="sm" align="flex-end" mb="xl">
        <TextInput className="keyNameInput" value={name} onChange={(e) => setName(e.currentTarget.value)} placeholder="Key 名称" />
        <Button leftSection={<KeyRound size={18} />} onClick={create}>创建 API Key</Button>
      </Group>
      {created?.key && <CopyBox value={created.key} />}
      
      {keys.length === 0 ? <EmptyState>还没有 API Key</EmptyState> : (
        <div className="keyFlowList">
          {keys.map((key) => (
            <div className="keyFlowCard" key={key.id}>
              <div className="keyFlowHeader">
                <Group gap="sm">
                  <Text className="keyNameTag">{key.name || 'unnamed'}</Text>
                  <Text size="xs" c="var(--color-text-subtle)">创建于 {formatDate(key.created_at)}</Text>
                </Group>
                <Group gap="xs">
                  {key.key && <CopyButton value={key.key} className="copyAction" />}
                  <ActionIcon
                    className="deleteAction"
                    disabled={deleting === key.id}
                    onClick={() => revoke(key.id)}
                    title="删除"
                  >
                    <Trash2 size={16} />
                  </ActionIcon>
                </Group>
              </div>
              <Group gap="xl" wrap="wrap">
                <Code className="machineCode">{key.masked_key}</Code>
                <Group gap="md">
                  <Text size="sm" c="var(--color-text-body)">今日调用 <Text component="span" fw={700} c="var(--ocean-80)">{formatNumber(key.requests_today)}</Text></Text>
                  <Text size="sm" c="var(--color-text-body)">总调用 <Text component="span" fw={700} c="var(--ocean-80)">{formatNumber(key.total_requests)}</Text></Text>
                  <Text size="sm" c="var(--color-text-subtle)">
                    {key.last_used_at ? `最后使用 ${formatDate(key.last_used_at)}` : '尚未使用'}
                  </Text>
                </Group>
              </Group>
            </div>
          ))}
        </div>
      )}
    </Box>
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
    <Box mt="xl">
      <SectionTitle icon={Upload} title="DeepSeek 账号贡献" />
      <Box className="importCanvas">
        <Textarea
          autosize
          minRows={6}
          value={lines}
          onChange={(e) => setLines(e.currentTarget.value)}
          placeholder={'在此粘贴 DeepSeek 账号和密码，每行一个\n格式：账号:密码'}
          className="monoInput importTextarea"
        />
        <Button className="importButton" variant="filled" color="brandBlue" leftSection={<Upload size={18} />} onClick={submit} disabled={loading || !lines.trim()}>
          {loading ? '验证中...' : '导入并自动验证'}
        </Button>
      </Box>
      <ErrorText>{error}</ErrorText>
      {result && (
        <Stack gap={8} mt="xl">
          <Text fw={700} size="sm" c="var(--color-text-title)">导入结果摘要：总计 {result.total} / 有效 {result.valid} / 无效 {result.invalid} / 重复 {result.duplicate}</Text>
          <Group gap="sm" wrap="wrap">
            {result.results?.slice(0, 30).map((r) => (
              <Text className={`mini ${r.status}`} key={r.account}>{r.account} - {r.message}</Text>
            ))}
          </Group>
        </Stack>
      )}
    </Box>
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
    <Box mt="xl">
      <SectionTitle icon={CheckCircle2} title="最近贡献记录" />
      {items.length === 0 ? <EmptyState>暂无有效贡献记录</EmptyState> : (
        <Card p={0} bg="transparent" shadow="none">
          <Table className="adminUsersTable">
            <Table.Tbody>
              {items.map((item) => (
                <Table.Tr key={item.id}>
                  <Table.Td data-label="账号"><Text size="sm" fw={600} truncate>{item.account}</Text></Table.Td>
                  <Table.Td data-label="状态"><Badge className={item.status} variant="light">{item.status}</Badge></Table.Td>
                  <Table.Td data-label="详情"><Text size="sm" c="var(--color-text-muted)">{item.message}</Text></Table.Td>
                  <Table.Td data-label="时间"><Text size="sm" c="var(--color-text-subtle)">{formatDate(item.created_at)}</Text></Table.Td>
                  <Table.Td data-label="操作" ta="right">
                    {item.status === 'valid' && (
                      <ActionIcon className="deleteAction" disabled={deleting === item.id} onClick={() => remove(item)} title="从池中移除">
                        <Trash2 size={16} />
                      </ActionIcon>
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Card>
      )}
    </Box>
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
    setMessage('配置已生效')
    setTimeout(() => setMessage(''), 3000)
  }
  const updateAutoProxy = (patch: Partial<ServiceConfig['ds2api_auto_proxy']>) => {
    if (draft) update({ ds2api_auto_proxy: { ...draft.ds2api_auto_proxy, ...patch } })
  }
  if (!draft) return null

  return (
    <Card style={{ maxWidth: 960 }}>
      <SectionTitle icon={Shield} title="中继服务核心配置" />
      <div className="configGrid">
        <TextInput label="New-API 接口地址" value={draft.new_api_base_url} onChange={(e) => update({ new_api_base_url: e.currentTarget.value })} />
        <TextInput label="New-API Root Key" type="password" value={draft.new_api_key || ''} placeholder={config?.new_api_key_preview || '留空保持原有值不变'} onChange={(e) => update({ new_api_key: e.currentTarget.value })} />
        <TextInput label="DS2API 接口地址" value={draft.ds2api_base_url} onChange={(e) => update({ ds2api_base_url: e.currentTarget.value })} />
        <TextInput label="DS2API Admin Key" type="password" value={draft.ds2api_admin_key || ''} placeholder={config?.ds2api_admin_key_preview || '留空保持原有值不变'} onChange={(e) => update({ ds2api_admin_key: e.currentTarget.value })} />
        
        <Switch className="configSwitch" size="md" label="启用 DS2API 自动代理池" checked={draft.ds2api_auto_proxy.enabled} onChange={(e) => updateAutoProxy({ enabled: e.currentTarget.checked })} />
        <Select label="代理协议类型" data={['socks5', 'socks5h']} value={draft.ds2api_auto_proxy.type} onChange={(value) => updateAutoProxy({ type: value || 'socks5' })} />
        <TextInput label="代理主机 Host" value={draft.ds2api_auto_proxy.host} onChange={(e) => updateAutoProxy({ host: e.currentTarget.value })} />
        <NumberInput label="代理端口 Port" value={draft.ds2api_auto_proxy.port || 0} onChange={(value) => updateAutoProxy({ port: Number(value) })} />
        <TextInput label="代理认证名模板" value={draft.ds2api_auto_proxy.username_template} onChange={(e) => updateAutoProxy({ username_template: e.currentTarget.value })} />
        <TextInput label="代理密码" type="password" value={draft.ds2api_auto_proxy.password || ''} placeholder={config?.ds2api_auto_proxy.password_preview || '留空保持原有值不变'} onChange={(e) => updateAutoProxy({ password: e.currentTarget.value })} />
        <TextInput label="代理名称模板" value={draft.ds2api_auto_proxy.name_template} onChange={(e) => updateAutoProxy({ name_template: e.currentTarget.value })} />
        <TextInput label="APP_BASE_URL" value={draft.app_base_url} onChange={(e) => update({ app_base_url: e.currentTarget.value })} />
        <TextInput label="Discord Client ID" value={draft.discord_client_id} onChange={(e) => update({ discord_client_id: e.currentTarget.value })} />
        <TextInput label="Discord Client Secret" type="password" value={draft.discord_client_secret || ''} placeholder={config?.discord_secret_preview || '留空保持原有值不变'} onChange={(e) => update({ discord_client_secret: e.currentTarget.value })} />
        <TextInput label="Discord 回调地址" value={draft.discord_redirect_url} onChange={(e) => update({ discord_redirect_url: e.currentTarget.value })} />
      </div>
      <Group mt="xl" pt="md" style={{ borderTop: '1px solid var(--color-border-subtle)' }}>
        <Button leftSection={<Shield size={18} />} onClick={save}>应用更改</Button>
        {message && <Text c="var(--ocean-80)" size="sm" fw={600}>{message}</Text>}
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
    <Box mt="xl">
      <SectionTitle icon={Users} title="用户权限与限制管理" />
      {stats && (
        <div className="statsGrid" style={{ marginBottom: '24px' }}>
          <MetricCard label="注册用户总数" value={stats.users} icon={Users} />
          <MetricCard label="活跃 API Key" value={stats.active_api_keys} icon={KeyRound} />
        </div>
      )}
      <Group gap="sm" align="flex-end" mb="xl">
        <TextInput size="md" style={{ width: 'min(400px, 100%)' }} value={q} onChange={(e) => setQ(e.currentTarget.value)} placeholder="搜索 Discord ID 或 用户名" />
        <Button size="md" variant="light" leftSection={<Search size={18} />} onClick={load}>搜索过滤</Button>
      </Group>
      
      <Card p={0} bg="transparent" shadow="none">
        <Table className="adminUsersTable">
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Discord 用户</Table.Th>
              <Table.Th>有效贡献 / 总调用</Table.Th>
              <Table.Th>并发</Table.Th>
              <Table.Th>分钟限额</Table.Th>
              <Table.Th>每日限额</Table.Th>
              <Table.Th ta="right">操作</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {users.map((user) => <EditableUser key={user.id} user={user} onSave={update} />)}
          </Table.Tbody>
        </Table>
      </Card>
    </Box>
  )
}

function EditableUser({ user, onSave }: { user: User; onSave: (user: User) => void }) {
  const [draft, setDraft] = useState(user)
  useEffect(() => setDraft(user), [user])
  
  const isChanged = 
    draft.requests_per_minute !== user.requests_per_minute || 
    draft.requests_per_day !== user.requests_per_day || 
    draft.max_concurrent_requests !== user.max_concurrent_requests

  return (
    <Table.Tr>
      <Table.Td data-label="用户">
        <Box className="userIdentity">
          <Text fw={700} size="sm" c="var(--color-text-title)" truncate>{displayName(user)}</Text>
          <Text size="xs" c="var(--color-text-muted)">{user.discord_id}</Text>
        </Box>
      </Table.Td>
      <Table.Td data-label="贡献/调用">
        <Group gap={4}>
          <Badge color="teal" variant="light" size="sm">{formatNumber(user.valid_uploads)}</Badge>
          <Text size="xs" c="var(--color-text-subtle)">/</Text>
          <Badge color="blue" variant="light" size="sm">{formatNumber(user.total_requests)}</Badge>
        </Group>
      </Table.Td>
      <Table.Td data-label="并发" className="stealthInput">
        <NumberInput className="compactNumber" value={draft.max_concurrent_requests} onChange={(value) => setDraft({ ...draft, max_concurrent_requests: Number(value) })} />
      </Table.Td>
      <Table.Td data-label="每分钟" className="stealthInput">
        <NumberInput className="compactNumber" value={draft.requests_per_minute} onChange={(value) => setDraft({ ...draft, requests_per_minute: Number(value) })} />
      </Table.Td>
      <Table.Td data-label="每日" className="stealthInput">
        <NumberInput className="compactNumber" value={draft.requests_per_day} onChange={(value) => setDraft({ ...draft, requests_per_day: Number(value) })} />
      </Table.Td>
      <Table.Td data-label="操作" ta="right">
        <Button 
          size="xs" 
          className={`saveActionBtn ${isChanged ? 'show' : ''}`} 
          onClick={() => onSave(draft)}
        >
          保存
        </Button>
      </Table.Td>
    </Table.Tr>
  )
}
