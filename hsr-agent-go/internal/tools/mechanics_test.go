package tools

import (
	"math"
	"testing"

	"hsr-agent-go/internal/calc"
)

func TestInferScalingStatPrefersHPScalerTag(t *testing.T) {
	got := inferScalingStat([]AxisRow{
		{Kind: "needs", Stat: "atk_percent"},
		{Kind: "tag", Stat: "hp_scaler"},
	})
	if got != "hp" {
		t.Fatalf("got %q want hp", got)
	}
}

func TestModifierTargetsAttackerHonorsElementCondition(t *testing.T) {
	row := ModifierRow{TargetScope: "all_allies", ElementKey: "quantum"}
	if modifierTargetsAttacker(row, false, "Wind") {
		t.Fatalf("quantum-only modifier should not target Wind attacker")
	}
	if !modifierTargetsAttacker(row, false, "Quantum") {
		t.Fatalf("quantum-only modifier should target Quantum attacker")
	}
}

func TestModifierAffectsDamageSubjectAcceptsEnemyDebuffs(t *testing.T) {
	row := ModifierRow{TargetScope: "one_enemy", StatKey: "def_shred", Value: floatPtr(0.23), ElementKey: "fire"}
	if modifierTargetsAttacker(row, false, "Fire") {
		t.Fatalf("enemy debuff should not be treated as an ally-targeting buff")
	}
	if !modifierAffectsDamageSubject(row, false, "Fire") {
		t.Fatalf("enemy debuff should affect the damage subject")
	}
	if modifierAffectsDamageSubject(row, false, "Quantum") {
		t.Fatalf("element-specific enemy debuff should still honor element conditions")
	}
}

func TestInferEffectSideAndGrouping(t *testing.T) {
	enemy := modifierBrief(ModifierRow{ModifierID: 1, TargetScope: "all_enemies", StatKey: "res_reduction"})
	field := modifierBrief(ModifierRow{ModifierID: 2, TargetScope: "field", StatKey: "weakness_break_efficiency"})
	utility := modifierBrief(ModifierRow{ModifierID: 3, TargetScope: "all_allies", StatKey: "sp_recovery"})
	ally := modifierBrief(ModifierRow{ModifierID: 4, TargetScope: "all_allies", StatKey: "crit_dmg"})
	if enemy.EffectSide != "enemy_debuff" || field.EffectSide != "field_effect" || utility.EffectSide != "utility" || ally.EffectSide != "ally_buff" {
		t.Fatalf("unexpected effect sides: %q %q %q %q", enemy.EffectSide, field.EffectSide, utility.EffectSide, ally.EffectSide)
	}
	groups := groupModifierBriefsBySide([]ModifierBrief{enemy, field, utility, ally})
	if len(groups["enemy_debuff"]) != 1 || len(groups["field_effect"]) != 1 || len(groups["utility"]) != 1 || len(groups["ally_buff"]) != 1 {
		t.Fatalf("unexpected modifier groups: %#v", groups)
	}
}

func TestModifierRelevantForAttackUsesExplicitAttackTag(t *testing.T) {
	row := ModifierRow{StatKey: "dmg_bonus", AttackTag: "Skill"}
	if !modifierRelevantForAttack(row, "skill") {
		t.Fatalf("explicit attack_tag should match case-insensitively")
	}
	if modifierRelevantForAttack(row, "ult") {
		t.Fatalf("explicit attack_tag should filter non-matching attacks")
	}
}

func TestModifierRelevantForAttackAllowsBreakBonusesForSuperBreak(t *testing.T) {
	row := ModifierRow{StatKey: "break_dmg_bonus"}
	if !modifierRelevantForAttack(row, "super_break") {
		t.Fatalf("break damage bonuses should also be relevant for super break estimates")
	}
	superOnly := ModifierRow{StatKey: "super_break_dmg_bonus"}
	if modifierRelevantForAttack(superOnly, "break") {
		t.Fatalf("super break-only bonuses should not apply to regular break estimates")
	}
	baseMultiplier := ModifierRow{StatKey: "super_break_base_multiplier"}
	if modifierRelevantForAttack(baseMultiplier, "break") {
		t.Fatalf("super break base multipliers should not apply to regular break estimates")
	}
	if !modifierRelevantForAttack(baseMultiplier, "super_break") {
		t.Fatalf("super break base multipliers should apply to super break estimates")
	}
}

