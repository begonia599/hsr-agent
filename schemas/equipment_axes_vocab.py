from __future__ import annotations

from typing import Any

from schemas.axes_vocab import STATS, TAGS, TARGETS, UPTIMES

KINDS = ["provides", "needs", "restricts", "tag"]

EQUIPMENT_AXIS_ITEM_SCHEMA: dict[str, Any] = {
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
        "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    },
    "required": ["stat"],
}

EQUIPMENT_AXES_INPUT_SCHEMA: dict[str, Any] = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "provides": {"type": "array", "items": EQUIPMENT_AXIS_ITEM_SCHEMA},
        "needs": {"type": "array", "items": EQUIPMENT_AXIS_ITEM_SCHEMA},
        "restricts": {"type": "array", "items": EQUIPMENT_AXIS_ITEM_SCHEMA},
        "tags": {
            "type": "array",
            "items": {"type": "string", "enum": TAGS},
            "uniqueItems": True,
        },
        "notes": {"type": "string"},
    },
    "required": ["provides", "needs", "restricts", "tags"],
}


def equipment_tool_schema() -> dict[str, Any]:
    return {
        "name": "emit_equipment_axes",
        "description": "Return normalized lightcone or relic-set axes.",
        "input_schema": EQUIPMENT_AXES_INPUT_SCHEMA,
    }


def equipment_vocab_prompt() -> str:
    return "\n".join(
        [
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
    confidence = item.get("confidence")
    if isinstance(confidence, (int, float)):
        normalized["confidence"] = max(0.0, min(1.0, float(confidence)))
    return normalized


def normalize_equipment_axes(payload: dict[str, Any]) -> dict[str, Any]:
    out = {
        "provides": [],
        "needs": [],
        "restricts": [],
        "tags": _dedupe(payload.get("tags") or [], set(TAGS)),
    }
    for key in ["provides", "needs", "restricts"]:
        seen: set[tuple[Any, ...]] = set()
        for item in payload.get(key) or []:
            if not isinstance(item, dict):
                continue
            normalized = _normalize_axis_item(item)
            if not normalized:
                continue
            fingerprint = (
                normalized.get("stat"),
                normalized.get("target", ""),
                normalized.get("value"),
                normalized.get("uptime", ""),
                normalized.get("condition", ""),
                normalized.get("source", ""),
            )
            if fingerprint in seen:
                continue
            seen.add(fingerprint)
            out[key].append(normalized)

    notes = payload.get("notes")
    if isinstance(notes, str) and notes.strip():
        out["notes"] = notes.strip()
    return out
