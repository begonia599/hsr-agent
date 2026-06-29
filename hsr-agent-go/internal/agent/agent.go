package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"hsr-agent-go/internal/calc"
	apptools "hsr-agent-go/internal/tools"
)

const DefaultSystemPrompt = `你是崩坏星穹铁道的配队/抽取顾问。

回答任何配队/抽取问题前必须遵守:
1. 用户提到的所有角色,先用 get_character 拉出来,确认 id、命途、元素、roles。
2. 对每个核心角色,调用 find_needs 看它需要什么轴的支援。
3. 用 find_buffers_for / find_synergies / co_occurrence 拉候选。
4. 用户意图模糊、只描述机制或装备时,可用 semantic_search 做召回,再用精确工具核对。
5. 必要时用 recommend_lightcones / recommend_relics 拉装备建议。
6. 涉及"是否契合"、"提升多少"、"数值加成"、"为什么适合"时,必须调用 list_character_modifiers / compare_character_fit / estimate_damage_gain 中至少一个机制工具。
7. 对关键数值结论,用 explain_modifier_sources 或 list_character_modifiers 给出来源依据。
8. 不要捏造未在工具返回中出现的角色或机制。
9. 引用角色/光锥/遗器时,优先复用工具返回里的 char_id/item_id/id,写成站内 markdown 链接,如 [流萤](/characters/1310)、[荡除蠹灾的铁骑](/relic-sets/119)。不要自己编造 id/url。
10. 如果最终答案要提到某个角色/光锥/遗器,但当前工具结果里没有可靠 id,生成最终答案前必须用 resolve_entities 一次性批量解析; found=false 的实体只写纯文本,不要加链接。

格式:
- 用国服译名,不要用英文直译。
- 引用任何角色/光锥/遗器时使用站内 markdown 链接;不确定 id 时才附纯文本名称。
- 最终建议给出 1-3 套队伍,每套说明 buff 链如何成立。
- 列出考虑过的关键候选和排除理由。
- 涉及数值估算时,明确说明它是默认面板的局部估算,不是完整行动轴或实战伤害。`

const MechanicSystemPromptAddon = `补充规则:
- 涉及 DoT、击破、超击破、治疗、护盾或覆盖率时,优先调用 estimate_dot_damage / estimate_break_damage / estimate_super_break_damage / estimate_healing / estimate_shield / estimate_uptime。
- 默认不导入真实角色面板、遗器或光锥; 数值结论必须说明这是局部场景估算。
- 涉及按敌人数分档的效果时,调用击破/超击破工具必须传 enemy_count; 用户未说明敌人数时,比较 1/3/5 敌或明确默认值。
- 默认按 E0; 用户提到 E1/E2/E6/星魂/命座时,用 eidolons 精确传入对应开关,不要直接默认全星魂。`

type Config struct {
	BaseURL     string
	APIKey      string
	Model       string
	TraceWriter io.Writer
}

type Runner struct {
	config Config
	tools  *apptools.Service
	client *http.Client
}

type Event struct {
	Type       string          `json:"type"`
	TraceID    string          `json:"trace_id,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
	Result     any             `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
}

