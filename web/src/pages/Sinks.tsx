import { useEffect, useState } from 'react'
import { Plus, Trash2, KeyRound, Save, ShieldCheck } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import type { Community, V3User } from '@/lib/types'
import { useCommunities, useV3Users, useSettings, useInvalidate } from '@/lib/queries'
import { useToast } from '@/lib/toast'
import { PageHeader, LoadingRow, ErrorNote, EmptyState, Mono } from '@/components/common'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Select } from '@/components/ui/select'
import { Dialog } from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { ConfirmDialog } from '@/components/ConfirmDialog'

export function SinksPage() {
  const { data: communities = [], isLoading, error } = useCommunities()
  const [adding, setAdding] = useState(false)
  const [deleting, setDeleting] = useState<Community | null>(null)

  return (
    <div>
      <PageHeader
        title="Sinks"
        description="Where traps arrive. Community strings are sealed at rest and never shown again."
      />

      <div className="space-y-6">
        <BindAddress />

        <Card>
          <CardHeader
            title="v2c communities"
            description="Traps whose community matches an enabled entry are accepted."
            action={
              <Button variant="primary" size="sm" onClick={() => setAdding(true)}>
                <Plus className="h-4 w-4" /> Add community
              </Button>
            }
          />
          <CardBody className="p-0">
            {error ? (
              <div className="p-5">
                <ErrorNote message={(error as Error).message} />
              </div>
            ) : isLoading ? (
              <LoadingRow />
            ) : communities.length === 0 ? (
              <EmptyState
                icon={<KeyRound className="h-8 w-8" />}
                title="No communities yet"
                description="Add a v2c community string to start accepting traps."
              />
            ) : (
              <ul className="divide-y divide-border/60">
                {communities.map((c) => (
                  <CommunityRow key={c.id} community={c} onDelete={() => setDeleting(c)} />
                ))}
              </ul>
            )}
          </CardBody>
        </Card>

        <V3Users />
      </div>

      {adding && <AddCommunity onClose={() => setAdding(false)} />}
      <DeleteCommunity community={deleting} onClose={() => setDeleting(null)} />
    </div>
  )
}

const AUTH_PROTOCOLS = ['SHA', 'SHA256', 'SHA512', 'SHA224', 'SHA384', 'MD5']
const PRIV_PROTOCOLS = ['AES', 'AES256', 'AES192', 'DES']

function V3Users() {
  const { data: users = [], isLoading, error } = useV3Users()
  const [adding, setAdding] = useState(false)
  const [deleting, setDeleting] = useState<V3User | null>(null)

  return (
    <Card>
      <CardHeader
        title="SNMPv3 users"
        description="USM credentials. authNoPriv minimum, authPriv recommended — passwords are sealed at rest."
        action={
          <Button variant="primary" size="sm" onClick={() => setAdding(true)}>
            <Plus className="h-4 w-4" /> Add user
          </Button>
        }
      />
      <CardBody className="p-0">
        {error ? (
          <div className="p-5">
            <ErrorNote message={(error as Error).message} />
          </div>
        ) : isLoading ? (
          <LoadingRow />
        ) : users.length === 0 ? (
          <EmptyState
            icon={<ShieldCheck className="h-8 w-8" />}
            title="No v3 users yet"
            description="Add a USM user to accept authenticated (and optionally encrypted) v3 traps."
          />
        ) : (
          <ul className="divide-y divide-border/60">
            {users.map((u) => (
              <V3UserRow key={u.id} user={u} onDelete={() => setDeleting(u)} />
            ))}
          </ul>
        )}
      </CardBody>
      {adding && <AddV3User onClose={() => setAdding(false)} />}
      <DeleteV3User user={deleting} onClose={() => setDeleting(null)} />
    </Card>
  )
}

