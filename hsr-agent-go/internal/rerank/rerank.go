package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Provider     string
	BaseURL      string
	APIKey       string
	Model        string
	ExtraHeaders map[string]string
}

type Metadata struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type Result struct {
	Index int
	Score float64
}

type Client struct {
	config Config
	http   *http.Client
}

type requestBody struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

type responseBody struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

func NewClient(config Config) *Client {
	config.Provider = normalizeProvider(config.Provider)
	if config.Provider == "" {
		config.Provider = "disabled"
	}
	return &Client{
		config: config,
		http:   &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *Client) Metadata() Metadata {
	if c == nil {
		return Metadata{}
	}
	return Metadata{Provider: c.config.Provider, Model: c.config.Model}
}

func (c *Client) Enabled() bool {
	return c != nil &&
		c.config.Provider == "openai_compatible" &&
		c.config.BaseURL != "" &&
		c.config.APIKey != "" &&
		c.config.Model != ""
}

func (c *Client) Rerank(ctx context.Context, query string, documents []string, topN int) ([]Result, error) {
	if !c.Enabled() {
		return nil, errors.New("reranker client is not configured")
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("query is required")
	}
	if len(documents) == 0 {
		return nil, nil
	}
	if topN <= 0 || topN > len(documents) {
		topN = len(documents)
	}

	payload := requestBody{
		Model:     c.config.Model,
		Query:     query,
		Documents: documents,
		TopN:      topN,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, RerankURL(c.config.BaseURL), bytes.NewReader(data))
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
			if attempt < 2 {
				sleep(ctx, attempt)
				continue
			}
			return nil, err
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode >= 500 && attempt < 2 {
			lastErr = fmt.Errorf("rerank HTTP %d: %s", res.StatusCode, string(body))
			sleep(ctx, attempt)
			continue
		}
		if res.StatusCode >= 300 {
			return nil, fmt.Errorf("rerank HTTP %d: %s", res.StatusCode, string(body))
		}
		var decoded responseBody
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, err
		}
		out := make([]Result, 0, len(decoded.Results))
		seen := map[int]bool{}
		for _, item := range decoded.Results {
			if item.Index < 0 || item.Index >= len(documents) || seen[item.Index] {
				continue
			}
			seen[item.Index] = true
			out = append(out, Result{Index: item.Index, Score: item.RelevanceScore})
		}
		return out, nil
	}
	return nil, lastErr
}

func RerankURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/rerank"
	}
	return base + "/v1/rerank"
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

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func sleep(ctx context.Context, attempt int) {
	timer := time.NewTimer(time.Duration(attempt*2) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
