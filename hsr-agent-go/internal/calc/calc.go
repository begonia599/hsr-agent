package calc

import (
	"math"
	"strings"
)

type Scenario struct {
	AttackerLevel int
	EnemyLevel    int

	BaseScalingStat   float64
	ScalingStat       float64
	AbilityMultiplier float64
	FlatDamage        float64

	CritRate        float64
	CritDamage      float64
	DamageBonus     float64
	DefReduction    float64
	DefIgnore       float64
	Resistance      float64
	ResReduction    float64
	ResPen          float64
	Vulnerability   float64
	DamageReduction float64

	EnemyBroken bool
	AttackTag   string
	ElementKey  string
}

type Modifier struct {
	StatKey      string
	Value        float64
	ModifierZone string
	AttackTag    string
	ElementKey   string
	Condition    string
}

type UtilityEffect struct {
	StatKey   string  `json:"stat_key"`
	Value     float64 `json:"value"`
	Condition string  `json:"condition,omitempty"`
}

type Breakdown struct {
	BaseDamage              float64         `json:"base_damage"`
	CritMultiplier          float64         `json:"crit_multiplier"`
	DamageBonusMultiplier   float64         `json:"damage_bonus_multiplier"`
	DefenseMultiplier       float64         `json:"defense_multiplier"`
	ResistanceMultiplier    float64         `json:"resistance_multiplier"`
	VulnerabilityMultiplier float64         `json:"vulnerability_multiplier"`
	MitigationMultiplier    float64         `json:"mitigation_multiplier"`
	ToughnessMultiplier     float64         `json:"toughness_multiplier"`
	TotalDamage             float64         `json:"total_damage"`
	Utilities               []UtilityEffect `json:"utilities,omitempty"`
}

type BreakScenario struct {
	AttackerLevel int
	EnemyLevel    int

	ElementKey           string
	EnemyCount           int
	BreakEffect          float64
	BreakDamageBonus     float64
	SuperBreakBonus      float64
	ToughnessReduction   float64
	MaxToughness         float64
	SuperBreakMultiplier float64

	DefReduction    float64
	DefIgnore       float64
	Resistance      float64
	ResReduction    float64
	ResPen          float64
	Vulnerability   float64
	DamageReduction float64
}

type BreakBreakdown struct {
	LevelMultiplier          float64         `json:"level_multiplier"`
	ElementMultiplier        float64         `json:"element_multiplier,omitempty"`
	MaxToughnessMultiplier   float64         `json:"max_toughness_multiplier,omitempty"`
	ToughnessReductionFactor float64         `json:"toughness_reduction_factor,omitempty"`
	BreakEffectMultiplier    float64         `json:"break_effect_multiplier"`
	BreakDamageMultiplier    float64         `json:"break_damage_multiplier,omitempty"`
	SuperBreakMultiplier     float64         `json:"super_break_multiplier,omitempty"`
	DefenseMultiplier        float64         `json:"defense_multiplier"`
	ResistanceMultiplier     float64         `json:"resistance_multiplier"`
	VulnerabilityMultiplier  float64         `json:"vulnerability_multiplier"`
	MitigationMultiplier     float64         `json:"mitigation_multiplier"`
	TotalDamage              float64         `json:"total_damage"`
	Utilities                []UtilityEffect `json:"utilities,omitempty"`
}

type SustainScenario struct {
	BaseScalingStat   float64
	ScalingStat       float64
	AbilityMultiplier float64
	FlatValue         float64

	OutgoingHealBonus    float64
	HealingReceivedBonus float64
	ShieldStrength       float64
}

type SustainBreakdown struct {
	BaseValue          float64         `json:"base_value"`
	OutgoingMultiplier float64         `json:"outgoing_multiplier,omitempty"`
	ReceivedMultiplier float64         `json:"received_multiplier,omitempty"`
	ShieldMultiplier   float64         `json:"shield_multiplier,omitempty"`
	TotalValue         float64         `json:"total_value"`
	Utilities          []UtilityEffect `json:"utilities,omitempty"`
}

