package tools

import "testing"

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

func TestNormalizeSuperBreakBonusValueTreatsConversionAsBaseMultiplier(t *testing.T) {
	row := ModifierRow{
		StatKey:       "super_break_dmg_bonus",
		ConditionText: "\u8f6c\u5316\u4e3a125%\u7684\u8d85\u51fb\u7834\u4f24\u5bb3",
	}
	got := normalizeSuperBreakBonusValue(row, 1.25)
	if got != 0.25 {
		t.Fatalf("got %.2f want 0.25", got)
	}

	regularBonus := ModifierRow{
		StatKey:       "super_break_dmg_bonus",
		ConditionText: "\u8d85\u51fb\u7834\u4f24\u5bb3\u63d0\u9ad840%",
	}
	got = normalizeSuperBreakBonusValue(regularBonus, 0.4)
	if got != 0.4 {
		t.Fatalf("got %.2f want 0.4", got)
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
