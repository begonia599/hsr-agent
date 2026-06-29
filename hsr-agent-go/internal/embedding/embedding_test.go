package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAICompatibleEmbeddingQueryCache(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %s, want /v1/embeddings", r.URL.Path)
		}
		atomic.AddInt32(&requests, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1, 0.2, 0.3}},
			},
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		Provider:       "openai_compatible",
		BaseURL:        server.URL,
		APIKey:         "test-key",
		Model:          "test-embedding",
		Dimensions:     3,
		EncodingFormat: "float",
		QueryCacheTTL:  time.Minute,
		QueryCacheMax:  8,
	})

	first, err := client.Embed(context.Background(), "击破辅助")
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 9.9
	second, err := client.Embed(context.Background(), "击破辅助")
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	if second[0] != 0.1 {
		t.Fatalf("cached vector was mutated, got first value %v", second[0])
	}
}

func TestOpenAICompatibleEmbeddingCacheCanExpire(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1, 0.2}},
			},
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		Provider:      "openai_compatible",
		BaseURL:       server.URL,
		APIKey:        "test-key",
		Model:         "test-embedding",
		Dimensions:    2,
		QueryCacheTTL: time.Nanosecond,
		QueryCacheMax: 8,
	})

	if _, err := client.Embed(context.Background(), "花火"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if _, err := client.Embed(context.Background(), "花火"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("requests = %d, want 2 after cache expiry", got)
	}
}
