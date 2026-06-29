# 后端需求:对话持久化 + Agent 行为溯源

> 状态:**需求文档(待 Codex 实现)**。本文档定义两块后端能力:① 对话信息持久化保存;② 可溯源的 Agent 行为记录(审计/追踪)。包含数据模型、API 契约、与现有代码的集成点、非功能要求与验收标准。

---

## 1. 背景与现状

### 1.1 目标
- **对话持久化**:用户与 agent 的多轮对话要落库,支持会话列表、历史回看、删除;刷新/换设备不丢。
- **Agent 行为溯源**:每次问答要可追溯 —— 调用了哪些工具、传了什么参数、返回了什么、各步耗时、用了哪个模型、消耗多少 token、成功/失败/中断。用于调试、质量分析、向用户解释"答案是怎么来的"。

### 1.2 现状(代码已确认,均为缺失)
| 项 | 现状 |
|---|---|
| 对话存储 | ❌ DB 无 `conversations`/`messages`/`session` 等表;`agent.Run()` 无状态,每次独立 |
| 工具调用记录 | ❌ HTTP 模式不落库。`TraceWriter`(`agent.go:46/150/168`)仅 CLI `--trace-tools` 写 stderr 文本 |
| token 用量 | ❌ `agent.chat` 只解析 `finish_reason`(`agent.go:101`),**未解析 response.usage** |
| 追踪 id | ❌ 无 trace_id / request id |
| 集成入口 | `RunWithEvents(ctx, userMessage, emit func(Event))`(`agent.go:120`);emit 已发 `tool_call`/`tool_result` 事件,`httpapi.handleAgentStream`(`server.go:370`)在 emit 里 `writeSSE` |

**关键机会**:`RunWithEvents` 的 emit 回调已经把工具调用流推出来了,溯源**只需在同一处把事件收集落库**,对 `agent` 包侵入很小。

---

## 2. 需求 A:对话信息持久化

### 2.1 数据模型
```sql
-- 会话
CREATE TABLE conversations (
    id           BIGSERIAL PRIMARY KEY,
    session_id   TEXT,                         -- 匿名会话标识(无鉴权时由前端生成/后端下发)
    user_id      TEXT,                         -- 预留多用户,当前可空
    title        TEXT,                         -- 首条用户消息摘要,可空
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    meta         JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX ON conversations (session_id, updated_at DESC);

-- 消息(对话历史正文)
CREATE TABLE messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('user','assistant')),
    content         TEXT NOT NULL,
    turn_id         BIGINT,                     -- assistant 消息关联其溯源 turn(见需求 B)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON messages (conversation_id, created_at);
```

### 2.2 写入时机
在 `handleAgent` / `handleAgentStream`:
1. 取 `conversation_id`(请求体新增字段,空 → 新建 conversation,`title` 取用户消息前若干字)。
2. 插入 `messages`(role=user)。
3. 跑 agent(见需求 B 创建 turn)。
4. 结束后插入 `messages`(role=assistant, content=最终回答, turn_id=本轮 turn),并 `UPDATE conversations SET updated_at=now()`。

