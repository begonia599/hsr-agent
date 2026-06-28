package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
