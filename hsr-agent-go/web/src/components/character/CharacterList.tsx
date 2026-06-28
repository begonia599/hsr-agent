import { useEffect, useMemo, useState } from 'react'
import { api } from '@/api/client'
import type { Character } from '@/api/types'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { ELEMENT_LABELS, PATH_LABELS } from '@/lib/hsr'
import { CharacterCard } from './CharacterCard'

export function CharacterList() {
  const [all, setAll] = useState<Character[]>([])
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [q, setQ] = useState('')
  const [element, setElement] = useState('')
  const [path, setPath] = useState('')

  useEffect(() => {
    setLoading(true)
    api
      .listCharacters({ limit: 300 })
      .then((rows) => setAll(rows))
      .catch((e) => setErr(e.message))
      .finally(() => setLoading(false))
  }, [])

  const filtered = useMemo(() => {
    return all.filter((c) => {
      if (element && c.element !== element) return false
      if (path && c.path !== path) return false
      if (q) {
        const needle = q.toLowerCase()
        if (
          !c.name_zh.toLowerCase().includes(needle) &&
          !c.name_en.toLowerCase().includes(needle) &&
          !String(c.id).includes(needle)
        )
          return false
      }
      return true
    })
  }, [all, q, element, path])

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
          placeholder="搜索角色（中文名 / 英文名 / id）"
          className="max-w-sm"
        />
        <div className="flex flex-wrap gap-1.5">
          <button className={chip(element === '')} onClick={() => setElement('')}>
            全部元素
          </button>
          {Object.entries(ELEMENT_LABELS).map(([key, label]) => (
            <button key={key} className={chip(element === key)} onClick={() => setElement(key)}>
              {label}
            </button>
          ))}
        </div>
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
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : err ? (
        <p className="text-sm text-destructive">加载失败：{err}</p>
      ) : (
        <>
          <p className="mb-3 text-xs text-muted-foreground">{filtered.length} 个角色</p>
          <div className="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-7">
            {filtered.map((c) => (
              <CharacterCard key={c.id} char={c} />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