type UptimeScenario struct {
	DurationTurns   float64
	CooldownTurns   float64
	CycleTurns      float64
	StartDelayTurns float64
}

type UptimeBreakdown struct {
	DurationTurns float64 `json:"duration_turns"`
	CooldownTurns float64 `json:"cooldown_turns"`
	CycleTurns    float64 `json:"cycle_turns"`
	ActiveTurns   float64 `json:"active_turns"`
	Uptime        float64 `json:"uptime"`
}

func EstimateStandardDamage(input Scenario, modifiers []Modifier) Breakdown {
	scenario, utilities := ApplyModifiers(input, modifiers)
	baseDamage := BaseDamage(scenario)
	critMultiplier := CritMultiplier(scenario.CritRate, scenario.CritDamage)
	damageBonusMultiplier := 1 + scenario.DamageBonus
	defenseMultiplier := DefenseMultiplier(scenario.AttackerLevel, scenario.EnemyLevel, scenario.DefReduction, scenario.DefIgnore)
	resistanceMultiplier := ResistanceMultiplier(scenario.Resistance, scenario.ResReduction, scenario.ResPen)
	vulnerabilityMultiplier := 1 + scenario.Vulnerability
	mitigationMultiplier := math.Max(0, 1-scenario.DamageReduction)
	toughnessMultiplier := ToughnessStateMultiplier(scenario.EnemyBroken)

	total := baseDamage *
		critMultiplier *
		damageBonusMultiplier *
		defenseMultiplier *
		resistanceMultiplier *
		vulnerabilityMultiplier *
		mitigationMultiplier *
		toughnessMultiplier

	return Breakdown{
		BaseDamage:              baseDamage,
		CritMultiplier:          critMultiplier,
		DamageBonusMultiplier:   damageBonusMultiplier,
		DefenseMultiplier:       defenseMultiplier,
		ResistanceMultiplier:    resistanceMultiplier,
		VulnerabilityMultiplier: vulnerabilityMultiplier,
		MitigationMultiplier:    mitigationMultiplier,
		ToughnessMultiplier:     toughnessMultiplier,
		TotalDamage:             total,
		Utilities:               utilities,
	}
}

func EstimateDotDamage(input Scenario, modifiers []Modifier) Breakdown {
	input.AttackTag = "dot"
	scenario, utilities := ApplyModifiers(input, modifiers)
	baseDamage := BaseDamage(scenario)
	damageBonusMultiplier := 1 + scenario.DamageBonus
	defenseMultiplier := DefenseMultiplier(scenario.AttackerLevel, scenario.EnemyLevel, scenario.DefReduction, scenario.DefIgnore)
	resistanceMultiplier := ResistanceMultiplier(scenario.Resistance, scenario.ResReduction, scenario.ResPen)
	vulnerabilityMultiplier := 1 + scenario.Vulnerability
	mitigationMultiplier := math.Max(0, 1-scenario.DamageReduction)
	toughnessMultiplier := ToughnessStateMultiplier(scenario.EnemyBroken)

	total := baseDamage *
		damageBonusMultiplier *
		defenseMultiplier *
		resistanceMultiplier *
		vulnerabilityMultiplier *
		mitigationMultiplier *
		toughnessMultiplier

	return Breakdown{
		BaseDamage:              baseDamage,
		CritMultiplier:          1,
		DamageBonusMultiplier:   damageBonusMultiplier,
		DefenseMultiplier:       defenseMultiplier,
		ResistanceMultiplier:    resistanceMultiplier,
		VulnerabilityMultiplier: vulnerabilityMultiplier,
		MitigationMultiplier:    mitigationMultiplier,
		ToughnessMultiplier:     toughnessMultiplier,
		TotalDamage:             total,
		Utilities:               utilities,
	}
}

