import { useEffect, useState } from 'react'
import { Plus, Trash2, KeyRound, Save, Lock } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import type { Community } from '@/lib/types'
import { useCommunities, useSettings, useInvalidate } from '@/lib/queries'
import { useToast } from '@/lib/toast'
import { PageHeader, LoadingRow, ErrorNote, EmptyState, Mono } from '@/components/common'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
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

        <Card>
          <CardBody className="flex items-start gap-3">
            <Lock className="mt-0.5 h-4 w-4 shrink-0 text-muted" />
            <p className="text-sm text-muted">
              SNMPv3 users (USM auth/priv credentials) arrive in a later release. For now, use v2c
              communities above.
            </p>
          </CardBody>
        </Card>
      </div>

      {adding && <AddCommunity onClose={() => setAdding(false)} />}
      <DeleteCommunity community={deleting} onClose={() => setDeleting(null)} />
    </div>
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
