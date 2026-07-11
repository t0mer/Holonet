import { useState, type FormEvent } from 'react'
import { ShieldCheck, KeyRound, ArrowRight } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { Wordmark } from '@/components/Wordmark'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field } from '@/components/ui/label'

function AuthShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-background px-4">
      <div className="grid-backdrop pointer-events-none absolute inset-0 opacity-40" aria-hidden />
      <div className="pointer-events-none absolute left-1/2 top-1/3 h-72 w-72 -translate-x-1/2 rounded-full bg-holo/10 blur-3xl" aria-hidden />
      <div className="relative w-full max-w-sm">{children}</div>
    </div>
  )
}

/** First-run setup wizard: create the admin account (design §5). */
export function SetupScreen({ onDone }: { onDone: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (password.length < 8) {
      setError('Choose a password of at least 8 characters.')
      return
    }
    if (password !== confirm) {
      setError('Passwords do not match.')
      return
    }
    setBusy(true)
    try {
      await api.post('/auth/setup', { username: username.trim(), password })
      onDone()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not complete setup.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell>
      <div className="mb-6 flex flex-col items-center gap-3 text-center">
        <Wordmark />
        <div>
          <h1 className="text-xl font-bold text-foreground">Set up HoloNet</h1>
          <p className="mt-1 text-sm text-muted">
            Create the administrator account to secure this console.
          </p>
        </div>
      </div>
      <form onSubmit={submit} className="space-y-4 rounded-xl border border-border bg-surface p-6">
        <div className="mb-1 flex items-center gap-2 text-holo">
          <ShieldCheck className="h-4 w-4" />
          <span className="text-xs font-medium">First-run setup</span>
        </div>
        <Field label="Username">
          <Input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            required
            autoFocus
            placeholder="admin"
          />
        </Field>
        <Field label="Password" hint="min 8 characters">
          <Input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
            required
          />
        </Field>
        <Field label="Confirm password">
          <Input
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            required
          />
        </Field>
        {error && <p className="text-sm text-red-400">{error}</p>}
        <Button type="submit" variant="primary" className="w-full justify-center" loading={busy}>
          Create account <ArrowRight className="h-4 w-4" />
        </Button>
      </form>
    </AuthShell>
  )
}

/** Login screen for a configured, auth-enabled instance. */
export function LoginScreen({ onDone }: { onDone: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await api.post('/auth/login', { username: username.trim(), password })
      onDone()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not sign in.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell>
      <div className="mb-6 flex flex-col items-center gap-3 text-center">
        <Wordmark />
        <div>
          <h1 className="text-xl font-bold text-foreground">Sign in</h1>
          <p className="mt-1 text-sm text-muted">Enter your credentials to reach the console.</p>
        </div>
      </div>
      <form onSubmit={submit} className="space-y-4 rounded-xl border border-border bg-surface p-6">
        <div className="mb-1 flex items-center gap-2 text-holo">
          <KeyRound className="h-4 w-4" />
          <span className="text-xs font-medium">Authentication</span>
        </div>
        <Field label="Username">
          <Input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            required
            autoFocus
          />
        </Field>
        <Field label="Password">
          <Input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
        </Field>
        {error && <p className="text-sm text-red-400">{error}</p>}
        <Button type="submit" variant="primary" className="w-full justify-center" loading={busy}>
          Sign in <ArrowRight className="h-4 w-4" />
        </Button>
      </form>
    </AuthShell>
  )
}
