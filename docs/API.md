# HSR Agent HTTP API

M7 后端以同一端口同时提供 API、SSE 和前端静态文件:

```powershell
Set-Location hsr-agent-go
go run ./cmd/hsr-agent --serve
```

默认监听 `HTTP_ADDR=127.0.0.1:8080`,默认前端目录 `WEB_ROOT=web/dist`。也可以用参数覆盖:

```powershell
go run ./cmd/hsr-agent --serve --addr 127.0.0.1:8080 --web-root ..\frontend\dist
```

路由约定:

- `/api/*` 永远是后端 JSON/SSE API。
- 非 `/api/*` 先按 `WEB_ROOT` 查静态文件;找不到时回退到 `WEB_ROOT/index.html`,支持前端 SPA history 路由刷新。
- 同端口部署不需要 CORS 或前端 dev proxy。

## 通用错误

```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "message is required"
  }
}
```

常用错误码:`BAD_REQUEST`,`NOT_FOUND`,`METHOD_NOT_ALLOWED`,`DB_UNAVAILABLE`,`SEMANTIC_SEARCH_DISABLED`,`SEMANTIC_SEARCH_NOT_READY`,`LLM_NOT_CONFIGURED`,`LLM_UPSTREAM_ERROR`,`TOOL_EXECUTION_ERROR`。

## Health

`GET /api/health`

返回数据库、LLM 和 embedding 配置状态。HTTP semantic search 只有在 `EMBEDDING_PROVIDER=openai_compatible` 且对应模型在 `entity_embeddings` 中完成覆盖时可用。

## Models

`GET /api/models`

返回前端可展示的 embedding / reranker 模型 catalog,不返回任何 API key。

- `embedding.default_id`:默认 embedding id。
- `embedding.query_cache`:在线 query embedding 短期缓存配置,包含 `enabled`、`ttl_seconds`、`max_entries`。
- `embedding.models[].ready/selectable`:只有 `entity_embeddings` 中该 `embedding_model_id` 对角色、光锥、遗器三类实体全量覆盖,且 provider/model/storage_dimensions/quality 匹配时才为 `true`。
- `embedding.models[].metadata`:按实体类型返回 `rows`、`expected_rows`、`storage_dimensions`、`native_dimensions`、`projection_strategy` 和 ready 状态。
- `rerank.default_id`:默认 reranker id。
- `rerank.default_top_n`:默认送入 reranker 精排的候选数。
- `rerank.max_documents`:当前 moark rerank 端点实测最大 `documents` 数,后端会自动截到该上限。
- `rerank.models[].selectable`:该 reranker 配置完整即可为 `true`;`bge-reranker-v2-m3` 已接入 `/api/search/semantic` 精排。

前端选择 embedding 时只应允许选择 `selectable=true` 的项;否则应提示需要先重建向量。

## Search

`GET /api/search/keyword?q=花火&kind=character&limit=10`

`kind` 可取 `character`、`lightcone`、`relic_set`、`all`。

`GET /api/search/semantic?q=击破辅助&kind=character&limit=10&embedding_model_id=bge-m3&rerank_model_id=bge-reranker-v2-m3`

可选参数:

- `embedding_model_id`:选择已 ready 的 embedding 模型;省略时使用默认模型。
- `rerank_model_id`:选择 reranker;省略时使用默认 reranker。
- `rerank=false`:关闭 reranker,只用向量召回和本地规则排序。
- `rerank_top_n`:送入 reranker 的候选数;后端会截到 `rerank.max_documents`。
- `recall_limit`:每类向量粗召回候选数;默认按 rerank/top-k 自动扩大。semantic API 还会额外合并 name exact / pg_trgm / 机制关键词补召回候选。
- `include_meta=true` 或 `format=object`:返回 `{query, kind, limit, count, items}` envelope;默认保持旧行为,直接返回数组。

未配置真实 embedding 时返回 `503 SEMANTIC_SEARCH_DISABLED`;选择了未完成离线向量覆盖的模型时返回 `503 SEMANTIC_SEARCH_NOT_READY`。启用步骤:

1. 在 `.env` 中配置 `EMBEDDING_MODEL_IDS` 和对应的 `EMBEDDING_MODEL_<ID>_*` catalog。
2. 运行 `python scripts/migrate.py`。
3. 运行 `python scripts/embed.py --model-id bge-m3 --kind all --force`,把该模型的 characters/lightcones/relic_sets 向量写入 `entity_embeddings`。
4. 启动后端 `go run ./cmd/hsr-agent --serve`。

