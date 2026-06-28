import { Link } from 'react-router-dom'
import type { Synergy, TeamPlan } from '@/api/types'
import { elementLabel, pathLabel, roleLabel, roundIconUrl } from '@/lib/hsr'

function MiniAvatar({ id, name }: { id: number; name: string }) {
  return (
    <Link
      to={`/characters/${id}`}
      className="flex w-14 shrink-0 flex-col items-center gap-1 text-center"
      title={name}
    >
      <img
        src={roundIconUrl(id)}
        alt={name}
        loading="lazy"
        className="size-11 rounded-full bg-muted object-cover ring-1 ring-foreground/10 transition hover:ring-foreground/30"
        onError={(e) => {
          ;(e.target as HTMLImageElement).style.visibility = 'hidden'
        }}
      />
      <span className="truncate text-[0.7rem] leading-tight text-muted-foreground">{name}</span>
    </Link>
  )
}

export function SynergyList({ synergies }: { synergies: Synergy[] }) {
  if (synergies.length === 0) return <p className="text-sm text-muted-foreground">暂无</p>
  return (
    <ul className="space-y-2">
      {synergies.map((s) => (
        <li key={s.char_id} className="flex gap-3 rounded-lg bg-muted/40 px-3 py-2">
          <MiniAvatar id={s.char_id} name={s.name_zh} />
          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex items-center gap-2 text-sm">
              <Link to={`/characters/${s.char_id}`} className="font-medium hover:underline">
                {s.name_zh}
              </Link>
              <span className="text-xs text-muted-foreground">
                {elementLabel(s.element)} · {pathLabel(s.path)}
              </span>
              <span className="ml-auto text-xs text-muted-foreground">
                {s.roles.map(roleLabel).join('/')}
              </span>
            </div>
            {s.reasons.length > 0 && (
              <p className="text-xs leading-relaxed text-muted-foreground">
                {s.reasons.join('；')}
              </p>
            )}
          </div>
        </li>
      ))}
    </ul>
  )
}

export function TeamList({ teams }: { teams: TeamPlan[] }) {
  if (teams.length === 0) return <p className="text-sm text-muted-foreground">暂无</p>
  return (
    <div className="space-y-3">
      {teams.map((team, i) => (
        <div key={i} className="rounded-lg bg-muted/40 p-3">
          <div className="flex flex-wrap items-start gap-2">
            <div className="flex flex-col items-center gap-1">
              <MiniAvatar id={team.core_id} name={team.core_name_zh} />
              <span className="text-[0.65rem] text-muted-foreground">核心</span>
            </div>
            <div className="mt-1 flex flex-wrap gap-1">
              {team.members.map((m) => (
                <MiniAvatar key={m.char_id} id={m.char_id} name={m.name_zh} />
              ))}
            </div>
          </div>
          {team.notes.length > 0 && (
            <p className="mt-2 text-xs leading-relaxed text-muted-foreground">{team.notes[0]}</p>
          )}
        </div>
      ))}
    </div>
  )
}