func New(config Config, tools *apptools.Service) *Runner {
	return &Runner{
		config: config,
		tools:  tools,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Tools       []toolDef `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

type toolDef struct {
	Type     string       `json:"type"`
	Function functionSpec `json:"function"`
}

type functionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func (r *Runner) Run(ctx context.Context, userMessage string) (string, error) {
	return r.run(ctx, userMessage, nil)
}

func (r *Runner) RunWithEvents(ctx context.Context, userMessage string, emit func(Event)) (string, error) {
	return r.run(ctx, userMessage, emit)
}

func (r *Runner) run(ctx context.Context, userMessage string, emit func(Event)) (string, error) {
	if strings.TrimSpace(r.config.APIKey) == "" {
		return "", fmt.Errorf("LLM_API_KEY is required")
	}
	messages := []message{
		{Role: "system", Content: DefaultSystemPrompt + "\n" + MechanicSystemPromptAddon},
		{Role: "user", Content: userMessage},
	}

	for step := 0; step < 8; step++ {
		resp, err := r.chat(ctx, messages, true)
		if err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("LLM returned no choices")
		}
		assistantMsg := resp.Choices[0].Message
		messages = append(messages, assistantMsg)
		if len(assistantMsg.ToolCalls) == 0 {
			return assistantMsg.Content, nil
		}
		for _, call := range assistantMsg.ToolCalls {
			if emit != nil {
				emit(Event{Type: "tool_call", ToolCallID: call.ID, Name: call.Function.Name, Args: safeRawJSON(call.Function.Arguments)})
			}
			if r.config.TraceWriter != nil {
				fmt.Fprintf(r.config.TraceWriter, "tool_call name=%s args=%s\n", call.Function.Name, call.Function.Arguments)
			}
			result, err := r.dispatchTool(ctx, call.Function.Name, call.Function.Arguments)
			if err != nil {
				result = map[string]any{"error": err.Error()}
			}
			result = compactToolResult(call.Function.Name, result)
			if emit != nil {
				event := Event{Type: "tool_result", ToolCallID: call.ID, Name: call.Function.Name, Result: result}
				if row, ok := result.(map[string]any); ok {
					if text, ok := row["error"].(string); ok {
						event.Error = text
					}
				}
				emit(event)
			}
			data, _ := json.Marshal(result)
			if r.config.TraceWriter != nil {
				fmt.Fprintf(r.config.TraceWriter, "tool_result name=%s bytes=%d\n", call.Function.Name, len(data))
			}
			messages = append(messages, message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    string(data),
			})
		}
	}

	messages = append(messages, message{
		Role:    "user",
		Content: "工具调用已达到上限。请停止调用工具,只基于以上工具结果给出最终中文回答;如果信息不足,明确说明不确定点。",
	})
	resp, err := r.chat(ctx, messages, false)
	if err != nil {
		return "", fmt.Errorf("agent reached max tool-use steps and finalization failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("agent reached max tool-use steps and LLM returned no final choices")
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("agent reached max tool-use steps and LLM returned empty final answer")
	}
	return content, nil
}

func safeRawJSON(text string) json.RawMessage {
	raw := json.RawMessage(strings.TrimSpace(text))
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(`{}`)
	}
	return raw
}

func (r *Runner) chat(ctx context.Context, messages []message, withTools bool) (*chatResponse, error) {
	reqBody := chatRequest{
		Model:       r.config.Model,
		Messages:    messages,
		Temperature: 0.2,
		MaxTokens:   4096,
	}
	if withTools {
		reqBody.Tools = toolDefinitions()
		reqBody.ToolChoice = "auto"
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL(r.config.BaseURL), bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+r.config.APIKey)
		req.Header.Set("Content-Type", "application/json")
		res, err := r.client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < 3 {
				sleepBeforeRetry(ctx, attempt)
				continue
			}
			return nil, err
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode < 300 {
			var out chatResponse
			if err := json.Unmarshal(body, &out); err != nil {
				return nil, err
			}
			return &out, nil
		}
		lastErr = fmt.Errorf("LLM HTTP %d: %s", res.StatusCode, string(body))
		if res.StatusCode < 500 || attempt == 3 {
			return nil, lastErr
		}
		if r.config.TraceWriter != nil {
			fmt.Fprintf(r.config.TraceWriter, "llm_retry attempt=%d status=%d\n", attempt, res.StatusCode)
		}
		sleepBeforeRetry(ctx, attempt)
	}
	return nil, lastErr
}

func sleepBeforeRetry(ctx context.Context, attempt int) {
	timer := time.NewTimer(time.Duration(attempt*2) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func chatURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func toolDefinitions() []toolDef {
	return []toolDef{
		tool("get_character", "Look up a character by id or Chinese/English name.", object(map[string]any{
			"query": stringSchema("id or name"),
		}, []string{"query"})),
		tool("resolve_entities", "Resolve character, lightcone, and relic-set names into authoritative site links. Use batch mode before final answer for entities without reliable ids; do not guess when found=false.", object(map[string]any{
			"entities": arraySchema(object(map[string]any{
				"name": stringSchema("entity name or id"),
				"kind": stringSchema("character/lightcone/relic_set"),
			}, []string{"name", "kind"})),
			"display": stringSchema("link/image/both, default link"),
		}, []string{"entities"})),
		tool("semantic_search", "Fuzzy pgvector search across characters, lightcones, and relic sets.", object(map[string]any{
			"query": stringSchema("Chinese user intent or mechanics text"),
			"kind":  stringSchema("character/lightcone/relic_set/all, default character"),
			"limit": integerSchema("max rows"),
		}, []string{"query"})),
		tool("find_needs", "Get axes needed by a character.", object(map[string]any{
			"char_id": integerSchema("character id"),
		}, []string{"char_id"})),
		tool("find_buffers_for", "Find characters that provide an axis.", object(map[string]any{
			"axis":   stringSchema("axis stat"),
			"target": stringSchema("target, default all_allies"),
			"limit":  integerSchema("max rows"),
		}, []string{"axis"})),
		tool("find_synergies", "Find synergistic characters for a core character.", object(map[string]any{
			"char_id": integerSchema("character id"),
			"limit":   integerSchema("max rows"),
		}, []string{"char_id"})),
		tool("suggest_team", "Suggest a heuristic team around a core character.", object(map[string]any{
			"char_id": integerSchema("character id"),
			"slots":   integerSchema("team slots, default 4"),
			"exclude": arraySchema(integerSchema("excluded character id")),
		}, []string{"char_id"})),
		tool("co_occurrence", "Get nanoka team co-occurrence candidates.", object(map[string]any{
			"char_id": integerSchema("character id"),
			"limit":   integerSchema("max rows"),
		}, []string{"char_id"})),
		tool("recommend_lightcones", "Get nanoka recommended lightcones.", object(map[string]any{
			"char_id": integerSchema("character id"),
		}, []string{"char_id"})),
		tool("recommend_relics", "Get nanoka recommended relic sets.", object(map[string]any{
			"char_id": integerSchema("character id"),
		}, []string{"char_id"})),
		tool("get_assets", "Get local/CDN image assets.", object(map[string]any{
			"entity_kind": stringSchema("character/lightcone/relic_set/item/path/element/slot"),
			"entity_id":   stringSchema("entity id"),
			"variants":    arraySchema(stringSchema("variant")),
		}, []string{"entity_kind", "entity_id"})),
		tool("list_character_modifiers", "List extracted mechanics modifiers for one character.", object(map[string]any{
			"char_id":      integerSchema("character id"),
			"stat_key":     stringSchema("optional modifier stat_key filter"),
			"target_scope": stringSchema("optional target_scope filter"),
			"limit":        integerSchema("max rows"),
		}, []string{"char_id"})),
		tool("explain_modifier_sources", "Return source text snippets and modifiers for traceable mechanics explanation.", object(map[string]any{
			"char_id": integerSchema("character id"),
			"limit":   integerSchema("max source rows"),
		}, []string{"char_id"})),
		tool("compare_character_fit", "Heuristically compare whether a support's modifiers fit an attacker.", object(map[string]any{
			"attacker_id":      integerSchema("attacker/core character id"),
			"support_id":       integerSchema("support/candidate character id"),
			"attack_tag":       stringSchema("optional attack tag: basic/skill/ult/fua/dot/break/super_break"),
			"include_eidolons": booleanSchema("include eidolon modifiers; default false/E0"),
			"eidolons":         arraySchema(integerSchema("enabled eidolons, e.g. [1,2,6]; default empty/E0")),
		}, []string{"attacker_id", "support_id"})),
		tool("estimate_damage_gain", "Estimate local standard-damage multiplier from support modifiers under a default scenario.", object(map[string]any{
			"attacker_id":      integerSchema("attacker/core character id"),
			"support_ids":      arraySchema(integerSchema("support character id")),
			"attack_tag":       stringSchema("optional attack tag: basic/skill/ult/fua/dot/break/super_break"),
			"include_eidolons": booleanSchema("include eidolon modifiers; default false/E0"),
			"eidolons":         arraySchema(integerSchema("enabled eidolons, e.g. [1,2,6]; default empty/E0")),
		}, []string{"attacker_id", "support_ids"})),
		tool("estimate_dot_damage", "Estimate local DoT damage multiplier; DoT ignores crit by default.", object(map[string]any{
			"attacker_id":      integerSchema("attacker/core character id"),
			"support_ids":      arraySchema(integerSchema("support character id; defaults to self if empty")),
			"include_eidolons": booleanSchema("include eidolon modifiers; default false/E0"),
			"eidolons":         arraySchema(integerSchema("enabled eidolons, e.g. [1,2,6]; default empty/E0")),
		}, []string{"attacker_id"})),
		tool("estimate_break_damage", "Estimate local break damage with explicit break scenario inputs.", object(map[string]any{
			"attacker_id":      integerSchema("attacker/core character id"),
			"support_ids":      arraySchema(integerSchema("support character id; defaults to self if empty")),
			"element":          stringSchema("physical/fire/ice/thunder/wind/quantum/imaginary; default attacker element"),
			"break_effect":     floatSchema("break effect decimal, e.g. 1.8 for 180%"),
			"break_dmg_bonus":  floatSchema("break damage bonus decimal"),
			"max_toughness":    floatSchema("enemy max toughness, default 90"),
			"enemy_count":      integerSchema("enemy count for conditional modifiers, default 1"),
			"enemy_resistance": floatSchema("enemy resistance decimal, default 0.2"),
			"def_reduction":    floatSchema("enemy defense reduction decimal"),
			"def_ignore":       floatSchema("defense ignore decimal"),
			"res_reduction":    floatSchema("enemy resistance reduction decimal"),
			"res_pen":          floatSchema("resistance penetration decimal"),
			"vulnerability":    floatSchema("vulnerability decimal"),
			"damage_reduction": floatSchema("enemy damage reduction decimal"),
			"include_eidolons": booleanSchema("include eidolon modifiers; default false/E0"),
			"eidolons":         arraySchema(integerSchema("enabled eidolons, e.g. [1,2,6]; default empty/E0")),
		}, []string{"attacker_id"})),
		tool("estimate_super_break_damage", "Estimate local super break damage with explicit toughness reduction inputs.", object(map[string]any{
			"attacker_id":            integerSchema("attacker/core character id"),
			"support_ids":            arraySchema(integerSchema("support character id; defaults to self if empty")),
			"element":                stringSchema("physical/fire/ice/thunder/wind/quantum/imaginary; default attacker element"),
			"break_effect":           floatSchema("break effect decimal, e.g. 1.8 for 180%"),
			"break_dmg_bonus":        floatSchema("break damage bonus decimal"),
			"super_break_dmg_bonus":  floatSchema("super break damage bonus decimal"),
			"toughness_reduction":    floatSchema("attack toughness reduction, default 30"),
			"super_break_multiplier": floatSchema("super break base multiplier, default 1"),
			"enemy_count":            integerSchema("enemy count for conditional modifiers, default 1"),
			"enemy_resistance":       floatSchema("enemy resistance decimal, default 0.2"),
			"def_reduction":          floatSchema("enemy defense reduction decimal"),
			"def_ignore":             floatSchema("defense ignore decimal"),
			"res_reduction":          floatSchema("enemy resistance reduction decimal"),
			"res_pen":                floatSchema("resistance penetration decimal"),
			"vulnerability":          floatSchema("vulnerability decimal"),
			"damage_reduction":       floatSchema("enemy damage reduction decimal"),
			"include_eidolons":       booleanSchema("include eidolon modifiers; default false/E0"),
			"eidolons":               arraySchema(integerSchema("enabled eidolons, e.g. [1,2,6]; default empty/E0")),
		}, []string{"attacker_id"})),
		tool("estimate_healing", "Estimate local healing value from scaling stat, ability multiplier, flat heal, and modifiers.", object(map[string]any{
			"char_id":            integerSchema("healer character id"),
			"support_ids":        arraySchema(integerSchema("support character id; defaults to self if empty")),
			"scaling_stat":       stringSchema("atk/hp/def; inferred if empty"),
			"base_scaling_stat":  floatSchema("base scaling stat, default 1000"),
			"ability_multiplier": floatSchema("ability multiplier, default 1"),
			"flat_value":         floatSchema("flat heal value"),
			"include_eidolons":   booleanSchema("include eidolon modifiers; default false/E0"),
			"eidolons":           arraySchema(integerSchema("enabled eidolons, e.g. [1,2,6]; default empty/E0")),
		}, []string{"char_id"})),
		tool("estimate_shield", "Estimate local shield value from scaling stat, ability multiplier, flat shield, and modifiers.", object(map[string]any{
			"char_id":            integerSchema("shielder character id"),
			"support_ids":        arraySchema(integerSchema("support character id; defaults to self if empty")),
			"scaling_stat":       stringSchema("atk/hp/def; inferred if empty"),
			"base_scaling_stat":  floatSchema("base scaling stat, default 1000"),
			"ability_multiplier": floatSchema("ability multiplier, default 1"),
			"flat_value":         floatSchema("flat shield value"),
			"include_eidolons":   booleanSchema("include eidolon modifiers; default false/E0"),
			"eidolons":           arraySchema(integerSchema("enabled eidolons, e.g. [1,2,6]; default empty/E0")),
		}, []string{"char_id"})),
		tool("estimate_uptime", "Estimate simple duration/cycle uptime ratio.", object(map[string]any{
			"duration_turns":    floatSchema("active duration in turns"),
			"cooldown_turns":    floatSchema("cooldown or refresh interval in turns"),
			"cycle_turns":       floatSchema("cycle length; defaults to cooldown then duration"),
			"start_delay_turns": floatSchema("start delay inside the first cycle"),
		}, []string{"duration_turns"})),
	}
}

func tool(name string, description string, parameters map[string]any) toolDef {
	return toolDef{Type: "function", Function: functionSpec{Name: name, Description: description, Parameters: parameters}}
}

func object(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func floatSchema(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func booleanSchema(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func arraySchema(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func (r *Runner) dispatchTool(ctx context.Context, name string, rawArgs string) (any, error) {
	var args map[string]any
	if strings.TrimSpace(rawArgs) != "" {
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return nil, err
		}
	}
	switch name {
	case "get_character":
		return r.tools.GetCharacter(ctx, strArg(args, "query"))
	case "resolve_entities":
		return r.tools.ResolveEntities(ctx, entityRequestsArg(args, "entities"), strArgDefault(args, "display", "link"))
	case "semantic_search":
		return r.tools.SemanticSearch(ctx, strArg(args, "query"), strArgDefault(args, "kind", "character"), intArgDefault(args, "limit", 10))
	case "find_needs":
		return r.tools.FindNeeds(ctx, intArg(args, "char_id"))
	case "find_buffers_for":
		return r.tools.FindBuffersFor(ctx, strArg(args, "axis"), strArgDefault(args, "target", "all_allies"), intArgDefault(args, "limit", 10))
	case "find_synergies":
		return r.tools.FindSynergies(ctx, intArg(args, "char_id"), intArgDefault(args, "limit", 8))
	case "suggest_team":
		return r.tools.SuggestTeam(ctx, intArg(args, "char_id"), intArgDefault(args, "slots", 4), intSliceArg(args, "exclude"))
	case "co_occurrence":
		return r.tools.CoOccurrence(ctx, intArg(args, "char_id"), intArgDefault(args, "limit", 10))
	case "recommend_lightcones":
		return r.tools.RecommendLightcones(ctx, intArg(args, "char_id"))
	case "recommend_relics":
		return r.tools.RecommendRelics(ctx, intArg(args, "char_id"))
	case "get_assets":
		return r.tools.GetAssets(ctx, strArg(args, "entity_kind"), strArg(args, "entity_id"), strSliceArg(args, "variants"))
	case "list_character_modifiers":
		return r.tools.ListCharacterModifiers(ctx, intArg(args, "char_id"), strArg(args, "stat_key"), strArg(args, "target_scope"), intArgDefault(args, "limit", 40))
	case "explain_modifier_sources":
		return r.tools.ExplainModifierSources(ctx, intArg(args, "char_id"), intArgDefault(args, "limit", 12))
	case "compare_character_fit":
		return r.tools.CompareCharacterFitWithOptions(ctx, intArg(args, "attacker_id"), intArg(args, "support_id"), strArg(args, "attack_tag"), modifierOptionsArg(args))
	case "estimate_damage_gain":
		return r.tools.EstimateDamageGainWithOptions(ctx, intArg(args, "attacker_id"), intSliceArg(args, "support_ids"), strArg(args, "attack_tag"), modifierOptionsArg(args))
	case "estimate_dot_damage":
		return r.tools.EstimateDotDamage(ctx, intArg(args, "attacker_id"), intSliceArg(args, "support_ids"), modifierOptionsArg(args))
	case "estimate_break_damage":
		return r.tools.EstimateBreakDamage(ctx, intArg(args, "attacker_id"), intSliceArg(args, "support_ids"), calc.BreakScenario{
			ElementKey:       strArg(args, "element"),
			EnemyCount:       intArgDefault(args, "enemy_count", 1),
			BreakEffect:      floatArgDefault(args, "break_effect", 0),
			BreakDamageBonus: floatArgDefault(args, "break_dmg_bonus", 0),
			MaxToughness:     floatArgDefault(args, "max_toughness", 90),
			Resistance:       floatArgDefault(args, "enemy_resistance", 0.2),
			DefReduction:     floatArgDefault(args, "def_reduction", 0),
			DefIgnore:        floatArgDefault(args, "def_ignore", 0),
			ResReduction:     floatArgDefault(args, "res_reduction", 0),
			ResPen:           floatArgDefault(args, "res_pen", 0),
			Vulnerability:    floatArgDefault(args, "vulnerability", 0),
			DamageReduction:  floatArgDefault(args, "damage_reduction", 0),
		}, modifierOptionsArg(args))
	case "estimate_super_break_damage":
		return r.tools.EstimateSuperBreakDamage(ctx, intArg(args, "attacker_id"), intSliceArg(args, "support_ids"), calc.BreakScenario{
			ElementKey:           strArg(args, "element"),
			EnemyCount:           intArgDefault(args, "enemy_count", 1),
			BreakEffect:          floatArgDefault(args, "break_effect", 0),
			BreakDamageBonus:     floatArgDefault(args, "break_dmg_bonus", 0),
			SuperBreakBonus:      floatArgDefault(args, "super_break_dmg_bonus", 0),
			ToughnessReduction:   floatArgDefault(args, "toughness_reduction", 30),
			SuperBreakMultiplier: floatArgDefault(args, "super_break_multiplier", 1),
			Resistance:           floatArgDefault(args, "enemy_resistance", 0.2),
			DefReduction:         floatArgDefault(args, "def_reduction", 0),
			DefIgnore:            floatArgDefault(args, "def_ignore", 0),
			ResReduction:         floatArgDefault(args, "res_reduction", 0),
			ResPen:               floatArgDefault(args, "res_pen", 0),
			Vulnerability:        floatArgDefault(args, "vulnerability", 0),
			DamageReduction:      floatArgDefault(args, "damage_reduction", 0),
		}, modifierOptionsArg(args))
	case "estimate_healing":
		return r.tools.EstimateHealing(ctx, intArg(args, "char_id"), intSliceArg(args, "support_ids"), strArg(args, "scaling_stat"), calc.SustainScenario{
			BaseScalingStat:   floatArgDefault(args, "base_scaling_stat", 1000),
			AbilityMultiplier: floatArgDefault(args, "ability_multiplier", 1),
			FlatValue:         floatArgDefault(args, "flat_value", 0),
		}, modifierOptionsArg(args))
	case "estimate_shield":
		return r.tools.EstimateShield(ctx, intArg(args, "char_id"), intSliceArg(args, "support_ids"), strArg(args, "scaling_stat"), calc.SustainScenario{
			BaseScalingStat:   floatArgDefault(args, "base_scaling_stat", 1000),
			AbilityMultiplier: floatArgDefault(args, "ability_multiplier", 1),
			FlatValue:         floatArgDefault(args, "flat_value", 0),
		}, modifierOptionsArg(args))
	case "estimate_uptime":
		return r.tools.EstimateUptime(ctx, calc.UptimeScenario{
			DurationTurns:   floatArgDefault(args, "duration_turns", 0),
			CooldownTurns:   floatArgDefault(args, "cooldown_turns", 0),
			CycleTurns:      floatArgDefault(args, "cycle_turns", 0),
			StartDelayTurns: floatArgDefault(args, "start_delay_turns", 0),
		})
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func compactToolResult(name string, result any) any {
	switch value := result.(type) {
	case *apptools.Character:
		return compactCharacter(value)
	case []apptools.Synergy:
		if len(value) > 8 {
			return value[:8]
		}
	case []apptools.CoOccurrence:
		if len(value) > 8 {
			return value[:8]
		}
	case []apptools.SemanticMatch:
		if len(value) > 8 {
			return value[:8]
		}
	case []apptools.ModifierRow:
		if len(value) > 24 {
			return value[:24]
		}
	case []apptools.EffectSourceExplanation:
		if len(value) > 8 {
			return value[:8]
		}
	case *apptools.FitResult:
		if len(value.UsefulEffects) > 10 {
			value.UsefulEffects = value.UsefulEffects[:10]
		}
		if len(value.LowValueEffects) > 5 {
			value.LowValueEffects = value.LowValueEffects[:5]
		}
	case *apptools.DamageGainEstimate:
		if len(value.Applied) > 16 {
			value.Applied = value.Applied[:16]
		}
		if len(value.Skipped) > 10 {
			value.Skipped = value.Skipped[:10]
		}
	case *apptools.MechanicEstimate:
		if len(value.Applied) > 16 {
			value.Applied = value.Applied[:16]
		}
		if len(value.Skipped) > 10 {
			value.Skipped = value.Skipped[:10]
		}
	}
	return result
}

func compactCharacter(c *apptools.Character) map[string]any {
	out := map[string]any{
		"id":              c.ID,
		"name_zh":         c.NameZH,
		"name_en":         c.NameEN,
		"url":             fmt.Sprintf("/characters/%d", c.ID),
		"markdown":        fmt.Sprintf("[%s](/characters/%d)", c.NameZH, c.ID),
		"rarity":          c.Rarity,
		"path":            c.Path,
		"element":         c.Element,
		"roles":           c.Roles,
		"sp_need":         c.SPNeed,
		"is_trailblazer":  c.IsTrailblazer,
		"is_collab":       c.IsCollab,
		"is_variant":      c.IsVariant,
		"skill_text_hint": truncate(c.SkillTextBrief, 120),
	}
	if len(c.Axes) > 0 {
		var axes map[string]any
		if err := json.Unmarshal(c.Axes, &axes); err == nil {
			out["axes"] = compactAxes(axes)
		}
	}
	return out
}

func compactAxes(axes map[string]any) map[string]any {
	out := make(map[string]any)
	if tags, ok := axes["tags"]; ok {
		out["tags"] = tags
	}
	if roles, ok := axes["roles"]; ok {
		out["roles"] = roles
	}
	if notes, ok := axes["notes"].(string); ok {
		out["notes"] = truncate(notes, 120)
	}
	for _, key := range []string{"needs", "provides", "restricts"} {
		if rows, ok := axes[key].([]any); ok {
			out[key] = compactAxisRows(rows, 8)
		}
	}
	return out
}

func compactAxisRows(rows []any, limit int) []map[string]any {
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item, ok := row.(map[string]any)
		if !ok {
			continue
		}
		compact := make(map[string]any)
		for _, key := range []string{"stat", "target", "value", "uptime", "condition", "source", "reason"} {
			if value, ok := item[key]; ok {
				if text, ok := value.(string); ok {
					compact[key] = truncate(text, 70)
				} else {
					compact[key] = value
				}
			}
		}
		out = append(out, compact)
	}
	return out
}

func truncate(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func strArg(args map[string]any, key string) string {
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}

func strArgDefault(args map[string]any, key string, fallback string) string {
	if value := strArg(args, key); value != "" {
		return value
	}
	return fallback
}

func intArg(args map[string]any, key string) int {
	return intArgDefault(args, key, 0)
}

func intArgDefault(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return fallback
	}
}

func boolArgDefault(args map[string]any, key string, fallback bool) bool {
	if value, ok := args[key].(bool); ok {
		return value
	}
	return fallback
}

func floatArgDefault(args map[string]any, key string, fallback float64) float64 {
	switch value := args[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return fallback
	}
}

func modifierOptionsArg(args map[string]any) apptools.ModifierOptions {
	return apptools.NewModifierOptions(
		boolArgDefault(args, "include_eidolons", false),
		intSliceArg(args, "eidolons"),
	)
}

func strSliceArg(args map[string]any, key string) []string {
	values, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && text != "" {
			out = append(out, text)
		}
	}
	return out
}

func intSliceArg(args map[string]any, key string) []int {
	values, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if number, ok := value.(float64); ok {
			out = append(out, int(number))
		}
	}
	return out
}

func entityRequestsArg(args map[string]any, key string) []apptools.EntityResolveRequest {
	values, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]apptools.EntityResolveRequest, 0, len(values))
	for _, value := range values {
		row, ok := value.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, apptools.EntityResolveRequest{
			Name: strArg(row, "name"),
			Kind: strArg(row, "kind"),
		})
	}
	return out
}
