import React, { useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import {
  Activity,
  CheckCircle2,
  Copy,
  Gauge,
  KeyRound,
  LogIn,
  LogOut,
  RefreshCw,
  Search,
  Shield,
  Trash2,
  Upload,
  Users,
} from 'lucide-react'
import './styles.css'

type User = {
  id: number
  discord_id: string
  discord_username: string
  discord_global_name: string
  discord_avatar: string
  role: string
  valid_uploads: number
  total_requests: number
  requests_per_minute: number
  requests_per_day: number
  max_concurrent_requests: number
}

type APIKey = {
  id: number
  name: string
  prefix: string
  masked_key: string
  total_requests: number
  requests_today: number
  created_at: string
  last_used_at?: string
  revoked_at?: string
  key?: string
}

type Contribution = {
  id: number
  account: string
  status: string
  message: string
  created_at: string
  response_time_ms?: number
}

type MeResponse = {
  user: User
  stats: {
    valid_uploads: number
    total_requests: number
    requests_today: number
    requests_remaining: number
  }
  keys: APIKey[]
  proxy_base_url: string
}

type PublicConfig = {
  discord_enabled: boolean
  ds2api_enabled: boolean
  new_api_enabled: boolean
  base_url: string
}

type AdminStats = {
  users: number
  valid_uploads: number
  total_requests: number
  active_api_keys: number
}

type ServiceConfig = {
  new_api_base_url: string
  new_api_key?: string
  new_api_key_preview: string
  ds2api_base_url: string
  ds2api_admin_key?: string
  ds2api_admin_key_preview: string
  ds2api_auto_proxy: {
    enabled: boolean
    type: string
    host: string
    port: number
    username_template: string
    password?: string
    password_preview: string
    name_template: string
  }
  discord_client_id: string
  discord_client_secret?: string
  discord_secret_preview: string
  discord_redirect_url: string
  app_base_url: string
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
    ...init,
  })
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new Error(data?.error?.message || data?.message || `HTTP ${res.status}`)
  }
  return data as T
}

function App() {
  const [config, setConfig] = useState<PublicConfig | null>(null)
  const [me, setMe] = useState<MeResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState('')
  const [contributionsVersion, setContributionsVersion] = useState(0)
  const isAdminPath = window.location.pathname === '/admin'

  const refresh = async () => {
    setLoading(true)
    try {
      const [cfg, mine] = await Promise.all([
        api<PublicConfig>('/api/config'),
        api<MeResponse>('/api/me').catch(() => null),
      ])
      setConfig(cfg)
      setMe(mine)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch((err) => setMessage(err.message))
  }, [])

  const logout = async () => {
    await api('/api/auth/logout', { method: 'POST', body: '{}' })
    setMe(null)
  }
  const refreshAfterImport = () => {
    refresh()
    setContributionsVersion((version) => version + 1)
  }

  if (loading) return <div className="center"><RefreshCw className="spin" />加载中</div>
  if (!me) {
    return isAdminPath
      ? <AdminLogin onLogin={refresh} message={message} />
      : <PublicLogin config={config} message={message} />
  }
  if (isAdminPath && me.user.role !== 'admin') {
    return <Forbidden onLogout={logout} />
  }

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">
          <Shield />
          <div>
            <strong>DShare</strong>
            <span>Discord gated relay</span>
          </div>
        </div>
        <div className="profile">
          <div className="avatar">{displayName(me.user).slice(0, 1).toUpperCase()}</div>
          <div>
            <strong>{displayName(me.user)}</strong>
            <span>{me.user.role === 'admin' ? '管理员' : '用户'}</span>
          </div>
        </div>
        <button className="ghost" onClick={logout}><LogOut />退出</button>
      </aside>
      <main className="main">
        <section className="topbar">
          <div>
            <h1>{isAdminPath ? '管理员' : '账号'}</h1>
            <p>代理地址：<code>{me.proxy_base_url || window.location.origin}</code></p>
          </div>
          <div className="status">
            <span className={config?.new_api_enabled ? 'ok' : 'bad'}>new-api</span>
            <span className={config?.ds2api_enabled ? 'ok' : 'bad'}>ds2api</span>
          </div>
        </section>
        <Stats me={me} />
        <KeyManager keys={me.keys} onChanged={refresh} />
        <ImportPanel onDone={refreshAfterImport} />
        <ContributionList version={contributionsVersion} onChanged={refresh} />
        {isAdminPath && <ServiceConfigPanel />}
        {isAdminPath && <AdminPanel />}
      </main>
    </div>
  )
}

