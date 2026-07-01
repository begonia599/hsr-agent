package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hsr-agent-go/internal/agent"
	"hsr-agent-go/internal/calc"
	"hsr-agent-go/internal/config"
	"hsr-agent-go/internal/tools"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	cfg   config.Config
	db    *pgxpool.Pool
	tools *tools.Service
}

type errorBody struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type chatRequest struct {
	Message        string `json:"message"`
	ConversationID int64  `json:"conversation_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
}

type updateConversationRequest struct {
	Title string `json:"title"`
}

type resolveEntitiesRequest struct {
	Entities []tools.EntityResolveRequest `json:"entities"`
	Display  string                       `json:"display"`
}

type mechanicRequest struct {
	AttackerID               int      `json:"attacker_id"`
	SupportID                int      `json:"support_id"`
	SupportIDs               []int    `json:"support_ids"`
	CharID                   int      `json:"char_id"`
	AttackTag                string   `json:"attack_tag"`
	IncludeEidolons          bool     `json:"include_eidolons"`
	Eidolons                 []int    `json:"eidolons"`
	ActiveContexts           []string `json:"active_contexts"`
	InactiveContexts         []string `json:"inactive_contexts"`
	Element                  string   `json:"element"`
	EnemyCount               int      `json:"enemy_count"`
	BreakEffect              float64  `json:"break_effect"`
	BreakDamageBonus         float64  `json:"break_dmg_bonus"`
	SuperBreakBonus          float64  `json:"super_break_dmg_bonus"`
	ToughnessReduction       float64  `json:"toughness_reduction"`
	MaxToughness             float64  `json:"max_toughness"`
	SuperBreakBaseMultiplier float64  `json:"super_break_base_multiplier"`
	SuperBreakMultiplier     float64  `json:"super_break_multiplier"`
	EnemyResistance          float64  `json:"enemy_resistance"`
	DefReduction             float64  `json:"def_reduction"`
	DefIgnore                float64  `json:"def_ignore"`
	ResReduction             float64  `json:"res_reduction"`
	ResPen                   float64  `json:"res_pen"`
	Vulnerability            float64  `json:"vulnerability"`
	DamageReduction          float64  `json:"damage_reduction"`
	ScalingStat              string   `json:"scaling_stat"`
	BaseScalingStat          float64  `json:"base_scaling_stat"`
	AbilityMultiplier        float64  `json:"ability_multiplier"`
	FlatValue                float64  `json:"flat_value"`
	DurationTurns            float64  `json:"duration_turns"`
	CooldownTurns            float64  `json:"cooldown_turns"`
	CycleTurns               float64  `json:"cycle_turns"`
	StartDelayTurns          float64  `json:"start_delay_turns"`
}

func New(cfg config.Config, db *pgxpool.Pool, toolService *tools.Service) *Server {
	return &Server{cfg: cfg, db: db, tools: toolService}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if recovered := recover(); recovered != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprint(recovered))
		}
	}()
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
		s.handleAPI(w, r)
		return
	}
	if r.URL.Path == tools.AssetURLPrefix || strings.HasPrefix(r.URL.Path, tools.AssetURLPrefix+"/") {
		s.serveMedia(w, r)
		return
	}
	s.serveStatic(w, r)
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api"))
	if len(parts) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"name": "hsr-agent-api"})
		return
	}
	switch parts[0] {
	case "health":
		s.handleHealth(w, r)
	case "models":
		s.handleModels(w, r)
	case "characters":
		s.handleCharacters(w, r, parts[1:])
	case "lightcones":
		s.handleLightcones(w, r, parts[1:])
	case "relic-sets":
		s.handleRelicSets(w, r, parts[1:])
	case "search":
		s.handleSearch(w, r, parts[1:])
	case "agent":
		s.handleAgent(w, r, parts[1:])
	case "mechanics":
		s.handleMechanics(w, r, parts[1:])
	case "assets":
		s.handleAssets(w, r, parts[1:])
	case "entities":
		s.handleEntities(w, r, parts[1:])
	case "conversations":
		s.handleConversations(w, r, parts[1:])
	case "turns":
		s.handleTurns(w, r, parts[1:])
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown API route")
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	dbStatus := "unconfigured"
	data := map[string]any{}
	if s.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := s.db.Ping(ctx); err != nil {
			dbStatus = "unavailable"
		} else {
			dbStatus = "ok"
			var version string
			var characters int
			var lightcones int
			var relicSets int
			if err := s.db.QueryRow(ctx, `
SELECT
  coalesce(max(version), ''),
  (SELECT count(*) FROM characters),
  (SELECT count(*) FROM lightcones),
  (SELECT count(*) FROM relic_sets)
FROM characters`).Scan(&version, &characters, &lightcones, &relicSets); err == nil {
				data["version"] = version
				data["characters"] = characters
				data["lightcones"] = lightcones
				data["relic_sets"] = relicSets
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"database": map[string]any{
			"status": dbStatus,
		},
		"data": data,
		"llm": map[string]any{
			"configured": strings.TrimSpace(s.cfg.LLMAPIKey) != "",
			"model":      s.cfg.LLMModel,
			"format":     s.cfg.LLMAPIFormat,
		},
		"embedding": map[string]any{
			"provider":   s.cfg.EmbeddingProvider,
			"model":      s.cfg.EmbeddingModel,
			"dimensions": s.cfg.EmbeddingDimensions,
			"quality":    embeddingQuality(s.cfg.EmbeddingProvider),
		},
		"web": map[string]any{
			"root": s.cfg.WebRoot,
		},
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	totals, coverage := s.entityEmbeddingCoverage(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"embedding": map[string]any{
			"default_id": s.cfg.DefaultEmbeddingID,
			"query_cache": map[string]any{
				"ttl_seconds": s.cfg.EmbeddingCacheTTLSeconds,
				"max_entries": s.cfg.EmbeddingCacheMaxEntries,
				"enabled":     s.cfg.EmbeddingCacheTTLSeconds > 0 && s.cfg.EmbeddingCacheMaxEntries > 0,
			},
			"models": s.publicEmbeddingModels(totals, coverage),
		},
		"rerank": map[string]any{
			"default_id":    s.cfg.DefaultRerankID,
			"default_top_n": s.cfg.RerankTopN,
			"max_documents": 25,
			"models":        s.publicRerankModels(),
		},
	})
}

type entityEmbeddingCoverageKey struct {
	ModelID string
	Kind    string
}

type entityEmbeddingCoverageRow struct {
	Provider           string
	Model              string
	NativeDimensions   int
	StorageDimensions  int
	ProjectionStrategy string
	Quality            string
	Rows               int
}

func (s *Server) entityEmbeddingCoverage(ctx context.Context) (map[string]int, map[entityEmbeddingCoverageKey][]entityEmbeddingCoverageRow) {
	totals := map[string]int{
		"character": 0,
		"lightcone": 0,
		"relic_set": 0,
	}
	coverage := map[entityEmbeddingCoverageKey][]entityEmbeddingCoverageRow{}
	if s.db == nil {
		return totals, coverage
	}
	var characters int
	var lightcones int
	var relicSets int
	if err := s.db.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM characters),
  (SELECT count(*) FROM lightcones),
  (SELECT count(*) FROM relic_sets)`).Scan(&characters, &lightcones, &relicSets); err == nil {
		totals["character"] = characters
		totals["lightcone"] = lightcones
		totals["relic_set"] = relicSets
	}
	rows, err := s.db.Query(ctx, `
SELECT entity_kind, embedding_model_id, provider, model, native_dimensions, storage_dimensions, projection_strategy, quality, count(*)::int
FROM entity_embeddings
GROUP BY entity_kind, embedding_model_id, provider, model, native_dimensions, storage_dimensions, projection_strategy, quality`)
	if err != nil {
		return totals, coverage
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var modelID string
		var item entityEmbeddingCoverageRow
		if err := rows.Scan(
			&kind,
			&modelID,
			&item.Provider,
			&item.Model,
			&item.NativeDimensions,
			&item.StorageDimensions,
			&item.ProjectionStrategy,
			&item.Quality,
			&item.Rows,
		); err == nil {
			key := entityEmbeddingCoverageKey{ModelID: modelID, Kind: kind}
			coverage[key] = append(coverage[key], item)
		}
	}
	return totals, coverage
}

