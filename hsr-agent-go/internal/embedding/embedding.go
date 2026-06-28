package embedding

import (
	"crypto/sha256"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

const Dimensions = 1024

var asciiTokenRE = regexp.MustCompile(`[A-Za-z0-9_]+`)

var aliases = map[string][]string{
	"crit_rate":      {"crit_rate", "暴击率", "双暴"},
	"crit_dmg":       {"crit_dmg", "暴击伤害", "爆伤", "暴伤", "双暴"},
	"atk_percent":    {"atk_percent", "攻击力", "攻击百分比", "atk"},
	"dmg_percent":    {"dmg_percent", "增伤", "伤害提高", "造成的伤害提高"},
	"speed":          {"speed", "速度", "spd"},
	"turn_advance":   {"turn_advance", "行动提前", "拉条", "提前行动"},
	"energy_regen":   {"energy_regen", "能量恢复效率", "充能", "回能", "能量"},
	"energy_restore": {"energy_restore", "恢复能量", "回能", "能量"},
	"sp_generation":  {"sp_generation", "战技点上限", "产点", "sp"},
	"sp_recovery":    {"sp_recovery", "恢复战技点", "回点", "sp"},
	"sp_consumption": {"sp_consumption", "消耗战技点", "耗点", "sp"},
	"fua":            {"fua", "追加攻击", "追击", "fua_team", "fua_dmg"},
	"break":          {"break", "击破", "超击破", "削韧", "break_specialist"},
	"dot":            {"dot", "持续伤害", "dot_enabler", "dot_detonator"},
	"debuff":         {"debuff", "负面效果", "减防", "易伤", "debuffer"},
	"sustain":        {"sustain", "治疗", "护盾", "生存", "sustain_healer", "sustain_shielder"},
	"main_dps":       {"main_dps", "主c", "主 C", "输出位"},
	"sub_dps":        {"sub_dps", "副c", "副 C", "副输出"},
	"amplifier":      {"amplifier", "同谐", "辅助", "拐"},
}

func Embed(text string) []float64 {
	normalized := strings.ToLower(text)
	vec := make([]float64, Dimensions)
	for _, token := range asciiTokenRE.FindAllString(normalized, -1) {
		addFeature(vec, "tok:"+token, 2.0)
		for _, part := range strings.Split(token, "_") {
			if part != "" {
				addFeature(vec, "tok:"+part, 1.0)
			}
		}
	}
	for _, segment := range cjkSegments(normalized) {
		runes := []rune(segment)
		for _, char := range runes {
			addFeature(vec, "c1:"+string(char), 0.15)
		}
		for _, spec := range []struct {
			n      int
			weight float64
		}{{2, 0.8}, {3, 1.0}, {4, 0.6}} {
			if len(runes) < spec.n {
				continue
			}
			for i := 0; i <= len(runes)-spec.n; i++ {
				addFeature(vec, fmt.Sprintf("c%d:%s", spec.n, string(runes[i:i+spec.n])), spec.weight)
			}
		}
	}
	for feature, weight := range aliasFeatures(normalized) {
		addFeature(vec, feature, weight)
	}

	var norm float64
	for _, value := range vec {
		norm += value * value
	}
	if norm == 0 {
		return vec
	}
	norm = math.Sqrt(norm)
	for i, value := range vec {
		vec[i] = value / norm
	}
	return vec
}

func VectorLiteral(vec []float64) string {
	parts := make([]string, len(vec))
	for i, value := range vec {
		parts[i] = fmt.Sprintf("%.8f", value)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func addFeature(vec []float64, feature string, weight float64) {
	if feature == "" {
		return
	}
	digest := sha256.Sum256([]byte(feature))
	bucket := int(uint32(digest[0])<<24|uint32(digest[1])<<16|uint32(digest[2])<<8|uint32(digest[3])) % Dimensions
	sign := -1.0
	if digest[4]&1 == 1 {
		sign = 1.0
	}
	vec[bucket] += sign * weight
}

func aliasFeatures(text string) map[string]float64 {
	out := make(map[string]float64)
	compact := compactSpaces(text)
	for canonical, terms := range aliases {
		found := false
		for _, term := range terms {
			if strings.Contains(compact, compactSpaces(strings.ToLower(term))) {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		out["axis:"+canonical] += 6.0
		for _, term := range terms {
			out["alias:"+compactSpaces(strings.ToLower(term))] += 2.0
		}
	}
	return out
}

func compactSpaces(text string) string {
	return strings.Join(strings.Fields(text), "")
}

func cjkSegments(text string) []string {
	var out []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			out = append(out, string(current))
			current = nil
		}
	}
	for _, r := range text {
		if isCJK(r) {
			current = append(current, r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func isCJK(r rune) bool {
	return (r >= 0x3400 && r <= 0x9fff) || unicode.Is(unicode.Han, r)
}
