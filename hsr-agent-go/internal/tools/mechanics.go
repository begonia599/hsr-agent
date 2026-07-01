package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"hsr-agent-go/internal/calc"
)

type CharacterRef struct {
	ID      int      `json:"id"`
	NameZH  string   `json:"name_zh"`
	Rarity  int      `json:"rarity,omitempty"`
	Path    string   `json:"path,omitempty"`
	Element string   `json:"element,omitempty"`
	Roles   []string `json:"roles,omitempty"`
}

type ModifierRow struct {
	CharacterID          int             `json:"character_id"`
	CharacterNameZH      string          `json:"character_name_zh,omitempty"`
	SourceID             int64           `json:"source_id"`
	SourceKind           string          `json:"source_kind"`
	SourceKey            string          `json:"source_key"`
	SourceNameZH         string          `json:"source_name_zh"`
	ModifierID           int64           `json:"modifier_id"`
	TargetScope          string          `json:"target_scope"`
	StatKey              string          `json:"stat_key"`
	Value                *float64        `json:"value,omitempty"`
	ValueUnit            string          `json:"value_unit"`
	ModifierZone         string          `json:"modifier_zone"`
	AttackTag            string          `json:"attack_tag,omitempty"`
	ElementKey           string          `json:"element_key,omitempty"`
	TargetPath           string          `json:"target_path,omitempty"`
	ConditionText        string          `json:"condition_text,omitempty"`
	ConditionJSON        json.RawMessage `json:"condition_jsonb,omitempty"`
	SourceStatDependency json.RawMessage `json:"source_stat_dependency,omitempty"`
	DurationKey          string          `json:"duration_key,omitempty"`
	StackRule            string          `json:"stack_rule,omitempty"`
	EffectSide           string          `json:"effect_side,omitempty"`
	Confidence           float64         `json:"confidence"`
	Reviewed             bool            `json:"reviewed"`
}

type ModifierBrief struct {
	ModifierID           int64           `json:"modifier_id"`
	SourceKind           string          `json:"source_kind,omitempty"`
	SourceKey            string          `json:"source_key,omitempty"`
	SourceNameZH         string          `json:"source_name_zh,omitempty"`
	StatKey              string          `json:"stat_key"`
	Value                *float64        `json:"value,omitempty"`
	ValueUnit            string          `json:"value_unit"`
	ModifierZone         string          `json:"modifier_zone"`
	TargetScope          string          `json:"target_scope"`
	EffectSide           string          `json:"effect_side,omitempty"`
	ActiveContext        string          `json:"active_context,omitempty"`
	SkipReason           string          `json:"skip_reason,omitempty"`
	AttackTag            string          `json:"attack_tag,omitempty"`
	ElementKey           string          `json:"element_key,omitempty"`
	ConditionText        string          `json:"condition_text,omitempty"`
	SourceStatDependency json.RawMessage `json:"source_stat_dependency,omitempty"`
	Reviewed             bool            `json:"reviewed"`
	Confidence           float64         `json:"confidence"`
}

type SourcePanel struct {
	CharacterID int      `json:"character_id,omitempty"`
	Atk         *float64 `json:"atk,omitempty"`
	HP          *float64 `json:"hp,omitempty"`
	Def         *float64 `json:"def,omitempty"`
	CritDamage  *float64 `json:"crit_dmg,omitempty"`
	BreakEffect *float64 `json:"break_effect,omitempty"`
}

type SourceStatDependency struct {
	Source string  `json:"source"`
	Stat   string  `json:"stat"`
	Ratio  float64 `json:"ratio"`
	Flat   float64 `json:"flat,omitempty"`
}

type EffectSourceExplanation struct {
	SourceID      int64           `json:"source_id"`
	CharacterID   int             `json:"character_id"`
	CharacterName string          `json:"character_name_zh"`
	SourceKind    string          `json:"source_kind"`
	SourceKey     string          `json:"source_key"`
	SourceNameZH  string          `json:"source_name_zh"`
	SourceTextZH  string          `json:"source_text_zh"`
	SourceHash    string          `json:"source_hash"`
	ModifierCount int             `json:"modifier_count"`
	Modifiers     []ModifierBrief `json:"modifiers,omitempty"`
}

type FitModifier struct {
	Modifier    ModifierBrief `json:"modifier"`
	Score       float64       `json:"score"`
	AxisMatches []string      `json:"axis_matches,omitempty"`
	Reason      string        `json:"reason"`
}

type FitResult struct {
	Attacker        CharacterRef  `json:"attacker"`
	Support         CharacterRef  `json:"support"`
	Score           float64       `json:"score"`
	Rating          string        `json:"rating"`
	AttackTag       string        `json:"attack_tag,omitempty"`
	AttackerNeeds   []string      `json:"attacker_needs,omitempty"`
	AttackerTags    []string      `json:"attacker_tags,omitempty"`
	UsefulEffects   []FitModifier `json:"useful_effects"`
	LowValueEffects []FitModifier `json:"low_value_effects,omitempty"`
	Caveats         []string      `json:"caveats,omitempty"`
	Notes           []string      `json:"notes,omitempty"`
}

type DamageGainEstimate struct {
	Attacker         CharacterRef    `json:"attacker"`
	Supports         []CharacterRef  `json:"supports"`
	AttackTag        string          `json:"attack_tag,omitempty"`
	ScalingStat      string          `json:"scaling_stat"`
	Eidolons         []int           `json:"eidolons,omitempty"`
	ActiveContexts   []string        `json:"active_contexts,omitempty"`
	InactiveContexts []string        `json:"inactive_contexts,omitempty"`
	Baseline         calc.Breakdown  `json:"baseline"`
	WithModifiers    calc.Breakdown  `json:"with_modifiers"`
	TotalMultiplier  float64         `json:"total_multiplier"`
	DamageGainPct    float64         `json:"damage_gain_pct"`
	Applied          []ModifierBrief `json:"applied_modifiers,omitempty"`
	Skipped          []ModifierBrief `json:"skipped_modifiers,omitempty"`
	AppliedBySide    ModifierGroups  `json:"applied_by_side,omitempty"`
	SkippedBySide    ModifierGroups  `json:"skipped_by_side,omitempty"`
	Assumptions      []string        `json:"assumptions"`
	Caveats          []string        `json:"caveats,omitempty"`
}

type MechanicEstimate struct {
	Mechanic         string          `json:"mechanic"`
	Subject          CharacterRef    `json:"subject"`
	Supports         []CharacterRef  `json:"supports,omitempty"`
	AttackTag        string          `json:"attack_tag,omitempty"`
	ScalingStat      string          `json:"scaling_stat,omitempty"`
	ElementKey       string          `json:"element_key,omitempty"`
	EnemyCount       int             `json:"enemy_count,omitempty"`
	Eidolons         []int           `json:"eidolons,omitempty"`
	ActiveContexts   []string        `json:"active_contexts,omitempty"`
	InactiveContexts []string        `json:"inactive_contexts,omitempty"`
	Baseline         any             `json:"baseline"`
	WithModifiers    any             `json:"with_modifiers"`
	TotalMultiplier  float64         `json:"total_multiplier"`
	GainPct          float64         `json:"gain_pct"`
	Applied          []ModifierBrief `json:"applied_modifiers,omitempty"`
	Skipped          []ModifierBrief `json:"skipped_modifiers,omitempty"`
	AppliedBySide    ModifierGroups  `json:"applied_by_side,omitempty"`
	SkippedBySide    ModifierGroups  `json:"skipped_by_side,omitempty"`
	Assumptions      []string        `json:"assumptions"`
	Caveats          []string        `json:"caveats,omitempty"`
}

type ModifierGroups map[string][]ModifierBrief

type ModifierOptions struct {
	IncludeEidolons    bool          `json:"include_eidolons"`
	Eidolons           []int         `json:"eidolons,omitempty"`
	ActiveContexts     []string      `json:"active_contexts,omitempty"`
	InactiveContexts   []string      `json:"inactive_contexts,omitempty"`
	SourcePanels       []SourcePanel `json:"source_panels,omitempty"`
	eidolonSet         map[string]bool
	activeContextSet   map[string]bool
	inactiveContextSet map[string]bool
	sourcePanelByID    map[int]SourcePanel
}

func NewModifierOptions(includeEidolons bool, eidolons []int) ModifierOptions {
	return NewModifierOptionsWithContexts(includeEidolons, eidolons, nil, nil)
}

