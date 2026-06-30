import type { ReactNode, SyntheticEvent } from 'react'
import { Link } from 'react-router-dom'
import { Gem } from 'lucide-react'
import {
  ELEMENT_COLORS,
  elementLabel,
  formatAxisValue,
  lightconeIconUrl,
  pathLabel,
  roundIconUrl,
  statLabel,
  targetLabel,
} from '@/lib/hsr'

// agent tool_result 是 unknown;用宽松访问器读字段(结构变动时兜底,不崩)。
type Rec = Record<string, unknown>
function rec(x: unknown): Rec {
  return x && typeof x === 'object' ? (x as Rec) : {}
}
function num(x: unknown): number | undefined {
  return typeof x === 'number' ? x : undefined
}
function str(x: unknown): string | undefined {
  return typeof x === 'string' && x ? x : undefined
}
function arr(x: unknown): unknown[] {
  return Array.isArray(x) ? x : []
}

// 站内 URL:优先 result 自带 url,否则按 kind+id 拼
function entityHref(kind: string | undefined, id: number | undefined, url?: string): string | null {
  if (url) return url
  if (id == null) return null
  switch (kind) {
    case 'character':
      return `/characters/${id}`
    case 'lightcone':
      return `/lightcones/${id}`
    case 'relic_set':
      return `/relic-sets/${id}`
    default:
      return null
  }
}

function hideOnError(e: SyntheticEvent<HTMLImageElement>) {
  ;(e.target as HTMLImageElement).style.visibility = 'hidden'
}

const chipClass =
  'group flex items-center gap-2 rounded-lg bg-card px-2 py-1.5 ring-1 ring-foreground/10 transition-shadow hover:ring-foreground/25'

export function CharacterChip({
  id,
  name,
  element,
  path,
  badge,
  sub,
}: {
  id: number
  name: string
  element?: string
  path?: string
  badge?: ReactNode
  sub?: string
}) {
  const color = element ? ELEMENT_COLORS[element] : undefined
  return (
    <Link to={`/characters/${id}`} className={chipClass}>
      <img
        src={roundIconUrl(id)}
        alt={name}
        loading="lazy"
        className="size-8 shrink-0 rounded-full object-cover ring-1 ring-foreground/10"
        style={color ? { ['--tw-ring-color' as string]: color } : undefined}
        onError={hideOnError}
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1">
          <span className="truncate text-xs font-medium group-hover:text-foreground">{name}</span>
          {badge != null && (
            <span className="ml-auto shrink-0 text-[0.65rem] text-muted-foreground">{badge}</span>
          )}
        </div>
        {(element || path) && (
          <div className="truncate text-[0.65rem] text-muted-foreground">
            {element && <span style={color ? { color } : undefined}>{elementLabel(element)}</span>}
            {element && path && ' · '}
            {path && pathLabel(path)}
          </div>
        )}
        {sub && <div className="truncate text-[0.65rem] text-muted-foreground/80">{sub}</div>}
      </div>
    </Link>
  )
}

export function LightconeChip({ id, name }: { id: number; name: string }) {
  return (
    <Link to={`/lightcones/${id}`} className={chipClass}>
      <img
        src={lightconeIconUrl(id)}
        alt={name}
        loading="lazy"
        className="size-8 shrink-0 rounded bg-muted/50 object-contain"
        onError={hideOnError}
      />
      <span className="truncate text-xs font-medium group-hover:text-foreground">{name}</span>
    </Link>
  )
}

export function RelicChip({ id, name }: { id: number; name: string }) {
  return (
    <Link to={`/relic-sets/${id}`} className={chipClass}>
      <span className="flex size-8 shrink-0 items-center justify-center rounded bg-muted/50">
        <Gem className="size-4 text-muted-foreground" />
      </span>
      <span className="truncate text-xs font-medium group-hover:text-foreground">{name}</span>
    </Link>
  )
}

// 跨类实体卡:semantic_search / resolve_entities 用,按 kind 分发
export function EntityChip({
  kind,
  id,
  name,
  element,
  path,
  url,
}: {
  kind?: string
  id?: number
  name: string
  element?: string
  path?: string
  url?: string
}) {
  if (id != null) {
    if (kind === 'character') return <CharacterChip id={id} name={name} element={element} path={path} />
    if (kind === 'lightcone') return <LightconeChip id={id} name={name} />
    if (kind === 'relic_set') return <RelicChip id={id} name={name} />
  }
  const href = entityHref(kind, id, url)
  if (href)
    return (
      <Link to={href} className={chipClass}>
        <span className="truncate text-xs font-medium group-hover:text-foreground">{name}</span>
      </Link>
    )
  return (
    <span className="rounded-lg bg-muted/40 px-2 py-1.5 text-xs text-muted-foreground">{name}</span>
  )
}

export function AxisChip({
  stat,
  value,
  target,
}: {
  stat: string
  value?: number
  target?: string
}) {
  const v = formatAxisValue(value)
  return (
    <span className="inline-flex items-center gap-1 rounded-md bg-muted px-1.5 py-0.5 text-[0.7rem]">
      <span className="font-medium">{statLabel(stat)}</span>
      {v && <span className="text-foreground/70">{v}</span>}
      {target && <span className="text-muted-foreground">{targetLabel(target)}</span>}
    </span>
  )
}

