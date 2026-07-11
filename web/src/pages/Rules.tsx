import { useMemo, useState } from 'react'
import { Plus, Pencil, Trash2, ChevronUp, ChevronDown, GitBranch, Zap } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import type { Channel, Device, Rule, Severity } from '@/lib/types'
import { useRules, useChannels, useDevices, useSeverities, useInvalidate } from '@/lib/queries'
import { useToast } from '@/lib/toast'
import { PageHeader, LoadingRow, ErrorNote, EmptyState, Mono } from '@/components/common'
import { SeverityBadge } from '@/components/Severity'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Field, Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Dialog } from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { cn } from '@/lib/utils'

export function RulesPage() {
  const { data: rules = [], isLoading, error } = useRules()
  const { data: channels = [] } = useChannels()
  const { data: devices = [] } = useDevices()
  const { data: severities = [] } = useSeverities()
  const invalidate = useInvalidate()
  const toast = useToast()

  const [editing, setEditing] = useState<Rule | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<Rule | null>(null)

  const ordered = useMemo(() => rules.slice().sort((a, b) => a.ord - b.ord), [rules])
  const sevById = useMemo(() => new Map(severities.map((s) => [s.id, s])), [severities])

  const move = async (index: number, dir: -1 | 1) => {
    const next = index + dir
    if (next < 0 || next >= ordered.length) return
    const ids = ordered.map((r) => r.id)
    ;[ids[index], ids[next]] = [ids[next], ids[index]]
    try {
      await api.put('/rules/reorder', { ordered_ids: ids })
      invalidate('rules')
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not reorder rules.')
    }
  }

  const toggle = async (rule: Rule, enabled: boolean) => {
    try {
      await api.put(`/rules/${rule.id}`, { ...rule, enabled })
      invalidate('rules')
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not update rule.')
    }
  }

  return (
    <div>
      <PageHeader
        title="Rules"
        description="Evaluated top to bottom — the first match assigns severity and routes the trap."
        action={
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4" /> Add rule
          </Button>
        }
      />

      {error ? (
        <ErrorNote message={(error as Error).message} />
      ) : isLoading ? (
        <Card>
          <LoadingRow />
        </Card>
      ) : ordered.length === 0 ? (
        <Card>
          <EmptyState
            icon={<GitBranch className="h-8 w-8" />}
            title="No rules yet"
            description="Add one to start routing traps by OID, device, or content."
            action={
              <Button variant="primary" onClick={() => setCreating(true)}>
                <Plus className="h-4 w-4" /> Add rule
              </Button>
            }
          />
        </Card>
      ) : (
        <div className="space-y-2">
          {ordered.map((rule, i) => (
            <RuleRow
              key={rule.id}
              rule={rule}
              index={i}
              count={ordered.length}
              severity={rule.severity_id != null ? sevById.get(rule.severity_id) : undefined}
              channelCount={rule.channel_ids?.length ?? 0}
              onMove={move}
              onToggle={(enabled) => toggle(rule, enabled)}
              onEdit={() => setEditing(rule)}
              onDelete={() => setDeleting(rule)}
            />
          ))}
        </div>
      )}

      {(editing || creating) && (
        <RuleDialog
          rule={editing}
          channels={channels}
          devices={devices}
          severities={severities}
          nextOrd={ordered.length + 1}
          onClose={() => { setEditing(null); setCreating(false) }}
        />
      )}
      <DeleteRule rule={deleting} onClose={() => setDeleting(null)} />
    </div>
  )
}

