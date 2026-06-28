# HSR Agent

本项目基于本地 `nanoka_hsr/4.3.54` 数据构建崩坏星穹铁道配队/抽取建议 agent。

当前约定:

- 数据存储: PostgreSQL + pgvector
- 主语料: `zh/` 国服中文详情
- 在线后端: Go
- 离线数据脚本: Python
- 默认模型: DeepSeek/OpenAI-compatible Chat Completions

## 启动数据库

```powershell
Copy-Item .env.example .env
docker compose up -d
```

默认连接串:

```text
postgresql://hsr:hsr@localhost:55432/hsr_agent
```

## Python 环境

```powershell
python -m venv .venv
.\.venv\Scripts\Activate.ps1
python -m pip install -e .
```

## 执行迁移

```powershell
python scripts/migrate.py
```

迁移会创建 `schema_migrations` 表并记录 checksum。已执行的 migration 不会重复执行。

## 装载本地数据

```powershell
python scripts/load.py
python scripts/compute_cooccur.py
python scripts/build_asset_paths.py
```

当前 `4.3.54` 数据验收计数:

```text
characters=95
lightcones=165
relic_sets=60
items=1574
character_recommendations=1495
team_cooccur=2038
asset_paths=4087
```

## Axes 预处理

先确认 prompt 和中文主语料:

```powershell
python scripts/enrich.py --dry-run --ids 1309
```

配置 `.env` 里的 `LLM_API_KEY` 后，先跑 4 个样板角色。OpenAI-compatible 网关使用:

```powershell
$env:LLM_BASE_URL='https://api.deepseek.com'
$env:LLM_API_FORMAT='openai'
$env:LLM_MODEL='deepseek-chat'
```

如果使用自建 newapi 网关，只替换 `LLM_BASE_URL` 和 `LLM_MODEL`，不要把 key 写进仓库文件。

```powershell
python scripts/enrich.py --ids 1309 1102 1213 1306
python scripts/load_axes.py --ids 1309 1102 1213 1306
```

确认样板合理后再批量:

```powershell
python scripts/enrich.py --all
python scripts/load_axes.py
```

当前 `4.3.54` 角色 axes 已完成:

```text
characters_with_axes=95
character_axes=2156
```

装备 axes 已完成 v1:

```text
lightcones_with_axes=165
relic_sets_with_axes=60
equipment_axes=1402
```

遗器 axes 由本地中文套装效果规则抽取,已写入 `relic_sets.axes` 和 `equipment_axes`。光锥本地 `lightcone.json` 目前缺少真实效果文本,所以 `lightcones.axes` 是基于命途和 nanoka 推荐角色画像反推的弱 axes,只能用于弱检索/推荐解释,不能当作光锥机制事实。

## 本地向量索引

默认使用本地 `local-hash-ngram-v1` 生成 1024 维向量，不调用外部 embedding API:

注意:`local-hash-ngram-v1` 只是临时 ngram/hash 召回,用于跑通 pgvector 链路和机制词搜索;它不是上线级语义 embedding。M7 暴露 HTTP `semantic_search` 前必须接入真实 embedding provider,否则该接口应显式禁用并让前端使用关键词搜索/筛选。

```powershell
python scripts/embed.py
```

当前 embedding 覆盖:

```text
character_embeddings=95
lightcone_embeddings=165
relic_set_embeddings=60
```

## Go 后端骨架

```powershell
Set-Location hsr-agent-go
go mod tidy
go run ./cmd/hsr-agent
```

当前 Go 入口可以验证数据库连接，也可以直接运行核心 SQL 工具:

```powershell
go run ./cmd/hsr-agent --tool get_character --query 知更鸟
go run ./cmd/hsr-agent --tool semantic_search --kind character --query "需要暴击伤害辅助" --limit 8
go run ./cmd/hsr-agent --tool find_needs --char-id 1309
go run ./cmd/hsr-agent --tool find_buffers_for --axis dmg_percent --target all_allies --limit 5
go run ./cmd/hsr-agent --tool find_synergies --char-id 1309 --limit 8
go run ./cmd/hsr-agent --tool suggest_team --char-id 1309 --limit 4
go run ./cmd/hsr-agent --tool co_occurrence --char-id 1309 --limit 5
go run ./cmd/hsr-agent --tool recommend_lightcones --char-id 1309
go run ./cmd/hsr-agent --tool recommend_relics --char-id 1309
go run ./cmd/hsr-agent --tool get_assets --kind character --id 1309 --variants round,drawcard
```

这些函数已经接入 `internal/agent` 的 LLM tool-use 循环。

