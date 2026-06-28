package config

import "os"

type Config struct {
	DatabaseURL         string
	LLMBaseURL          string
	LLMAPIKey           string
	LLMModel            string
	LLMAPIFormat        string
	HTTPAddr            string
	WebRoot             string
	EmbeddingProvider   string
	EmbeddingModel      string
	EmbeddingDimensions int
}

func Load() Config {
	return Config{
		DatabaseURL:         getenv("DATABASE_URL", "postgresql://hsr:hsr@localhost:55432/hsr_agent"),
		LLMBaseURL:          getenv("LLM_BASE_URL", "https://api.deepseek.com"),
		LLMAPIKey:           os.Getenv("LLM_API_KEY"),
		LLMModel:            getenv("LLM_MODEL", "deepseek-chat"),
		LLMAPIFormat:        getenv("LLM_API_FORMAT", "openai"),
		HTTPAddr:            getenv("HTTP_ADDR", "127.0.0.1:8080"),
		WebRoot:             getenv("WEB_ROOT", "web/dist"),
		EmbeddingProvider:   getenv("EMBEDDING_PROVIDER", "disabled"),
		EmbeddingModel:      getenv("EMBEDDING_MODEL", ""),
		EmbeddingDimensions: getenvInt("EMBEDDING_DIMENSIONS", 1024),
	}
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
