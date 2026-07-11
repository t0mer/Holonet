import { useState, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  Radio,
  GitBranch,
  Send,
  ListTree,
  SignalHigh,
  Antenna,
  Settings as SettingsIcon,
  Moon,
  Sun,
  LogOut,
  Menu,
  X,
} from 'lucide-react'
import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'
import { Wordmark, VersionTag } from './Wordmark'
import { Button } from './ui/button'

interface NavItem {
  to: string
  label: string
  icon: typeof Radio
}

const NAV: NavItem[] = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/events', label: 'Events', icon: Radio },
  { to: '/rules', label: 'Rules', icon: GitBranch },
  { to: '/channels', label: 'Channels', icon: Send },
  { to: '/oidmap', label: 'OID Map', icon: ListTree },
  { to: '/severities', label: 'Severities', icon: SignalHigh },
  { to: '/sinks', label: 'Sinks', icon: Antenna },
  { to: '/settings', label: 'Settings', icon: SettingsIcon },
]

interface LayoutProps {
  children: ReactNode
  version?: string
  showAuthControls: boolean
  onLogout: () => void
}

export function Layout({ children, version, showAuthControls, onLogout }: LayoutProps) {
  const [mobileOpen, setMobileOpen] = useState(false)
  const { theme, toggle } = useTheme()
  const location = useLocation()
  const current = NAV.find((n) => (n.to === '/' ? location.pathname === '/' : location.pathname.startsWith(n.to)))

  return (
    <div className="min-h-screen bg-background">
      {/* Sidebar — desktop */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 flex-col border-r border-border bg-surface lg:flex">
        <SidebarContent version={version} showAuthControls={showAuthControls} onLogout={onLogout} />
      </aside>

      {/* Sidebar — mobile drawer */}
      {mobileOpen && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={() => setMobileOpen(false)} />
          <aside className="absolute inset-y-0 left-0 flex w-64 flex-col border-r border-border bg-surface">
            <SidebarContent
              version={version}
              showAuthControls={showAuthControls}
              onLogout={onLogout}
              onNavigate={() => setMobileOpen(false)}
            />
          </aside>
        </div>
      )}

      {/* Main column */}
      <div className="lg:pl-60">
        {/* Top bar */}
        <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-border bg-background/80 px-4 backdrop-blur sm:px-6">
          <div className="flex items-center gap-3">
            <button
              className="text-muted transition hover:text-foreground lg:hidden"
              onClick={() => setMobileOpen(true)}
              aria-label="Open menu"
            >
              <Menu className="h-5 w-5" />
            </button>
            <span className="font-display text-sm font-semibold text-foreground">
              {current?.label ?? 'HoloNet'}
            </span>
          </div>
          <div className="flex items-center gap-1.5">
            <Button variant="ghost" size="icon" onClick={toggle} aria-label="Toggle theme">
              {theme === 'dark' ? <Sun className="h-[1.15rem] w-[1.15rem]" /> : <Moon className="h-[1.15rem] w-[1.15rem]" />}
            </Button>
            {showAuthControls && (
              <Button variant="ghost" size="sm" onClick={onLogout} className="hidden sm:inline-flex">
                <LogOut className="h-4 w-4" /> Sign out
              </Button>
            )}
          </div>
        </header>

        <main className="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6 sm:py-8">{children}</main>
      </div>
    </div>
  )
}

function SidebarContent({
  version,
  showAuthControls,
  onLogout,
  onNavigate,
}: {
  version?: string
  showAuthControls: boolean
  onLogout: () => void
  onNavigate?: () => void
}) {
  return (
    <>
      <div className="flex h-14 items-center justify-between border-b border-border px-4">
        <Wordmark />
        {onNavigate && (
          <button onClick={onNavigate} className="text-muted lg:hidden" aria-label="Close menu">
            <X className="h-5 w-5" />
          </button>
        )}
      </div>
      <nav className="flex-1 space-y-0.5 overflow-y-auto p-3">
        <p className="px-3 pb-2 pt-1 text-[0.65rem] font-semibold uppercase tracking-widest text-muted/60">
          Operations
        </p>
        {NAV.map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            onClick={onNavigate}
            className={({ isActive }) =>
              cn(
                'group flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-holo/10 text-holo'
                  : 'text-muted hover:bg-surface-2/70 hover:text-foreground',
              )
            }
          >
            {({ isActive }) => (
              <>
                <Icon className={cn('h-[1.15rem] w-[1.15rem] shrink-0', isActive && 'holo-glow')} />
                {label}
              </>
            )}
          </NavLink>
        ))}
      </nav>
      <div className="border-t border-border p-3">
        {showAuthControls && (
          <button
            onClick={onLogout}
            className="mb-2 flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted transition-colors hover:bg-surface-2/70 hover:text-foreground"
          >
            <LogOut className="h-[1.15rem] w-[1.15rem]" /> Sign out
          </button>
        )}
        <div className="flex items-center justify-between px-3">
          <span className="text-[0.7rem] text-muted">SNMP trap console</span>
          <VersionTag version={version} />
        </div>
      </div>
    </>
  )
}
