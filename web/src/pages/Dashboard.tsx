import { Link } from 'react-router-dom'
import { Activity, Radio, CheckCircle2, XCircle, Clock, ArrowRight } from 'lucide-react'
import { useDashboard, useSeverities } from '@/lib/queries'
import type { Severity, TrapView } from '@/lib/types'
import { PageHeader, LoadingRow, ErrorNote, EmptyState, Mono } from '@/components/common'
import { SeverityBadge, BUILTIN_SEVERITY_COLORS } from '@/components/Severity'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { relativeTime } from '@/lib/utils'

export function DashboardPage() {
  const { data, isLoading, error } = useDashboard()
  const { data: severities = [] } = useSeverities()

  if (isLoading) return <LoadingRow label="Loading console…" />
  if (error) return <ErrorNote message={(error as Error).message} />
  if (!data) return null

  const notif = data.notifications ?? {}
  const sent = notif.sent ?? 0
  const failed = notif.failed ?? 0
  const held = notif.held ?? 0

  return (
    <div>
      <PageHeader title="Dashboard" description="Live readout of trap intake and notification dispatch." />

      <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
        <StatTile icon={Radio} label="Traps received" value={data.traps_total} accent="holo" />
        <StatTile icon={CheckCircle2} label="Notifications sent" value={sent} accent="success" />
        <StatTile icon={XCircle} label="Failed" value={failed} accent="danger" />
        <StatTile icon={Clock} label="Held" value={held} accent="warn" />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-5 sm:mt-6">
        <Card className="lg:col-span-2">
          <CardHeader title="By severity" description="Trap volume across priority levels" />
          <CardBody>
            <SeverityBreakdown counts={data.traps_by_severity ?? {}} severities={severities} total={data.traps_total} />
          </CardBody>
        </Card>

        <Card className="lg:col-span-3">
          <CardHeader
            title="Recent activity"
            action={
              <Link
                to="/events"
                className="inline-flex items-center gap-1 text-sm font-medium text-holo hover:underline"
              >
                All events <ArrowRight className="h-3.5 w-3.5" />
              </Link>
            }
          />
          <CardBody className="p-0">
            <RecentActivity traps={data.recent ?? []} />
          </CardBody>
        </Card>
      </div>
    </div>
  )
}

const ACCENTS = {
  holo: 'text-holo',
  success: 'text-emerald-400',
  danger: 'text-red-400',
  warn: 'text-amber-400',
} as const

function StatTile({
  icon: Icon,
  label,
  value,
  accent,
}: {
  icon: typeof Radio
  label: string
  value: number
  accent: keyof typeof ACCENTS
}) {
  return (
    <Card className="relative overflow-hidden">
      <CardBody className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wide text-muted">{label}</span>
          <Icon className={`h-4 w-4 ${ACCENTS[accent]}`} />
        </div>
        <span className="font-display text-3xl font-bold tabular-nums text-foreground">
          {value.toLocaleString()}
        </span>
      </CardBody>
    </Card>
  )
}

function SeverityBreakdown({
  counts,
  severities,
  total,
}: {
  counts: Record<string, number>
  severities: Severity[]
  total: number
}) {
  // Order rows by the configured severity rank; append any names not in the set.
  const known = severities.slice().sort((a, b) => a.rank - b.rank)
  const rows: { name: string; color: string; emoji: string; count: number }[] = []
  for (const s of known) {
    rows.push({ name: s.name, color: s.color || BUILTIN_SEVERITY_COLORS[s.name] || '#6b7280', emoji: s.emoji, count: counts[s.name] ?? 0 })
  }
  for (const [name, count] of Object.entries(counts)) {
    if (!known.some((s) => s.name === name)) {
      rows.push({ name, color: BUILTIN_SEVERITY_COLORS[name] || '#6b7280', emoji: '', count })
    }
  }

  if (total === 0) {
    return <p className="py-6 text-center text-sm text-muted">No traps received yet.</p>
  }

  return (
    <div className="space-y-3">
      {rows.map((r) => {
        const pct = total > 0 ? Math.round((r.count / total) * 100) : 0
        return (
          <div key={r.name}>
            <div className="mb-1 flex items-center justify-between text-sm">
              <SeverityBadge name={r.name} color={r.color} emoji={r.emoji || undefined} />
              <span className="tabular-nums text-muted">
                {r.count.toLocaleString()} <span className="text-muted/60">· {pct}%</span>
              </span>
            </div>
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-surface-2">
              <div className="h-full rounded-full" style={{ width: `${pct}%`, backgroundColor: r.color }} />
            </div>
          </div>
        )
      })}
    </div>
  )
}

function RecentActivity({ traps }: { traps: TrapView[] }) {
  if (traps.length === 0) {
    return (
      <EmptyState
        icon={<Activity className="h-8 w-8" />}
        title="No activity yet"
        description="Traps will appear here as your devices send them."
      />
    )
  }
  return (
    <ul className="divide-y divide-border/60">
      {traps.map((t) => (
        <li key={t.id} className="flex items-center gap-3 px-5 py-3">
          <span
            className="h-8 w-0.5 shrink-0 rounded-full"
            style={{ backgroundColor: t.severity_color || (t.severity_name ? BUILTIN_SEVERITY_COLORS[t.severity_name] : '') || '#334155' }}
          />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium text-foreground">{t.resolved_name}</p>
            <p className="truncate text-xs text-muted">
              <Mono>{t.device_name || t.source_ip}</Mono>
              {t.suppressed && <span className="ml-2 text-amber-400/80">suppressed</span>}
            </p>
          </div>
          <div className="flex shrink-0 flex-col items-end gap-1">
            <SeverityBadge name={t.severity_name} color={t.severity_color} emoji={t.severity_emoji} />
            <span className="text-[0.7rem] text-muted">{relativeTime(t.received_at)}</span>
          </div>
        </li>
      ))}
    </ul>
  )
}
