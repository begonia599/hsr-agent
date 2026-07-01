# HSR 伤害机制审计报告

> 目的:核对 `hsr-agent-go/internal/calc/calc.go` 的伤害计算机制与权威来源的一致性,修复确证问题。
> 方法:多源交叉验证工作流(5 个 agent)—— ① 克隆 **Fribbels HSR Optimizer** 源码提取其实际实现;② 查 **KQM Star Rail Library / Fandom Wiki / Prydwen** 权威公式;③ 对抗式复审本地 `calc.go` / `mechanics.go`;④ 综合终裁 + ⑤ 专职对抗验证击破系数(要求两独立来源互证)。
> 结论优先级:**Fribbels 源码 + 权威文档互相印证** > 单一来源 > 本地推断。分歧项由人工决策裁定。

---

## 一、总体结论

`calc.go` 的**乘区结构与主干公式扎实、与权威一致**(纯连乘、区内相加区间相乘)。不存在结构性大错。核对发现:**7 项确证问题已修复**,**3 项"疑似问题"经查证为正确、未改**(避免了改坏),**若干低优先项**记录待办。

**已验证正确、无需改动的部分**(交叉印证):
- 乘区连乘结构;`BaseDamage = ScalingStat×AbilityMult + Flat`
- **DEF 乘区** `(A+20)/((E+20)(1-defRed-defIgn)+(A+20))`:与官方 `1-DEF/(DEF+200+10L)` 数学等价(×10 约掉),且比 Fribbels 写死 L80 更通用
- **元素击破倍率**:物/火=2、风=1.5、冰/雷=1、量子/虚数=0.5 —— 与 Fribbels `calculateContext.ts:52-60` 及 Wiki 逐一吻合
- **LevelMultiplier(80)=3767.5533**:与数据挖掘常数吻合
- **超击破无元素乘区、无暴击、无增伤**:与 Fribbels `SuperBreakDamageFunction` 及 HoYoWiki 一致
- **超击破基数**:我们 `LevelMultiplier(80)×(削韧/10)` 与 Fribbels `(3767.5533/10)×toughnessReduction` **完全等价** —— 这是两套实现唯一能精确对齐的锚点,反证削韧量纲一致
- 暴击期望值 `1+CR×CD`、DoT 置暴击=1、治疗=outgoing×received、护盾独立

---

## 二、已修复的问题(7 项)

| # | 问题 | 严重度 | 佐证来源 | 修法 |
|---|---|---|---|---|
| 1 | **RES 乘区无 clamp** | 高 | Fandom/KQM | 有效抗性夹到 `[-1.0, 0.9]`(乘区 `[0.10, 2.00]`) |
| 2 | **击破/超击破未乘未破 0.9 通用减伤** | 高 | **Fribbels + Fandom 互证** | 击破/超击破补 `ToughnessStateMultiplier` |
| 3 | **减伤应连乘而非相加** | 高 | Fandom/KQM | `∏(1-rᵢ)` 取代 `1-Σr` |
| 4 | **`additional_dmg` 误入百分比增伤区** | 中 | Fribbels(独立结算) | 从增伤 case 移除 |
| 5 | **削韧 `*=`/`+=` 顺序敏感(非确定性)** | 中 | 本地审查 | 两阶段:`(base+Σreduce)×∏(1+eff)` |
| 6 | **attack_tag 为空时全放行专属增伤(高估)** | 中 | 本地审查 | 场景无 tag 时拒绝带具体 tag 的专属增伤 |
| 7 | **CritMultiplier 不 clamp 负暴伤** | 低 | 本地审查 | 加 `damage<0→0` 保护 |

**细节:**

**#1 RES clamp** — `ResistanceMultiplier` 现将 `(resistance-resReduction-resPen)` 夹到 `[-1.0, 0.9]`。惠及标准/DoT/击破/超击破四条路径(共用该函数)。堆多层穿透不再虚高。

**#2 击破 0.9** — 关键修正:此前我曾判断"击破省略 0.9 是对的",**工作流推翻了我** —— Fribbels `BreakDamageFunction`/`SuperBreakDamageFunction` 都以 `baseUniversalMulti`(未破0.9/已破1.0)起头,Fandom 亦原文"including the Break DMG from Toughness depletion"。已给 `BreakScenario` 加 `EnemyBroken` 字段,击破/超击破乘 `ToughnessStateMultiplier`。**默认 `EnemyBroken=true`(已破 ×1.0,向后兼容)**;经 `enemy_broken=false` 可模拟破韧那一发的 ×0.9。新增 `TestEstimateBreakDamageUnbroken` 覆盖。

