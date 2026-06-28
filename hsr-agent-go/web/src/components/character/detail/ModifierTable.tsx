import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import type { Modifier } from '@/api/types'
import { cn } from '@/lib/utils'
import { formatAxisValue, statLabel, targetLabel, zoneLabel } from '@/lib/hsr'

function valueText(m: Modifier): string {
  if (m.value == null) return '—'
  if (m.value_unit === 'percent') return formatAxisValue(m.value)
  if (m.value_unit === 'flat') return String(m.value)
  return String(m.value)
}

export function ModifierTable({ modifiers }: { modifiers: Modifier[] }) {
  const [open, setOpen] = useState(false)
  if (modifiers.length === 0) return null

  const unreviewed = modifiers.filter((m) => !m.reviewed).length

  return (
    <div className="rounded-xl bg-card ring-1 ring-foreground/10">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-4 py-3 text-left"
      >
        <ChevronRight className={cn('size-4 transition-transform', open && 'rotate-90')} />
        <span className="font-serif text-lg font-semibold tracking-tight">机制效果</span>
        <span className="text-xs text-muted-foreground">{modifiers.length} 条</span>
        {unreviewed > 0 && (
          <span className="ml-auto rounded bg-amber-500/15 px-1.5 py-0.5 text-[0.7rem] text-amber-600 dark:text-amber-400">
            {unreviewed} 条待校验
          </span>
        )}
      </button>
      {open && (
        <div className="overflow-x-auto border-t border-border/60 px-4 py-3">
          <p className="mb-2 text-xs text-muted-foreground">
            由 LLM 从技能文本抽取，用于机制/数值校验。标注「待校验」的条目尚未经人工复核，仅供参考。
          </p>
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-muted-foreground">
                <th className="py-1.5 pr-3 font-medium">效果</th>
                <th className="py-1.5 pr-3 font-medium">数值</th>
                <th className="py-1.5 pr-3 font-medium">作用</th>
                <th className="py-1.5 pr-3 font-medium">乘区</th>
                <th className="py-1.5 pr-3 font-medium">来源</th>
                <th className="py-1.5 font-medium">条件</th>
              </tr>
            </thead>
            <tbody>
              {modifiers.map((m, i) => (
                <tr key={i} className="border-t border-border/40 align-top">
                  <td className="py-1.5 pr-3">
                    <span className="font-medium">{statLabel(m.stat_key)}</span>
                    {!m.reviewed && (
                      <span className="ml-1 text-[0.65rem] text-amber-600 dark:text-amber-400">
                        待校验
                      </span>
                    )}
                  </td>
                  <td className="py-1.5 pr-3 whitespace-nowrap">{valueText(m)}</td>
                  <td className="py-1.5 pr-3 whitespace-nowrap text-muted-foreground">
                    {targetLabel(m.target_scope)}
                  </td>
                  <td className="py-1.5 pr-3 whitespace-nowrap text-muted-foreground">
                    {zoneLabel(m.modifier_zone)}
                  </td>
                  <td className="py-1.5 pr-3 whitespace-nowrap text-muted-foreground">
                    {m.source_name_zh}
                  </td>
                  <td className="py-1.5 text-muted-foreground">{m.condition_text || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
