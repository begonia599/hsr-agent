package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"hsr-agent-go/internal/embedding"
	"hsr-agent-go/internal/rerank"

	"github.com/jackc/pgx/v5/pgxpool"
)

const maxRerankDocuments = 25

type Service struct {
	db                 *pgxpool.Pool
	embedder           *embedding.Client
	embedders          map[string]*embedding.Client
	defaultEmbeddingID string
	rerankers          map[string]*rerank.Client
	defaultRerankID    string
	rerankTopN         int
}

func New(db *pgxpool.Pool) *Service {
	return NewWithEmbedder(db, embedding.NewClient(embedding.Config{Provider: "local_hash", Dimensions: embedding.Dimensions}))
}

func NewWithEmbedder(db *pgxpool.Pool, embedder *embedding.Client) *Service {
	return &Service{db: db, embedder: embedder}
}

func NewWithEmbedders(db *pgxpool.Pool, defaultEmbeddingID string, embedders map[string]*embedding.Client) *Service {
	return NewWithModels(db, defaultEmbeddingID, embedders, "", nil, 0)
}

func NewWithModels(db *pgxpool.Pool, defaultEmbeddingID string, embedders map[string]*embedding.Client, defaultRerankID string, rerankers map[string]*rerank.Client, rerankTopN int) *Service {
	service := &Service{db: db, embedders: embedders, defaultEmbeddingID: defaultEmbeddingID}
	service.rerankers = rerankers
	service.defaultRerankID = defaultRerankID
	service.rerankTopN = rerankTopN
	if defaultEmbeddingID != "" {
		service.embedder = embedders[defaultEmbeddingID]
	}
	if service.embedder == nil {
		for id, client := range embedders {
			if service.defaultEmbeddingID == "" {
				service.defaultEmbeddingID = id
			}
			service.embedder = client
			break
		}
	}
	if service.rerankTopN <= 0 {
		service.rerankTopN = 50
	}
	return service
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
	ID          int             `json:"id"`
	Version     string          `json:"version"`
	Rarity      int             `json:"rarity"`
	Path        string          `json:"path"`
	NameZH      string          `json:"name_zh"`
	NameEN      string          `json:"name_en"`
	DescZH      string          `json:"desc_zh,omitempty"`
	Axes        json.RawMessage `json:"axes"`
	DataQuality string          `json:"data_quality,omitempty"`
	Warning     string          `json:"warning,omitempty"`
}

type RelicSet struct {
	ID        int             `json:"id"`
	Version   string          `json:"version"`
	Kind      string          `json:"kind"`
	NameZH    string          `json:"name_zh"`
	NameEN    string          `json:"name_en"`
	Set2Desc  string          `json:"set2_desc,omitempty"`
	Set4Desc  string          `json:"set4_desc,omitempty"`
	FigureURL string          `json:"figure_url,omitempty"`
	Axes      json.RawMessage `json:"axes"`
}

// relicFigureURL 从 relic_sets.raw_zh.icon(形如 SpriteOutput/ItemIcon/71001.png)
// 解析出 icon 编号,拼成 /media/itemfigures/<icon>.webp。遗器套装图按 icon 编号(71xxx)
// 而非套装 id 存放,故必须走这里 —— 用 set id 会撞到无关物品图或缺图。
func relicFigureURL(icon string) string {
	icon = strings.ReplaceAll(strings.TrimSpace(icon), "\\", "/")
	if icon == "" {
		return ""
	}
	base := icon[strings.LastIndex(icon, "/")+1:]
	if i := strings.LastIndex(base, "."); i >= 0 {
		base = base[:i]
	}
	if base == "" {
		return ""
	}
	return AssetURLPrefix + "/itemfigures/" + base + ".webp"
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
	LocalURL   string `json:"local_url,omitempty"`
	Bytes      *int   `json:"bytes,omitempty"`
}

// AssetURLPrefix 是后端本地资源静态路由前缀。前端 ASSET_BASE 指向它即可同源
// 加载图片,避免跨境 CDN。必须与 httpapi serveMedia 的路由前缀保持一致。
const AssetURLPrefix = "/media"

// LocalAssetURL 把 asset_paths.local_path(形如 nanoka_hsr/<ver>/assets/hsr/<sub>)
// 映射成同源后端路径 /media/<sub>。找不到 assets/hsr 标记时返回空,调用方回退 cdn_url。
func LocalAssetURL(localPath string) string {
	if strings.TrimSpace(localPath) == "" {
		return ""
	}
	p := strings.ReplaceAll(localPath, "\\", "/")
	const marker = "/assets/hsr/"
	idx := strings.Index(p, marker)
	if idx < 0 {
		return ""
	}
	sub := strings.TrimPrefix(p[idx+len(marker):], "/")
	if sub == "" {
		return ""
	}
	return AssetURLPrefix + "/" + sub
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
	Kind                string   `json:"kind"`
	ID                  int      `json:"id"`
	NameZH              string   `json:"name_zh"`
	URL                 string   `json:"url,omitempty"`
	Markdown            string   `json:"markdown,omitempty"`
	Score               float64  `json:"score"`
	ScoreExplain        string   `json:"score_explain,omitempty"`
	RecallSource        string   `json:"recall_source,omitempty"`
	RecallScore         float64  `json:"recall_score,omitempty"`
	RuleScore           float64  `json:"rule_score,omitempty"`
	RerankScore         *float64 `json:"rerank_score,omitempty"`
	FinalScore          float64  `json:"final_score,omitempty"`
	EmbeddingModel      string   `json:"embedding_model,omitempty"`
	EmbeddingModelID    string   `json:"embedding_model_id,omitempty"`
	EmbeddingProvider   string   `json:"embedding_provider,omitempty"`
	EmbeddingDimensions int      `json:"embedding_dimensions,omitempty"`
	EmbeddingQuality    string   `json:"embedding_quality,omitempty"`
	RerankModel         string   `json:"rerank_model,omitempty"`
	RerankModelID       string   `json:"rerank_model_id,omitempty"`
	RerankProvider      string   `json:"rerank_provider,omitempty"`
	Rarity              int      `json:"rarity,omitempty"`
	Path                string   `json:"path,omitempty"`
	Element             string   `json:"element,omitempty"`
	Roles               []string `json:"roles,omitempty"`
	CandidateText       string   `json:"-"`
}

