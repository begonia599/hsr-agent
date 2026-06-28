import * as React from 'react'

export function Section({
  title,
  count,
  children,
}: {
  title: string
  count?: number
  children: React.ReactNode
}) {
  return (
    <section className="space-y-3">
      <div className="flex items-baseline gap-2">
        <h2 className="font-serif text-lg font-semibold tracking-tight">{title}</h2>
        {count != null && <span className="text-xs text-muted-foreground">{count}</span>}
      </div>
      {children}
    </section>
  )
}
