import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import type {
  Channel,
  Community,
  Dashboard,
  Device,
  Notification,
  OIDEntry,
  Rule,
  Severity,
  Settings,
  TrapView,
} from './types'

const LIVE_REFRESH = 60_000 // design: live data refreshes ~60s

// ---- Queries ----

export function useDashboard() {
  return useQuery({
    queryKey: ['dashboard'],
    queryFn: () => api.get<Dashboard>('/dashboard'),
    refetchInterval: LIVE_REFRESH,
  })
}

export function useTraps(sort: string, order: 'asc' | 'desc', limit: number) {
  return useQuery({
    queryKey: ['traps', sort, order, limit],
    queryFn: () =>
      api.get<TrapView[] | null>(`/traps?sort=${sort}&order=${order}&limit=${limit}`).then((v) => v ?? []),
    refetchInterval: LIVE_REFRESH,
    placeholderData: (prev) => prev,
  })
}

export function useTrapNotifications(trapId: number | null) {
  return useQuery({
    queryKey: ['trap-notifications', trapId],
    queryFn: () => api.get<Notification[] | null>(`/traps/${trapId}/notifications`).then((v) => v ?? []),
    enabled: trapId != null,
  })
}

export function useSeverities() {
  return useQuery({
    queryKey: ['severities'],
    queryFn: () => api.get<Severity[] | null>('/severities').then((v) => v ?? []),
  })
}

export function useChannels() {
  return useQuery({
    queryKey: ['channels'],
    queryFn: () => api.get<Channel[] | null>('/channels').then((v) => v ?? []),
  })
}

export function useRules() {
  return useQuery({
    queryKey: ['rules'],
    queryFn: () => api.get<Rule[] | null>('/rules').then((v) => v ?? []),
  })
}

export function useDevices() {
  return useQuery({
    queryKey: ['devices'],
    queryFn: () => api.get<Device[] | null>('/devices').then((v) => v ?? []),
  })
}

export function useOIDMap() {
  return useQuery({
    queryKey: ['oidmap'],
    queryFn: () => api.get<OIDEntry[] | null>('/oidmap').then((v) => v ?? []),
  })
}

export function useCommunities() {
  return useQuery({
    queryKey: ['communities'],
    queryFn: () => api.get<Community[] | null>('/sinks/communities').then((v) => v ?? []),
  })
}

export function useSettings() {
  return useQuery({
    queryKey: ['settings'],
    queryFn: () => api.get<Settings>('/settings'),
  })
}

export function useRoutes() {
  return useQuery({
    queryKey: ['routes'],
    queryFn: () => api.get<Record<string, number[]>>('/routes'),
  })
}

// ---- Mutation helper ----

/** Invalidate one or more query keys after a successful mutation. */
export function useInvalidate() {
  const qc = useQueryClient()
  return (...keys: string[]) => keys.forEach((k) => qc.invalidateQueries({ queryKey: [k] }))
}

export { useMutation }
