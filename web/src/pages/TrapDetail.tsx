import { RefreshCw, MapPin } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import type { TrapView } from '@/lib/types'
import { useTrapNotifications, useSeverities, useInvalidate } from '@/lib/queries'
import { useToast } from '@/lib/toast'
import { useState } from 'react'
import { Dialog } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Field } from '@/components/ui/label'
import { SeverityBadge } from '@/components/Severity'
import { Mono, LoadingRow } from '@/components/common'
import { formatTimestamp, prettyJSON } from '@/lib/utils'

const STATUS_TONE: Record<string, 'success' | 'danger' | 'warn' | 'muted'> = {
  sent: 'success',
  failed: 'danger',
  held: 'warn',
}

export function TrapDetail({ trap, onClose }: { trap: TrapView | null; onClose: () => void }) {
  const { data: notifications = [], isLoading } = useTrapNotifications(trap?.id ?? null)
  const invalidate = useInvalidate()
  const toast = useToast()
  const [replaying, setReplaying] = useState(false)
  const [mapping, setMapping] = useState(false)

  const replay = async () => {
    if (!trap) return
    setReplaying(true)
    try {
      await api.post(`/replay/${trap.id}`)
      toast.push('success', 'Replayed routing for this trap.')
      invalidate('traps', 'dashboard')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Replay failed.')
    } finally {
      setReplaying(false)
    }
  }

  return (
    <>
    <Dialog
      open={trap != null}
      onClose={onClose}
      size="lg"
      title={trap?.resolved_name ?? 'Trap'}
      description={trap ? `Trap #${trap.id} · received ${formatTimestamp(trap.received_at)}` : undefined}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
          {trap?.unmapped && (
            <Button variant="ghost" onClick={() => setMapping(true)}>
              <MapPin className="h-4 w-4" /> Map this OID
            </Button>
          )}
          <Button variant="primary" onClick={replay} loading={replaying}>
            <RefreshCw className="h-4 w-4" /> Replay routing
          </Button>
        </>
      }
    >
      {trap && (
        <div className="space-y-5">
          <dl className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3">
            <Detail label="Severity">
              <SeverityBadge name={trap.severity_name} color={trap.severity_color} emoji={trap.severity_emoji} />
            </Detail>
            <Detail label="Source">
              <Mono>{trap.device_name || trap.source_ip}</Mono>
            </Detail>
            <Detail label="SNMP version">
              <Badge tone="muted">{trap.snmp_version}</Badge>
            </Detail>
            <Detail label="Matched rule">
              {trap.matched_rule_name ? (
                <span className="text-sm text-foreground">{trap.matched_rule_name}</span>
              ) : (
                <span className="text-sm text-muted">No rule matched</span>
              )}
            </Detail>
            <Detail label="State">
              {trap.suppressed ? (
                <Badge tone="warn">Suppressed</Badge>
              ) : (
                <Badge tone="success">Dispatched</Badge>
              )}
              {trap.unmapped && <Badge tone="muted">Unmapped OID</Badge>}
            </Detail>
            <Detail label="Trap OID" className="col-span-2 sm:col-span-3">
              <Mono className="break-all">{trap.trap_oid}</Mono>
            </Detail>
          </dl>

          <section>
            <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted">Varbinds</h4>
            <pre className="max-h-64 overflow-auto rounded-lg border border-border bg-background p-3 font-mono text-xs leading-relaxed text-foreground">
              {prettyJSON(trap.varbinds_json || '{}')}
            </pre>
          </section>

          <section>
            <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted">Notifications</h4>
            {isLoading ? (
              <LoadingRow label="Loading dispatch log…" />
            ) : notifications.length === 0 ? (
              <p className="rounded-lg border border-border bg-background px-3 py-4 text-sm text-muted">
                No notifications recorded for this trap.
              </p>
            ) : (
              <ul className="space-y-2">
                {notifications.map((n) => (
                  <li
                    key={n.id}
                    className="flex items-center justify-between gap-3 rounded-lg border border-border bg-background px-3 py-2 text-sm"
                  >
                    <div className="min-w-0">
                      <span className="text-foreground">Channel #{n.channel_id ?? '—'}</span>
                      {n.last_error && <p className="truncate text-xs text-red-400">{n.last_error}</p>}
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      {n.attempts > 0 && <span className="text-xs text-muted">{n.attempts} attempts</span>}
                      <Badge tone={STATUS_TONE[n.status] ?? 'muted'}>{n.status}</Badge>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>
      )}
    </Dialog>
    {mapping && trap && (
      <QuickMapDialog
        oid={trap.trap_oid}
        onClose={() => setMapping(false)}
        onMapped={() => { invalidate('oidmap'); setMapping(false) }}
      />
    )}
    </>
  )
}

function QuickMapDialog({ oid, onClose, onMapped }: { oid: string; onClose: () => void; onMapped: () => void }) {
  const { data: severities = [] } = useSeverities()
  const toast = useToast()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [severityId, setSeverityId] = useState('')
  const [busy, setBusy] = useState(false)

  const save = async () => {
    setBusy(true)
    try {
      await api.post('/oidmap', {
        oid,
        name: name.trim(),
        description: description.trim(),
        default_severity_id: severityId ? Number(severityId) : null,
        is_builtin: false,
      })
      toast.push('success', 'OID mapped. New traps for it will resolve by name.')
      onMapped()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not map the OID.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title="Map OID"
      description="Give this OID a name and default severity so future traps classify automatically."
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={save} loading={busy} disabled={!name.trim()}>
            Map OID
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="OID">
          <Input value={oid} readOnly className="font-mono opacity-70" />
        </Field>
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus placeholder="sfosLiveUserLogin" />
        </Field>
        <Field label="Description" hint="optional">
          <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="What this event means" />
        </Field>
        <Field label="Default severity" hint="applied when no rule overrides it">
          <Select value={severityId} onChange={(e) => setSeverityId(e.target.value)}>
            <option value="">Unclassified</option>
            {severities.slice().sort((a, b) => a.rank - b.rank).map((s) => (
              <option key={s.id} value={s.id}>{s.emoji ? `${s.emoji} ` : ''}{s.name}</option>
            ))}
          </Select>
        </Field>
      </div>
    </Dialog>
  )
}

function Detail({ label, children, className }: { label: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={className}>
      <dt className="mb-1 text-xs font-medium uppercase tracking-wide text-muted">{label}</dt>
      <dd className="flex flex-wrap items-center gap-1.5">{children}</dd>
    </div>
  )
}
