import { useState } from 'react'
import { Plus, Pencil, Trash2, Send, SendHorizontal, X } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import type { Channel, ChannelKind, WebhookHeader } from '@/lib/types'
import { useChannels, useInvalidate } from '@/lib/queries'
import { useToast } from '@/lib/toast'
import { PageHeader, LoadingRow, ErrorNote, EmptyState } from '@/components/common'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Field, Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Dialog } from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { ConfirmDialog } from '@/components/ConfirmDialog'

const KIND_LABELS: Record<ChannelKind, string> = {
  shoutrrr: 'Shoutrrr',
  whatsapp: 'WhatsApp',
  greenapi: 'GreenAPI (WhatsApp Cloud)',
  webhook: 'Webhook',
}

const KIND_HINTS: Record<ChannelKind, string> = {
  shoutrrr: 'Telegram, Discord, Slack, ntfy, email, and more via one URL',
  whatsapp: 'go-whatsapp-web-multidevice gateway',
  greenapi: 'WhatsApp Cloud via the GreenAPI service',
  webhook: 'Generic HTTP POST with your own body',
}

export function ChannelsPage() {
  const { data: channels = [], isLoading, error } = useChannels()
  const [editing, setEditing] = useState<Channel | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<Channel | null>(null)

  return (
    <div>
      <PageHeader
        title="Channels"
        description="Where notifications are delivered. Credentials are write-only and never shown again."
        action={
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4" /> Add channel
          </Button>
        }
      />

      {error ? (
        <ErrorNote message={(error as Error).message} />
      ) : isLoading ? (
        <Card>
          <LoadingRow />
        </Card>
      ) : channels.length === 0 ? (
        <Card>
          <EmptyState
            icon={<Send className="h-8 w-8" />}
            title="No channels yet"
            description="Add a channel to start delivering trap notifications."
            action={
              <Button variant="primary" onClick={() => setCreating(true)}>
                <Plus className="h-4 w-4" /> Add channel
              </Button>
            }
          />
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {channels.map((c) => (
            <ChannelCard
              key={c.id}
              channel={c}
              onEdit={() => setEditing(c)}
              onDelete={() => setDeleting(c)}
            />
          ))}
        </div>
      )}

      {(editing || creating) && (
        <ChannelDialog channel={editing} onClose={() => { setEditing(null); setCreating(false) }} />
      )}
      <DeleteChannel channel={deleting} onClose={() => setDeleting(null)} />
    </div>
  )
}

function ChannelCard({
  channel,
  onEdit,
  onDelete,
}: {
  channel: Channel
  onEdit: () => void
  onDelete: () => void
}) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const [testing, setTesting] = useState(false)

  const toggle = async (enabled: boolean) => {
    try {
      await api.put(`/channels/${channel.id}`, { name: channel.name, kind: channel.kind, enabled, config: null })
      invalidate('channels')
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not update channel.')
    }
  }

  const sendTest = async () => {
    setTesting(true)
    try {
      await api.post(`/test/${channel.id}`)
      toast.push('success', `Test sent to ${channel.name}.`)
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Test failed.')
    } finally {
      setTesting(false)
    }
  }

  return (
    <Card>
      <div className="flex items-start justify-between gap-3 p-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="truncate font-semibold text-foreground">{channel.name}</span>
            {!channel.enabled && <Badge tone="muted">disabled</Badge>}
          </div>
          <p className="mt-0.5 text-xs text-muted">{KIND_LABELS[channel.kind as ChannelKind] ?? channel.kind}</p>
        </div>
        <Switch checked={channel.enabled} onChange={toggle} aria-label="Enable channel" />
      </div>
      <div className="flex items-center gap-1 border-t border-border px-3 py-2">
        <Button variant="ghost" size="sm" onClick={sendTest} loading={testing}>
          <SendHorizontal className="h-4 w-4" /> Send test
        </Button>
        <div className="flex-1" />
        <Button variant="ghost" size="icon" onClick={onEdit} aria-label="Edit">
          <Pencil className="h-4 w-4" />
        </Button>
        <Button variant="ghost" size="icon" onClick={onDelete} aria-label="Delete">
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </Card>
  )
}

// ---- Create / edit ----