func TestModifierOptionsContextFiltering(t *testing.T) {
	options := NewModifierOptions(false, nil)
	technique := ModifierRow{SourceKind: "technique", DurationKey: "fixed_turns", StatKey: "def_shred"}
	if ok, reason := options.ContextAllows(technique); ok || reason != "inactive_context:technique" {
		t.Fatalf("default context should skip technique, got ok=%v reason=%q", ok, reason)
	}
	skillActive := ModifierRow{SourceKind: "skill", DurationKey: "fixed_turns", ConditionText: "持有【狐祈】的我方目标攻击时"}
	if ok, reason := options.ContextAllows(skillActive); !ok || reason != "" {
		t.Fatalf("default context should allow skill_active, got ok=%v reason=%q", ok, reason)
	}
	onBreak := ModifierRow{SourceKind: "trace", DurationKey: "instant", ConditionText: "敌方目标弱点被击破时触发"}
	if ok, reason := options.ContextAllows(onBreak); ok || reason != "inactive_context:on_break" {
		t.Fatalf("default context should skip on_break, got ok=%v reason=%q", ok, reason)
	}
	withBreak := NewModifierOptionsWithContexts(false, nil, []string{"on_break"}, nil)
	if ok, reason := withBreak.ContextAllows(onBreak); !ok || reason != "" {
		t.Fatalf("active_contexts should enable on_break, got ok=%v reason=%q", ok, reason)
	}
	forcedOff := NewModifierOptionsWithContexts(false, nil, nil, []string{"skill_active"})
	if ok, reason := forcedOff.ContextAllows(skillActive); ok || reason != "inactive_context:skill_active" {
		t.Fatalf("inactive_contexts should force off skill_active, got ok=%v reason=%q", ok, reason)
	}
}

func TestResolveSourceStatDependencyUsesDefaultPanel(t *testing.T) {
	row := ModifierRow{
		CharacterID:          8005,
		StatKey:              "break_effect",
		SourceStatDependency: []byte(`{"source":"caster","stat":"break_effect","ratio":0.15,"flat":0}`),
	}
	resolved, ok := NewModifierOptions(false, nil).ResolveSourceStatDependency(row)
	if !ok {
		t.Fatalf("source stat dependency should resolve")
	}
	if resolved.Value == nil || math.Abs(*resolved.Value-0.27) > 1e-9 {
		t.Fatalf("got %v want 0.27 from default 180%% break effect", resolved.Value)
	}
}

func TestResolveSourceStatDependencyUsesPanelOverride(t *testing.T) {
	critDamage := 2.0
	row := ModifierRow{
		CharacterID:          1306,
		StatKey:              "crit_dmg",
		SourceStatDependency: []byte(`{"source":"caster","stat":"crit_dmg","ratio":0.3,"flat":0.54}`),
	}
	options := NewModifierOptionsWithPanels(false, nil, nil, nil, []SourcePanel{{CharacterID: 1306, CritDamage: &critDamage}})
	resolved, ok := options.ResolveSourceStatDependency(row)
	if !ok {
		t.Fatalf("source stat dependency should resolve")
	}
	if resolved.Value == nil || math.Abs(*resolved.Value-1.14) > 1e-9 {
		t.Fatalf("got %v want 1.14 from 200%% crit damage", resolved.Value)
	}
}