function RuleRow({
  rule,
  index,
  count,
  severity,
  channelCount,
  onMove,
  onToggle,
  onEdit,
  onDelete,
}: {
  rule: Rule
  index: number
  count: number
  severity: Severity | undefined
  channelCount: number
  onMove: (index: number, dir: -1 | 1) => void
  onToggle: (enabled: boolean) => void
  onEdit: () => void
  onDelete: () => void
}) {
  return (
    <Card className={cn('flex items-center gap-3 p-3', !rule.enabled && 'opacity-60')}>
      <div className="flex flex-col">
        <button
          onClick={() => onMove(index, -1)}
          disabled={index === 0}
          className="text-muted transition hover:text-holo disabled:opacity-30"
          aria-label="Move up"
        >
          <ChevronUp className="h-4 w-4" />
        </button>
        <button
          onClick={() => onMove(index, 1)}
          disabled={index === count - 1}
          className="text-muted transition hover:text-holo disabled:opacity-30"
          aria-label="Move down"
        >
          <ChevronDown className="h-4 w-4" />
        </button>
      </div>

      <span className="w-8 shrink-0 text-center font-mono text-sm text-muted">{index + 1}</span>

      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium text-foreground">{rule.name}</span>
          {severity && <SeverityBadge name={severity.name} color={severity.color} emoji={severity.emoji} />}
          {rule.bypass_flood_control && (
            <Badge tone="holo">
              <Zap className="h-3 w-3" /> bypass flood
            </Badge>
          )}
          {rule.continue_on_match && <Badge tone="muted">continue</Badge>}
        </div>
        <p className="mt-0.5 truncate text-xs text-muted">
          <Mono>{rule.match_oid_glob || '*'}</Mono>
          {rule.match_varbind_regex && <span> · regex {rule.match_varbind_regex}</span>}
          <span> · {channelCount === 0 ? 'default routes' : `${channelCount} channel${channelCount === 1 ? '' : 's'}`}</span>
        </p>
      </div>

      <Switch checked={rule.enabled} onChange={onToggle} aria-label="Enable rule" />
      <Button variant="ghost" size="icon" onClick={onEdit} aria-label="Edit">
        <Pencil className="h-4 w-4" />
      </Button>
      <Button variant="ghost" size="icon" onClick={onDelete} aria-label="Delete">
        <Trash2 className="h-4 w-4" />
      </Button>
    </Card>
  )
}

