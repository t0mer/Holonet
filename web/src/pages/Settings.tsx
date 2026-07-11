import { useEffect, useMemo, useState } from 'react'
import { Save, Waves, ShieldCheck, Radio, HelpCircle } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import type { Settings } from '@/lib/types'
import { useSettings, useSeverities, useInvalidate } from '@/lib/queries'
import { useToast } from '@/lib/toast'
import { PageHeader, LoadingRow, ErrorNote } from '@/components/common'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Field } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

// Only these keys are writable by the settings API (backend allow-list).
const STRATEGIES = [
  { value: 'none', label: 'None', hint: 'Every event dispatches immediately.' },
  { value: 'dedupe', label: 'Dedupe', hint: 'Suppress identical events within a window.' },
  { value: 'rate_limit', label: 'Rate limit', hint: 'Cap events per key, then summarize.' },
  { value: 'digest', label: 'Digest', hint: 'Roll events up into periodic summaries.' },
] as const

export function SettingsPage() {
  const { data: settings, isLoading, error } = useSettings()
  const { data: severities = [] } = useSeverities()
  const invalidate = useInvalidate()
  const toast = useToast()

  // Local working copy; we PUT only changed keys.
  const [form, setForm] = useState<Settings>({})
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (settings) setForm(settings)
  }, [settings])

  const changedKeys = useMemo(() => {
    if (!settings) return [] as string[]
    return Object.keys(form).filter((k) => form[k] !== settings[k])
  }, [form, settings])

  const set = (key: string, value: string) => setForm((f) => ({ ...f, [key]: value }))
  const get = (key: string, fallback = '') => form[key] ?? fallback

  const save = async () => {
    if (changedKeys.length === 0) return
    const patch: Settings = {}
    for (const k of changedKeys) patch[k] = form[k]
    setBusy(true)
    try {
      await api.put('/settings', patch)
      toast.push('success', 'Settings saved.')
      invalidate('settings')
    } catch (err) {
      toast.push('error', err instanceof ApiError ? err.message : 'Could not save settings.')
    } finally {
      setBusy(false)
    }
  }

  if (error) return <ErrorNote message={(error as Error).message} />
  if (isLoading) return <LoadingRow label="Loading settings…" />

  const strategy = get('flood.strategy', 'none')
  const authEnabled = get('auth.enabled', 'true') === 'true'

  return (
    <div>
      <PageHeader
        title="Settings"
        description="Flood control, defaults, and access — changes apply without a restart."
        action={
          <Button variant="primary" onClick={save} loading={busy} disabled={changedKeys.length === 0}>
            <Save className="h-4 w-4" /> Save changes
          </Button>
        }
      />

      <div className="space-y-6">
        <Card>
          <CardHeader
            title={<span className="flex items-center gap-2"><Waves className="h-4 w-4 text-holo" /> Flood control</span>}
            description="Choose how HoloNet suppresses noisy trap bursts."
          />
          <CardBody className="space-y-5">
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              {STRATEGIES.map((s) => (
                <button
                  key={s.value}
                  onClick={() => set('flood.strategy', s.value)}
                  className={cn(
                    'rounded-lg border p-3 text-left transition-colors',
                    strategy === s.value
                      ? 'border-holo bg-holo/10'
                      : 'border-border hover:bg-surface-2/60',
                  )}
                >
                  <span className={cn('text-sm font-medium', strategy === s.value ? 'text-holo' : 'text-foreground')}>
                    {s.label}
                  </span>
                  <p className="mt-1 text-xs text-muted">{s.hint}</p>
                </button>
              ))}
            </div>

            {strategy === 'dedupe' && (
              <Field label="Dedupe window" hint="duration, e.g. 30s" className="max-w-xs">
                <Input value={get('flood.dedupe_window', '30s')} onChange={(e) => set('flood.dedupe_window', e.target.value)} className="font-mono" />
              </Field>
            )}

            {strategy === 'rate_limit' && (
              <div className="grid max-w-md gap-4 sm:grid-cols-2">
                <Field label="Max per window" hint="integer">
                  <Input
                    type="number"
                    value={get('flood.rate_n', '10')}
                    onChange={(e) => set('flood.rate_n', e.target.value)}
                    className="font-mono"
                  />
                </Field>
                <Field label="Window" hint="duration, e.g. 1m">
                  <Input value={get('flood.rate_window', '1m')} onChange={(e) => set('flood.rate_window', e.target.value)} className="font-mono" />
                </Field>
              </div>
            )}

            {strategy === 'digest' && (
              <Field label="Digest interval" hint="duration, e.g. 5m" className="max-w-xs">
                <Input value={get('flood.digest_interval', '5m')} onChange={(e) => set('flood.digest_interval', e.target.value)} className="font-mono" />
              </Field>
            )}

            {strategy === 'none' && (
              <p className="flex items-center gap-2 text-sm text-muted">
                <HelpCircle className="h-4 w-4" /> No suppression — every classified trap is dispatched.
              </p>
            )}
          </CardBody>
        </Card>

        <Card>
          <CardHeader
            title={<span className="flex items-center gap-2"><Radio className="h-4 w-4 text-holo" /> Classification</span>}
            description="What happens to traps that no OID mapping or rule covers."
          />
          <CardBody>
            <Field label="Unknown-event default severity" hint="applied to unmapped traps" className="max-w-xs">
              <Select
                value={get('unknown_default_severity_id')}
                onChange={(e) => set('unknown_default_severity_id', e.target.value)}
              >
                <option value="">None</option>
                {severities.slice().sort((a, b) => a.rank - b.rank).map((s) => (
                  <option key={s.id} value={s.id}>{s.emoji ? `${s.emoji} ` : ''}{s.name}</option>
                ))}
              </Select>
            </Field>
          </CardBody>
        </Card>

        <Card>
          <CardHeader
            title={<span className="flex items-center gap-2"><ShieldCheck className="h-4 w-4 text-holo" /> Access</span>}
            description="Console authentication."
          />
          <CardBody>
            <div className="flex items-start justify-between gap-4 rounded-lg border border-border p-4">
              <div>
                <p className="text-sm font-medium text-foreground">Require sign-in</p>
                <p className="mt-1 max-w-md text-sm text-muted">
                  Keep this on unless the console sits behind a trusted proxy such as Cloudflare
                  Access. Turning it off leaves the API and UI open to anyone who can reach them.
                </p>
              </div>
              <Switch
                checked={authEnabled}
                onChange={(v) => set('auth.enabled', v ? 'true' : 'false')}
                aria-label="Require sign-in"
              />
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  )
}
