package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type EmbeddingModel struct {
	ID                 string
	Label              string
	Provider           string
	BaseURL            string
	APIKey             string
	Model              string
	Dimensions         int
	NativeDimensions   int
	ProjectionStrategy string
	EncodingFormat     string
	ExtraHeaders       string
	Notes              string
}

type RerankModel struct {
	ID            string
	Label         string
	Provider      string
	BaseURL       string
	APIKey        string
	Model         string
	ExtraHeaders  string
	ContextLength int
	Notes         string
}

type Config struct {
	DatabaseURL              string
	LLMBaseURL               string
	LLMAPIKey                string
	LLMModel                 string
	LLMAPIFormat             string
	HTTPAddr                 string
	WebRoot                  string
	AssetRoot                string
	EmbeddingProvider        string
	EmbeddingBaseURL         string
	EmbeddingAPIKey          string
	EmbeddingModel           string
	EmbeddingDimensions      int
	EmbeddingEncoding        string
	EmbeddingHeaders         string
	EmbeddingCacheTTLSeconds int
	EmbeddingCacheMaxEntries int
	DefaultEmbeddingID       string
	EmbeddingModels          []EmbeddingModel
	RerankProvider           string
	RerankBaseURL            string
	RerankAPIKey             string
	RerankModel              string
	RerankHeaders            string
	DefaultRerankID          string
	RerankTopN               int
	RerankModels             []RerankModel
}

func Load() Config {
	loadDotEnv()
	cfg := Config{
		DatabaseURL:              getenv("DATABASE_URL", "postgresql://hsr:hsr@localhost:55432/hsr_agent"),
		LLMBaseURL:               getenv("LLM_BASE_URL", "https://api.deepseek.com"),
		LLMAPIKey:                os.Getenv("LLM_API_KEY"),
		LLMModel:                 getenv("LLM_MODEL", "deepseek-chat"),
		LLMAPIFormat:             getenv("LLM_API_FORMAT", "openai"),
		HTTPAddr:                 getenv("HTTP_ADDR", "127.0.0.1:8080"),
		WebRoot:                  getenv("WEB_ROOT", "web/dist"),
		AssetRoot:                resolveAssetRoot(getenv("ASSET_ROOT", "nanoka_hsr/4.3.54/assets/hsr")),
		EmbeddingProvider:        getenv("EMBEDDING_PROVIDER", "disabled"),
		EmbeddingBaseURL:         os.Getenv("EMBEDDING_BASE_URL"),
		EmbeddingAPIKey:          os.Getenv("EMBEDDING_API_KEY"),
		EmbeddingModel:           getenv("EMBEDDING_MODEL", ""),
		EmbeddingDimensions:      getenvInt("EMBEDDING_DIMENSIONS", 1024),
		EmbeddingEncoding:        getenv("EMBEDDING_ENCODING_FORMAT", "float"),
		EmbeddingHeaders:         os.Getenv("EMBEDDING_EXTRA_HEADERS"),
		EmbeddingCacheTTLSeconds: getenvInt("EMBEDDING_QUERY_CACHE_TTL_SECONDS", 600),
		EmbeddingCacheMaxEntries: getenvInt("EMBEDDING_QUERY_CACHE_MAX_ENTRIES", 256),
		DefaultEmbeddingID:       os.Getenv("EMBEDDING_DEFAULT_ID"),
		RerankProvider:           getenv("RERANK_PROVIDER", "disabled"),
		RerankBaseURL:            os.Getenv("RERANK_BASE_URL"),
		RerankAPIKey:             os.Getenv("RERANK_API_KEY"),
		RerankModel:              os.Getenv("RERANK_MODEL"),
		RerankHeaders:            os.Getenv("RERANK_EXTRA_HEADERS"),
		DefaultRerankID:          os.Getenv("RERANK_DEFAULT_ID"),
		RerankTopN:               getenvInt("RERANK_TOP_N", 25),
	}
	cfg.EmbeddingModels = loadEmbeddingModels(cfg)
	cfg.RerankModels = loadRerankModels(cfg)
	cfg.applyDefaultEmbeddingModel()
	cfg.applyDefaultRerankModel()
	return cfg
}

