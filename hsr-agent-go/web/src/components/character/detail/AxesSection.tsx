import type { AxisEntry } from '@/api/types'
import { formatAxisValue, statLabel, targetLabel, uptimeLabel } from '@/lib/hsr'

function AxisRow({ entry, tone }: { entry: AxisEntry; tone: 'provide' | 'need' }) {
  const value = formatAxisValue(entry.value)
  const dot = tone === 'provide' ? 'bg-emerald-500/70' : 'bg-amber-500/70'
  return (
    <li className="flex gap-2.5 rounded-lg bg-muted/40 px-3 py-2">
      <span className={`mt-1.5 size-1.5 shrink-0 rounded-full ${dot}`} />
      <div className="min-w-0 space-y-0.5">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-sm">
          <span className="font-medium">{statLabel(entry.stat)}</span>
          {value && <span className="text-foreground/80">{value}</span>}
          {entry.target && (
            <span className="text-xs text-muted-foreground">→ {targetLabel(entry.target)}</span>
          )}
          {entry.uptime && (
            <span className="rounded bg-background px-1 text-[0.7rem] text-muted-foreground">
              {uptimeLabel(entry.uptime)}
            </span>
          )}
          {entry.source && (
            <span className="text-[0.7rem] text-muted-foreground">[{entry.source}]</span>
          )}
        </div>
        {entry.reason && (
          <p className="text-xs leading-relaxed text-muted-foreground">{entry.reason}</p>
        )}
      </div>
    </li>
  )
}

export function AxesSection({
  provides,
  needs,
}: {
  provides: AxisEntry[]
  needs: AxisEntry[]
}) {
  return (
    <div className="grid gap-5 md:grid-cols-2">
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-muted-foreground">提供的增益</h3>
        {provides.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无</p>
        ) : (
          <ul className="space-y-1.5">
            {provides.map((e, i) => (
              <AxisRow key={i} entry={e} tone="provide" />
            ))}
          </ul>
        )}
      </div>
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-muted-foreground">队伍/配装需求</h3>
        {needs.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无</p>
        ) : (
          <ul className="space-y-1.5">
            {needs.map((e, i) => (
              <AxisRow key={i} entry={e} tone="need" />
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
