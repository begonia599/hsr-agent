// 命途 / 元素 的中英映射与配色,前端展示用。

export const PATH_LABELS: Record<string, string> = {
  Knight: '存护',
  Mage: '智识',
  Priest: '丰饶',
  Rogue: '巡猎',
  Shaman: '同谐',
  Warlock: '虚无',
  Warrior: '毁灭',
  Memory: '记忆',
}

export const ELEMENT_LABELS: Record<string, string> = {
  Fire: '火',
  Ice: '冰',
  Imaginary: '虚数',
  Physical: '物理',
  Quantum: '量子',
  Thunder: '雷',
  Wind: '风',
}

export const ROLE_LABELS: Record<string, string> = {
  main_dps: '主C',
  sub_dps: '副C',
  amplifier: '增幅',
  debuffer: '负面',
  sustain_healer: '治疗',
  sustain_shielder: '护盾',
  sustain_hybrid: '混合生存',
  remembrance: '记忆',
  generalist: '多功能',
  break_specialist: '击破',
}

// 元素配色(低饱和,贴合暖灰主题)
export const ELEMENT_COLORS: Record<string, string> = {
  Fire: 'oklch(0.62 0.17 35)',
  Ice: 'oklch(0.68 0.10 230)',
  Imaginary: 'oklch(0.78 0.14 90)',
  Physical: 'oklch(0.60 0.01 80)',
  Quantum: 'oklch(0.55 0.16 300)',
  Thunder: 'oklch(0.60 0.16 320)',
  Wind: 'oklch(0.66 0.13 160)',
}

export function pathLabel(path: string): string {
  return PATH_LABELS[path] ?? path
}

export function elementLabel(element: string): string {
  return ELEMENT_LABELS[element] ?? element
}

export function roleLabel(role: string): string {
  return ROLE_LABELS[role] ?? role
}

// stat / stat_key 的中文名(同时覆盖 axes 的 *_percent 与 modifiers 的 *_pct 写法)
export const STAT_LABELS: Record<string, string> = {
  atk_percent: '攻击力%',
  atk_pct: '攻击力%',
  atk_flat: '固定攻击力',
  atk_flat_scaling_from_self_atk: '攻击力加成(基于自身攻击)',
  atk_scaler: '攻击力缩放',
  hp_percent: '生命值%',
  hp_pct: '生命值%',
  hp_flat: '固定生命值',
  hp_scaler: '生命值缩放',
  def_percent: '防御力%',
  def_pct: '防御力%',
  def_flat: '固定防御力',
  def_scaler: '防御力缩放',
  def_ignore: '无视防御',
  def_shred: '减防',
  speed_percent: '速度%',
  speed_pct: '速度%',
  speed_flat: '速度',
  speed_team: '全队速度',
  crit_rate: '暴击率',
  crit_dmg: '暴击伤害',
  crit_scaler: '暴击缩放',
  dmg_percent: '增伤',
  dmg_bonus: '增伤',
  element_dmg_bonus: '元素增伤',
  basic_dmg: '普攻伤害',
  basic_dmg_bonus: '普攻增伤',
  skill_dmg: '战技伤害',
  skill_dmg_bonus: '战技增伤',
  ult_dmg_bonus: '终结技增伤',
  fua_dmg: '追击伤害',
  fua_dmg_bonus: '追击增伤',
  dot_dmg: '持续伤害',
  dot_dmg_bonus: '持续增伤',
  break_dmg: '击破伤害',
  break_dmg_bonus: '击破增伤',
  super_break_dmg: '超击破伤害',
  super_break_dmg_bonus: '超击破增伤',
  additional_dmg: '附加伤害',
  true_dmg: '真实伤害',
  dmg_taken_reduce: '减伤',
  dmg_reduction: '减伤',
  vulnerability: '易伤',
  res_pen: '抗性穿透',
  res_reduce: '减抗',
  res_reduction: '减抗',
  break_eff: '击破特攻',
  break_effect: '击破特攻',
  weakness_break_efficiency: '弱点击破效率',
  weakness_implant: '弱点植入',
  toughness_reduce: '削韧',
  toughness_ignore: '无视韧性',
  effect_hit: '效果命中',
  effect_hit_rate: '效果命中',
  effect_res: '效果抵抗',
  debuff_apply: '施加负面',
  debuff_extend: '负面延长',
  debuff_resist: '负面抵抗',
  buff_advance: '增益延长',
  buff_extend: '增益延长',
  heal_percent: '治疗量%',
  heal_over_time: '持续治疗',
  outgoing_heal: '治疗加成',
  healing_received: '受治疗加成',
  shield_apply: '护盾',
  shield_strength: '护盾强度',
  cleanse: '净化',
  revive: '复活',
  energy_regen: '能量恢复效率',
  energy_restore: '回能',
  energy_drain: '能量回复(削减敌方)',
  energy_dependent: '依赖能量',
  sp_recovery: '回战技点',
  sp_consumption: '消耗战技点',
  sp_generation: '产出战技点',
  sp_positive: '战技点正收益',
  sp_negative: '战技点负收益',
  sp_neutral: '战技点中性',
  max_sp: '战技点上限',
  turn_advance: '行动提前',
  action_advance: '行动提前',
  action_delay: '行动延后',
  action_value: '行动值',
  extra_action: '额外行动',
  fua_trigger: '触发追击',
  dot_trigger: '触发持续伤害',
  aggro: '嘲讽值',
  taunt: '嘲讽',
  crowd_control: '控制',
  unknown: '其它',
}

