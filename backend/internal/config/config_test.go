package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictYAMLRejectsUnknownFieldsWithoutEchoingValues(t *testing.T) {
	secretLikeValue := "credential-shaped-private-value"
	cfg := Default()
	err := decodeStrictConfig([]byte("server:\n  max_concurent_ai: 4\ndatabase:\n  password: "+secretLikeValue+"\n"), cfg)
	if err == nil {
		t.Fatal("unknown YAML field was accepted")
	}
	if strings.Contains(err.Error(), secretLikeValue) {
		t.Fatalf("YAML value leaked in error: %v", err)
	}
}

func TestStrictYAMLRejectsMultipleDocuments(t *testing.T) {
	cfg := Default()
	err := decodeStrictConfig([]byte("server:\n  port: 8080\n---\nserver:\n  port: 9090\n"), cfg)
	if err == nil || !strings.Contains(err.Error(), "exactly one YAML document") {
		t.Fatalf("expected multiple-document error, got %v", err)
	}
}

func TestStrictYAMLAllowsEmptyAndKnownSingleDocument(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("server:\n  port: 9090\n")} {
		cfg := Default()
		if err := decodeStrictConfig(data, cfg); err != nil {
			t.Fatalf("valid YAML rejected: %v", err)
		}
	}
}

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
	cfg.Database.User = ""
	cfg.Database.SSLMode = "disable statement_timeout=0"
	cfg.Database.SearchPath = "novel_agent,public;DROP"
	cfg.LLM.Provider = "unknown"
	cfg.LLM.BaseURL = "not-a-url"
	cfg.LLM.Temperature = 3
	cfg.LLM.MaxTokens = 0
	cfg.Embedding.Dimension = 0
	cfg.Database.MaxOpenConns = 0
	cfg.Database.MaxIdleConns = 1
	cfg.Database.ConnMaxLifetimeSec = 0
	cfg.Database.ConnMaxIdleTimeSec = 0
	cfg.Database.StatementTimeoutMS = 0
	cfg.Database.ConnectTimeoutSec = 0
	cfg.Server.Port = 70000
	cfg.Server.MaxConcurrentAI = 0
	cfg.Server.MaxHeaderBytes = 1024
	err := cfg.Validate()
	for _, expected := range []string{"llm.provider", "llm.base_url", "llm.temperature", "llm.max_tokens", "embedding.dimension", "database.user", "database.sslmode", "database.search_path", "database.max_open_conns", "database.max_idle_conns", "database.conn_max_lifetime_seconds", "database.conn_max_idle_time_seconds", "database.statement_timeout_ms", "database.connect_timeout_seconds", "server.port", "server.max_concurrent_ai", "server.max_header_bytes"} {
		if err == nil || !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected %s validation error, got %v", expected, err)
		}
	}
}

func TestValidSearchPathAcceptsBoundedIdentifierListsOnly(t *testing.T) {
	for _, value := range []string{"novel_agent,public", "tenant_1, public", "schema$1"} {
		if !validSearchPath(value) {
			t.Errorf("valid search path rejected: %q", value)
		}
	}
	for _, value := range []string{"", "public;DROP TABLE x", `"quoted"`, "1invalid", "public,,other"} {
		if validSearchPath(value) {
			t.Errorf("invalid search path accepted: %q", value)
		}
	}
}

func TestDatabaseTimeoutEnvironmentOverrides(t *testing.T) {
	t.Setenv("DB_MAX_OPEN_CONNS", "40")
	t.Setenv("DB_MAX_IDLE_CONNS", "8")
	t.Setenv("DB_CONN_MAX_LIFETIME_SECONDS", "2400")
	t.Setenv("DB_CONN_MAX_IDLE_TIME_SECONDS", "400")
	t.Setenv("DB_STATEMENT_TIMEOUT_MS", "25000")
	t.Setenv("DB_CONNECT_TIMEOUT_SECONDS", "12")
	cfg := Default()
	if err := loadEnvOverrides(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Database.MaxOpenConns != 40 || cfg.Database.MaxIdleConns != 8 || cfg.Database.ConnMaxLifetimeSec != 2400 || cfg.Database.ConnMaxIdleTimeSec != 400 || cfg.Database.StatementTimeoutMS != 25000 || cfg.Database.ConnectTimeoutSec != 12 {
		t.Fatalf("unexpected database pool configuration: %#v", cfg.Database)
	}
}

func TestAIConcurrencyEnvironmentOverride(t *testing.T) {
	t.Setenv("API_MAX_CONCURRENT_AI", "7")
	cfg := Default()
	if err := loadEnvOverrides(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.MaxConcurrentAI != 7 {
		t.Fatalf("AI concurrency limit = %d, want 7", cfg.Server.MaxConcurrentAI)
	}
}

func TestHeaderLimitEnvironmentOverride(t *testing.T) {
	t.Setenv("API_MAX_HEADER_BYTES", "32768")
	cfg := Default()
	if err := loadEnvOverrides(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.MaxHeaderBytes != 32768 {
		t.Fatalf("header byte limit = %d, want 32768", cfg.Server.MaxHeaderBytes)
	}
}

func TestInvalidNumericEnvironmentOverrideFailsWithoutEchoingValue(t *testing.T) {
	secretLikeValue := "not-an-integer-secret"
	t.Setenv("API_MAX_HEADER_BYTES", secretLikeValue)
	cfg := Default()
	err := loadEnvOverrides(cfg)
	if err == nil || !strings.Contains(err.Error(), "API_MAX_HEADER_BYTES") {
		t.Fatalf("expected named parse error, got %v", err)
	}
	if strings.Contains(err.Error(), secretLikeValue) {
		t.Fatalf("environment value leaked in error: %v", err)
	}
	if cfg.Server.MaxHeaderBytes != 64<<10 {
		t.Fatalf("invalid override mutated config: %d", cfg.Server.MaxHeaderBytes)
	}
}

func TestProviderSpecificBaseURLOverridesDoNotLeakAcrossProviders(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", "https://generic.example/v1")
	t.Setenv("MODELSCOPE_BASE_URL", "https://modelscope.example/v1")
	t.Setenv("OPENAI_BASE_URL", "https://openai.example/v1")
	cfg := Default()
	if err := loadEnvOverrides(cfg); err != nil {
		t.Fatal(err)
	}
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
