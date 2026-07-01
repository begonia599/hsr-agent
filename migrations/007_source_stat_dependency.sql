ALTER TABLE character_modifiers
    ADD COLUMN IF NOT EXISTS source_stat_dependency JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE character_modifiers m
SET source_stat_dependency = jsonb_build_object(
        'source', 'caster',
        'stat', 'break_effect',
        'ratio', 0.15,
        'flat', 0
    )
FROM character_effect_sources s
WHERE m.source_id = s.id
  AND m.stat_key = 'break_effect'
  AND s.source_kind = 'eidolon'
  AND s.source_key IN ('4', 'e4', 'E4')
  AND m.condition_text LIKE '%等同于%击破特攻%'
  AND s.character_id IN (8005, 8006);

UPDATE character_modifiers m
SET source_stat_dependency = jsonb_build_object(
        'source', 'caster',
        'stat', 'crit_dmg',
        'ratio', 0.3,
        'flat', 0.54
    )
FROM character_effect_sources s
WHERE m.source_id = s.id
  AND m.stat_key = 'crit_dmg'
  AND s.character_id = 1306
  AND s.source_kind = 'skill'
  AND s.source_key = '130602'
  AND m.condition_text LIKE '%等同于花火%暴击伤害%';

UPDATE character_modifiers m
SET source_stat_dependency = jsonb_build_object(
        'source', 'caster',
        'stat', 'crit_dmg',
        'ratio', 0.3,
        'flat', 0
    )
FROM character_effect_sources s
WHERE m.source_id = s.id
  AND m.stat_key = 'crit_dmg'
  AND s.character_id = 1306
  AND s.source_kind = 'eidolon'
  AND s.source_key IN ('6', 'e6', 'E6')
  AND m.condition_text LIKE '%等同于花火%暴击伤害%';

DROP VIEW IF EXISTS v_character_modifiers;

CREATE VIEW v_character_modifiers AS
SELECT
    c.id AS character_id,
    c.name_zh AS character_name_zh,
    s.id AS source_id,
    s.source_kind,
    s.source_key,
    s.name_zh AS source_name_zh,
    m.id AS modifier_id,
    m.target_scope,
    m.stat_key,
    m.value,
    m.value_unit,
    m.modifier_zone,
    m.attack_tag,
    m.element_key,
    m.target_path,
    m.condition_text,
    m.condition_jsonb,
    m.source_stat_dependency,
    m.duration_key,
    m.stack_rule,
    m.confidence,
    m.reviewed
FROM character_modifiers m
JOIN character_effect_sources s ON s.id = m.source_id
JOIN characters c ON c.id = s.character_id;
