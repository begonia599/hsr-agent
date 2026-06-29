package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRerankOpenAICompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" {
			t.Fatalf("path = %s, want /v1/rerank", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q", got)
		}
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "bge-reranker-v2-m3" || body.Query != "击破辅助" || len(body.Documents) != 2 || body.TopN != 2 {
			t.Fatalf("unexpected request body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"index":1,"relevance_score":0.91},{"index":0,"relevance_score":0.12}]}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Provider: "openai_compatible",
		BaseURL:  server.URL,
		APIKey:   "test-key",
		Model:    "bge-reranker-v2-m3",
	})
	results, err := client.Rerank(context.Background(), "击破辅助", []string{"doc0", "doc1"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Index != 1 || results[0].Score != 0.91 {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestRerankURL(t *testing.T) {
	if got := RerankURL("https://api.example.com/v1"); got != "https://api.example.com/v1/rerank" {
		t.Fatalf("RerankURL with /v1 = %s", got)
	}
	if got := RerankURL("https://api.example.com"); got != "https://api.example.com/v1/rerank" {
		t.Fatalf("RerankURL without /v1 = %s", got)
	}
}