func (s *Server) publicEmbeddingModels(totals map[string]int, coverage map[entityEmbeddingCoverageKey][]entityEmbeddingCoverageRow) []map[string]any {
	models := s.cfg.EmbeddingModels
	if len(models) == 0 {
		models = []config.EmbeddingModel{{
			ID:                 "default",
			Label:              s.cfg.EmbeddingModel,
			Provider:           s.cfg.EmbeddingProvider,
			BaseURL:            s.cfg.EmbeddingBaseURL,
			APIKey:             s.cfg.EmbeddingAPIKey,
			Model:              s.cfg.EmbeddingModel,
			Dimensions:         s.cfg.EmbeddingDimensions,
			NativeDimensions:   s.cfg.EmbeddingDimensions,
			ProjectionStrategy: "none",
			EncodingFormat:     s.cfg.EmbeddingEncoding,
			ExtraHeaders:       s.cfg.EmbeddingHeaders,
		}}
	}
	out := make([]map[string]any, 0, len(models))
	for _, model := range models {
		quality := embeddingQuality(model.Provider)
		configured := embeddingConfigured(model.Provider, model.BaseURL, model.APIKey, model.Model)
		nativeDimensions := model.NativeDimensions
		if nativeDimensions <= 0 {
			nativeDimensions = model.Dimensions
		}
		projectionStrategy := model.ProjectionStrategy
		if strings.TrimSpace(projectionStrategy) == "" {
			projectionStrategy = "none"
			if nativeDimensions != model.Dimensions {
				projectionStrategy = fmt.Sprintf("truncate_%d", model.Dimensions)
			}
		}
		kinds := map[string]any{}
		ready := configured && quality == "semantic"
		for _, kind := range []string{"character", "lightcone", "relic_set"} {
			expected := totals[kind]
			row, ok := matchingEmbeddingCoverage(
				coverage[entityEmbeddingCoverageKey{ModelID: model.ID, Kind: kind}],
				model,
				nativeDimensions,
				projectionStrategy,
				quality,
			)
			kindReady := configured && quality == "semantic" && ok && expected > 0 && row.Rows == expected
			if !kindReady {
				ready = false
			}
			kinds[kind] = map[string]any{
				"ready":               kindReady,
				"rows":                row.Rows,
				"expected_rows":       expected,
				"provider":            row.Provider,
				"model":               row.Model,
				"native_dimensions":   fallbackInt(row.NativeDimensions, nativeDimensions),
				"storage_dimensions":  fallbackInt(row.StorageDimensions, model.Dimensions),
				"projection_strategy": fallback(row.ProjectionStrategy, projectionStrategy),
				"quality":             fallback(row.Quality, quality),
			}
		}
		out = append(out, map[string]any{
			"id":                  model.ID,
			"label":               fallback(model.Label, model.ID),
			"provider":            model.Provider,
			"model":               model.Model,
			"dimensions":          model.Dimensions,
			"native_dimensions":   nativeDimensions,
			"storage_dimensions":  model.Dimensions,
			"projection_strategy": projectionStrategy,
			"encoding_format":     model.EncodingFormat,
			"quality":             quality,
			"default":             model.ID == s.cfg.DefaultEmbeddingID,
			"configured":          configured,
			"ready":               ready,
			"selectable":          ready,
			"metadata":            kinds,
			"notes":               model.Notes,
		})
	}
	return out
}