function V3UserRow({ user, onDelete }: { user: V3User; onDelete: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const toggle = async (enabled: boolean) => {
    try {
      await api.put(`/sinks/v3users/${user.id}`, {
        username: user.username,
        security_level: user.security_level,
        auth_protocol: user.auth_protocol,
        priv_protocol: user.priv_protocol,
        engine_id: user.engine_id,
        enabled,
      })
      invalidate('v3users')
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not update user.')
    }
  }
  return (
    <li className="flex items-center justify-between gap-3 px-5 py-3">
      <div className="flex items-center gap-3">
        <ShieldCheck className="h-4 w-4 text-muted" />
        <div>
          <p className="font-mono text-sm font-medium text-foreground">{user.username}</p>
          <p className="text-xs text-muted">
            {user.security_level} · auth {user.auth_protocol}
            {user.security_level === 'authPriv' ? ` · priv ${user.priv_protocol}` : ''}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-3">
        {user.enabled ? <Badge tone="success">enabled</Badge> : <Badge tone="muted">disabled</Badge>}
        <Switch checked={user.enabled} onChange={toggle} aria-label="Enable user" />
        <Button variant="ghost" size="icon" onClick={onDelete} aria-label="Delete">
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </li>
  )
}

function AddV3User({ onClose }: { onClose: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const [form, setForm] = useState({
    username: '',
    security_level: 'authPriv',
    auth_protocol: 'SHA',
    auth_pass: '',
    priv_protocol: 'AES',
    priv_pass: '',
    engine_id: '',
    enabled: true,
  })
  const set = (k: keyof typeof form, v: string | boolean) => setForm((f) => ({ ...f, [k]: v }))
  const priv = form.security_level === 'authPriv'
  const valid = form.username.trim() && form.auth_pass.length >= 8 && (!priv || form.priv_pass.length >= 8)

  const save = async () => {
    setBusy(true)
    try {
      await api.post('/sinks/v3users', {
        ...form,
        username: form.username.trim(),
        priv_pass: priv ? form.priv_pass : '',
        priv_protocol: priv ? form.priv_protocol : '',
      })
      toast.push('success', 'SNMPv3 user added.')
      invalidate('v3users')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not add user.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title="Add SNMPv3 user"
      description="Passwords are sealed on save and never shown again. noAuthNoPriv is not allowed."
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={save} loading={busy} disabled={!valid}>
            Add user
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Username">
          <Input value={form.username} onChange={(e) => set('username', e.target.value)} className="font-mono" autoFocus placeholder="sfvhuser" />
        </Field>
        <Field label="Security level">
          <Select value={form.security_level} onChange={(e) => set('security_level', e.target.value)}>
            <option value="authNoPriv">authNoPriv — authenticate only</option>
            <option value="authPriv">authPriv — authenticate + encrypt (recommended)</option>
          </Select>
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Auth protocol">
            <Select value={form.auth_protocol} onChange={(e) => set('auth_protocol', e.target.value)}>
              {AUTH_PROTOCOLS.map((p) => <option key={p} value={p}>{p}</option>)}
            </Select>
          </Field>
          <Field label="Auth password" hint="min 8 characters">
            <Input type="password" value={form.auth_pass} onChange={(e) => set('auth_pass', e.target.value)} placeholder="••••••••" />
          </Field>
        </div>
        {priv && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Privacy protocol">
              <Select value={form.priv_protocol} onChange={(e) => set('priv_protocol', e.target.value)}>
                {PRIV_PROTOCOLS.map((p) => <option key={p} value={p}>{p}</option>)}
              </Select>
            </Field>
            <Field label="Privacy password" hint="min 8 characters">
              <Input type="password" value={form.priv_pass} onChange={(e) => set('priv_pass', e.target.value)} placeholder="••••••••" />
            </Field>
          </div>
        )}
        <Field label="Engine ID" hint="optional — leave blank to auto-discover">
          <Input value={form.engine_id} onChange={(e) => set('engine_id', e.target.value)} className="font-mono" placeholder="" />
        </Field>
        <div className="flex items-center justify-between rounded-lg border border-border p-3">
          <span className="text-sm font-medium text-foreground">Enabled</span>
          <Switch checked={form.enabled} onChange={(v) => set('enabled', v)} aria-label="Enabled" />
        </div>
      </div>
    </Dialog>
  )
}

function DeleteV3User({ user, onClose }: { user: V3User | null; onClose: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const remove = async () => {
    if (!user) return
    setBusy(true)
    try {
      await api.del(`/sinks/v3users/${user.id}`)
      toast.push('success', 'User removed.')
      invalidate('v3users')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not delete user.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <ConfirmDialog
      open={user != null}
      onClose={onClose}
      onConfirm={remove}
      loading={busy}
      title="Delete SNMPv3 user"
      body={`Remove user "${user?.username}"? v3 traps using it will no longer be accepted.`}
    />
  )
}

function BindAddress() {
  const { data: settings, isLoading } = useSettings()
  const invalidate = useInvalidate()
  const toast = useToast()
  const [addr, setAddr] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (settings) setAddr(settings['snmp.bind_addr'] ?? '')
  }, [settings])

  const save = async () => {
    setBusy(true)
    try {
      await api.put('/settings', { 'snmp.bind_addr': addr.trim() })
      toast.push('success', 'Bind address saved.')
      invalidate('settings')
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not save bind address.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader title="Listener" description="Where the trap sink binds for incoming UDP." />
      <CardBody>
        {isLoading ? (
          <LoadingRow />
        ) : (
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
            <Field label="Bind address" hint="host:port" className="flex-1">
              <Input value={addr} onChange={(e) => setAddr(e.target.value)} className="font-mono" placeholder="0.0.0.0:1162" />
            </Field>
            <Button variant="primary" onClick={save} loading={busy}>
              <Save className="h-4 w-4" /> Save changes
            </Button>
          </div>
        )}
        <p className="mt-2 text-xs text-muted">
          Ports below 1024 need <Mono>NET_BIND_SERVICE</Mono>; the default maps host 162 to 1162 in
          Docker.
        </p>
      </CardBody>
    </Card>
  )
}

function CommunityRow({ community, onDelete }: { community: Community; onDelete: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const toggle = async (enabled: boolean) => {
    try {
      await api.put(`/sinks/communities/${community.id}`, { enabled })
      invalidate('communities')
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not update community.')
    }
  }
  return (
    <li className="flex items-center justify-between gap-3 px-5 py-3">
      <div className="flex items-center gap-3">
        <KeyRound className="h-4 w-4 text-muted" />
        <div>
          <p className="text-sm font-medium text-foreground">Community #{community.id}</p>
          <p className="text-xs text-muted">Value hidden — sealed at rest</p>
        </div>
      </div>
      <div className="flex items-center gap-3">
        {community.enabled ? <Badge tone="success">enabled</Badge> : <Badge tone="muted">disabled</Badge>}
        <Switch checked={community.enabled} onChange={toggle} aria-label="Enable community" />
        <Button variant="ghost" size="icon" onClick={onDelete} aria-label="Delete">
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </li>
  )
}

function AddCommunity({ onClose }: { onClose: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const [community, setCommunity] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [busy, setBusy] = useState(false)

  const save = async () => {
    setBusy(true)
    try {
      await api.post('/sinks/communities', { community: community.trim(), enabled })
      toast.push('success', 'Community added.')
      invalidate('communities')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not add community.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title="Add v2c community"
      description="The value is sealed on save and cannot be viewed afterward."
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={save} loading={busy} disabled={!community.trim()}>
            Add community
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Community string">
          <Input
            value={community}
            onChange={(e) => setCommunity(e.target.value)}
            className="font-mono"
            autoFocus
            placeholder="public"
          />
        </Field>
        <div className="flex items-center justify-between rounded-lg border border-border p-3">
          <span className="text-sm font-medium text-foreground">Enabled</span>
          <Switch checked={enabled} onChange={setEnabled} aria-label="Enabled" />
        </div>
      </div>
    </Dialog>
  )
}

function DeleteCommunity({ community, onClose }: { community: Community | null; onClose: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const remove = async () => {
    if (!community) return
    setBusy(true)
    try {
      await api.del(`/sinks/communities/${community.id}`)
      toast.push('success', 'Community removed.')
      invalidate('communities')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not delete community.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <ConfirmDialog
      open={community != null}
      onClose={onClose}
      onConfirm={remove}
      loading={busy}
      title="Delete community"
      body={`Remove community #${community?.id}? Traps using it will be dropped.`}
    />
  )
}
