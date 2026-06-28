# HSR Agent 项目计划文档

> 目的:把"基于 nanoka.cc 数据 + DeepSeek/LLM 的崩坏星穹铁道配队/抽取建议 agent"的设计、数据、决策完整交接给后续实现者(Codex)。本文档自包含,实现时不需要回看本对话。

---

## 0. TL;DR

做一个**本地运行**的星穹铁道 AI 助手,流程是:

```
nanoka.cc 中文原始数据(已抓,zh 为主语料)
   ↓ enrich.py(一次性,用 LLM 把中文技能散文抽取成 axes)
PostgreSQL + pgvector(三张主表)
   ↓ tools.py(7 个 SQL/向量查询函数)
OpenAI-compatible tool-use 循环(默认 DeepSeek,Claude/Anthropic 仅可选兜底)
   ↓
用户问"花火配什么队" → AI 自主调用工具拉数据 → 给出有依据的建议
```

**关键设计选择**(从讨论中确定的,不要再翻案):

1. 不套 LangChain / LlamaIndex / adalflow,直接用厂商兼容 tool use;当前 Go 在线后端使用 OpenAI-compatible `/v1/chat/completions` tools
2. Postgres + pgvector 一个库搞定,不引入独立向量数据库
3. 主力查询走 SQL on 结构化 axes 字段,向量做"长尾意图"的兜底,**不要颠倒分工**
4. axes 预处理是项目的核心杠杆 — 字段定义得越规范,后面 SQL 越爽
5. 角色译名和机制描述优先用 `zh/` 国服语料(知更鸟,不是罗宾),避免英中翻译误差
6. `en/` 详情只作为 fallback / 调试对照 / 英文别名检索,不要作为 enrich 主输入
7. 主模型默认 DeepSeek / newapi 网关模型,Claude 只作为高难兜底或对照评测
8. 镜像数据保留版本号目录(`4.3.54/`),版本升级时重新跑 enrich,旧版本保留

---

## 1. 已有数据资产盘点

### 1.1 目录布局(项目根:`d:\aitest\`)

```
nanoka_hsr/
└── 4.3.54/                       # 当前 nanoka 版本号
    ├── character.json            # 95 角色总览(id → 基本元数据)
    ├── lightcone.json            # 165 光锥总览
    ├── relicset.json             # 60 遗器套装总览
    ├── en/                       # 英文详情
    │   ├── character/{id}.json   # 95 个角色详情(技能、星魂、行迹等)
    │   └── item.json             # 1574 个物品(材料/虚拟物品/凭证)
    ├── zh/                       # 同 en,简体中文(国服译名)
    ├── ko/                       # 韩文
    ├── ja/                       # 日文
    ├── assets/hsr/               # 全部图片资产(410 MB)
    │   ├── avatarroundicon/{id}.webp        # 圆头像(~10 KB)
    │   ├── avatarshopicon/{id}.webp         # 商店头像(~100 KB)
    │   ├── avataricon/avatar/{id}.webp      # 立绘头像(~20 KB)
    │   ├── avatardrawcard/{id}.webp         # 抽卡大立绘(~870 KB)
    │   ├── og/{id}.png                      # OG 宣传图,带水印(~1 MB)
    │   ├── rank/_dependencies/textures/{id}/{id}_Rank_{1..6}.webp  # 星魂大图
    │   ├── skillicons/*.webp                # 技能/行迹/星魂图标/属性图标
    │   ├── lightconemediumicon/{id}.webp    # 光锥中图(~60 KB)
    │   ├── lightconemaxfigures/{id}.webp    # 光锥大图(~320 KB)
    │   ├── itemfigures/{stem}.webp          # 物品/遗器套装图
    │   ├── pathicon/{lowercase_path}.webp   # 9 个命途图标
    │   ├── element/{lowercase_element}.webp # 7 个元素图标
    │   └── relicfigures/IconRelic{slot}.webp # 6 个遗器槽位图标
    └── failed_assets.txt         # CDN 上游真实缺失清单(58 张,正常)

scripts/scrape_nanoka_hsr.py        # JSON 抓取器(已跑通)
scripts/scrape_nanoka_hsr_assets.py # 图片抓取器(已跑通,含资源 URL 模板文档)
```

### 1.2 character.json 字段(总览,id → 基本信息)

```json
{
  "1309": {
    "release": 1715158800,              // unix epoch
    "icon": "robin",                    // 资源名(英文)
    "rank": "CombatPowerAvatarRarityType5",   // 稀有度枚举
    "baseType": "Shaman",               // 命途(Knight/Mage/Priest/Rogue/Shaman/Warlock/Warrior/Memory)
    "damageType": "Physical",           // 元素(Fire/Ice/Imaginary/Physical/Quantum/Thunder/Wind)
    "en": "Robin", "zh": "知更鸟", "ko": "로빈", "ja": "ロビン",
    "desc": "...",                      // 英文简介(其他语言在各语言子目录)
    "enhance": []                       // 加强/重塑标记
  }
}
```

### 1.3 角色详情 `<lang>/character/<id>.json` 字段

```
name desc rarity avatar_vo_tag sp_need base_type damage_type
chara_info { camp, va, stories, voicelines }
ranks      { "1".."6": {name, desc, icon, params} }    # 星魂
skills     { id: {name, desc, type, icon, max_level, level_data, ...} }
            # type ∈ Normal/BPSkill/Ultra/Maze/Talent/QTE/...
skill_trees { point01..point18: {1..N: {name, desc, icon, ...}} }
            # point01 普攻,point02 战技,point03 终结技,point04 天赋
            # point05 秘技,point06-08 三个加点小天赋(SkillTree1/2/3)
            # point09+ 属性突破点
enhanced unique memosprite   # 通常为空,部分角色有
stats      { "0".."6": [HP, ATK, DEF, SPD, CRIT, CRIT_DMG, AGGRO, BASE_AGGRO] }
            # 0=初始, 1=20升级, 2=30升级, ..., 6=80满级
relics     { avatar_id, set4_id_list, set2_id_list, property_list3..6, sub_affix_property_list, score_rank_list }
            # nanoka 整理的"推荐遗器/属性"
lightcones [21002, 23005, 24002]   # 推荐光锥 id 列表
teams      [{team_id, position, member_list, backup_list1..3, backup_group_list1..3}]
            # 推荐队伍构筑
skin       { skin_id: {...} }
```

**重要**:`teams.member_list` 是当前队伍的核心阵容;`backup_list*` 是替代位的候选 id;`backup_group_list*` 是"群组角色 id"(角色类别 id,如 101=主 C 群组)。**这是 nanoka 整理的玩家配队共识,后续 `team_cooccur` 表的主要来源**。

### 1.4 lightcone.json 字段

```json
{
  "21002": {
    "rarity": "...", "path": "...",
    "en": "...", "zh": "...", ...,
    "desc": "...",                // 光锥效果(散文)
    "params": [...]                // 数值参数,通常按叠加层数 1-5
  }
}
```

### 1.5 relicset.json 字段

```json
{
  "101": {
    "icon": "SpriteOutput/ItemIcon/71000.png",   // 原游戏路径,实际取图用 itemfigures/{set_id}
    "set": [
        {"num": 2, "desc": "..."},
        {"num": 4, "desc": "..."}
    ],
    "en": "...", "zh": "...", ...
  }
}
```

### 1.6 已确认的"坑"和注意事项

| 项 | 说明 |
|---|---|
| **访问 nanoka.cc** | 默认拒绝无 UA 请求(403),必须带 `User-Agent: Mozilla/5.0` |
| **版本号定位** | 从 `https://hsr.nanoka.cc/character` HTML 里正则 `static\.nanoka\.cc/hsr/(\d+\.\d+\.\d+)/` 抓 |
| **资源 URL 规则** | 文字数据在 `static.nanoka.cc/hsr/<version>/...`,图片在 `static.nanoka.cc/assets/hsr/...`(图片**不带**版本号) |
| **pathicon / element** | 文件名**强制小写**(`shaman.webp` 不是 `Shaman.webp`) |
| **遗器槽位文件名** | `Head/Hands/Body/Foot/Neck/Goods` — 注意 JS 源码里 `Hand→Hands`、`Object→Goods` 的特判 |
| **图标后缀转换** | JSON 里 `icon: "SkillIcon_1309_Normal.png"`,CDN 实际是 `.webp`(去后缀再加 `.webp`) |
| **特殊角色** | id 1014 (Saber) / 1015 (Archer) 是 FATE 联动;1506-1510 是变体(银狼LV.999, 千冶•刃, 远坂凛, 吉尔伽美什, 姬子•启行);8001-8010 是开拓者多形态(name 字段为 `{NICKNAME}`,需要从 game data 补名字) |
| **{NICKNAME} 占位符** | 开拓者角色 (id 8001-8010) 的 name 字段是占位符,需要自定义显示文案 |
| **{RUBY_B/E}** | 日文字段含注音标记(`{RUBY_B#みつき}三月{RUBY_E#}なのか`),展示时需 strip 或转 HTML ruby |
| **`<unbreak>` 标签** | desc 字段里有 `<unbreak>67</unbreak>` 之类的私有标签,渲染时处理 |
| **\n 转义** | JSON 里换行是字面 `\\n`,反序列化后再处理 |
| **robots.txt** | nanoka 声明 `Content-Signal: search=yes, ai-train=no` — 项目个人使用 OK,但**不要把这些数据拿去训练任何模型** |
| **58 张缺失** | `failed_assets.txt` 列出来的是 CDN 上游本来就没的,不要再重抓 |
| **抽取与稀有度枚举** | `CombatPowerAvatarRarityType5/4` 映射 5星/4星;光锥三档 5/4/3 同理 |