function PublicLogin({ config, message }: { config: PublicConfig | null; message: string }) {
  return (
    <div className="login publicOnly">
      <div className="loginPanel">
        <Shield className="logo" />
        <h1>DShare</h1>
        <p>使用 Discord 登录后获取项目 API Key，或提交 DeepSeek 账号贡献到账号池。</p>
        {config?.discord_enabled ? (
          <a className="primary wide" href="/api/auth/discord/start"><LogIn />Discord 登录</a>
        ) : (
          <div className="error">Discord 登录尚未配置，请联系管理员。</div>
        )}
        {message && <div className="error">{message}</div>}
      </div>
    </div>
  )
}

function AdminLogin({ onLogin, message }: { onLogin: () => void; message: string }) {
  const [username, setUsername] = useState('inliver')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState(message)
  const adminLogin = async () => {
    setErr('')
    try {
      await api('/api/auth/admin/login', { method: 'POST', body: JSON.stringify({ username, password }) })
      onLogin()
    } catch (e) {
      setErr((e as Error).message)
    }
  }
  return (
    <div className="login">
      <div className="loginPanel">
        <Shield className="logo" />
        <h1>管理员登录</h1>
        <p>管理员入口只用于查看用户贡献、调用量，并修改每个用户的请求限制。</p>
        <div className="adminLogin">
          <input placeholder="管理员账号" value={username} onChange={(e) => setUsername(e.target.value)} />
          <input type="password" placeholder="管理员密码" value={password} onChange={(e) => setPassword(e.target.value)} />
          <button onClick={adminLogin}><Shield />管理员登录</button>
        </div>
        {err && <div className="error">{err}</div>}
      </div>
    </div>
  )
}

function Forbidden({ onLogout }: { onLogout: () => void }) {
  return (
    <div className="login">
      <div className="loginPanel">
        <Shield className="logo" />
        <h1>无管理员权限</h1>
        <p>当前登录账号不是管理员。请退出后使用管理员账号登录。</p>
        <button onClick={onLogout}><LogOut />退出</button>
      </div>
    </div>
  )
}

function Stats({ me }: { me: MeResponse }) {
  const dailyLimit = me.user.requests_per_day
  const dailyUsage = dailyLimit > 0 ? `${me.stats.requests_today}/${dailyLimit}` : `${me.stats.requests_today}/不限`
  const remaining = dailyLimit > 0 ? me.stats.requests_remaining : '不限'
  const cards = [
    ['有效上传', me.stats.valid_uploads, CheckCircle2],
    ['总调用量', me.stats.total_requests, Activity],
    ['今日已用', dailyUsage, Gauge],
    ['剩余额度', remaining, Gauge],
    ['每分钟限制', me.user.requests_per_minute || '不限', Gauge],
  ] as const
  return <div className="grid stats">{cards.map(([label, value, Icon]) => <div className="metric" key={label}><Icon /><span>{label}</span><strong>{value}</strong></div>)}</div>
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
    <section>
      <div className="sectionTitle"><KeyRound /><h2>项目 API Key</h2></div>
      <div className="inlineForm">
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Key 名称" />
        <button onClick={create}><KeyRound />创建</button>
      </div>
      {created?.key && <CopyBox value={created.key} />}
      <div className="table keyTable">
        {keys.map((key) => <div className="row keyRow" key={key.id}>
          <div>
            <strong>{key.name || 'unnamed'}</strong>
            <small>创建 {formatDate(key.created_at)}</small>
          </div>
          <code>{key.masked_key}</code>
          <span>今日 {key.requests_today}</span>
          <span>总计 {key.total_requests}</span>
          <small>{key.last_used_at ? `最后使用 ${formatDate(key.last_used_at)}` : '尚未使用'}</small>
          {key.key ? <CopyKeyButton value={key.key} /> : <small>旧 Key 不可复制</small>}
          <button className="icon dangerIcon" disabled={deleting === key.id} onClick={() => revoke(key.id)} title="删除"><Trash2 /></button>
        </div>)}
        {keys.length === 0 && <div className="empty">还没有 API Key</div>}
      </div>
    </section>
  )
}

