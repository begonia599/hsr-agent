import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import type { Character } from '@/api/types'
import { elementLabel, ELEMENT_COLORS, pathLabel, roleLabel, roundIconUrl } from '@/lib/hsr'

export function CharacterCard({ char }: { char: Character }) {
  const color = ELEMENT_COLORS[char.element] ?? 'var(--muted-foreground)'
  return (
    <Link
      to={`/characters/${char.id}`}
      className="group flex flex-col items-center gap-2 rounded-xl bg-card p-3 ring-1 ring-foreground/10 transition-shadow hover:ring-foreground/25"
    >
      <div
        className="relative size-16 overflow-hidden rounded-full ring-2"
        style={{ ['--tw-ring-color' as string]: color }}
      >
        <img
          src={roundIconUrl(char.id)}
          alt={char.name_zh}
          loading="lazy"
          className="size-full object-cover"
          onError={(e) => {
            ;(e.target as HTMLImageElement).style.visibility = 'hidden'
          }}
        />
      </div>
      <div className="flex flex-col items-center gap-1 text-center">
        <span className="text-sm font-medium leading-tight group-hover:text-foreground">
          {char.name_zh}
        </span>
        <div className="flex items-center gap-1">
          <span className="text-xs" style={{ color }}>
            {elementLabel(char.element)}
          </span>
          <span className="text-xs text-muted-foreground">·</span>
          <span className="text-xs text-muted-foreground">{pathLabel(char.path)}</span>
        </div>
        {char.roles.length > 0 && (
          <div className="mt-0.5 flex flex-wrap justify-center gap-1">
            {char.roles.slice(0, 2).map((r) => (
              <Badge key={r} className="px-1 py-0 text-[0.65rem]">
                {roleLabel(r)}
              </Badge>
            ))}
          </div>
        )}
      </div>
    </Link>
  )
}