function Grid({ children }: { children: ReactNode }) {
  return <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">{children}</div>
}

// ── 各工具渲染器 ────────────────────────────────────────────

export function renderCharacter(result: unknown): ReactNode {
  const c = rec(result)
  const id = num(c.id)
  if (id == null) return null
  return (
    <CharacterChip
      id={id}
      name={str(c.name_zh) ?? str(c.name_en) ?? String(id)}
      element={str(c.element)}
      path={str(c.path)}
    />
  )
}

// find_synergies / co_occurrence / search_by_role:行里 id 可能是 id 或 char_id
export function renderCharacterRows(result: unknown): ReactNode {
  const rows = arr(result)
  if (!rows.length) return null
  return (
    <Grid>
      {rows.map((row, i) => {
        const r = rec(row)
        const id = num(r.id) ?? num(r.char_id)
        if (id == null) return null
        const weight = num(r.weight)
        const reasons = arr(r.reasons)
        const sub = reasons.length ? str(reasons[0]) : undefined
        return (
          <CharacterChip
            key={i}
            id={id}
            name={str(r.name_zh) ?? String(id)}
            element={str(r.element)}
            path={str(r.path)}
            badge={weight != null ? `共现 ${weight}` : undefined}
            sub={sub}
          />
        )
      })}
    </Grid>
  )
}

export function renderTeams(result: unknown): ReactNode {
  const teams = arr(result)
  if (!teams.length) return null
  return (
    <div className="space-y-2">
      {teams.map((team, i) => {
        const t = rec(team)
        const coreId = num(t.core_id)
        const members = arr(t.members)
        return (
          <div key={i} className="space-y-1.5 rounded-lg bg-muted/40 p-2">
            {coreId != null && (
              <CharacterChip id={coreId} name={str(t.core_name_zh) ?? String(coreId)} badge="核心" />
            )}
            <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
              {members.map((mm, j) => {
                const m = rec(mm)
                const id = num(m.char_id) ?? num(m.id)
                if (id == null) return null
                return (
                  <CharacterChip
                    key={j}
                    id={id}
                    name={str(m.name_zh) ?? String(id)}
                    element={str(m.element)}
                    path={str(m.path)}
                  />
                )
              })}
            </div>
          </div>
        )
      })}
    </div>
  )
}

export function renderEntities(result: unknown): ReactNode {
  const rows = arr(result)
  if (!rows.length) return null
  return (
    <Grid>
      {rows.map((row, i) => {
        const r = rec(row)
        if (r.found === false) return null
        const id = num(r.id)
        return (
          <EntityChip
            key={i}
            kind={str(r.kind)}
            id={id}
            name={str(r.name_zh) ?? str(r.name) ?? String(id ?? i)}
            element={str(r.element)}
            path={str(r.path)}
            url={str(r.url)}
          />
        )
      })}
    </Grid>
  )
}

export function renderLightcones(result: unknown): ReactNode {
  const rows = arr(result)
  if (!rows.length) return null
  return (
    <Grid>
      {rows.map((row, i) => {
        const r = rec(row)
        const id = num(r.item_id) ?? num(r.id)
        if (id == null) return null
        return <LightconeChip key={i} id={id} name={str(r.name_zh) ?? String(id)} />
      })}
    </Grid>
  )
}

export function renderRelics(result: unknown): ReactNode {
  const rows = arr(result)
  if (!rows.length) return null
  return (
    <Grid>
      {rows.map((row, i) => {
        const r = rec(row)
        const id = num(r.item_id) ?? num(r.id)
        if (id == null) return null
        return <RelicChip key={i} id={id} name={str(r.name_zh) ?? String(id)} />
      })}
    </Grid>
  )
}

// find_needs:展示需求的 stat 标签
export function renderNeeds(result: unknown): ReactNode {
  const rows = arr(result)
  if (!rows.length) return null
  return (
    <div className="flex flex-wrap gap-1">
      {rows.map((row, i) => {
        const r = rec(row)
        const stat = str(r.stat)
        if (!stat) return null
        return <AxisChip key={i} stat={stat} value={num(r.value)} target={str(r.target)} />
      })}
    </div>
  )
}

// find_buffers_for:展示提供该增益的角色
export function renderBuffers(result: unknown): ReactNode {
  const rows = arr(result)
  if (!rows.length) return null
  return (
    <Grid>
      {rows.map((row, i) => {
        const r = rec(row)
        const id = num(r.char_id) ?? num(r.id)
        const stat = str(r.stat)
        if (id == null) {
          return stat ? <AxisChip key={i} stat={stat} value={num(r.value)} target={str(r.target)} /> : null
        }
        const v = formatAxisValue(num(r.value))
        const sub = stat ? `${statLabel(stat)}${v ? ' ' + v : ''}` : undefined
        return <CharacterChip key={i} id={id} name={str(r.name_zh) ?? String(id)} sub={sub} />
      })}
    </Grid>
  )
}
