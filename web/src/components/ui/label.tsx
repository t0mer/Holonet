import type { LabelHTMLAttributes, ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function Label({
  className,
  children,
  hint,
  ...props
}: LabelHTMLAttributes<HTMLLabelElement> & { hint?: ReactNode }) {
  return (
    <label
      className={cn('mb-1.5 flex items-center justify-between text-xs font-medium text-muted', className)}
      {...props}
    >
      <span>{children}</span>
      {hint && <span className="font-normal text-muted/70">{hint}</span>}
    </label>
  )
}

/** Field wraps a label + control with consistent spacing. */
export function Field({
  label,
  hint,
  htmlFor,
  children,
  className,
}: {
  label: ReactNode
  hint?: ReactNode
  htmlFor?: string
  children: ReactNode
  className?: string
}) {
  return (
    <div className={className}>
      <Label htmlFor={htmlFor} hint={hint}>
        {label}
      </Label>
      {children}
    </div>
  )
}
