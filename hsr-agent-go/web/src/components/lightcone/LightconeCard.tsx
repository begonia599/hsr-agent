import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import type { Lightcone } from '@/api/types'
import { lightconeIconUrl, pathLabel, rarityStars } from '@/lib/hsr'

export function LightconeCard({ lc }: { lc: Lightcone }) {
  return (
    <Link
      to={`/lightcones/${lc.id}`}
      className="group flex flex-col items-center gap-2 rounded-xl bg-card p-3 ring-1 ring-foreground/10 transition-shadow hover:ring-foreground/25"
    >
      <div className="flex size-16 items-center justify-center overflow-hidden rounded-lg bg-muted/50">
        <img
          src={lightconeIconUrl(lc.id)}
          alt={lc.name_zh}
          loading="lazy"
          className="size-full object-contain"
          onError={(e) => {
            ;(e.target as HTMLImageElement).style.visibility = 'hidden'
          }}
        />
      </div>
      <div className="flex flex-col items-center gap-1 text-center">
        <span className="text-sm font-medium leading-tight group-hover:text-foreground">
          {lc.name_zh}
        </span>
        <div className="flex items-center gap-1">
          <span className="text-[0.7rem] text-amber-500">{rarityStars(lc.rarity)}</span>
        </div>
        <Badge className="px-1 py-0 text-[0.65rem]">{pathLabel(lc.path)}</Badge>
      </div>
    </Link>
  )
}
