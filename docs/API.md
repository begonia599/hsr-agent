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

常用错误码:`BAD_REQUEST`,`NOT_FOUND`,`METHOD_NOT_ALLOWED`,`DB_UNAVAILABLE`,`SEMANTIC_SEARCH_DISABLED`,`LLM_NOT_CONFIGURED`,`LLM_UPSTREAM_ERROR`,`TOOL_EXECUTION_ERROR`。

## Health

`GET /api/health`

返回数据库、LLM 和 embedding 配置状态。当前 HTTP semantic search 默认禁用,避免把 `local-hash-ngram-v1` 当真语义搜索暴露给前端。

## Search

`GET /api/search/keyword?q=花火&kind=character&limit=10`

`kind` 可取 `character`、`lightcone`、`relic_set`、`all`。

`GET /api/search/semantic?q=击破辅助&kind=character&limit=10`

当前返回 `503 SEMANTIC_SEARCH_DISABLED`;等真实 embedding 生成和查询链路接入后再开放。

## Characters

`GET /api/characters?q=&role=&element=&path=&rarity=&limit=40`

`GET /api/characters/{id}`

`GET /api/characters/{id}/assets?variants=round,drawcard`

`GET /api/assets/{kind}/{id}?variants=round,drawcard`

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

`GET /api/relic-sets?q=&kind=&limit=40`

`GET /api/relic-sets/{id}`

## Agent

非流式:

```http
POST /api/agent/chat
Content-Type: application/json
```

```json
{"message":"花火怎么配队"}
```

流式 SSE:

```http
POST /api/agent/chat/stream
Content-Type: application/json
Accept: text/event-stream
```

事件格式:

```text
event: tool_call
data: {"type":"tool_call","name":"get_character","args":{"query":"花火"}}

event: tool_result
data: {"type":"tool_result","name":"get_character","result":{...}}

event: final
data: {"message":"..."}

event: error
data: {"code":"LLM_UPSTREAM_ERROR","message":"..."}
```

注意:当前 SSE 会流式输出工具调用轨迹和最终答案;LLM token 级 delta 需要后续把 Agent 上游请求切到 chat completion streaming。

## Mechanics

机制接口统一 `POST application/json`。

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
