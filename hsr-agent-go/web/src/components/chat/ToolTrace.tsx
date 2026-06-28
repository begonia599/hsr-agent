import { useState } from 'react'
import { ChevronRight, Wrench } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface ToolStep {
  name: string
  args?: unknown
  result?: unknown
}

// 工具名 → 中文说明,让用户看懂 agent 在干嘛
const TOOL_LABELS: Record<string, string> = {
  get_character: '查角色资料',
  search_by_role: '按定位筛选',
  find_needs: '分析角色需求',
  find_buffers_for: '查增益来源',
  find_synergies: '找协同角色',
  suggest_team: '生成配队方案',
  co_occurrence: '查队伍共现',
  recommend_lightcones: '推荐光锥',
  recommend_relics: '推荐遗器',
  semantic_search: '语义检索',
  keyword_search: '关键词检索',
  list_character_modifiers: '查机制效果',
  explain_modifier_sources: '溯源效果来源',
  compare_character_fit: '契合度对比',
  estimate_damage_gain: '估算伤害收益',
  estimate_dot_damage: '估算持续伤害',
  estimate_break_damage: '估算击破伤害',
  estimate_super_break_damage: '估算超击破',
  estimate_healing: '估算治疗量',
  estimate_shield: '估算护盾量',
  estimate_uptime: '估算覆盖率',
}

function toolLabel(name: string): string {
  return TOOL_LABELS[name] ?? name
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
