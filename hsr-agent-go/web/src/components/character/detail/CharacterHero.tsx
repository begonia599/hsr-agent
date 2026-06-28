import { Badge } from '@/components/ui/badge'
import type { Character } from '@/api/types'
import {
  drawCardUrl,
  elementLabel,
  ELEMENT_COLORS,
  pathLabel,
  rarityStars,
  roleLabel,
  tagLabel,
} from '@/lib/hsr'

export function CharacterHero({ char }: { char: Character }) {
  const color = ELEMENT_COLORS[char.element] ?? 'var(--muted-foreground)'
  const tags = char.axes?.tags ?? []
  return (
    <div className="flex flex-col gap-5 sm:flex-row">
      <div className="mx-auto w-44 shrink-0 sm:mx-0">
        <div className="aspect-3/4 overflow-hidden rounded-xl bg-muted ring-1 ring-foreground/10">
          <img
            src={drawCardUrl(char.id)}
            alt={char.name_zh}
            className="size-full object-cover object-top"
            onError={(e) => {
              ;(e.target as HTMLImageElement).style.visibility = 'hidden'
            }}
          />
        </div>
      </div>

      <div className="flex flex-1 flex-col gap-3">
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <h1 className="font-serif text-2xl font-semibold tracking-tight">{char.name_zh}</h1>
          <span className="text-sm text-muted-foreground">{char.name_en}</span>
          <span className="text-sm text-amber-500">{rarityStars(char.rarity)}</span>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Badge style={{ color, borderColor: color }} className="bg-transparent">
            {elementLabel(char.element)}
          </Badge>
          <Badge className="bg-transparent">{pathLabel(char.path)}</Badge>
          {char.roles.map((r) => (
            <Badge key={r}>{roleLabel(r)}</Badge>
          ))}
          {char.sp_need != null && (
            <span className="text-xs text-muted-foreground">终结技能量 {char.sp_need}</span>
          )}
        </div>

        {tags.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {tags.map((t) => (
              <span
                key={t}
                className="rounded-md bg-muted px-1.5 py-0.5 text-xs text-muted-foreground"
              >
                {tagLabel(t)}
              </span>
            ))}
          </div>
        )}

        {char.axes?.notes && (
          <p className="mt-1 text-sm leading-relaxed text-foreground/90">{char.axes.notes}</p>
        )}
      </div>
    </div>
  )
}