**#3 减伤连乘** — `ApplyModifiers`/`ApplyBreakModifiers` 改为累乘存活率 `∏(1-rᵢ)`,再回填 `DamageReduction=1-存活率`。单来源时与旧行为等价(不破坏现有测试);两个 30% 减伤:旧 0.60 → 新 0.49(正确)。

**#6 attack_tag 空场景(⚠️ 行为变更,请复核)** — 这是唯一会改变现有 API 输出的修复。此前 `scenario.AttackTag==""` 时,`basic/skill/ult` 专属增伤会**全部叠加**导致高估;现改为"无 tag 时只放行通用增伤"。凡是不传 attack_tag 的估算(部分 `EstimateDamageGain`/`CompareCharacterFit` 路径)结果会**下降到更真实的水平**。属正确修复,但因改变数值,单独列出供你确认。

---

## 三、疑似问题经查证为正确、**未改**(3 项,避免改坏)

**A. 标准击破 `MaxToughnessMultiplier = maxToughness/40 + 0.5`(默认90→2.75)—— 正确,保留**
这是被特别锁定的争议点。两个 Verify agent 分歧:综合裁判判 uncertain(Fribbels 用 `/120@360`、文档用 `/40`,不能互证);但**专职对抗验证员判 matches**,理由:
- Fandom Wiki 逐字给出 `Max Toughness Multiplier = 0.5 + (Max Toughness_Target / 40)`,`Max Toughness` 取**显示韧性值**(90/120/150…);
- 我们的基数 `3767.5533` 正好与"`/40` + 显示值"口径**配套**;
- 我们自己的 `calc_test.go:92` 断言 2.75 佐证;
- Fribbels 的 `/120@360` 只是**内部单位缩放**的同一机制(基数与除数一起放大),不矛盾。

**人工裁定:采信专职验证员** —— 系数正确,**不改**。⚠️ 唯一约束:传入的 `maxToughness` 必须是**显示韧性值**(90),不能传 ÷10 的"内部单位"(9),否则击破伤害会低估约 4×。默认 90 与现有调用方都符合此约定。

**B. 元素击破 雷=1.0 —— 正确,保留**
本地审查曾指控"雷应为 1.5"。经 Fribbels 源码(Ice/Lightning=1.0)与 Wiki 双重核对,**该指控不成立**:雷(thunder/lightning)击破倍率就是 1.0(同冰)。**不改**(建议补一条 `ElementBreakMultiplier("lightning")==1` 回归测试)。

**C. `value_unit` 归一化 —— 数据已是 ratio,不需归一**
曾担心 DB 存"百分数"(50 表 50%)而 calc 当 ratio 会放大 100×。实查 DB:`percent` 单位值域 -0.4~14.625、`ratio` 单位 0.008~16,**均为小数(ratio)制**。入库已归一,calc 按 ratio 处理一致。**若贸然 /100 反而全错**,故不改(建议加一条断言/回归测试固化约定)。

---

## 四、低优先 / 待办(未改,记录)

- **缺 `finalDmgMulti`(最终伤害提高)/ `trueDmg`(真实伤害)/ `Weaken`(削弱)独立乘区**:Fribbels 有独立乘区,我们只有单一 `(1+DamageBonus)`。默认 0 时无影响;一旦对拍角色带这类效果(如银狼削弱、某些"最终伤害提高"trace)会分叉。需要时在 `Scenario` 增字段,各作独立乘区。
- **`super_break_base_multiplier` 语义二义**(`addSuperBreakBaseMultiplier` 的 `base+value-1`):默认 1.0 正确;建议明确"增量 vs 倍率"语义。
- **`isShieldStrengthBuff` 关键词白名单过窄 + 死代码**(`mechanics.go`):会漏算部分护盾强度 buff,取决于上游供给。
- **对拍口径**:Fribbels 默认敌人 L95/韧性360,我们默认 L80/韧性90;要与 Fribbels 数值对拍须先对齐敌人面板与韧性量纲。

---

## 五、验证

- `go build ./...` ✓、`go vet ./...` ✓、`gofmt -l` 无输出 ✓
- `go test ./...` 全过(含新增 `TestEstimateBreakDamageUnbroken`);现有断言(2.75、DEF=0.5、RES=0.9 等)均未回归
- 改动文件:`internal/calc/calc.go`、`internal/calc/calc_test.go`、`internal/tools/mechanics.go`

---

## 六、一句话

**主干公式可靠;修了 RES clamp、击破 0.9、减伤连乘、附加伤害归类、削韧顺序、空 tag 高估、暴伤保护 7 项;查证并保留了击破 maxToughness 系数、雷击破倍率、value_unit 三项(均正确,差点被误改);其余为低优先增强。** 对抗验证的价值在此次尤为明显 —— 它既揪出了我漏判的"击破该乘 0.9",又拦下了"按 Fribbels /120 去改一个其实正确的系数"。