// resolveAssetRoot 解析本地资源根目录。相对路径时按 cwd 与上一级各探一次
// (与 loadDotEnv 探测 .env / ../.env 同思路),兼容从仓库根或 hsr-agent-go 启动。
// 都不存在时原样返回(serveMedia 会优雅 404)。
func resolveAssetRoot(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || filepath.IsAbs(raw) {
		return raw
	}
	for _, cand := range []string{raw, filepath.Join("..", raw)} {
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			return cand
		}
	}
	return raw
}

func loadDotEnv() {
	for _, path := range []string{".env", filepath.Join("..", ".env")} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			key, value, _ := strings.Cut(line, "=")
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"'`)
			if key != "" && os.Getenv(key) == "" {
				_ = os.Setenv(key, value)
			}
		}
		return
	}
}

func (c *Config) applyDefaultEmbeddingModel() {
	if len(c.EmbeddingModels) == 0 {
		return
	}
	index := 0
	if c.DefaultEmbeddingID != "" {
		for i, model := range c.EmbeddingModels {
			if model.ID == c.DefaultEmbeddingID {
				index = i
				break
			}
		}
	}
	model := c.EmbeddingModels[index]
	c.DefaultEmbeddingID = model.ID
	c.EmbeddingProvider = model.Provider
	c.EmbeddingBaseURL = model.BaseURL
	c.EmbeddingAPIKey = model.APIKey
	c.EmbeddingModel = model.Model
	c.EmbeddingDimensions = model.Dimensions
	c.EmbeddingEncoding = model.EncodingFormat
	c.EmbeddingHeaders = model.ExtraHeaders
}

func (c *Config) applyDefaultRerankModel() {
	if len(c.RerankModels) == 0 {
		return
	}
	index := 0
	if c.DefaultRerankID != "" {
		for i, model := range c.RerankModels {
			if model.ID == c.DefaultRerankID {
				index = i
				break
			}
		}
	}
	model := c.RerankModels[index]
	c.DefaultRerankID = model.ID
	c.RerankProvider = model.Provider
	c.RerankBaseURL = model.BaseURL
	c.RerankAPIKey = model.APIKey
	c.RerankModel = model.Model
	c.RerankHeaders = model.ExtraHeaders
}

func loadEmbeddingModels(cfg Config) []EmbeddingModel {
	ids := csvEnv("EMBEDDING_MODEL_IDS")
	models := make([]EmbeddingModel, 0, len(ids))
	for _, id := range ids {
		prefix := "EMBEDDING_MODEL_" + envID(id) + "_"
		model := EmbeddingModel{
			ID:             id,
			Label:          getenv(prefix+"LABEL", id),
			Provider:       getenv(prefix+"PROVIDER", "openai_compatible"),
			BaseURL:        os.Getenv(prefix + "BASE_URL"),
			APIKey:         os.Getenv(prefix + "API_KEY"),
			Model:          os.Getenv(prefix + "MODEL"),
			Dimensions:     getenvInt(prefix+"DIMENSIONS", 1024),
			EncodingFormat: getenv(prefix+"ENCODING_FORMAT", "float"),
			ExtraHeaders:   os.Getenv(prefix + "EXTRA_HEADERS"),
			Notes:          os.Getenv(prefix + "NOTES"),
		}
		if model.Model == "" {
			model.Model = id
		}
		model.NativeDimensions = getenvInt(prefix+"NATIVE_DIMENSIONS", inferNativeDimensions(model.Model, model.Dimensions))
		model.ProjectionStrategy = getenv(prefix+"PROJECTION_STRATEGY", defaultProjectionStrategy(model.NativeDimensions, model.Dimensions))
		models = append(models, model)
	}
	if len(models) == 0 && strings.TrimSpace(cfg.EmbeddingProvider) != "" && cfg.EmbeddingProvider != "disabled" {
		id := getenv("EMBEDDING_ID", "default")
		models = append(models, EmbeddingModel{
			ID:                 id,
			Label:              getenv("EMBEDDING_LABEL", cfg.EmbeddingModel),
			Provider:           cfg.EmbeddingProvider,
			BaseURL:            cfg.EmbeddingBaseURL,
			APIKey:             cfg.EmbeddingAPIKey,
			Model:              cfg.EmbeddingModel,
			Dimensions:         cfg.EmbeddingDimensions,
			NativeDimensions:   getenvInt("EMBEDDING_NATIVE_DIMENSIONS", inferNativeDimensions(cfg.EmbeddingModel, cfg.EmbeddingDimensions)),
			ProjectionStrategy: getenv("EMBEDDING_PROJECTION_STRATEGY", defaultProjectionStrategy(inferNativeDimensions(cfg.EmbeddingModel, cfg.EmbeddingDimensions), cfg.EmbeddingDimensions)),
			EncodingFormat:     cfg.EmbeddingEncoding,
			ExtraHeaders:       cfg.EmbeddingHeaders,
		})
		if cfg.DefaultEmbeddingID == "" {
			cfg.DefaultEmbeddingID = id
		}
	}
	return models
}