type EntityResolveRequest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type EntityResolveResult struct {
	Name          string  `json:"name"`
	Kind          string  `json:"kind"`
	Found         bool    `json:"found"`
	ID            *int    `json:"id,omitempty"`
	NameZH        string  `json:"name_zh,omitempty"`
	URL           string  `json:"url,omitempty"`
	ImageURL      string  `json:"image_url,omitempty"`
	LocalImageURL string  `json:"local_image_url,omitempty"`
	Markdown      string  `json:"markdown,omitempty"`
	Score         float64 `json:"score,omitempty"`
	Reason        string  `json:"reason,omitempty"`
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

func (s *Service) ResolveEntities(ctx context.Context, entities []EntityResolveRequest, display string) ([]EntityResolveResult, error) {
	if len(entities) == 0 {
		return nil, fmt.Errorf("entities is required")
	}
	out := make([]EntityResolveResult, 0, len(entities))
	for _, entity := range entities {
		result, err := s.resolveEntity(ctx, entity, display)
		if err != nil {
			return nil, err
		}
		out = append(out, result)
	}
	return out, nil
}

func (s *Service) resolveEntity(ctx context.Context, entity EntityResolveRequest, display string) (EntityResolveResult, error) {
	name := strings.TrimSpace(entity.Name)
	kind := strings.TrimSpace(entity.Kind)
	result := EntityResolveResult{Name: name, Kind: kind}
	if name == "" {
		result.Reason = "empty name"
		return result, nil
	}
	sql, routePrefix, imageVariant, ok := entityResolveSQL(kind)
	if !ok {
		result.Reason = "unknown kind"
		return result, nil
	}
	var id int
	var nameZH string
	var score float64
	var exact bool
	err := s.db.QueryRow(ctx, sql, name).Scan(&id, &nameZH, &score, &exact)
	if err != nil {
		result.Reason = "not found"
		return result, nil
	}
	if !exact && score < 0.15 {
		result.Reason = "low similarity"
		result.Score = score
		return result, nil
	}
	result.Found = true
	result.ID = &id
	result.NameZH = nameZH
	result.URL = fmt.Sprintf("%s/%d", routePrefix, id)
	result.Markdown = fmt.Sprintf("[%s](%s)", nameZH, result.URL)
	result.Score = score
	if display == "image" || display == "both" {
		result.ImageURL, result.LocalImageURL = s.resolveEntityImageURL(ctx, kind, id, imageVariant)
	}
	return result, nil
}

func entityResolveSQL(kind string) (string, string, string, bool) {
	switch kind {
	case "character":
		return `
SELECT id, name_zh,
       greatest(similarity(name_zh, $1), similarity(name_en, $1), CASE WHEN id::text = $1 THEN 1 ELSE 0 END)::float8 AS score,
       (id::text = $1 OR name_zh = $1 OR name_en = $1) AS exact
FROM characters
WHERE id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%'
ORDER BY exact DESC, score DESC, rarity DESC, id
LIMIT 1`, "/characters", "round", true
	case "lightcone":
		return `
SELECT id, name_zh,
       greatest(similarity(name_zh, $1), similarity(name_en, $1), CASE WHEN id::text = $1 THEN 1 ELSE 0 END)::float8 AS score,
       (id::text = $1 OR name_zh = $1 OR name_en = $1) AS exact
FROM lightcones
WHERE id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%'
ORDER BY exact DESC, score DESC, rarity DESC, id
LIMIT 1`, "/lightcones", "medium", true
	case "relic_set":
		return `
SELECT id, name_zh,
       greatest(similarity(name_zh, $1), similarity(name_en, $1), CASE WHEN id::text = $1 THEN 1 ELSE 0 END)::float8 AS score,
       (id::text = $1 OR name_zh = $1 OR name_en = $1) AS exact
FROM relic_sets
WHERE id::text = $1 OR name_zh ILIKE '%' || $1 || '%' OR name_en ILIKE '%' || $1 || '%'
ORDER BY exact DESC, score DESC, id
LIMIT 1`, "/relic-sets", "figure", true
	default:
		return "", "", "", false
	}
}

func (s *Service) resolveEntityImageURL(ctx context.Context, kind string, id int, variant string) (cdnURL string, localURL string) {
	assets, err := s.GetAssets(ctx, kind, strconv.Itoa(id), []string{variant})
	if err != nil || len(assets) == 0 {
		return "", ""
	}
	return assets[0].CDNURL, assets[0].LocalURL
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
SELECT id, version, rarity, path, name_zh, name_en, coalesce(desc_zh, ''),
       CASE
           WHEN coalesce(desc_zh, '') <> '' THEN axes
           ELSE jsonb_build_object(
               'provides', '[]'::jsonb,
               'needs', '[]'::jsonb,
               'restricts', '[]'::jsonb,
               'tags', '[]'::jsonb,
               'notes', '光锥效果文本缺失;弱画像 axes 已从光锥接口隐藏。'
           )
       END,
       CASE WHEN coalesce(desc_zh, '') <> '' THEN 'effect_text_extracted' ELSE 'weak_profile_inferred' END,
       CASE WHEN coalesce(desc_zh, '') = '' THEN '光锥效果文本缺失;当前 axes 不可作为机制事实。补抓光锥详情后重建。' ELSE '' END
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
		if err := rows.Scan(&item.ID, &item.Version, &item.Rarity, &item.Path, &item.NameZH, &item.NameEN, &item.DescZH, &item.Axes, &item.DataQuality, &item.Warning); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) GetLightcone(ctx context.Context, id int) (*Lightcone, error) {
	var item Lightcone
	err := s.db.QueryRow(ctx, `
SELECT id, version, rarity, path, name_zh, name_en, coalesce(desc_zh, ''),
       CASE
           WHEN coalesce(desc_zh, '') <> '' THEN axes
           ELSE jsonb_build_object(
               'provides', '[]'::jsonb,
               'needs', '[]'::jsonb,
               'restricts', '[]'::jsonb,
               'tags', '[]'::jsonb,
               'notes', '光锥效果文本缺失;弱画像 axes 已从光锥接口隐藏。'
           )
       END,
       CASE WHEN coalesce(desc_zh, '') <> '' THEN 'effect_text_extracted' ELSE 'weak_profile_inferred' END,
       CASE WHEN coalesce(desc_zh, '') = '' THEN '光锥效果文本缺失;当前 axes 不可作为机制事实。补抓光锥详情后重建。' ELSE '' END
FROM lightcones
WHERE id = $1`, id).Scan(&item.ID, &item.Version, &item.Rarity, &item.Path, &item.NameZH, &item.NameEN, &item.DescZH, &item.Axes, &item.DataQuality, &item.Warning)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// GetLightconeRefinements 返回光锥原始叠影效果模板和 1-5 叠影参数,
// 占位符与富文本标签由前端解析,Go 侧只透传 JSONB。
func (s *Service) GetLightconeRefinements(ctx context.Context, id int) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRow(ctx, `
SELECT coalesce(raw_zh->'refinements', 'null'::jsonb)
FROM lightcones WHERE id = $1`, id).Scan(&raw)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *Service) ListRelicSets(ctx context.Context, query string, kind string, limit int) ([]RelicSet, error) {
	if limit <= 0 {
		limit = 40
	}
	rows, err := s.db.Query(ctx, `
SELECT id, version, kind, name_zh, name_en, coalesce(set2_desc, ''), coalesce(set4_desc, ''), coalesce(raw_zh->>'icon', ''), axes
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
		var icon string
		if err := rows.Scan(&item.ID, &item.Version, &item.Kind, &item.NameZH, &item.NameEN, &item.Set2Desc, &item.Set4Desc, &icon, &item.Axes); err != nil {
			return nil, err
		}
		item.FigureURL = relicFigureURL(icon)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) GetRelicSet(ctx context.Context, id int) (*RelicSet, error) {
	var item RelicSet
	var icon string
	err := s.db.QueryRow(ctx, `
SELECT id, version, kind, name_zh, name_en, coalesce(set2_desc, ''), coalesce(set4_desc, ''), coalesce(raw_zh->>'icon', ''), axes
FROM relic_sets
WHERE id = $1`, id).Scan(&item.ID, &item.Version, &item.Kind, &item.NameZH, &item.NameEN, &item.Set2Desc, &item.Set4Desc, &icon, &item.Axes)
	if err != nil {
		return nil, err
	}
	item.FigureURL = relicFigureURL(icon)
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
			applyEntityLink(&item)
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
			applyEntityLink(&item)
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
			applyEntityLink(&item)
			out = append(out, item)
		}
		return out, rows.Err()
	default:
		return nil, fmt.Errorf("unknown keyword search kind %q", kind)
	}
}

func (s *Service) SemanticSearch(ctx context.Context, query string, kind string, limit int) ([]SemanticMatch, error) {
	return s.SemanticSearchAdvanced(ctx, query, kind, limit, SemanticSearchOptions{})
}

func (s *Service) SemanticSearchWithModel(ctx context.Context, query string, kind string, limit int, embeddingModelID string) ([]SemanticMatch, error) {
	return s.SemanticSearchAdvanced(ctx, query, kind, limit, SemanticSearchOptions{EmbeddingModelID: embeddingModelID})
}

type SemanticSearchOptions struct {
	EmbeddingModelID string
	RerankModelID    string
	DisableReranker  bool
	RerankTopN       int
	RecallLimit      int
}

func (s *Service) SemanticSearchAdvanced(ctx context.Context, query string, kind string, limit int, options SemanticSearchOptions) ([]SemanticMatch, error) {
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

	embedder, selectedModelID, err := s.embeddingClient(options.EmbeddingModelID)
	if err != nil {
		return nil, err
	}
	runtimeMeta := embedder.Metadata()
	searchKinds := []string{kind}
	if kind == "all" {
		searchKinds = []string{"character", "lightcone", "relic_set"}
	}
	for _, subKind := range searchKinds {
		if err := s.ensureEntityEmbeddingCoverage(ctx, subKind, selectedModelID, runtimeMeta); err != nil {
			return nil, err
		}
	}

	vec, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	if isZeroVector(vec) {
		return nil, fmt.Errorf("query produced an empty embedding")
	}
	vectorText := embedding.VectorLiteral(vec)
	recallLimit := semanticRecallLimit(limit, options)

	var out []SemanticMatch
	if kind == "all" {
		perKindLimit := recallLimit
		if perKindLimit > 30 {
			perKindLimit = 30
		}
		if perKindLimit < limit {
			perKindLimit = limit
		}
		for _, subKind := range searchKinds {
			rows, err := s.semanticSearchKind(ctx, vectorText, subKind, perKindLimit, runtimeMeta, selectedModelID)
			if err != nil {
				return nil, err
			}
			out = append(out, rows...)
		}
	} else {
		out, err = s.semanticSearchKind(ctx, vectorText, kind, recallLimit, runtimeMeta, selectedModelID)
		if err != nil {
			return nil, err
		}
	}
	for i := range out {
		out[i].EmbeddingModelID = selectedModelID
	}
	supplemental, err := s.semanticSupplementalRecall(ctx, query, searchKinds, recallLimit, runtimeMeta, selectedModelID)
	if err != nil {
		return nil, err
	}
	out = mergeSemanticCandidates(out, supplemental)
	rankLimit := limit
	if kind == "all" {
		rankLimit = len(out)
	}
	ranked, err := s.rankSemanticMatches(ctx, query, out, rankLimit, options)
	if err != nil {
		return nil, err
	}
	if kind == "all" {
		ranked = diversifySemanticMatches(query, ranked, limit)
	}
	return trimSemanticMatches(ranked, limit), nil
}

func (s *Service) semanticSupplementalRecall(ctx context.Context, query string, kinds []string, recallLimit int, meta embedding.Metadata, modelID string) ([]SemanticMatch, error) {
	terms := semanticKeywordTerms(query)
	if recallLimit <= 0 {
		recallLimit = 20
	}
	if recallLimit > 30 {
		recallLimit = 30
	}
	var out []SemanticMatch
	for _, kind := range kinds {
		rows, err := s.semanticKeywordRecallKind(ctx, query, kind, terms, recallLimit, meta, modelID)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	recommendations, err := s.semanticRecommendationRecall(ctx, query, kinds, recallLimit, meta, modelID)
	if err != nil {
		return nil, err
	}
	out = append(out, recommendations...)
	return out, nil
}

func (s *Service) semanticRecommendationRecall(ctx context.Context, query string, kinds []string, limit int, meta embedding.Metadata, modelID string) ([]SemanticMatch, error) {
	charIDs, err := s.mentionedCharacterIDs(ctx, query, 3)
	if err != nil {
		return nil, err
	}
	if len(charIDs) == 0 {
		return nil, nil
	}
	kindSet := map[string]bool{}
	for _, kind := range kinds {
		kindSet[kind] = true
	}
	var out []SemanticMatch
	if kindSet["lightcone"] {
		rows, err := s.recommendedLightconeRecall(ctx, charIDs, limit, meta, modelID)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	if kindSet["relic_set"] {
		rows, err := s.recommendedRelicRecall(ctx, charIDs, limit, meta, modelID)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func (s *Service) mentionedCharacterIDs(ctx context.Context, query string, limit int) ([]int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}
	normalized := normalizeSearchToken(query)
	rows, err := s.db.Query(ctx, `
WITH candidates AS (
    SELECT id, name_zh, name_en, rarity,
           lower(replace(replace(replace(name_zh, '•', ''), '·', ''), ' ', '')) AS name_zh_norm,
           lower(replace(replace(replace(name_en, '•', ''), '·', ''), ' ', '')) AS name_en_norm
    FROM characters
),
scored AS (
    SELECT id, rarity,
           (position(name_zh_norm in $2) > 0 OR position(name_en_norm in $2) > 0 OR id::text = $1) AS mentioned,
           greatest(similarity(name_zh, $1), similarity(name_en, $1), similarity(name_zh_norm, $2), similarity(name_en_norm, $2))::float8 AS score
    FROM candidates
)
SELECT id
FROM scored
WHERE mentioned OR score >= 0.28
ORDER BY mentioned DESC, score DESC, rarity DESC, id
LIMIT $3`, query, normalized, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Service) recommendedLightconeRecall(ctx context.Context, charIDs []int, limit int, meta embedding.Metadata, modelID string) ([]SemanticMatch, error) {
	rows, err := s.db.Query(ctx, `
SELECT DISTINCT ON (l.id)
       l.id, l.name_zh, l.rarity, l.path,
       greatest(0.35, 0.95 - cr.rank * 0.04)::float8 AS score,
       concat_ws(E'\n', l.name_zh, l.name_en, l.path, coalesce(l.desc_zh, ''), l.axes::text) AS candidate_text
FROM character_recommendations cr
JOIN lightcones l ON l.id = cr.item_id
WHERE cr.char_id = ANY($1::int[]) AND cr.recommend_kind = 'lightcone'
ORDER BY l.id, cr.rank
LIMIT $2`, charIDs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SemanticMatch
	for rows.Next() {
		var item SemanticMatch
		item.Kind = "lightcone"
		if err := rows.Scan(&item.ID, &item.NameZH, &item.Rarity, &item.Path, &item.Score, &item.CandidateText); err != nil {
			return nil, err
		}
		item.RecallScore = item.Score
		item.RecallSource = "recommendation"
		applySemanticMetadata(&item, meta)
		item.EmbeddingModelID = modelID
		applyEntityLink(&item)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) recommendedRelicRecall(ctx context.Context, charIDs []int, limit int, meta embedding.Metadata, modelID string) ([]SemanticMatch, error) {
	rows, err := s.db.Query(ctx, `
SELECT DISTINCT ON (r.id)
       r.id, r.name_zh, r.kind,
       greatest(0.35, 0.95 - cr.rank * 0.04)::float8 AS score,
       concat_ws(E'\n', r.name_zh, r.name_en, r.kind, coalesce(r.set2_desc, ''), coalesce(r.set4_desc, ''), r.axes::text) AS candidate_text
FROM character_recommendations cr
JOIN relic_sets r ON r.id = cr.item_id
WHERE cr.char_id = ANY($1::int[]) AND cr.recommend_kind IN ('relic_set4', 'relic_set2')
ORDER BY r.id, cr.rank
LIMIT $2`, charIDs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SemanticMatch
	for rows.Next() {
		var item SemanticMatch
		item.Kind = "relic_set"
		if err := rows.Scan(&item.ID, &item.NameZH, &item.Path, &item.Score, &item.CandidateText); err != nil {
			return nil, err
		}
		item.RecallScore = item.Score
		item.RecallSource = "recommendation"
		applySemanticMetadata(&item, meta)
		item.EmbeddingModelID = modelID
		applyEntityLink(&item)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) semanticKeywordRecallKind(ctx context.Context, query string, kind string, terms []string, limit int, meta embedding.Metadata, modelID string) ([]SemanticMatch, error) {
	query = strings.TrimSpace(query)
	normalizedQuery := normalizeSearchToken(applySearchSynonyms(query))
	if terms == nil {
		terms = []string{}
	}
	switch kind {
	case "character":
		rows, err := s.db.Query(ctx, `
WITH candidates AS (
    SELECT c.id, c.name_zh, c.name_en, c.rarity, c.path, c.element, c.roles,
           concat_ws(E'\n', c.name_zh, c.name_en, c.path, c.element, array_to_string(c.roles, ','), c.axes::text, left(c.skill_text_zh, 1200)) AS candidate_text,
           lower(concat_ws(E'\n', c.name_zh, c.name_en, c.path, c.element, array_to_string(c.roles, ','), c.axes::text, left(c.skill_text_zh, 1200))) AS candidate_text_lower,
           lower(replace(replace(replace(c.name_zh, '•', ''), '·', ''), ' ', '')) AS name_zh_norm,
           lower(replace(replace(replace(c.name_en, '•', ''), '·', ''), ' ', '')) AS name_en_norm
    FROM characters c
),
scored AS (
    SELECT *,
           (id::text = $1 OR name_zh = $1 OR lower(name_en) = lower($1)) AS exact_match,
           (char_length($2) >= 2 AND (
               position(name_zh_norm in $2) > 0 OR position($2 in name_zh_norm) > 0 OR
               position(name_en_norm in $2) > 0 OR position($2 in name_en_norm) > 0
           )) AS name_mentioned,
           greatest(similarity(name_zh, $1), similarity(name_en, $1), similarity(name_zh_norm, $2), similarity(name_en_norm, $2))::float8 AS name_score,
           (
               SELECT count(*)::int
               FROM unnest($3::text[]) term
               WHERE term <> '' AND candidate_text_lower LIKE '%' || lower(term) || '%'
           ) AS keyword_hits
    FROM candidates
)
SELECT id, name_zh, rarity, path, element, roles,
       greatest(
           CASE WHEN exact_match THEN 1.0 WHEN name_mentioned THEN 0.92 ELSE 0 END,
           least(0.85, name_score * 0.75),
           CASE WHEN keyword_hits > 0 THEN least(0.82, 0.35 + keyword_hits * 0.07) ELSE 0 END
       )::float8 AS score,
       candidate_text
FROM scored
WHERE exact_match OR name_mentioned OR name_score >= 0.20 OR keyword_hits > 0
ORDER BY exact_match DESC, name_mentioned DESC, score DESC, rarity DESC, id
LIMIT $4`, query, normalizedQuery, terms, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "character"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Rarity, &item.Path, &item.Element, &item.Roles, &item.Score, &item.CandidateText); err != nil {
				return nil, err
			}
			item.RecallScore = item.Score
			item.RecallSource = "keyword"
			applySemanticMetadata(&item, meta)
			item.EmbeddingModelID = modelID
			applyEntityLink(&item)
			out = append(out, item)
		}
		return out, rows.Err()
	case "lightcone":
		rows, err := s.db.Query(ctx, `
WITH candidates AS (
    SELECT l.id, l.name_zh, l.name_en, l.rarity, l.path,
           concat_ws(E'\n', l.name_zh, l.name_en, l.path, coalesce(l.desc_zh, ''), l.axes::text) AS candidate_text,
           lower(concat_ws(E'\n', l.name_zh, l.name_en, l.path, coalesce(l.desc_zh, ''), l.axes::text)) AS candidate_text_lower,
           lower(replace(replace(replace(l.name_zh, '•', ''), '·', ''), ' ', '')) AS name_zh_norm,
           lower(replace(replace(replace(l.name_en, '•', ''), '·', ''), ' ', '')) AS name_en_norm
    FROM lightcones l
),
scored AS (
    SELECT *,
           (id::text = $1 OR name_zh = $1 OR lower(name_en) = lower($1)) AS exact_match,
           (char_length($2) >= 2 AND (
               position(name_zh_norm in $2) > 0 OR position($2 in name_zh_norm) > 0 OR
               position(name_en_norm in $2) > 0 OR position($2 in name_en_norm) > 0
           )) AS name_mentioned,
           greatest(similarity(name_zh, $1), similarity(name_en, $1), similarity(name_zh_norm, $2), similarity(name_en_norm, $2))::float8 AS name_score,
           (
               SELECT count(*)::int
               FROM unnest($3::text[]) term
               WHERE term <> '' AND candidate_text_lower LIKE '%' || lower(term) || '%'
           ) AS keyword_hits
    FROM candidates
)
SELECT id, name_zh, rarity, path,
       greatest(
           CASE WHEN exact_match THEN 1.0 WHEN name_mentioned THEN 0.92 ELSE 0 END,
           least(0.85, name_score * 0.75),
           CASE WHEN keyword_hits > 0 THEN least(0.82, 0.35 + keyword_hits * 0.07) ELSE 0 END
       )::float8 AS score,
       candidate_text
FROM scored
WHERE exact_match OR name_mentioned OR name_score >= 0.20 OR keyword_hits > 0
ORDER BY exact_match DESC, name_mentioned DESC, score DESC, rarity DESC, id
LIMIT $4`, query, normalizedQuery, terms, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "lightcone"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Rarity, &item.Path, &item.Score, &item.CandidateText); err != nil {
				return nil, err
			}
			item.RecallScore = item.Score
			item.RecallSource = "keyword"
			applySemanticMetadata(&item, meta)
			item.EmbeddingModelID = modelID
			applyEntityLink(&item)
			out = append(out, item)
		}
		return out, rows.Err()
	case "relic_set":
		rows, err := s.db.Query(ctx, `
WITH candidates AS (
    SELECT r.id, r.name_zh, r.name_en, r.kind,
           concat_ws(E'\n', r.name_zh, r.name_en, r.kind, coalesce(r.set2_desc, ''), coalesce(r.set4_desc, ''), r.axes::text) AS candidate_text,
           lower(concat_ws(E'\n', r.name_zh, r.name_en, r.kind, coalesce(r.set2_desc, ''), coalesce(r.set4_desc, ''), r.axes::text)) AS candidate_text_lower,
           lower(replace(replace(replace(r.name_zh, '•', ''), '·', ''), ' ', '')) AS name_zh_norm,
           lower(replace(replace(replace(r.name_en, '•', ''), '·', ''), ' ', '')) AS name_en_norm
    FROM relic_sets r
),
scored AS (
    SELECT *,
           (id::text = $1 OR name_zh = $1 OR lower(name_en) = lower($1)) AS exact_match,
           (char_length($2) >= 2 AND (
               position(name_zh_norm in $2) > 0 OR position($2 in name_zh_norm) > 0 OR
               position(name_en_norm in $2) > 0 OR position($2 in name_en_norm) > 0
           )) AS name_mentioned,
           greatest(similarity(name_zh, $1), similarity(name_en, $1), similarity(name_zh_norm, $2), similarity(name_en_norm, $2))::float8 AS name_score,
           (
               SELECT count(*)::int
               FROM unnest($3::text[]) term
               WHERE term <> '' AND candidate_text_lower LIKE '%' || lower(term) || '%'
           ) AS keyword_hits
    FROM candidates
)
SELECT id, name_zh, kind,
       greatest(
           CASE WHEN exact_match THEN 1.0 WHEN name_mentioned THEN 0.92 ELSE 0 END,
           least(0.85, name_score * 0.75),
           CASE WHEN keyword_hits > 0 THEN least(0.82, 0.35 + keyword_hits * 0.07) ELSE 0 END
       )::float8 AS score,
       candidate_text
FROM scored
WHERE exact_match OR name_mentioned OR name_score >= 0.20 OR keyword_hits > 0
ORDER BY exact_match DESC, name_mentioned DESC, score DESC, id
LIMIT $4`, query, normalizedQuery, terms, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "relic_set"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Path, &item.Score, &item.CandidateText); err != nil {
				return nil, err
			}
			item.RecallScore = item.Score
			item.RecallSource = "keyword"
			applySemanticMetadata(&item, meta)
			item.EmbeddingModelID = modelID
			applyEntityLink(&item)
			out = append(out, item)
		}
		return out, rows.Err()
	default:
		return nil, fmt.Errorf("unknown semantic search kind %q", kind)
	}
}

func semanticRecallLimit(limit int, options SemanticSearchOptions) int {
	recallLimit := options.RecallLimit
	if recallLimit <= 0 {
		recallLimit = options.RerankTopN
	}
	if recallLimit <= 0 {
		recallLimit = 50
	}
	if recallLimit < limit {
		recallLimit = limit
	}
	if recallLimit > 100 {
		recallLimit = 100
	}
	return recallLimit
}

func mergeSemanticCandidates(primary []SemanticMatch, supplemental []SemanticMatch) []SemanticMatch {
	out := make([]SemanticMatch, 0, len(primary)+len(supplemental))
	index := make(map[string]int, len(primary)+len(supplemental))
	add := func(item SemanticMatch) {
		key := semanticCandidateKey(item)
		if key == "" {
			return
		}
		if item.RecallSource == "" {
			item.RecallSource = "embedding"
		}
		if pos, ok := index[key]; ok {
			existing := &out[pos]
			if item.Score > existing.Score {
				existing.Score = item.Score
			}
			if item.RecallScore > existing.RecallScore {
				existing.RecallScore = item.RecallScore
			}
			if existing.CandidateText == "" && item.CandidateText != "" {
				existing.CandidateText = item.CandidateText
			}
			if existing.URL == "" && item.URL != "" {
				existing.URL = item.URL
			}
			if existing.Markdown == "" && item.Markdown != "" {
				existing.Markdown = item.Markdown
			}
			existing.RecallSource = mergeSemanticRecallSource(existing.RecallSource, item.RecallSource)
			return
		}
		index[key] = len(out)
		out = append(out, item)
	}
	for _, item := range primary {
		add(item)
	}
	for _, item := range supplemental {
		add(item)
	}
	return out
}

func semanticCandidateKey(item SemanticMatch) string {
	if item.Kind == "" || item.ID == 0 {
		return ""
	}
	return item.Kind + ":" + strconv.Itoa(item.ID)
}

func mergeSemanticRecallSource(left string, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return right
	}
	if right == "" || left == right {
		return left
	}
	for _, part := range strings.Split(left, "+") {
		if part == right {
			return left
		}
	}
	return left + "+" + right
}

func semanticKeywordTerms(query string) []string {
	normalized := applySearchSynonyms(query)
	var terms []string
	for _, field := range strings.FieldsFunc(normalized, isSearchSeparator) {
		field = sanitizeSearchTerm(field)
		if searchTermUseful(field) {
			terms = appendUniqueSearchTerm(terms, field)
		}
	}
	searchable := normalizeSearchToken(normalized)
	for _, term := range semanticDomainTerms() {
		if strings.Contains(searchable, normalizeSearchToken(term)) {
			terms = appendUniqueSearchTerm(terms, term)
		}
	}
	if len(terms) > 32 {
		return terms[:32]
	}
	return terms
}

func semanticDomainTerms() []string {
	return []string{
		"击破", "超击破", "击破特攻", "弱点击破", "削韧", "break", "super_break",
		"追击", "追加攻击", "fua",
		"持续伤害", "dot", "灼烧", "触电", "风化", "裂伤",
		"暴击", "暴击率", "暴伤", "暴击伤害", "双暴", "crit",
		"速度", "拉条", "行动提前", "提前行动", "speed", "advance",
		"治疗", "回复", "生命回复", "heal",
		"护盾", "盾量", "shield",
		"减防", "无视防御", "防御无视", "减抗", "抗性穿透", "穿透", "易伤", "负面", "debuff",
		"能量", "回能", "充能", "终结技", "energy",
		"战技点", "产点", "耗点", "sp",
		"召唤物", "记忆精灵",
		"攻击", "攻击力", "生命", "生命值", "防御", "防御力", "效果命中", "效果抵抗",
		"毁灭", "巡猎", "智识", "同谐", "虚无", "存护", "丰饶", "记忆",
		"物理", "火", "冰", "雷", "风", "量子", "虚数",
		"光锥", "专武", "叠影", "遗器", "套装", "位面", "内圈", "外圈",
		"开拓者", "主角",
	}
}

func applySearchSynonyms(text string) string {
	replacer := strings.NewReplacer(
		"同协", "同谐",
		"阮梅", "阮•梅",
		"防御无视", "无视防御",
		"爆伤", "暴伤",
	)
	return replacer.Replace(strings.TrimSpace(text))
}

func normalizeSearchToken(text string) string {
	text = strings.ToLower(applySearchSynonyms(text))
	replacer := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		"•", "",
		"·", "",
		"-", "",
		"_", "",
		"'", "",
		"\"", "",
		"“", "",
		"”", "",
		"‘", "",
		"’", "",
		"(", "",
		")", "",
		"（", "",
		"）", "",
	)
	return replacer.Replace(text)
}

func sanitizeSearchTerm(term string) string {
	term = strings.TrimSpace(term)
	term = strings.Trim(term, ".,，。:：;；!?！？()（）[]【】{}<>《》\"'“”‘’")
	if strings.ContainsAny(term, "%_") {
		return ""
	}
	return term
}

func searchTermUseful(term string) bool {
	if term == "" {
		return false
	}
	switch term {
	case "火", "冰", "雷", "风":
		return true
	}
	runeCount := len([]rune(term))
	if runeCount < 2 && !strings.EqualFold(term, "sp") {
		return false
	}
	switch strings.ToLower(term) {
	case "哪个", "是否", "适合", "收益", "平替", "多少", "需要", "如果", "那么", "这个", "那个":
		return false
	default:
		return true
	}
}

func appendUniqueSearchTerm(terms []string, term string) []string {
	term = sanitizeSearchTerm(term)
	if !searchTermUseful(term) {
		return terms
	}
	normalized := normalizeSearchToken(term)
	for _, existing := range terms {
		if normalizeSearchToken(existing) == normalized {
			return terms
		}
	}
	return append(terms, term)
}

func isSearchSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', ',', '，', '.', '。', ':', '：', ';', '；', '?', '？', '!', '！', '/', '\\', '|', '+':
		return true
	default:
		return false
	}
}

func (s *Service) embeddingClient(modelID string) (*embedding.Client, string, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		modelID = s.defaultEmbeddingID
	}
	if modelID != "" && s.embedders != nil {
		client := s.embedders[modelID]
		if client == nil {
			return nil, modelID, fmt.Errorf("unknown embedding model id %q", modelID)
		}
		return client, modelID, nil
	}
	if s.embedder == nil {
		return nil, modelID, fmt.Errorf("embedding client is not configured")
	}
	return s.embedder, modelID, nil
}

func (s *Service) rerankerClient(modelID string) (*rerank.Client, string, bool, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		modelID = s.defaultRerankID
	}
	if modelID == "" || s.rerankers == nil {
		return nil, modelID, false, nil
	}
	client := s.rerankers[modelID]
	if client == nil {
		return nil, modelID, false, fmt.Errorf("unknown rerank model id %q", modelID)
	}
	if !client.Enabled() {
		return client, modelID, false, nil
	}
	return client, modelID, true, nil
}

func (s *Service) rankSemanticMatches(ctx context.Context, query string, rows []SemanticMatch, limit int, options SemanticSearchOptions) ([]SemanticMatch, error) {
	if len(rows) == 0 {
		return rows, nil
	}
	applyLocalSemanticRanking(query, rows, "")

	if !options.DisableReranker {
		client, modelID, enabled, err := s.rerankerClient(options.RerankModelID)
		if err != nil {
			applyLocalSemanticRanking(query, rows, "reranker not used: "+err.Error())
			return trimSemanticMatches(rows, limit), nil
		}
		if enabled {
			topN := options.RerankTopN
			if topN <= 0 {
				topN = s.rerankTopN
			}
			if topN <= 0 {
				topN = 50
			}
			if topN > maxRerankDocuments {
				topN = maxRerankDocuments
			}
			if topN > len(rows) {
				topN = len(rows)
			}
			documents := make([]string, topN)
			for i := 0; i < topN; i++ {
				documents[i] = compactRerankDocument(rows[i].CandidateText, 1200)
				if strings.TrimSpace(documents[i]) == "" {
					documents[i] = compactRerankDocument(semanticFallbackDocument(rows[i]), 1200)
				}
			}
			results, err := client.Rerank(ctx, query, documents, len(documents))
			if err != nil {
				applyLocalSemanticRanking(query, rows, "reranker unavailable: "+err.Error())
				return trimSemanticMatches(rows, limit), nil
			}
			meta := client.Metadata()
			for _, result := range results {
				if result.Index < 0 || result.Index >= topN {
					continue
				}
				score := result.Score
				rows[result.Index].RerankScore = &score
				rows[result.Index].RerankModelID = modelID
				rows[result.Index].RerankModel = meta.Model
				rows[result.Index].RerankProvider = meta.Provider
				rows[result.Index].FinalScore = score*0.75 + rows[result.Index].RecallScore*0.20 + rows[result.Index].RuleScore*0.05
				rows[result.Index].Score = rows[result.Index].FinalScore
				rows[result.Index].ScoreExplain = fmt.Sprintf(
					"final_score=0.75*rerank_score+0.20*recall_score+0.05*rule_score; recall_source=%s rerank_score=%.4f recall_score=%.4f rule_score=%.4f",
					fallbackRecallSource(rows[result.Index].RecallSource),
					score,
					rows[result.Index].RecallScore,
					rows[result.Index].RuleScore,
				)
			}
			for i := topN; i < len(rows); i++ {
				rows[i].Score *= 0.25
				rows[i].FinalScore = rows[i].Score
				rows[i].ScoreExplain += "; not sent to reranker"
			}
			sort.SliceStable(rows, func(i, j int) bool {
				return rows[i].Score > rows[j].Score
			})
			return trimSemanticMatches(rows, limit), nil
		}
	}

	applyLocalSemanticRanking(query, rows, "reranker disabled or not configured")
	return trimSemanticMatches(rows, limit), nil
}

func applyLocalSemanticRanking(query string, rows []SemanticMatch, note string) {
	for i := range rows {
		if rows[i].RecallScore == 0 {
			rows[i].RecallScore = rows[i].Score
		}
		rows[i].RuleScore = semanticRuleScore(query, rows[i])
		rows[i].FinalScore = rows[i].RecallScore*0.85 + rows[i].RuleScore*0.15
		rows[i].Score = rows[i].FinalScore
		rows[i].ScoreExplain = fmt.Sprintf(
			"final_score=0.85*recall_score+0.15*rule_score; recall_source=%s recall_score=%.4f rule_score=%.4f",
			fallbackRecallSource(rows[i].RecallSource),
			rows[i].RecallScore,
			rows[i].RuleScore,
		)
		if note != "" {
			rows[i].ScoreExplain += "; " + note
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Score > rows[j].Score
	})
}

func fallbackRecallSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "embedding"
	}
	return source
}

func semanticRuleScore(query string, item SemanticMatch) float64 {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	document := strings.ToLower(item.CandidateText + "\n" + semanticFallbackDocument(item))
	score := 0.0

	if item.Kind == "relic_set" && containsAny(normalizedQuery, "遗器", "套装", "位面", "内圈", "外圈", "铁骑", "钟表匠") {
		score += 0.35
	}
	if item.Kind == "lightcone" && containsAny(normalizedQuery, "光锥", "专武", "叠影", "武器") {
		score += 0.35
	}
	if item.Kind == "character" && containsAny(normalizedQuery, "角色", "队友", "配队", "辅助", "主c", "主C", "奶", "盾") {
		score += 0.18
	}
	if item.Kind == "character" && containsAny(normalizedQuery, "火", "火属性") && strings.EqualFold(item.Element, "Fire") {
		score += 0.12
	}
	if item.Kind == "character" && containsAny(normalizedQuery, "冰", "冰属性") && strings.EqualFold(item.Element, "Ice") {
		score += 0.12
	}
	if item.Kind == "character" && containsAny(normalizedQuery, "量子") && strings.EqualFold(item.Element, "Quantum") {
		score += 0.12
	}
	if item.Kind == "character" && containsAny(normalizedQuery, "虚数") && strings.EqualFold(item.Element, "Imaginary") {
		score += 0.12
	}
	if item.Kind == "character" && containsAny(normalizedQuery, "主c", "主C", "输出") && hasAnyRole(item.Roles, "main_dps") {
		score += 0.12
	}
	if item.Kind == "character" && containsAny(normalizedQuery, "队友", "配队", "辅助") && hasAnyRole(item.Roles, "amplifier", "debuffer") {
		score += 0.12
	}
	if item.Kind == "character" && containsAny(normalizedQuery, "击破", "超击破", "削韧") && hasAnyRole(item.Roles, "break_specialist") {
		score += 0.18
	}
	for _, group := range [][]string{
		{"击破", "超击破", "削韧", "break", "super_break"},
		{"追击", "追加攻击", "fua"},
		{"持续伤害", "dot", "灼烧", "触电", "风化", "裂伤"},
		{"暴击", "暴伤", "双暴", "crit"},
		{"速度", "拉条", "行动提前", "speed", "advance"},
		{"治疗", "回复", "奶", "heal"},
		{"护盾", "盾", "shield"},
		{"减防", "减抗", "易伤", "负面", "debuff"},
		{"能量", "回能", "充能", "energy"},
	} {
		if containsAny(normalizedQuery, group...) && containsAny(document, group...) {
			score += 0.12
		}
	}
	if containsAny(document, strings.Fields(normalizedQuery)...) {
		score += 0.05
	}
	if score > 1 {
		return 1
	}
	return score
}

func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func semanticFallbackDocument(item SemanticMatch) string {
	parts := []string{item.Kind, item.NameZH, item.Path, item.Element, strings.Join(item.Roles, " ")}
	return strings.Join(parts, "\n")
}

func compactRerankDocument(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(text), " ")
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

func trimSemanticMatches(rows []SemanticMatch, limit int) []SemanticMatch {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func diversifySemanticMatches(query string, rows []SemanticMatch, limit int) []SemanticMatch {
	desired := desiredSemanticKinds(query)
	if len(desired) <= 1 || len(rows) <= limit {
		return rows
	}
	desiredSet := map[string]bool{}
	for _, kind := range desired {
		desiredSet[kind] = true
	}
	selectedDesired := map[string]bool{}
	used := map[string]bool{}
	var out []SemanticMatch
	for _, row := range rows {
		key := row.Kind + ":" + strconv.Itoa(row.ID)
		if used[key] {
			continue
		}
		remainingSlots := limit - len(out)
		missing := 0
		for _, kind := range desired {
			if !selectedDesired[kind] {
				missing++
			}
		}
		if remainingSlots <= missing && (!desiredSet[row.Kind] || selectedDesired[row.Kind]) {
			continue
		}
		out = append(out, row)
		used[key] = true
		if desiredSet[row.Kind] {
			selectedDesired[row.Kind] = true
		}
		if len(out) >= limit {
			break
		}
	}
	for _, kind := range desired {
		if selectedDesired[kind] {
			continue
		}
		for _, row := range rows {
			key := row.Kind + ":" + strconv.Itoa(row.ID)
			if row.Kind == kind && !used[key] {
				out = append(out, row)
				used[key] = true
				selectedDesired[kind] = true
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return trimSemanticMatches(out, limit)
}

func desiredSemanticKinds(query string) []string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	var out []string
	if containsAny(normalized, "角色", "队友", "配队", "主c", "主C", "辅助", "奶", "盾") {
		out = append(out, "character")
	}
	if containsAny(normalized, "光锥", "专武", "叠影", "武器") {
		out = append(out, "lightcone")
	}
	if containsAny(normalized, "遗器", "套装", "位面", "内圈", "外圈") {
		out = append(out, "relic_set")
	}
	return out
}

func (s *Service) ensureEntityEmbeddingCoverage(ctx context.Context, kind string, modelID string, runtime embedding.Metadata) error {
	if strings.TrimSpace(modelID) == "" {
		return fmt.Errorf("embedding_model_id is required for semantic search")
	}
	expected, err := s.entityKindCount(ctx, kind)
	if err != nil {
		return err
	}
	var rows int
	err = s.db.QueryRow(ctx, `
SELECT count(*)::int
FROM entity_embeddings
WHERE entity_kind = $1
  AND embedding_model_id = $2
  AND provider = $3
  AND model = $4
  AND storage_dimensions = $5
  AND quality = $6`, kind, modelID, runtime.Provider, runtime.Model, runtime.Dimensions, runtime.Quality).Scan(&rows)
	if err != nil {
		return fmt.Errorf("entity_embeddings coverage check failed for %s/%s: %w", modelID, kind, err)
	}
	if rows == 0 {
		return fmt.Errorf("entity embeddings for %s/%s are missing; run scripts/embed.py --model-id %s --kind %s", modelID, kind, modelID, kind)
	}
	if expected > 0 && rows < expected {
		return fmt.Errorf("entity embeddings for %s/%s are incomplete: rows=%d expected=%d; rerun scripts/embed.py --model-id %s --kind %s --resume", modelID, kind, rows, expected, modelID, kind)
	}
	return nil
}

func (s *Service) entityKindCount(ctx context.Context, kind string) (int, error) {
	var rows int
	var sql string
	switch kind {
	case "character":
		sql = `SELECT count(*)::int FROM characters`
	case "lightcone":
		sql = `SELECT count(*)::int FROM lightcones`
	case "relic_set":
		sql = `SELECT count(*)::int FROM relic_sets`
	default:
		return 0, fmt.Errorf("unknown semantic search kind %q", kind)
	}
	if err := s.db.QueryRow(ctx, sql).Scan(&rows); err != nil {
		return 0, err
	}
	return rows, nil
}

func (s *Service) semanticSearchKind(ctx context.Context, vectorText string, kind string, limit int, meta embedding.Metadata, modelID string) ([]SemanticMatch, error) {
	switch kind {
	case "character":
		rows, err := s.db.Query(ctx, `
SELECT c.id, c.name_zh, c.rarity, c.path, c.element, c.roles,
       1 - (ee.embedding <=> $1::vector) AS score,
       concat_ws(E'\n', c.name_zh, c.name_en, c.path, c.element, array_to_string(c.roles, ','), c.axes::text, left(c.skill_text_zh, 1200)) AS candidate_text
FROM entity_embeddings ee
JOIN characters c ON c.id = ee.entity_id
WHERE ee.entity_kind = 'character'
  AND ee.embedding_model_id = $2
  AND ee.provider = $3
  AND ee.model = $4
  AND ee.storage_dimensions = $5
  AND ee.quality = $6
ORDER BY ee.embedding <=> $1::vector
LIMIT $7`, vectorText, modelID, meta.Provider, meta.Model, meta.Dimensions, meta.Quality, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "character"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Rarity, &item.Path, &item.Element, &item.Roles, &item.Score, &item.CandidateText); err != nil {
				return nil, err
			}
			item.RecallScore = item.Score
			item.RecallSource = "embedding"
			applySemanticMetadata(&item, meta)
			applyEntityLink(&item)
			out = append(out, item)
		}
		return out, rows.Err()
	case "lightcone":
		rows, err := s.db.Query(ctx, `
SELECT l.id, l.name_zh, l.rarity, l.path,
       1 - (ee.embedding <=> $1::vector) AS score,
       concat_ws(E'\n', l.name_zh, l.name_en, l.path, coalesce(l.desc_zh, ''), l.axes::text) AS candidate_text
FROM entity_embeddings ee
JOIN lightcones l ON l.id = ee.entity_id
WHERE ee.entity_kind = 'lightcone'
  AND ee.embedding_model_id = $2
  AND ee.provider = $3
  AND ee.model = $4
  AND ee.storage_dimensions = $5
  AND ee.quality = $6
ORDER BY ee.embedding <=> $1::vector
LIMIT $7`, vectorText, modelID, meta.Provider, meta.Model, meta.Dimensions, meta.Quality, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "lightcone"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Rarity, &item.Path, &item.Score, &item.CandidateText); err != nil {
				return nil, err
			}
			item.RecallScore = item.Score
			item.RecallSource = "embedding"
			applySemanticMetadata(&item, meta)
			applyEntityLink(&item)
			out = append(out, item)
		}
		return out, rows.Err()
	case "relic_set":
		rows, err := s.db.Query(ctx, `
SELECT r.id, r.name_zh, r.kind,
       1 - (ee.embedding <=> $1::vector) AS score,
       concat_ws(E'\n', r.name_zh, r.name_en, r.kind, coalesce(r.set2_desc, ''), coalesce(r.set4_desc, ''), r.axes::text) AS candidate_text
FROM entity_embeddings ee
JOIN relic_sets r ON r.id = ee.entity_id
WHERE ee.entity_kind = 'relic_set'
  AND ee.embedding_model_id = $2
  AND ee.provider = $3
  AND ee.model = $4
  AND ee.storage_dimensions = $5
  AND ee.quality = $6
ORDER BY ee.embedding <=> $1::vector
LIMIT $7`, vectorText, modelID, meta.Provider, meta.Model, meta.Dimensions, meta.Quality, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var out []SemanticMatch
		for rows.Next() {
			var item SemanticMatch
			item.Kind = "relic_set"
			if err := rows.Scan(&item.ID, &item.NameZH, &item.Path, &item.Score, &item.CandidateText); err != nil {
				return nil, err
			}
			item.RecallScore = item.Score
			item.RecallSource = "embedding"
			applySemanticMetadata(&item, meta)
			applyEntityLink(&item)
			out = append(out, item)
		}
		return out, rows.Err()
	default:
		return nil, fmt.Errorf("unknown semantic search kind %q", kind)
	}
}

func applySemanticMetadata(item *SemanticMatch, meta embedding.Metadata) {
	item.EmbeddingProvider = meta.Provider
	item.EmbeddingModel = meta.Model
	item.EmbeddingDimensions = meta.Dimensions
	item.EmbeddingQuality = meta.Quality
	item.ScoreExplain = "cosine similarity: 1 - pgvector_distance"
}

func applyEntityLink(item *SemanticMatch) {
	if item == nil || item.ID == 0 || item.NameZH == "" {
		return
	}
	prefix := entityRoutePrefix(item.Kind)
	if prefix == "" {
		return
	}
	item.URL = fmt.Sprintf("%s/%d", prefix, item.ID)
	item.Markdown = fmt.Sprintf("[%s](%s)", item.NameZH, item.URL)
}

func entityRoutePrefix(kind string) string {
	switch kind {
	case "character":
		return "/characters"
	case "lightcone":
		return "/lightcones"
	case "relic_set":
		return "/relic-sets"
	default:
		return ""
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
           'axes', CASE
               WHEN coalesce(l.desc_zh, '') <> '' THEN coalesce(l.axes, '{}'::jsonb)
               ELSE jsonb_build_object(
                   'provides', '[]'::jsonb,
                   'needs', '[]'::jsonb,
                   'restricts', '[]'::jsonb,
                   'tags', '[]'::jsonb,
                   'notes', '光锥效果文本缺失;弱画像 axes 已从推荐接口隐藏。'
               )
           END,
           'data_quality', CASE
               WHEN coalesce(l.desc_zh, '') <> '' THEN 'effect_text_extracted'
               ELSE 'weak_profile_inferred'
           END,
           'basis', CASE
               WHEN coalesce(l.desc_zh, '') <> '' THEN 'nanoka_rank_plus_equipment_axes'
               ELSE 'nanoka_rank_plus_path_only_pending_lightcone_effects'
           END,
           'warning', CASE
               WHEN coalesce(l.desc_zh, '') = '' THEN '光锥效果文本缺失;当前只信任 nanoka 推荐排名和命途匹配,不使用弱画像 axes 作为机制依据。'
           END,
           'score', round((
               greatest(0, 100 - cr.rank * 8)
               + CASE WHEN l.path = cp.path THEN 20 ELSE -80 END
               + CASE WHEN coalesce(l.desc_zh, '') <> '' THEN (
                   coalesce(array_length(mp.stats, 1), 0) * 12
                   + coalesce(array_length(mr.stats, 1), 0) * 5
                   + coalesce(array_length(mt.tags, 1), 0) * 8
               ) ELSE 0 END
           )::numeric, 1),
           'matched_provides', to_jsonb(coalesce(mp.stats, '{}'::text[])),
           'matched_requirements', to_jsonb(coalesce(mr.stats, '{}'::text[])),
           'matched_tags', to_jsonb(coalesce(mt.tags, '{}'::text[])),
           'reasons', to_jsonb(array_remove(ARRAY[
               CASE WHEN l.path = cp.path THEN '光锥命途匹配' ELSE '光锥命途不匹配或缺失' END,
               CASE WHEN coalesce(l.desc_zh, '') = '' THEN '光锥效果文本缺失;仅使用 nanoka 推荐排名和命途匹配' END,
               CASE WHEN coalesce(l.desc_zh, '') <> '' AND coalesce(array_length(mp.stats, 1), 0) > 0 THEN '提供角色需求轴: ' || array_to_string(mp.stats, ', ') END,
               CASE WHEN coalesce(l.desc_zh, '') <> '' AND coalesce(array_length(mr.stats, 1), 0) > 0 THEN '触发需求匹配角色机制: ' || array_to_string(mr.stats, ', ') END,
               CASE WHEN coalesce(l.desc_zh, '') <> '' AND coalesce(array_length(mt.tags, 1), 0) > 0 THEN '标签匹配: ' || array_to_string(mt.tags, ', ') END
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
      AND coalesce(l.desc_zh, '') <> ''
) mp ON true
LEFT JOIN LATERAL (
    SELECT array_agg(DISTINCT ea.stat ORDER BY ea.stat) AS stats
    FROM equipment_axes ea
    JOIN char_fit_stats cfs ON cfs.stat = ea.stat
    WHERE ea.entity_kind = 'lightcone' AND ea.entity_id = l.id AND ea.kind = 'needs'
      AND coalesce(l.desc_zh, '') <> ''
) mr ON true
LEFT JOIN LATERAL (
    SELECT array_agg(DISTINCT ea.stat ORDER BY ea.stat) AS tags
    FROM equipment_axes ea
    JOIN char_tags ct ON ct.tag = ea.stat
    WHERE ea.entity_kind = 'lightcone' AND ea.entity_id = l.id AND ea.kind = 'tag'
      AND coalesce(l.desc_zh, '') <> ''
) mt ON true
WHERE cr.char_id = $1 AND cr.recommend_kind = 'lightcone'
ORDER BY (
    greatest(0, 100 - cr.rank * 8)
    + CASE WHEN l.path = cp.path THEN 20 ELSE -80 END
    + CASE WHEN coalesce(l.desc_zh, '') <> '' THEN (
        coalesce(array_length(mp.stats, 1), 0) * 12
        + coalesce(array_length(mr.stats, 1), 0) * 5
        + coalesce(array_length(mt.tags, 1), 0) * 8
    ) ELSE 0 END
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
		item.LocalURL = LocalAssetURL(item.LocalPath)
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
