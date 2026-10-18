package service

import (
	"strings"
	"testing"

	"novel-agent/internal/config"
)

func TestNewServiceRejectsInvalidConfigurationBeforeClientConstruction(t *testing.T) {
	cfg := config.Default()
	cfg.Database.Password = "test-password"
	cfg.LLM.Provider = "unsupported-provider"
	originalProvider := cfg.LLM.Provider
	result, err := NewService(cfg)
	if err == nil || result != nil || !strings.Contains(err.Error(), "service configuration is invalid") {
		t.Fatalf("NewService(invalid) returned service=%#v err=%v", result, err)
	}
	if cfg.LLM.Provider != originalProvider {
		t.Fatalf("NewService mutated caller config: provider=%q", cfg.LLM.Provider)
	}
}

func TestNewServiceRejectsInlineRemoteCredentialsWithoutKeyFileProvenance(t *testing.T) {
	for name, mutate := range map[string]func(*config.Config){
		"LLM": func(cfg *config.Config) {
			cfg.LLM.Provider = "openai"
			cfg.LLM.BaseURL = "https://api.example.com/v1"
			cfg.LLM.APIKeyFile = "claimed-but-unread.key"
			cfg.LLM.APIKey = "inline-secret"
		},
		"embedding": func(cfg *config.Config) {
			cfg.LLM.Provider = "ollama"
			cfg.Embedding.Provider = "modelscope"
			cfg.Embedding.BaseURL = "https://api.example.com/v1"
			cfg.Embedding.APIKeyFile = "claimed-but-unread.key"
			cfg.Embedding.APIKey = "inline-secret"
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Database.Password = "test-password"
			mutate(cfg)
			result, err := NewService(cfg)
			if err == nil || result != nil || !strings.Contains(err.Error(), "credentials are not loaded from a key file") {
				t.Fatalf("service=%#v err=%v", result, err)
			}
			if strings.Contains(err.Error(), "inline-secret") {
				t.Fatalf("credential error leaked key: %v", err)
			}
		})
	}
}

func TestNewServiceClearsStaleEmbeddingKeyForKeylessLocalConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Database.Password = "test-password"
	cfg.LLM.Provider = "ollama"
	cfg.Embedding.APIKeyFile = ""
	cfg.Embedding.APIKey = "stale-secret"
	result, err := NewService(cfg)
	if err != nil || result == nil {
		t.Fatalf("service=%#v err=%v", result, err)
	}
	if result.Config.Embedding.APIKey != "" {
		t.Fatal("service retained stale embedding key for keyless local endpoint")
	}
	if cfg.Embedding.APIKey != "stale-secret" {
		t.Fatal("service credential cleanup mutated caller config")
	}
}

func TestNewServiceRejectsNilConfiguration(t *testing.T) {
	result, err := NewService(nil)
	if err == nil || result != nil {
		t.Fatalf("NewService(nil) returned service=%#v err=%v", result, err)
	}
	if err.Error() != "service configuration is required" {
		t.Fatalf("unexpected nil configuration error: %v", err)
	}
}
