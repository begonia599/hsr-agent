import type { ReactNode } from 'react'
import {
  renderBuffers,
  renderCharacter,
  renderCharacterRows,
  renderEntities,
  renderLightcones,
  renderNeeds,
  renderRelics,
  renderTeams,
} from './cards'

type Renderer = (result: unknown, args?: unknown) => ReactNode

// tool_name → 渲染器。未注册或渲染为空时回退到折叠 JSON(渐进增强)。
const renderers: Record<string, Renderer> = {
  get_character: renderCharacter,
  find_synergies: renderCharacterRows,
  co_occurrence: renderCharacterRows,
  search_by_role: renderCharacterRows,
  suggest_team: renderTeams,
  semantic_search: renderEntities,
  keyword_search: renderEntities,
  resolve_entities: renderEntities,
  recommend_lightcones: renderLightcones,
  recommend_relics: renderRelics,
  find_needs: renderNeeds,
  find_buffers_for: renderBuffers,
}

export function renderToolResult(name: string, result: unknown, args?: unknown): ReactNode {
  const fn = renderers[name]
  if (fn && result != null) {
    const node = fn(result, args)
    if (node) return node
  }
  if (Array.isArray(result) && result.length === 0) {
    return <p className="text-[0.7rem] text-muted-foreground">无匹配结果</p>
  }
  return <JsonFallback result={result} />
}

function JsonFallback({ result }: { result: unknown }) {
  if (result == null) return null
  return (
    <details className="rounded-md bg-muted/40 px-2 py-1 text-[0.7rem] text-muted-foreground">
      <summary className="cursor-pointer select-none">原始数据</summary>
      <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono">
        {JSON.stringify(result, null, 2)}
      </pre>
    </details>
  )
}
