package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"hsr-agent-go/internal/agent"
	"hsr-agent-go/internal/calc"
	"hsr-agent-go/internal/config"
	"hsr-agent-go/internal/db"
	"hsr-agent-go/internal/embedding"
	"hsr-agent-go/internal/httpapi"
	"hsr-agent-go/internal/rerank"
	"hsr-agent-go/internal/tools"
)

func main() {
	serve := flag.Bool("serve", false, "start HTTP API server and static frontend host")
	addr := flag.String("addr", "", "HTTP listen address, default HTTP_ADDR or 127.0.0.1:8080")
	webRoot := flag.String("web-root", "", "frontend build directory, default WEB_ROOT or web/dist")
	toolName := flag.String("tool", "", "tool to run: get_character, resolve_entities, search_by_role, semantic_search, find_needs, find_buffers_for, find_synergies, suggest_team, co_occurrence, recommend_lightcones, recommend_relics, get_assets, list_character_modifiers, explain_modifier_sources, compare_character_fit, estimate_damage_gain, estimate_dot_damage, estimate_break_damage, estimate_super_break_damage, estimate_healing, estimate_shield, estimate_uptime")
	ask := flag.String("ask", "", "ask the LLM agent a question")
	traceTools := flag.Bool("trace-tools", false, "print LLM tool calls to stderr when using --ask")
	query := flag.String("query", "", "query text for get_character")
	charID := flag.Int("char-id", 0, "character id")
	axis := flag.String("axis", "", "axis stat")
	target := flag.String("target", "", "axis target")
	role := flag.String("role", "", "role filter")
	element := flag.String("element", "", "element filter")
	path := flag.String("path", "", "path filter")
	rarity := flag.Int("rarity", 0, "rarity filter")
	limit := flag.Int("limit", 10, "row limit")
	entityKind := flag.String("kind", "", "asset entity kind or semantic kind: character, lightcone, relic_set, all")
	entityID := flag.String("id", "", "asset entity id")
	variantCSV := flag.String("variants", "", "comma-separated asset variants")
	excludeCSV := flag.String("exclude", "", "comma-separated excluded character ids")
	supportID := flag.Int("support-id", 0, "support character id for mechanics tools")
	supportIDsCSV := flag.String("support-ids", "", "comma-separated support character ids for estimate_damage_gain")
	attackTag := flag.String("attack-tag", "", "attack tag: basic, skill, ult, fua, dot, break, super_break")
	includeEidolons := flag.Bool("include-eidolons", false, "include eidolon modifiers in mechanics estimates")
	eidolonsCSV := flag.String("eidolons", "", "comma-separated enabled eidolons, e.g. 1,2,6")
	scalingStat := flag.String("scaling-stat", "", "scaling stat for sustain tools: atk, hp, def")
	baseScalingStat := flag.Float64("base-scaling-stat", 1000, "base scaling stat for local mechanic estimates")
	abilityMultiplier := flag.Float64("ability-multiplier", 1, "ability multiplier, e.g. 2.4 for 240%")
	flatValue := flag.Float64("flat-value", 0, "flat damage/heal/shield value")
	breakEffect := flag.Float64("break-effect", 0, "break effect as decimal, e.g. 1.8 for 180%")
	breakDamageBonus := flag.Float64("break-dmg-bonus", 0, "break damage bonus as decimal")
	superBreakBonus := flag.Float64("super-break-dmg-bonus", 0, "super break damage bonus as decimal")
	toughnessReduction := flag.Float64("toughness-reduction", 30, "toughness reduction for super break")
	maxToughness := flag.Float64("max-toughness", 90, "enemy max toughness for break damage")
	enemyCount := flag.Int("enemy-count", 1, "enemy count for conditional break/super-break modifiers")
	superBreakMultiplier := flag.Float64("super-break-multiplier", 1, "super break base multiplier")
	enemyResistance := flag.Float64("enemy-resistance", 0.2, "enemy resistance as decimal")
	defReduction := flag.Float64("def-reduction", 0, "enemy defense reduction as decimal")
	defIgnore := flag.Float64("def-ignore", 0, "defense ignore as decimal")
	resReduction := flag.Float64("res-reduction", 0, "enemy resistance reduction as decimal")
	resPen := flag.Float64("res-pen", 0, "resistance penetration as decimal")
	vulnerability := flag.Float64("vulnerability", 0, "vulnerability as decimal")
	damageReduction := flag.Float64("damage-reduction", 0, "enemy damage reduction as decimal")
	durationTurns := flag.Float64("duration-turns", 0, "active duration in turns for estimate_uptime")
	cooldownTurns := flag.Float64("cooldown-turns", 0, "cooldown or refresh interval in turns for estimate_uptime")
	cycleTurns := flag.Float64("cycle-turns", 0, "cycle length in turns for estimate_uptime")
	startDelayTurns := flag.Float64("start-delay-turns", 0, "start delay in turns for estimate_uptime")
	flag.Parse()

	cfg := config.Load()
	if strings.TrimSpace(*addr) != "" {
		cfg.HTTPAddr = strings.TrimSpace(*addr)
	}
	if strings.TrimSpace(*webRoot) != "" {
		cfg.WebRoot = strings.TrimSpace(*webRoot)
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStartup()

	pool, err := db.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(startupCtx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	defaultEmbeddingID, embedders, err := embeddingClientsFromConfig(cfg)
	if err != nil {
		log.Fatalf("embedding config: %v", err)
	}
	defaultRerankID, rerankers, err := rerankClientsFromConfig(cfg)
	if err != nil {
		log.Fatalf("rerank config: %v", err)
	}

	if *serve {
		service := tools.NewWithModels(pool, defaultEmbeddingID, embedders, defaultRerankID, rerankers, cfg.RerankTopN)
		server := &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           httpapi.New(cfg, pool, service),
			ReadHeaderTimeout: 5 * time.Second,
		}
		log.Printf("hsr-agent HTTP server listening on http://%s (web_root=%s)", cfg.HTTPAddr, cfg.WebRoot)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
		return
	}

	if *toolName != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		service := tools.NewWithModels(pool, defaultEmbeddingID, embedders, defaultRerankID, rerankers, cfg.RerankTopN)
		result, err := runTool(ctx, service, *toolName, *query, *charID, *axis, *target, *role, *element, *path, *rarity, *limit, *entityKind, *entityID, *variantCSV, *excludeCSV, *supportID, *supportIDsCSV, *attackTag, *includeEidolons, *eidolonsCSV, *scalingStat, *baseScalingStat, *abilityMultiplier, *flatValue, *breakEffect, *breakDamageBonus, *superBreakBonus, *toughnessReduction, *maxToughness, *enemyCount, *superBreakMultiplier, *enemyResistance, *defReduction, *defIgnore, *resReduction, *resPen, *vulnerability, *damageReduction, *durationTurns, *cooldownTurns, *cycleTurns, *startDelayTurns)
		if err != nil {
			log.Fatal(err)
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			log.Fatal(err)
		}
		return
	}
	if strings.TrimSpace(*ask) != "" {
		if cfg.LLMAPIFormat != "openai" {
			log.Fatalf("Go agent currently supports LLM_API_FORMAT=openai, got %q", cfg.LLMAPIFormat)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		service := tools.NewWithModels(pool, defaultEmbeddingID, embedders, defaultRerankID, rerankers, cfg.RerankTopN)
		agentConfig := agent.Config{BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Model: cfg.LLMModel}
		if *traceTools {
			agentConfig.TraceWriter = os.Stderr
		}
		runner := agent.New(agentConfig, service)
		answer, err := runner.Run(ctx, *ask)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintln(os.Stdout, answer)
		return
	}

	model := cfg.LLMModel
	if model == "" {
		model = "(not configured)"
	}
	fmt.Fprintf(os.Stdout, "hsr-agent ready: database ok, model=%s\n", model)
}

func embeddingClientsFromConfig(cfg config.Config) (string, map[string]*embedding.Client, error) {
	out := map[string]*embedding.Client{}
	if len(cfg.EmbeddingModels) == 0 {
		headers, err := embedding.ParseExtraHeaders(cfg.EmbeddingHeaders)
		if err != nil {
			return "", nil, err
		}
		out["default"] = embedding.NewClient(embedding.Config{
			Provider:       cfg.EmbeddingProvider,
			BaseURL:        cfg.EmbeddingBaseURL,
			APIKey:         cfg.EmbeddingAPIKey,
			Model:          cfg.EmbeddingModel,
			Dimensions:     cfg.EmbeddingDimensions,
			EncodingFormat: cfg.EmbeddingEncoding,
			ExtraHeaders:   headers,
			QueryCacheTTL:  time.Duration(cfg.EmbeddingCacheTTLSeconds) * time.Second,
			QueryCacheMax:  cfg.EmbeddingCacheMaxEntries,
		})
		return "default", out, nil
	}
	for _, model := range cfg.EmbeddingModels {
		headers, err := embedding.ParseExtraHeaders(model.ExtraHeaders)
		if err != nil {
			return "", nil, fmt.Errorf("%s: %w", model.ID, err)
		}
		out[model.ID] = embedding.NewClient(embedding.Config{
			Provider:       model.Provider,
			BaseURL:        model.BaseURL,
			APIKey:         model.APIKey,
			Model:          model.Model,
			Dimensions:     model.Dimensions,
			EncodingFormat: model.EncodingFormat,
			ExtraHeaders:   headers,
			QueryCacheTTL:  time.Duration(cfg.EmbeddingCacheTTLSeconds) * time.Second,
			QueryCacheMax:  cfg.EmbeddingCacheMaxEntries,
		})
	}
	defaultID := cfg.DefaultEmbeddingID
	if defaultID == "" && len(cfg.EmbeddingModels) > 0 {
		defaultID = cfg.EmbeddingModels[0].ID
	}
	return defaultID, out, nil
}

func rerankClientsFromConfig(cfg config.Config) (string, map[string]*rerank.Client, error) {
	out := map[string]*rerank.Client{}
	if len(cfg.RerankModels) == 0 {
		headers, err := rerank.ParseExtraHeaders(cfg.RerankHeaders)
		if err != nil {
			return "", nil, err
		}
		out["default"] = rerank.NewClient(rerank.Config{
			Provider:     cfg.RerankProvider,
			BaseURL:      cfg.RerankBaseURL,
			APIKey:       cfg.RerankAPIKey,
			Model:        cfg.RerankModel,
			ExtraHeaders: headers,
		})
		return "default", out, nil
	}
	for _, model := range cfg.RerankModels {
		headers, err := rerank.ParseExtraHeaders(model.ExtraHeaders)
		if err != nil {
			return "", nil, fmt.Errorf("%s: %w", model.ID, err)
		}
		out[model.ID] = rerank.NewClient(rerank.Config{
			Provider:     model.Provider,
			BaseURL:      model.BaseURL,
			APIKey:       model.APIKey,
			Model:        model.Model,
			ExtraHeaders: headers,
		})
	}
	defaultID := cfg.DefaultRerankID
	if defaultID == "" && len(cfg.RerankModels) > 0 {
		defaultID = cfg.RerankModels[0].ID
	}
	return defaultID, out, nil
}

func runTool(
	ctx context.Context,
	service *tools.Service,
	toolName string,
	query string,
	charID int,
	axis string,
	target string,
	role string,
	element string,
	path string,
	rarity int,
	limit int,
	entityKind string,
	entityID string,
	variantCSV string,
	excludeCSV string,
	supportID int,
	supportIDsCSV string,
	attackTag string,
	includeEidolons bool,
	eidolonsCSV string,
	scalingStat string,
	baseScalingStat float64,
	abilityMultiplier float64,
	flatValue float64,
	breakEffect float64,
	breakDamageBonus float64,
	superBreakBonus float64,
	toughnessReduction float64,
	maxToughness float64,
	enemyCount int,
	superBreakMultiplier float64,
	enemyResistance float64,
	defReduction float64,
	defIgnore float64,
	resReduction float64,
	resPen float64,
	vulnerability float64,
	damageReduction float64,
	durationTurns float64,
	cooldownTurns float64,
	cycleTurns float64,
	startDelayTurns float64,
) (any, error) {
	eidolons, err := parseCSVInts(eidolonsCSV)
	if err != nil {
		return nil, err
	}
	modifierOptions := tools.NewModifierOptions(includeEidolons, eidolons)

	switch toolName {
	case "get_character":
		return service.GetCharacter(ctx, query)
	case "resolve_entities":
		if query == "" || entityKind == "" {
			return nil, fmt.Errorf("--query and --kind are required")
		}
		return service.ResolveEntities(ctx, []tools.EntityResolveRequest{{Name: query, Kind: entityKind}}, "both")
	case "search_by_role":
		return service.SearchByRole(ctx, role, element, path, rarity, limit)
	case "semantic_search":
		return service.SemanticSearch(ctx, query, entityKind, limit)
	case "find_needs":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		return service.FindNeeds(ctx, charID)
	case "find_buffers_for":
		if axis == "" {
			return nil, fmt.Errorf("--axis is required")
		}
		return service.FindBuffersFor(ctx, axis, target, limit)
	case "find_synergies":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		return service.FindSynergies(ctx, charID, limit)
	case "suggest_team":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		var exclude []int
		if strings.TrimSpace(excludeCSV) != "" {
			for _, item := range strings.Split(excludeCSV, ",") {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				id, err := tools.ParseInt(item)
				if err != nil {
					return nil, err
				}
				exclude = append(exclude, id)
			}
		}
		return service.SuggestTeam(ctx, charID, limit, exclude)
	case "co_occurrence":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		return service.CoOccurrence(ctx, charID, limit)
	case "recommend_lightcones":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		return service.RecommendLightcones(ctx, charID)
	case "recommend_relics":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		return service.RecommendRelics(ctx, charID)
	case "get_assets":
		if entityKind == "" || entityID == "" {
			return nil, fmt.Errorf("--kind and --id are required")
		}
		var variants []string
		if strings.TrimSpace(variantCSV) != "" {
			for _, item := range strings.Split(variantCSV, ",") {
				item = strings.TrimSpace(item)
				if item != "" {
					variants = append(variants, item)
				}
			}
		}
		return service.GetAssets(ctx, entityKind, entityID, variants)
	case "list_character_modifiers":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		return service.ListCharacterModifiers(ctx, charID, axis, target, limit)
	case "explain_modifier_sources":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		return service.ExplainModifierSources(ctx, charID, limit)
	case "compare_character_fit":
		if charID == 0 || supportID == 0 {
			return nil, fmt.Errorf("--char-id and --support-id are required")
		}
		return service.CompareCharacterFitWithOptions(ctx, charID, supportID, attackTag, modifierOptions)
	case "estimate_damage_gain":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		supportIDs, err := parseCSVInts(supportIDsCSV)
		if err != nil {
			return nil, err
		}
		if len(supportIDs) == 0 && supportID != 0 {
			supportIDs = []int{supportID}
		}
		return service.EstimateDamageGainWithOptions(ctx, charID, supportIDs, attackTag, modifierOptions)
	case "estimate_dot_damage":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		supportIDs, err := parseSupportIDs(supportIDsCSV, supportID)
		if err != nil {
			return nil, err
		}
		return service.EstimateDotDamage(ctx, charID, supportIDs, modifierOptions)
	case "estimate_break_damage":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		supportIDs, err := parseSupportIDs(supportIDsCSV, supportID)
		if err != nil {
			return nil, err
		}
		scenario := calc.BreakScenario{
			ElementKey:       element,
			EnemyCount:       enemyCount,
			BreakEffect:      breakEffect,
			BreakDamageBonus: breakDamageBonus,
			MaxToughness:     maxToughness,
			Resistance:       enemyResistance,
			DefReduction:     defReduction,
			DefIgnore:        defIgnore,
			ResReduction:     resReduction,
			ResPen:           resPen,
			Vulnerability:    vulnerability,
			DamageReduction:  damageReduction,
		}
		return service.EstimateBreakDamage(ctx, charID, supportIDs, scenario, modifierOptions)
	case "estimate_super_break_damage":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		supportIDs, err := parseSupportIDs(supportIDsCSV, supportID)
		if err != nil {
			return nil, err
		}
		scenario := calc.BreakScenario{
			ElementKey:           element,
			EnemyCount:           enemyCount,
			BreakEffect:          breakEffect,
			BreakDamageBonus:     breakDamageBonus,
			SuperBreakBonus:      superBreakBonus,
			ToughnessReduction:   toughnessReduction,
			SuperBreakMultiplier: superBreakMultiplier,
			Resistance:           enemyResistance,
			DefReduction:         defReduction,
			DefIgnore:            defIgnore,
			ResReduction:         resReduction,
			ResPen:               resPen,
			Vulnerability:        vulnerability,
			DamageReduction:      damageReduction,
		}
		return service.EstimateSuperBreakDamage(ctx, charID, supportIDs, scenario, modifierOptions)
	case "estimate_healing":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		supportIDs, err := parseSupportIDs(supportIDsCSV, supportID)
		if err != nil {
			return nil, err
		}
		scenario := calc.SustainScenario{
			BaseScalingStat:   baseScalingStat,
			AbilityMultiplier: abilityMultiplier,
			FlatValue:         flatValue,
		}
		return service.EstimateHealing(ctx, charID, supportIDs, scalingStat, scenario, modifierOptions)
	case "estimate_shield":
		if charID == 0 {
			return nil, fmt.Errorf("--char-id is required")
		}
		supportIDs, err := parseSupportIDs(supportIDsCSV, supportID)
		if err != nil {
			return nil, err
		}
		scenario := calc.SustainScenario{
			BaseScalingStat:   baseScalingStat,
			AbilityMultiplier: abilityMultiplier,
			FlatValue:         flatValue,
		}
		return service.EstimateShield(ctx, charID, supportIDs, scalingStat, scenario, modifierOptions)
	case "estimate_uptime":
		return service.EstimateUptime(ctx, calc.UptimeScenario{
			DurationTurns:   durationTurns,
			CooldownTurns:   cooldownTurns,
			CycleTurns:      cycleTurns,
			StartDelayTurns: startDelayTurns,
		})
	default:
		return nil, fmt.Errorf("unknown tool %q", toolName)
	}
}

func parseCSVInts(csv string) ([]int, error) {
	var out []int
	if strings.TrimSpace(csv) == "" {
		return out, nil
	}
	for _, item := range strings.Split(csv, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		id, err := tools.ParseInt(item)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func parseSupportIDs(csv string, single int) ([]int, error) {
	supportIDs, err := parseCSVInts(csv)
	if err != nil {
		return nil, err
	}
	if len(supportIDs) == 0 && single != 0 {
		supportIDs = []int{single}
	}
	return supportIDs, nil
}