### 2.3 API
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/conversations?session_id=&limit=&offset=` | 会话列表(按 updated_at 倒序,分页) |
| GET | `/api/conversations/{id}` | 会话详情,含 messages 数组 |
| DELETE | `/api/conversations/{id}` | 删除会话(级联删 messages/turns/tool_calls) |
| PATCH | `/api/conversations/{id}` | 改 title(可选) |

`POST /api/agent/chat` 与 `/chat/stream` 的请求体在 `{message}` 基础上新增可选 `conversation_id`;响应(及 SSE)需回带 `conversation_id` 与 `trace_id`(见 B),供前端后续查询。

---

## 3. 需求 B:Agent 行为溯源

一次用户提问 = 一个 **turn**;turn 内含若干 **tool_call**。

### 3.1 数据模型
```sql
-- 溯源主记录:一次问答
CREATE TABLE agent_turns (
    id                BIGSERIAL PRIMARY KEY,
    conversation_id   BIGINT REFERENCES conversations(id) ON DELETE CASCADE,
    trace_id          TEXT NOT NULL UNIQUE,     -- 对外可引用(uuid)
    user_message      TEXT NOT NULL,
    final_answer      TEXT,
    model             TEXT,                      -- 实际使用的 LLM 模型
    status            TEXT NOT NULL,             -- running|completed|error|aborted|max_steps
    step_count        INT NOT NULL DEFAULT 0,    -- LLM 往返轮数(agent.go for step 循环数)
    tool_call_count   INT NOT NULL DEFAULT 0,
    prompt_tokens     INT,                       -- 若 LLM 返回 usage,累计
    completion_tokens INT,
    total_tokens      INT,
    error_code        TEXT,
    error_message     TEXT,
    latency_ms        INT,                       -- 整轮耗时
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at       TIMESTAMPTZ
);
CREATE INDEX ON agent_turns (conversation_id, started_at DESC);

