package calc

import (
	"math"
	"testing"
)

func TestEstimateStandardDamageBaseline(t *testing.T) {
	got := EstimateStandardDamage(Scenario{
		AttackerLevel:     80,
		EnemyLevel:        80,
		ScalingStat:       1000,
		AbilityMultiplier: 2,
		CritRate:          0.5,
		CritDamage:        1.0,
		Resistance:        0.2,
		EnemyBroken:       false,
	}, nil)

	assertClose(t, got.BaseDamage, 2000)
	assertClose(t, got.CritMultiplier, 1.5)
	assertClose(t, got.DefenseMultiplier, 0.5)
	assertClose(t, got.ResistanceMultiplier, 0.8)
	assertClose(t, got.ToughnessMultiplier, 0.9)
	assertClose(t, got.TotalDamage, 1080)
}

func TestEstimateStandardDamageWithModifiers(t *testing.T) {
	got := EstimateStandardDamage(Scenario{
		AttackerLevel:     80,
		EnemyLevel:        80,
		BaseScalingStat:   1000,
		AbilityMultiplier: 1,
		CritRate:          0.5,
		CritDamage:        1.0,
		Resistance:        0.2,
		AttackTag:         "skill",
		ElementKey:        "quantum",
		EnemyBroken:       true,
	}, []Modifier{
		{StatKey: "atk_pct", Value: 0.2},
		{StatKey: "crit_dmg", Value: 0.54},
		{StatKey: "dmg_bonus", Value: 0.3},
		{StatKey: "def_shred", Value: 0.1},
		{StatKey: "res_pen", Value: 0.1, ElementKey: "quantum"},
		{StatKey: "action_advance", Value: 0.5, ModifierZone: "utility"},
		{StatKey: "fua_dmg_bonus", Value: 9.9, AttackTag: "fua"},
	})

	assertClose(t, got.BaseDamage, 1200)
	assertClose(t, got.CritMultiplier, 1.77)
	assertClose(t, got.DamageBonusMultiplier, 1.3)
	assertClose(t, got.DefenseMultiplier, 0.5263157894736842)
	assertClose(t, got.ResistanceMultiplier, 0.9)
	if len(got.Utilities) != 1 || got.Utilities[0].StatKey != "action_advance" {
		t.Fatalf("expected action_advance utility, got %#v", got.Utilities)
	}
}

func TestCritMultiplierCapsRate(t *testing.T) {
	assertClose(t, CritMultiplier(1.5, 2), 3)
	assertClose(t, CritMultiplier(-1, 2), 1)
}

func TestEstimateDotDamageNoCrit(t *testing.T) {
	got := EstimateDotDamage(Scenario{
		AttackerLevel:     80,
		EnemyLevel:        80,
		ScalingStat:       1000,
		AbilityMultiplier: 1,
		CritRate:          1,
		CritDamage:        10,
		EnemyBroken:       true,
	}, nil)

	assertClose(t, got.CritMultiplier, 1)
	assertClose(t, got.TotalDamage, 500)
}

func TestEstimateBreakDamage(t *testing.T) {
	got := EstimateBreakDamage(BreakScenario{
		AttackerLevel: 80,
		EnemyLevel:    80,
		ElementKey:    "fire",
		BreakEffect:   1,
		MaxToughness:  90,
		Resistance:    0.2,
	}, nil)

	want := LevelMultiplier(80) * 2 * 2.75 * 2 * 0.5 * 0.8
	assertClose(t, got.ElementMultiplier, 2)
	assertClose(t, got.MaxToughnessMultiplier, 2.75)
	assertClose(t, got.TotalDamage, want)
}

func TestEstimateSuperBreakDamage(t *testing.T) {
	got := EstimateSuperBreakDamage(BreakScenario{
		AttackerLevel:        80,
		EnemyLevel:           80,
		BreakEffect:          1,
		BreakDamageBonus:     0.5,
		ToughnessReduction:   20,
		SuperBreakMultiplier: 1,
		Resistance:           0.2,
	}, nil)

	want := LevelMultiplier(80) * 2 * 1 * 2 * 1.5 * 0.5 * 0.8
	assertClose(t, got.ToughnessReductionFactor, 2)
	assertClose(t, got.TotalDamage, want)
}

func TestEstimateSuperBreakDamageBaseMultiplierModifier(t *testing.T) {
	scenario := BreakScenario{
		AttackerLevel:            80,
		EnemyLevel:               80,
		BreakEffect:              1,
		ToughnessReduction:       20,
		SuperBreakBaseMultiplier: 1,
		Resistance:               0.2,
	}
	baseline := EstimateSuperBreakDamage(scenario, nil)
	got := EstimateSuperBreakDamage(scenario, []Modifier{
		{StatKey: "super_break_base_multiplier", Value: 1.25, AttackTag: "super_break"},
		{StatKey: "super_break_dmg_bonus", Value: 0.4, AttackTag: "super_break"},
	})

	assertClose(t, got.SuperBreakBaseMultiplier, 1.25)
	assertClose(t, got.SuperBreakMultiplier, 1.25)
	assertClose(t, got.BreakDamageMultiplier, 1.4)
	assertClose(t, got.TotalDamage/baseline.TotalDamage, 1.25*1.4)
}

func TestEstimateHealing(t *testing.T) {
	got := EstimateHealing(SustainScenario{
		ScalingStat:          1000,
		AbilityMultiplier:    0.6,
		FlatValue:            100,
		OutgoingHealBonus:    0.2,
		HealingReceivedBonus: 0.1,
	}, nil)

	assertClose(t, got.BaseValue, 700)
	assertClose(t, got.TotalValue, 924)
}

func TestEstimateShield(t *testing.T) {
	got := EstimateShield(SustainScenario{
		ScalingStat:       1000,
		AbilityMultiplier: 0.3,
		FlatValue:         500,
		ShieldStrength:    0.25,
	}, nil)

	assertClose(t, got.BaseValue, 800)
	assertClose(t, got.TotalValue, 1000)
}

func TestEstimateUptime(t *testing.T) {
	got := EstimateUptime(UptimeScenario{
		DurationTurns:   2,
		CooldownTurns:   3,
		StartDelayTurns: 0.5,
	})

	assertClose(t, got.CycleTurns, 3)
	assertClose(t, got.ActiveTurns, 1.5)
	assertClose(t, got.Uptime, 0.5)
}

func TestLevelAndElementMultipliers(t *testing.T) {
	assertClose(t, LevelMultiplier(80), 3767.5533)
	assertClose(t, ElementBreakMultiplier("physical"), 2)
	assertClose(t, ElementBreakMultiplier("wind"), 1.5)
	assertClose(t, ElementBreakMultiplier("quantum"), 0.5)
}

func assertClose(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %.12f want %.12f", got, want)
	}
}
