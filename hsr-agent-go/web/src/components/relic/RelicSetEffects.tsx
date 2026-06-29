import { useMemo } from 'react'
import type { AxisEntry } from '@/api/types'
import {
  formatAxisValue,
  relicPieceLabel,
  relicPieceOrder,
  statLabel,
  targetLabel,
  uptimeLabel,
} from '@/lib/hsr'

function AxisRow({ entry }: { entry: AxisEntry }) {
  const value = formatAxisValue(entry.value)
  return (
    <li className="flex gap-2.5 rounded-lg bg-muted/40 px-3 py-2">
      <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-emerald-500/70" />
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
        </div>
        {entry.condition && (
          <p className="text-xs leading-relaxed text-muted-foreground">{entry.condition}</p>
        )}
        {entry.reason && (
          <p className="text-xs leading-relaxed text-muted-foreground">{entry.reason}</p>
        )}
      </div>
    </li>
  )
}

// 把 provides 按 source(2pc/4pc)分组;未标注 source 的归入「套装效果」。
function groupBySource(provides: AxisEntry[]): { label: string; rows: AxisEntry[] }[] {
  const buckets = new Map<string, AxisEntry[]>()
  for (const e of provides) {
    const key = e.source ?? ''
    if (!buckets.has(key)) buckets.set(key, [])
    buckets.get(key)!.push(e)
  }
  // 2件套 在前,4件套 在后,其余兜底
  return [...buckets.entries()]
    .sort((a, b) => relicPieceOrder(a[0]) - relicPieceOrder(b[0]))
    .map(([key, rows]) => ({ label: relicPieceLabel(key) || '套装效果', rows }))
}

export function RelicSetEffects({
  provides,
  needs,
}: {
  provides: AxisEntry[]
  needs: AxisEntry[]
}) {
  const groups = useMemo(() => groupBySource(provides), [provides])

  return (
    <div className="space-y-5">
      {groups.length === 0 ? (
        <p className="text-sm text-muted-foreground">暂无效果数据</p>
      ) : (
        groups.map((g) => (
          <div key={g.label} className="space-y-2">
            <h3 className="text-sm font-medium text-muted-foreground">{g.label}</h3>
            <ul className="space-y-1.5">
              {g.rows.map((e, i) => (
                <AxisRow key={i} entry={e} />
              ))}
            </ul>
          </div>
        ))
      )}

      {needs.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-medium text-muted-foreground">适配需求</h3>
          <ul className="space-y-1.5">
            {needs.map((e, i) => (
              <AxisRow key={i} entry={e} />
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