## HTTP API 与前端同端口挂载

M7 起 Go 后端可以直接托管前端 build 产物。`/api/*` 固定走后端 API,其余路径走 `WEB_ROOT` 静态文件;找不到文件时回退到 `index.html`,支持前端 SPA 路由刷新:

```powershell
Set-Location hsr-agent-go
go run ./cmd/hsr-agent --serve --addr 127.0.0.1:8080 --web-root ..\frontend\dist
```

环境变量等价配置:

```powershell
$env:HTTP_ADDR='127.0.0.1:8080'
$env:WEB_ROOT='..\frontend\dist'
go run ./cmd/hsr-agent --serve
```

接口契约见 `docs/API.md`。当前 `/api/search/semantic` 在 HTTP 层默认返回 `503 SEMANTIC_SEARCH_DISABLED`,前端先使用 `/api/search/keyword`、筛选和推荐接口。

## Go Agent 问答

Go 侧 Agent 使用 OpenAI-compatible `/v1/chat/completions` tool calls:

```powershell
Set-Location hsr-agent-go
$env:LLM_BASE_URL='https://api.deepseek.com'
$env:LLM_API_FORMAT='openai'
$env:LLM_MODEL='deepseek-chat'
$env:LLM_API_KEY='<set-in-shell-only>'
go run ./cmd/hsr-agent --ask "花火配什么队"
```

验收/调试时可以加 `--trace-tools`，它会把实际 tool calls 输出到 stderr:

```powershell
go run ./cmd/hsr-agent --ask "花火配什么队" --trace-tools
```

## M5 机制与数值校验准备

新增机制表迁移:

```powershell
python scripts/migrate.py
```

先查看单角色可追溯来源,不调用 LLM:

```powershell
python scripts/extract_modifiers.py --ids 1306 --dry-run
```

只把技能/行迹/星魂来源写入 PG,不抽取 modifiers:

```powershell
python scripts/extract_modifiers.py --ids 1306 1309 1303 1101 1205 1308 1005 1310 1203 1217 1304 1208 --sources-only
```

配置 `LLM_API_KEY` 后再执行正式抽取。抽取结果写入 `character_modifiers`,不是长期维护 JSON 文件:

```powershell
python scripts/extract_modifiers.py --ids 1306
```

OpenAI-compatible 抽取默认使用流式请求,可降低长请求被网关超时的概率;如需对照非流式行为,加 `--no-stream`。

当前 `4.3.54` 角色 modifiers 已完成:

```text
characters_with_modifiers=95
character_effect_source_characters=95
character_modifiers=2274
```

机制规格见 `docs/MECHANICS.md`。第一版只做局部数值校验,不做完整行动轴或遗器优化。

Go 侧最小计算内核位于 `hsr-agent-go/internal/calc`,可用常规测试验证:

```powershell
Set-Location hsr-agent-go
go test ./internal/calc
```

M5.5 机制工具已接入 Go CLI 和 Agent:

```powershell
Set-Location hsr-agent-go
go run ./cmd/hsr-agent --tool list_character_modifiers --char-id 1306 --limit 8
go run ./cmd/hsr-agent --tool explain_modifier_sources --char-id 1306 --limit 4
go run ./cmd/hsr-agent --tool compare_character_fit --char-id 1205 --support-id 1306 --attack-tag basic
go run ./cmd/hsr-agent --tool estimate_damage_gain --char-id 1205 --support-ids 1306 --attack-tag basic
go run ./cmd/hsr-agent --tool estimate_dot_damage --char-id 1005 --support-ids 1308
go run ./cmd/hsr-agent --tool estimate_break_damage --char-id 1310 --support-ids 1303 --element fire --break-effect 1.8 --max-toughness 90
go run ./cmd/hsr-agent --tool estimate_super_break_damage --char-id 1310 --support-ids 1303 --element fire --break-effect 1.8 --toughness-reduction 30 --super-break-multiplier 1
go run ./cmd/hsr-agent --tool estimate_healing --char-id 1203 --scaling-stat atk --ability-multiplier 0.6 --flat-value 800
go run ./cmd/hsr-agent --tool estimate_shield --char-id 1304 --scaling-stat def --ability-multiplier 0.3 --flat-value 500
go run ./cmd/hsr-agent --tool estimate_uptime --duration-turns 2 --cooldown-turns 3
```

机制估算默认按 E0 处理,不计入星魂来源;需要纳入全部星魂时加 `--include-eidolons`,只启用指定星魂时用 `--eidolons 1,2,6`。这些工具只做局部场景估算,不导入真实角色面板/遗器/光锥,也不模拟完整行动轴。
