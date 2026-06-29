import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { api } from '@/api/client'
import type { Lightcone, LightconeRefinements } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Section } from '@/components/character/detail/Section'
import { AxesSection } from '@/components/character/detail/AxesSection'
import { RefinementSection } from '@/components/lightcone/RefinementSection'
import { lightconeFigureUrl, pathLabel, rarityStars, tagLabel } from '@/lib/hsr'

export function LightconeDetailPage() {
  const { id } = useParams<{ id: string }>()
  const lcId = Number(id)
  const [lc, setLc] = useState<Lightcone | null>(null)
  const [refinements, setRefinements] = useState<LightconeRefinements | null>(null)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    if (!lcId) return
    let cancelled = false
    setLoading(true)
    setErr(null)
    setLc(null)
    setRefinements(null)

    // 基本信息是硬依赖;叠影模板允许缺失(部分光锥无 refinements 结构)
    api
      .getLightcone(lcId)
      .then(async (data) => {
        const ref = await api.getLightconeRefinements(lcId).catch(() => null)
        if (cancelled) return
        setLc(data)
        // 端点在缺失时返回 null
        setRefinements(ref && ref.desc ? ref : null)
      })
      .catch((e) => {
        if (!cancelled) setErr(e.message ?? '加载失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [lcId])

  const tags = lc?.axes?.tags ?? []

  return (
    <div className="mx-auto h-full w-full max-w-3xl overflow-y-auto px-4 py-5">
      <Link
        to="/lightcones"
        className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="size-4" />
        返回光锥列表
      </Link>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : err ? (
        <p className="text-sm text-destructive">加载失败：{err}</p>
      ) : lc ? (
        <div className="space-y-7 pb-10">
          <Card>
            <CardContent>
              <div className="flex flex-col gap-5 sm:flex-row">
                <div className="mx-auto w-40 shrink-0 sm:mx-0">
                  <div className="aspect-3/4 overflow-hidden rounded-xl bg-muted ring-1 ring-foreground/10">
                    <img
                      src={lightconeFigureUrl(lc.id)}
                      alt={lc.name_zh}
                      className="size-full object-cover object-top"
                      onError={(e) => {
                        ;(e.target as HTMLImageElement).style.visibility = 'hidden'
                      }}
                    />
                  </div>
                </div>
                <div className="flex flex-1 flex-col gap-3">
                  <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                    <h1 className="font-serif text-2xl font-semibold tracking-tight">
                      {lc.name_zh}
                    </h1>
                    <span className="text-sm text-muted-foreground">{lc.name_en}</span>
                    <span className="text-sm text-amber-500">{rarityStars(lc.rarity)}</span>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge className="bg-transparent">{pathLabel(lc.path)}</Badge>
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
                  {lc.axes?.notes && (
                    <p className="mt-1 text-sm leading-relaxed text-foreground/90">
                      {lc.axes.notes}
                    </p>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>

          {refinements && (
            <Section title="叠影效果">
              <RefinementSection data={refinements} />
            </Section>
          )}

          <Section title="能力轴">
            <AxesSection
              provides={lc.axes?.provides ?? []}
              needs={lc.axes?.needs ?? []}
            />
          </Section>
        </div>
      ) : null}
    </div>
  )
}