function ChannelDialog({ channel, onClose }: { channel: Channel | null; onClose: () => void }) {
  const isEdit = channel != null
  const invalidate = useInvalidate()
  const toast = useToast()

  const [name, setName] = useState(channel?.name ?? '')
  const [kind, setKind] = useState<ChannelKind>((channel?.kind as ChannelKind) ?? 'shoutrrr')
  const [enabled, setEnabled] = useState(channel?.enabled ?? true)

  // Provider config — secrets are write-only, so edit starts blank.
  const [shoutrrrUrl, setShoutrrrUrl] = useState('')
  const [waBase, setWaBase] = useState('')
  const [waRecipient, setWaRecipient] = useState('')
  const [waEndpoint, setWaEndpoint] = useState('')
  const [waUser, setWaUser] = useState('')
  const [waPass, setWaPass] = useState('')
  const [waToken, setWaToken] = useState('')
  const [gaInstance, setGaInstance] = useState('')
  const [gaToken, setGaToken] = useState('')
  const [gaRecipient, setGaRecipient] = useState('')
  const [gaApiUrl, setGaApiUrl] = useState('')
  const [hookUrl, setHookUrl] = useState('')
  const [hookHeaders, setHookHeaders] = useState<WebhookHeader[]>([])
  const [hookBody, setHookBody] = useState('')

  const [busy, setBusy] = useState(false)
  const [testing, setTesting] = useState(false)

  const buildConfig = (): Record<string, unknown> | null => {
    if (kind === 'shoutrrr') {
      return shoutrrrUrl.trim() ? { url: shoutrrrUrl.trim() } : null
    }
    if (kind === 'whatsapp') {
      if (!waBase.trim() && !waRecipient.trim()) return null
      const cfg: Record<string, unknown> = {
        base_url: waBase.trim(),
        recipient: waRecipient.trim(),
      }
      if (waEndpoint.trim()) cfg.endpoint_path = waEndpoint.trim()
      if (waUser.trim()) cfg.username = waUser.trim()
      if (waPass) cfg.password = waPass
      if (waToken.trim()) cfg.token = waToken.trim()
      return cfg
    }
    if (kind === 'greenapi') {
      // On edit, the token is write-only: blank means keep the sealed value, so
      // don't force a rebuild just because it's empty.
      if (!gaInstance.trim() && !gaRecipient.trim() && !gaToken.trim()) return null
      const cfg: Record<string, unknown> = {
        instance_id: gaInstance.trim(),
        recipient: gaRecipient.trim(),
      }
      if (gaToken.trim()) cfg.token = gaToken.trim()
      if (gaApiUrl.trim()) cfg.api_url = gaApiUrl.trim()
      return cfg
    }
    // webhook
    if (!hookUrl.trim()) return null
    const headers: Record<string, string> = {}
    for (const h of hookHeaders) if (h.key.trim()) headers[h.key.trim()] = h.value
    return {
      url: hookUrl.trim(),
      headers,
      body_template: hookBody,
    }
  }

  const save = async () => {
    setBusy(true)
    try {
      const config = buildConfig()
      // On edit with no new config, send null to preserve the saved secret.
      const body = { name: name.trim(), kind, enabled, config }
      if (isEdit) await api.put(`/channels/${channel.id}`, body)
      else await api.post('/channels', body)
      toast.push('success', isEdit ? 'Channel saved.' : 'Channel added.')
      invalidate('channels')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not save channel.')
    } finally {
      setBusy(false)
    }
  }

  // Test unsaved values through POST /test {kind, config}.
  const sendTest = async () => {
    const config = buildConfig()
    if (!config) {
      toast.push('error', 'Enter the channel details before sending a test.')
      return
    }
    setTesting(true)
    try {
      await api.post('/test', { kind, config })
      toast.push('success', 'Test message sent.')
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Test failed.')
    } finally {
      setTesting(false)
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title={isEdit ? 'Edit channel' : 'Add channel'}
      description={isEdit ? 'Leave secret fields blank to keep the saved values.' : undefined}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="secondary" onClick={sendTest} loading={testing}>
            <SendHorizontal className="h-4 w-4" /> Send test
          </Button>
          <Button variant="primary" onClick={save} loading={busy} disabled={!name.trim()}>
            Save changes
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus placeholder="Ops Telegram" />
        </Field>

        <Field label="Provider" hint={KIND_HINTS[kind]}>
          <Select value={kind} onChange={(e) => setKind(e.target.value as ChannelKind)}>
            {(Object.keys(KIND_LABELS) as ChannelKind[]).map((k) => (
              <option key={k} value={k}>{KIND_LABELS[k]}</option>
            ))}
          </Select>
        </Field>

        <div className="flex items-center justify-between rounded-lg border border-border p-3">
          <div>
            <p className="text-sm font-medium text-foreground">Enabled</p>
            <p className="text-xs text-muted">Disabled channels are skipped during dispatch.</p>
          </div>
          <Switch checked={enabled} onChange={setEnabled} aria-label="Enabled" />
        </div>

        {kind === 'shoutrrr' && (
          <Field label="Shoutrrr URL" hint={isEdit ? 'leave blank to keep' : undefined}>
            <Input
              value={shoutrrrUrl}
              onChange={(e) => setShoutrrrUrl(e.target.value)}
              className="font-mono"
              placeholder="telegram://token@telegram?chats=@channel"
            />
          </Field>
        )}

        {kind === 'whatsapp' && (
          <div className="space-y-4">
            <Field label="Gateway base URL">
              <Input value={waBase} onChange={(e) => setWaBase(e.target.value)} placeholder="https://wa.example.com" />
            </Field>
            <Field label="Recipient" hint="JID or group">
              <Input value={waRecipient} onChange={(e) => setWaRecipient(e.target.value)} placeholder="123456789@s.whatsapp.net" />
            </Field>
            <Field label="Send endpoint path" hint="optional; tracks your gateway version">
              <Input value={waEndpoint} onChange={(e) => setWaEndpoint(e.target.value)} className="font-mono" placeholder="/send/message" />
            </Field>
            <div className="grid grid-cols-2 gap-4">
              <Field label="Username" hint="optional">
                <Input value={waUser} onChange={(e) => setWaUser(e.target.value)} autoComplete="off" />
              </Field>
              <Field label="Password" hint={isEdit ? 'blank = keep' : 'optional'}>
                <Input type="password" value={waPass} onChange={(e) => setWaPass(e.target.value)} autoComplete="new-password" />
              </Field>
            </div>
            <Field label="Bearer token" hint={isEdit ? 'blank = keep' : 'optional'}>
              <Input type="password" value={waToken} onChange={(e) => setWaToken(e.target.value)} autoComplete="new-password" />
            </Field>
          </div>
        )}

        {kind === 'greenapi' && (
          <div className="space-y-4">
            <Field label="Instance ID">
              <Input value={gaInstance} onChange={(e) => setGaInstance(e.target.value)} className="font-mono" placeholder="7103xxxxxx" />
            </Field>
            <Field label="API token" hint={isEdit ? 'blank = keep' : undefined}>
              <Input type="password" value={gaToken} onChange={(e) => setGaToken(e.target.value)} autoComplete="new-password" placeholder="copy from the GreenAPI console" />
            </Field>
            <Field label="Recipient phone" hint="international format, digits only — no + or spaces (e.g. 972501234567); or a JID">
              <Input value={gaRecipient} onChange={(e) => setGaRecipient(e.target.value)} className="font-mono" placeholder="972501234567" />
            </Field>
            <Field label="API URL" hint="optional; blank = https://api.green-api.com — set your cluster URL if the console shows one">
              <Input value={gaApiUrl} onChange={(e) => setGaApiUrl(e.target.value)} className="font-mono" placeholder="https://7103.api.greenapi.com" />
            </Field>
          </div>
        )}

        {kind === 'webhook' && (
          <div className="space-y-4">
            <Field label="URL">
              <Input value={hookUrl} onChange={(e) => setHookUrl(e.target.value)} placeholder="https://hooks.example.com/holonet" />
            </Field>
            <HeaderEditor headers={hookHeaders} onChange={setHookHeaders} />
            <Field label="Body template" hint="text/template; trap fields available">
              <Textarea
                value={hookBody}
                onChange={(e) => setHookBody(e.target.value)}
                placeholder={'{"text": "{{.Title}} — {{.Body}}"}'}
              />
            </Field>
          </div>
        )}
      </div>
    </Dialog>
  )
}

function HeaderEditor({ headers, onChange }: { headers: WebhookHeader[]; onChange: (h: WebhookHeader[]) => void }) {
  const update = (i: number, patch: Partial<WebhookHeader>) =>
    onChange(headers.map((h, idx) => (idx === i ? { ...h, ...patch } : h)))
  return (
    <div>
      <Label>Headers</Label>
      <div className="space-y-2">
        {headers.map((h, i) => (
          <div key={i} className="flex items-center gap-2">
            <Input
              value={h.key}
              onChange={(e) => update(i, { key: e.target.value })}
              placeholder="Header"
              className="font-mono"
            />
            <Input
              value={h.value}
              onChange={(e) => update(i, { value: e.target.value })}
              placeholder="Value"
              className="font-mono"
            />
            <Button
              variant="ghost"
              size="icon"
              onClick={() => onChange(headers.filter((_, idx) => idx !== i))}
              aria-label="Remove header"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        ))}
        <Button variant="outline" size="sm" onClick={() => onChange([...headers, { key: '', value: '' }])}>
          <Plus className="h-4 w-4" /> Add header
        </Button>
      </div>
    </div>
  )
}

function DeleteChannel({ channel, onClose }: { channel: Channel | null; onClose: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const remove = async () => {
    if (!channel) return
    setBusy(true)
    try {
      await api.del(`/channels/${channel.id}`)
      toast.push('success', 'Channel removed.')
      invalidate('channels')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not delete channel.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <ConfirmDialog
      open={channel != null}
      onClose={onClose}
      onConfirm={remove}
      loading={busy}
      title="Delete channel"
      body={`Remove "${channel?.name}"? Rules routing to it will stop delivering here.`}
    />
  )
}
