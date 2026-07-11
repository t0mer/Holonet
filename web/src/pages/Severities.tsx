import { useState } from 'react'
import { Plus, Pencil, Trash2, Lock } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import type { Severity } from '@/lib/types'
import { useSeverities, useInvalidate } from '@/lib/queries'
import { useToast } from '@/lib/toast'
import { PageHeader, LoadingRow, ErrorNote } from '@/components/common'
import { SeverityBadge, BUILTIN_SEVERITY_COLORS } from '@/components/Severity'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field } from '@/components/ui/label'
import { Dialog } from '@/components/ui/dialog'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Table, THead, TBody, TR, TH, TD } from '@/components/ui/table'

const EMOJI_CHOICES = ['🔴', '🟠', '🟡', '🔵', '⚪', '🟣', '🟢', '⚫']

export function SeveritiesPage() {
  const { data: severities = [], isLoading, error } = useSeverities()
  const [editing, setEditing] = useState<Severity | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<Severity | null>(null)

  const sorted = severities.slice().sort((a, b) => a.rank - b.rank)

  return (
    <div>
      <PageHeader
        title="Severities"
        description="Priority levels drive routing and the signal colors across the console."
        action={
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4" /> Add level
          </Button>
        }
      />

      {error ? (
        <ErrorNote message={(error as Error).message} />
      ) : isLoading ? (
        <Card>
          <LoadingRow />
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <Table>
            <THead>
              <TR className="border-border">
                <TH>Rank</TH>
                <TH>Level</TH>
                <TH>Color</TH>
                <TH className="text-right">Actions</TH>
              </TR>
            </THead>
            <TBody>
              {sorted.map((s) => (
                <TR key={s.id}>
                  <TD className="w-16 font-mono text-muted">{s.rank}</TD>
                  <TD>
                    <div className="flex items-center gap-2">
                      <SeverityBadge name={s.name} color={s.color} emoji={s.emoji} />
                      {s.is_builtin && (
                        <span className="inline-flex items-center gap-1 text-xs text-muted">
                          <Lock className="h-3 w-3" /> built-in
                        </span>
                      )}
                    </div>
                  </TD>
                  <TD>
                    <span className="inline-flex items-center gap-2 font-mono text-xs text-muted">
                      <span
                        className="h-3.5 w-3.5 rounded border border-border"
                        style={{ backgroundColor: s.color || BUILTIN_SEVERITY_COLORS[s.name] }}
                      />
                      {s.color}
                    </span>
                  </TD>
                  <TD className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="icon" onClick={() => setEditing(s)} aria-label="Edit">
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setDeleting(s)}
                        disabled={s.is_builtin}
                        aria-label="Delete"
                        title={s.is_builtin ? 'Built-in levels cannot be deleted' : 'Delete'}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Card>
      )}

      {(editing || creating) && (
        <SeverityDialog severity={editing} onClose={() => { setEditing(null); setCreating(false) }} />
      )}
      <DeleteSeverity severity={deleting} onClose={() => setDeleting(null)} />
    </div>
  )
}

function SeverityDialog({ severity, onClose }: { severity: Severity | null; onClose: () => void }) {
  const isEdit = severity != null
  const invalidate = useInvalidate()
  const toast = useToast()
  const [name, setName] = useState(severity?.name ?? '')
  const [rank, setRank] = useState(severity?.rank ?? 100)
  const [color, setColor] = useState(severity?.color ?? '#22d3ee')
  const [emoji, setEmoji] = useState(severity?.emoji ?? '')
  const [busy, setBusy] = useState(false)

  const save = async () => {
    setBusy(true)
    try {
      const body = { name: name.trim(), rank, color, emoji, is_builtin: severity?.is_builtin ?? false }
      if (isEdit) await api.put(`/severities/${severity.id}`, body)
      else await api.post('/severities', body)
      toast.push('success', isEdit ? 'Severity updated.' : 'Severity added.')
      invalidate('severities', 'dashboard', 'traps')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not save severity.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title={isEdit ? 'Edit severity' : 'Add severity'}
      description={severity?.is_builtin ? 'Built-in level — rename and recolor, but it cannot be removed.' : undefined}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={save} loading={busy} disabled={!name.trim()}>
            Save changes
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <div className="rounded-lg border border-border bg-background p-3">
          <span className="mb-2 block text-xs text-muted">Preview</span>
          <SeverityBadge name={name || 'Level'} color={color} emoji={emoji || undefined} />
        </div>
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus placeholder="e.g. Urgent" />
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field label="Rank" hint="lower = higher priority">
            <Input type="number" value={rank} onChange={(e) => setRank(Number(e.target.value))} />
          </Field>
          <Field label="Color">
            <div className="flex items-center gap-2">
              <input
                type="color"
                value={color}
                onChange={(e) => setColor(e.target.value)}
                className="h-9 w-10 shrink-0 cursor-pointer rounded-md border border-border bg-background"
                aria-label="Pick color"
              />
              <Input value={color} onChange={(e) => setColor(e.target.value)} className="font-mono" />
            </div>
          </Field>
        </div>
        <Field label="Emoji" hint="optional signal glyph">
          <div className="flex flex-wrap items-center gap-1.5">
            {EMOJI_CHOICES.map((e) => (
              <button
                key={e}
                type="button"
                onClick={() => setEmoji(emoji === e ? '' : e)}
                className={`flex h-9 w-9 items-center justify-center rounded-md border text-lg transition ${
                  emoji === e ? 'border-holo bg-holo/10' : 'border-border hover:bg-surface-2'
                }`}
              >
                {e}
              </button>
            ))}
          </div>
        </Field>
      </div>
    </Dialog>
  )
}

function DeleteSeverity({ severity, onClose }: { severity: Severity | null; onClose: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const remove = async () => {
    if (!severity) return
    setBusy(true)
    try {
      await api.del(`/severities/${severity.id}`)
      toast.push('success', 'Severity removed.')
      invalidate('severities')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not delete severity.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <ConfirmDialog
      open={severity != null}
      onClose={onClose}
      onConfirm={remove}
      loading={busy}
      title="Delete severity"
      body={`Remove "${severity?.name}"? Rules and traps using it will fall back to defaults.`}
    />
  )
}
