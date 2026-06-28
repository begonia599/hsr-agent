from __future__ import annotations

from typing import Any

SOURCE_KINDS = [
    "basic",
    "skill",
    "ult",
    "talent",
    "technique",
    "trace",
    "eidolon",
    "memosprite",
    "summon",
    "passive",
    "other",
]

TARGET_SCOPES = [
    "self",
    "one_ally",
    "all_allies",
    "self_and_allies",
    "summon",
    "memosprite",
    "one_enemy",
    "all_enemies",
    "adjacent_enemies",
    "field",
]

STAT_KEYS = [
    "atk_pct",
    "atk_flat",
    "atk_flat_scaling_from_self_atk",
    "hp_pct",
    "hp_flat",
    "def_pct",
    "def_flat",
    "speed_pct",
    "speed_flat",
    "crit_rate",
    "crit_dmg",
    "break_effect",
    "effect_hit_rate",
    "effect_res",
    "energy_regen",
    "energy_restore",
    "dmg_bonus",
    "element_dmg_bonus",
    "basic_dmg_bonus",
    "skill_dmg_bonus",
    "ult_dmg_bonus",
    "fua_dmg_bonus",
    "dot_dmg_bonus",
    "break_dmg_bonus",
    "super_break_dmg_bonus",
    "additional_dmg",
    "def_ignore",
    "def_shred",
    "res_pen",
    "res_reduction",
    "vulnerability",
    "dmg_reduction",
    "outgoing_heal",
    "healing_received",
    "shield_strength",
    "action_advance",
    "action_delay",
    "weakness_break_efficiency",
    "toughness_reduce",
    "toughness_ignore",
    "weakness_implant",
    "sp_recovery",
    "sp_generation",
    "sp_consumption",
    "max_sp",
    "energy_drain",
    "cleanse",
    "revive",
    "aggro",
    "taunt",
    "debuff_apply",
    "debuff_resist",
    "buff_extend",
    "debuff_extend",
    "extra_action",
    "fua_trigger",
    "dot_trigger",
    "unknown",
]

VALUE_UNITS = [
    "percent",
    "flat",
    "ratio",
    "count",
    "turn",
    "stack",
    "energy",
    "toughness",
    "action_percent",
    "action_value",
    "unknown",
]

MODIFIER_ZONES = [
    "base",
    "crit",
    "dmg_bonus",
    "def",
    "res",
    "vuln",
    "mitigation",
    "toughness",
    "break",
    "heal",
    "shield",
    "utility",
    "unknown",
]

ATTACK_TAGS = [
    "any",
    "basic",
    "skill",
    "ult",
    "fua",
    "dot",
    "break",
    "super_break",
    "additional",
]

ELEMENT_KEYS = [
    "any",
    "physical",
    "fire",
    "ice",
    "thunder",
    "wind",
    "quantum",
    "imaginary",
]

PATH_KEYS = [
    "any",
    "knight",
    "mage",
    "priest",
    "rogue",
    "shaman",
    "warlock",
    "warrior",
    "memory",
]

DURATION_KEYS = [
    "instant",
    "passive",
    "until_turn_start",
    "until_turn_end",
    "fixed_turns",
    "field_active",
    "ult_active",
    "skill_active",
    "stack_based",
    "permanent",
    "unknown",
]

STACK_RULES = [
    "none",
    "refresh",
    "stack_add",
    "stack_independent",
    "replace",
    "max_only",
    "unknown",
]

MODIFIER_ITEM_SCHEMA: dict[str, Any] = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "source_key": {"type": "string"},
        "source_kind": {"type": "string", "enum": SOURCE_KINDS},
        "target_scope": {"type": "string", "enum": TARGET_SCOPES},
        "stat_key": {"type": "string", "enum": STAT_KEYS},
        "value": {"type": ["number", "null"]},
        "value_unit": {"type": "string", "enum": VALUE_UNITS},
        "modifier_zone": {"type": "string", "enum": MODIFIER_ZONES},
        "attack_tag": {"type": "string", "enum": ATTACK_TAGS},
        "element_key": {"type": "string", "enum": ELEMENT_KEYS},
        "target_path": {"type": "string", "enum": PATH_KEYS},
        "condition_text": {"type": "string"},
        "condition_jsonb": {"type": "object"},
        "duration_key": {"type": "string", "enum": DURATION_KEYS},
        "stack_rule": {"type": "string", "enum": STACK_RULES},
        "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    },
    "required": [
        "source_key",
        "source_kind",
        "target_scope",
        "stat_key",
        "value_unit",
        "modifier_zone",
        "condition_text",
        "condition_jsonb",
        "confidence",
    ],
}

