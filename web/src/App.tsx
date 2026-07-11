import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, setUnauthorizedHandler } from '@/lib/api'
import type { AuthStatus } from '@/lib/types'
import { Layout } from '@/components/Layout'
import { Wordmark } from '@/components/Wordmark'
import { Spinner } from '@/components/common'
import { SetupScreen, LoginScreen } from '@/pages/AuthScreens'
import { DashboardPage } from '@/pages/Dashboard'
import { EventsPage } from '@/pages/Events'
import { RulesPage } from '@/pages/Rules'
import { ChannelsPage } from '@/pages/Channels'
import { OIDMapPage } from '@/pages/OIDMap'
import { SeveritiesPage } from '@/pages/Severities'
import { SinksPage } from '@/pages/Sinks'
import { SettingsPage } from '@/pages/Settings'

interface Health {
  status: string
  version: string
}

export function App() {
  const qc = useQueryClient()

  const { data: status, isLoading } = useQuery({
    queryKey: ['auth-status'],
    queryFn: () => api.get<AuthStatus>('/auth/status'),
    retry: false,
  })

  const { data: health } = useQuery({
    queryKey: ['health'],
    queryFn: () => fetch('/health', { credentials: 'include' }).then((r) => r.json() as Promise<Health>),
    retry: false,
    staleTime: Infinity,
  })

  // A 401 on any protected call means the session lapsed — re-probe auth so the
  // gate below swaps in the login screen.
  useEffect(() => {
    setUnauthorizedHandler(() => {
      qc.setQueryData<AuthStatus>(['auth-status'], (prev) =>
        prev ? { ...prev, authenticated: false } : prev,
      )
    })
  }, [qc])

  const refreshAuth = () => qc.invalidateQueries({ queryKey: ['auth-status'] })

  const logout = async () => {
    try {
      await api.post('/auth/logout')
    } finally {
      qc.clear()
      refreshAuth()
    }
  }

  if (isLoading || !status) return <Splash />

  if (!status.configured) return <SetupScreen onDone={refreshAuth} />
  if (status.auth_enabled && !status.authenticated) return <LoginScreen onDone={refreshAuth} />

  return (
    <BrowserRouter>
      <Layout version={health?.version} showAuthControls={status.auth_enabled} onLogout={logout}>
        <Routes>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/events" element={<EventsPage />} />
          <Route path="/rules" element={<RulesPage />} />
          <Route path="/channels" element={<ChannelsPage />} />
          <Route path="/oidmap" element={<OIDMapPage />} />
          <Route path="/severities" element={<SeveritiesPage />} />
          <Route path="/sinks" element={<SinksPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  )
}

function Splash() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-background">
      <Wordmark />
      <Spinner />
    </div>
  )
}
