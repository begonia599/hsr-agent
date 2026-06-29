import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { api } from '@/api/client'
import type { RelicSet } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Section } from '@/components/character/detail/Section'
import { RelicSetEffects } from '@/components/relic/RelicSetEffects'
import { kindLabel, tagLabel } from '@/lib/hsr'

export function RelicSetDetailPage() {
  const { id } = useParams<{ id: string }>()
  const setId = Number(id)
  const [set, setSet] = useState<RelicSet | null>(null)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    if (!setId) return
    let cancelled = false
    setLoading(true)
    setErr(null)
    setSet(null)
    api
      .getRelicSet(setId)
      .then((s) => {
        if (!cancelled) setSet(s)
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
  }, [setId])

  const tags = set?.axes?.tags ?? []

  return (
    <div className="mx-auto h-full w-full max-w-3xl overflow-y-auto px-4 py-5">
      <Link
        to="/relic-sets"
        className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="size-4" />
        返回遗器列表
      </Link>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : err ? (
        <p className="text-sm text-destructive">加载失败：{err}</p>
      ) : set ? (
        <div className="space-y-7 pb-10">
          <Card>
            <CardContent>
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
                <div className="mx-auto flex size-28 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-muted/50 sm:mx-0">
                  {set.figure_url && (
                    <img
                      src={set.figure_url}
                      alt={set.name_zh}
                      className="size-full object-contain"
                      onError={(e) => {
                        ;(e.target as HTMLImageElement).style.visibility = 'hidden'
                      }}
                    />
                  )}
                </div>
                <div className="flex flex-1 flex-col gap-2">
                  <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                    <h1 className="font-serif text-2xl font-semibold tracking-tight">
                      {set.name_zh}
                    </h1>
                    <span className="text-sm text-muted-foreground">{set.name_en}</span>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge className="bg-transparent">{kindLabel(set.kind)}</Badge>
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
                  {set.axes?.notes && (
                    <p className="mt-1 text-sm leading-relaxed text-foreground/90">
                      {set.axes.notes}
                    </p>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>

          <Section title="套装效果">
            <RelicSetEffects
              provides={set.axes?.provides ?? []}
              needs={set.axes?.needs ?? []}
            />
          </Section>
        </div>
      ) : null}
    </div>
  )
}
