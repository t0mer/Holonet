import type { Severity, TrapView } from '@/lib/types'
import { cn } from '@/lib/utils'

// Built-in severity defaults (design §3.3). The API drives per-severity name,
// color, and emoji; these are the fallback when a value is missing.
export const BUILTIN_SEVERITY_COLORS: Record<string, string> = {
  Critical: '#dc2626',
  High: '#ea580c',
  Medium: '#d97706',
  Low: '#2563eb',
  Info: '#6b7280',
}

interface SeverityBadgeProps {
  name: string | null | undefined
  color?: string | null
  emoji?: string | null
  /** Critical events get a subtle pulse to draw the eye. */
  pulse?: boolean
  className?: string
}

/**
 * The severity signal — HoloNet's signature element. A dot in the severity's
 * own color plus its label, tinted from the same color at low opacity so the
 * chip reads as "that severity" at a glance.
 */
export function SeverityBadge({ name, color, emoji, pulse, className }: SeverityBadgeProps) {
  if (!name) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full border border-border px-2 py-0.5 text-xs text-muted">
        Unclassified
      </span>
    )
  }
  const c = color || BUILTIN_SEVERITY_COLORS[name] || '#6b7280'
  const isCritical = pulse ?? name.toLowerCase() === 'critical'
  return (
    <span
      className={cn('inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium', className)}
      style={{
        color: c,
        borderColor: `color-mix(in srgb, ${c} 45%, transparent)`,
        backgroundColor: `color-mix(in srgb, ${c} 12%, transparent)`,
      }}
    >
      {emoji ? (
        <span aria-hidden>{emoji}</span>
      ) : (
        <span
          aria-hidden
          className={cn('h-1.5 w-1.5 rounded-full', isCritical && 'sev-pulse')}
          style={{ backgroundColor: c }}
        />
      )}
      {name}
    </span>
  )
}

/** The color used for a trap row's left accent bar. */
export function trapSeverityColor(t: TrapView): string {
  return t.severity_color || (t.severity_name ? BUILTIN_SEVERITY_COLORS[t.severity_name] : '') || '#334155'
}

/** Look up a severity's color by id, for selects and previews. */
export function severityColor(sev: Severity | undefined): string {
  if (!sev) return '#6b7280'
  return sev.color || BUILTIN_SEVERITY_COLORS[sev.name] || '#6b7280'
}