func NewModifierOptionsWithContexts(includeEidolons bool, eidolons []int, activeContexts []string, inactiveContexts []string) ModifierOptions {
	options := ModifierOptions{IncludeEidolons: includeEidolons}
	seen := map[int]bool{}
	for _, eidolon := range eidolons {
		if eidolon < 1 || eidolon > 6 || seen[eidolon] {
			continue
		}
		seen[eidolon] = true
		options.Eidolons = append(options.Eidolons, eidolon)
	}
	sort.Ints(options.Eidolons)
	options.eidolonSet = make(map[string]bool, len(options.Eidolons)*2)
	for _, eidolon := range options.Eidolons {
		key := fmt.Sprint(eidolon)
		options.eidolonSet[key] = true
		options.eidolonSet["e"+key] = true
	}
	options.ActiveContexts = normalizeContextList(activeContexts)
	options.InactiveContexts = normalizeContextList(inactiveContexts)
	options.activeContextSet = defaultActiveContextSet()
	for _, contextKey := range options.ActiveContexts {
		options.activeContextSet[contextKey] = true
	}
	options.inactiveContextSet = make(map[string]bool, len(options.InactiveContexts))
	for _, contextKey := range options.InactiveContexts {
		options.inactiveContextSet[contextKey] = true
		delete(options.activeContextSet, contextKey)
	}
	return options
}

func NewModifierOptionsWithPanels(includeEidolons bool, eidolons []int, activeContexts []string, inactiveContexts []string, sourcePanels []SourcePanel) ModifierOptions {
	options := NewModifierOptionsWithContexts(includeEidolons, eidolons, activeContexts, inactiveContexts)
	options.SourcePanels = normalizeSourcePanels(sourcePanels)
	options.sourcePanelByID = make(map[int]SourcePanel, len(options.SourcePanels))
	for _, panel := range options.SourcePanels {
		if panel.CharacterID > 0 {
			options.sourcePanelByID[panel.CharacterID] = panel
		}
	}
	return options
}

func (options ModifierOptions) Normalized() ModifierOptions {
	if options.eidolonSet == nil || options.activeContextSet == nil || (len(options.SourcePanels) > 0 && options.sourcePanelByID == nil) {
		return NewModifierOptionsWithPanels(options.IncludeEidolons, options.Eidolons, options.ActiveContexts, options.InactiveContexts, options.SourcePanels)
	}
	return options
}

func (options ModifierOptions) Allows(row ModifierRow) bool {
	if row.SourceKind != "eidolon" {
		return true
	}
	options = options.Normalized()
	if options.IncludeEidolons {
		return true
	}
	key := strings.ToLower(strings.TrimSpace(row.SourceKey))
	return options.eidolonSet[key]
}

func (options ModifierOptions) AssumptionText() string {
	options = options.Normalized()
	if options.IncludeEidolons {
		return "本次 include_eidolons=true,已把全部 eidolon/星魂 来源纳入估算。"
	}
	if len(options.Eidolons) > 0 {
		parts := make([]string, 0, len(options.Eidolons))
		for _, eidolon := range options.Eidolons {
			parts = append(parts, fmt.Sprintf("E%d", eidolon))
		}
		return "本次只启用指定星魂来源: " + strings.Join(parts, ", ") + "。"
	}
	return "默认按 E0 估算,不计入 eidolon/星魂 来源;需要星魂时传 eidolons=[1..6] 或 include_eidolons=true。"
}

func (options ModifierOptions) ContextAllows(row ModifierRow) (bool, string) {
	options = options.Normalized()
	contextKey := inferActiveContext(row)
	if options.inactiveContextSet[contextKey] {
		return false, "inactive_context:" + contextKey
	}
	if options.activeContextSet[contextKey] {
		return true, ""
	}
	return false, "inactive_context:" + contextKey
}

func (options ModifierOptions) ActiveContextList() []string {
	options = options.Normalized()
	out := make([]string, 0, len(options.activeContextSet))
	for contextKey := range options.activeContextSet {
		out = append(out, contextKey)
	}
	sort.Strings(out)
	return out
}

func (options ModifierOptions) InactiveContextList() []string {
	options = options.Normalized()
	out := make([]string, 0, len(options.inactiveContextSet))
	for contextKey := range options.inactiveContextSet {
		out = append(out, contextKey)
	}
	sort.Strings(out)
	return out
}

func (options ModifierOptions) ContextAssumptionText() string {
	return "默认机制场景启用常驻、战技/终结技持续类效果; technique/combat_start/on_break 等触发场景需要 active_contexts 显式开启,也可用 inactive_contexts 排除。"
}

func (options ModifierOptions) SourcePanelAssumptionText() string {
	return "施放者面板依赖默认按 crit_dmg=100%、break_effect=180% 估算; 可用 source_panels 按角色覆盖。"
}

func normalizeSourcePanels(panels []SourcePanel) []SourcePanel {
	out := make([]SourcePanel, 0, len(panels))
	seen := map[int]bool{}
	for _, panel := range panels {
		if panel.CharacterID <= 0 || seen[panel.CharacterID] {
			continue
		}
		seen[panel.CharacterID] = true
		out = append(out, panel)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CharacterID < out[j].CharacterID })
	return out
}

func (options ModifierOptions) ResolveSourceStatDependency(row ModifierRow) (ModifierRow, bool) {
	dependency, ok := parseSourceStatDependency(row.SourceStatDependency)
	if !ok {
		return row, false
	}
	statValue, ok := options.sourceStatValue(row.CharacterID, dependency.Stat)
	if !ok {
		return row, false
	}
	value := statValue*dependency.Ratio + dependency.Flat
	row.Value = &value
	return row, true
}

func parseSourceStatDependency(raw json.RawMessage) (SourceStatDependency, bool) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return SourceStatDependency{}, false
	}
	var dependency SourceStatDependency
	if err := json.Unmarshal(raw, &dependency); err != nil {
		return SourceStatDependency{}, false
	}
	dependency.Source = strings.ToLower(strings.TrimSpace(dependency.Source))
	dependency.Stat = normalizeSourcePanelStat(dependency.Stat)
	if dependency.Source != "caster" || dependency.Stat == "" {
		return SourceStatDependency{}, false
	}
	return dependency, true
}

func (options ModifierOptions) sourceStatValue(characterID int, stat string) (float64, bool) {
	options = options.Normalized()
	panel := defaultSourcePanel()
	if override, ok := options.sourcePanelByID[characterID]; ok {
		panel = mergeSourcePanel(panel, override)
	}
	switch normalizeSourcePanelStat(stat) {
	case "atk":
		return derefFloat(panel.Atk, 1000), true
	case "hp":
		return derefFloat(panel.HP, 3000), true
	case "def":
		return derefFloat(panel.Def, 1000), true
	case "crit_dmg":
		return derefFloat(panel.CritDamage, 1.0), true
	case "break_effect":
		return derefFloat(panel.BreakEffect, 1.8), true
	default:
		return 0, false
	}
}

func defaultSourcePanel() SourcePanel {
	return SourcePanel{
		Atk:         floatPtrValue(1000),
		HP:          floatPtrValue(3000),
		Def:         floatPtrValue(1000),
		CritDamage:  floatPtrValue(1.0),
		BreakEffect: floatPtrValue(1.8),
	}
}

func mergeSourcePanel(base SourcePanel, override SourcePanel) SourcePanel {
	base.CharacterID = override.CharacterID
	if override.Atk != nil {
		base.Atk = override.Atk
	}
	if override.HP != nil {
		base.HP = override.HP
	}
	if override.Def != nil {
		base.Def = override.Def
	}
	if override.CritDamage != nil {
		base.CritDamage = override.CritDamage
	}
	if override.BreakEffect != nil {
		base.BreakEffect = override.BreakEffect
	}
	return base
}

func normalizeSourcePanelStat(stat string) string {
	switch strings.ToLower(strings.TrimSpace(stat)) {
	case "atk", "attack":
		return "atk"
	case "hp":
		return "hp"
	case "def", "defense":
		return "def"
	case "crit_dmg", "crit_damage", "critical_damage":
		return "crit_dmg"
	case "break_effect", "break_eff":
		return "break_effect"
	default:
		return ""
	}
}

