import { useEffect, useState } from 'react'
import type { MeResponse, PublicConfig } from './types/api'
import { api } from './services/api'
import { AppShell } from './layout/AppShell'
import { LoadingScreen } from './components/Feedback'
import { AdminLogin, Forbidden, PublicLogin } from './pages/LoginPages'
import { AdminSections, DashboardPage } from './pages/DashboardPage'
import { RankPage } from './pages/RankPage'

export function App() {
  const [config, setConfig] = useState<PublicConfig | null>(null)
  const [me, setMe] = useState<MeResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState('')
  const path = window.location.pathname
  const isAdminPath = path === '/admin'
  const isRankPath = path === '/rank'

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

  if (isRankPath) return <RankPage />
  if (loading) return <LoadingScreen />
  if (!me) {
    return isAdminPath
      ? <AdminLogin onLogin={refresh} message={message} />
      : <PublicLogin config={config} message={message} />
  }
  if (isAdminPath && me.user.role !== 'admin') {
    return <Forbidden onLogout={logout} />
  }

  return (
    <AppShell user={me.user} config={config} isAdminPath={isAdminPath} proxyBaseURL={me.proxy_base_url} onLogout={logout}>
      {isAdminPath ? <AdminSections /> : <DashboardPage me={me} onChanged={refresh} />}
    </AppShell>
  )
}
