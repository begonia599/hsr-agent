# HSR 机制规格 v1

本文档记录本项目自己的机制理解和计算边界。外部项目只用于校对公开机制原理:

- Fribbels HSR Optimizer: https://github.com/fribbels/hsr-optimizer
- THCHelper: https://github.com/Tytyn000/THCHelper
- hsr-tct: https://github.com/j4rv/hsr-tct

本项目不复制这些项目的代码和数据模型。正式数据维护以 PostgreSQL 为准,JSON 只作为 LLM 抽取中间态、原始追溯和测试 fixture。

## 1. 第一版目标

M5 第一版只做局部数值校验:

- 给定攻击者、敌人默认参数和一组角色效果,计算常规直伤的分乘区变化。
- 已实现 DoT、击破、超击破、治疗、护盾和简化覆盖率的局部估算工具; 拉条、战技点、回能等仍作为 utility 解释。
- 不做完整遗器优化、不做完整行动轴、不模拟敌人行动、不模拟自动战斗。

## 2. 术语约定

### 2.1 攻击标签

- `basic`: 普攻
- `skill`: 战技
- `ult`: 终结技
- `fua`: 追加攻击
- `dot`: 持续伤害
- `break`: 击破伤害
- `super_break`: 超击破伤害
- `additional`: 附加伤害
- `any`: 未限定攻击类型

注意:

- 追加攻击是独立攻击事件,可吃"追加攻击伤害提高"等条件,也可能触发"我方发动追加攻击时"的效果。
- 附加伤害通常是附着在其他攻击上的额外伤害,不等同于追加攻击。除非文本明确写"追加攻击",抽取时不要标成 `fua`。
- "追加伤害"如果不是官方机制关键字,先按原文语义落到 `additional` 或保留 `condition_text` 待人工审核。

### 2.2 主要乘区

- `base`: 基础伤害/属性缩放区
- `crit`: 暴击期望区
- `dmg_bonus`: 增伤区
- `def`: 防御区
- `res`: 抗性区
- `vuln`: 易伤/受到伤害提高区
- `mitigation`: 减伤区
- `toughness`: 韧性击破状态区
- `break`: 击破/超击破专用区
- `heal`: 治疗区
- `shield`: 护盾区
- `utility`: 拉条、战技点、回能、削韧等非直接伤害效果

## 3. 常规直伤

### 3.1 总公式

常规直伤第一版按以下结构拆区:

```text
damage =
  base_damage
  * crit_multiplier
  * dmg_bonus_multiplier
  * defense_multiplier
  * resistance_multiplier
  * vulnerability_multiplier
  * mitigation_multiplier
  * toughness_state_multiplier
```

这个结构用于普攻、战技、终结技、追加攻击和多数直伤类附加伤害。特殊角色机制如果修改某个乘区,通过 `character_modifiers.modifier_zone` 表达。

### 3.2 基础伤害

```text
base_damage = scaling_stat * ability_multiplier + flat_damage
```

- `scaling_stat`: 攻击者当前 ATK/HP/DEF 等有效属性。
- `ability_multiplier`: 技能倍率,用小数表达,例如 240% 写 2.4。
- `flat_damage`: 技能额外固定伤害,没有则为 0。

第一版不自动解析每个技能的完整等级倍率,只在测试场景中显式传入倍率或使用默认倍率。

### 3.3 暴击期望

```text
crit_multiplier = 1 + min(crit_rate, 1.0) * crit_damage
```

- `crit_rate` 和 `crit_damage` 均用小数表达。
- DoT、击破、超击破默认不吃暴击,除非角色文本明确说明。
- 如果场景要求"必暴/不暴",后续可以增加 `crit_mode`,第一版先使用期望伤害。

### 3.4 增伤区

```text
dmg_bonus_multiplier = 1 + sum(applicable_damage_bonus)
```

增伤区包括通用增伤、元素增伤、普攻/战技/终结技/追加攻击/DoT/击破等类型增伤。是否适用由 `attack_tag`、`element_key`、`condition_jsonb` 决定。

### 3.5 防御区

第一版使用等价的等级公式:

```text
defense_multiplier =
  (attacker_level + 20)
  / ((enemy_level + 20) * (1 - def_reduction - def_ignore) + attacker_level + 20)
```

- `def_reduction`: 作用在敌人身上的减防。
- `def_ignore`: 攻击者无视防御。
- 两者在公式中同区相加。
- 第一版将有效防御下限钳制为 0,避免异常输入导致倍率失真。

### 3.6 抗性区

```text
resistance_multiplier = 1 - effective_resistance
effective_resistance = enemy_resistance - resistance_reduction - resistance_penetration
```

- `resistance_reduction`: 敌方抗性降低。
- `resistance_penetration`: 攻击者抗性穿透。
- 默认敌人抗性第一版由场景输入,没有输入时使用 20% 作为非弱点默认值。
- 对弱点/抗性/特殊敌人,后续由敌人配置覆盖。

### 3.7 易伤区

```text
vulnerability_multiplier = 1 + sum(applicable_vulnerability)
```

易伤是"敌方受到伤害提高"类效果,和我方增伤不是同一乘区。按元素、攻击标签或击破类型区分适用条件。

### 3.8 减伤区

```text
mitigation_multiplier = 1 - sum(applicable_damage_reduction)
```

减伤主要来自敌人或特殊机制。第一版只支持直接减伤,不处理复杂护盾/转移/分摊机制。

### 3.9 韧性击破状态倍率

```text
toughness_state_multiplier = 1.0 if enemy_broken else 0.9
```

这是敌人韧性未击破时常见的独立倍率。第一版只把它作为输入布尔值,不模拟削韧过程。