返回结果包含 `url`、`markdown`、`recall_source`、`recall_score`、`rule_score`、`rerank_score`、`final_score`、`embedding_provider`、`embedding_model`、`embedding_dimensions`、`embedding_quality`、`rerank_model_id` 和 `score_explain`。`url` 是站内路径,`markdown` 可直接用于 agent/富文本渲染。`recall_source` 可为 `embedding`、`keyword` 或 `embedding+keyword`。reranker 未配置、关闭或上游错误时接口降级为本地规则排序,并在 `score_explain` 标记原因。

搜索回归:

```powershell
python scripts/search_regression.py --base-url http://127.0.0.1:8080
python scripts/search_regression.py --base-url http://127.0.0.1:8080 --rerank false
```

## Characters

`GET /api/characters?q=&role=&element=&path=&rarity=&limit=40`

`GET /api/characters/{id}`

`GET /api/characters/{id}/assets?variants=round,drawcard`

`GET /api/assets/{kind}/{id}?variants=round,drawcard`

## Entity Links

`POST /api/entities/resolve`

单实体也可以用 GET:

`GET /api/entities/resolve?name=流萤&kind=character&display=both`

```json
{
  "display": "both",
  "entities": [
    {"name": "流萤", "kind": "character"},
    {"name": "梦应归于何处", "kind": "lightcone"},
    {"name": "荡除蠹灾的铁骑", "kind": "relic_set"}
  ]
}
```

返回站内 URL、可直接用于 markdown 的链接和可选图片 URL。低相似度时 `found=false`,后端不会猜:

```json
[
  {
    "name": "流萤",
    "kind": "character",
    "found": true,
    "id": 1310,
    "name_zh": "流萤",
    "url": "/characters/1310",
    "image_url": "https://static.nanoka.cc/assets/hsr/avatarroundicon/1310.webp",
    "markdown": "[流萤](/characters/1310)",
    "score": 1
  }
]
```

`GET /api/characters/{id}/needs`

`GET /api/characters/{id}/synergies?limit=8`

`GET /api/characters/{id}/teams?slots=4&exclude=1306,1309`

`GET /api/characters/{id}/lightcones`

`GET /api/characters/{id}/relics`

`GET /api/characters/{id}/modifiers?stat_key=&target_scope=&limit=40`

`GET /api/characters/{id}/modifier-sources?limit=12`

## Equipment

`GET /api/lightcones?q=&path=&rarity=&limit=40`

`GET /api/lightcones/{id}`

光锥效果文本已从 `nanoka_hsr/4.3.54/<lang>/lightcone/{id}.json` 的 `refinements.desc` 入库,并已用 LLM 重建 equipment axes。正常响应会返回:

- `data_quality: "effect_text_extracted"`
- `axes`:包含 LLM 抽取的 `provides/needs/restricts/tags`
- `desc_zh`:包含光锥技能名、叠影 1 渲染文本和参数

`GET /api/lightcones/{id}/refinements`

返回 `lightcones.raw_zh->'refinements'` 原始 JSON,用于前端叠影滑杆按 1-5 级渲染占位符文本。后端不解析 `#N[i]` / `#N[f1]` / `#N[f2]`,前端复用角色技能文本解析器。

响应示例:

```json
{
  "name": "抚慰",
  "desc": "我方角色每次攻击时...能量恢复效率提高#1[f1]%...",
  "level": {
    "1": {"param_list": [0.03, 5, 0.24, 0.48, 1]},
    "5": {"param_list": [0.05, 5, 0.4, 0.96, 1]}
  }
}
```

如果该光锥缺少叠影原始结构,返回 `null`。

`GET /api/relic-sets?q=&kind=&limit=40`

`GET /api/relic-sets/{id}`

## Agent

非流式:

```http
POST /api/agent/chat
Content-Type: application/json
```

```json
{"message":"花火怎么配队","conversation_id":123,"session_id":"anon-browser-session"}
```

`conversation_id` 和 `session_id` 可选。不传 `conversation_id` 时,后端会自动创建会话;旧 `{message}` 调用保持可用。

响应:

```json
{"message":"...","trace_id":"...","conversation_id":123}
```

响应头也会包含 `X-Trace-Id`。

流式 SSE:

```http
POST /api/agent/chat/stream
Content-Type: application/json
Accept: text/event-stream
```

事件格式:

