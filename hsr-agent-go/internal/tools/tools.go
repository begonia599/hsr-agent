package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"hsr-agent-go/internal/embedding"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

type Character struct {
	ID             int             `json:"id"`
	Version        string          `json:"version"`
	Rarity         int             `json:"rarity"`
	Path           string          `json:"path"`
	Element        string          `json:"element"`
	NameZH         string          `json:"name_zh"`
	NameEN         string          `json:"name_en"`
	SPNeed         *int            `json:"sp_need,omitempty"`
	Roles          []string        `json:"roles"`
	Axes           json.RawMessage `json:"axes"`
	IsTrailblazer  bool            `json:"is_trailblazer"`
	IsCollab       bool            `json:"is_collab"`
	IsVariant      bool            `json:"is_variant"`
	SkillTextBrief string          `json:"skill_text_brief,omitempty"`
}

type Lightcone struct {
	ID      int             `json:"id"`
	Version string          `json:"version"`
	Rarity  int             `json:"rarity"`
	Path    string          `json:"path"`
	NameZH  string          `json:"name_zh"`
	NameEN  string          `json:"name_en"`
	DescZH  string          `json:"desc_zh,omitempty"`
	Axes    json.RawMessage `json:"axes"`
}

type RelicSet struct {
	ID       int             `json:"id"`
	Version  string          `json:"version"`
	Kind     string          `json:"kind"`
	NameZH   string          `json:"name_zh"`
	NameEN   string          `json:"name_en"`
	Set2Desc string          `json:"set2_desc,omitempty"`
	Set4Desc string          `json:"set4_desc,omitempty"`
	Axes     json.RawMessage `json:"axes"`
}

type AxisRow struct {
	CharID    int      `json:"char_id"`
	NameZH    string   `json:"name_zh,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Stat      string   `json:"stat"`
	Target    string   `json:"target,omitempty"`
	Value     *float64 `json:"value,omitempty"`
	Uptime    string   `json:"uptime,omitempty"`
	Condition string   `json:"condition,omitempty"`
}

type CoOccurrence struct {
	CharID       int    `json:"char_id"`
	NameZH       string `json:"name_zh"`
	Weight       int    `json:"weight"`
	IsMainLineup bool   `json:"is_main_lineup"`
}

type Recommendation struct {
	Kind    string          `json:"kind"`
	ItemID  *int            `json:"item_id,omitempty"`
	Rank    int             `json:"rank"`
	NameZH  string          `json:"name_zh,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Asset struct {
	EntityKind string `json:"entity_kind"`
	EntityID   string `json:"entity_id"`
	Variant    string `json:"variant"`
	LocalPath  string `json:"local_path"`
	CDNURL     string `json:"cdn_url"`
	Bytes      *int   `json:"bytes,omitempty"`
}

type Synergy struct {
	CharID          int      `json:"char_id"`
	NameZH          string   `json:"name_zh"`
	Rarity          int      `json:"rarity"`
	Path            string   `json:"path"`
	Element         string   `json:"element"`
	Roles           []string `json:"roles"`
	Score           float64  `json:"score"`
	CooccurWeight   int      `json:"cooccur_weight"`
	MatchedNeedAxes []string `json:"matched_need_axes,omitempty"`
	Reasons         []string `json:"reasons"`
}

type TeamPlan struct {
	CoreID     int       `json:"core_id"`
	CoreNameZH string    `json:"core_name_zh"`
	Members    []Synergy `json:"members"`
	Notes      []string  `json:"notes"`
}

type SemanticMatch struct {
	Kind    string   `json:"kind"`
	ID      int      `json:"id"`
	NameZH  string   `json:"name_zh"`
	Score   float64  `json:"score"`
	Rarity  int      `json:"rarity,omitempty"`
	Path    string   `json:"path,omitempty"`
	Element string   `json:"element,omitempty"`
	Roles   []string `json:"roles,omitempty"`
}

