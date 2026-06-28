import { useMemo, useState } from 'react'
import type { CharacterSkills, Skill } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { renderSkillDesc } from '@/lib/skillText'

// 技能类型展示顺序与中文兜底(type_name 为空时用 type 映射)
const TYPE_ORDER: Record<string, number> = {
  Normal: 0,
  BPSkill: 1,
  Ultra: 2,
  Talent: 3,
  MazeNormal: 4,
  Maze: 5,
}
const TYPE_FALLBACK: Record<string, string> = {
  Normal: '普攻',
  BPSkill: '战技',
  Ultra: '终结技',
  Talent: '天赋',
  Maze: '秘技',
  MazeNormal: '普攻',
}

function maxLevel(skill: Skill): number {
  const keys = Object.keys(skill.level ?? {})
  return keys.length || 1
}

function SkillCard({ skill }: { skill: Skill }) {
  const max = maxLevel(skill)
  const [level, setLevel] = useState(max)
  const params = skill.level?.[String(level)]?.param_list ?? []
  const typeName = skill.type_name || TYPE_FALLBACK[skill.type] || skill.type

  return (
    <div className="rounded-lg bg-muted/40 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge>{typeName}</Badge>
        <span className="text-sm font-medium">{skill.name}</span>
        {max > 1 && (
          <span className="ml-auto text-xs text-muted-foreground">
            等级 {level}/{max}
          </span>
        )}
      </div>
      {max > 1 && (
        <input
          type="range"
          min={1}
          max={max}
          value={level}
          onChange={(e) => setLevel(Number(e.target.value))}
          className="mt-2 h-1 w-full cursor-pointer accent-amber-500"
        />
      )}
      <p className="mt-2 text-sm leading-relaxed text-foreground/90">
        {renderSkillDesc(skill.desc, params)}
      </p>
    </div>
  )
}

function EidolonList({ ranks }: { ranks: CharacterSkills['ranks'] }) {
  const ordered = Object.keys(ranks)
    .sort((a, b) => Number(a) - Number(b))
    .map((k) => ranks[k])
  if (ordered.length === 0) return null
  return (
    <div className="space-y-2">
      {ordered.map((rank, i) => (
        <div key={i} className="rounded-lg bg-muted/40 p-3">
          <div className="flex items-center gap-2">
            <span className="flex size-5 items-center justify-center rounded-full bg-background text-xs font-medium">
              {i + 1}
            </span>
            <span className="text-sm font-medium">{rank.name}</span>
          </div>
          <p className="mt-1.5 text-sm leading-relaxed text-foreground/90">
            {renderSkillDesc(rank.desc, rank.param_list)}
          </p>
        </div>
      ))}
    </div>
  )
}

export function SkillsSection({ data }: { data: CharacterSkills }) {
  const skills = useMemo(() => {
    return Object.values(data.skills ?? {}).sort((a, b) => {
      const oa = TYPE_ORDER[a.type] ?? 99
      const ob = TYPE_ORDER[b.type] ?? 99
      return oa - ob || a.id - b.id
    })
  }, [data.skills])

  return (
    <div className="grid gap-5 lg:grid-cols-2">
      <div className="space-y-2.5">
        <h3 className="text-sm font-medium text-muted-foreground">技能</h3>
        {skills.map((s) => (
          <SkillCard key={s.id} skill={s} />
        ))}
      </div>
      <div className="space-y-2.5">
        <h3 className="text-sm font-medium text-muted-foreground">星魂</h3>
        <EidolonList ranks={data.ranks} />
      </div>
    </div>
  )
}