---

## 2. 架构与设计决策

### 2.1 为什么不直接用 RAG / 不引入向量数据库为主力

调研过 [deepwiki-open](https://github.com/AsyncFuncAI/deepwiki-open) 这类项目,**它本质是 retrieve-then-answer**(adalflow + FAISS,把仓库切块后做相似度检索)。这个模式**不适合本项目**,原因:

1. **HSR 配队问题是多跳关系查询,不是相似度查询**
   - 「知更鸟配什么队」需要:查知更鸟需求 → 找匹配 DPS → 看队伍 SP/破韧覆盖 → 排序候选
   - 向量 top-k 只能返回"看起来相关"的 chunk,**漏掉的角色 LLM 不会知道**
2. **数据规模太小**,95 角色 × 1-2 KB enriched 数据 ≈ 200 KB,**全量塞 system prompt 并利用供应商缓存/Context Caching,比向量检索更快更准**
3. **关键关系是结构化的**:命途、元素、buff 轴、推荐共现 — 这些 SQL 表达比向量好得多

**结论**:SQL 主力(精确属性匹配 + 多跳 JOIN),向量做兜底(模糊语义意图)。

### 2.2 为什么不用 LangChain / LlamaIndex / adalflow

**直接用兼容 Chat Completions 的 tool use 循环;模型供应商通过配置切换**。理由:

- 项目规模太小不需要框架抽象
- 框架隐藏 prompt 和 tool schema,出错难调
- 我们的核心难点不在编排,在**axes 数据规范化**(框架帮不上忙)
- newapi 网关的 OpenAI-compatible tool calls 已验证可用;Anthropic-compatible 接口在该网关上未稳定触发 tool_use,暂不作为默认在线 Agent 协议
- 实现时不要把业务逻辑写死到某个厂商 SDK,封装一个薄的 `llm_client`

**唯一值得引入的依赖**:
- 普通 HTTP client — 用于 OpenAI-compatible `/v1/chat/completions`
- `psycopg[binary]` 或 `psycopg2-binary` — PG 驱动
- `pgvector` (Python 客户端) — pgvector 字段适配
- (可选) `chromadb` — 不要,pgvector 取代之

### 2.3 axes 是什么、为什么是项目的核心

**axes = 把中文散文技能描述抽取/归一化成结构化、受控词表化、可被 SQL JOIN 的能力字段**。

举例:知更鸟 (id 1309) 大招描述是

> "开启演奏期间,我方全体造成的伤害提高 50%,且为我方全体施加额外攻击力的加成"

抽取后的 axes:

```json
{
  "provides": [
    {"stat": "dmg_percent",   "target": "all_allies", "value": 0.5, "uptime": "ult_active"},
    {"stat": "atk_flat_scaling_from_self_atk", "target": "all_allies", "uptime": "ult_active"}
  ],
  "needs": [
    {"stat": "atk_main_stat", "reason": "scales own ATK to team"},
    {"stat": "follow_up_team", "reason": "coordinated ult phase"}
  ],
  "tags": ["sub_dps_amplifier", "fua_team", "ult_dependent"]
}
```

**没有 axes 的成本**:每次回答问题都要让 LLM 重新阅读所有候选角色的散文技能,容易漏字段、慢、贵。

**有 axes 的红利**:一条 SQL 就能筛"所有给全队 ATK% 的角色,排除消耗 SP 大的",AI agent 直接拿候选集做精排和解释。

### 2.4 工具(tool)是 AI 的"延迟读取"机制

回答用户问题前,agent 不是把全部数据灌进上下文,而是**根据问题自主调工具**。系统 prompt 强制至少考察 N 个候选,等价于"先 Grep 找完所有调用点再下结论"。这正是我们要复刻的"AI 跨文件关联推理"机制。

---

## 3. 设计规格

### 3.1 axes 受控词表(v1 草案,实施时可微调)

> 实施时:`enrich.py` 必须强制 LLM 只能用以下枚举值,**禁止自由文本字段**(`condition` 例外)。词表写到 `schemas/axes_vocab.py`,作为 LLM prompt 的一部分。

#### `stat`(能力维度,~40 个)

```
# 数值类
atk_percent atk_flat hp_percent hp_flat def_percent def_flat speed_flat speed_percent
crit_rate crit_dmg break_eff effect_hit effect_res
dmg_percent dmg_taken_reduce def_ignore res_pen heal_percent shield_strength
sp_recovery energy_regen ult_dmg fua_dmg dot_dmg break_dmg true_dmg

# 控制/状态类
weakness_implant  # 弱点植入
cleanse           # 净化
revive
buff_advance      # buff 延长
debuff_extend     # debuff 延长
turn_advance      # 行动提前
turn_delay        # 行动延后
toughness_reduce  # 削韧
shield_apply      # 护盾施加
heal_over_time
fua_trigger       # 触发追击
extra_action      # 额外动作
energy_drain      # 能量减少(对敌)

# 元规则
def_unique_buff   # 独占类 buff (互斥需求)
```

#### `target`(作用对象)

```
self  one_ally  one_random_ally  all_allies  self_and_allies
one_enemy  all_enemies  enemies_adjacent  random_enemy
field_aoe        # 战场 AoE 持续区域
```

#### `uptime`(持续 / 触发条件)

```
passive                  # 被动常驻
combat_start             # 战斗开始
on_attack                # 普攻时
on_skill                 # 战技时
on_ult                   # 终结技时
on_fua                   # 追击时
ult_active               # 终结技激活期间
skill_active             # 战技激活期间
on_hit_received          # 受击时
on_ally_attack           # 友方攻击时
on_enemy_debuff          # 敌方负面时
conditional              # 复杂条件(放 condition 字段)
stack_based              # 层数触发(参数 max_stacks/per_stack_value)
```

#### `kind`(在 character_axes 表里区分)

```
provides   # 提供能力(我加成 / 我治疗 / 我护盾)
needs      # 需求(我吃什么 / 队伍需要什么配合)
restricts  # 限制(独占 buff / 必须单 DPS)
tag        # 风格标签(fua_team / hyper_carry / dot_dps / break_dps / aggro_tank)
```

#### `role`(角色定位,characters.roles 数组)

```
main_dps   sub_dps   amplifier   debuffer
sustain_healer  sustain_shielder  sustain_hybrid
remembrance       # 记忆主战
generalist        # 多功能
break_specialist  # 击破特化
```

### 3.2 PostgreSQL Schema

#### 3.2.1 启用扩展

```sql
CREATE EXTENSION IF NOT EXISTS vector;        -- pgvector
CREATE EXTENSION IF NOT EXISTS pg_trgm;       -- 模糊文本搜索
```

#### 3.2.2 主表

```sql
-- ============================================================
-- 角色主表
-- ============================================================
CREATE TABLE characters (
    id              INT PRIMARY KEY,                -- nanoka 角色 id
    version         TEXT NOT NULL,                  -- e.g. '4.3.54'
    release_at      TIMESTAMPTZ,
    icon_name       TEXT,                           -- avatar_vo_tag (e.g. 'robin')
    rarity          SMALLINT NOT NULL CHECK (rarity IN (4, 5)),
    path            TEXT NOT NULL,                  -- Knight/Mage/...
    element         TEXT NOT NULL,                  -- Fire/Ice/...
    name_zh         TEXT NOT NULL,
    name_en         TEXT NOT NULL,
    name_ko         TEXT,
    name_ja         TEXT,
    desc_zh         TEXT,
    desc_en         TEXT,
    sp_need         INT,
    roles           TEXT[] NOT NULL DEFAULT '{}',   -- 见上 role 词表
    raw_zh          JSONB NOT NULL,                 -- 原始 detail (zh)
    raw_en          JSONB NOT NULL,
    axes            JSONB NOT NULL DEFAULT '{}',    -- 见上 axes 结构
    skill_text_zh   TEXT NOT NULL DEFAULT '',       -- 拼接的技能原文,给 pg_trgm
    skill_text_en   TEXT NOT NULL DEFAULT '',
    embedding       vector(1024),                   -- 角色综合描述向量(模型自选,记录维度)
    is_trailblazer  BOOLEAN NOT NULL DEFAULT FALSE,
    is_collab       BOOLEAN NOT NULL DEFAULT FALSE, -- Saber/Archer
    is_variant      BOOLEAN NOT NULL DEFAULT FALSE  -- 1506-1510 等变体
);

CREATE INDEX idx_chars_roles ON characters USING gin (roles);
CREATE INDEX idx_chars_axes ON characters USING gin (axes jsonb_path_ops);
CREATE INDEX idx_chars_skilltext_zh ON characters USING gin (skill_text_zh gin_trgm_ops);
CREATE INDEX idx_chars_skilltext_en ON characters USING gin (skill_text_en gin_trgm_ops);
CREATE INDEX idx_chars_embedding ON characters USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_chars_path ON characters (path);
CREATE INDEX idx_chars_element ON characters (element);


-- ============================================================
-- 能力轴展开表(供 JOIN)
-- 来源:enrich.py 把 characters.axes 拍平后插入
-- ============================================================
CREATE TABLE character_axes (
    char_id   INT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind      TEXT NOT NULL,                        -- provides/needs/restricts/tag
    stat      TEXT NOT NULL,                        -- 见 stat 词表
    target    TEXT,                                 -- 见 target 词表(tag/restricts 时可空)
    value     NUMERIC,
    uptime    TEXT,                                 -- 见 uptime 词表
    condition TEXT,                                 -- 自由文本兜底(复杂条件)
    PRIMARY KEY (char_id, kind, stat, COALESCE(target, ''), COALESCE(uptime, ''))
);

CREATE INDEX idx_caxes_kind_stat ON character_axes (kind, stat);
CREATE INDEX idx_caxes_kind_target ON character_axes (kind, target);


-- ============================================================
-- 光锥
-- ============================================================
CREATE TABLE lightcones (
    id              INT PRIMARY KEY,
    version         TEXT NOT NULL,
    rarity          SMALLINT NOT NULL CHECK (rarity IN (3, 4, 5)),
    path            TEXT NOT NULL,
    name_zh         TEXT NOT NULL,
    name_en         TEXT NOT NULL,
    desc_zh         TEXT,
    desc_en         TEXT,
    raw             JSONB NOT NULL,
    axes            JSONB NOT NULL DEFAULT '{}',
    embedding       vector(1024)
);

CREATE INDEX idx_lc_path ON lightcones (path);
CREATE INDEX idx_lc_axes ON lightcones USING gin (axes jsonb_path_ops);
CREATE INDEX idx_lc_embedding ON lightcones USING hnsw (embedding vector_cosine_ops);


-- ============================================================
-- 遗器套装
-- ============================================================
CREATE TABLE relic_sets (
    id              INT PRIMARY KEY,
    version         TEXT NOT NULL,
    kind            TEXT NOT NULL,                  -- 'cavern' (4件套) | 'planar' (2件套)
    name_zh         TEXT NOT NULL,
    name_en         TEXT NOT NULL,
    set2_desc       TEXT,
    set4_desc       TEXT,                           -- planar 套时为 null
    raw             JSONB NOT NULL,
    axes            JSONB NOT NULL DEFAULT '{}',
    embedding       vector(1024)
);

CREATE INDEX idx_rset_axes ON relic_sets USING gin (axes jsonb_path_ops);


-- ============================================================
-- 推荐配置(nanoka 整理的)
-- ============================================================
CREATE TABLE character_recommendations (
    char_id           INT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    recommend_kind    TEXT NOT NULL,        -- 'lightcone' | 'relic_set4' | 'relic_set2' | 'main_stat' | 'sub_affix' | 'score'
    item_id           INT,                  -- 对 lightcone/relic_set 是 id;对 main_stat 是 axis stat 名(NULL 时看 payload)
    rank              INT NOT NULL DEFAULT 0, -- 推荐排序(0=最佳)
    payload           JSONB                 -- 复杂时存原始字段
);

CREATE INDEX idx_crec_char ON character_recommendations (char_id, recommend_kind);


-- ============================================================
-- 队伍共现(从 character.teams 字段统计出来)
-- ============================================================
CREATE TABLE team_cooccur (
    char_a            INT NOT NULL REFERENCES characters(id),
    char_b            INT NOT NULL REFERENCES characters(id),
    weight            INT NOT NULL,         -- 共现次数,position=1 时权重 ×2
    is_main_lineup    BOOLEAN NOT NULL DEFAULT FALSE,  -- 是 member_list 还是 backup_list
    PRIMARY KEY (char_a, char_b)
);

CREATE INDEX idx_coocc_a ON team_cooccur (char_a, weight DESC);


-- ============================================================
-- 物品/材料(角色突破等)
-- ============================================================
CREATE TABLE items (
    id              INT PRIMARY KEY,
    item_sub_type   TEXT,
    purpose_type    INT,
    rarity          TEXT,
    name_zh         TEXT NOT NULL,
    name_en         TEXT NOT NULL,
    figure_stem     TEXT                   -- 资源文件名(无后缀)
);


-- ============================================================
-- 资产路径表(便于工具引用,前端按需取)
-- ============================================================
CREATE TABLE asset_paths (
    entity_kind   TEXT NOT NULL,           -- 'character' | 'lightcone' | 'relic_set' | 'item' | 'path' | 'element' | 'slot'
    entity_id     TEXT NOT NULL,           -- 数字 id 或枚举值(string)
    variant       TEXT NOT NULL,           -- 'round' | 'shop' | 'avatar' | 'drawcard' | 'og' | 'rank_1'..'rank_6' | 'medium' | 'maxfigure' | 'skill_<name>'
    local_path    TEXT NOT NULL,           -- 相对项目根: 'nanoka_hsr/4.3.54/assets/hsr/...'
    cdn_url       TEXT NOT NULL,           -- 公网回源 URL
    bytes         INT,
    PRIMARY KEY (entity_kind, entity_id, variant)
);

CREATE INDEX idx_assets_entity ON asset_paths (entity_kind, entity_id);
```

#### 3.2.3 一些用得上的视图

```sql
-- 给定一个角色,它能提供的所有 buff(展开 axes)
CREATE VIEW v_provides AS
SELECT char_id, stat, target, value, uptime, condition
FROM character_axes WHERE kind = 'provides';

-- 给定一个角色,它需要什么
CREATE VIEW v_needs AS
SELECT char_id, stat, target, value, uptime, condition
FROM character_axes WHERE kind = 'needs';

-- 一句 SQL:谁给"全队 ATK%"
CREATE VIEW v_team_atk_buffers AS
SELECT c.id, c.name_zh, c.rarity, ca.value, ca.uptime
FROM characters c
JOIN character_axes ca ON ca.char_id = c.id
WHERE ca.kind = 'provides'
  AND ca.stat = 'atk_percent'
  AND ca.target IN ('all_allies', 'self_and_allies');
```

### 3.3 工具集(Agent 调的函数)

> 在线后端实现在 `hsr-agent-go/internal/tools`,每个工具一个函数,签名稳定。返回 JSON 友好的 struct / list,**不返回 ORM 对象**。所有工具都注册到 OpenAI-compatible chat completions 的 `tools` 参数。

| 工具名 | 入参 | 出参 | 实现 |
|---|---|---|---|
| `get_character` | `query: str` (id 或名字) | character dict | SQL ILIKE name_zh/name_en + id lookup |
| `search_by_role` | `role, element=None, path=None, rarity=None` | character list | SQL with array ANY + filters |
| `find_buffers_for` | `axis: str, target: str = 'all_allies'` | character list | JOIN character_axes,按 value DESC |
| `find_needs` | `char_id: int` | list of needs | SELECT FROM character_axes kind=needs |
| `find_synergies` | `char_id: int, k: int = 8` | list of (char, score, reasons) | 综合:axes 匹配 + team_cooccur + 同元素/命途 加权 |
| `suggest_team` | `core_id: int, slots: int = 4, exclude: list[int] = []` | list of team plans | 启发式:DPS 锁定 → 找 buffer/debuffer → 找 sustain,axes 互补 |
| `co_occurrence` | `char_id: int, k: int = 10` | list of (char_id, weight) | SELECT FROM team_cooccur |
| `recommend_lightcones` | `char_id: int` | list of lc with score | character_recommendations + axes 加权 |
| `recommend_relics` | `char_id: int` | list of relic sets | character_recommendations |
| `semantic_search` | `query: str, kind: str, k: int = 10` | list of matches | pgvector cosine,kind ∈ character/lightcone/relic_set |
| `compare_characters` | `id_a: int, id_b: int` | side-by-side dict | 拉两份 + axes diff |
| `get_assets` | `entity_kind: str, entity_id: str, variants: list[str] = None` | dict variant → path/url | asset_paths 表 lookup |

**工具命名原则**:动词 + 名词,LLM 看 schema 就能猜功能;参数尽量原生类型;返回值结构平,避免嵌套过深。

### 3.4 Agent 循环

```go
# 当前实现: hsr-agent-go/internal/agent
messages := []message{systemPrompt, userMessage}
for step := 0; step < 8; step++ {
    resp := chatCompletions(messages, tools)
    if len(resp.ToolCalls) == 0 {
        return resp.Content
    }
    for _, call := range resp.ToolCalls {
        result := dispatchTool(call.Function.Name, call.Function.Arguments)
        messages = append(messages, toolResult(call.ID, result))
    }
}
```

**关键点**:
- `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` / `LLM_API_FORMAT` 控制供应商;Go agent 当前要求 `LLM_API_FORMAT=openai`
- API key 只从进程环境变量读取,不要写入 `.env`、日志或计划文档
- 工具结果以 JSON 字符串作为 `role=tool` 消息塞回去,保留中文
- 限制最多 8 轮工具调用,避免模型无限循环
- 主模型默认 DeepSeek / newapi 网关模型;Claude 只作为可选兜底

### 3.5 enrich.py 设计

```
输入: nanoka_hsr/4.3.54/zh/character/<id>.json
原则: `zh/` 是 enrich 的主语料;`en/` 只保留为 fallback、调试对照和英文别名检索,避免先英后中导致国服机制/译名偏差
处理:
  1. 拼接所有技能/星魂/行迹的 desc 字段
  2. 把上面的 axes 词表 + 这段散文,丢给 LLM(默认 DeepSeek)
  3. 强制输出 JSON schema(用 tool use 的 schema 约束)
输出: enriched/4.3.54/character/<id>.json (axes dict)
完成后: load.py 把 enriched JSON + 原始 JSON 合并 UPSERT 进 PG
```

**enrich.py 的 prompt 框架**(实现时可调):

```
你是 HSR 数据分析师。请把下面的中文角色技能描述,抽取/归一化成结构化 JSON。
严格遵守以下规则:
- stat 只能从以下词表选: [<列出 stat 词表>]
- target 只能从: [...]
- uptime 只能从: [...]
- 不在词表里的能力,放进 tags 字段(自由文本)
- 数值统一为小数(50% 写 0.5)
- 不确定的字段不要瞎填,留空或放进 condition
- 不要按英文名或英文机制改写;角色名、技能名、机制描述以国服中文为准

角色 id: {id}
角色名: {name_zh}
命途: {path}
元素: {element}

技能描述:
{concatenated_skill_text}

输出 schema: {schema_json}
```

**为什么用 tool use 约束 JSON**:LLM 自由生成 JSON 容易格式错误,用 tool use 强制 schema 验证、自动重试。

### 3.6 项目目录结构

```
d:\aitest\
├── nanoka_hsr/                  # (已有)抓取的数据
│   └── 4.3.54/
├── enriched/                    # axes 预处理产物
│   └── 4.3.54/
│       ├── character/{id}.json
│       ├── lightcone/{id}.json
│       └── relic_set/{id}.json
├── scripts/
│   ├── scrape_nanoka_hsr.py     # (已有)JSON 抓取
│   ├── scrape_nanoka_hsr_assets.py  # (已有)图片抓取
│   ├── migrate.py               # 执行 migrations/*.sql
│   ├── enrich.py                # 用 LLM(默认 DeepSeek)跑 axes 预处理
│   ├── load.py                  # axes + 原始数据 → PG (UPSERT)
│   ├── embed.py                 # 为 characters/lightcones/relic_sets 生成向量
│   └── compute_cooccur.py       # 从 raw.teams 统计 team_cooccur
├── migrations/
│   ├── 001_schema.sql           # 表 + 索引
│   ├── 002_views.sql            # 视图
│   └── 003_seed_shared.sql      # 命途/元素/槽位枚举数据(如有)
├── schemas/
│   └── axes_vocab.py            # 受控词表(stat/target/uptime/role 常量)
├── hsr_agent/
│   ├── db.py                    # PG 连接 + 简单查询封装
│   ├── tools.py                 # Agent 工具函数
│   ├── llm_client.py            # LLM 供应商薄封装(base_url/model/api_key)
│   ├── tools_schema.py          # Python 版 tool schema(暂缓;Go 在线后端已实现)
│   ├── agent.py                 # tool-use loop
│   └── prompts.py               # system prompt + enrich prompt 模板
├── hsr-agent-go/                # 在线 Agent 后端(Go)
│   ├── cmd/hsr-agent/main.go    # 后端入口
│   └── internal/{config,db}/     # 配置和 PG 连接池
├── docker-compose.yml           # PG + pgvector(`pgvector/pgvector:pg17` 镜像)
├── pyproject.toml               # Python 离线脚本依赖
├── requirements.lock            # Python 依赖锁定快照
├── .env.example                 # LLM_BASE_URL, LLM_API_KEY, LLM_MODEL, DATABASE_URL
└── README.md
```

---

## 4. 实施计划(分里程碑)

### M0 — 基础设施(0.5 天)

- [x] `docker-compose.yml`:`pgvector/pgvector:pg17`,容器 5432 / 宿主 55432,持久化到 `./pgdata`
- [x] `pyproject.toml` + `requirements.lock`,Python 3.11+
- [x] `migrations/001_schema.sql` 跑通,所有表 + 索引创建成功
- [x] `hsr_agent/db.py` 提供 `get_conn()` / `execute()` / `fetch()` 三个辅助
- [x] `.env.example` + `README.md` 启动说明
- [x] Go 后端基础骨架(`hsr-agent-go`)可连接 PG

**验收**:`docker compose up -d` + `python scripts/migrate.py` 一键起,`\d characters` 显示表结构。已通过;因本机 5432 被占用,宿主端口使用 55432。

### M1 — 数据装载(无 axes 版本)(0.5 天)

- [x] `scripts/load.py`:
  - 读 `nanoka_hsr/4.3.54/character.json` 和 `<lang>/character/<id>.json`
  - `zh/` 详情作为主语料写入 `raw_zh` / `name_zh` / `desc_zh` / `skill_text_zh`
  - `en/` 详情只保留为 `raw_en` / `name_en` / `desc_en` / `skill_text_en`,供 fallback 和英文别名检索
  - UPSERT 到 `characters`(axes 字段先留空 `{}`)
  - 同步处理 `lightcones`、`relic_sets`、`items`
  - 拼接 `skill_text_zh` / `skill_text_en`
  - 写入 `character_recommendations`(从 detail JSON 的 `lightcones`/`relics` 字段)
  - 处理 `is_trailblazer/is_collab/is_variant` 标记(id 8001-8010 / 1014-1015 / 1506-1510)
- [x] `scripts/compute_cooccur.py`:遍历 detail JSON 的 `teams` 字段,累加权重
- [x] `scripts/build_asset_paths.py`:扫 `assets/hsr/` 目录,生成 `asset_paths` 行
- [x] 处理特殊字符串:
  - `\\n` → 实际换行
  - `<unbreak>X</unbreak>` → `X`(skill_text 字段)
  - `{RUBY_B#X}` / `{RUBY_E#}` → 去掉(日文)
  - `{NICKNAME}` → 「开拓者」(name_zh) / 「Trailblazer」(name_en)

**验收**:`SELECT COUNT(*) FROM characters` = 95;`SELECT COUNT(*) FROM lightcones` = 165;查 1309 看 `raw_zh.teams` 有数据。已通过:characters=95, lightcones=165, relic_sets=60, items=1574, character_recommendations=1495, team_cooccur=2038, asset_paths=4087。

### M2 — axes 预处理(1-2 天,核心)

- [x] `schemas/axes_vocab.py` 定型受控词表
- [x] `scripts/enrich.py`:
  - [x] 加载词表
  - [x] 对每个角色 detail,只从 `zh/character/<id>.json` 拼接技能/星魂/行迹文本作为主输入
  - [x] 英文 detail 不进入默认 enrich prompt;只有中文缺字段或人工排查时才读取
  - [x] 调 LLM(默认 DeepSeek/OpenAI-compatible API;保留 Anthropic 格式作为可选离线预处理路径)
  - [x] 保存到 `enriched/4.3.54/character/{id}.json`
  - [x] **从知更鸟 (1309)、希儿 (1102)、丹恒•饮月 (1213)、花火 (1306) 这 4 个角色样板做起**,人工抽查 axes 是否合理,调词表 → 再批量
  - [x] 95/95 角色 axes 批量生成并装载完成
- [x] `scripts/load_axes.py`:把 enriched JSON 合并进 PG
  - [x] UPDATE `characters.axes`
  - [x] 拍平到 `character_axes` 表
- [x] 同样对 `lightcones` / `relic_sets` 做一遍 axes(规模小,简单);这是 M7 推荐接口上线前的阻断项,否则 `recommend_lightcones` / `recommend_relics` 只能给 nanoka 排名,解释和精排质量不足。
  - [x] `migrations/003_equipment_axes.sql`:新增 `equipment_axes`
  - [x] `scripts/enrich_equipment.py`:遗器从中文套装效果规则抽取;光锥当前因本地 `lightcone.json` 缺少效果文本,只做 nanoka 推荐角色画像反推的弱 axes
  - [x] `scripts/load_equipment_axes.py`:写回 `lightcones.axes` / `relic_sets.axes`,并拍平到 `equipment_axes`
  - [x] Go `recommend_lightcones` / `recommend_relics`:返回 equipment axes、综合 score、reasons、matched_provides / matched_requirements / matched_tags

**当前状态**:`enrich.py --dry-run --ids 1309` 已通过;95/95 角色 axes 已生成并装载,`characters` 中 `axes <> '{}'` 的角色数为 95,`character_axes` 行数为 2156。装备 axes 已完成 v1:lightcones_with_axes=165,relic_sets_with_axes=60,equipment_axes=1402。日志见 `logs/enrich_worker.log`,状态见 `logs/enrich_worker_state.json`。

**验收**:
- 抽查知更鸟的 axes,`provides` 至少有 `atk_percent`、`dmg_percent` 两条针对 `all_allies`
- SQL `SELECT name_zh FROM characters c JOIN character_axes ca ON ca.char_id=c.id WHERE ca.kind='provides' AND ca.stat='atk_percent' AND ca.target='all_allies'` 应返回至少 5 个合理结果(知更鸟、花火、布洛妮娅等)

### M3 — 向量(0.5 天;当前为临时链路)

- [x] 临时 embedding 模型:当前使用本地 `local-hash-ngram-v1` 1024 维,不调用外部 API;它只用于验证 pgvector 链路和机制词召回,不是最终语义模型。
- [ ] M7 暴露 HTTP semantic search 前,必须替换成真实 embedding provider,或显式禁用 semantic search API。
- [x] schema 里的 `vector(1024)` 已匹配当前本地 embedding
- [x] `scripts/embed.py`:
  - [x] 对每个角色:拼接 `name_zh + name_en + path + element + roles + axes + skill_text_zh` 做 embedding
  - [x] 光锥/遗器套装同样写入 embedding
  - [x] 写入 `embedding` 列
- [x] HNSW 索引已在 `001_schema.sql` 创建
- [x] Go 工具层已实现 `semantic_search`

**当前状态**:`character_embeddings=95`, `lightcone_embeddings=165`, `relic_set_embeddings=60`。`semantic_search("需要暴击伤害辅助", "character", 8)` 能召回花火、星期日、知更鸟等;`semantic_search("持续伤害 dot 卡芙卡 队友", "character", 8)` 能召回卡芙卡、黑天鹅、桑博、桂乃芬等。

**注意**:本地 hash embedding 是为了把 pgvector 链路跑通并做机制词召回,不是最终语义排序模型。最终推荐仍以 axes SQL / co-occurrence / Agent 解释为主。M7 不允许把 `local-hash-ngram-v1` 包装成"真语义搜索"暴露给前端自由文本搜索。

### M4 — Agent + 工具(1-2 天)

- [x] Go 后端 `internal/tools` 已实现核心 SQL 工具:
  - `get_character`
  - `search_by_role`
  - `semantic_search`
  - `find_needs`
  - `find_buffers_for`
  - `find_synergies`
  - `suggest_team`
  - `co_occurrence`
  - `recommend_lightcones`
  - `recommend_relics`
  - `get_assets`
- [x] Go 后端 `internal/agent` 已实现 OpenAI-compatible tool-use 循环
- [x] Go CLI 已支持 `--ask`
- [x] Go CLI 已支持 `--trace-tools` 验收工具调用轨迹
- [x] `hsr_agent/llm_client.py` 封装 `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` / `LLM_API_FORMAT`
- [ ] Python 版 `hsr_agent/tools.py` / `agent.py` 暂缓;在线后端以 Go 为准
- [x] 用真实 LLM 跑 M4 验收问题;测试 key 仅注入进程环境,未持久化
- [x] Agent 工具结果已压缩,避免多轮候选查询把上下文撑爆
- [x] 达到工具调用上限后会强制 finalization,不再直接失败
- [x] LLM HTTP 5xx / 网络错误会自动重试最多 3 次

**验收 case 1**:「花火配什么队」
- agent 至少调用 get_character(花火) + find_needs(1306) + find_buffers_for(crit_dmg) + suggest_team(1306) + co_occurrence(1306)
- 最终回答列出 ≥ 8 个考察过的角色 id
- 至少给出 2 套不同的队伍方案
- 每套说明为什么这套成立(buff 链)

**已通过**:`deepseek-v4-pro-none` 调用了 `get_character` / `find_needs` / `find_synergies` / `suggest_team` / `co_occurrence` / `semantic_search`,最终给出 3 套方案和候选排除理由。

**验收 case 2**:「想抽个能带罗刹的 DPS」
- agent 先查罗刹定位(治疗、回血触发被动)
- 找出"被动需要持续 HP 损失/回血"的 DPS(刃、卡夫卡 dot、阿兰等)
- 给出推荐 + 排除其他 DPS 的理由

**已通过**:`deepseek-v4-pro-none` 初次遇到一次网关 502;压缩工具结果后重试通过,给出饮月/镜流/白厄等推荐和排除理由。

**验收 case 3**:「我现在有花火、银狼、刃,缺什么」
- agent 计算队伍当前的 axes 覆盖
- 找出缺失:可能缺破韧 / 缺生存
- 推荐 1-2 个补位角色

**已通过**:`deepseek-v4-pro-none` 多轮工具调用下有 502;切到同网关 `deepseek-v4-pro` 后通过。结论为缺生命拐治疗位,推荐风堇/玲可,并指出花火与刃机制相性一般。

**M4 注意**:newapi 网关的 `deepseek-v4-pro` 在多轮 tool-use 下比 `deepseek-v4-pro-none` 稳定。后续默认可优先用 `deepseek-v4-pro` 做 Agent,在线失败时再降级或重试。

### M5 — 机制规格与数值校验工具(2-3 天,下一阶段核心)

**目标**:做我们自己的机制规格和最小数值校验工具,让 Agent 在回答"某角色配队/契合度/抽取价值"时能调用工具验证关键加成,而不是让 LLM 口算或凭印象解释。

**边界**:

- 不做完整遗器优化器,不替代 Fribbels
- 不做完整回合模拟器,暂不处理精确行动轴、自动战斗、敌人出招
- 不搬 Fribbels / THCHelper / hsr-tct 的代码或数据模型
- 只借鉴公开机制原理:伤害乘区、击破/超击破关系、治疗/护盾公式、角色数值效果如何进入乘区
- PG 是正式事实来源;JSON 只允许作为 LLM 抽取中间结果、原始追溯、测试 fixture

#### M5.1 机制资料整理

- [x] 新建 `docs/MECHANICS.md`
- [x] 整理常规伤害公式:
  - 基础伤害
  - 暴击期望
  - 增伤区
  - 防御区
  - 抗性区
  - 易伤区
  - 减伤区
  - 击破状态倍率
- [x] 整理非直伤公式:
  - DoT 是否吃暴击、增伤、易伤、防御、抗性
  - 击破伤害
  - 超击破伤害
  - 追加伤害 vs 追加攻击 vs 附加伤害的区别
  - 治疗量
  - 护盾量
- [x] 记录机制来源和可信度:
  - Fribbels:优先参考,MIT,公式覆盖较完整,但工程形态偏优化器
  - THCHelper:参考击破/超击破/欢愉等公式说明,无仓库级 LICENSE,仅看机制不复用代码
  - hsr-tct:参考它覆盖了哪些常规乘区,GPL-3.0,不复用代码和结构
- [x] 每条机制写成"我们自己的文字 + 公式变量解释 + 适用范围 + 暂不支持项"

#### M5.2 PG 表设计

- [x] 新增 migration,把角色机制效果从 `axes JSONB` 的粗粒度描述推进到可查询表
- [x] 新表草案:

```sql
CREATE TABLE character_effect_sources (
    id              BIGSERIAL PRIMARY KEY,
    character_id    INT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    source_kind     TEXT NOT NULL,      -- skill / talent / ult / trace / eidolon / technique
    source_key      TEXT NOT NULL,      -- nanoka 原始 skill id / point id / rank id
    name_zh         TEXT NOT NULL,
    source_text_zh  TEXT NOT NULL,
    game_version    TEXT NOT NULL,
    source_hash     TEXT NOT NULL,
    UNIQUE(character_id, source_kind, source_key, game_version)
);

CREATE TABLE character_modifiers (
    id              BIGSERIAL PRIMARY KEY,
    source_id       BIGINT NOT NULL REFERENCES character_effect_sources(id) ON DELETE CASCADE,
    target_scope    TEXT NOT NULL,      -- self / one_ally / all_allies / one_enemy / all_enemies
    stat_key        TEXT NOT NULL,      -- atk_pct / crit_dmg / dmg_bonus / def_shred / res_pen ...
    value           NUMERIC,
    value_unit      TEXT NOT NULL,      -- percent / flat / ratio / stack / unknown
    modifier_zone   TEXT NOT NULL,      -- base / crit / dmg_bonus / def / res / vuln / mitigation / break / utility
    attack_tag      TEXT,               -- basic / skill / ult / fua / dot / break / any
    element_key     TEXT,               -- fire / ice / quantum / any / NULL
    target_path     TEXT,               -- optional path condition
    condition_text  TEXT,
    condition_jsonb JSONB NOT NULL DEFAULT '{}',
    duration_key    TEXT,
    stack_rule      TEXT,
    confidence      NUMERIC NOT NULL DEFAULT 0.0,
    reviewed        BOOLEAN NOT NULL DEFAULT FALSE
);
```

- [x] 先不把复杂条件过度范式化;`condition_jsonb` 只用于机器可读条件,`condition_text` 保留中文解释
- [x] 后续稳定后再拆条件表,不要一开始设计过重

#### M5.3 抽取与审核流程

- [x] 新增 `schemas/modifier_vocab.py`,定义受控词表:
  - `stat_key`
  - `modifier_zone`
  - `target_scope`
  - `attack_tag`
  - `duration_key`
  - `stack_rule`
- [x] 新增 `scripts/extract_modifiers.py`
  - 输入:PG 中 `characters.raw_zh`
  - LLM 只输出结构化草稿(JSON 作为中间态)
  - 脚本校验词表、数值、引用源文本 hash
  - 校验通过后写入 PG 表
- [x] 新增 `scripts/load_modifiers.py` 或把写库并入 `extract_modifiers.py`
- [x] 新增抽检命令:
  - 单角色:花火(1306)、知更鸟(1309)、阮梅(1303)、布洛妮娅(1101)
  - DPS:刃(1205)、黄泉(1308)、卡芙卡(1005)、流萤(1310)
  - 生存:罗刹(1203)、藿藿(1217)、砂金(1304)、符玄(1208)
- [x] `reviewed=false` 的结果可被 Agent 使用,但回答时要降低措辞确定性;`reviewed=true` 才作为高可信数值依据
- [x] OpenAI-compatible 抽取默认启用流式请求,并保留 `--no-stream` 作为回归/对照开关

**当前状态**:`002_mechanics.sql` 已应用;`4.3.54` 全部 95 个角色均已写入 `character_effect_sources` 和 `character_modifiers`,当前 `v_character_modifiers` 覆盖 `characters_with_modifiers=95`,共 `character_modifiers=2274` 条。抽取结果仍为 `reviewed=false`,可供 Agent 低置信数值依据使用,后续高风险角色需要人工抽查后标记 `reviewed=true`。

#### M5.4 Go `internal/calc` 最小计算内核

- [x] 新增 `hsr-agent-go/internal/calc`
- [x] 第一版只实现"局部倍率校验",不做完整配装、不做完整战斗:
  - 给定攻击者基础面板
  - 给定敌人等级/抗性/防御默认值
  - 给定一组 modifiers
  - 输出每个乘区的倍率和总倍率变化
- [x] 支持常规直伤:
  - ATK/HP/DEF 缩放基础伤害
  - 暴击期望
  - 增伤区
  - 防御区
  - 抗性区
  - 易伤区
  - 减伤区
- [x] 支持非伤害型 utility 的解释性计分,不强行转伤害:
  - 拉条
  - 回战技点
  - 回能
  - 治疗
  - 护盾
  - 削韧
- [x] 第二批局部精算已加入:
  - 击破
  - 超击破
  - DoT
  - 治疗/护盾精算
  - 简化覆盖率/uptime
- [ ] 仍不做:
  - 真实角色面板/遗器/光锥导入
  - 完整行动轴
  - 敌人库和多轮循环

#### M5.5 Agent 工具接入

- [x] 新增 Go tools:
  - `list_character_modifiers(char_id)`
  - `compare_character_fit(attacker_id, support_id)`
  - `estimate_damage_gain(attacker_id, support_ids, attack_tag)`
  - `explain_modifier_sources(char_id)`
  - `estimate_dot_damage(attacker_id, support_ids)`
  - `estimate_break_damage(attacker_id, support_ids, element, break_effect, max_toughness)`
  - `estimate_super_break_damage(attacker_id, support_ids, toughness_reduction, super_break_multiplier)`
  - `estimate_healing(char_id, support_ids, scaling_stat, ability_multiplier, flat_value)`
  - `estimate_shield(char_id, support_ids, scaling_stat, ability_multiplier, flat_value)`
  - `estimate_uptime(duration_turns, cooldown_turns, cycle_turns)`
- [x] Agent 回答流程升级:
  - 先查 `get_character` / `suggest_team` / `co_occurrence`
  - 再查角色 modifiers
  - 对关键候选调用 calc 工具
  - 最终答案同时给"社区推荐依据"和"机制/数值依据"
- [x] 典型回答必须说明:
  - 哪些加成命中了角色需求
  - 哪些加成不吃或收益低
  - 数值估算基于默认面板,不是实战精确伤害

**当前状态**:Go CLI 与 Agent tool schema 已接入常规直伤、DoT、击破、超击破、治疗、护盾和 uptime 局部估算工具。默认按 E0 估算,不计入 `eidolon` 来源;CLI 可用 `--include-eidolons` 纳入全部星魂,也可用 `--eidolons 1,2,6` 只启用指定星魂;Agent tool 参数为 `include_eidolons` / `eidolons`。`compare_character_fit(刃1205,花火1306,basic)` 能识别花火爆伤、拉条、战技点和增伤价值,同时提示刃为 HP 缩放,攻击力类加成低收益。`estimate_break_damage` / `estimate_super_break_damage(流萤1310,阮梅1303)` 已能把弱点击破效率、全属性抗性穿透、击破特攻纳入对应乘区,并把普通增伤列为 skipped。真实 Agent smoke「花火和刃契合吗」已通过,trace 显示调用 `get_character`、`compare_character_fit`、`find_needs`、`list_character_modifiers`、`estimate_damage_gain`、`co_occurrence`、`find_synergies`。

#### M5.6 验收 case

- [x] 「花火和刃契合吗」
  - 必须指出花火的爆伤、拉条、战技点价值
  - 必须指出刃不主要吃攻击,攻击类拐收益低
  - 必须调用 `compare_character_fit` 或等价 calc 工具
- [ ] 「知更鸟适合追击队的原因」
  - 必须解释全队增伤/攻击/附加伤害/追击高频触发之间的关系
  - 必须区分"追加攻击"和"附加伤害"
- [ ] 「阮梅对流萤提升在哪里」
  - 必须解释击破效率、全抗性穿透、击破/超击破相关收益
  - 必须调用 `estimate_break_damage` 或 `estimate_super_break_damage` 给出局部场景估算
- [ ] 「银狼和量子 C 的契合」
  - 必须解释弱点植入、减防、减抗对单核队的价值
- [ ] 「我有花火、银狼、刃,第四人选谁」
  - 必须结合 M4 的配队工具和 M5 的机制校验
  - 生存位推荐要说明机制理由,不能只报共现

### M6 — 工程化与体验(可选,1 天;前端由用户负责)

- [ ] Web UI 由前端负责;后端只保证 API、SSE 和静态资源路径稳定。
- [ ] 头像/光锥图自动展示由前端实现;后端通过 `get_assets` / `/api/assets` 提供路径。
- [ ] 对话历史持久化如需要由后端提供表和接口,前端负责展示。
- [ ] 多用户如需要由后端提供鉴权和数据隔离,前端负责登录/会话体验。

### M7 — 前后端分工与 HTTP API 契约(1-2 天)

**目标**:把当前 CLI/Agent 能力整理成稳定后端服务,前端只依赖 HTTP/SSE 契约,不直接碰 PG、不调用 LLM、不理解机制计算细节。

**M7 前置质量门槛**:

- 当前 `local-hash-ngram-v1` 只是机制词/字面 ngram 召回,不能包装成"真语义搜索"给前端使用。
- M7 如果暴露 `semantic_search` HTTP API,必须先接入真实 embedding 模型,或在接口返回中明确 `embedding_quality=lexical_hash` 并禁止前端把它设计成自由文本语义搜索。
- 光锥与遗器套装 `axes` 已补 v1,`recommend_lightcones` / `recommend_relics` 已返回 score/reasons/matched_*。注意:光锥仍缺真实效果文本,当前 axes 是弱画像,不能当作光锥机制事实。

**分工约定**:

- 后端(Codex 负责):Go 服务、PG schema/migration、LLM tool-use 循环、机制计算、数据装载、接口文档、回归测试。
- 前端(用户负责):页面框架、角色/队伍交互、对话展示、工具调用过程可视化、结果图表、移动端/桌面端体验。
- 共享契约:OpenAPI/接口样例、错误码、流式事件格式、静态资源路径规范。

**后端交付**:

- [x] 新增 Go HTTP server,保留 CLI 作为调试入口;支持 `--serve`、`HTTP_ADDR`、`WEB_ROOT`,同端口托管前端 SPA。
- [x] 新增 embedding provider 配置占位:`EMBEDDING_PROVIDER` / `EMBEDDING_MODEL` / `EMBEDDING_DIMENSIONS`;真实 embedding 生成/查询链路未接入前,HTTP semantic search 仍禁用。
- [ ] 推荐默认方案:开发与线上轻量部署用 OpenAI-compatible embedding,维度配置为 1024 以兼容当前 PG schema;本地离线可选 BGE-M3 或 Qwen3-Embedding。
- [ ] `semantic_search` 结果必须返回 `embedding_model`、`embedding_dimensions`、`embedding_quality`、`score` 和 `score_explain`。
- [x] 如果真实 embedding 未配置,HTTP API 仍可启动,但 `/api/search/semantic` 返回 `503 SEMANTIC_SEARCH_DISABLED`,前端改走精确筛选/关键词搜索。
- [x] 补齐 relic_set axes v1:抽取 2件/4件/2件套 stat、适用输出类型、限制条件。
- [x] 补齐 lightcone weak axes v1:基于命途与 nanoka 推荐角色画像反推 needs/tags,用于弱检索和推荐解释。
- [ ] 补抓 lightcone 真实效果文本后,抽取提供/需求 stat、触发条件、叠影数值,替换当前弱 axes。
- [x] `GET /api/health`:返回数据库、版本、模型配置可用性。
- [x] `GET /api/characters`:角色列表,支持 `q/path/element/role/rarity` 过滤。
- [x] `GET /api/characters/{id}`:角色详情、roles、axes 摘要。
- [x] `GET /api/characters/{id}/assets`:返回本地/CDN asset path 映射,由前端决定如何展示。
- [x] `GET /api/characters/{id}/modifiers`:角色机制效果,支持 `stat_key/target_scope` 过滤。
- [x] `GET /api/lightcones` / `GET /api/lightcones/{id}`:光锥列表与详情,包含 axes。
- [x] `GET /api/relic-sets` / `GET /api/relic-sets/{id}`:遗器套装列表与详情,包含 axes。
- [x] `GET /api/search/semantic`:HTTP 层显式禁用,返回 `503 SEMANTIC_SEARCH_DISABLED`;真语义搜索等真实 embedding 接入后开放。
- [x] `GET /api/search/keyword`:trgm/LIKE 搜索;即使没有真实 embedding 也可用。
- [x] `POST /api/agent/chat`:非流式问答,用于调试和自动测试。
- [x] `POST /api/agent/chat/stream`:SSE 输出 tool_call、tool_result、final、error 事件;token 级 message_delta 留到上游 LLM streaming 改造。
- [x] `POST /api/mechanics/*`:暴露局部计算工具,先覆盖 damage/dot/break/super_break/heal/shield/uptime。
- [x] 生成 `docs/API.md`,包含请求/响应样例和 SSE event schema。

**SSE 事件草案**:

```json
{"type":"tool_call","name":"estimate_super_break_damage","args":{...}}
{"type":"tool_result","name":"estimate_super_break_damage","summary":{...}}
{"type":"final","message":"...","trace_id":"..."}
{"type":"error","code":"LLM_UPSTREAM_ERROR","message":"..."}
```

**验收标准**:

- [x] 前端能不读数据库完成角色搜索、角色详情、对话问答、工具轨迹展示。
- [x] 前端能不读数据库完成光锥/遗器套装搜索、详情和基础推荐展示。
- [x] 光锥/遗器套装 axes 不再为空,推荐结果能解释"为什么适合"。
- [x] `/api/search/semantic` 不再使用 `local-hash-ngram-v1`;未配置真实 embedding 时必须显式禁用。
- [ ] 同一个 agent 问题在 CLI 和 HTTP 下工具调用结果一致。
- [ ] LLM key 只从环境变量读取,接口和日志不泄露 key。
- [x] `go test ./...` 通过,并新增至少 3 个 HTTP handler 测试。

### M8 — 机制模型 v2:敌方 debuff、施放者面板、条件语义(2-4 天)

**目标**:补齐当前局部工具的主要盲区,让 agent 不再把"敌方减防/减抗"误判为未命中攻击者的 buff,也能处理"按施放者属性转化"这类效果。

**后端改造**:

- [ ] 明确 modifier 作用侧:`ally_buff` / `enemy_debuff` / `field_effect` / `utility`。
- [ ] 让 `one_enemy/all_enemies` 的减防、减抗、易伤进入伤害公式。
- [ ] 增加 `source_stat_dependency` 表达方式,支持"等同于施放者击破特攻的 15%"。
- [ ] 增加手动场景输入:`attacker_panel`、`support_panels`、`enemy_state`。
- [ ] 统一条件解析:`enemy_count`、阈值、叠层、持续回合、攻击标签、目标元素。
- [ ] 把"转化为 N% 超击破"从临时归一化升级为明确 stat,例如 `super_break_base_multiplier`。
- [ ] 为高风险角色建立人工 reviewed 流程:流萤、忘归人、同谐开拓者、阮梅、银狼、知更鸟、灵砂、加拉赫。

**优先校验 case**:

- [ ] 忘归人给流萤:狐祈、云火昭、减防、E1 弱点击破效率。
- [ ] 同谐开拓者给流萤:伴舞、卫我起舞 enemy_count、E4 击破特攻转化。
- [ ] 银狼给量子 C:弱点植入、减防、减抗、单敌收益。
- [ ] 阮梅给击破队:弱点击破效率、全抗穿透、延滞、击破伤害。

**验收标准**:

- [ ] `estimate_super_break_damage` 可以显式列出 ally buffs 与 enemy debuffs 分别进入了哪些乘区。
- [ ] 忘归人 23% 减防能进入流萤超击破估算,并在结果中解释为敌方 debuff。
- [ ] 同谐主 E4 不再固定写死 15%,而是可由开拓者面板输入或默认面板推导。
- [ ] 高风险角色的关键 modifier 至少有人工 reviewed 标记或测试 fixture 覆盖。

### M9 — 队伍/行动轴模拟 v1(4-7 天)

**目标**:从"单次乘区工具"升级到"一段战斗窗口内的队伍收益估算",用于回答 E2 拉条、终结技覆盖、SP 循环、全队超击破贡献这类问题。

**范围控制**:

- 第一版做可解释的简化模拟,不是完整自动战斗器。
- 不导入真实遗器/光锥,默认面板 + 用户手动覆盖。
- 不模拟复杂敌人 AI,敌人只提供韧性、弱点、抗性、波次和行动占位。

**核心模型**:

- [ ] `Combatant`:角色 id、速度、能量、初始能量、击破特攻、关键面板、星魂开关。
- [ ] `Team`:4 人队伍、站位、默认行动策略、技能点策略。
- [ ] `EnemyWave`:敌人数、韧性、弱点、抗性、等级、波次。
- [ ] `Timeline`:AV 推进、行动提前/延后、额外行动、终结技插入。
- [ ] `ResourceState`:SP、能量、buff/debuff 持续、叠层、冷却。
- [ ] `ToughnessState`:削韧、弱点击破效率、无视弱点削韧、破韧时点、再次击破/特殊韧性。

**先做的队伍模板**:

- [ ] 流萤击破队:流萤 + 同谐开拓者/忘归人 + 阮梅 + 灵砂/加拉赫。
- [ ] 追击队:知更鸟 + 追击核心 + 砂金/托帕等候选。
- [ ] 量子单核:量子 C + 银狼 + 花火/辅助 + 生存位。

**输出**:

- [ ] 总伤、超击破伤害、击破伤害、DoT、附加伤害分桶。
- [ ] 每个角色贡献占比。
- [ ] 关键 buff/debuff 覆盖率。
- [ ] SP 净消耗、能量循环缺口。
- [ ] 时间线事件列表,供前端画轴。
- [ ] "为什么 A 高于 B"的差异分解。

**验收标准**:

- [ ] 能回答"同谐主不能给流萤时,忘归人几魂平替"并区分单次伤害与循环总收益。
- [ ] 能量化忘归人 E2 的行动提前收益,不再只写 utility。
- [ ] 能量化忘归人 E6 的全队狐祈收益,至少覆盖全队超击破贡献。
- [ ] 输出稳定 JSON,前端可以直接渲染时间线和贡献图。

### M10 — 后端产品化与质量门槛(持续)

**目标**:让后端从研究脚本变成前端可长期依赖的服务。

**工程化**:

- [ ] Docker compose 增加 backend service,一条命令启动 PG + Go API。
- [ ] migration 支持版本检查和 checksum,禁止 silent drift。
- [ ] 增加 request trace id,每次 agent 问答可追踪 tool calls。
- [ ] 统一错误码:`BAD_REQUEST`、`NOT_FOUND`、`LLM_UPSTREAM_ERROR`、`DB_UNAVAILABLE`、`TOOL_EXECUTION_ERROR`。
- [ ] 增加缓存:角色详情、modifier 列表、常见计算结果、LLM 对话可选缓存。
- [ ] 增加 golden tests:固定问题、固定工具调用、固定关键结论。
- [ ] 增加数据质量报表:未 reviewed modifier、unknown stat、无 value 的关键效果、抽取置信度低的角色。

**接口稳定策略**:

- [ ] `/api/*` 保持向后兼容;破坏性变更走 `/api/v2/*`。
- [ ] 前端只依赖 `docs/API.md` 和 OpenAPI schema,不依赖 Go 内部结构。
- [ ] 所有机制计算结果必须带 `assumptions` 和 `caveats`,避免前端误展示成实战精确值。

**参考项目的后续使用边界**:

- [ ] Fribbels:只参考条件配置、面板/遗器输入体验、结果解释方式。
- [ ] THCHelper:只参考公式口径和边界 case,不复用代码。
- [ ] hsr-tct:只参考 sheeting 思路、团队 buff/debuff 表达和测试口径,不复用 GPL 代码。
- [ ] 我们的事实来源仍是 PG + nanoka 原始数据 + reviewed modifier;外部项目只做机制校验参照。

### M11 — 前端协作接口清单(给前端实现用)

前端不需要等待 M9 全部完成,可以按接口成熟度分批接:

1. **第一批**:角色列表、角色详情、图片路径、普通 agent 对话。
2. **第二批**:SSE 流式输出、工具调用轨迹、局部机制计算结果卡片。
3. **第三批**:队伍构建器、星魂/条件开关、敌人数/敌人韧性/弱点输入。
4. **第四批**:行动轴时间线、队伍贡献分解、候选方案对比。

前端需要后端提前稳定的字段:

- `character.id/name_zh/path/element/rarity/roles/assets`
- `modifier.source/stat_key/value/target_scope/condition/reviewed/confidence`
- `mechanic_result.baseline/with_modifiers/total_multiplier/applied/skipped/assumptions/caveats`
- `agent_event.type/text/tool_name/tool_args/tool_summary/trace_id`
- `simulation.timeline/events/contributions/resources/assumptions/caveats`

---

## 5. 关键技术取舍清单(实施时不要偏离)

| 决策 | 选择 | 不选什么 | 理由 |
|---|---|---|---|
| 数据存储 | PostgreSQL + pgvector | 独立 Chroma/Qdrant | 一个库统一管,数据规模小 |
| Agent 框架 | OpenAI-compatible tool use 薄封装(Go) | LangChain / LlamaIndex / adalflow | 直接工具循环能搞定,抽象层是负资产 |
| 向量定位 | 兜底 + 意图层 | 主力检索 | SQL on axes 比 top-k 准 |
| axes 生成 | LLM 一次性预处理 + 人工抽查 | 实时 LLM 解析 | 离线生成,后续查询不烧 token |
| ORM | 不用,直接 psycopg + SQL | SQLAlchemy / Django ORM | 查询透明,LLM 工具调试容易 |
| 语料/命名 | 国服中文语料(zh) | 英文详情作为主输入 / 英文直译 | 用户场景是中文玩家,可减少机制和译名误差 |
| 模型 | DeepSeek 默认,Claude 可选兜底 | 写死 Claude | 配队推理不需要默认上昂贵模型,兼容协议方便切换 |
| 缓存 | 使用供应商默认缓存/Context Caching | 写死 Claude `cache_control` | 不要把协议绑定到 Claude 专属字段 |
| 数据版本化 | 目录隔离 `4.3.54/` | DB schema 升版 | nanoka 也是按版本号目录组织,镜像即可 |
| 机制数值 | 自研 `docs/MECHANICS.md` + Go `internal/calc` | 直接集成 Fribbels/THCHelper/hsr-tct | 我们需要 Agent 可解释的局部校验,不是优化器/Unity UI/Excel 工具 |
| 效果维护 | PG 表是事实来源 | 长期维护 JSON 文件 | JSON 只做 LLM 抽取中间态、原始追溯、测试 fixture |
| 外部项目 | 只借鉴公开机制原理 | 复制代码/数据模型 | THCHelper 无明确 LICENSE,hsr-tct 是 GPL-3.0,Fribbels 工程耦合高 |

---

## 6. 已知缺口 / 待决问题(留给实施时确认)

1. **embedding 模型选型**
   - 已决策:M7 暴露 HTTP semantic search 前必须接入真实 embedding,不能继续用 `local-hash-ngram-v1` 冒充语义搜索。
   - 云端/网关优先:OpenAI-compatible embedding provider,维度配置为 1024 以兼容当前 `vector(1024)` schema;如模型只支持其他维度,需新增 migration 调整 vector 维度和 HNSW 索引。
   - 本地优先:可选 BGE-M3 或 Qwen3-Embedding,但必须通过离线回归集验证中文自由文本召回。
   - 兜底策略:真实 embedding 未配置时,`/api/search/semantic` 返回 503,前端使用 `/api/search/keyword`、角色筛选和推荐接口。

2. **是否需要"用户已有角色 / 等级"维度**
   - 当前设计假设用户问的是"我应不应该抽"或"假设我有这些角色"
   - 不存用户库存数据;每次会话由用户在 prompt 中说明
   - 如果要做账号 / 库存同步,加 `users` + `user_inventory` 表

3. **是否抓 mihomo / hoyolab 玩家面板**
   - 当前数据是"角色配置 + nanoka 推荐"
   - 不包含"全服玩家真实出装统计"(prydwen/honkai-star-rail.fandom 有)
   - 如果需要更强的"主流配队",未来可接 mihomo API

4. **遗器套装的 `kind`(cavern vs planar)**
   - relicset.json 没显式字段,需要从 `set` 数组长度 + id 区间猜
   - 一般 id 1XX/2XX 是 cavern(4件套),3XX 是 planar(2件套)— **实施时验证**

5. **lightcone 的 path 字段**
   - 在 lightcone.json 里需要找下,可能叫 `path` / `baseType` — 实施时确认

6. **enrich 的成本估算**
   - 95 角色 × 平均 5K token 输入,用 DeepSeek 默认模型批处理
   - 成本应显著低于默认 Claude;实际以当前 DeepSeek 价格页为准
   - DeepSeek Context Caching 默认开启,重复的词表/system prompt 会更便宜

7. **LLM API key / 模型切换**
   - `.env` 用 `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL`,不要写死厂商
   - 默认:DeepSeek / newapi 的 OpenAI-compatible API
   - 兜底:高难样本或回归评测时临时切 Claude-compatible 配置

8. **角色机制抽取的审核深度**
   - 第一版可以允许 `reviewed=false` 的 modifiers 进入 Agent,但回答要标明"估算/待校验"
   - 高风险角色(新版本角色、击破/超击破核心、复杂召唤物/记忆灵角色)应优先人工抽查
   - 后续可做一个简单审核 CLI,把 source_text 和抽取结果并排输出

9. **是否需要完整行动轴模拟**
   - 当前 M5 明确不做完整行动轴
   - 如果未来要做,应单独开 M9,不要把行动轴混进第一版 calc
   - 第一版只做"同一攻击场景下加成变化"和"机制契合解释"

---

## 7. 验收(Definition of Done)

最小可用版本(M0-M4 完成)算 DOD:

1. `docker compose up -d` 起 PG + pgvector
2. `python scripts/migrate.py && python scripts/load.py && python scripts/enrich.py && python scripts/load_axes.py && python scripts/embed.py && python scripts/compute_cooccur.py && python scripts/build_asset_paths.py` 一键准备数据
3. `go run ./cmd/hsr-agent --ask "花火配什么队"` 进入 Go Agent 问答
4. 输入「花火配什么队」,30 秒内得到符合 3.4 验收 case 1 标准的答案
5. 输入「现在哪个 5 星值得抽」(开放式问题),agent 能拉出新角色(release 字段),分析当前版本 meta 趋势(用 co_occurrence 和 axes),给出有依据的回答
6. 答案里的所有角色名都是国服译名,所有引用都带 id

M5 完成后的新增 DOD:

1. `docs/MECHANICS.md` 存在,并覆盖常规伤害、暴击期望、增伤、防御、抗性、易伤、减伤、击破/超击破边界说明
2. PG 中存在 `character_effect_sources` 和 `character_modifiers` 等机制效果表,不是靠长期维护 JSON 文件
3. 至少 8 个样板角色完成 modifiers 抽取并入库:花火、知更鸟、阮梅、刃、黄泉、流萤、罗刹、砂金
4. Go 侧存在 `internal/calc`,能对一组 modifiers 输出分乘区倍率和总倍率变化
5. Agent 至少新增一个数值校验工具,并在「花火和刃契合吗」这类问题中实际调用
6. 回答中必须区分"社区推荐/共现依据"和"机制数值依据"
7. 对未覆盖的边界必须明示,例如"第一版未做完整行动轴/真实面板导入/敌人库循环"

---

## 8. 不要做的事

- **不要**写一个"先把 95 角色全文本灌进 prompt"的版本 — 那不是 agent,是 RAG 的劣化版
- **不要**给 Agent 加"网络搜索"工具 — 数据已经本地化,加搜索会让答案不稳定
- **不要**让 enrich.py 产生自由文本字段当主键(condition 字段例外)— 词表受控是项目的基石
- **不要**默认拿 `en/character/<id>.json` 做 enrich 主输入 — 先用 `zh/` 国服中文语料,英文只做 fallback/对照
- **不要**用 `text` 列存 JSON 字符串再 `json_extract` — 用 JSONB
- **不要**把 enriched JSON 当唯一 source of truth — 受控词表会演进,raw_zh/raw_en 必须保留以便重跑 enrich
- **不要**直接复制本对话中"罗宾"这种译名错误 — 全项目用国服译名(检查方式:对 1309 应输出"知更鸟",对 1306 应输出"花火")
- **不要**把角色效果长期维护成一堆 JSON 文件 — PG 表是事实来源,JSON 只做中间态/fixture/追溯
- **不要**复制 Fribbels、THCHelper、hsr-tct 的代码或数据模型 — 只阅读机制原理,实现必须自研
- **不要**在 M5 第一版追求完整实战模拟 — 先做局部数值校验,否则范围会失控

---

## 9. 附录

### 9.1 资源 URL 模板速查

```
角色:
  圆头像:     https://static.nanoka.cc/assets/hsr/avatarroundicon/{id}.webp
  商店头像:    https://static.nanoka.cc/assets/hsr/avatarshopicon/{id}.webp
  立绘头像:    https://static.nanoka.cc/assets/hsr/avataricon/avatar/{id}.webp
  抽卡大图:    https://static.nanoka.cc/assets/hsr/avatardrawcard/{id}.webp
  星魂 1-6:    https://static.nanoka.cc/assets/hsr/rank/_dependencies/textures/{id}/{id}_Rank_{n}.webp
  OG 大图:     https://hsr.nanoka.cc/character/{id}/og.png
  技能图标:    https://static.nanoka.cc/assets/hsr/skillicons/{iconName无后缀}.webp

光锥:
  中图:        https://static.nanoka.cc/assets/hsr/lightconemediumicon/{id}.webp
  大图:        https://static.nanoka.cc/assets/hsr/lightconemaxfigures/{id}.webp

物品/遗器套装/材料:
  通用:        https://static.nanoka.cc/assets/hsr/itemfigures/{stem}.webp
              ({stem} 来自 item.item_figure_icon_path 的去后缀文件名,或 relic_set.id)

共享小图(已下,本地用即可,不需要每次取):
  命途:        pathicon/{lowercase_path}.webp   (knight/mage/priest/rogue/shaman/warlock/warrior/memory)
  元素:        element/{lowercase_element}.webp (fire/ice/imaginary/physical/quantum/thunder/wind)
  遗器槽位:    relicfigures/IconRelic{slot}.webp (Head/Hands/Body/Foot/Neck/Goods)
```

### 9.2 测试 case 集(用作 M4 验收前的 regression test)

| query | 必查角色 | 必含建议 | 必排除 |
|---|---|---|---|
| 「花火配什么队」 | 1306 | 含希儿(1102) 或 银枝(1218) | - |
| 「知更鸟队伍推荐」 | 1309 | 主 C 用追击/物理倾向角色,如克拉拉(1107)、阿文琴(1310) | - |
| 「想抽个能带罗刹的 DPS」 | 1203 | 推荐刃(1205)、卡夫卡 dot 队 | 不应推荐速攻队 |
| 「我有花火、银狼、刃,缺什么」 | 1306, 1006, 1205 | 缺生存,补罗刹/藿藿/三月七 | - |
| 「希儿和黄泉哪个更适合我现在的队伍」(无具体队伍) | 1102, 1217 | agent 应反问用户当前队伍 | - |

### 9.3 关键参考链接

- nanoka HSR 首页:https://hsr.nanoka.cc
- 静态资源根:https://static.nanoka.cc/hsr/<version>/ 和 https://static.nanoka.cc/assets/hsr/
- 全局 manifest:https://static.nanoka.cc/manifest.json(含 gi/hsr/ww/zzz/nte 各游戏版本)
- pgvector:https://github.com/pgvector/pgvector
- DeepSeek Anthropic API 文档:https://api-docs.deepseek.com/guides/anthropic_api
- DeepSeek 模型与价格:https://api-docs.deepseek.com/quick_start/pricing
- DeepSeek Tool Calls 文档:https://api-docs.deepseek.com/guides/function_calling
- DeepSeek Context Caching 文档:https://api-docs.deepseek.com/guides/kv_cache
- Anthropic tool use 文档(兼容协议参考):https://docs.anthropic.com/claude/docs/tool-use
- 数据上游(开源):Mar-7th/StarRailRes (GitHub)
- Fribbels HSR Optimizer:https://github.com/fribbels/hsr-optimizer
- THCHelper:https://github.com/Tytyn000/THCHelper
- hsr-tct:https://github.com/j4rv/hsr-tct

---

**文档版本**:v4 — 2026-06-27
**下一步**:后端先进入 M7,把现有 CLI/Agent 能力封装成 Go HTTP API 与 SSE 流式接口;同时保留 M5.6 回归问题作为 API/Agent 的 golden tests。