func (s *Service) ListCharacters(ctx context.Context, query string, role string, element string, path string, rarity int, limit int) ([]Character, error) {
	if limit <= 0 {
		limit = 40
	}
	rows, err := s.db.Query(ctx, `
SELECT id, version, rarity, path, element, name_zh, name_en, sp_need, roles,
       axes, is_trailblazer, is_collab, is_variant,
       CASE WHEN $1 = '' THEN '' ELSE left(skill_text_zh, 500) END AS skill_text_brief
FROM characters
WHERE ($1 = '' OR id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%')
  AND ($2 = '' OR $2 = ANY(roles))
  AND ($3 = '' OR element = $3)
  AND ($4 = '' OR path = $4)
  AND ($5 = 0 OR rarity = $5)
ORDER BY
    CASE WHEN $1 <> '' AND id::text = $1 THEN 0 WHEN $1 <> '' AND (name_zh = $1 OR name_en = $1) THEN 1 ELSE 2 END,
    CASE WHEN $1 = '' THEN 0 ELSE similarity(name_zh, $1) END DESC,
    rarity DESC, id
LIMIT $6`, strings.TrimSpace(query), role, element, path, rarity, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Character
	for rows.Next() {
		var c Character
		if err := rows.Scan(
			&c.ID, &c.Version, &c.Rarity, &c.Path, &c.Element, &c.NameZH, &c.NameEN,
			&c.SPNeed, &c.Roles, &c.Axes, &c.IsTrailblazer, &c.IsCollab, &c.IsVariant,
			&c.SkillTextBrief,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) GetCharacter(ctx context.Context, query string) (*Character, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	sql := `
SELECT id, version, rarity, path, element, name_zh, name_en, sp_need, roles,
       axes, is_trailblazer, is_collab, is_variant,
       left(skill_text_zh, 800) AS skill_text_brief
FROM characters
WHERE id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%'
ORDER BY
    CASE WHEN id::text = $1 THEN 0 WHEN name_zh = $1 OR name_en = $1 THEN 1 ELSE 2 END,
    similarity(name_zh, $1) DESC,
    id
LIMIT 1`

	var c Character
	err := s.db.QueryRow(ctx, sql, query).Scan(
		&c.ID, &c.Version, &c.Rarity, &c.Path, &c.Element, &c.NameZH, &c.NameEN,
		&c.SPNeed, &c.Roles, &c.Axes, &c.IsTrailblazer, &c.IsCollab, &c.IsVariant,
		&c.SkillTextBrief,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCharacterSkills 返回角色原始技能/星魂 wiki 文本(raw_zh 子树),
// 占位符与富文本标签由前端解析,Go 侧只透传 JSONB。
func (s *Service) GetCharacterSkills(ctx context.Context, charID int) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRow(ctx, `
SELECT jsonb_build_object('skills', raw_zh->'skills', 'ranks', raw_zh->'ranks')
FROM characters WHERE id = $1`, charID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *Service) ListLightcones(ctx context.Context, query string, path string, rarity int, limit int) ([]Lightcone, error) {
	if limit <= 0 {
		limit = 40
	}
	rows, err := s.db.Query(ctx, `
SELECT id, version, rarity, path, name_zh, name_en, coalesce(desc_zh, ''), axes
FROM lightcones
WHERE ($1 = '' OR id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%')
  AND ($2 = '' OR path = $2)
  AND ($3 = 0 OR rarity = $3)
ORDER BY
    CASE WHEN $1 <> '' AND id::text = $1 THEN 0 WHEN $1 <> '' AND (name_zh = $1 OR name_en = $1) THEN 1 ELSE 2 END,
    CASE WHEN $1 = '' THEN 0 ELSE similarity(name_zh, $1) END DESC,
    rarity DESC, id
LIMIT $4`, strings.TrimSpace(query), path, rarity, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Lightcone
	for rows.Next() {
		var item Lightcone
		if err := rows.Scan(&item.ID, &item.Version, &item.Rarity, &item.Path, &item.NameZH, &item.NameEN, &item.DescZH, &item.Axes); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) GetLightcone(ctx context.Context, id int) (*Lightcone, error) {
	var item Lightcone
	err := s.db.QueryRow(ctx, `
SELECT id, version, rarity, path, name_zh, name_en, coalesce(desc_zh, ''), axes
FROM lightcones
WHERE id = $1`, id).Scan(&item.ID, &item.Version, &item.Rarity, &item.Path, &item.NameZH, &item.NameEN, &item.DescZH, &item.Axes)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) ListRelicSets(ctx context.Context, query string, kind string, limit int) ([]RelicSet, error) {
	if limit <= 0 {
		limit = 40
	}
	rows, err := s.db.Query(ctx, `
SELECT id, version, kind, name_zh, name_en, coalesce(set2_desc, ''), coalesce(set4_desc, ''), axes
FROM relic_sets
WHERE ($1 = '' OR id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%')
  AND ($2 = '' OR kind = $2)
ORDER BY
    CASE WHEN $1 <> '' AND id::text = $1 THEN 0 WHEN $1 <> '' AND (name_zh = $1 OR name_en = $1) THEN 1 ELSE 2 END,
    CASE WHEN $1 = '' THEN 0 ELSE similarity(name_zh, $1) END DESC,
    id
LIMIT $3`, strings.TrimSpace(query), kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RelicSet
	for rows.Next() {
		var item RelicSet
		if err := rows.Scan(&item.ID, &item.Version, &item.Kind, &item.NameZH, &item.NameEN, &item.Set2Desc, &item.Set4Desc, &item.Axes); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) GetRelicSet(ctx context.Context, id int) (*RelicSet, error) {
	var item RelicSet
	err := s.db.QueryRow(ctx, `
SELECT id, version, kind, name_zh, name_en, coalesce(set2_desc, ''), coalesce(set4_desc, ''), axes
FROM relic_sets
WHERE id = $1`, id).Scan(&item.ID, &item.Version, &item.Kind, &item.NameZH, &item.NameEN, &item.Set2Desc, &item.Set4Desc, &item.Axes)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) SearchByRole(ctx context.Context, role string, element string, path string, rarity int, limit int) ([]Character, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(ctx, `
SELECT id, version, rarity, path, element, name_zh, name_en, sp_need, roles,
       axes, is_trailblazer, is_collab, is_variant, '' AS skill_text_brief
FROM characters
WHERE ($1 = '' OR $1 = ANY(roles))
  AND ($2 = '' OR element = $2)
  AND ($3 = '' OR path = $3)
  AND ($4 = 0 OR rarity = $4)
ORDER BY rarity DESC, id
LIMIT $5`, role, element, path, rarity, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Character
	for rows.Next() {
		var c Character
		if err := rows.Scan(
			&c.ID, &c.Version, &c.Rarity, &c.Path, &c.Element, &c.NameZH, &c.NameEN,
			&c.SPNeed, &c.Roles, &c.Axes, &c.IsTrailblazer, &c.IsCollab, &c.IsVariant,
			&c.SkillTextBrief,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) KeywordSearch(ctx context.Context, query string, kind string, limit int) ([]SemanticMatch, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 10
	}
	if kind == "" {
		kind = "character"
	}
	if kind == "all" {
		var out []SemanticMatch
		for _, subKind := range []string{"character", "lightcone", "relic_set"} {
			rows, err := s.keywordSearchKind(ctx, query, subKind, limit)
			if err != nil {
				return nil, err
			}
			out = append(out, rows...)
		}
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Score > out[j].Score
		})
		if len(out) > limit {
			out = out[:limit]
		}
		return out, nil
	}
	return s.keywordSearchKind(ctx, query, kind, limit)
}

func (s *Service) keywordSearchKind(ctx context.Context, query string, kind string, limit int) ([]SemanticMatch, error) {
	switch kind {
	case "character":
		rows, err := s.db.Query(ctx, `
SELECT id, name_zh, rarity, path, element, roles,
       greatest(similarity(name_zh, $1), similarity(name_en, $1), CASE WHEN id::text = $1 THEN 1 ELSE 0 END)::float8 AS score
FROM characters
WHERE id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%'
ORDER BY score DESC, rarity DESC, id
LIMIT $2`, query, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "character"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Rarity, &item.Path, &item.Element, &item.Roles, &item.Score); err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	case "lightcone":
		rows, err := s.db.Query(ctx, `
SELECT id, name_zh, rarity, path,
       greatest(similarity(name_zh, $1), similarity(name_en, $1), CASE WHEN id::text = $1 THEN 1 ELSE 0 END)::float8 AS score
FROM lightcones
WHERE id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%'
ORDER BY score DESC, rarity DESC, id
LIMIT $2`, query, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "lightcone"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Rarity, &item.Path, &item.Score); err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	case "relic_set":
		rows, err := s.db.Query(ctx, `
SELECT id, name_zh, kind,
       greatest(similarity(name_zh, $1), similarity(name_en, $1), CASE WHEN id::text = $1 THEN 1 ELSE 0 END)::float8 AS score
FROM relic_sets
WHERE id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%'
ORDER BY score DESC, id
LIMIT $2`, query, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "relic_set"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Path, &item.Score); err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	default:
		return nil, fmt.Errorf("unknown keyword search kind %q", kind)
	}
}

func (s *Service) SemanticSearch(ctx context.Context, query string, kind string, limit int) ([]SemanticMatch, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 10
	}
	if kind == "" {
		kind = "character"
	}

	vec := embedding.Embed(query)
	if isZeroVector(vec) {
		return nil, fmt.Errorf("query produced an empty embedding")
	}
	vectorText := embedding.VectorLiteral(vec)

	if kind == "all" {
		var out []SemanticMatch
		for _, subKind := range []string{"character", "lightcone", "relic_set"} {
			rows, err := s.semanticSearchKind(ctx, vectorText, subKind, limit)
			if err != nil {
				return nil, err
			}
			out = append(out, rows...)
		}
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Score > out[j].Score
		})
		if len(out) > limit {
			out = out[:limit]
		}
		return out, nil
	}
	return s.semanticSearchKind(ctx, vectorText, kind, limit)
}

func (s *Service) semanticSearchKind(ctx context.Context, vectorText string, kind string, limit int) ([]SemanticMatch, error) {
	switch kind {
	case "character":
		rows, err := s.db.Query(ctx, `
SELECT id, name_zh, rarity, path, element, roles, 1 - (embedding <=> $1::vector) AS score
FROM characters
WHERE embedding IS NOT NULL
ORDER BY embedding <=> $1::vector
LIMIT $2`, vectorText, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "character"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Rarity, &item.Path, &item.Element, &item.Roles, &item.Score); err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	case "lightcone":
		rows, err := s.db.Query(ctx, `
SELECT id, name_zh, rarity, path, 1 - (embedding <=> $1::vector) AS score
FROM lightcones
WHERE embedding IS NOT NULL
ORDER BY embedding <=> $1::vector
LIMIT $2`, vectorText, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "lightcone"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Rarity, &item.Path, &item.Score); err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	case "relic_set":
		rows, err := s.db.Query(ctx, `
SELECT id, name_zh, kind, 1 - (embedding <=> $1::vector) AS score
FROM relic_sets
WHERE embedding IS NOT NULL
ORDER BY embedding <=> $1::vector
LIMIT $2`, vectorText, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "relic_set"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Path, &item.Score); err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	default:
		return nil, fmt.Errorf("unknown semantic search kind %q", kind)
	}
}

func isZeroVector(vec []float64) bool {
	for _, value := range vec {
		if value != 0 {
			return false
		}
	}
	return true
}

func (s *Service) FindNeeds(ctx context.Context, charID int) ([]AxisRow, error) {
	return s.axisRows(ctx, `
SELECT ca.char_id, c.name_zh, ca.kind, ca.stat, ca.target, ca.value::float8, ca.uptime, coalesce(ca.condition, '')
FROM character_axes ca
JOIN characters c ON c.id = ca.char_id
WHERE ca.char_id = $1 AND ca.kind = 'needs'
ORDER BY ca.stat, ca.target`, charID)
}

func (s *Service) FindBuffersFor(ctx context.Context, axis string, target string, limit int) ([]AxisRow, error) {
	if target == "" {
		target = "all_allies"
	}
	if limit <= 0 {
		limit = 20
	}
	return s.axisRows(ctx, `
SELECT ca.char_id, c.name_zh, ca.kind, ca.stat, ca.target, ca.value::float8, ca.uptime, coalesce(ca.condition, '')
FROM character_axes ca
JOIN characters c ON c.id = ca.char_id
WHERE ca.kind = 'provides' AND ca.stat = $1 AND ($2 = '' OR ca.target = $2)
ORDER BY ca.value DESC NULLS LAST, c.rarity DESC, c.id
LIMIT $3`, axis, target, limit)
}

func (s *Service) axisRows(ctx context.Context, sql string, args ...any) ([]AxisRow, error) {
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AxisRow
	for rows.Next() {
		var item AxisRow
		if err := rows.Scan(&item.CharID, &item.NameZH, &item.Kind, &item.Stat, &item.Target, &item.Value, &item.Uptime, &item.Condition); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) CoOccurrence(ctx context.Context, charID int, limit int) ([]CoOccurrence, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(ctx, `
SELECT t.char_b, c.name_zh, t.weight, t.is_main_lineup
FROM team_cooccur t
JOIN characters c ON c.id = t.char_b
WHERE t.char_a = $1
ORDER BY t.weight DESC, c.rarity DESC, c.id
LIMIT $2`, charID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CoOccurrence
	for rows.Next() {
		var item CoOccurrence
		if err := rows.Scan(&item.CharID, &item.NameZH, &item.Weight, &item.IsMainLineup); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) FindSynergies(ctx context.Context, charID int, limit int) ([]Synergy, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := s.db.Query(ctx, `
WITH core_needs AS (
    SELECT DISTINCT stat
    FROM character_axes
    WHERE char_id = $1 AND kind = 'needs'
),
matched_axes AS (
    SELECT ca.char_id, array_agg(DISTINCT ca.stat ORDER BY ca.stat) AS axes, count(DISTINCT ca.stat) AS match_count
    FROM character_axes ca
    JOIN core_needs n ON n.stat = ca.stat
    WHERE ca.kind = 'provides' AND ca.char_id <> $1
    GROUP BY ca.char_id
),
candidate AS (
    SELECT c.id, c.name_zh, c.rarity, c.path, c.element, c.roles,
           coalesce(t.weight, 0) AS cooccur_weight,
           coalesce(m.match_count, 0) AS match_count,
           coalesce(m.axes, '{}'::text[]) AS matched_need_axes,
           CASE
             WHEN c.roles && ARRAY['sustain_healer','sustain_shielder','sustain_hybrid']::text[] THEN 4
             WHEN c.roles && ARRAY['amplifier','debuffer']::text[] THEN 3
             ELSE 0
           END AS role_bonus
    FROM characters c
    LEFT JOIN team_cooccur t ON t.char_a = $1 AND t.char_b = c.id
    LEFT JOIN matched_axes m ON m.char_id = c.id
    WHERE c.id <> $1
)
SELECT id, name_zh, rarity, path, element, roles,
       (cooccur_weight * 2 + match_count * 5 + role_bonus)::float8 AS score,
       cooccur_weight, matched_need_axes
FROM candidate
WHERE cooccur_weight > 0 OR match_count > 0 OR role_bonus > 0
ORDER BY score DESC, rarity DESC, id
LIMIT $2`, charID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Synergy
	for rows.Next() {
		var item Synergy
		if err := rows.Scan(
			&item.CharID, &item.NameZH, &item.Rarity, &item.Path, &item.Element,
			&item.Roles, &item.Score, &item.CooccurWeight, &item.MatchedNeedAxes,
		); err != nil {
			return nil, err
		}
		item.Reasons = synergyReasons(item)
		out = append(out, item)
	}
	return out, rows.Err()
}

func synergyReasons(item Synergy) []string {
	reasons := make([]string, 0, 3)
	if item.CooccurWeight > 0 {
		reasons = append(reasons, fmt.Sprintf("nanoka 推荐队伍共现权重 %d", item.CooccurWeight))
	}
	if len(item.MatchedNeedAxes) > 0 {
		reasons = append(reasons, "提供核心角色需求轴: "+strings.Join(item.MatchedNeedAxes, ", "))
	}
	if hasAnyRole(item.Roles, "sustain_healer", "sustain_shielder", "sustain_hybrid") {
		reasons = append(reasons, "提供生存位")
	}
	if hasAnyRole(item.Roles, "amplifier", "debuffer") {
		reasons = append(reasons, "提供辅助/负面位")
	}
	return reasons
}

func hasAnyRole(roles []string, wanted ...string) bool {
	set := make(map[string]bool, len(roles))
	for _, role := range roles {
		set[role] = true
	}
	for _, role := range wanted {
		if set[role] {
			return true
		}
	}
	return false
}

func isSustain(roles []string) bool {
	return hasAnyRole(roles, "sustain_healer", "sustain_shielder", "sustain_hybrid")
}

func isSupport(roles []string) bool {
	return hasAnyRole(roles, "amplifier", "debuffer")
}

func (s *Service) SuggestTeam(ctx context.Context, coreID int, slots int, exclude []int) ([]TeamPlan, error) {
	if slots <= 0 {
		slots = 4
	}
	if slots < 2 {
		return nil, fmt.Errorf("slots must be >= 2")
	}
	core, err := s.GetCharacter(ctx, strconv.Itoa(coreID))
	if err != nil {
		return nil, err
	}
	synergies, err := s.FindSynergies(ctx, coreID, 24)
	if err != nil {
		return nil, err
	}
	excluded := map[int]bool{coreID: true}
	for _, id := range exclude {
		excluded[id] = true
	}

	members := make([]Synergy, 0, slots-1)
	hasMemberSustain := func() bool {
		for _, member := range members {
			if isSustain(member.Roles) {
				return true
			}
		}
		return false
	}
	addCandidate := func(predicate func(Synergy) bool) {
		if len(members) >= slots-1 {
			return
		}
		for _, item := range synergies {
			if excluded[item.CharID] || !predicate(item) {
				continue
			}
			members = append(members, item)
			excluded[item.CharID] = true
			return
		}
	}

	coreIsSupport := isSupport(core.Roles) || isSustain(core.Roles)
	if coreIsSupport {
		addCandidate(func(item Synergy) bool {
			return hasAnyRole(item.Roles, "main_dps", "sub_dps") && !isSustain(item.Roles)
		})
		addCandidate(func(item Synergy) bool {
			return hasAnyRole(item.Roles, "main_dps", "sub_dps") || (!isSupport(item.Roles) && !isSustain(item.Roles))
		})
		addCandidate(func(item Synergy) bool { return isSupport(item.Roles) && !isSustain(item.Roles) })
		if !isSustain(core.Roles) && !hasMemberSustain() {
			addCandidate(func(item Synergy) bool { return isSustain(item.Roles) })
		}
	} else {
		addCandidate(func(item Synergy) bool { return isSupport(item.Roles) && !isSustain(item.Roles) })
		addCandidate(func(item Synergy) bool { return isSupport(item.Roles) })
		if !hasMemberSustain() {
			addCandidate(func(item Synergy) bool { return isSustain(item.Roles) })
		}
	}
	for len(members) < slots-1 {
		before := len(members)
		addCandidate(func(Synergy) bool { return true })
		if len(members) == before {
			break
		}
	}

	return []TeamPlan{
		{
			CoreID:     core.ID,
			CoreNameZH: core.NameZH,
			Members:    members,
			Notes: []string{
				"启发式方案: 优先取能满足 needs 的辅助/负面位、一个生存位,再按共现和轴匹配补齐。",
				"最终队伍仍需要 Agent 根据用户已有角色、星魂、装备和战技点压力解释取舍。",
			},
		},
	}, nil
}

func (s *Service) RecommendLightcones(ctx context.Context, charID int) ([]Recommendation, error) {
	return s.recommendations(ctx, `
WITH char_profile AS (
    SELECT path, axes
    FROM characters
    WHERE id = $1
),
char_need_stats AS (
    SELECT DISTINCT stat
    FROM character_axes
    WHERE char_id = $1 AND kind = 'needs'
),
char_fit_stats AS (
    SELECT DISTINCT stat
    FROM character_axes
    WHERE char_id = $1 AND kind IN ('needs', 'provides')
),
char_tags AS (
    SELECT DISTINCT jsonb_array_elements_text(coalesce(axes->'tags', '[]'::jsonb)) AS tag
    FROM char_profile
)
SELECT cr.recommend_kind, cr.item_id, cr.rank, coalesce(l.name_zh, ''),
       coalesce(cr.payload, '{}'::jsonb) || jsonb_build_object(
           'axes', coalesce(l.axes, '{}'::jsonb),
           'basis', 'nanoka_rank_plus_equipment_axes',
           'score', round((
               greatest(0, 100 - cr.rank * 8)
               + CASE WHEN l.path = cp.path THEN 20 ELSE -80 END
               + coalesce(array_length(mp.stats, 1), 0) * 12
               + coalesce(array_length(mr.stats, 1), 0) * 5
               + coalesce(array_length(mt.tags, 1), 0) * 8
           )::numeric, 1),
           'matched_provides', to_jsonb(coalesce(mp.stats, '{}'::text[])),
           'matched_requirements', to_jsonb(coalesce(mr.stats, '{}'::text[])),
           'matched_tags', to_jsonb(coalesce(mt.tags, '{}'::text[])),
           'reasons', to_jsonb(array_remove(ARRAY[
               CASE WHEN l.path = cp.path THEN '光锥命途匹配' ELSE '光锥命途不匹配或缺失' END,
               CASE WHEN coalesce(array_length(mp.stats, 1), 0) > 0 THEN '提供角色需求轴: ' || array_to_string(mp.stats, ', ') END,
               CASE WHEN coalesce(array_length(mr.stats, 1), 0) > 0 THEN '触发需求匹配角色机制: ' || array_to_string(mr.stats, ', ') END,
               CASE WHEN coalesce(array_length(mt.tags, 1), 0) > 0 THEN '标签匹配: ' || array_to_string(mt.tags, ', ') END
           ]::text[], NULL))
       )
FROM character_recommendations cr
JOIN char_profile cp ON true
LEFT JOIN lightcones l ON l.id = cr.item_id
LEFT JOIN LATERAL (
    SELECT array_agg(DISTINCT ea.stat ORDER BY ea.stat) AS stats
    FROM equipment_axes ea
    JOIN char_need_stats cns ON cns.stat = ea.stat
    WHERE ea.entity_kind = 'lightcone' AND ea.entity_id = l.id AND ea.kind = 'provides'
) mp ON true
LEFT JOIN LATERAL (
    SELECT array_agg(DISTINCT ea.stat ORDER BY ea.stat) AS stats
    FROM equipment_axes ea
    JOIN char_fit_stats cfs ON cfs.stat = ea.stat
    WHERE ea.entity_kind = 'lightcone' AND ea.entity_id = l.id AND ea.kind = 'needs'
) mr ON true
LEFT JOIN LATERAL (
    SELECT array_agg(DISTINCT ea.stat ORDER BY ea.stat) AS tags
    FROM equipment_axes ea
    JOIN char_tags ct ON ct.tag = ea.stat
    WHERE ea.entity_kind = 'lightcone' AND ea.entity_id = l.id AND ea.kind = 'tag'
) mt ON true
WHERE cr.char_id = $1 AND cr.recommend_kind = 'lightcone'
ORDER BY (
    greatest(0, 100 - cr.rank * 8)
    + CASE WHEN l.path = cp.path THEN 20 ELSE -80 END
    + coalesce(array_length(mp.stats, 1), 0) * 12
    + coalesce(array_length(mr.stats, 1), 0) * 5
    + coalesce(array_length(mt.tags, 1), 0) * 8
) DESC, cr.rank`, charID)
}

func (s *Service) RecommendRelics(ctx context.Context, charID int) ([]Recommendation, error) {
	return s.recommendations(ctx, `
WITH char_profile AS (
    SELECT axes
    FROM characters
    WHERE id = $1
),
char_need_stats AS (
    SELECT DISTINCT stat
    FROM character_axes
    WHERE char_id = $1 AND kind = 'needs'
),
char_fit_stats AS (
    SELECT DISTINCT stat
    FROM character_axes
    WHERE char_id = $1 AND kind IN ('needs', 'provides')
),
char_tags AS (
    SELECT DISTINCT jsonb_array_elements_text(coalesce(axes->'tags', '[]'::jsonb)) AS tag
    FROM char_profile
)
SELECT cr.recommend_kind, cr.item_id, cr.rank, coalesce(r.name_zh, ''),
       coalesce(cr.payload, '{}'::jsonb) || jsonb_build_object(
           'axes', coalesce(r.axes, '{}'::jsonb),
           'basis', 'nanoka_rank_plus_equipment_axes',
           'score', round((
               greatest(0, 100 - cr.rank * 8)
               + coalesce(array_length(mp.stats, 1), 0) * 12
               + coalesce(array_length(mr.stats, 1), 0) * 5
               + coalesce(array_length(mt.tags, 1), 0) * 8
           )::numeric, 1),
           'matched_provides', to_jsonb(coalesce(mp.stats, '{}'::text[])),
           'matched_requirements', to_jsonb(coalesce(mr.stats, '{}'::text[])),
           'matched_tags', to_jsonb(coalesce(mt.tags, '{}'::text[])),
           'reasons', to_jsonb(array_remove(ARRAY[
               CASE WHEN coalesce(array_length(mp.stats, 1), 0) > 0 THEN '提供角色需求轴: ' || array_to_string(mp.stats, ', ') END,
               CASE WHEN coalesce(array_length(mr.stats, 1), 0) > 0 THEN '触发需求匹配角色机制: ' || array_to_string(mr.stats, ', ') END,
               CASE WHEN coalesce(array_length(mt.tags, 1), 0) > 0 THEN '标签匹配: ' || array_to_string(mt.tags, ', ') END
           ]::text[], NULL))
       )
FROM character_recommendations cr
LEFT JOIN relic_sets r ON r.id = cr.item_id
LEFT JOIN LATERAL (
    SELECT array_agg(DISTINCT ea.stat ORDER BY ea.stat) AS stats
    FROM equipment_axes ea
    JOIN char_need_stats cns ON cns.stat = ea.stat
    WHERE ea.entity_kind = 'relic_set' AND ea.entity_id = r.id AND ea.kind = 'provides'
) mp ON true
LEFT JOIN LATERAL (
    SELECT array_agg(DISTINCT ea.stat ORDER BY ea.stat) AS stats
    FROM equipment_axes ea
    JOIN char_fit_stats cfs ON cfs.stat = ea.stat
    WHERE ea.entity_kind = 'relic_set' AND ea.entity_id = r.id AND ea.kind = 'needs'
) mr ON true
LEFT JOIN LATERAL (
    SELECT array_agg(DISTINCT ea.stat ORDER BY ea.stat) AS tags
    FROM equipment_axes ea
    JOIN char_tags ct ON ct.tag = ea.stat
    WHERE ea.entity_kind = 'relic_set' AND ea.entity_id = r.id AND ea.kind = 'tag'
) mt ON true
WHERE cr.char_id = $1 AND cr.recommend_kind IN ('relic_set4', 'relic_set2')
ORDER BY cr.recommend_kind, (
    greatest(0, 100 - cr.rank * 8)
    + coalesce(array_length(mp.stats, 1), 0) * 12
    + coalesce(array_length(mr.stats, 1), 0) * 5
    + coalesce(array_length(mt.tags, 1), 0) * 8
) DESC, cr.rank`, charID)
}

func (s *Service) recommendations(ctx context.Context, sql string, charID int) ([]Recommendation, error) {
	rows, err := s.db.Query(ctx, sql, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Recommendation
	for rows.Next() {
		var item Recommendation
		if err := rows.Scan(&item.Kind, &item.ItemID, &item.Rank, &item.NameZH, &item.Payload); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) GetAssets(ctx context.Context, entityKind string, entityID string, variants []string) ([]Asset, error) {
	rows, err := s.db.Query(ctx, `
SELECT entity_kind, entity_id, variant, local_path, cdn_url, bytes
FROM asset_paths
WHERE entity_kind = $1 AND entity_id = $2 AND (coalesce(cardinality($3::text[]), 0) = 0 OR variant = ANY($3::text[]))
ORDER BY variant`, entityKind, entityID, variants)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Asset
	for rows.Next() {
		var item Asset
		if err := rows.Scan(&item.EntityKind, &item.EntityID, &item.Variant, &item.LocalPath, &item.CDNURL, &item.Bytes); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func ParseInt(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
}