func EstimateBreakDamage(input BreakScenario, modifiers []Modifier) BreakBreakdown {
	scenario, utilities := ApplyBreakModifiers(input, modifiers, false)
	levelMultiplier := LevelMultiplier(scenario.AttackerLevel)
	elementMultiplier := ElementBreakMultiplier(scenario.ElementKey)
	maxToughnessMultiplier := MaxToughnessMultiplier(scenario.MaxToughness)
	breakEffectMultiplier := 1 + scenario.BreakEffect
	breakDamageMultiplier := 1 + scenario.BreakDamageBonus
	defenseMultiplier := DefenseMultiplier(scenario.AttackerLevel, scenario.EnemyLevel, scenario.DefReduction, scenario.DefIgnore)
	resistanceMultiplier := ResistanceMultiplier(scenario.Resistance, scenario.ResReduction, scenario.ResPen)
	vulnerabilityMultiplier := 1 + scenario.Vulnerability
	mitigationMultiplier := math.Max(0, 1-scenario.DamageReduction)

	total := levelMultiplier *
		elementMultiplier *
		maxToughnessMultiplier *
		breakEffectMultiplier *
		breakDamageMultiplier *
		defenseMultiplier *
		resistanceMultiplier *
		vulnerabilityMultiplier *
		mitigationMultiplier

	return BreakBreakdown{
		LevelMultiplier:         levelMultiplier,
		ElementMultiplier:       elementMultiplier,
		MaxToughnessMultiplier:  maxToughnessMultiplier,
		BreakEffectMultiplier:   breakEffectMultiplier,
		BreakDamageMultiplier:   breakDamageMultiplier,
		DefenseMultiplier:       defenseMultiplier,
		ResistanceMultiplier:    resistanceMultiplier,
		VulnerabilityMultiplier: vulnerabilityMultiplier,
		MitigationMultiplier:    mitigationMultiplier,
		TotalDamage:             total,
		Utilities:               utilities,
	}
}

func EstimateSuperBreakDamage(input BreakScenario, modifiers []Modifier) BreakBreakdown {
	scenario, utilities := ApplyBreakModifiers(input, modifiers, true)
	levelMultiplier := LevelMultiplier(scenario.AttackerLevel)
	toughnessReductionFactor := scenario.ToughnessReduction / 10
	if toughnessReductionFactor < 0 {
		toughnessReductionFactor = 0
	}
	superBreakMultiplier := scenario.SuperBreakMultiplier
	if superBreakMultiplier == 0 {
		superBreakMultiplier = 1
	}
	breakEffectMultiplier := 1 + scenario.BreakEffect
	breakDamageMultiplier := 1 + scenario.BreakDamageBonus + scenario.SuperBreakBonus
	defenseMultiplier := DefenseMultiplier(scenario.AttackerLevel, scenario.EnemyLevel, scenario.DefReduction, scenario.DefIgnore)
	resistanceMultiplier := ResistanceMultiplier(scenario.Resistance, scenario.ResReduction, scenario.ResPen)
	vulnerabilityMultiplier := 1 + scenario.Vulnerability
	mitigationMultiplier := math.Max(0, 1-scenario.DamageReduction)

	total := levelMultiplier *
		toughnessReductionFactor *
		superBreakMultiplier *
		breakEffectMultiplier *
		breakDamageMultiplier *
		defenseMultiplier *
		resistanceMultiplier *
		vulnerabilityMultiplier *
		mitigationMultiplier

	return BreakBreakdown{
		LevelMultiplier:          levelMultiplier,
		ToughnessReductionFactor: toughnessReductionFactor,
		BreakEffectMultiplier:    breakEffectMultiplier,
		BreakDamageMultiplier:    breakDamageMultiplier,
		SuperBreakMultiplier:     superBreakMultiplier,
		DefenseMultiplier:        defenseMultiplier,
		ResistanceMultiplier:     resistanceMultiplier,
		VulnerabilityMultiplier:  vulnerabilityMultiplier,
		MitigationMultiplier:     mitigationMultiplier,
		TotalDamage:              total,
		Utilities:                utilities,
	}
}