function ImportPanel({ onDone }: { onDone: () => void }) {
  const [lines, setLines] = useState('')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<any>(null)
  const submit = async () => {
    setLoading(true)
    try {
      const data = await api('/api/ds2api/import', { method: 'POST', body: JSON.stringify({ lines }) })
      setResult(data)
      onDone()
    } finally {
      setLoading(false)
    }
  }
  return (
    <section>
      <div className="sectionTitle"><Upload /><h2>DeepSeek 账号贡献</h2></div>
      <textarea value={lines} onChange={(e) => setLines(e.target.value)} placeholder={'账号:密码\nuser@example.com:password'} />
      <button onClick={submit} disabled={loading || !lines.trim()}><Upload />{loading ? '验证中' : '导入并验证'}</button>
      {result && <div className="result">
        <strong>总数 {result.total}，有效 {result.valid}，无效 {result.invalid}，重复 {result.duplicate}</strong>
        {result.results?.slice(0, 20).map((r: any) => <div className={`mini ${r.status}`} key={r.account}>{r.account} · {r.status} · {r.message}</div>)}
      </div>}
    </section>
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
    <section>
      <div className="sectionTitle"><CheckCircle2 /><h2>最近贡献</h2></div>
      <div className="table compact">
        {items.map((item) => <div className="row" key={item.id}>
          <span>{item.account}</span>
          <span className={`pill ${item.status}`}>{item.status}</span>
          <small>{item.message}</small>
          <small>{formatDate(item.created_at)}</small>
          {item.status === 'valid'
            ? <button className="icon dangerIcon" disabled={deleting === item.id} onClick={() => remove(item)} title="从 ds2api 删除"><Trash2 /></button>
            : <span />}
        </div>)}
        {items.length === 0 && <div className="empty">还没有贡献记录</div>}
      </div>
    </section>
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
    <section>
      <div className="sectionTitle"><Shield /><h2>服务配置</h2></div>
      <div className="settingsGrid">
        <label>new-api 地址<input value={draft.new_api_base_url} onChange={(e) => update({ new_api_base_url: e.target.value })} /></label>
        <label>new-api 后端 Key<input type="password" value={draft.new_api_key || ''} placeholder={config?.new_api_key_preview || '留空保留'} onChange={(e) => update({ new_api_key: e.target.value })} /></label>
        <label>ds2api 地址<input value={draft.ds2api_base_url} onChange={(e) => update({ ds2api_base_url: e.target.value })} /></label>
        <label>ds2api 管理 Key<input type="password" value={draft.ds2api_admin_key || ''} placeholder={config?.ds2api_admin_key_preview || '留空保留'} onChange={(e) => update({ ds2api_admin_key: e.target.value })} /></label>
        <label className="checkLabel">ds2api 自动代理<input type="checkbox" checked={draft.ds2api_auto_proxy.enabled} onChange={(e) => updateAutoProxy({ enabled: e.target.checked })} /></label>
        <label>代理类型<select value={draft.ds2api_auto_proxy.type} onChange={(e) => updateAutoProxy({ type: e.target.value })}><option value="socks5">socks5</option><option value="socks5h">socks5h</option></select></label>
        <label>代理 Host<input value={draft.ds2api_auto_proxy.host} onChange={(e) => updateAutoProxy({ host: e.target.value })} /></label>
        <label>代理端口<input type="number" value={draft.ds2api_auto_proxy.port || 0} onChange={(e) => updateAutoProxy({ port: Number(e.target.value) })} /></label>
        <label>代理用户名模板<input value={draft.ds2api_auto_proxy.username_template} onChange={(e) => updateAutoProxy({ username_template: e.target.value })} /></label>
        <label>代理密码<input type="password" value={draft.ds2api_auto_proxy.password || ''} placeholder={config?.ds2api_auto_proxy.password_preview || '留空保留'} onChange={(e) => updateAutoProxy({ password: e.target.value })} /></label>
        <label>代理名称模板<input value={draft.ds2api_auto_proxy.name_template} onChange={(e) => updateAutoProxy({ name_template: e.target.value })} /></label>
        <label>APP_BASE_URL<input value={draft.app_base_url} onChange={(e) => update({ app_base_url: e.target.value })} /></label>
        <label>Discord Client ID<input value={draft.discord_client_id} onChange={(e) => update({ discord_client_id: e.target.value })} /></label>
        <label>Discord Client Secret<input type="password" value={draft.discord_client_secret || ''} placeholder={config?.discord_secret_preview || '留空保留'} onChange={(e) => update({ discord_client_secret: e.target.value })} /></label>
        <label>Discord 回调地址<input value={draft.discord_redirect_url} onChange={(e) => update({ discord_redirect_url: e.target.value })} /></label>
      </div>
      <div className="inlineForm">
        <button onClick={save}><Shield />保存配置</button>
        {message && <span className="saveMessage">{message}</span>}
      </div>
    </section>
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
    <section>
      <div className="sectionTitle"><Users /><h2>管理员</h2></div>
      {stats && <div className="grid stats">
        <div className="metric"><Users /><span>用户</span><strong>{stats.users}</strong></div>
        <div className="metric"><CheckCircle2 /><span>有效贡献</span><strong>{stats.valid_uploads}</strong></div>
        <div className="metric"><Activity /><span>调用</span><strong>{stats.total_requests}</strong></div>
        <div className="metric"><KeyRound /><span>Key</span><strong>{stats.active_api_keys}</strong></div>
      </div>}
      <div className="inlineForm">
        <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="搜索 Discord ID / 用户名" />
        <button onClick={load}><Search />搜索</button>
      </div>
      <div className="table users">
        {users.map((user) => <EditableUser key={user.id} user={user} onSave={update} />)}
      </div>
    </section>
  )
}

