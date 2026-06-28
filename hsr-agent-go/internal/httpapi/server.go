package httpapi

import (
	"bytes"
	"context"
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
	Message string `json:"message"`
}

type mechanicRequest struct {
	AttackerID           int     `json:"attacker_id"`
	SupportID            int     `json:"support_id"`
	SupportIDs           []int   `json:"support_ids"`
	CharID               int     `json:"char_id"`
	AttackTag            string  `json:"attack_tag"`
	IncludeEidolons      bool    `json:"include_eidolons"`
	Eidolons             []int   `json:"eidolons"`
	Element              string  `json:"element"`
	EnemyCount           int     `json:"enemy_count"`
	BreakEffect          float64 `json:"break_effect"`
	BreakDamageBonus     float64 `json:"break_dmg_bonus"`
	SuperBreakBonus      float64 `json:"super_break_dmg_bonus"`
	ToughnessReduction   float64 `json:"toughness_reduction"`
	MaxToughness         float64 `json:"max_toughness"`
	SuperBreakMultiplier float64 `json:"super_break_multiplier"`
	EnemyResistance      float64 `json:"enemy_resistance"`
	DefReduction         float64 `json:"def_reduction"`
	DefIgnore            float64 `json:"def_ignore"`
	ResReduction         float64 `json:"res_reduction"`
	ResPen               float64 `json:"res_pen"`
	Vulnerability        float64 `json:"vulnerability"`
	DamageReduction      float64 `json:"damage_reduction"`
	ScalingStat          string  `json:"scaling_stat"`
	BaseScalingStat      float64 `json:"base_scaling_stat"`
	AbilityMultiplier    float64 `json:"ability_multiplier"`
	FlatValue            float64 `json:"flat_value"`
	DurationTurns        float64 `json:"duration_turns"`
	CooldownTurns        float64 `json:"cooldown_turns"`
	CycleTurns           float64 `json:"cycle_turns"`
	StartDelayTurns      float64 `json:"start_delay_turns"`
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
	item, err := s.tools.GetLightcone(r.Context(), id)
	writeResult(w, item, err)
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
		writeResult(w, rows, err)
	case "semantic":
		if !semanticHTTPEnabled(s.cfg.EmbeddingProvider) {
			writeError(w, http.StatusServiceUnavailable, "SEMANTIC_SEARCH_DISABLED", "semantic search HTTP API requires a real embedding provider; use /api/search/keyword for now")
			return
		}
		if !s.requireTools(w) {
			return
		}
		rows, err := s.tools.SemanticSearch(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("kind"), queryInt(r, "limit", 10))
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
		runner := agent.New(agent.Config{BaseURL: s.cfg.LLMBaseURL, APIKey: s.cfg.LLMAPIKey, Model: s.cfg.LLMModel}, s.tools)
		answer, err := runner.Run(ctx, req.Message)
		writeResult(w, map[string]any{"message": answer}, err)
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
	var req chatRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "message is required")
		return
	}
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

	writeSSE(w, "status", map[string]any{"message": "started"})
	flusher.Flush()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	runner := agent.New(agent.Config{BaseURL: s.cfg.LLMBaseURL, APIKey: s.cfg.LLMAPIKey, Model: s.cfg.LLMModel}, s.tools)
	answer, err := runner.RunWithEvents(ctx, req.Message, func(event agent.Event) {
		writeSSE(w, event.Type, event)
		flusher.Flush()
	})
	if err != nil {
		writeSSE(w, "error", map[string]any{"code": "LLM_UPSTREAM_ERROR", "message": err.Error()})
		flusher.Flush()
		return
	}
	writeSSE(w, "final", map[string]any{"message": answer})
	flusher.Flush()
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
	options := tools.NewModifierOptions(req.IncludeEidolons, req.Eidolons)
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
	superBreakMultiplier := req.SuperBreakMultiplier
	if superBreak && superBreakMultiplier == 0 {
		superBreakMultiplier = 1
	}
	resistance := req.EnemyResistance
	if resistance == 0 {
		resistance = 0.2
	}
	return calc.BreakScenario{
		ElementKey:           req.Element,
		EnemyCount:           enemyCount,
		BreakEffect:          req.BreakEffect,
		BreakDamageBonus:     req.BreakDamageBonus,
		SuperBreakBonus:      req.SuperBreakBonus,
		ToughnessReduction:   toughnessReduction,
		MaxToughness:         maxToughness,
		SuperBreakMultiplier: superBreakMultiplier,
		Resistance:           resistance,
		DefReduction:         req.DefReduction,
		DefIgnore:            req.DefIgnore,
		ResReduction:         req.ResReduction,
		ResPen:               req.ResPen,
		Vulnerability:        req.Vulnerability,
		DamageReduction:      req.DamageReduction,
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

func semanticHTTPEnabled(provider string) bool {
	// The HTTP API must not expose local-hash-ngram-v1 as semantic search.
	// Keep this disabled until the embedding generation/query path uses a real provider.
	return false
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