function RuleDialog({
  rule,
  channels,
  devices,
  severities,
  nextOrd,
  onClose,
}: {
  rule: Rule | null
  channels: Channel[]
  devices: Device[]
  severities: Severity[]
  nextOrd: number
  onClose: () => void
}) {
  const isEdit = rule != null
  const invalidate = useInvalidate()
  const toast = useToast()

  const [name, setName] = useState(rule?.name ?? '')
  const [enabled, setEnabled] = useState(rule?.enabled ?? true)
  const [continueOnMatch, setContinueOnMatch] = useState(rule?.continue_on_match ?? false)
  const [bypassFlood, setBypassFlood] = useState(rule?.bypass_flood_control ?? false)
  const [deviceId, setDeviceId] = useState(rule?.match_device_id != null ? String(rule.match_device_id) : '')
  const [oidGlob, setOidGlob] = useState(rule?.match_oid_glob ?? '*')
  const [regex, setRegex] = useState(rule?.match_varbind_regex ?? '')
  const [severityId, setSeverityId] = useState(rule?.severity_id != null ? String(rule.severity_id) : '')
  const [channelIds, setChannelIds] = useState<number[]>(rule?.channel_ids ?? [])
  const [busy, setBusy] = useState(false)

  const toggleChannel = (id: number) =>
    setChannelIds((prev) => (prev.includes(id) ? prev.filter((c) => c !== id) : [...prev, id]))

  const save = async () => {
    setBusy(true)
    try {
      const body: Omit<Rule, 'id'> = {
        ord: rule?.ord ?? nextOrd,
        name: name.trim(),
        enabled,
        continue_on_match: continueOnMatch,
        bypass_flood_control: bypassFlood,
        match_device_id: deviceId ? Number(deviceId) : null,
        match_oid_glob: oidGlob.trim() || '*',
        match_varbind_regex: regex.trim() ? regex.trim() : null,
        severity_id: severityId ? Number(severityId) : null,
        channel_ids: channelIds,
      }
      if (isEdit) await api.put(`/rules/${rule.id}`, { ...body, id: rule.id })
      else await api.post('/rules', body)
      toast.push('success', isEdit ? 'Rule saved.' : 'Rule added.')
      invalidate('rules')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not save rule.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      size="lg"
      title={isEdit ? 'Edit rule' : 'Add rule'}
      description="Match on device and OID; assign a severity and route to channels."
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={save} loading={busy} disabled={!name.trim()}>
            Save changes
          </Button>
        </>
      }
    >
      <div className="space-y-5">
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus placeholder="Critical link failures" />
        </Field>

        <fieldset className="space-y-4 rounded-lg border border-border p-4">
          <legend className="px-1 text-xs font-semibold uppercase tracking-wide text-muted">Match</legend>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Device" hint="any if unset">
              <Select value={deviceId} onChange={(e) => setDeviceId(e.target.value)}>
                <option value="">Any device</option>
                {devices.map((d) => (
                  <option key={d.id} value={d.id}>{d.name || d.source_ip}</option>
                ))}
              </Select>
            </Field>
            <Field label="OID glob" hint='"*" matches any'>
              <Input value={oidGlob} onChange={(e) => setOidGlob(e.target.value)} className="font-mono" placeholder="1.3.6.1.6.3.1.1.5.*" />
            </Field>
          </div>
          <Field label="Varbind regex" hint="optional; applied to the message text">
            <Input value={regex} onChange={(e) => setRegex(e.target.value)} className="font-mono" placeholder="(down|failed)" />
          </Field>
        </fieldset>

        <fieldset className="space-y-4 rounded-lg border border-border p-4">
          <legend className="px-1 text-xs font-semibold uppercase tracking-wide text-muted">Assign & route</legend>
          <Field label="Severity" hint="inherit from OID map if unset">
            <Select value={severityId} onChange={(e) => setSeverityId(e.target.value)}>
              <option value="">Inherit from OID map</option>
              {severities.slice().sort((a, b) => a.rank - b.rank).map((s) => (
                <option key={s.id} value={s.id}>{s.emoji ? `${s.emoji} ` : ''}{s.name}</option>
              ))}
            </Select>
          </Field>
          <div>
            <Label hint="none = severity default routes">Channels</Label>
            {channels.length === 0 ? (
              <p className="text-sm text-muted">No channels yet — add one on the Channels page.</p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {channels.map((c) => {
                  const on = channelIds.includes(c.id)
                  return (
                    <button
                      key={c.id}
                      type="button"
                      onClick={() => toggleChannel(c.id)}
                      className={cn(
                        'rounded-full border px-3 py-1 text-sm transition-colors',
                        on ? 'border-holo bg-holo/10 text-holo' : 'border-border text-muted hover:text-foreground',
                      )}
                    >
                      {c.name}
                    </button>
                  )
                })}
              </div>
            )}
          </div>
        </fieldset>

        <div className="grid gap-3 sm:grid-cols-3">
          <ToggleRow label="Enabled" checked={enabled} onChange={setEnabled} />
          <ToggleRow
            label="Continue on match"
            hint="keep evaluating later rules"
            checked={continueOnMatch}
            onChange={setContinueOnMatch}
          />
          <ToggleRow
            label="Bypass flood control"
            hint="always send, even when suppressing"
            checked={bypassFlood}
            onChange={setBypassFlood}
          />
        </div>
      </div>
    </Dialog>
  )
}

function ToggleRow({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint?: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-start justify-between gap-2 rounded-lg border border-border p-3">
      <div>
        <p className="text-sm font-medium text-foreground">{label}</p>
        {hint && <p className="text-xs text-muted">{hint}</p>}
      </div>
      <Switch checked={checked} onChange={onChange} aria-label={label} />
    </div>
  )
}

function DeleteRule({ rule, onClose }: { rule: Rule | null; onClose: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const remove = async () => {
    if (!rule) return
    setBusy(true)
    try {
      await api.del(`/rules/${rule.id}`)
      toast.push('success', 'Rule removed.')
      invalidate('rules')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not delete rule.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <ConfirmDialog
      open={rule != null}
      onClose={onClose}
      onConfirm={remove}
      loading={busy}
      title="Delete rule"
      body={`Remove "${rule?.name}"? Traps it matched will fall through to later rules.`}
    />
  )
}