-- 工具调用明细
CREATE TABLE agent_tool_calls (
    id           BIGSERIAL PRIMARY KEY,
    turn_id      BIGINT NOT NULL REFERENCES agent_turns(id) ON DELETE CASCADE,
    seq          INT NOT NULL,                   -- 本轮内调用顺序(0,1,2...)
    tool_name    TEXT NOT NULL,
    arguments    JSONB,                          -- 模型传入的参数(Event.Args)
    result       JSONB,                          -- 工具返回(压缩后,Event.Result)
    is_error     BOOLEAN NOT NULL DEFAULT FALSE,
    error_text   TEXT,                           -- Event.Error
    latency_ms   INT,                            -- 该工具执行耗时
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON agent_tool_calls (turn_id, seq);
```

### 3.2 记录内容来源(对接现有 emit 流)
| 字段 | 来源 |
|---|---|
| tool_call 的 name/arguments | `Event{Type:"tool_call", Name, Args}`(`agent.go:148`) |
| tool_call 的 result/error | `Event{Type:"tool_result", Name, Result, Error}`(`agent.go:159-165`) |
| step_count / tool_call_count | agent loop 计数(`for step` 与 tool 循环) |
| model | `Runner.config.Model` |
| latency(整轮/单工具) | handler 计时,或在 Event 增加耗时字段(见 5.1) |
| token usage | **需新增**:`agent.chat` 解析 response.usage(见 5.2) |
| status | 正常 end_turn=completed;达到 8 步上限=max_steps;LLM/工具错误=error;前端断开/`ctx` 取消=aborted |

### 3.3 写入时机(集成点)
在 `handleAgentStream` / `handleAgent`:
1. 进入时:创建 `agent_turns`(status=running, trace_id=uuid, started_at)。
2. emit 回调里(已 writeSSE 处):把 `tool_call`/`tool_result` 收集到内存 buffer(或边到边插 `agent_tool_calls`)。
3. 正常结束:`UPDATE agent_turns` 写 final_answer/status=completed/latency/usage/counts/finished_at,批量 `INSERT agent_tool_calls`。
4. 异常(error/abort/max_steps):同样落库已收集到的 tool_calls + 对应 status + error_code/message。
5. SSE 在 `status`(started)或 `final` 事件中携带 `trace_id` + `conversation_id`。

### 3.4 API
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/conversations/{id}/turns` | 该会话所有 turn 摘要(时间、状态、模型、tool_call_count、tokens、latency) |
| GET | `/api/turns/{trace_id}` | 单 turn 完整溯源:基本信息 + 按 seq 排序的 tool_calls(含 args/result/耗时/错误) |

---

## 4. 数据模型关系一览
```
conversations 1──* messages
conversations 1──* agent_turns 1──* agent_tool_calls
messages.turn_id ──> agent_turns.id   (assistant 消息 ↔ 其溯源)
```

---

## 5. 实现注意事项

### 5.1 耗时(latency)采集
现有 `Event` 无时间戳。两种方案任选:
- **低侵入**:在 `agent.go` 给 `Event` 增加 `LatencyMs int` 字段,`dispatchTool` 前后 `time.Since` 填入 tool_result 事件;整轮耗时由 handler 记 start/end。
- **零侵入**:handler 在收到相邻 `tool_call`→`tool_result` 间记时(精度略低)。
> 注:`agent` 包内不能用 `Date.now` 之类被禁的调用?不适用 —— 这是 Go,正常使用 `time.Now()`。

### 5.2 token usage 采集
`agent.chat` 当前未解析 usage。需:
- 在 chat response 结构体加 `Usage struct{ PromptTokens, CompletionTokens, TotalTokens int }`(OpenAI 兼容字段 `usage.prompt_tokens` 等)。
- 多轮 LLM 调用累加到 turn。
- 若渠道不返回 usage(部分网关不返回),字段留空,不阻断。

### 5.3 不阻塞 SSE 流式体验
- 流式过程中**优先把事件推给前端**;落库放到 turn 结束后批量执行,或用独立 goroutine + channel 异步写,避免 DB 慢拖累首字/流式延迟。
- 落库失败**不得影响**用户拿到回答(记录 error log,降级为"答案正常返回但本轮未持久化")。

### 5.4 隐私与安全
- **LLM API key 绝不入库**:`agent_turns.model` 只存模型名,不存 key/base_url 中的凭证。
- `arguments`/`result` 入库前沿用现有 `compactToolResult`(`agent.go:157`)压缩,避免超大 JSON。
- `user_message`/`final_answer` 属用户数据,入库为预期行为;需支持按 conversation 删除(级联)以满足清理诉求。

### 5.5 trace_id 生成
用 uuid(`github.com/google/uuid`)或 `crypto/rand`;`agent_turns.trace_id` 唯一,对前端/日志公开可引用。

### 5.6 迁移
新增 `migrations/003_persistence.sql`(沿用 `scripts/migrate.py` 的 checksum 机制),包含上述 4 张表 + 索引。

---

## 6. 非功能需求
- **性能**:落库不增加 SSE 首字延迟(异步/turn 末批量)。
- **可靠**:持久化失败降级,不影响在线问答。
- **保留策略**:支持按会话删除级联清理;可选后续加 TTL/定期归档(本期不强制)。
- **向后兼容**:`conversation_id` 为可选字段;不传时自动建会话,旧前端调用不报错。

---

## 7. 验收标准
1. 发起一次流式问答后,DB 中 `conversations`/`messages`/`agent_turns`/`agent_tool_calls` 均有正确、关联完整的记录。
2. `GET /api/conversations/{id}` 返回完整多轮对话(user+assistant 顺序正确)。
3. `GET /api/turns/{trace_id}` 返回该次问答按顺序排列的工具调用链(每个含 tool_name/arguments/result/latency/is_error);与 SSE 实时显示的轨迹一致。
4. turn 记录了 model、step_count、tool_call_count、status;若渠道返回 usage 则有 token 数。
5. 中断(前端断开)与达到工具调用上限的场景,turn.status 分别为 `aborted`/`max_steps`,且已发生的 tool_calls 被保留。
6. 任何表/接口/日志中**不出现 LLM API key**。
7. 落库逻辑不显著增加流式延迟;落库失败时用户仍正常收到回答。
8. `go test ./...` 通过,新增至少:1 个持久化写入测试 + 1 个溯源查询 handler 测试。

---

## 8. 边界(本期不做)
- 不做多用户鉴权(`user_id` 字段预留)。
- 不做对话全文检索(可后续)。
- 不存 LLM 完整 raw prompt/请求体(只存 user_message + 工具链 + final_answer;若将来要完整审计,可加 `agent_turns.raw JSONB`,默认不开)。
- 不做前端页面(本文档只定义后端能力;前端会话列表/历史回看/溯源展示另行对接)。