func matchingEmbeddingCoverage(rows []entityEmbeddingCoverageRow, model config.EmbeddingModel, nativeDimensions int, projectionStrategy string, quality string) (entityEmbeddingCoverageRow, bool) {
	for _, row := range rows {
		if row.Provider == model.Provider &&
			row.Model == model.Model &&
			row.NativeDimensions == nativeDimensions &&
			row.StorageDimensions == model.Dimensions &&
			row.ProjectionStrategy == projectionStrategy &&
			row.Quality == quality {
			return row, true
		}
	}
	return entityEmbeddingCoverageRow{}, false
}

func (s *Server) publicRerankModels() []map[string]any {
	models := s.cfg.RerankModels
	if len(models) == 0 {
		models = []config.RerankModel{{
			ID:           "default",
			Label:        s.cfg.RerankModel,
			Provider:     s.cfg.RerankProvider,
			BaseURL:      s.cfg.RerankBaseURL,
			APIKey:       s.cfg.RerankAPIKey,
			Model:        s.cfg.RerankModel,
			ExtraHeaders: s.cfg.RerankHeaders,
		}}
	}
	out := make([]map[string]any, 0, len(models))
	for _, model := range models {
		configured := rerankConfigured(model.Provider, model.BaseURL, model.APIKey, model.Model)
		out = append(out, map[string]any{
			"id":             model.ID,
			"label":          fallback(model.Label, model.ID),
			"provider":       model.Provider,
			"model":          model.Model,
			"context_length": model.ContextLength,
			"max_documents":  25,
			"default":        model.ID == s.cfg.DefaultRerankID,
			"configured":     configured,
			"selectable":     configured,
			"notes":          model.Notes,
		})
	}
	return out
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request, parts []string) {
	if !s.requireTools(w) {
		return
	}
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "asset route requires kind and id")
		return
	}
	rows, err := s.tools.GetAssets(r.Context(), parts[0], parts[1], queryCSV(r, "variants"))
	writeResult(w, rows, err)
}