func EstimateHealing(input SustainScenario, modifiers []Modifier) SustainBreakdown {
	scenario, utilities := ApplySustainModifiers(input, modifiers)
	baseValue := SustainBaseValue(scenario)
	outgoingMultiplier := 1 + scenario.OutgoingHealBonus
	receivedMultiplier := 1 + scenario.HealingReceivedBonus
	total := baseValue * outgoingMultiplier * receivedMultiplier
	return SustainBreakdown{
		BaseValue:          baseValue,
		OutgoingMultiplier: outgoingMultiplier,
		ReceivedMultiplier: receivedMultiplier,
		TotalValue:         total,
		Utilities:          utilities,
	}
}

func EstimateShield(input SustainScenario, modifiers []Modifier) SustainBreakdown {
	scenario, utilities := ApplySustainModifiers(input, modifiers)
	baseValue := SustainBaseValue(scenario)
	shieldMultiplier := 1 + scenario.ShieldStrength
	total := baseValue * shieldMultiplier
	return SustainBreakdown{
		BaseValue:        baseValue,
		ShieldMultiplier: shieldMultiplier,
		TotalValue:       total,
		Utilities:        utilities,
	}
}

func EstimateUptime(input UptimeScenario) UptimeBreakdown {
	cycle := input.CycleTurns
	if cycle <= 0 {
		cycle = input.CooldownTurns
	}
	if cycle <= 0 {
		cycle = input.DurationTurns
	}
	active := input.DurationTurns
	if input.StartDelayTurns > 0 {
		active = math.Max(0, active-input.StartDelayTurns)
	}
	if cycle > 0 && active > cycle {
		active = cycle
	}
	uptime := 0.0
	if cycle > 0 {
		uptime = active / cycle
	}
	return UptimeBreakdown{
		DurationTurns: input.DurationTurns,
		CooldownTurns: input.CooldownTurns,
		CycleTurns:    cycle,
		ActiveTurns:   active,
		Uptime:        uptime,
	}
}

func ApplyModifiers(input Scenario, modifiers []Modifier) (Scenario, []UtilityEffect) {
	scenario := input
	scalingPct := 0.0
	scalingFlat := 0.0
	utilities := []UtilityEffect{}

	for _, modifier := range modifiers {
		if !modifierApplies(scenario, modifier) {
			continue
		}
		switch modifier.StatKey {
		case "atk_pct", "hp_pct", "def_pct":
			scalingPct += modifier.Value
		case "atk_flat", "hp_flat", "def_flat":
			scalingFlat += modifier.Value
		case "crit_rate":
			scenario.CritRate += modifier.Value
		case "crit_dmg":
			scenario.CritDamage += modifier.Value
		case "dmg_bonus", "element_dmg_bonus", "basic_dmg_bonus", "skill_dmg_bonus", "ult_dmg_bonus", "fua_dmg_bonus", "dot_dmg_bonus", "additional_dmg":
			scenario.DamageBonus += modifier.Value
		case "def_ignore":
			scenario.DefIgnore += modifier.Value
		case "def_shred":
			scenario.DefReduction += modifier.Value
		case "res_pen":
			scenario.ResPen += modifier.Value
		case "res_reduction":
			scenario.ResReduction += modifier.Value
		case "vulnerability":
			scenario.Vulnerability += modifier.Value
		case "dmg_reduction":
			scenario.DamageReduction += modifier.Value
		case "action_advance", "action_delay", "sp_recovery", "sp_generation", "sp_consumption", "max_sp", "energy_restore", "energy_regen", "toughness_reduce", "weakness_implant", "weakness_break_efficiency", "cleanse", "revive", "speed_pct", "speed_flat", "break_effect", "effect_res", "healing_received", "outgoing_heal", "shield_strength", "toughness_ignore", "buff_extend", "debuff_extend", "extra_action", "fua_trigger", "dot_trigger", "debuff_apply", "debuff_resist":
			utilities = append(utilities, UtilityEffect{StatKey: modifier.StatKey, Value: modifier.Value, Condition: modifier.Condition})
		}
	}

	if scenario.BaseScalingStat > 0 {
		scenario.ScalingStat = scenario.BaseScalingStat*(1+scalingPct) + scalingFlat
	} else {
		scenario.ScalingStat += scalingFlat
		if scalingPct != 0 {
			scenario.ScalingStat *= 1 + scalingPct
		}
	}
	return scenario, utilities
}

