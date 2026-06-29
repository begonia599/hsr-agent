import { useEffect, useMemo, useState } from 'react'
import { api } from '@/api/client'
import type { Lightcone } from '@/api/types'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { PATH_LABELS } from '@/lib/hsr'
import { LightconeCard } from './LightconeCard'

const RARITIES = [5, 4, 3]

export function LightconeList() {
  const [all, setAll] = useState<Lightcone[]>([])
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [q, setQ] = useState('')
  const [path, setPath] = useState('')
  const [rarity, setRarity] = useState(0)

  useEffect(() => {
    setLoading(true)
    api
      .listLightcones(undefined, undefined, undefined, 400)
      .then((rows) => setAll(rows))
      .catch((e) => setErr(e.message))
      .finally(() => setLoading(false))
  }, [])

  const filtered = useMemo(() => {
    return all.filter((lc) => {
      if (path && lc.path !== path) return false
      if (rarity && lc.rarity !== rarity) return false
      if (q) {
        const needle = q.toLowerCase()
        if (
          !lc.name_zh.toLowerCase().includes(needle) &&
          !lc.name_en.toLowerCase().includes(needle) &&
          !String(lc.id).includes(needle)
        )
          return false
      }
      return true
    })
  }, [all, q, path, rarity])

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
          placeholder="搜索光锥（中文名 / 英文名 / id）"
          className="max-w-sm"
        />
        <div className="flex flex-wrap gap-1.5">
          <button className={chip(path === '')} onClick={() => setPath('')}>
            全部命途
          </button>
          {Object.entries(PATH_LABELS).map(([key, label]) => (
            <button key={key} className={chip(path === key)} onClick={() => setPath(key)}>
              {label}
            </button>
          ))}
        </div>
        <div className="flex flex-wrap gap-1.5">
          <button className={chip(rarity === 0)} onClick={() => setRarity(0)}>
            全部稀有度
          </button>
          {RARITIES.map((r) => (
            <button key={r} className={chip(rarity === r)} onClick={() => setRarity(r)}>
              {r}★
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
          <p className="mb-3 text-xs text-muted-foreground">{filtered.length} 把光锥</p>
          <div className="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-7">
            {filtered.map((lc) => (
              <LightconeCard key={lc.id} lc={lc} />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
