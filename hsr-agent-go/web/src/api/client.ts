import type {
  Asset,
  Character,
  CharacterSkills,
  HealthInfo,
  Lightcone,
  Modifier,
  Recommendation,
  RelicSet,
  Synergy,
  TeamPlan,
} from './types'

export class ApiError extends Error {
  code: string
  status: number
  constructor(code: string, message: string, status: number) {
    super(message)
    this.code = code
    this.status = status
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
  })
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    const err = body?.error
    throw new ApiError(err?.code ?? 'UNKNOWN', err?.message ?? res.statusText, res.status)
  }
  return body as T
}

export interface CharacterFilter {
  q?: string
  role?: string
  element?: string
  path?: string
  rarity?: number
  limit?: number
}

function qs(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '' && value !== 0) {
      search.set(key, String(value))
    }
  }
  const str = search.toString()
  return str ? `?${str}` : ''
}

export const api = {
  health: () => request<HealthInfo>('/api/health'),

  listCharacters: (filter: CharacterFilter = {}) =>
    request<Character[]>(`/api/characters${qs({ ...filter })}`),

  getCharacter: (id: number) => request<Character>(`/api/characters/${id}`),

  getCharacterAssets: (id: number, variants?: string[]) =>
    request<Asset[]>(
      `/api/characters/${id}/assets${variants?.length ? `?variants=${variants.join(',')}` : ''}`,
    ),

  getCharacterSynergies: (id: number, limit = 8) =>
    request<Synergy[]>(`/api/characters/${id}/synergies?limit=${limit}`),

  getCharacterTeams: (id: number) => request<TeamPlan[]>(`/api/characters/${id}/teams`),

  getCharacterLightcones: (id: number) =>
    request<Recommendation[]>(`/api/characters/${id}/lightcones`),

  getCharacterRelics: (id: number) => request<Recommendation[]>(`/api/characters/${id}/relics`),

  getCharacterModifiers: (id: number, limit = 60) =>
    request<Modifier[]>(`/api/characters/${id}/modifiers?limit=${limit}`),

  getCharacterSkills: (id: number) =>
    request<CharacterSkills>(`/api/characters/${id}/skills`),

  listLightcones: (q?: string, path?: string, rarity?: number, limit?: number) =>
    request<Lightcone[]>(`/api/lightcones${qs({ q, path, rarity, limit })}`),

  listRelicSets: (q?: string, kind?: string, limit?: number) =>
    request<RelicSet[]>(`/api/relic-sets${qs({ q, kind, limit })}`),

  keywordSearch: (q: string, kind?: string, limit?: number) =>
    request<unknown[]>(`/api/search/keyword${qs({ q, kind, limit })}`),
}
