import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import type { RelicSet } from '@/api/types'
import { kindLabel } from '@/lib/hsr'

export function RelicSetCard({ set }: { set: RelicSet }) {
  return (
    <Link
      to={`/relic-sets/${set.id}`}
      className="group flex flex-col items-center gap-2 rounded-xl bg-card p-3 ring-1 ring-foreground/10 transition-shadow hover:ring-foreground/25"
    >
      <div className="flex size-16 items-center justify-center overflow-hidden rounded-lg bg-muted/50">
        {set.figure_url && (
          <img
            src={set.figure_url}
            alt={set.name_zh}
            loading="lazy"
            className="size-full object-contain"
            onError={(e) => {
              ;(e.target as HTMLImageElement).style.visibility = 'hidden'
            }}
          />
        )}
      </div>
      <div className="flex flex-col items-center gap-1 text-center">
        <span className="text-sm font-medium leading-tight group-hover:text-foreground">
          {set.name_zh}
        </span>
        <Badge className="px-1 py-0 text-[0.65rem]">{kindLabel(set.kind)}</Badge>
      </div>
    </Link>
  )
}
