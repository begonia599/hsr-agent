// 与 hsr-agent-go/internal/tools 的 Go struct 对齐的 TS 类型。

export interface Character {
  id: number
  version: string
  rarity: number
  path: string
  element: string
  name_zh: string
  name_en: string
  sp_need?: number
  roles: string[]
  axes: CharacterAxes
  is_trailblazer: boolean
  is_collab: boolean
  is_variant: boolean
  skill_text_brief?: string
}

// axes JSONB 结构(enrich 产出)
export interface AxisEntry {
  stat: string
  target?: string
  value?: number
  uptime?: string
  reason?: string
  source?: string
  condition?: string
}

export interface CharacterAxes {
  roles?: string[]
  tags?: string[]
  notes?: string
  provides?: AxisEntry[]
  needs?: AxisEntry[]
  restricts?: AxisEntry[]
}

// /characters/{id}/synergies
export interface Synergy {
  char_id: number
  name_zh: string
  rarity: number
  path: string
  element: string
  roles: string[]
  score: number
  cooccur_weight: number
  matched_need_axes?: string[]
  reasons: string[]
}

// /characters/{id}/teams
export interface TeamPlan {
  core_id: number
  core_name_zh: string
  members: Synergy[]
  notes: string[]
}

// /characters/{id}/lightcones | /relics
export interface Recommendation {
  kind: string
  item_id?: number
  rank: number
  name_zh?: string
  payload?: unknown
}

// /characters/{id}/modifiers
export interface Modifier {
  character_id: number
  character_name_zh: string
  source_kind: string
  source_name_zh: string
  target_scope: string
  stat_key: string
  value?: number
  value_unit: string
  modifier_zone: string
  attack_tag?: string
  element_key?: string
  condition_text?: string
  duration_key?: string
  stack_rule?: string
  confidence: number
  reviewed: boolean
}

export interface Lightcone {
  id: number
  version: string
  rarity: number
  path: string
  name_zh: string
  name_en: string
  desc_zh?: string
  axes: CharacterAxes
  data_quality?: string
  warning?: string
}

export interface RelicSet {
  id: number
  version: string
  kind: string
  name_zh: string
  name_en: string
  set2_desc?: string
  set4_desc?: string
  figure_url?: string
  axes: CharacterAxes
}

// /lightcones/{id}/refinements —— 透传 raw_zh->'refinements',叠影 1-5 模板。
// desc 带 #N[i]/#N[f1]/#N[f2] 占位符,level[lv].param_list 按叠影等级填值。
export interface LightconeRefinements {
  name: string
  desc: string
  level: Record<string, { param_list: number[] }>
}

export interface Asset {
  entity_kind: string
  entity_id: string
  variant: string
  local_path: string
  cdn_url: string
  bytes?: number
}

export interface HealthInfo {
  status: string
  database: { status: string }
  data: {
    version?: string
    characters?: number
    lightcones?: number
    relic_sets?: number
  }
  llm: { configured: boolean; model: string; format: string }
  embedding: { provider: string; model: string; dimensions: number; quality: string }
  web: { root: string }
}

// /characters/{id}/skills —— 原始技能/星魂 wiki 文本(raw_zh 子树)
export interface SkillLevel {
  level: number
  param_list: number[]
}

export interface Skill {
  id: number
  name: string
  desc: string
  type: string // Normal/BPSkill/Ultra/Talent/Maze/MazeNormal
  type_name?: string // 普攻/战技/终结技/天赋/秘技
  level: Record<string, SkillLevel>
}

export interface SkillRank {
  id: number
  name: string
  desc: string
  param_list: number[]
}

export interface CharacterSkills {
  skills: Record<string, Skill>
  ranks: Record<string, SkillRank>
}

// SSE 事件 —— 对齐 agent.Event + server.go writeSSE。
export type ChatEvent =
  | { kind: 'status'; message: string }
  | { kind: 'tool_call'; name: string; args: unknown; toolCallId?: string }
  | { kind: 'tool_result'; name: string; result: unknown; toolCallId?: string }
  | { kind: 'final'; message: string }
  | { kind: 'error'; code: string; message: string }
