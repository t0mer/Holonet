import { useMemo, useState } from 'react'
import { Plus, Pencil, Trash2, Search, ListTree } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import type { OIDEntry, Severity } from '@/lib/types'
import { useOIDMap, useSeverities, useInvalidate } from '@/lib/queries'
import { useToast } from '@/lib/toast'
import { PageHeader, LoadingRow, ErrorNote, EmptyState, Mono } from '@/components/common'
import { SeverityBadge } from '@/components/Severity'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Field } from '@/components/ui/label'
import { Dialog } from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Table, THead, TBody, TR, TH, TD } from '@/components/ui/table'

export function OIDMapPage() {
  const { data: entries = [], isLoading, error } = useOIDMap()
  const { data: severities = [] } = useSeverities()
  const [query, setQuery] = useState('')
  const [editing, setEditing] = useState<OIDEntry | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<OIDEntry | null>(null)

  const sevById = useMemo(() => new Map(severities.map((s) => [s.id, s])), [severities])
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return entries
    return entries.filter((e) => e.oid.toLowerCase().includes(q) || e.name.toLowerCase().includes(q))
  }, [entries, query])

  return (
    <div>
      <PageHeader
        title="OID Map"
        description="Resolve incoming trap OIDs to readable names and a default severity."
        action={
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4" /> Add OID
          </Button>
        }
      />

      <div className="relative mb-4 max-w-sm">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted" />
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Filter by OID or name…"
          className="pl-9"
        />
      </div>

      {error ? (
        <ErrorNote message={(error as Error).message} />
      ) : isLoading ? (
        <Card>
          <LoadingRow />
        </Card>
      ) : filtered.length === 0 ? (
        <Card>
          <EmptyState
            icon={<ListTree className="h-8 w-8" />}
            title={query ? 'No matches' : 'No OIDs mapped yet'}
            description={query ? 'Try a different search term.' : 'Import your device MIB or add entries to name incoming traps.'}
          />
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <Table>
            <THead>
              <TR className="border-border">
                <TH>OID</TH>
                <TH>Name</TH>
                <TH>Default severity</TH>
                <TH className="text-right">Actions</TH>
              </TR>
            </THead>
            <TBody>
              {filtered.map((e) => (
                <TR key={e.id}>
                  <TD className="whitespace-nowrap">
                    <Mono className="text-muted">{e.oid}</Mono>
                  </TD>
                  <TD>
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{e.name}</span>
                      {e.is_builtin && <Badge tone="muted">built-in</Badge>}
                    </div>
                    {e.description && <p className="mt-0.5 text-xs text-muted">{e.description}</p>}
                  </TD>
                  <TD>
                    {e.default_severity_id != null ? (
                      <SeverityBadge
                        name={sevById.get(e.default_severity_id)?.name ?? null}
                        color={sevById.get(e.default_severity_id)?.color}
                        emoji={sevById.get(e.default_severity_id)?.emoji}
                      />
                    ) : (
                      <span className="text-sm text-muted">—</span>
                    )}
                  </TD>
                  <TD className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="icon" onClick={() => setEditing(e)} aria-label="Edit">
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button variant="ghost" size="icon" onClick={() => setDeleting(e)} aria-label="Delete">
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
        <OIDDialog entry={editing} severities={severities} onClose={() => { setEditing(null); setCreating(false) }} />
      )}
      <DeleteOID entry={deleting} onClose={() => setDeleting(null)} />
    </div>
  )
}

function OIDDialog({
  entry,
  severities,
  onClose,
}: {
  entry: OIDEntry | null
  severities: Severity[]
  onClose: () => void
}) {
  const isEdit = entry != null
  const invalidate = useInvalidate()
  const toast = useToast()
  const [oid, setOid] = useState(entry?.oid ?? '')
  const [name, setName] = useState(entry?.name ?? '')
  const [description, setDescription] = useState(entry?.description ?? '')
  const [severityId, setSeverityId] = useState<string>(
    entry?.default_severity_id != null ? String(entry.default_severity_id) : '',
  )
  const [busy, setBusy] = useState(false)

  const save = async () => {
    setBusy(true)
    try {
      const body = {
        oid: oid.trim(),
        name: name.trim(),
        description: description.trim(),
        default_severity_id: severityId ? Number(severityId) : null,
        is_builtin: entry?.is_builtin ?? false,
      }
      if (isEdit) await api.put(`/oidmap/${entry.id}`, body)
      else await api.post('/oidmap', body)
      toast.push('success', isEdit ? 'OID updated.' : 'OID added.')
      invalidate('oidmap')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not save OID.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title={isEdit ? 'Edit OID' : 'Add OID'}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={save} loading={busy} disabled={!oid.trim() || !name.trim()}>
            Save changes
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="OID">
          <Input
            value={oid}
            onChange={(e) => setOid(e.target.value)}
            className="font-mono"
            placeholder="1.3.6.1.6.3.1.1.5.3"
            disabled={isEdit && entry.is_builtin}
          />
        </Field>
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="linkDown" />
        </Field>
        <Field label="Description" hint="optional">
          <Textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="min-h-16 font-sans"
            placeholder="What this trap indicates"
          />
        </Field>
        <Field label="Default severity" hint="applied when no rule overrides">
          <Select value={severityId} onChange={(e) => setSeverityId(e.target.value)}>
            <option value="">Use unknown-event default</option>
            {severities.slice().sort((a, b) => a.rank - b.rank).map((s) => (
              <option key={s.id} value={s.id}>
                {s.emoji ? `${s.emoji} ` : ''}{s.name}
              </option>
            ))}
          </Select>
        </Field>
      </div>
    </Dialog>
  )
}

function DeleteOID({ entry, onClose }: { entry: OIDEntry | null; onClose: () => void }) {
  const invalidate = useInvalidate()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const remove = async () => {
    if (!entry) return
    setBusy(true)
    try {
      await api.del(`/oidmap/${entry.id}`)
      toast.push('success', 'OID removed.')
      invalidate('oidmap')
      onClose()
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not delete OID.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <ConfirmDialog
      open={entry != null}
      onClose={onClose}
      onConfirm={remove}
      loading={busy}
      title="Delete OID mapping"
      body={`Remove the mapping for ${entry?.oid}? Traps with this OID will show as unmapped.`}
    />
  )
}
