package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAPIKeyNormalizesProviderName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.key"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.LLM.Provider = " OpenAI "
	cfg.LLM.APIKeyFile = "openai.key"
	if err := loadAPIKey(cfg, filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Provider != "openai" || cfg.LLM.APIKey != "secret" {
		t.Fatalf("provider or key not normalized: %#v", cfg.LLM)
	}
}

func TestLoadAPIKeyFromConfigRelativeFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "modelscope.key")
	if err := os.WriteFile(keyPath, []byte(" secret-value \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Default()
	cfg.LLM.Provider = "modelscope"
	cfg.LLM.APIKeyFile = "modelscope.key"
	if err := loadAPIKey(cfg, filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatalf("loadAPIKey returned error: %v", err)
	}
	if cfg.LLM.APIKey != "secret-value" {
		t.Fatalf("API key was not trimmed and loaded")
	}
}

func TestLoadAPIKeyRejectsMissingOrEmptyFile(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.LLM.Provider = "modelscope"
	cfg.LLM.APIKeyFile = "missing.key"
	if err := loadAPIKey(cfg, filepath.Join(dir, "config.yaml")); err == nil || !strings.Contains(err.Error(), "failed to read") {
		t.Fatalf("expected missing-file error, got %v", err)
	}

	emptyPath := filepath.Join(dir, "empty.key")
	if err := os.WriteFile(emptyPath, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.LLM.APIKeyFile = "empty.key"
	if err := loadAPIKey(cfg, filepath.Join(dir, "config.yaml")); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-file error, got %v", err)
	}
}

func TestValidateRequiresDatabasePassword(t *testing.T) {
	cfg := Default()
	cfg.LLM.Provider = "ollama"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "database.password is required") {
		t.Fatalf("expected missing database password error, got %v", err)
	}
	cfg.Database.Password = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid database password rejected: %v", err)
	}
}

func TestValidateRejectsUnknownProviderAndInvalidBounds(t *testing.T) {
	cfg := Default()
	cfg.Database.Password = "secret"
	cfg.LLM.Provider = "unknown"
	cfg.LLM.BaseURL = "not-a-url"
	cfg.LLM.Temperature = 3
	cfg.LLM.MaxTokens = 0
	cfg.Embedding.Dimension = 0
	cfg.Server.Port = 70000
	err := cfg.Validate()
	for _, expected := range []string{"llm.provider", "llm.base_url", "llm.temperature", "llm.max_tokens", "embedding.dimension", "server.port"} {
		if err == nil || !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected %s validation error, got %v", expected, err)
		}
	}
}

func TestProviderSpecificBaseURLOverridesDoNotLeakAcrossProviders(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", "https://generic.example/v1")
	t.Setenv("MODELSCOPE_BASE_URL", "https://modelscope.example/v1")
	t.Setenv("OPENAI_BASE_URL", "https://openai.example/v1")
	cfg := Default()
	loadEnvOverrides(cfg)
	if cfg.LLM.Provider != "openai" || cfg.LLM.BaseURL != "https://openai.example/v1" {
		t.Fatalf("unexpected provider override: %#v", cfg.LLM)
	}
}

func TestLoadAPIKeyNotRequiredForLocalProvider(t *testing.T) {
	cfg := Default()
	cfg.LLM.Provider = "ollama"
	cfg.LLM.APIKeyFile = "missing.key"
	if err := loadAPIKey(cfg, filepath.Join(t.TempDir(), "config.yaml")); err != nil {
		t.Fatalf("local provider should not require key file: %v", err)
	}
}
