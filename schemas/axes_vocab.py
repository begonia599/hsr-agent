from __future__ import annotations

from typing import Any

STATS = [
    "atk_percent",
    "atk_flat",
    "atk_flat_scaling_from_self_atk",
    "hp_percent",
    "hp_flat",
    "def_percent",
    "def_flat",
    "speed_flat",
    "speed_percent",
    "crit_rate",
    "crit_dmg",
    "break_eff",
    "effect_hit",
    "effect_res",
    "dmg_percent",
    "dmg_taken_reduce",
    "def_ignore",
    "def_shred",
    "res_pen",
    "res_reduce",
    "vulnerability",
    "heal_percent",
    "outgoing_heal",
    "shield_strength",
    "sp_recovery",
    "sp_generation",
    "sp_consumption",
    "energy_regen",
    "energy_restore",
    "ult_dmg",
    "skill_dmg",
    "basic_dmg",
    "fua_dmg",
    "additional_dmg",
    "dot_dmg",
    "break_dmg",
    "super_break_dmg",
    "true_dmg",
    "weakness_implant",
    "weakness_break_efficiency",
    "cleanse",
    "revive",
    "buff_advance",
    "debuff_extend",
    "turn_advance",
    "turn_delay",
    "toughness_reduce",
    "toughness_ignore",
    "shield_apply",
    "heal_over_time",
    "fua_trigger",
    "dot_trigger",
    "extra_action",
    "energy_drain",
    "action_value",
    "aggro",
    "taunt",
    "debuff_apply",
    "debuff_resist",
    "crowd_control",
    "def_unique_buff",
]

TARGETS = [
    "self",
    "one_ally",
    "one_random_ally",
    "all_allies",
    "self_and_allies",
    "summon",
    "one_enemy",
    "all_enemies",
    "enemies_adjacent",
    "random_enemy",
    "field_aoe",
]

UPTIMES = [
    "passive",
    "combat_start",
    "on_attack",
    "on_basic",
    "on_skill",
    "on_ult",
    "on_fua",
    "on_dot",
    "on_break",
    "on_kill",
    "on_wave_start",
    "ult_active",
    "skill_active",
    "field_active",
    "on_hit_received",
    "on_ally_attack",
    "on_enemy_debuff",
    "conditional",
    "stack_based",
]

ROLES = [
    "main_dps",
    "sub_dps",
    "amplifier",
    "debuffer",
    "sustain_healer",
    "sustain_shielder",
    "sustain_hybrid",
    "remembrance",
    "generalist",
    "break_specialist",
]

TAGS = [
    "hyper_carry",
    "dual_dps",
    "fua_team",
    "dot_team",
    "break_team",
    "super_break_team",
    "ult_team",
    "summon_team",
    "speed_team",
    "sp_positive",
    "sp_neutral",
    "sp_negative",
    "crit_scaler",
    "atk_scaler",
    "hp_scaler",
    "def_scaler",
    "break_scaler",
    "debuff_dependent",
    "shield_dependent",
    "heal_dependent",
    "hp_loss_team",
    "energy_dependent",
    "single_target",
    "aoe",
    "blast",
    "single_dps_preferred",
    "multi_dps_preferred",
]

KINDS = ["provides", "needs", "restricts", "tag"]

AXIS_ITEM_SCHEMA: dict[str, Any] = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "stat": {"type": "string", "enum": STATS},
        "target": {"type": "string", "enum": TARGETS},
        "value": {"type": ["number", "null"]},
        "uptime": {"type": "string", "enum": UPTIMES},
        "condition": {"type": "string"},
        "reason": {"type": "string"},
        "source": {"type": "string"},
    },
    "required": ["stat"],
}

CHARACTER_AXES_INPUT_SCHEMA: dict[str, Any] = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "roles": {
            "type": "array",
            "items": {"type": "string", "enum": ROLES},
            "uniqueItems": True,
        },
        "provides": {"type": "array", "items": AXIS_ITEM_SCHEMA},
        "needs": {"type": "array", "items": AXIS_ITEM_SCHEMA},
        "restricts": {"type": "array", "items": AXIS_ITEM_SCHEMA},
        "tags": {
            "type": "array",
            "items": {"type": "string", "enum": TAGS},
            "uniqueItems": True,
        },
        "notes": {"type": "string"},
    },
    "required": ["roles", "provides", "needs", "restricts", "tags"],
}


def tool_schema() -> dict[str, Any]:
    return {
        "name": "emit_character_axes",
        "description": "Return normalized character roles, axes, and team-style tags.",
        "input_schema": CHARACTER_AXES_INPUT_SCHEMA,
    }


def vocab_prompt() -> str:
    return "\n".join(
        [
            f"role 只能取: {', '.join(ROLES)}",
            f"stat 只能取: {', '.join(STATS)}",
            f"target 只能取: {', '.join(TARGETS)}",
            f"uptime 只能取: {', '.join(UPTIMES)}",
            f"tag 只能取: {', '.join(TAGS)}",
        ]
    )


def _dedupe(values: list[str], allowed: set[str]) -> list[str]:
    out: list[str] = []
    for value in values or []:
        if value in allowed and value not in out:
            out.append(value)
    return out


def _normalize_axis_item(item: dict[str, Any]) -> dict[str, Any] | None:
    stat = item.get("stat")
    if stat not in STATS:
        return None

    normalized: dict[str, Any] = {"stat": stat}
    target = item.get("target")
    if target in TARGETS:
        normalized["target"] = target
    if "value" in item and item.get("value") is not None:
        normalized["value"] = item.get("value")
    uptime = item.get("uptime")
    if uptime in UPTIMES:
        normalized["uptime"] = uptime
    for key in ["condition", "reason", "source"]:
        value = item.get(key)
        if isinstance(value, str) and value.strip():
            normalized[key] = value.strip()
    return normalized


def normalize_axes(payload: dict[str, Any]) -> dict[str, Any]:
    allowed_roles = set(ROLES)
    allowed_tags = set(TAGS)
    out = {
        "roles": _dedupe(payload.get("roles") or [], allowed_roles),
        "provides": [],
        "needs": [],
        "restricts": [],
        "tags": _dedupe(payload.get("tags") or [], allowed_tags),
    }

    for key in ["provides", "needs", "restricts"]:
        for item in payload.get(key) or []:
            if isinstance(item, dict):
                normalized = _normalize_axis_item(item)
                if normalized:
                    out[key].append(normalized)

    notes = payload.get("notes")
    if isinstance(notes, str) and notes.strip():
        out["notes"] = notes.strip()
    return out
