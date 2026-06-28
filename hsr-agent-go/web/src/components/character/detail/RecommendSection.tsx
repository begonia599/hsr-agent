import type { Recommendation } from '@/api/types'

function RecRow({ rec }: { rec: Recommendation }) {
  return (
    <li className="flex items-center gap-2 rounded-lg bg-muted/40 px-3 py-2 text-sm">
      <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-background text-xs text-muted-foreground">
        {rec.rank + 1}
      </span>
      <span className="font-medium">{rec.name_zh || `#${rec.item_id}`}</span>
    </li>
  )
}

export function RecommendSection({
  lightcones,
  relics,
}: {
  lightcones: Recommendation[]
  relics: Recommendation[]
}) {
  const set4 = relics.filter((r) => r.kind === 'relic_set4').sort((a, b) => a.rank - b.rank)
  const set2 = relics.filter((r) => r.kind === 'relic_set2').sort((a, b) => a.rank - b.rank)
  const lcSorted = [...lightcones].sort((a, b) => a.rank - b.rank)

  return (
    <div className="grid gap-5 md:grid-cols-3">
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-muted-foreground">推荐光锥</h3>
        {lcSorted.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无</p>
        ) : (
          <ul className="space-y-1.5">
            {lcSorted.map((r, i) => (
              <RecRow key={i} rec={r} />
            ))}
          </ul>
        )}
      </div>
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-muted-foreground">四件套(隧洞)</h3>
        {set4.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无</p>
        ) : (
          <ul className="space-y-1.5">
            {set4.map((r, i) => (
              <RecRow key={i} rec={r} />
            ))}
          </ul>
        )}
      </div>
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-muted-foreground">二件套(位面)</h3>
        {set2.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无</p>
        ) : (
          <ul className="space-y-1.5">
            {set2.map((r, i) => (
              <RecRow key={i} rec={r} />
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
