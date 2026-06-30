import { useState } from 'react'
import { ChevronRight, Wrench } from 'lucide-react'
import { cn } from '@/lib/utils'
import { toolLabel } from '@/lib/toolMeta'

export interface ToolStep {
  name: string
  args?: unknown
  result?: unknown
  toolCallId?: string
}

export function ToolTrace({ steps, busy }: { steps: ToolStep[]; busy: boolean }) {
  const [open, setOpen] = useState(false)
  if (steps.length === 0) return null

  return (
    <div className="mb-2 rounded-lg border border-border/70 bg-muted/40 text-xs">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-1.5 px-2.5 py-1.5 text-muted-foreground transition-colors hover:text-foreground"
      >
        <ChevronRight className={cn('size-3.5 transition-transform', open && 'rotate-90')} />
        <Wrench className="size-3.5" />
        <span className="font-medium">
          {busy ? '正在调用工具' : '调用了'} {steps.length} 个工具
        </span>
        {busy && (
          <span className="ml-1 inline-flex gap-0.5">
            <span className="size-1 animate-pulse rounded-full bg-current" />
            <span className="size-1 animate-pulse rounded-full bg-current [animation-delay:150ms]" />
            <span className="size-1 animate-pulse rounded-full bg-current [animation-delay:300ms]" />
          </span>
        )}
      </button>
      {open && (
        <div className="space-y-2 border-t border-border/70 px-2.5 py-2">
          {steps.map((step, i) => (
            <div key={i} className="space-y-1">
              <div className="flex items-center gap-1.5">
                <span className="text-muted-foreground">{i + 1}.</span>
                <span className="font-medium text-foreground">{toolLabel(step.name)}</span>
                <span className="font-mono text-[0.7rem] text-muted-foreground">{step.name}</span>
              </div>
              {step.args != null && (
                <pre className="overflow-x-auto rounded bg-background/60 px-2 py-1 font-mono text-[0.7rem] text-muted-foreground">
                  {JSON.stringify(step.args)}
                </pre>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
