import { useEffect, useMemo, useState } from 'react'
import { api } from '@/api/client'
import type { RelicSet } from '@/api/types'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { KIND_LABELS } from '@/lib/hsr'
import { RelicSetCard } from './RelicSetCard'

export function RelicSetList() {
  const [all, setAll] = useState<RelicSet[]>([])
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [q, setQ] = useState('')
  const [kind, setKind] = useState('')

  useEffect(() => {
    setLoading(true)
    api
      .listRelicSets(undefined, undefined, 300)
      .then((rows) => setAll(rows))
      .catch((e) => setErr(e.message))
      .finally(() => setLoading(false))
  }, [])

  const filtered = useMemo(() => {
    return all.filter((s) => {
      if (kind && s.kind !== kind) return false
      if (q) {
        const needle = q.toLowerCase()
        if (
          !s.name_zh.toLowerCase().includes(needle) &&
          !s.name_en.toLowerCase().includes(needle) &&
          !String(s.id).includes(needle)
        )
          return false
      }
      return true
    })
  }, [all, q, kind])

  const chip = (active: boolean) =>
    cn(
      'rounded-full border px-2.5 py-1 text-xs transition-colors',
      active
        ? 'border-primary bg-primary text-primary-foreground'
        : 'border-border bg-card text-muted-foreground hover:text-foreground',
    )

  return (
    <div className="mx-auto h-full w-full max-w-5xl overflow-y-auto px-4 py-6">
      <div className="mb-4 space-y-3">
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="搜索遗器套装（中文名 / 英文名 / id）"
          className="max-w-sm"
        />
        <div className="flex flex-wrap gap-1.5">
          <button className={chip(kind === '')} onClick={() => setKind('')}>
            全部类型
          </button>
          {Object.entries(KIND_LABELS).map(([key, label]) => (
            <button key={key} className={chip(kind === key)} onClick={() => setKind(key)}>
              {label}
            </button>
          ))}
        </div>
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : err ? (
        <p className="text-sm text-destructive">加载失败：{err}</p>
      ) : (
        <>
          <p className="mb-3 text-xs text-muted-foreground">{filtered.length} 个套装</p>
          <div className="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-7">
            {filtered.map((s) => (
              <RelicSetCard key={s.id} set={s} />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
