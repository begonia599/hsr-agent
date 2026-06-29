package embedding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

const Dimensions = 1024

type Config struct {
	Provider       string
	BaseURL        string
	APIKey         string
	Model          string
	Dimensions     int
	EncodingFormat string
	ExtraHeaders   map[string]string
	QueryCacheTTL  time.Duration
	QueryCacheMax  int
}

type Metadata struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
	Quality    string `json:"quality"`
}

type Client struct {
	config     Config
	http       *http.Client
	cacheMu    sync.Mutex
	cache      map[string]cachedEmbedding
	cacheOrder []string
}

type cachedEmbedding struct {
	Vector    []float64
	ExpiresAt time.Time
}

type embeddingsRequest struct {
	Input          any    `json:"input"`
	Model          string `json:"model"`
	Dimensions     int    `json:"dimensions,omitempty"`
	EncodingFormat string `json:"encoding_format,omitempty"`
}

type embeddingsResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func NewClient(config Config) *Client {
	if config.Dimensions <= 0 {
		config.Dimensions = Dimensions
	}
	config.Provider = normalizeProvider(config.Provider)
	if config.Provider == "" {
		config.Provider = "disabled"
	}
	if config.Provider == "local_hash" && config.Model == "" {
		config.Model = "local-hash-ngram-v1"
	}
	if config.Provider == "openai_compatible" && config.EncodingFormat == "" {
		config.EncodingFormat = "float"
	}
	if config.QueryCacheMax < 0 {
		config.QueryCacheMax = 0
	}
	return &Client{
		config: config,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Metadata() Metadata {
	return Metadata{
		Provider:   c.config.Provider,
		Model:      c.config.Model,
		Dimensions: c.config.Dimensions,
		Quality:    Quality(c.config.Provider),
	}
}

func (c *Client) SemanticEnabled() bool {
	return c != nil && c.config.Provider == "openai_compatible" && c.config.BaseURL != "" && c.config.APIKey != "" && c.config.Model != ""
}

func (c *Client) Embed(ctx context.Context, text string) ([]float64, error) {
	if c == nil {
		return nil, errors.New("embedding client is not configured")
	}
	switch c.config.Provider {
	case "local_hash":
		return EmbedWithDimensions(text, c.config.Dimensions), nil
	case "openai_compatible":
		if vec, ok := c.cached(text); ok {
			return vec, nil
		}
		vec, err := c.embedOpenAICompatible(ctx, text)
		if err != nil {
			return nil, err
		}
		c.storeCached(text, vec)
		return vec, nil
	default:
		return nil, fmt.Errorf("embedding provider %q is disabled or unsupported", c.config.Provider)
	}
}

func (c *Client) cached(text string) ([]float64, bool) {
	if c.config.QueryCacheTTL <= 0 || c.config.QueryCacheMax <= 0 {
		return nil, false
	}
	key := c.cacheKey(text)
	now := time.Now()
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if c.cache == nil {
		return nil, false
	}
	item, ok := c.cache[key]
	if !ok {
		return nil, false
	}
	if now.After(item.ExpiresAt) {
		delete(c.cache, key)
		return nil, false
	}
	return cloneVector(item.Vector), true
}

func (c *Client) storeCached(text string, vec []float64) {
	if c.config.QueryCacheTTL <= 0 || c.config.QueryCacheMax <= 0 || len(vec) == 0 {
		return
	}
	key := c.cacheKey(text)
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if c.cache == nil {
		c.cache = make(map[string]cachedEmbedding)
	}
	if _, exists := c.cache[key]; !exists {
		c.cacheOrder = append(c.cacheOrder, key)
	}
	c.cache[key] = cachedEmbedding{
		Vector:    cloneVector(vec),
		ExpiresAt: time.Now().Add(c.config.QueryCacheTTL),
	}
	for len(c.cache) > c.config.QueryCacheMax && len(c.cacheOrder) > 0 {
		oldest := c.cacheOrder[0]
		c.cacheOrder = c.cacheOrder[1:]
		delete(c.cache, oldest)
	}
}

func (c *Client) cacheKey(text string) string {
	return strings.Join([]string{
		c.config.Provider,
		c.config.BaseURL,
		c.config.Model,
		fmt.Sprint(c.config.Dimensions),
		c.config.EncodingFormat,
		text,
	}, "\x00")
}

func cloneVector(vec []float64) []float64 {
	out := make([]float64, len(vec))
	copy(out, vec)
	return out
}

func (c *Client) embedOpenAICompatible(ctx context.Context, text string) ([]float64, error) {
	reqBody := embeddingsRequest{
		Input:          text,
		Model:          c.config.Model,
		Dimensions:     c.config.Dimensions,
		EncodingFormat: c.config.EncodingFormat,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, EmbeddingsURL(c.config.BaseURL), bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		req.Header.Set("Content-Type", "application/json")
		for key, value := range c.config.ExtraHeaders {
			req.Header.Set(key, value)
		}
		res, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if attempt < 3 {
				sleep(ctx, attempt)
				continue
			}
			return nil, err
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode >= 500 && attempt < 3 {
			lastErr = fmt.Errorf("embedding HTTP %d: %s", res.StatusCode, string(body))
			sleep(ctx, attempt)
			continue
		}
		if res.StatusCode >= 300 {
			return nil, fmt.Errorf("embedding HTTP %d: %s", res.StatusCode, string(body))
		}
		var out embeddingsResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, err
		}
		if len(out.Data) == 0 {
			return nil, errors.New("embedding response contained no data")
		}
		vec := out.Data[0].Embedding
		if len(vec) != c.config.Dimensions {
			return nil, fmt.Errorf("embedding dimensions mismatch: got %d, expected %d", len(vec), c.config.Dimensions)
		}
		return vec, nil
	}
	return nil, lastErr
}

func EmbeddingsURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/embeddings"
	}
	return base + "/v1/embeddings"
}