func loadRerankModels(cfg Config) []RerankModel {
	ids := csvEnv("RERANK_MODEL_IDS")
	models := make([]RerankModel, 0, len(ids))
	for _, id := range ids {
		prefix := "RERANK_MODEL_" + envID(id) + "_"
		model := RerankModel{
			ID:            id,
			Label:         getenv(prefix+"LABEL", id),
			Provider:      getenv(prefix+"PROVIDER", "openai_compatible"),
			BaseURL:       os.Getenv(prefix + "BASE_URL"),
			APIKey:        os.Getenv(prefix + "API_KEY"),
			Model:         os.Getenv(prefix + "MODEL"),
			ExtraHeaders:  os.Getenv(prefix + "EXTRA_HEADERS"),
			ContextLength: getenvInt(prefix+"CONTEXT_LENGTH", 0),
			Notes:         os.Getenv(prefix + "NOTES"),
		}
		if model.Model == "" {
			model.Model = id
		}
		models = append(models, model)
	}
	if len(models) == 0 && strings.TrimSpace(cfg.RerankProvider) != "" && cfg.RerankProvider != "disabled" {
		id := getenv("RERANK_ID", "default")
		models = append(models, RerankModel{
			ID:           id,
			Label:        getenv("RERANK_LABEL", cfg.RerankModel),
			Provider:     cfg.RerankProvider,
			BaseURL:      cfg.RerankBaseURL,
			APIKey:       cfg.RerankAPIKey,
			Model:        cfg.RerankModel,
			ExtraHeaders: cfg.RerankHeaders,
		})
		if cfg.DefaultRerankID == "" {
			cfg.DefaultRerankID = id
		}
	}
	return models
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	var out int
	for _, r := range value {
		if r < '0' || r > '9' {
			return fallback
		}
		out = out*10 + int(r-'0')
	}
	if out <= 0 {
		return fallback
	}
	return out
}

func csvEnv(key string) []string {
	var out []string
	for _, item := range strings.Split(os.Getenv(key), ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func envID(id string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToUpper(id) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func inferNativeDimensions(model string, storageDimensions int) int {
	name := strings.ToLower(model)
	switch {
	case strings.Contains(name, "qwen3-embedding-8b"):
		return 4096
	case strings.Contains(name, "qwen3-embedding-4b"):
		return 2560
	case strings.Contains(name, "qwen3-embedding-0.6b"), strings.Contains(name, "bge-m3"):
		return 1024
	default:
		return storageDimensions
	}
}

func defaultProjectionStrategy(nativeDimensions int, storageDimensions int) string {
	if nativeDimensions == storageDimensions {
		return "none"
	}
	if nativeDimensions > storageDimensions {
		return "truncate_" + strconv.Itoa(storageDimensions)
	}
	return "requested_dimensions"
}