## 4. DoT

DoT 第一版按常规伤害结构处理,但默认:

- 不吃暴击区。
- 使用 `attack_tag = dot` 匹配 DoT 增伤、易伤、抗性、防御等条件。
- 是否吃韧性击破状态倍率作为可配置项保留,默认跟随常规伤害结构。

后续如果要做严肃 DoT 模拟,需要补:

- 触发频率
- 结算时点
- 持续回合
- 叠层规则
- 额外触发型 DoT

## 5. 击破伤害

击破伤害已接入局部精算工具,用于同一场景下比较 support modifiers 的乘区影响。

结构:

```text
break_damage =
  level_multiplier
  * element_break_multiplier
  * toughness_multiplier
  * (1 + break_effect)
  * (1 + break_damage_bonus)
  * defense_multiplier
  * resistance_multiplier
  * vulnerability_multiplier
  * mitigation_multiplier
```

```text
toughness_multiplier = enemy_max_toughness / 40 + 0.5
```

当前边界:

- 元素击破基础倍率采用公开机制常用表: 物理/火=2,风=1.5,冰/雷=1,量子/虚数=0.5。
- 不模拟各元素击破附加状态的后续结算。
- 角色额外击破伤害、特殊独立倍率和完整敌人库仍需后续阶段补充。

## 6. 超击破伤害

超击破伤害已接入局部精算工具,不承诺完整实战循环伤害。

结构:

```text
super_break_damage =
  level_multiplier
  * toughness_reduction / 10
  * super_break_base_multiplier
  * (1 + break_effect)
  * (1 + break_damage_bonus)
  * (1 + super_break_damage_bonus)
  * defense_multiplier
  * resistance_multiplier
  * vulnerability_multiplier
  * mitigation_multiplier
```

其中:

- `toughness_reduction`: 本次攻击造成的削韧值,受弱点击破效率、削韧提高、韧性易伤等影响。
- `super_break_base_multiplier`: 由触发超击破的角色或机制提供。
- 超击破通常不吃暴击。
- 工具默认削韧值为 30、超击破基础倍率为 1; 用户或 Agent 可显式传入。

## 7. 治疗

治疗第一版按以下结构:

```text
healing =
  (scaling_stat * ability_multiplier + flat_heal)
  * (1 + outgoing_healing_bonus)
  * (1 + healing_received_bonus)
```

- `outgoing_healing_bonus`: 治疗者治疗量提高。
- `healing_received_bonus`: 被治疗者受到治疗提高。
- 不处理溢出治疗转护盾、治疗触发攻击等特殊机制,这些作为 utility modifier 记录。
- 当前不导入真实角色面板/遗器/光锥; `scaling_stat`、`ability_multiplier`、`flat_heal` 由场景参数传入。

## 8. 护盾

护盾第一版按以下结构:

```text
shield =
  (scaling_stat * ability_multiplier + flat_shield)
  * (1 + shield_strength)
```

- `shield_strength`: 护盾量提高。
- 护盾持续、刷新、叠加覆盖规则由 `duration_key` 和 `stack_rule` 表达,不在第一版自动模拟。
- 当前不导入真实角色面板/遗器/光锥; `scaling_stat`、`ability_multiplier`、`flat_shield` 由场景参数传入。
- 抽取结果中"提供 X% 防御力 + 固定值护盾"这类基础公式不会被当成 `shield_strength` 乘区直接叠加,避免把基础护盾公式误算成护盾强效。

## 9. Utility 效果

以下效果第一版不折算为伤害,但必须结构化保存,供 Agent 解释队伍契合度:

- `action_advance`: 行动提前/拉条
- `action_delay`: 行动延后
- `speed_flat` / `speed_pct`: 速度变化
- `sp_recovery` / `sp_generation` / `sp_consumption`: 战技点
- `energy_restore` / `energy_regen`: 能量
- `toughness_reduce`: 削韧
- `weakness_implant`: 弱点植入
- `cleanse`: 解控/净化
- `revive`: 复活
- `aggro`: 受击概率变化

Agent 回答必须把 utility 和伤害乘区分开描述。例如花火对刃的价值不能只看伤害倍率,还要看拉条和战技点;但也要指出刃不主要吃攻击类加成。

## 10. 第一批样板角色

用于抽取、人工抽查和回归:

- 花火(1306):爆伤、拉条、战技点、单核辅助
- 知更鸟(1309):全队增伤、攻击、附加伤害、追击队触发
- 阮梅(1303):弱点击破效率、全抗性穿透、击破相关收益
- 布洛妮娅(1101):拉条、增伤、爆伤
- 刃(1205):HP 缩放、耗血、低攻击收益
- 黄泉(1308):负面状态、终结技核心
- 卡芙卡(1005):DoT 触发
- 流萤(1310):击破/超击破核心
- 罗刹(1203):治疗、自动治疗、攻击缩放治疗
- 藿藿(1217):治疗、回能、攻击提高
- 砂金(1304):护盾、追加攻击、效果抵抗/防御相关
- 符玄(1208):承伤、减伤、暴击率、生存辅助

## 11. 可信度规则

每条抽取出的 modifier 都有 `confidence` 和 `reviewed`:

- `reviewed=true`: 可作为高可信数值依据。
- `reviewed=false`: 可以用于候选解释,但回答中要使用"估算/待校验"措辞。
- 缺少明确数值的机制使用 `value_unit = unknown` 或 `value = NULL`,不要猜数值。

## 12. 后续扩展

M5 之后可单独开阶段处理:

- 简化行动轴
- 角色技能等级倍率表
- 用户面板/遗器导入
- 敌人库和弱点库
- 多轮循环/自动战斗模拟
