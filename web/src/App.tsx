import { lazy, Suspense, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import type { MeResponse, PublicConfig } from './types/api'
import { api } from './services/api'
import { LoadingScreen } from './components/Feedback'

const AppShell = lazy(() => import('./layout/AppShell').then((module) => ({ default: module.AppShell })))
const PublicLogin = lazy(() => import('./pages/LoginPages').then((module) => ({ default: module.PublicLogin })))
const AdminLogin = lazy(() => import('./pages/LoginPages').then((module) => ({ default: module.AdminLogin })))
const Forbidden = lazy(() => import('./pages/LoginPages').then((module) => ({ default: module.Forbidden })))
const DashboardPage = lazy(() => import('./pages/DashboardPage').then((module) => ({ default: module.DashboardPage })))
const AdminSections = lazy(() => import('./pages/DashboardPage').then((module) => ({ default: module.AdminSections })))
const RankPage = lazy(() => import('./pages/RankPage').then((module) => ({ default: module.RankPage })))

function LazyBoundary({ children }: { children: ReactNode }) {
  return <Suspense fallback={<LoadingScreen />}>{children}</Suspense>
}

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

  if (isRankPath) {
    return (
      <LazyBoundary>
        <RankPage />
      </LazyBoundary>
    )
  }
  if (loading) return <LoadingScreen />
  if (!me) {
    return (
      <LazyBoundary>
        {isAdminPath
          ? <AdminLogin onLogin={refresh} message={message} />
          : <PublicLogin config={config} message={message} />}
      </LazyBoundary>
    )
  }
  if (isAdminPath && me.user.role !== 'admin') {
    return (
      <LazyBoundary>
        <Forbidden onLogout={logout} />
      </LazyBoundary>
    )
  }

  return (
    <LazyBoundary>
      <AppShell user={me.user} config={config} isAdminPath={isAdminPath} proxyBaseURL={me.proxy_base_url} onLogout={logout}>
        {isAdminPath ? <AdminSections /> : <DashboardPage me={me} onChanged={refresh} />}
      </AppShell>
    </LazyBoundary>
  )
}