MODIFIER_RESPONSE_SCHEMA: dict[str, Any] = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "modifiers": {"type": "array", "items": MODIFIER_ITEM_SCHEMA},
        "notes": {"type": "string"},
    },
    "required": ["modifiers"],
}


def tool_schema() -> dict[str, Any]:
    return {
        "name": "emit_character_modifiers",
        "description": "Return normalized HSR character numerical/utility modifiers from Chinese source text.",
        "input_schema": MODIFIER_RESPONSE_SCHEMA,
    }


def openai_tool_schema() -> dict[str, Any]:
    return {
        "type": "function",
        "function": {
            "name": "emit_character_modifiers",
            "description": "Return normalized HSR character numerical/utility modifiers from Chinese source text.",
            "parameters": MODIFIER_RESPONSE_SCHEMA,
        },
    }


def vocab_prompt() -> str:
    return "\n".join(
        [
            f"source_kind: {', '.join(SOURCE_KINDS)}",
            f"target_scope: {', '.join(TARGET_SCOPES)}",
            f"stat_key: {', '.join(STAT_KEYS)}",
            f"value_unit: {', '.join(VALUE_UNITS)}",
            f"modifier_zone: {', '.join(MODIFIER_ZONES)}",
            f"attack_tag: {', '.join(ATTACK_TAGS)}",
            f"element_key: {', '.join(ELEMENT_KEYS)}",
            f"target_path: {', '.join(PATH_KEYS)}",
            f"duration_key: {', '.join(DURATION_KEYS)}",
            f"stack_rule: {', '.join(STACK_RULES)}",
        ]
    )


def _clean_optional_enum(value: Any, allowed: set[str]) -> str | None:
    if isinstance(value, str) and value in allowed:
        return value
    return None


def normalize_modifier(item: dict[str, Any], valid_sources: set[tuple[str, str]]) -> dict[str, Any] | None:
    source_key = str(item.get("source_key") or "").strip()
    source_kind = str(item.get("source_kind") or "").strip()
    if (source_kind, source_key) not in valid_sources:
        return None
    if source_kind not in SOURCE_KINDS:
        return None

    target_scope = item.get("target_scope")
    stat_key = item.get("stat_key")
    value_unit = item.get("value_unit")
    modifier_zone = item.get("modifier_zone")
    if target_scope not in TARGET_SCOPES or stat_key not in STAT_KEYS:
        return None
    if value_unit not in VALUE_UNITS or modifier_zone not in MODIFIER_ZONES:
        return None

    value = item.get("value")
    if value is not None:
        try:
            value = float(value)
        except (TypeError, ValueError):
            value = None

    condition_jsonb = item.get("condition_jsonb")
    if not isinstance(condition_jsonb, dict):
        condition_jsonb = {}

    confidence = item.get("confidence", 0)
    try:
        confidence = max(0.0, min(1.0, float(confidence)))
    except (TypeError, ValueError):
        confidence = 0.0

    return {
        "source_kind": source_kind,
        "source_key": source_key,
        "target_scope": target_scope,
        "stat_key": stat_key,
        "value": value,
        "value_unit": value_unit,
        "modifier_zone": modifier_zone,
        "attack_tag": _clean_optional_enum(item.get("attack_tag"), set(ATTACK_TAGS)),
        "element_key": _clean_optional_enum(item.get("element_key"), set(ELEMENT_KEYS)),
        "target_path": _clean_optional_enum(item.get("target_path"), set(PATH_KEYS)),
        "condition_text": str(item.get("condition_text") or "").strip(),
        "condition_jsonb": condition_jsonb,
        "duration_key": _clean_optional_enum(item.get("duration_key"), set(DURATION_KEYS)),
        "stack_rule": _clean_optional_enum(item.get("stack_rule"), set(STACK_RULES)),
        "confidence": confidence,
    }


def normalize_modifier_payload(payload: dict[str, Any], valid_sources: set[tuple[str, str]]) -> dict[str, Any]:
    out: dict[str, Any] = {"modifiers": []}
    for item in payload.get("modifiers") or []:
        if isinstance(item, dict):
            normalized = normalize_modifier(item, valid_sources)
            if normalized is not None:
                out["modifiers"].append(normalized)
    notes = payload.get("notes")
    if isinstance(notes, str) and notes.strip():
        out["notes"] = notes.strip()
    return out