function EditableUser({ user, onSave }: { user: User; onSave: (user: User) => void }) {
  const [draft, setDraft] = useState(user)
  useEffect(() => setDraft(user), [user])
  return (
    <div className="row userRow">
      <div><strong>{displayName(user)}</strong><small>{user.discord_id}</small></div>
      <span>贡献 {user.valid_uploads}</span>
      <span>调用 {user.total_requests}</span>
      <label>分钟<input type="number" value={draft.requests_per_minute} onChange={(e) => setDraft({ ...draft, requests_per_minute: Number(e.target.value) })} /></label>
      <label>每日<input type="number" value={draft.requests_per_day} onChange={(e) => setDraft({ ...draft, requests_per_day: Number(e.target.value) })} /></label>
      <label>并发<input type="number" value={draft.max_concurrent_requests} onChange={(e) => setDraft({ ...draft, max_concurrent_requests: Number(e.target.value) })} /></label>
      <button onClick={() => onSave(draft)}>保存</button>
    </div>
  )
}

function CopyBox({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  const display = useMemo(() => value, [value])
  const copy = async () => {
    await copyText(value)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1800)
  }
  return <div className="copyBox"><code>{display}</code><button onClick={copy}><Copy />{copied ? '已复制' : '复制'}</button></div>
}

function CopyKeyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    await copyText(value)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1800)
  }
  return <button className="icon" onClick={copy} title="复制完整 Key"><Copy />{copied ? '已复制' : ''}</button>
}

async function copyText(value: string) {
  if (navigator.clipboard?.writeText && window.isSecureContext) {
    await navigator.clipboard.writeText(value)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  textarea.select()
  try {
    document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
}

function displayName(user: User) {
  return user.discord_global_name || user.discord_username || user.discord_id
}

function formatDate(value?: string) {
  if (!value) return ''
  return new Date(value).toLocaleString()
}

createRoot(document.getElementById('root')!).render(<App />)