func derefFloat(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func floatPtrValue(value float64) *float64 {
	return &value
}

func (s *Service) ListCharacterModifiers(ctx context.Context, charID int, statKey string, targetScope string, limit int) ([]ModifierRow, error) {
	if charID == 0 {
		return nil, fmt.Errorf("char_id is required")
	}
	if limit <= 0 || limit > 120 {
		limit = 80
	}
	rows, err := s.db.Query(ctx, `
SELECT character_id, character_name_zh, source_id, source_kind, source_key, source_name_zh,
       modifier_id, target_scope, stat_key, value::float8, value_unit, modifier_zone,
       coalesce(attack_tag, ''), coalesce(element_key, ''), coalesce(target_path, ''),
       coalesce(condition_text, ''), condition_jsonb, source_stat_dependency, coalesce(duration_key, ''),
       coalesce(stack_rule, ''), confidence::float8, reviewed
FROM v_character_modifiers
WHERE character_id = $1
  AND ($2 = '' OR stat_key = $2)
  AND ($3 = '' OR target_scope = $3)
ORDER BY reviewed DESC, confidence DESC, source_id, modifier_id
LIMIT $4`, charID, statKey, targetScope, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ModifierRow
	for rows.Next() {
		item, err := scanModifier(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ExplainModifierSources(ctx context.Context, charID int, limit int) ([]EffectSourceExplanation, error) {
	if charID == 0 {
		return nil, fmt.Errorf("char_id is required")
	}
	if limit <= 0 || limit > 80 {
		limit = 40
	}
	modifiers, err := s.ListCharacterModifiers(ctx, charID, "", "", 160)
	if err != nil {
		return nil, err
	}
	bySource := make(map[int64][]ModifierBrief)
	for _, modifier := range modifiers {
		bySource[modifier.SourceID] = append(bySource[modifier.SourceID], modifierBrief(modifier))
	}

	rows, err := s.db.Query(ctx, `
SELECT s.id, s.character_id, c.name_zh, s.source_kind, s.source_key, s.name_zh,
       left(s.source_text_zh, 900) AS source_text_zh, s.source_hash, count(m.id)::int AS modifier_count
FROM character_effect_sources s
JOIN characters c ON c.id = s.character_id
LEFT JOIN character_modifiers m ON m.source_id = s.id
WHERE s.character_id = $1
GROUP BY s.id, s.character_id, c.name_zh, s.source_kind, s.source_key, s.name_zh, s.source_text_zh, s.source_hash
HAVING count(m.id) > 0
ORDER BY s.source_kind, s.source_key
LIMIT $2`, charID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EffectSourceExplanation
	for rows.Next() {
		var item EffectSourceExplanation
		if err := rows.Scan(
			&item.SourceID, &item.CharacterID, &item.CharacterName, &item.SourceKind,
			&item.SourceKey, &item.SourceNameZH, &item.SourceTextZH, &item.SourceHash,
			&item.ModifierCount,
		); err != nil {
			return nil, err
		}
		item.Modifiers = bySource[item.SourceID]
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) CompareCharacterFit(ctx context.Context, attackerID int, supportID int, attackTag string, includeEidolons bool) (*FitResult, error) {
	return s.CompareCharacterFitWithOptions(ctx, attackerID, supportID, attackTag, NewModifierOptions(includeEidolons, nil))
}

func (s *Service) CompareCharacterFitWithOptions(ctx context.Context, attackerID int, supportID int, attackTag string, options ModifierOptions) (*FitResult, error) {
	if attackerID == 0 || supportID == 0 {
		return nil, fmt.Errorf("attacker_id and support_id are required")
	}
	options = options.Normalized()
	attacker, err := s.GetCharacter(ctx, fmt.Sprint(attackerID))
	if err != nil {
		return nil, err
	}
	support, err := s.GetCharacter(ctx, fmt.Sprint(supportID))
	if err != nil {
		return nil, err
	}
	needs, err := s.axisRows(ctx, `
SELECT ca.char_id, c.name_zh, ca.kind, ca.stat, coalesce(ca.target, ''), ca.value::float8, coalesce(ca.uptime, ''), coalesce(ca.condition, '')
FROM character_axes ca
JOIN characters c ON c.id = ca.char_id
WHERE ca.char_id = $1 AND ca.kind IN ('needs', 'tag')
ORDER BY ca.kind, ca.stat`, attackerID)
	if err != nil {
		return nil, err
	}
	modifiers, err := s.ListCharacterModifiers(ctx, supportID, "", "", 120)
	if err != nil {
		return nil, err
	}

	needSet, needList, tags := splitNeedAxes(needs)
	result := &FitResult{
		Attacker:      characterRef(attacker),
		Support:       characterRef(support),
		AttackTag:     strings.TrimSpace(attackTag),
		AttackerNeeds: needList,
		AttackerTags:  tags,
		Notes: []string{
			"评分是启发式: 基于已抽取 modifiers、受控词表和默认权重,不是完整行动轴模拟。",
			"reviewed=false 的抽取结果可用作低置信依据,高风险结论仍需人工复核。",
			options.AssumptionText(),
			options.SourcePanelAssumptionText(),
		},
	}

	selfSupport := attackerID == supportID
	for _, modifier := range modifiers {
		if !options.Allows(modifier) {
			continue
		}
		if !modifierAffectsDamageSubject(modifier, selfSupport, attacker.Element) {
			continue
		}
		if !modifierRelevantForAttack(modifier, attackTag) {
			continue
		}
		if ok, _ := options.ContextAllows(modifier); !ok {
			continue
		}
		modifier, _ = options.ResolveSourceStatDependency(modifier)
		fit := scoreModifierFit(modifier, needSet, tags)
		if fit.Score >= 5 {
			result.UsefulEffects = append(result.UsefulEffects, fit)
			result.Score += fit.Score
		} else if fit.Score > 0 {
			result.LowValueEffects = append(result.LowValueEffects, fit)
		}
	}
	sortFitModifiers(result.UsefulEffects)
	sortFitModifiers(result.LowValueEffects)
	if len(result.UsefulEffects) > 14 {
		result.UsefulEffects = result.UsefulEffects[:14]
	}
	if len(result.LowValueEffects) > 8 {
		result.LowValueEffects = result.LowValueEffects[:8]
	}

	result.Caveats = fitCaveats(attacker, support, tags, result)
	result.Score = math.Min(100, math.Round(result.Score))
	result.Rating = fitRating(result.Score)
	return result, nil
}

func (s *Service) EstimateDamageGain(ctx context.Context, attackerID int, supportIDs []int, attackTag string, includeEidolons bool) (*DamageGainEstimate, error) {
	return s.EstimateDamageGainWithOptions(ctx, attackerID, supportIDs, attackTag, NewModifierOptions(includeEidolons, nil))
}

func (s *Service) EstimateDamageGainWithOptions(ctx context.Context, attackerID int, supportIDs []int, attackTag string, options ModifierOptions) (*DamageGainEstimate, error) {
	if attackerID == 0 {
		return nil, fmt.Errorf("attacker_id is required")
	}
	if len(supportIDs) == 0 {
		return nil, fmt.Errorf("support_ids is required")
	}
	options = options.Normalized()
	attacker, err := s.GetCharacter(ctx, fmt.Sprint(attackerID))
	if err != nil {
		return nil, err
	}
	axes, err := s.axisRows(ctx, `
SELECT ca.char_id, c.name_zh, ca.kind, ca.stat, coalesce(ca.target, ''), ca.value::float8, coalesce(ca.uptime, ''), coalesce(ca.condition, '')
FROM character_axes ca
JOIN characters c ON c.id = ca.char_id
WHERE ca.char_id = $1 AND ca.kind IN ('needs', 'tag')
ORDER BY ca.kind, ca.stat`, attackerID)
	if err != nil {
		return nil, err
	}
	scalingStat := inferScalingStat(axes)

	estimate := &DamageGainEstimate{
		Attacker:         characterRef(attacker),
		AttackTag:        strings.TrimSpace(attackTag),
		ScalingStat:      scalingStat,
		Eidolons:         options.Eidolons,
		ActiveContexts:   options.ActiveContextList(),
		InactiveContexts: options.InactiveContextList(),
		Assumptions: []string{
			"默认攻击者/敌人等级均为80,敌人基础抗性20%,敌人已破韧以避免韧性状态干扰对比。",
			"默认面板: 1000点主缩放属性、100%技能倍率、50%暴击率、100%暴击伤害。",
			options.SourcePanelAssumptionText(),
			"只计算已能落入常规直伤乘区且有明确数值的 modifiers; 拉条、回能、战技点、治疗、护盾等作为 utility 返回。",
			options.AssumptionText(),
		},
		Caveats: []string{
			"不模拟行动轴、覆盖率、星魂开关、遗器、光锥、敌人机制和队伍循环。",
		},
	}
	estimate.Assumptions = append(estimate.Assumptions, options.ContextAssumptionText())
	var collected collectedModifiers
	for _, supportID := range supportIDs {
		support, err := s.GetCharacter(ctx, fmt.Sprint(supportID))
		if err != nil {
			return nil, err
		}
		estimate.Supports = append(estimate.Supports, characterRef(support))
		rows, err := s.ListCharacterModifiers(ctx, supportID, "", "", 120)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if !options.Allows(row) {
				continue
			}
			if !modifierAffectsDamageSubject(row, supportID == attackerID, attacker.Element) || !modifierRelevantForAttack(row, attackTag) {
				continue
			}
			if ok, reason := options.ContextAllows(row); !ok {
				if isPotentiallyUseful(row) {
					collected.Skipped = append(collected.Skipped, modifierBriefWithSkipReason(row, reason))
				}
				continue
			}
			row, _ = options.ResolveSourceStatDependency(row)
			modifier, ok := toCalcModifier(row, scalingStat, attackTag)
			if !ok {
				if isPotentiallyUseful(row) {
					collected.Skipped = append(collected.Skipped, modifierBrief(row))
				}
				continue
			}
			collected.addApplied(row, modifier)
		}
	}
	estimate.Applied = collected.Applied
	estimate.Skipped = collected.Skipped
	if len(estimate.Applied) > 32 {
		estimate.Applied = estimate.Applied[:32]
	}
	if len(estimate.Skipped) > 24 {
		estimate.Skipped = estimate.Skipped[:24]
	}
	estimate.AppliedBySide = groupModifierBriefsBySide(estimate.Applied)
	estimate.SkippedBySide = groupModifierBriefsBySide(estimate.Skipped)
	calcModifiers := collected.Modifiers

	scenario := calc.Scenario{
		AttackerLevel:     80,
		EnemyLevel:        80,
		BaseScalingStat:   1000,
		AbilityMultiplier: 1,
		CritRate:          0.5,
		CritDamage:        1.0,
		Resistance:        0.2,
		EnemyBroken:       true,
		AttackTag:         strings.TrimSpace(attackTag),
		ElementKey:        strings.ToLower(attacker.Element),
	}
	estimate.Baseline = calc.EstimateStandardDamage(scenario, nil)
	estimate.WithModifiers = calc.EstimateStandardDamage(scenario, calcModifiers)
	if estimate.Baseline.TotalDamage > 0 {
		estimate.TotalMultiplier = estimate.WithModifiers.TotalDamage / estimate.Baseline.TotalDamage
		estimate.DamageGainPct = estimate.TotalMultiplier - 1
	}
	return estimate, nil
}

func (s *Service) EstimateDotDamage(ctx context.Context, attackerID int, supportIDs []int, options ModifierOptions) (*MechanicEstimate, error) {
	if attackerID == 0 {
		return nil, fmt.Errorf("attacker_id is required")
	}
	options = options.Normalized()
	attacker, err := s.GetCharacter(ctx, fmt.Sprint(attackerID))
	if err != nil {
		return nil, err
	}
	axes, err := s.axisRows(ctx, `
SELECT ca.char_id, c.name_zh, ca.kind, ca.stat, coalesce(ca.target, ''), ca.value::float8, coalesce(ca.uptime, ''), coalesce(ca.condition, '')
FROM character_axes ca
JOIN characters c ON c.id = ca.char_id
WHERE ca.char_id = $1 AND ca.kind IN ('needs', 'tag')
ORDER BY ca.kind, ca.stat`, attackerID)
	if err != nil {
		return nil, err
	}
	scalingStat := inferScalingStat(axes)
	collected, err := s.collectModifiers(ctx, attacker, attacker.Element, defaultSupportIDs(attackerID, supportIDs), "dot", options, modifierAffectsDamageSubject, func(row ModifierRow) (calc.Modifier, bool) {
		return toCalcModifier(row, scalingStat, "dot")
	})
	if err != nil {
		return nil, err
	}
	scenario := defaultDamageScenario(attacker, "dot")
	baseline := calc.EstimateDotDamage(scenario, nil)
	withModifiers := calc.EstimateDotDamage(scenario, collected.Modifiers)
	estimate := newMechanicEstimate("dot_damage", attacker, collected, options)
	estimate.AttackTag = "dot"
	estimate.ScalingStat = scalingStat
	estimate.ElementKey = strings.ToLower(attacker.Element)
	estimate.Baseline = baseline
	estimate.WithModifiers = withModifiers
	estimate.Assumptions = append(estimate.Assumptions,
		"DoT 默认不吃暴击,使用 attack_tag=dot 匹配 DoT 增伤、易伤、抗性、减防等 modifier。",
		"默认场景: 80级、1000主缩放属性、100%倍率、敌人基础抗性20%、敌人已破韧。",
	)
	setMechanicRatio(estimate, baseline.TotalDamage, withModifiers.TotalDamage)
	return estimate, nil
}

func (s *Service) EstimateBreakDamage(ctx context.Context, attackerID int, supportIDs []int, scenario calc.BreakScenario, options ModifierOptions) (*MechanicEstimate, error) {
	if attackerID == 0 {
		return nil, fmt.Errorf("attacker_id is required")
	}
	options = options.Normalized()
	attacker, err := s.GetCharacter(ctx, fmt.Sprint(attackerID))
	if err != nil {
		return nil, err
	}
	scenario = normalizeBreakScenario(attacker, scenario)
	collected, err := s.collectModifiers(ctx, attacker, scenario.ElementKey, defaultSupportIDs(attackerID, supportIDs), "break", options, modifierAffectsDamageSubject, func(row ModifierRow) (calc.Modifier, bool) {
		return toBreakCalcModifier(row, false, scenario.EnemyCount)
	})
	if err != nil {
		return nil, err
	}
	baseline := calc.EstimateBreakDamage(scenario, nil)
	withModifiers := calc.EstimateBreakDamage(scenario, collected.Modifiers)
	estimate := newMechanicEstimate("break_damage", attacker, collected, options)
	estimate.AttackTag = "break"
	estimate.ElementKey = scenario.ElementKey
	estimate.EnemyCount = scenario.EnemyCount
	estimate.Baseline = baseline
	estimate.WithModifiers = withModifiers
	estimate.Assumptions = append(estimate.Assumptions,
		"击破伤害按等级倍率、元素击破倍率、敌方最大韧性、击破特攻、击破伤害加成、减防、抗性、易伤和减伤乘区估算。",
		"不模拟击破附加状态的后续结算,例如裂伤/灼烧/风化/冻结/纠缠/禁锢的持续结算。",
	)
	setMechanicRatio(estimate, baseline.TotalDamage, withModifiers.TotalDamage)
	return estimate, nil
}

func (s *Service) EstimateSuperBreakDamage(ctx context.Context, attackerID int, supportIDs []int, scenario calc.BreakScenario, options ModifierOptions) (*MechanicEstimate, error) {
	if attackerID == 0 {
		return nil, fmt.Errorf("attacker_id is required")
	}
	options = options.Normalized()
	attacker, err := s.GetCharacter(ctx, fmt.Sprint(attackerID))
	if err != nil {
		return nil, err
	}
	scenario = normalizeBreakScenario(attacker, scenario)
	if scenario.ToughnessReduction == 0 {
		scenario.ToughnessReduction = 30
	}
	collected, err := s.collectModifiers(ctx, attacker, scenario.ElementKey, defaultSupportIDs(attackerID, supportIDs), "super_break", options, modifierAffectsDamageSubject, func(row ModifierRow) (calc.Modifier, bool) {
		return toBreakCalcModifier(row, true, scenario.EnemyCount)
	})
	if err != nil {
		return nil, err
	}
	baseline := calc.EstimateSuperBreakDamage(scenario, nil)
	withModifiers := calc.EstimateSuperBreakDamage(scenario, collected.Modifiers)
	estimate := newMechanicEstimate("super_break_damage", attacker, collected, options)
	estimate.AttackTag = "super_break"
	estimate.ElementKey = scenario.ElementKey
	estimate.EnemyCount = scenario.EnemyCount
	estimate.Baseline = baseline
	estimate.WithModifiers = withModifiers
	estimate.Assumptions = append(estimate.Assumptions,
		"超击破伤害按等级倍率、削韧值/10、超击破基础倍率、击破特攻、击破/超击破伤害加成、减防、抗性、易伤和减伤乘区估算。",
		"默认削韧值为30; 未传 super_break_base_multiplier/super_break_multiplier 时底层公式按 1.0 处理。",
	)
	if hasSuperBreakBaseMultiplier(collected.Applied) {
		estimate.Assumptions = append(estimate.Assumptions, "\u8f6c\u5316\u4e3a N% \u8d85\u51fb\u7834\u4f24\u5bb3\u7684\u6548\u679c\u4f5c\u4e3a super_break_base_multiplier \u8fdb\u5165\u57fa\u7840\u500d\u7387\u4e58\u533a,\u4e0d\u518d\u5f52\u5165\u8d85\u51fb\u7834\u589e\u4f24\u533a\u3002")
	}
	setMechanicRatio(estimate, baseline.TotalDamage, withModifiers.TotalDamage)
	return estimate, nil
}

func (s *Service) EstimateHealing(ctx context.Context, charID int, supportIDs []int, scalingStat string, scenario calc.SustainScenario, options ModifierOptions) (*MechanicEstimate, error) {
	return s.estimateSustain(ctx, "healing", charID, supportIDs, scalingStat, scenario, options, calc.EstimateHealing)
}

func (s *Service) EstimateShield(ctx context.Context, charID int, supportIDs []int, scalingStat string, scenario calc.SustainScenario, options ModifierOptions) (*MechanicEstimate, error) {
	return s.estimateSustain(ctx, "shield", charID, supportIDs, scalingStat, scenario, options, calc.EstimateShield)
}

func (s *Service) EstimateUptime(ctx context.Context, scenario calc.UptimeScenario) (calc.UptimeBreakdown, error) {
	_ = ctx
	if scenario.DurationTurns < 0 || scenario.CooldownTurns < 0 || scenario.CycleTurns < 0 || scenario.StartDelayTurns < 0 {
		return calc.UptimeBreakdown{}, fmt.Errorf("uptime turn values must be non-negative")
	}
	return calc.EstimateUptime(scenario), nil
}

type collectedModifiers struct {
	Supports      []CharacterRef
	Modifiers     []calc.Modifier
	Applied       []ModifierBrief
	Skipped       []ModifierBrief
	AppliedBySide ModifierGroups
	SkippedBySide ModifierGroups
	nonStacking   map[string]int
}

func (s *Service) estimateSustain(
	ctx context.Context,
	mechanic string,
	charID int,
	supportIDs []int,
	scalingStat string,
	scenario calc.SustainScenario,
	options ModifierOptions,
	estimateFunc func(calc.SustainScenario, []calc.Modifier) calc.SustainBreakdown,
) (*MechanicEstimate, error) {
	if charID == 0 {
		return nil, fmt.Errorf("char_id is required")
	}
	options = options.Normalized()
	subject, err := s.GetCharacter(ctx, fmt.Sprint(charID))
	if err != nil {
		return nil, err
	}
	if scalingStat == "" {
		axes, err := s.axisRows(ctx, `
SELECT ca.char_id, c.name_zh, ca.kind, ca.stat, coalesce(ca.target, ''), ca.value::float8, coalesce(ca.uptime, ''), coalesce(ca.condition, '')
FROM character_axes ca
JOIN characters c ON c.id = ca.char_id
WHERE ca.char_id = $1 AND ca.kind IN ('needs', 'tag')
ORDER BY ca.kind, ca.stat`, charID)
		if err != nil {
			return nil, err
		}
		scalingStat = inferScalingStat(axes)
	}
	scalingStat = normalizeScalingStat(scalingStat)
	scenario = normalizeSustainScenario(scenario)
	collected, err := s.collectModifiers(ctx, subject, subject.Element, supportIDs, "", options, modifierTargetsAttacker, func(row ModifierRow) (calc.Modifier, bool) {
		return toSustainCalcModifier(row, scalingStat)
	})
	if err != nil {
		return nil, err
	}
	baseline := estimateFunc(scenario, nil)
	withModifiers := estimateFunc(scenario, collected.Modifiers)
	estimate := newMechanicEstimate(mechanic, subject, collected, options)
	estimate.ScalingStat = scalingStat
	estimate.Baseline = baseline
	estimate.WithModifiers = withModifiers
	estimate.Assumptions = append(estimate.Assumptions,
		"治疗/护盾估算使用显式传入的基础属性、技能倍率和固定值; 目前不导入真实面板、遗器或光锥。",
		"默认基础属性1000、倍率100%、固定值0; 相关自增益或队友 modifier 会按星魂开关过滤后叠加。",
	)
	setMechanicRatio(estimate, baseline.TotalValue, withModifiers.TotalValue)
	return estimate, nil
}

func (s *Service) collectModifiers(
	ctx context.Context,
	subject *Character,
	targetElement string,
	supportIDs []int,
	attackTag string,
	options ModifierOptions,
	targetFilter func(ModifierRow, bool, string) bool,
	convert func(ModifierRow) (calc.Modifier, bool),
) (collectedModifiers, error) {
	var out collectedModifiers
	for _, supportID := range uniqueInts(supportIDs) {
		support, err := s.GetCharacter(ctx, fmt.Sprint(supportID))
		if err != nil {
			return out, err
		}
		out.Supports = append(out.Supports, characterRef(support))
		rows, err := s.ListCharacterModifiers(ctx, supportID, "", "", 160)
		if err != nil {
			return out, err
		}
		for _, row := range rows {
			if !options.Allows(row) {
				continue
			}
			if !targetFilter(row, supportID == subject.ID, targetElement) {
				continue
			}
			if !modifierRelevantForAttack(row, attackTag) {
				continue
			}
			if ok, reason := options.ContextAllows(row); !ok {
				if isPotentiallyUseful(row) {
					out.Skipped = append(out.Skipped, modifierBriefWithSkipReason(row, reason))
				}
				continue
			}
			row, _ = options.ResolveSourceStatDependency(row)
			modifier, ok := convert(row)
			if !ok {
				if isPotentiallyUseful(row) {
					out.Skipped = append(out.Skipped, modifierBrief(row))
				}
				continue
			}
			out.addApplied(row, modifier)
		}
	}
	out.Applied = limitModifierBriefs(out.Applied, 40)
	out.Skipped = limitModifierBriefs(out.Skipped, 28)
	out.AppliedBySide = groupModifierBriefsBySide(out.Applied)
	out.SkippedBySide = groupModifierBriefsBySide(out.Skipped)
	return out, nil
}

func (out *collectedModifiers) addApplied(row ModifierRow, modifier calc.Modifier) {
	brief := modifierBriefForApplied(row, modifier)
	key, ok := nonStackingKey(row, modifier)
	if !ok {
		out.Modifiers = append(out.Modifiers, modifier)
		out.Applied = append(out.Applied, brief)
		return
	}
	if out.nonStacking == nil {
		out.nonStacking = map[string]int{}
	}
	if existingIndex, exists := out.nonStacking[key]; exists {
		existing := out.Applied[existingIndex]
		if nonStackingPriority(brief) > nonStackingPriority(existing) {
			existing.SkipReason = "non_stacking_replaced_by:" + fmt.Sprint(row.ModifierID)
			out.Skipped = append(out.Skipped, existing)
			out.Modifiers[existingIndex] = modifier
			out.Applied[existingIndex] = brief
			return
		}
		brief.SkipReason = "non_stacking_duplicate_of:" + fmt.Sprint(existing.ModifierID)
		out.Skipped = append(out.Skipped, brief)
		return
	}
	out.nonStacking[key] = len(out.Applied)
	out.Modifiers = append(out.Modifiers, modifier)
	out.Applied = append(out.Applied, brief)
}

func newMechanicEstimate(mechanic string, subject *Character, collected collectedModifiers, options ModifierOptions) *MechanicEstimate {
	return &MechanicEstimate{
		Mechanic:         mechanic,
		Subject:          characterRef(subject),
		Supports:         collected.Supports,
		Eidolons:         options.Eidolons,
		ActiveContexts:   options.ActiveContextList(),
		InactiveContexts: options.InactiveContextList(),
		Applied:          collected.Applied,
		Skipped:          collected.Skipped,
		AppliedBySide:    collected.AppliedBySide,
		SkippedBySide:    collected.SkippedBySide,
		Assumptions:      []string{options.AssumptionText(), options.ContextAssumptionText(), options.SourcePanelAssumptionText()},
		Caveats: []string{
			"这是局部乘区估算,不等于完整行动轴或实战总伤/总奶/总盾。",
			"reviewed=false 的 modifier 仍可能需要人工复核。",
		},
	}
}

func setMechanicRatio(estimate *MechanicEstimate, baseline float64, withModifiers float64) {
	if baseline > 0 {
		estimate.TotalMultiplier = withModifiers / baseline
		estimate.GainPct = estimate.TotalMultiplier - 1
	}
}

func defaultDamageScenario(attacker *Character, attackTag string) calc.Scenario {
	return calc.Scenario{
		AttackerLevel:     80,
		EnemyLevel:        80,
		BaseScalingStat:   1000,
		AbilityMultiplier: 1,
		CritRate:          0.5,
		CritDamage:        1.0,
		Resistance:        0.2,
		EnemyBroken:       true,
		AttackTag:         strings.TrimSpace(attackTag),
		ElementKey:        strings.ToLower(attacker.Element),
	}
}

func normalizeBreakScenario(attacker *Character, scenario calc.BreakScenario) calc.BreakScenario {
	if scenario.AttackerLevel <= 0 {
		scenario.AttackerLevel = 80
	}
	if scenario.EnemyLevel <= 0 {
		scenario.EnemyLevel = 80
	}
	if strings.TrimSpace(scenario.ElementKey) == "" {
		scenario.ElementKey = attacker.Element
	}
	scenario.ElementKey = strings.ToLower(strings.TrimSpace(scenario.ElementKey))
	if scenario.MaxToughness <= 0 {
		scenario.MaxToughness = 90
	}
	if scenario.EnemyCount <= 0 {
		scenario.EnemyCount = 1
	}
	return scenario
}

func normalizeSustainScenario(scenario calc.SustainScenario) calc.SustainScenario {
	if scenario.BaseScalingStat == 0 && scenario.ScalingStat == 0 {
		scenario.BaseScalingStat = 1000
	}
	if scenario.AbilityMultiplier == 0 {
		scenario.AbilityMultiplier = 1
	}
	return scenario
}

func defaultSupportIDs(primaryID int, supportIDs []int) []int {
	if len(supportIDs) == 0 {
		return []int{primaryID}
	}
	return supportIDs
}

func uniqueInts(values []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func limitModifierBriefs(items []ModifierBrief, limit int) []ModifierBrief {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

type scanner func(dest ...any) error

func scanModifier(scan scanner) (ModifierRow, error) {
	var item ModifierRow
	var conditionJSON []byte
	var sourceStatDependency []byte
	err := scan(
		&item.CharacterID, &item.CharacterNameZH, &item.SourceID, &item.SourceKind,
		&item.SourceKey, &item.SourceNameZH, &item.ModifierID, &item.TargetScope,
		&item.StatKey, &item.Value, &item.ValueUnit, &item.ModifierZone,
		&item.AttackTag, &item.ElementKey, &item.TargetPath, &item.ConditionText,
		&conditionJSON, &sourceStatDependency, &item.DurationKey, &item.StackRule, &item.Confidence, &item.Reviewed,
	)
	if err != nil {
		return item, err
	}
	item.ConditionJSON = json.RawMessage(conditionJSON)
	item.SourceStatDependency = json.RawMessage(sourceStatDependency)
	item.EffectSide = inferEffectSide(item)
	return item, nil
}

func modifierBrief(row ModifierRow) ModifierBrief {
	effectSide := row.EffectSide
	if effectSide == "" {
		effectSide = inferEffectSide(row)
	}
	return ModifierBrief{
		ModifierID:           row.ModifierID,
		SourceKind:           row.SourceKind,
		SourceKey:            row.SourceKey,
		SourceNameZH:         row.SourceNameZH,
		StatKey:              row.StatKey,
		Value:                row.Value,
		ValueUnit:            row.ValueUnit,
		ModifierZone:         row.ModifierZone,
		TargetScope:          row.TargetScope,
		EffectSide:           effectSide,
		ActiveContext:        inferActiveContext(row),
		AttackTag:            row.AttackTag,
		ElementKey:           row.ElementKey,
		ConditionText:        truncateText(row.ConditionText, 120),
		SourceStatDependency: row.SourceStatDependency,
		Reviewed:             row.Reviewed,
		Confidence:           row.Confidence,
	}
}

func modifierBriefForApplied(row ModifierRow, modifier calc.Modifier) ModifierBrief {
	brief := modifierBrief(row)
	if modifier.StatKey != "" {
		brief.StatKey = modifier.StatKey
	}
	value := modifier.Value
	brief.Value = &value
	if modifier.AttackTag != "" {
		brief.AttackTag = modifier.AttackTag
	}
	if modifier.ElementKey != "" {
		brief.ElementKey = modifier.ElementKey
	}
	return brief
}

func modifierBriefWithSkipReason(row ModifierRow, reason string) ModifierBrief {
	brief := modifierBrief(row)
	brief.SkipReason = reason
	return brief
}

func characterRef(c *Character) CharacterRef {
	return CharacterRef{ID: c.ID, NameZH: c.NameZH, Rarity: c.Rarity, Path: c.Path, Element: c.Element, Roles: c.Roles}
}

func splitNeedAxes(rows []AxisRow) (map[string]bool, []string, []string) {
	needSet := map[string]bool{}
	var needs []string
	var tags []string
	for _, row := range rows {
		switch row.Kind {
		case "needs":
			for _, stat := range axisAliases(row.Stat) {
				needSet[stat] = true
			}
			needs = append(needs, row.Stat)
		case "tag":
			tags = append(tags, row.Stat)
		}
	}
	sort.Strings(needs)
	sort.Strings(tags)
	return needSet, uniqueStrings(needs), uniqueStrings(tags)
}

func scoreModifierFit(row ModifierRow, needSet map[string]bool, attackerTags []string) FitModifier {
	score := modifierBaseWeight(row.StatKey)
	matches := []string{}
	for _, alias := range modifierAxisAliases(row.StatKey) {
		if needSet[alias] {
			matches = append(matches, alias)
			score += 8
		}
	}
	if row.Value == nil && score > 0 {
		score *= 0.85
	}
	if containsString(attackerTags, "hp_scaler") && strings.HasPrefix(row.StatKey, "atk_") {
		score *= 0.25
	}
	if containsString(attackerTags, "def_scaler") && (strings.HasPrefix(row.StatKey, "atk_") || strings.HasPrefix(row.StatKey, "hp_")) {
		score *= 0.25
	}
	score *= confidenceFactor(row.Confidence)
	reason := modifierReason(row, matches)
	return FitModifier{Modifier: modifierBrief(row), Score: math.Round(score*10) / 10, AxisMatches: uniqueStrings(matches), Reason: reason}
}

func modifierBaseWeight(statKey string) float64 {
	switch statKey {
	case "crit_rate", "crit_dmg":
		return 12
	case "dmg_bonus", "element_dmg_bonus", "basic_dmg_bonus", "skill_dmg_bonus", "ult_dmg_bonus", "fua_dmg_bonus", "dot_dmg_bonus":
		return 11
	case "def_ignore", "def_shred", "res_pen", "res_reduction", "vulnerability":
		return 11
	case "break_effect", "weakness_break_efficiency", "break_dmg_bonus", "super_break_dmg_bonus", "super_break_base_multiplier", "toughness_ignore", "weakness_implant":
		return 10
	case "action_advance", "extra_action":
		return 9
	case "speed_pct", "speed_flat":
		return 8
	case "atk_pct", "atk_flat", "hp_pct", "hp_flat", "def_pct", "def_flat", "atk_flat_scaling_from_self_atk":
		return 7
	case "sp_recovery", "sp_generation", "max_sp", "energy_restore", "energy_regen":
		return 6
	case "healing_received", "outgoing_heal", "shield_strength", "cleanse", "effect_res":
		return 5
	default:
		return 1
	}
}

func confidenceFactor(confidence float64) float64 {
	if confidence <= 0 {
		return 0.7
	}
	if confidence > 1 {
		return 1
	}
	return 0.7 + confidence*0.3
}

func modifierReason(row ModifierRow, matches []string) string {
	pieces := []string{fmt.Sprintf("%s/%s 提供 %s", row.SourceKind, row.SourceNameZH, row.StatKey)}
	if row.Value != nil {
		pieces = append(pieces, fmt.Sprintf("数值 %.4g %s", *row.Value, row.ValueUnit))
	} else {
		pieces = append(pieces, "数值需要运行时条件或施放者面板")
	}
	if len(matches) > 0 {
		pieces = append(pieces, "命中需求轴 "+strings.Join(uniqueStrings(matches), ", "))
	}
	if row.ConditionText != "" {
		pieces = append(pieces, "条件: "+truncateText(row.ConditionText, 80))
	}
	return strings.Join(pieces, "; ")
}

func modifierTargetsAttacker(row ModifierRow, selfSupport bool, attackerElement string) bool {
	if !modifierElementMatches(row, attackerElement) {
		return false
	}
	switch row.TargetScope {
	case "one_ally", "all_allies", "self_and_allies", "field", "summon", "memosprite":
		return true
	case "self":
		return selfSupport
	default:
		return false
	}
}

func modifierAffectsDamageSubject(row ModifierRow, selfSupport bool, attackerElement string) bool {
	if !modifierElementMatches(row, attackerElement) {
		return false
	}
	if isEnemyDebuff(row) {
		return true
	}
	return modifierTargetsAttacker(row, selfSupport, attackerElement)
}

func modifierElementMatches(row ModifierRow, attackerElement string) bool {
	elementKey := strings.TrimSpace(row.ElementKey)
	if elementKey == "" || strings.EqualFold(elementKey, "any") {
		return true
	}
	return strings.EqualFold(elementKey, strings.TrimSpace(attackerElement))
}

func modifierRelevantForAttack(row ModifierRow, attackTag string) bool {
	attackTag = strings.ToLower(strings.TrimSpace(attackTag))
	targetTag := targetAttackTag(row)
	if targetTag == "" || targetTag == "any" || attackTag == "" {
		return true
	}
	if attackTag == "super_break" && targetTag == "break" {
		return true
	}
	return targetTag == attackTag
}

func targetAttackTag(row ModifierRow) string {
	if tag := strings.ToLower(strings.TrimSpace(row.AttackTag)); tag != "" {
		return tag
	}
	switch row.StatKey {
	case "basic_dmg_bonus":
		return "basic"
	case "skill_dmg_bonus":
		return "skill"
	case "ult_dmg_bonus":
		return "ult"
	case "fua_dmg_bonus":
		return "fua"
	case "dot_dmg_bonus":
		return "dot"
	case "break_dmg_bonus":
		return "break"
	case "super_break_dmg_bonus", "super_break_base_multiplier":
		return "super_break"
	default:
		return ""
	}
}

func defaultActiveContextSet() map[string]bool {
	return map[string]bool{
		"passive":      true,
		"field_active": true,
		"skill_active": true,
		"ult_active":   true,
		"conditional":  true,
		"on_attack":    true,
	}
}

func normalizeContextList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeContextKey(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeContextKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	switch value {
	case "skill", "skill_buff", "skill_active":
		return "skill_active"
	case "ult", "ultimate", "ultimate_active", "ult_active":
		return "ult_active"
	case "talent", "passive":
		return "passive"
	case "field", "field_active":
		return "field_active"
	case "tech", "technique":
		return "technique"
	case "combat_start", "battle_start", "start":
		return "combat_start"
	case "wave_start", "on_wave_start":
		return "on_wave_start"
	case "break", "on_break":
		return "on_break"
	case "attack", "on_attack":
		return "on_attack"
	case "instant":
		return "instant"
	case "conditional", "condition":
		return "conditional"
	default:
		return value
	}
}

func inferActiveContext(row ModifierRow) string {
	sourceKind := strings.ToLower(strings.TrimSpace(row.SourceKind))
	durationKey := strings.ToLower(strings.TrimSpace(row.DurationKey))
	conditionText := row.ConditionText
	attackTag := targetAttackTag(row)
	if sourceKind == "technique" {
		return "technique"
	}
	if containsAny(conditionText, "每个波次", "波次开始") {
		return "on_wave_start"
	}
	if containsAny(conditionText, "战斗开始", "进入战斗后") {
		return "combat_start"
	}
	if containsAny(conditionText, "弱点被击破", "造成弱点击破", "击破后", "on_break") {
		return "on_break"
	}
	if containsAny(conditionText, "持有【狐祈】", "持有[狐祈]", "狐祈") {
		return "skill_active"
	}
	if durationKey == "skill_active" {
		return "skill_active"
	}
	if durationKey == "ult_active" || attackTag == "ult" && durationKey != "instant" {
		return "ult_active"
	}
	switch sourceKind {
	case "skill":
		return "skill_active"
	case "ult", "ultimate":
		if durationKey == "instant" {
			return "instant"
		}
		return "ult_active"
	case "talent":
		if containsAny(conditionText, "在场时") {
			return "field_active"
		}
		return "passive"
	case "trace":
		if durationKey == "passive" {
			return "passive"
		}
	case "eidolon":
		if attackTag == "ult" {
			return "ult_active"
		}
	}
	switch durationKey {
	case "passive":
		return "passive"
	case "field_active":
		return "field_active"
	case "fixed_turns":
		return "conditional"
	case "instant":
		return "instant"
	case "":
		return "conditional"
	default:
		return normalizeContextKey(durationKey)
	}
}

func inferEffectSide(row ModifierRow) string {
	if isEnemyDebuff(row) {
		return "enemy_debuff"
	}
	if isFieldEffect(row) {
		return "field_effect"
	}
	if isUtilityEffect(row) {
		return "utility"
	}
	return "ally_buff"
}

func isEnemyDebuff(row ModifierRow) bool {
	if isEnemyTargetScope(row.TargetScope) {
		return true
	}
	switch row.StatKey {
	case "def_shred", "res_reduction", "vulnerability", "action_delay", "weakness_implant", "debuff_apply", "debuff_extend", "debuff_resist":
		return true
	default:
		return false
	}
}

func isEnemyTargetScope(scope string) bool {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "one_enemy", "all_enemies", "enemy", "enemies", "field_enemy", "enemy_field":
		return true
	default:
		return false
	}
}

func isFieldEffect(row ModifierRow) bool {
	scope := strings.ToLower(strings.TrimSpace(row.TargetScope))
	return scope == "field" || scope == "ally_field"
}

func isUtilityEffect(row ModifierRow) bool {
	if strings.EqualFold(row.ModifierZone, "utility") {
		return true
	}
	switch row.StatKey {
	case "action_advance", "extra_action", "sp_recovery", "sp_generation", "sp_consumption", "max_sp",
		"energy_restore", "energy_regen", "cleanse", "revive", "effect_res", "buff_extend",
		"fua_trigger", "dot_trigger", "speed_pct", "speed_flat":
		return true
	default:
		return false
	}
}

func groupModifierBriefsBySide(items []ModifierBrief) ModifierGroups {
	if len(items) == 0 {
		return nil
	}
	groups := ModifierGroups{}
	for _, item := range items {
		side := item.EffectSide
		if side == "" {
			side = "ally_buff"
		}
		groups[side] = append(groups[side], item)
	}
	return groups
}

func nonStackingKey(row ModifierRow, modifier calc.Modifier) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(row.StackRule), "none") || row.Value == nil {
		return "", false
	}
	statKey := strings.ToLower(strings.TrimSpace(modifier.StatKey))
	if statKey == "" || statKey == "unknown" {
		return "", false
	}
	value := modifier.Value
	effectSide := row.EffectSide
	if effectSide == "" {
		effectSide = inferEffectSide(row)
	}
	attackTag := modifier.AttackTag
	if attackTag == "" {
		attackTag = targetAttackTag(row)
	}
	elementKey := modifier.ElementKey
	if elementKey == "" {
		elementKey = row.ElementKey
	}
	return strings.Join([]string{
		fmt.Sprint(row.CharacterID),
		statKey,
		strings.ToLower(strings.TrimSpace(row.ModifierZone)),
		effectSide,
		attackTag,
		strings.ToLower(strings.TrimSpace(elementKey)),
		fmt.Sprintf("%.8g", value),
		strings.ToLower(strings.TrimSpace(row.ValueUnit)),
	}, "|"), true
}

func nonStackingPriority(brief ModifierBrief) int {
	switch normalizeContextKey(brief.ActiveContext) {
	case "skill_active":
		return 90
	case "ult_active":
		return 80
	case "field_active", "passive":
		return 70
	case "on_break", "on_attack":
		return 60
	case "conditional":
		return 50
	case "combat_start", "on_wave_start":
		return 40
	case "technique":
		return 30
	case "instant":
		return 10
	default:
		return 20
	}
}

func modifierAxisAliases(statKey string) []string {
	switch statKey {
	case "atk_pct":
		return []string{"atk_percent", "atk_pct"}
	case "hp_pct":
		return []string{"hp_percent", "hp_pct"}
	case "def_pct":
		return []string{"def_percent", "def_pct"}
	case "break_effect":
		return []string{"break_eff", "break_effect"}
	case "break_dmg_bonus", "super_break_dmg_bonus", "super_break_base_multiplier":
		return []string{"break", "super_break", statKey}
	case "dmg_bonus", "element_dmg_bonus":
		return []string{"dmg_percent", "dmg_bonus"}
	case "basic_dmg_bonus":
		return []string{"basic_dmg", "dmg_percent"}
	case "skill_dmg_bonus":
		return []string{"skill_dmg", "dmg_percent"}
	case "ult_dmg_bonus":
		return []string{"ult_dmg", "dmg_percent"}
	case "fua_dmg_bonus":
		return []string{"fua_dmg", "dmg_percent"}
	case "action_advance":
		return []string{"turn_advance", "action_advance"}
	default:
		return []string{statKey}
	}
}

func axisAliases(stat string) []string {
	switch stat {
	case "atk_percent":
		return []string{"atk_percent", "atk_pct"}
	case "hp_percent":
		return []string{"hp_percent", "hp_pct"}
	case "def_percent":
		return []string{"def_percent", "def_pct"}
	case "break_eff":
		return []string{"break_eff", "break_effect"}
	case "dmg_percent":
		return []string{"dmg_percent", "dmg_bonus", "element_dmg_bonus"}
	case "turn_advance":
		return []string{"turn_advance", "action_advance"}
	default:
		return []string{stat}
	}
}

func fitCaveats(attacker *Character, support *Character, tags []string, result *FitResult) []string {
	tagSet := make(map[string]bool, len(tags))
	for _, tag := range tags {
		tagSet[tag] = true
	}
	var caveats []string
	if tagSet["hp_scaler"] && (hasStat(result.UsefulEffects, "atk_pct", "atk_flat", "atk_flat_scaling_from_self_atk") || hasStat(result.LowValueEffects, "atk_pct", "atk_flat", "atk_flat_scaling_from_self_atk")) {
		caveats = append(caveats, fmt.Sprintf("%s 带 hp_scaler 标签,攻击力类加成不应按满收益理解。", attacker.NameZH))
	}
	if tagSet["break_scaler"] || tagSet["super_break_team"] {
		if !hasStat(result.UsefulEffects, "break_effect", "weakness_break_efficiency", "break_dmg_bonus", "super_break_dmg_bonus", "super_break_base_multiplier", "res_pen", "def_ignore", "def_shred") {
			caveats = append(caveats, fmt.Sprintf("%s 偏击破/超击破,当前 %s modifiers 未明显覆盖击破效率或击破乘区。", attacker.NameZH, support.NameZH))
		}
	}
	if len(result.UsefulEffects) == 0 {
		caveats = append(caveats, "没有找到可直接作用于攻击者的高价值 modifiers;可能需要改查共现队伍或人工确认特殊机制。")
	}
	return caveats
}

func hasStat(items []FitModifier, stats ...string) bool {
	wanted := map[string]bool{}
	for _, stat := range stats {
		wanted[stat] = true
	}
	for _, item := range items {
		if wanted[item.Modifier.StatKey] {
			return true
		}
	}
	return false
}

func fitRating(score float64) string {
	switch {
	case score >= 70:
		return "strong"
	case score >= 45:
		return "good"
	case score >= 25:
		return "situational"
	default:
		return "weak"
	}
}

func sortFitModifiers(items []FitModifier) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Modifier.ModifierID < items[j].Modifier.ModifierID
		}
		return items[i].Score > items[j].Score
	})
}

func inferScalingStat(rows []AxisRow) string {
	for _, row := range rows {
		if row.Kind == "tag" && row.Stat == "hp_scaler" {
			return "hp"
		}
	}
	for _, row := range rows {
		if row.Kind == "tag" && row.Stat == "def_scaler" {
			return "def"
		}
	}
	for _, row := range rows {
		if row.Kind == "needs" {
			switch row.Stat {
			case "hp_percent", "hp_pct", "hp_flat":
				return "hp"
			case "def_percent", "def_pct", "def_flat":
				return "def"
			}
		}
	}
	return "atk"
}

func toCalcModifier(row ModifierRow, scalingStat string, attackTag string) (calc.Modifier, bool) {
	if row.Value == nil {
		return calc.Modifier{}, false
	}
	statKey := row.StatKey
	if !baseStatMatchesScaling(statKey, scalingStat) {
		return calc.Modifier{}, false
	}
	if !calcSupportsStat(statKey) {
		return calc.Modifier{}, false
	}
	return calc.Modifier{
		StatKey:      statKey,
		Value:        *row.Value,
		ModifierZone: row.ModifierZone,
		AttackTag:    targetAttackTag(row),
		ElementKey:   row.ElementKey,
		Condition:    row.ConditionText,
	}, true
}

func toBreakCalcModifier(row ModifierRow, superBreak bool, enemyCount int) (calc.Modifier, bool) {
	if row.Value == nil {
		return calc.Modifier{}, false
	}
	if !modifierMatchesEnemyCount(row, enemyCount) {
		return calc.Modifier{}, false
	}
	statKey := row.StatKey
	value := *row.Value
	if row.StatKey == "super_break_dmg_bonus" && isSuperBreakConversion(row) {
		statKey = "super_break_base_multiplier"
	}
	if !breakCalcSupportsStat(statKey, superBreak) {
		return calc.Modifier{}, false
	}
	return calc.Modifier{
		StatKey:      statKey,
		Value:        value,
		ModifierZone: row.ModifierZone,
		AttackTag:    targetAttackTag(row),
		ElementKey:   row.ElementKey,
		Condition:    row.ConditionText,
	}, true
}

func isSuperBreakConversion(row ModifierRow) bool {
	text := strings.ToLower(row.ConditionText)
	return (strings.Contains(text, "\u8f6c\u5316\u4e3a") || strings.Contains(text, "convert")) &&
		(strings.Contains(text, "\u8d85\u51fb\u7834\u4f24\u5bb3") || strings.Contains(text, "super break"))
}

func hasSuperBreakBaseMultiplier(modifiers []ModifierBrief) bool {
	for _, modifier := range modifiers {
		if modifier.StatKey == "super_break_base_multiplier" {
			return true
		}
	}
	return false
}

func modifierMatchesEnemyCount(row ModifierRow, enemyCount int) bool {
	if len(row.ConditionJSON) == 0 {
		return true
	}
	var condition map[string]any
	if err := json.Unmarshal(row.ConditionJSON, &condition); err != nil {
		return true
	}
	raw, ok := condition["enemy_count"]
	if !ok {
		return true
	}
	if enemyCount <= 0 {
		enemyCount = 1
	}
	switch value := raw.(type) {
	case float64:
		return enemyCount == int(value)
	case string:
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "5_or_more" || value == ">=5" || value == "5+" {
			return enemyCount >= 5
		}
		parsed, err := ParseInt(value)
		if err != nil {
			return false
		}
		return enemyCount == parsed
	default:
		return false
	}
}

func toSustainCalcModifier(row ModifierRow, scalingStat string) (calc.Modifier, bool) {
	if row.Value == nil {
		return calc.Modifier{}, false
	}
	statKey := row.StatKey
	if statKey == "shield_strength" && !isShieldStrengthBuff(row) {
		return calc.Modifier{}, false
	}
	if !baseStatMatchesScaling(statKey, scalingStat) {
		return calc.Modifier{}, false
	}
	if !sustainCalcSupportsStat(statKey) {
		return calc.Modifier{}, false
	}
	return calc.Modifier{
		StatKey:      statKey,
		Value:        *row.Value,
		ModifierZone: row.ModifierZone,
		Condition:    row.ConditionText,
	}, true
}

func normalizeScalingStat(scalingStat string) string {
	switch strings.ToLower(strings.TrimSpace(scalingStat)) {
	case "hp", "hp_pct", "hp_percent":
		return "hp"
	case "def", "def_pct", "def_percent":
		return "def"
	default:
		return "atk"
	}
}

func baseStatMatchesScaling(statKey string, scalingStat string) bool {
	scalingStat = normalizeScalingStat(scalingStat)
	switch statKey {
	case "atk_pct", "atk_flat":
		return scalingStat == "atk"
	case "hp_pct", "hp_flat":
		return scalingStat == "hp"
	case "def_pct", "def_flat":
		return scalingStat == "def"
	default:
		return true
	}
}

func calcSupportsStat(statKey string) bool {
	switch statKey {
	case "atk_pct", "atk_flat", "hp_pct", "hp_flat", "def_pct", "def_flat",
		"crit_rate", "crit_dmg", "dmg_bonus", "element_dmg_bonus", "basic_dmg_bonus",
		"skill_dmg_bonus", "ult_dmg_bonus", "fua_dmg_bonus", "dot_dmg_bonus",
		"break_dmg_bonus", "super_break_dmg_bonus", "super_break_base_multiplier", "additional_dmg", "def_ignore", "def_shred", "res_pen", "res_reduction",
		"vulnerability", "dmg_reduction", "action_advance", "action_delay",
		"sp_recovery", "sp_generation", "sp_consumption", "max_sp", "energy_restore",
		"energy_regen", "toughness_reduce", "weakness_implant", "weakness_break_efficiency",
		"cleanse", "revive", "speed_pct", "speed_flat", "break_effect", "effect_res",
		"healing_received", "outgoing_heal", "shield_strength", "toughness_ignore",
		"buff_extend", "debuff_extend", "extra_action", "fua_trigger", "dot_trigger",
		"debuff_apply", "debuff_resist":
		return true
	default:
		return false
	}
}

func breakCalcSupportsStat(statKey string, superBreak bool) bool {
	switch statKey {
	case "break_effect", "break_dmg_bonus", "weakness_break_efficiency", "toughness_reduce",
		"def_ignore", "def_shred", "res_pen", "res_reduction", "vulnerability", "dmg_reduction",
		"action_advance", "action_delay", "sp_recovery", "sp_generation", "sp_consumption",
		"max_sp", "energy_restore", "energy_regen", "weakness_implant", "toughness_ignore",
		"buff_extend", "extra_action":
		return true
	case "super_break_dmg_bonus", "super_break_base_multiplier":
		return superBreak
	default:
		return false
	}
}

func sustainCalcSupportsStat(statKey string) bool {
	switch statKey {
	case "atk_pct", "atk_flat", "hp_pct", "hp_flat", "def_pct", "def_flat",
		"outgoing_heal", "healing_received", "shield_strength", "cleanse", "revive",
		"effect_res", "dmg_reduction", "action_advance", "sp_recovery", "energy_restore":
		return true
	default:
		return false
	}
}

func isShieldStrengthBuff(row ModifierRow) bool {
	if row.ValueUnit != "percent" && row.ValueUnit != "ratio" {
		return false
	}
	text := row.SourceNameZH + row.ConditionText
	if strings.Contains(text, "护盾量提高") || strings.Contains(text, "护盾强效") {
		return true
	}
	if strings.Contains(text, "提供") || strings.Contains(text, "固定值") || strings.Contains(text, "防御力+") || strings.Contains(text, "等同于") || strings.Contains(text, "叠加") {
		return false
	}
	return false
}

func isPotentiallyUseful(row ModifierRow) bool {
	return modifierBaseWeight(row.StatKey) >= 5
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func truncateText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
