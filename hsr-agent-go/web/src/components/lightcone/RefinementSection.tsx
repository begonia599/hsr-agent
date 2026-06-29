import { useMemo, useState } from 'react'
import type { LightconeRefinements } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { renderSkillDesc } from '@/lib/skillText'

// 光锥叠影:与角色技能等级滑杆同构。raw_zh->'refinements' 提供带占位符的 desc
// 和 level["1".."5"].param_list,拖动滑杆按叠影等级把数值填进 desc。
export function RefinementSection({ data }: { data: LightconeRefinements }) {
  const levels = useMemo(
    () => Object.keys(data.level ?? {}).sort((a, b) => Number(a) - Number(b)),
    [data.level],
  )
  const min = levels.length ? Number(levels[0]) : 1
  const max = levels.length ? Number(levels[levels.length - 1]) : 1
  const [level, setLevel] = useState(max)

  const params = data.level?.[String(level)]?.param_list ?? []

  return (
    <div className="rounded-lg bg-muted/40 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <Badge>叠影</Badge>
        {data.name && <span className="text-sm font-medium">{data.name}</span>}
        {max > min && (
          <span className="ml-auto text-xs text-muted-foreground">
            叠影 {level}/{max}
          </span>
        )}
      </div>
      {max > min && (
        <input
          type="range"
          min={min}
          max={max}
          value={level}
          onChange={(e) => setLevel(Number(e.target.value))}
          className="mt-2 h-1 w-full cursor-pointer accent-amber-500"
        />
      )}
      <p className="mt-3 text-sm leading-relaxed text-foreground/90">
        {renderSkillDesc(data.desc, params)}
      </p>
    </div>
  )
}