func (s *Server) handleEntities(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) != 1 || parts[0] != "resolve" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown entities route")
		return
	}
	if !s.requireTools(w) {
		return
	}
	if r.Method == http.MethodGet {
		name := r.URL.Query().Get("name")
		if strings.TrimSpace(name) == "" {
			name = r.URL.Query().Get("q")
		}
		rows, err := s.tools.ResolveEntities(r.Context(), []tools.EntityResolveRequest{{
			Name: name,
			Kind: r.URL.Query().Get("kind"),
		}}, r.URL.Query().Get("display"))
		writeResult(w, rows, err)
		return
	}
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	var req resolveEntitiesRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	rows, err := s.tools.ResolveEntities(r.Context(), req.Entities, req.Display)
	writeResult(w, rows, err)
}

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request, parts []string) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database is not configured")
		return
	}
	if len(parts) == 0 {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		rows, err := s.listConversations(r.Context(), r.URL.Query().Get("session_id"), queryInt(r, "limit", 20), queryInt(r, "offset", 0))
		writeResult(w, rows, err)
		return
	}
	id, ok := parsePositiveInt64(parts[0])
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "conversation id must be an integer")
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			row, err := s.getConversation(r.Context(), id)
			writeResult(w, row, err)
		case http.MethodPatch:
			var req updateConversationRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			err := s.updateConversationTitle(r.Context(), id, req.Title)
			writeResult(w, map[string]any{"id": id, "title": strings.TrimSpace(req.Title)}, err)
		case http.MethodDelete:
			err := s.deleteConversation(r.Context(), id)
			writeResult(w, map[string]any{"deleted": true}, err)
		default:
			allowMethod(w, r, http.MethodGet, http.MethodPatch, http.MethodDelete)
		}
		return
	}
	if len(parts) == 2 && parts[1] == "turns" {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		rows, err := s.conversationTurns(r.Context(), id)
		writeResult(w, rows, err)
		return
	}
	writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown conversation route")
}

func (s *Server) handleTurns(w http.ResponseWriter, r *http.Request, parts []string) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database is not configured")
		return
	}
	if len(parts) != 1 {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "turn route is required")
		return
	}
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	traceID := strings.TrimSpace(parts[0])
	if traceID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "trace_id is required")
		return
	}
	row, err := s.getTurn(r.Context(), traceID)
	writeResult(w, row, err)
}

func (s *Server) handleCharacters(w http.ResponseWriter, r *http.Request, parts []string) {
	if !s.requireTools(w) {
		return
	}
	if len(parts) == 0 {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		rows, err := s.tools.ListCharacters(
			r.Context(),
			r.URL.Query().Get("q"),
			r.URL.Query().Get("role"),
			r.URL.Query().Get("element"),
			r.URL.Query().Get("path"),
			queryInt(r, "rarity", 0),
			queryInt(r, "limit", 40),
		)
		writeResult(w, rows, err)
		return
	}
	id, ok := parsePositiveInt(parts[0])
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "character id must be an integer")
		return
	}
	if len(parts) == 1 {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		item, err := s.tools.GetCharacter(r.Context(), strconv.Itoa(id))
		writeResult(w, item, err)
		return
	}
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	switch parts[1] {
	case "assets":
		rows, err := s.tools.GetAssets(r.Context(), "character", strconv.Itoa(id), queryCSV(r, "variants"))
		writeResult(w, rows, err)
	case "needs":
		rows, err := s.tools.FindNeeds(r.Context(), id)
		writeResult(w, rows, err)
	case "synergies":
		rows, err := s.tools.FindSynergies(r.Context(), id, queryInt(r, "limit", 8))
		writeResult(w, rows, err)
	case "teams":
		rows, err := s.tools.SuggestTeam(r.Context(), id, queryInt(r, "slots", 4), queryIntCSV(r, "exclude"))
		writeResult(w, rows, err)
	case "lightcones":
		rows, err := s.tools.RecommendLightcones(r.Context(), id)
		writeResult(w, rows, err)
	case "relics":
		rows, err := s.tools.RecommendRelics(r.Context(), id)
		writeResult(w, rows, err)
	case "modifiers":
		rows, err := s.tools.ListCharacterModifiers(r.Context(), id, r.URL.Query().Get("stat_key"), r.URL.Query().Get("target_scope"), queryInt(r, "limit", 40))
		writeResult(w, rows, err)
	case "modifier-sources":
		rows, err := s.tools.ExplainModifierSources(r.Context(), id, queryInt(r, "limit", 12))
		writeResult(w, rows, err)
	case "skills":
		raw, err := s.tools.GetCharacterSkills(r.Context(), id)
		writeResult(w, raw, err)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown character route")
	}
}

