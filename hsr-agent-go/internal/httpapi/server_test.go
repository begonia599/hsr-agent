package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hsr-agent-go/internal/agent"
	"hsr-agent-go/internal/config"
)

func TestHealthWithoutDB(t *testing.T) {
	server := New(config.Config{EmbeddingProvider: "disabled", EmbeddingDimensions: 1024}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	res := httptest.NewRecorder()

	server.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusOK, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"status":"unconfigured"`) {
		t.Fatalf("health body should report unconfigured db: %s", res.Body.String())
	}
}

func TestSemanticSearchHTTPDisabled(t *testing.T) {
	server := New(config.Config{EmbeddingProvider: "local_hash"}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/search/semantic?q=test", nil)
	res := httptest.NewRecorder()

	server.ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusServiceUnavailable, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "SEMANTIC_SEARCH_DISABLED") {
		t.Fatalf("semantic disabled error missing: %s", res.Body.String())
	}
}

func TestModelsDoesNotLeakSecrets(t *testing.T) {
	cfg := config.Config{
		DefaultEmbeddingID:       "bge-m3",
		EmbeddingCacheTTLSeconds: 600,
		EmbeddingCacheMaxEntries: 256,
		EmbeddingModels: []config.EmbeddingModel{
			{
				ID:         "bge-m3",
				Label:      "bge-m3",
				Provider:   "openai_compatible",
				BaseURL:    "https://example.test/v1",
				APIKey:     "secret-embedding-key",
				Model:      "bge-m3",
				Dimensions: 1024,
			},
		},
		DefaultRerankID: "bge-reranker-v2-m3",
		RerankModels: []config.RerankModel{
			{
				ID:       "bge-reranker-v2-m3",
				Label:    "bge-reranker-v2-m3",
				Provider: "openai_compatible",
				BaseURL:  "https://example.test/v1",
				APIKey:   "secret-rerank-key",
				Model:    "bge-reranker-v2-m3",
			},
		},
	}
	server := New(cfg, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	res := httptest.NewRecorder()

	server.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusOK, res.Body.String())
	}
	body := res.Body.String()
	for _, secret := range []string{"secret-embedding-key", "secret-rerank-key"} {
		if strings.Contains(body, secret) {
			t.Fatalf("models response leaked secret %q: %s", secret, body)
		}
	}
	for _, expected := range []string{"bge-m3", "bge-reranker-v2-m3", "default_id"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("models response missing %q: %s", expected, body)
		}
	}
	for _, expected := range []string{"query_cache", "ttl_seconds", "max_entries"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("models response missing query cache field %q: %s", expected, body)
		}
	}
}

func TestNewTraceID(t *testing.T) {
	first := newTraceID()
	second := newTraceID()
	if len(first) < 8 {
		t.Fatalf("trace id too short: %q", first)
	}
	if first == second {
		t.Fatalf("trace ids should differ, both were %q", first)
	}
}

func TestToolTraceCollectorMergesCallAndResult(t *testing.T) {
	collector := newToolTraceCollector()
	collector.Add(agent.Event{
		Type:       "tool_call",
		ToolCallID: "call_1",
		Name:       "get_character",
		Args:       json.RawMessage(`{"query":"花火"}`),
	})
	collector.Add(agent.Event{
		Type:       "tool_result",
		ToolCallID: "call_1",
		Name:       "get_character",
		Result:     map[string]any{"id": 1306, "name_zh": "花火"},
		LatencyMS:  17,
	})

	calls := collector.Calls()
	if len(calls) != 1 {
		t.Fatalf("len = %d, want 1", len(calls))
	}
	if calls[0].Seq != 0 || calls[0].ToolCallID != "call_1" || calls[0].Name != "get_character" {
		t.Fatalf("unexpected call identity: %#v", calls[0])
	}
	if calls[0].LatencyMS != 17 {
		t.Fatalf("latency = %d, want 17", calls[0].LatencyMS)
	}
	if !strings.Contains(string(calls[0].Args), "花火") || !strings.Contains(string(calls[0].Result), "1306") {
		t.Fatalf("args/result not preserved: args=%s result=%s", calls[0].Args, calls[0].Result)
	}
}

func TestConversationHistoryRequiresDB(t *testing.T) {
	server := New(config.Config{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
	res := httptest.NewRecorder()

	server.ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusServiceUnavailable, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "DB_UNAVAILABLE") {
		t.Fatalf("missing DB_UNAVAILABLE error: %s", res.Body.String())
	}
}

func TestStaticSPAFallback(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, "index.html")
	if err := os.WriteFile(indexPath, []byte("<!doctype html><div id=\"app\"></div>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{WebRoot: root}, nil, nil)

	assetReq := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	assetRes := httptest.NewRecorder()
	server.ServeHTTP(assetRes, assetReq)
	if assetRes.Code != http.StatusOK || strings.TrimSpace(assetRes.Body.String()) != "asset" {
		t.Fatalf("asset response = %d %q", assetRes.Code, assetRes.Body.String())
	}

	routeReq := httptest.NewRequest(http.MethodGet, "/characters/1310", nil)
	routeRes := httptest.NewRecorder()
	server.ServeHTTP(routeRes, routeReq)
	if routeRes.Code != http.StatusOK {
		t.Fatalf("fallback status = %d, want %d; body=%s", routeRes.Code, http.StatusOK, routeRes.Body.String())
	}
	if !strings.Contains(routeRes.Body.String(), `id="app"`) {
		t.Fatalf("fallback should serve index.html: %s", routeRes.Body.String())
	}
}
