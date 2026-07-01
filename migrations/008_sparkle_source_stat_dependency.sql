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
  AND s.source_key = '130602';

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
  AND s.source_key IN ('6', 'e6', 'E6');