func (s *Server) handleLightcones(w http.ResponseWriter, r *http.Request, parts []string) {
	if !s.requireTools(w) {
		return
	}
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	if len(parts) == 0 {
		rows, err := s.tools.ListLightcones(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("path"), queryInt(r, "rarity", 0), queryInt(r, "limit", 40))
		writeResult(w, rows, err)
		return
	}
	id, ok := parsePositiveInt(parts[0])
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "lightcone id must be an integer")
		return
	}
	if len(parts) == 1 {
		item, err := s.tools.GetLightcone(r.Context(), id)
		writeResult(w, item, err)
		return
	}
	switch parts[1] {
	case "refinements":
		raw, err := s.tools.GetLightconeRefinements(r.Context(), id)
		writeResult(w, raw, err)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown lightcone route")
	}
}

func (s *Server) handleRelicSets(w http.ResponseWriter, r *http.Request, parts []string) {
	if !s.requireTools(w) {
		return
	}
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	if len(parts) == 0 {
		rows, err := s.tools.ListRelicSets(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("kind"), queryInt(r, "limit", 40))
		writeResult(w, rows, err)
		return
	}
	id, ok := parsePositiveInt(parts[0])
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "relic set id must be an integer")
		return
	}
	item, err := s.tools.GetRelicSet(r.Context(), id)
	writeResult(w, item, err)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "search route is required")
		return
	}
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	switch parts[0] {
	case "keyword":
		if !s.requireTools(w) {
			return
		}
		rows, err := s.tools.KeywordSearch(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("kind"), queryInt(r, "limit", 10))
		if err == nil && wantsEnvelope(r) {
			writeJSON(w, http.StatusOK, map[string]any{
				"query": r.URL.Query().Get("q"),
				"kind":  fallback(r.URL.Query().Get("kind"), "character"),
				"limit": queryInt(r, "limit", 10),
				"count": len(rows),
				"items": rows,
			})
			return
		}
		writeResult(w, rows, err)
	case "semantic":
		if !semanticHTTPEnabled(s.cfg) {
			writeError(w, http.StatusServiceUnavailable, "SEMANTIC_SEARCH_DISABLED", "semantic search HTTP API requires a real embedding provider; use /api/search/keyword for now")
			return
		}
		if !s.requireTools(w) {
			return
		}
		options := tools.SemanticSearchOptions{
			EmbeddingModelID: r.URL.Query().Get("embedding_model_id"),
			RerankModelID:    r.URL.Query().Get("rerank_model_id"),
			DisableReranker:  !queryBool(r, "rerank", true),
			RerankTopN:       queryInt(r, "rerank_top_n", s.cfg.RerankTopN),
			RecallLimit:      queryInt(r, "recall_limit", 0),
		}
		rows, err := s.tools.SemanticSearchAdvanced(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("kind"), queryInt(r, "limit", 10), options)
		if err == nil && wantsEnvelope(r) {
			writeJSON(w, http.StatusOK, map[string]any{
				"query":              r.URL.Query().Get("q"),
				"kind":               fallback(r.URL.Query().Get("kind"), "character"),
				"limit":              queryInt(r, "limit", 10),
				"count":              len(rows),
				"embedding_model_id": fallback(options.EmbeddingModelID, s.cfg.DefaultEmbeddingID),
				"rerank_model_id":    fallback(options.RerankModelID, s.cfg.DefaultRerankID),
				"rerank":             !options.DisableReranker,
				"rerank_top_n":       queryInt(r, "rerank_top_n", s.cfg.RerankTopN),
				"recall_limit":       options.RecallLimit,
				"items":              rows,
			})
			return
		}
		writeResult(w, rows, err)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown search route")
	}
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "agent route is required")
		return
	}
	if !s.requireTools(w) || !s.requireLLM(w) {
		return
	}
	route := strings.Join(parts, "/")
	switch route {
	case "chat":
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		traceID := newTraceID()
		w.Header().Set("X-Trace-Id", traceID)
		var req chatRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.Message) == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "message is required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		started := time.Now()
		conversationID, turnID := s.prepareAgentPersistence(ctx, req, traceID)
		collector := newToolTraceCollector()
		runner := agent.New(agent.Config{BaseURL: s.cfg.LLMBaseURL, APIKey: s.cfg.LLMAPIKey, Model: s.cfg.LLMModel}, s.tools)
		result, err := runner.RunWithEventsDetailed(ctx, req.Message, collector.Add)
		status := result.Status
		if err != nil && errors.Is(ctx.Err(), context.Canceled) {
			status = "aborted"
		}
		if turnID > 0 {
			persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.finishAgentTurn(persistCtx, turnID, conversationID, result, status, err, started, collector.Calls())
			persistCancel()
		}
		body := map[string]any{"message": result.Message, "trace_id": traceID}
		if conversationID > 0 {
			body["conversation_id"] = conversationID
		}
		writeResult(w, body, err)
	case "chat/stream":
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		s.handleAgentStream(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown agent route")
	}
}