func TestApplyDamageScenarioUsesManualPanels(t *testing.T) {
	base := 3000.0
	critRate := 0.8
	critDamage := 1.5
	resistance := 0.1
	broken := false
	options := NewModifierOptionsWithScene(false, nil, nil, nil, nil, &AttackerPanel{
		Level:           70,
		BaseScalingStat: &base,
		CritRate:        &critRate,
		CritDamage:      &critDamage,
	}, &EnemyState{
		Level:       90,
		Resistance:  &resistance,
		EnemyBroken: &broken,
	})
	got := options.ApplyDamageScenario(defaultDamageScenario(&Character{Element: "Fire"}, "skill"))
	if got.AttackerLevel != 70 || got.EnemyLevel != 90 {
		t.Fatalf("levels were not applied: %#v", got)
	}
	if got.BaseScalingStat != 3000 || got.CritRate != 0.8 || got.CritDamage != 1.5 {
		t.Fatalf("attacker panel was not applied: %#v", got)
	}
	if got.Resistance != 0.1 || got.EnemyBroken {
		t.Fatalf("enemy state was not applied: %#v", got)
	}
}

func TestApplyBreakScenarioUsesManualPanelsAndEnemyState(t *testing.T) {
	breakEffect := 2.4
	toughnessReduction := 45.0
	maxToughness := 120.0
	resistance := 0.05
	options := NewModifierOptionsWithScene(false, nil, nil, nil, nil, &AttackerPanel{
		Level:       75,
		BreakEffect: &breakEffect,
		ElementKey:  "Fire",
	}, &EnemyState{
		Level:              88,
		EnemyCount:         3,
		ToughnessReduction: &toughnessReduction,
		MaxToughness:       &maxToughness,
		Resistance:         &resistance,
	})
	got := options.ApplyBreakScenario(calc.BreakScenario{ElementKey: "wind", EnemyCount: 1, ToughnessReduction: 30, MaxToughness: 90, Resistance: 0.2})
	if got.AttackerLevel != 75 || got.EnemyLevel != 88 || got.ElementKey != "fire" || got.EnemyCount != 3 {
		t.Fatalf("basic break scenario fields were not applied: %#v", got)
	}
	if got.BreakEffect != 2.4 || got.ToughnessReduction != 45 || got.MaxToughness != 120 || got.Resistance != 0.05 {
		t.Fatalf("break panel/enemy values were not applied: %#v", got)
	}
}

func TestCollectedModifiersDedupesNonStackingEffects(t *testing.T) {
	skill := fugueDefShredRow(894, "skill", "fixed_turns", "持有【狐祈】的我方目标施放攻击时")
	technique := fugueDefShredRow(901, "technique", "fixed_turns", "主动攻击晕眩敌人进入战斗后")
	var collected collectedModifiers
	collected.addApplied(skill, calcModifierForTest(skill))
	collected.addApplied(technique, calcModifierForTest(technique))
	if len(collected.Applied) != 1 || collected.Applied[0].ModifierID != skill.ModifierID {
		t.Fatalf("expected skill def shred to remain applied, got %#v", collected.Applied)
	}
	if len(collected.Skipped) != 1 || collected.Skipped[0].SkipReason != "non_stacking_duplicate_of:894" {
		t.Fatalf("expected technique duplicate to be skipped, got %#v", collected.Skipped)
	}
}

func TestCollectedModifiersReplacesLowerPriorityNonStackingEffect(t *testing.T) {
	skill := fugueDefShredRow(894, "skill", "fixed_turns", "持有【狐祈】的我方目标施放攻击时")
	technique := fugueDefShredRow(901, "technique", "fixed_turns", "主动攻击晕眩敌人进入战斗后")
	var collected collectedModifiers
	collected.addApplied(technique, calcModifierForTest(technique))
	collected.addApplied(skill, calcModifierForTest(skill))
	if len(collected.Applied) != 1 || collected.Applied[0].ModifierID != skill.ModifierID {
		t.Fatalf("expected skill def shred to replace technique, got %#v", collected.Applied)
	}
	if len(collected.Skipped) != 1 || collected.Skipped[0].SkipReason != "non_stacking_replaced_by:894" {
		t.Fatalf("expected technique to be marked replaced, got %#v", collected.Skipped)
	}
}

