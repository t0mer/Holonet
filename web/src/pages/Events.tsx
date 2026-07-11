import { useState } from 'react'
import { ArrowDown, ArrowUp, ChevronsUpDown, Radio } from 'lucide-react'
import { useTraps } from '@/lib/queries'
import type { TrapView } from '@/lib/types'
import { PageHeader, LoadingRow, ErrorNote, EmptyState, Mono } from '@/components/common'
import { SeverityBadge, trapSeverityColor } from '@/components/Severity'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Select } from '@/components/ui/select'
import { Table, THead, TBody, TR, TH, TD } from '@/components/ui/table'
import { TrapDetail } from './TrapDetail'
import { formatTimestamp, cn } from '@/lib/utils'

type SortKey = 'received_at' | 'source_ip' | 'resolved_name' | 'severity' | 'matched_rule' | 'status' | 'id'
type Order = 'asc' | 'desc'

interface Column {
  key: SortKey
  label: string
  className?: string
}

const COLUMNS: Column[] = [
  { key: 'received_at', label: 'Time' },
  { key: 'source_ip', label: 'Source' },
  { key: 'resolved_name', label: 'Event' },
  { key: 'severity', label: 'Severity' },
  { key: 'matched_rule', label: 'Rule' },
  { key: 'status', label: 'Status' },
  { key: 'id', label: 'Version' },
]

function statusOf(t: TrapView): { label: string; tone: 'success' | 'warn' | 'muted' } {
  if (t.suppressed) return { label: 'Suppressed', tone: 'warn' }
  if (t.unmapped) return { label: 'Unmapped', tone: 'muted' }
  return { label: 'Dispatched', tone: 'success' }
}

export function EventsPage() {
  const [sort, setSort] = useState<SortKey>('received_at')
  const [order, setOrder] = useState<Order>('desc')
  const [limit, setLimit] = useState(20)
  const [selected, setSelected] = useState<TrapView | null>(null)

  const { data: traps = [], isLoading, error } = useTraps(sort, order, limit)

  const toggleSort = (key: SortKey) => {
    if (key === sort) {
      setOrder((o) => (o === 'asc' ? 'desc' : 'asc'))
    } else {
      setSort(key)
      setOrder('desc')
    }
  }

  return (
    <div>
      <PageHeader
        title="Events"
        description="Traps received, decoded, and classified. Click a row for varbinds and dispatch."
        action={
          <label className="flex items-center gap-2 text-sm text-muted">
            Show
            <Select value={limit} onChange={(e) => setLimit(Number(e.target.value))} className="w-20">
              <option value={20}>20</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </Select>
          </label>
        }
      />

      {/* Mobile sort control (table collapses to cards) */}
      <div className="mb-3 flex items-center gap-2 sm:hidden">
        <Select value={sort} onChange={(e) => setSort(e.target.value as SortKey)} className="flex-1">
          {COLUMNS.map((c) => (
            <option key={c.key} value={c.key}>
              Sort by {c.label}
            </option>
          ))}
        </Select>
        <Select value={order} onChange={(e) => setOrder(e.target.value as Order)} className="w-28">
          <option value="desc">Desc</option>
          <option value="asc">Asc</option>
        </Select>
      </div>

      {error ? (
        <ErrorNote message={(error as Error).message} />
      ) : isLoading ? (
        <Card>
          <LoadingRow label="Loading events…" />
        </Card>
      ) : traps.length === 0 ? (
        <Card>
          <EmptyState
            icon={<Radio className="h-8 w-8" />}
            title="No events yet"
            description="Once your devices send traps to this host, they'll stream in here."
          />
        </Card>
      ) : (
        <>
          {/* Desktop / tablet table */}
          <Card className="hidden overflow-hidden sm:block">
            <Table>
              <THead>
                <TR className="border-border">
                  {COLUMNS.map((c) => (
                    <TH key={c.key} className={c.className}>
                      <SortHeader
                        label={c.label}
                        active={sort === c.key}
                        order={order}
                        onClick={() => toggleSort(c.key)}
                      />
                    </TH>
                  ))}
                </TR>
              </THead>
              <TBody>
                {traps.map((t) => {
                  const st = statusOf(t)
                  return (
                    <TR
                      key={t.id}
                      onClick={() => setSelected(t)}
                      className="cursor-pointer transition-colors hover:bg-surface-2/50"
                    >
                      <TD className="relative whitespace-nowrap">
                        <span
                          className="absolute inset-y-1 left-0 w-0.5 rounded-full"
                          style={{ backgroundColor: trapSeverityColor(t) }}
                        />
                        <Mono className="pl-2 text-muted">{formatTimestamp(t.received_at)}</Mono>
                      </TD>
                      <TD className="whitespace-nowrap">
                        {t.device_name ? (
                          <span className="text-foreground">{t.device_name}</span>
                        ) : (
                          <Mono className="text-muted">{t.source_ip}</Mono>
                        )}
                      </TD>
                      <TD className="max-w-xs">
                        <span className="block truncate font-medium">{t.resolved_name}</span>
                      </TD>
                      <TD>
                        <SeverityBadge name={t.severity_name} color={t.severity_color} emoji={t.severity_emoji} />
                      </TD>
                      <TD className="whitespace-nowrap text-muted">{t.matched_rule_name || '—'}</TD>
                      <TD>
                        <Badge tone={st.tone}>{st.label}</Badge>
                      </TD>
                      <TD className="whitespace-nowrap">
                        <Badge tone="muted">{t.snmp_version}</Badge>
                      </TD>
                    </TR>
                  )
                })}
              </TBody>
            </Table>
          </Card>

          {/* Mobile stacked cards */}
          <div className="space-y-2 sm:hidden">
            {traps.map((t) => {
              const st = statusOf(t)
              return (
                <button
                  key={t.id}
                  onClick={() => setSelected(t)}
                  className="flex w-full items-stretch gap-3 rounded-lg border border-border bg-surface p-3 text-left transition-colors hover:bg-surface-2/50"
                >
                  <span className="w-1 shrink-0 rounded-full" style={{ backgroundColor: trapSeverityColor(t) }} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-start justify-between gap-2">
                      <span className="truncate font-medium text-foreground">{t.resolved_name}</span>
                      <SeverityBadge name={t.severity_name} color={t.severity_color} emoji={t.severity_emoji} />
                    </div>
                    <p className="mt-1 truncate text-xs text-muted">
                      <Mono>{t.device_name || t.source_ip}</Mono> · {formatTimestamp(t.received_at)}
                    </p>
                    <div className="mt-2 flex items-center gap-2">
                      <Badge tone={st.tone}>{st.label}</Badge>
                      <Badge tone="muted">{t.snmp_version}</Badge>
                    </div>
                  </div>
                </button>
              )
            })}
          </div>
        </>
      )}

      <TrapDetail trap={selected} onClose={() => setSelected(null)} />
    </div>
  )
}

function SortHeader({
  label,
  active,
  order,
  onClick,
}: {
  label: string
  active: boolean
  order: Order
  onClick: () => void
}) {
  const Icon = !active ? ChevronsUpDown : order === 'asc' ? ArrowUp : ArrowDown
  return (
    <button
      onClick={onClick}
      className={cn(
        'inline-flex items-center gap-1 transition-colors hover:text-foreground',
        active && 'text-holo',
      )}
    >
      {label}
      <Icon className="h-3.5 w-3.5" />
    </button>
  )
}