func (s *Server) handleAgentStream(w http.ResponseWriter, r *http.Request) {
	traceID := newTraceID()
	w.Header().Set("X-Trace-Id", traceID)
	var req chatRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "message is required")
		return
	}
	started := time.Now()
	conversationID, turnID := s.prepareAgentPersistence(r.Context(), req, traceID)
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "STREAM_UNSUPPORTED", "response writer does not support streaming")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	statusEvent := map[string]any{"message": "started", "trace_id": traceID}
	if conversationID > 0 {
		statusEvent["conversation_id"] = conversationID
	}
	writeSSE(w, "status", statusEvent)
	flusher.Flush()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	collector := newToolTraceCollector()
	runner := agent.New(agent.Config{BaseURL: s.cfg.LLMBaseURL, APIKey: s.cfg.LLMAPIKey, Model: s.cfg.LLMModel}, s.tools)
	result, err := runner.RunWithEventsDetailed(ctx, req.Message, func(event agent.Event) {
		event.TraceID = traceID
		collector.Add(event)
		writeSSE(w, event.Type, event)
		flusher.Flush()
	})
	status := result.Status
	if err != nil && errors.Is(ctx.Err(), context.Canceled) {
		status = "aborted"
	}
	if turnID > 0 {
		persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.finishAgentTurn(persistCtx, turnID, conversationID, result, status, err, started, collector.Calls())
		persistCancel()
	}
	if err != nil {
		errorEvent := map[string]any{"code": "LLM_UPSTREAM_ERROR", "message": err.Error(), "trace_id": traceID}
		if conversationID > 0 {
			errorEvent["conversation_id"] = conversationID
		}
		writeSSE(w, "error", errorEvent)
		flusher.Flush()
		return
	}
	finalEvent := map[string]any{"message": result.Message, "trace_id": traceID}
	if conversationID > 0 {
		finalEvent["conversation_id"] = conversationID
	}
	writeSSE(w, "final", finalEvent)
	flusher.Flush()
}

func (s *Server) prepareAgentPersistence(ctx context.Context, req chatRequest, traceID string) (int64, int64) {
	if s.db == nil {
		return 0, 0
	}
	persistCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conversationID, err := s.ensureConversation(persistCtx, req.ConversationID, req.SessionID, req.Message)
	if err != nil || conversationID <= 0 {
		return 0, 0
	}
	_, _ = s.insertMessage(persistCtx, conversationID, "user", req.Message, nil)
	turnID, err := s.startAgentTurn(persistCtx, conversationID, traceID, s.cfg.LLMModel)
	if err != nil {
		return conversationID, 0
	}
	return conversationID, turnID
}