func ParseExtraHeaders(raw string) (map[string]string, error) {
	headers := map[string]string{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, value, ok := strings.Cut(item, ":")
		if !ok {
			return nil, fmt.Errorf("invalid extra header %q, expected Name:value", item)
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name != "" && value != "" {
			headers[name] = value
		}
	}
	return headers, nil
}

func Quality(provider string) string {
	switch normalizeProvider(provider) {
	case "openai_compatible":
		return "semantic"
	case "local_hash":
		return "lexical_hash"
	default:
		return "disabled"
	}
}

func normalizeProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "local-hash-ngram-v1" {
		return "local_hash"
	}
	return provider
}

func sleep(ctx context.Context, attempt int) {
	timer := time.NewTimer(time.Duration(attempt*2) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

var asciiTokenRE = regexp.MustCompile(`[A-Za-z0-9_]+`)

var aliases = map[string][]string{
	"crit_rate":      {"crit_rate", "暴击率", "双暴"},
	"crit_dmg":       {"crit_dmg", "暴击伤害", "爆伤", "暴伤", "双暴"},
	"atk_percent":    {"atk_percent", "攻击力", "攻击百分比", "atk"},
	"dmg_percent":    {"dmg_percent", "增伤", "伤害提高", "造成的伤害提高"},
	"speed":          {"speed", "速度", "spd"},
	"turn_advance":   {"turn_advance", "行动提前", "拉条", "提前行动"},
	"energy_regen":   {"energy_regen", "能量恢复效率", "充能", "回能", "能量"},
	"energy_restore": {"energy_restore", "恢复能量", "回能", "能量"},
	"sp_generation":  {"sp_generation", "战技点上限", "产点", "sp"},
	"sp_recovery":    {"sp_recovery", "恢复战技点", "回点", "sp"},
	"sp_consumption": {"sp_consumption", "消耗战技点", "耗点", "sp"},
	"fua":            {"fua", "追加攻击", "追击", "fua_team", "fua_dmg"},
	"break":          {"break", "击破", "超击破", "削韧", "break_specialist"},
	"dot":            {"dot", "持续伤害", "dot_enabler", "dot_detonator"},
	"debuff":         {"debuff", "负面效果", "减防", "易伤", "debuffer"},
	"sustain":        {"sustain", "治疗", "护盾", "生存", "sustain_healer", "sustain_shielder"},
	"main_dps":       {"main_dps", "主c", "主 C", "输出位"},
	"sub_dps":        {"sub_dps", "副c", "副 C", "副输出"},
	"amplifier":      {"amplifier", "同谐", "辅助", "拐"},
}

func Embed(text string) []float64 {
	return EmbedWithDimensions(text, Dimensions)
}

func EmbedWithDimensions(text string, dimensions int) []float64 {
	if dimensions <= 0 {
		dimensions = Dimensions
	}
	normalized := strings.ToLower(text)
	vec := make([]float64, dimensions)
	for _, token := range asciiTokenRE.FindAllString(normalized, -1) {
		addFeature(vec, "tok:"+token, 2.0)
		for _, part := range strings.Split(token, "_") {
			if part != "" {
				addFeature(vec, "tok:"+part, 1.0)
			}
		}
	}
	for _, segment := range cjkSegments(normalized) {
		runes := []rune(segment)
		for _, char := range runes {
			addFeature(vec, "c1:"+string(char), 0.15)
		}
		for _, spec := range []struct {
			n      int
			weight float64
		}{{2, 0.8}, {3, 1.0}, {4, 0.6}} {
			if len(runes) < spec.n {
				continue
			}
			for i := 0; i <= len(runes)-spec.n; i++ {
				addFeature(vec, fmt.Sprintf("c%d:%s", spec.n, string(runes[i:i+spec.n])), spec.weight)
			}
		}
	}
	for feature, weight := range aliasFeatures(normalized) {
		addFeature(vec, feature, weight)
	}

	var norm float64
	for _, value := range vec {
		norm += value * value
	}
	if norm == 0 {
		return vec
	}
	norm = math.Sqrt(norm)
	for i, value := range vec {
		vec[i] = value / norm
	}
	return vec
}

func VectorLiteral(vec []float64) string {
	parts := make([]string, len(vec))
	for i, value := range vec {
		parts[i] = fmt.Sprintf("%.8f", value)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func addFeature(vec []float64, feature string, weight float64) {
	if feature == "" {
		return
	}
	digest := sha256.Sum256([]byte(feature))
	bucket := int(uint32(digest[0])<<24|uint32(digest[1])<<16|uint32(digest[2])<<8|uint32(digest[3])) % len(vec)
	sign := -1.0
	if digest[4]&1 == 1 {
		sign = 1.0
	}
	vec[bucket] += sign * weight
}

func aliasFeatures(text string) map[string]float64 {
	out := make(map[string]float64)
	compact := compactSpaces(text)
	for canonical, terms := range aliases {
		found := false
		for _, term := range terms {
			if strings.Contains(compact, compactSpaces(strings.ToLower(term))) {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		out["axis:"+canonical] += 6.0
		for _, term := range terms {
			out["alias:"+compactSpaces(strings.ToLower(term))] += 2.0
		}
	}
	return out
}

func compactSpaces(text string) string {
	return strings.Join(strings.Fields(text), "")
}

func cjkSegments(text string) []string {
	var out []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			out = append(out, string(current))
			current = nil
		}
	}
	for _, r := range text {
		if isCJK(r) {
			current = append(current, r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func isCJK(r rune) bool {
	return (r >= 0x3400 && r <= 0x9fff) || unicode.Is(unicode.Han, r)
}
