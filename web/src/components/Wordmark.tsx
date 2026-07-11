import { cn } from '@/lib/utils'

/** The holographic-cyan HoloNet wordmark — a signal node with orbiting ring. */
export function Wordmark({ collapsed = false }: { collapsed?: boolean }) {
  return (
    <div className="flex items-center gap-2.5">
      <span className="relative flex h-8 w-8 shrink-0 items-center justify-center">
        <span className="absolute inset-0 rounded-lg border border-holo/40" />
        <span className="absolute inset-1.5 rounded-full border border-holo/30" />
        <span className="h-2 w-2 rounded-full bg-holo holo-glow" />
      </span>
      {!collapsed && (
        <span className="font-display text-lg font-bold tracking-tight text-foreground">
          Holo<span className="text-holo holo-glow">Net</span>
        </span>
      )}
    </div>
  )
}

/** Small version tag shown under the wordmark / in the footer. */
export function VersionTag({ version, className }: { version?: string; className?: string }) {
  if (!version) return null
  return (
    <span className={cn('font-mono text-[0.7rem] text-muted', className)}>v{version}</span>
  )
}