func (s *Server) handleMechanics(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "mechanics route is required")
		return
	}
	if !allowMethod(w, r, http.MethodPost) || !s.requireTools(w) {
		return
	}
	var req mechanicRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	options := tools.NewModifierOptionsWithContexts(req.IncludeEidolons, req.Eidolons, req.ActiveContexts, req.InactiveContexts)
	switch parts[0] {
	case "compare-character-fit":
		if req.AttackerID == 0 || req.SupportID == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "attacker_id and support_id are required")
			return
		}
		result, err := s.tools.CompareCharacterFitWithOptions(r.Context(), req.AttackerID, req.SupportID, req.AttackTag, options)
		writeResult(w, result, err)
	case "estimate-damage-gain":
		if req.AttackerID == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "attacker_id is required")
			return
		}
		result, err := s.tools.EstimateDamageGainWithOptions(r.Context(), req.AttackerID, supportIDs(req), req.AttackTag, options)
		writeResult(w, result, err)
	case "estimate-dot-damage":
		if req.AttackerID == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "attacker_id is required")
			return
		}
		result, err := s.tools.EstimateDotDamage(r.Context(), req.AttackerID, supportIDs(req), options)
		writeResult(w, result, err)
	case "estimate-break-damage":
		if req.AttackerID == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "attacker_id is required")
			return
		}
		result, err := s.tools.EstimateBreakDamage(r.Context(), req.AttackerID, supportIDs(req), breakScenario(req, false), options)
		writeResult(w, result, err)
	case "estimate-super-break-damage":
		if req.AttackerID == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "attacker_id is required")
			return
		}
		result, err := s.tools.EstimateSuperBreakDamage(r.Context(), req.AttackerID, supportIDs(req), breakScenario(req, true), options)
		writeResult(w, result, err)
	case "estimate-healing":
		if req.CharID == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "char_id is required")
			return
		}
		result, err := s.tools.EstimateHealing(r.Context(), req.CharID, supportIDs(req), req.ScalingStat, sustainScenario(req), options)
		writeResult(w, result, err)
	case "estimate-shield":
		if req.CharID == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "char_id is required")
			return
		}
		result, err := s.tools.EstimateShield(r.Context(), req.CharID, supportIDs(req), req.ScalingStat, sustainScenario(req), options)
		writeResult(w, result, err)
	case "estimate-uptime":
		result, err := s.tools.EstimateUptime(r.Context(), calc.UptimeScenario{
			DurationTurns:   req.DurationTurns,
			CooldownTurns:   req.CooldownTurns,
			CycleTurns:      req.CycleTurns,
			StartDelayTurns: req.StartDelayTurns,
		})
		writeResult(w, result, err)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown mechanics route")
	}
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not allowed")
		return
	}
	root := strings.TrimSpace(s.cfg.WebRoot)
	if root == "" {
		http.NotFound(w, r)
		return
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cleanPath := filepath.FromSlash(strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/"))
	fullPath := filepath.Join(rootAbs, cleanPath)
	if !isInside(rootAbs, fullPath) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, fullPath)
		return
	}
	indexPath := filepath.Join(rootAbs, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, indexPath)
		return
	}
	http.NotFound(w, r)
}

// serveMedia 同源伺服本地资源(角色/光锥/遗器图片),避免前端跨境直连 CDN。
// 只从 cfg.AssetRoot 读取,沿用 serveStatic 的 path.Clean + isInside 防路径穿越;
// 不做 SPA index.html 回退(缺图返回 404,而非 HTML)。
func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not allowed")
		return
	}
	root := strings.TrimSpace(s.cfg.AssetRoot)
	if root == "" {
		http.NotFound(w, r)
		return
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, tools.AssetURLPrefix)
	cleanPath := filepath.FromSlash(strings.TrimPrefix(path.Clean("/"+rel), "/"))
	if cleanPath == "" {
		http.NotFound(w, r)
		return
	}
	fullPath := filepath.Join(rootAbs, cleanPath)
	if !isInside(rootAbs, fullPath) {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	if ct := mediaContentType(fullPath); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, fullPath)
}

// mediaContentType 为常见图片扩展名返回确定的 MIME(避免依赖各平台 mime 注册表;
// .webp 在 Windows 上 mime.TypeByExtension 可能为空)。未知扩展名返回空,交给 ServeFile 嗅探。
func mediaContentType(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	default:
		return ""
	}
}

func (s *Server) requireTools(w http.ResponseWriter) bool {
	if s.tools == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "tool service is not configured")
		return false
	}
	return true
}

func (s *Server) requireLLM(w http.ResponseWriter) bool {
	if s.cfg.LLMAPIFormat != "openai" {
		writeError(w, http.StatusServiceUnavailable, "LLM_UNSUPPORTED_FORMAT", "Go agent currently supports LLM_API_FORMAT=openai")
		return false
	}
	if strings.TrimSpace(s.cfg.LLMAPIKey) == "" {
		writeError(w, http.StatusServiceUnavailable, "LLM_NOT_CONFIGURED", "LLM_API_KEY is required")
		return false
	}
	return true
}

func writeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		code := "TOOL_EXECUTION_ERROR"
		if errors.Is(err, pgx.ErrNoRows) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		if strings.Contains(err.Error(), "entity embeddings") {
			status = http.StatusServiceUnavailable
			code = "SEMANTIC_SEARCH_NOT_READY"
		}
		if strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
			code = "BAD_REQUEST"
		}
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, errorBody{Error: apiError{Code: code, Message: message}})
}

func writeSSE(w http.ResponseWriter, event string, value any) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.TrimSpace(buf.String()))
}