func TestScoreModifierFitDownweightsAttackForHPScaler(t *testing.T) {
	row := ModifierRow{StatKey: "atk_pct", Value: floatPtr(0.15), ValueUnit: "percent", TargetScope: "all_allies", Confidence: 1}
	got := scoreModifierFit(row, map[string]bool{}, []string{"hp_scaler"})
	if got.Score >= modifierBaseWeight("atk_pct") {
		t.Fatalf("expected atk_pct to be downweighted for hp_scaler, got %.2f", got.Score)
	}
}

func TestModifierOptionsAllowSpecificEidolons(t *testing.T) {
	options := NewModifierOptions(false, []int{1, 6})
	if !options.Allows(ModifierRow{SourceKind: "skill"}) {
		t.Fatalf("non-eidolon modifier should be allowed")
	}
	if !options.Allows(ModifierRow{SourceKind: "eidolon", SourceKey: "1"}) {
		t.Fatalf("E1 modifier should be allowed")
	}
	if options.Allows(ModifierRow{SourceKind: "eidolon", SourceKey: "2"}) {
		t.Fatalf("E2 modifier should be filtered out")
	}
	if !NewModifierOptions(true, nil).Allows(ModifierRow{SourceKind: "eidolon", SourceKey: "2"}) {
		t.Fatalf("include_eidolons should allow all eidolons")
	}
}

func TestToBreakCalcModifierRequiresSuperBreakForSuperBonus(t *testing.T) {
	row := ModifierRow{StatKey: "super_break_dmg_bonus", Value: floatPtr(0.4)}
	if _, ok := toBreakCalcModifier(row, false, 1); ok {
		t.Fatalf("super_break_dmg_bonus should not apply to regular break")
	}
	if _, ok := toBreakCalcModifier(row, true, 1); !ok {
		t.Fatalf("super_break_dmg_bonus should apply to super break")
	}
}

func TestToBreakCalcModifierTreatsConversionAsBaseMultiplier(t *testing.T) {
	row := ModifierRow{
		StatKey:       "super_break_dmg_bonus",
		Value:         floatPtr(1.25),
		ConditionText: "\u8f6c\u5316\u4e3a125%\u7684\u8d85\u51fb\u7834\u4f24\u5bb3",
	}
	got, ok := toBreakCalcModifier(row, true, 1)
	if !ok {
		t.Fatalf("conversion modifier should apply to super break")
	}
	if got.StatKey != "super_break_base_multiplier" {
		t.Fatalf("got stat %q want super_break_base_multiplier", got.StatKey)
	}
	if got.Value != 1.25 {
		t.Fatalf("got %.2f want 1.25", got.Value)
	}

	regularBonus := ModifierRow{
		StatKey:       "super_break_dmg_bonus",
		Value:         floatPtr(0.4),
		ConditionText: "\u8d85\u51fb\u7834\u4f24\u5bb3\u63d0\u9ad840%",
	}
	got, ok = toBreakCalcModifier(regularBonus, true, 1)
	if !ok {
		t.Fatalf("regular super break bonus should apply to super break")
	}
	if got.StatKey != "super_break_dmg_bonus" {
		t.Fatalf("got stat %q want super_break_dmg_bonus", got.StatKey)
	}
	if got.Value != 0.4 {
		t.Fatalf("got %.2f want 0.4", got.Value)
	}
}