export function statLabel(stat: string): string {
  return STAT_LABELS[stat] ?? stat
}

export const TARGET_LABELS: Record<string, string> = {
  self: '自身',
  one_ally: '单个队友',
  one_random_ally: '随机队友',
  all_allies: '全队',
  self_and_allies: '自身及队友',
  one_enemy: '单个敌方',
  all_enemies: '全体敌方',
  enemies_adjacent: '相邻敌方',
  random_enemy: '随机敌方',
  field_aoe: '战场范围',
}

export function targetLabel(target?: string): string {
  if (!target) return ''
  return TARGET_LABELS[target] ?? target
}

export const UPTIME_LABELS: Record<string, string> = {
  passive: '常驻',
  combat_start: '战斗开始',
  on_attack: '普攻时',
  on_skill: '战技时',
  on_ult: '终结技时',
  on_fua: '追击时',
  ult_active: '终结技期间',
  skill_active: '战技期间',
  on_hit_received: '受击时',
  on_ally_attack: '友方攻击时',
  on_enemy_debuff: '敌方负面时',
  on_wave_start: '波次开始',
  conditional: '条件触发',
  stack_based: '层数触发',
}

export function uptimeLabel(uptime?: string): string {
  if (!uptime) return ''
  return UPTIME_LABELS[uptime] ?? uptime
}

export const ZONE_LABELS: Record<string, string> = {
  base: '基础区',
  crit: '暴击区',
  dmg_bonus: '增伤区',
  def: '防御区',
  res: '抗性区',
  vuln: '易伤区',
  mitigation: '减伤区',
  break: '击破区',
  utility: '功能性',
}

export function zoneLabel(zone: string): string {
  return ZONE_LABELS[zone] ?? zone
}

// 标签(tags)的中文,缺失则原样返回
export const TAG_LABELS: Record<string, string> = {
  hyper_carry: '单核',
  single_dps_preferred: '偏单核',
  multi_dps_preferred: '偏多核',
  dual_dps: '双核',
  fua_team: '追击队',
  dot_team: '持续伤害队',
  break_team: '击破队',
  super_break_team: '超击破队',
  summon_team: '召唤物队',
  ult_team: '终结技流',
  crit_scaler: '暴击吃香',
  atk_scaler: '攻击力缩放',
  hp_scaler: '生命值缩放',
  def_scaler: '防御力缩放',
  break_scaler: '击破缩放',
  energy_dependent: '吃能量',
  debuff_dependent: '依赖负面',
  heal_dependent: '依赖治疗',
  shield_dependent: '依赖护盾',
  hp_loss_team: '扣血流',
  sp_positive: '战技点正收益',
  sp_negative: '战技点负收益',
  sp_neutral: '战技点中性',
  aoe: '群攻',
  blast: '扩散',
  single_target: '单体',
}

export function tagLabel(tag: string): string {
  return TAG_LABELS[tag] ?? tag
}

// 把 axes value(小数)格式化成展示文本:0.5 → 50%,2 → 2(整数视为点数/层数)
export function formatAxisValue(value?: number): string {
  if (value == null) return ''
  if (value > 0 && value < 3 && !Number.isInteger(value)) {
    return `${Math.round(value * 1000) / 10}%`
  }
  return String(value)
}

export function rarityStars(rarity: number): string {
  return '★'.repeat(rarity)
}

// 资源 CDN 模板(与后端 asset_paths 一致;直接拼 URL 省一次请求)
const ASSET_BASE = 'https://static.nanoka.cc/assets/hsr'

export function roundIconUrl(id: number): string {
  return `${ASSET_BASE}/avatarroundicon/${id}.webp`
}

export function drawCardUrl(id: number): string {
  return `${ASSET_BASE}/avatardrawcard/${id}.webp`
}

export function eidolonIconUrl(id: number, rank: number): string {
  return `${ASSET_BASE}/rank/_dependencies/textures/${id}/${id}_Rank_${rank}.webp`
}