func newTraceID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func allowMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not allowed")
	return false
}

func pathParts(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	raw := strings.Split(path, "/")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func queryInt(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func queryBool(r *http.Request, key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func wantsEnvelope(r *http.Request) bool {
	if queryBool(r, "include_meta", false) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "object")
}

func queryCSV(r *http.Request, key string) []string {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil
	}
	var out []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func queryIntCSV(r *http.Request, key string) []int {
	values := queryCSV(r, key)
	out := make([]int, 0, len(values))
	for _, value := range values {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			out = append(out, parsed)
		}
	}
	return out
}

func parsePositiveInt(text string) (int, bool) {
	value, err := strconv.Atoi(text)
	return value, err == nil && value > 0
}

func parsePositiveInt64(text string) (int64, bool) {
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil && value > 0
}

func supportIDs(req mechanicRequest) []int {
	if len(req.SupportIDs) > 0 {
		return req.SupportIDs
	}
	if req.SupportID != 0 {
		return []int{req.SupportID}
	}
	return nil
}

func breakScenario(req mechanicRequest, superBreak bool) calc.BreakScenario {
	enemyCount := req.EnemyCount
	if enemyCount == 0 {
		enemyCount = 1
	}
	maxToughness := req.MaxToughness
	if maxToughness == 0 {
		maxToughness = 90
	}
	toughnessReduction := req.ToughnessReduction
	if toughnessReduction == 0 {
		toughnessReduction = 30
	}
	superBreakBaseMultiplier := req.SuperBreakBaseMultiplier
	superBreakMultiplier := req.SuperBreakMultiplier
	if superBreak {
		if superBreakBaseMultiplier == 0 {
			superBreakBaseMultiplier = superBreakMultiplier
		}
		if superBreakBaseMultiplier == 0 {
			superBreakBaseMultiplier = 1
		}
		if superBreakMultiplier == 0 {
			superBreakMultiplier = superBreakBaseMultiplier
		}
	}
	resistance := req.EnemyResistance
	if resistance == 0 {
		resistance = 0.2
	}
	return calc.BreakScenario{
		ElementKey:               req.Element,
		EnemyCount:               enemyCount,
		BreakEffect:              req.BreakEffect,
		BreakDamageBonus:         req.BreakDamageBonus,
		SuperBreakBonus:          req.SuperBreakBonus,
		ToughnessReduction:       toughnessReduction,
		MaxToughness:             maxToughness,
		SuperBreakBaseMultiplier: superBreakBaseMultiplier,
		SuperBreakMultiplier:     superBreakMultiplier,
		Resistance:               resistance,
		DefReduction:             req.DefReduction,
		DefIgnore:                req.DefIgnore,
		ResReduction:             req.ResReduction,
		ResPen:                   req.ResPen,
		Vulnerability:            req.Vulnerability,
		DamageReduction:          req.DamageReduction,
	}
}

func sustainScenario(req mechanicRequest) calc.SustainScenario {
	base := req.BaseScalingStat
	if base == 0 {
		base = 1000
	}
	multiplier := req.AbilityMultiplier
	if multiplier == 0 {
		multiplier = 1
	}
	return calc.SustainScenario{
		BaseScalingStat:   base,
		AbilityMultiplier: multiplier,
		FlatValue:         req.FlatValue,
	}
}

func semanticHTTPEnabled(cfg config.Config) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.EmbeddingProvider), "openai_compatible") &&
		strings.TrimSpace(cfg.EmbeddingBaseURL) != "" &&
		strings.TrimSpace(cfg.EmbeddingAPIKey) != "" &&
		strings.TrimSpace(cfg.EmbeddingModel) != ""
}

func embeddingConfigured(provider string, baseURL string, apiKey string, model string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai_compatible":
		return strings.TrimSpace(baseURL) != "" && strings.TrimSpace(apiKey) != "" && strings.TrimSpace(model) != ""
	case "local_hash", "local-hash-ngram-v1":
		return strings.TrimSpace(model) != ""
	default:
		return false
	}
}

func rerankConfigured(provider string, baseURL string, apiKey string, model string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai_compatible":
		return strings.TrimSpace(baseURL) != "" && strings.TrimSpace(apiKey) != "" && strings.TrimSpace(model) != ""
	default:
		return false
	}
}

func embeddingQuality(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai_compatible", "ollama":
		return "semantic"
	case "local_hash", "local-hash-ngram-v1":
		return "lexical_hash"
	default:
		return "disabled"
	}
}

func fallback(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func fallbackInt(value int, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}

func isInside(root string, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
