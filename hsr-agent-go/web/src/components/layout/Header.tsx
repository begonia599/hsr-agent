import { NavLink } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { MessagesSquare, Users } from 'lucide-react'
import { api } from '@/api/client'
import type { HealthInfo } from '@/api/types'
import { cn } from '@/lib/utils'

export function Header() {
  const [health, setHealth] = useState<HealthInfo | null>(null)

  useEffect(() => {
    api.health().then(setHealth).catch(() => setHealth(null))
  }, [])

  const navItem = (to: string, icon: React.ReactNode, label: string) => (
    <NavLink
      to={to}
      className={({ isActive }) =>
        cn(
          'inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
          isActive ? 'bg-muted text-foreground' : 'text-muted-foreground hover:text-foreground',
        )
      }
    >
      {icon}
      {label}
    </NavLink>
  )

  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b border-border/60 px-4">
      <div className="flex items-center gap-3">
        <span className="font-serif text-base font-semibold tracking-tight">HSR Agent</span>
        <nav className="flex items-center gap-1">
          {navItem('/', <MessagesSquare className="size-4" />, '对话')}
          {navItem('/characters', <Users className="size-4" />, '角色')}
        </nav>
      </div>
      {health?.data?.version && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span
            className={cn(
              'size-1.5 rounded-full',
              health.database.status === 'ok' ? 'bg-emerald-500' : 'bg-destructive',
            )}
          />
          <span>v{health.data.version}</span>
          {health.llm?.model && <span className="font-mono">{health.llm.model}</span>}
        </div>
      )}
    </header>
  )
}