func ApplyBreakModifiers(input BreakScenario, modifiers []Modifier, superBreak bool) (BreakScenario, []UtilityEffect) {
	scenario := input
	utilities := []UtilityEffect{}
	for _, modifier := range modifiers {
		if !breakModifierApplies(scenario, modifier, superBreak) {
			continue
		}
		switch modifier.StatKey {
		case "break_effect":
			scenario.BreakEffect += modifier.Value
		case "break_dmg_bonus":
			scenario.BreakDamageBonus += modifier.Value
		case "super_break_dmg_bonus":
			if superBreak {
				scenario.SuperBreakBonus += modifier.Value
			}
		case "weakness_break_efficiency":
			scenario.ToughnessReduction *= 1 + modifier.Value
		case "toughness_reduce":
			scenario.ToughnessReduction += modifier.Value
		case "def_ignore":
			scenario.DefIgnore += modifier.Value
		case "def_shred":
			scenario.DefReduction += modifier.Value
		case "res_pen":
			scenario.ResPen += modifier.Value
		case "res_reduction":
			scenario.ResReduction += modifier.Value
		case "vulnerability":
			scenario.Vulnerability += modifier.Value
		case "dmg_reduction":
			scenario.DamageReduction += modifier.Value
		case "action_advance", "action_delay", "sp_recovery", "sp_generation", "sp_consumption", "max_sp", "energy_restore", "energy_regen", "weakness_implant", "toughness_ignore", "buff_extend", "extra_action":
			utilities = append(utilities, UtilityEffect{StatKey: modifier.StatKey, Value: modifier.Value, Condition: modifier.Condition})
		}
	}
	return scenario, utilities
}

func ApplySustainModifiers(input SustainScenario, modifiers []Modifier) (SustainScenario, []UtilityEffect) {
	scenario := input
	scalingPct := 0.0
	scalingFlat := 0.0
	utilities := []UtilityEffect{}
	for _, modifier := range modifiers {
		if !sustainModifierApplies(modifier) {
			continue
		}
		switch modifier.StatKey {
		case "atk_pct", "hp_pct", "def_pct":
			scalingPct += modifier.Value
		case "atk_flat", "hp_flat", "def_flat":
			scalingFlat += modifier.Value
		case "outgoing_heal":
			scenario.OutgoingHealBonus += modifier.Value
		case "healing_received":
			scenario.HealingReceivedBonus += modifier.Value
		case "shield_strength":
			scenario.ShieldStrength += modifier.Value
		case "cleanse", "revive", "effect_res", "dmg_reduction", "action_advance", "sp_recovery", "energy_restore":
			utilities = append(utilities, UtilityEffect{StatKey: modifier.StatKey, Value: modifier.Value, Condition: modifier.Condition})
		}
	}
	if scenario.BaseScalingStat > 0 {
		scenario.ScalingStat = scenario.BaseScalingStat*(1+scalingPct) + scalingFlat
	} else {
		scenario.ScalingStat += scalingFlat
		if scalingPct != 0 {
			scenario.ScalingStat *= 1 + scalingPct
		}
	}
	return scenario, utilities
}

func BaseDamage(input Scenario) float64 {
	return input.ScalingStat*input.AbilityMultiplier + input.FlatDamage
}

func SustainBaseValue(input SustainScenario) float64 {
	return input.ScalingStat*input.AbilityMultiplier + input.FlatValue
}

func CritMultiplier(rate float64, damage float64) float64 {
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	return 1 + rate*damage
}

func DefenseMultiplier(attackerLevel int, enemyLevel int, defReduction float64, defIgnore float64) float64 {
	if attackerLevel <= 0 {
		attackerLevel = 80
	}
	if enemyLevel <= 0 {
		enemyLevel = 80
	}
	attackerTerm := float64(attackerLevel + 20)
	enemyTerm := float64(enemyLevel+20) * math.Max(0, 1-defReduction-defIgnore)
	return attackerTerm / (enemyTerm + attackerTerm)
}