```text
event: tool_call
data: {"type":"tool_call","trace_id":"...","tool_call_id":"...","name":"get_character","args":{"query":"花火"}}

event: tool_result
data: {"type":"tool_result","trace_id":"...","tool_call_id":"...","name":"get_character","result":{...}}

event: final
data: {"message":"...","trace_id":"...","conversation_id":123}

event: error
data: {"code":"LLM_UPSTREAM_ERROR","message":"...","trace_id":"...","conversation_id":123}
```

注意:当前 SSE 会流式输出工具调用轨迹和最终答案;LLM token 级 delta 需要后续把 Agent 上游请求切到 chat completion streaming。

## Conversations

M7.6 起,Agent 问答会写入会话、消息和 tool trace。落库失败不会阻断实时回答;历史查询接口需要先运行 `python scripts/migrate.py` 应用 `006_persistence_audit.sql`。

`GET /api/conversations?session_id=&limit=20&offset=0`

按 `updated_at` 倒序返回会话列表,包含 `last_message`。

`GET /api/conversations/{id}`

返回会话详情和按时间排序的 `messages`:

```json
{
  "id": 123,
  "session_id": "anon-browser-session",
  "title": "花火怎么配队",
  "messages": [
    {"id": 1, "role": "user", "content": "花火怎么配队", "turn_id": null},
    {"id": 2, "role": "assistant", "content": "...", "turn_id": 9}
  ]
}
```

`PATCH /api/conversations/{id}`

```json
{"title":"花火配队"}
```

`DELETE /api/conversations/{id}`

删除会话并级联删除消息、turn 和 tool trace。

`GET /api/conversations/{id}/turns`

返回该会话的 Agent turn 摘要,包含 `trace_id/status/model/latency_ms/tool_call_count/token usage/error`。

`GET /api/turns/{trace_id}`

返回单次问答的完整工具链:

```json
{
  "trace_id": "...",
  "conversation_id": 123,
  "status": "completed",
  "tool_calls": [
    {
      "seq": 0,
      "tool_call_id": "call_...",
      "name": "get_character",
      "args": {"query": "花火"},
      "result": {"id": 1306, "name_zh": "花火"},
      "latency_ms": 12
    }
  ]
}
```

## Mechanics

机制接口统一 `POST application/json`。

通用机制字段:

- `include_eidolons` / `eidolons`:星魂开关,默认 E0。
- `active_contexts`:额外启用的场景上下文,如 `technique`、`combat_start`、`on_break`、`on_wave_start`。
- `inactive_contexts`:强制关闭的场景上下文,如 `skill_active`、`ult_active`。
- 默认启用 `passive`、`field_active`、`skill_active`、`ult_active`、`conditional`、`on_attack`;默认不启用秘技、开局、击破后、波次开始和一次性瞬发效果。
- 结果会返回 `active_contexts`、`inactive_contexts`、`applied_by_side`、`skipped_by_side`;被场景过滤或 non-stacking 去重的 modifier 会在 `skipped_modifiers[].skip_reason` 标注原因。
- `super_break_base_multiplier`:超击破基础倍率推荐字段;`super_break_multiplier` 保留为旧别名;`super_break_dmg_bonus` 只表示超击破增伤区。
- `source_panels`:可选施放者面板覆盖,用于解析 `source_stat_dependency`。格式如 `[{"character_id":8005,"break_effect":2.4},{"character_id":1306,"crit_dmg":2.0}]`;不传时默认 `crit_dmg=1.0`、`break_effect=1.8`。

`POST /api/mechanics/compare-character-fit`

```json
{"attacker_id":1310,"support_id":1222,"attack_tag":"super_break","include_eidolons":true,"eidolons":[6]}
```

`POST /api/mechanics/estimate-damage-gain`

`POST /api/mechanics/estimate-dot-damage`

`POST /api/mechanics/estimate-break-damage`

`POST /api/mechanics/estimate-super-break-damage`

```json
{
  "attacker_id": 1310,
  "support_ids": [1222],
  "enemy_count": 3,
  "break_effect": 2.5,
  "toughness_reduction": 30,
  "super_break_base_multiplier": 1,
  "source_panels": [{"character_id": 8005, "break_effect": 2.4}],
  "active_contexts": ["technique"],
  "include_eidolons": true,
  "eidolons": [6]
}
```

`POST /api/mechanics/estimate-healing`

`POST /api/mechanics/estimate-shield`

```json
{"char_id":1203,"support_ids":[],"scaling_stat":"hp","base_scaling_stat":5000,"ability_multiplier":0.1,"flat_value":200}
```

`POST /api/mechanics/estimate-uptime`

```json
{"duration_turns":2,"cooldown_turns":3}
```
