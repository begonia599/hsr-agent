// 工具名 → 中文说明,对话 ToolTrace 与 Agent 侧栏共用。
export const TOOL_LABELS: Record<string, string> = {
  get_character: '查角色资料',
  search_by_role: '按定位筛选',
  find_needs: '分析角色需求',
  find_buffers_for: '查增益来源',
  find_synergies: '找协同角色',
  suggest_team: '生成配队方案',
  co_occurrence: '查队伍共现',
  recommend_lightcones: '推荐光锥',
  recommend_relics: '推荐遗器',
  semantic_search: '语义检索',
  keyword_search: '关键词检索',
  list_character_modifiers: '查机制效果',
  explain_modifier_sources: '溯源效果来源',
  compare_character_fit: '契合度对比',
  estimate_damage_gain: '估算伤害收益',
  estimate_dot_damage: '估算持续伤害',
  estimate_break_damage: '估算击破伤害',
  estimate_super_break_damage: '估算超击破',
  estimate_healing: '估算治疗量',
  estimate_shield: '估算护盾量',
  estimate_uptime: '估算覆盖率',
  resolve_entities: '解析实体链接',
}

export function toolLabel(name: string): string {
  return TOOL_LABELS[name] ?? name
}