func ResistanceMultiplier(resistance float64, resReduction float64, resPen float64) float64 {
	effective := resistance - resReduction - resPen
	return 1 - effective
}

func ToughnessStateMultiplier(enemyBroken bool) float64 {
	if enemyBroken {
		return 1
	}
	return 0.9
}

func ElementBreakMultiplier(element string) float64 {
	switch strings.ToLower(element) {
	case "physical", "fire":
		return 2
	case "wind":
		return 1.5
	case "ice", "thunder", "lightning":
		return 1
	case "quantum", "imaginary":
		return 0.5
	default:
		return 1
	}
}

func MaxToughnessMultiplier(maxToughness float64) float64 {
	if maxToughness <= 0 {
		maxToughness = 90
	}
	return maxToughness/40 + 0.5
}

func LevelMultiplier(level int) float64 {
	if level <= 0 {
		level = 80
	}
	if level < 1 {
		level = 1
	}
	if level > len(levelMultipliers)-1 {
		level = len(levelMultipliers) - 1
	}
	return levelMultipliers[level]
}

var levelMultipliers = []float64{
	0,
	54.0000, 58.0000, 62.0000, 67.5264, 70.5094, 73.5228, 76.5660, 79.6385, 82.7395, 85.8684,
	91.4944, 97.0680, 102.5892, 108.0579, 113.4743, 118.8383, 124.1499, 129.4091, 134.6159, 139.7703,
	149.3323, 158.8011, 168.1768, 177.4594, 186.6489, 195.7452, 204.7484, 213.6585, 222.4754, 231.1992,
	246.4276, 261.1810, 275.4733, 289.3179, 302.7275, 315.7144, 328.2905, 340.4671, 352.2554, 363.6658,
	408.1240, 451.7883, 494.6798, 536.8188, 578.2249, 618.9172, 658.9138, 698.2325, 736.8905, 774.9041,
	871.0599, 964.8705, 1056.4206, 1145.7910, 1233.0585, 1318.2965, 1401.5750, 1482.9608, 1562.5178, 1640.3068,
	1752.3215, 1861.9011, 1969.1242, 2074.0659, 2176.7983, 2277.3904, 2375.9085, 2472.4160, 2566.9739, 2659.6406,
	2780.3044, 2898.6022, 3014.6029, 3128.3729, 3239.9758, 3349.4730, 3456.9236, 3562.3843, 3665.9099, 3767.5533,
	3957.8618, 4155.2118, 4359.8638, 4572.0878, 4792.1641, 5020.3833, 5257.0466, 5502.4664, 5756.9667, 6020.8836,
	6294.5654, 6578.3734, 6872.6823, 7177.8806, 7494.3713,
}

func modifierApplies(scenario Scenario, modifier Modifier) bool {
	if modifier.AttackTag != "" && modifier.AttackTag != "any" && scenario.AttackTag != "" && modifier.AttackTag != scenario.AttackTag {
		return false
	}
	if modifier.ElementKey != "" && modifier.ElementKey != "any" && scenario.ElementKey != "" && modifier.ElementKey != scenario.ElementKey {
		return false
	}
	return true
}

func breakModifierApplies(scenario BreakScenario, modifier Modifier, superBreak bool) bool {
	if modifier.ElementKey != "" && modifier.ElementKey != "any" && scenario.ElementKey != "" && !strings.EqualFold(modifier.ElementKey, scenario.ElementKey) {
		return false
	}
	if modifier.AttackTag == "" || modifier.AttackTag == "any" {
		return true
	}
	if superBreak {
		return modifier.AttackTag == "super_break" || modifier.AttackTag == "break"
	}
	return modifier.AttackTag == "break"
}

func sustainModifierApplies(modifier Modifier) bool {
	return modifier.AttackTag == "" || modifier.AttackTag == "any"
}