func TestAppliedBriefUsesConvertedModifierStat(t *testing.T) {
	row := ModifierRow{
		ModifierID:    42,
		StatKey:       "super_break_dmg_bonus",
		Value:         floatPtr(1.25),
		ConditionText: "\u8f6c\u5316\u4e3a125%\u7684\u8d85\u51fb\u7834\u4f24\u5bb3",
	}
	modifier := calc.Modifier{StatKey: "super_break_base_multiplier", Value: 1.25, AttackTag: "super_break"}
	var collected collectedModifiers
	collected.addApplied(row, modifier)

	if len(collected.Applied) != 1 {
		t.Fatalf("got %d applied modifiers want 1", len(collected.Applied))
	}
	if collected.Applied[0].StatKey != "super_break_base_multiplier" {
		t.Fatalf("got stat %q want super_break_base_multiplier", collected.Applied[0].StatKey)
	}
	if collected.Applied[0].Value == nil || *collected.Applied[0].Value != 1.25 {
		t.Fatalf("applied value was not copied from converted modifier")
	}
}

func TestToBreakCalcModifierHonorsEnemyCount(t *testing.T) {
	oneEnemy := ModifierRow{
		StatKey:       "super_break_dmg_bonus",
		Value:         floatPtr(0.6),
		ConditionJSON: []byte(`{"enemy_count":1}`),
	}
	if _, ok := toBreakCalcModifier(oneEnemy, true, 3); ok {
		t.Fatalf("1-enemy modifier should not apply to 3 enemies")
	}
	threeEnemies := ModifierRow{
		StatKey:       "super_break_dmg_bonus",
		Value:         floatPtr(0.4),
		ConditionJSON: []byte(`{"enemy_count":3}`),
	}
	if _, ok := toBreakCalcModifier(threeEnemies, true, 3); !ok {
		t.Fatalf("3-enemy modifier should apply to 3 enemies")
	}
	fiveOrMore := ModifierRow{
		StatKey:       "super_break_dmg_bonus",
		Value:         floatPtr(0.2),
		ConditionJSON: []byte(`{"enemy_count":"5_or_more"}`),
	}
	if _, ok := toBreakCalcModifier(fiveOrMore, true, 5); !ok {
		t.Fatalf("5_or_more modifier should apply to 5 enemies")
	}
}

func TestToSustainCalcModifierFiltersScalingStat(t *testing.T) {
	row := ModifierRow{StatKey: "hp_pct", Value: floatPtr(0.2)}
	if _, ok := toSustainCalcModifier(row, "atk"); ok {
		t.Fatalf("hp_pct should not apply to atk-scaling sustain")
	}
	if _, ok := toSustainCalcModifier(row, "hp"); !ok {
		t.Fatalf("hp_pct should apply to hp-scaling sustain")
	}
}

func TestToSustainCalcModifierSkipsShieldFormulaAsStrengthBuff(t *testing.T) {
	formula := ModifierRow{
		StatKey:       "shield_strength",
		Value:         floatPtr(0.28),
		ValueUnit:     "percent",
		ConditionText: "战技为我方全体提供护盾",
	}
	if _, ok := toSustainCalcModifier(formula, "def"); ok {
		t.Fatalf("shield base formula should not be treated as shield strength")
	}
	buff := ModifierRow{
		StatKey:       "shield_strength",
		Value:         floatPtr(0.2),
		ValueUnit:     "percent",
		ConditionText: "护盾量提高20%",
	}
	if _, ok := toSustainCalcModifier(buff, "def"); !ok {
		t.Fatalf("explicit shield strength buff should apply")
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func fugueDefShredRow(id int64, sourceKind string, durationKey string, conditionText string) ModifierRow {
	return ModifierRow{
		CharacterID:   1225,
		SourceKind:    sourceKind,
		SourceKey:     "test",
		SourceNameZH:  "测试",
		ModifierID:    id,
		TargetScope:   "one_enemy",
		StatKey:       "def_shred",
		Value:         floatPtr(0.23),
		ValueUnit:     "percent",
		ModifierZone:  "def",
		AttackTag:     "any",
		ElementKey:    "any",
		ConditionText: conditionText,
		DurationKey:   durationKey,
		StackRule:     "none",
		Confidence:    1,
	}
}

func calcModifierForTest(row ModifierRow) calc.Modifier {
	return calc.Modifier{StatKey: row.StatKey, Value: *row.Value, ModifierZone: row.ModifierZone}
}
