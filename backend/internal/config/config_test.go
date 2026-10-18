package config

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type credentialReadCloser struct {
	reader   io.Reader
	closeErr error
}

func (r credentialReadCloser) Read(buffer []byte) (int, error) { return r.reader.Read(buffer) }
func (r credentialReadCloser) Close() error                    { return r.closeErr }

type credentialErrorReader struct{}

func (credentialErrorReader) Read([]byte) (int, error) { return 0, errors.New("secret read detail") }

func TestReadCredentialRejectsInvalidUTF8(t *testing.T) {
	file := credentialReadCloser{reader: strings.NewReader(string([]byte{0xff, 0xfe}))}
	data, err := readCredential(file, "embedding")
	if err == nil || data != nil || err.Error() != "embedding API key file must contain valid UTF-8" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestReadCredentialRejectsReadAndCloseFailures(t *testing.T) {
	for name, file := range map[string]io.ReadCloser{
		"read":  credentialReadCloser{reader: credentialErrorReader{}},
		"close": credentialReadCloser{reader: strings.NewReader("secret-token"), closeErr: errors.New("secret close detail")},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := readCredential(file, "llm")
			if err == nil || data != nil || err.Error() != "failed to read llm API key file" {
				t.Fatalf("data=%q err=%v", data, err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("credential error leaked details: %v", err)
			}
		})
	}
}

func TestValidateDoesNotNormalizeCallerOnFailure(t *testing.T) {
	cfg := Default()
	cfg.Database.Password = "test-password"
	cfg.LLM.Provider = " OPENAI "
	cfg.LLM.Model = "   "
	originalProvider := cfg.LLM.Provider
	originalModel := cfg.LLM.Model
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid configuration was accepted")
	}
	if cfg.LLM.Provider != originalProvider || cfg.LLM.Model != originalModel {
		t.Fatalf("failed validation mutated caller config: provider=%q model=%q", cfg.LLM.Provider, cfg.LLM.Model)
	}
}

func TestValidateClearsStaleLLMKeyForLocalProvider(t *testing.T) {
	cfg := Default()
	cfg.Database.Password = "test-password"
	cfg.LLM.Provider = "ollama"
	cfg.LLM.APIKey = "stale-remote-secret"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.APIKey != "" {
		t.Fatal("local LLM provider retained stale remote API key")
	}
}

func TestValidatePreservesLoadedKeyForRemoteProvider(t *testing.T) {
	cfg := Default()
	cfg.Database.Password = "test-password"
	cfg.LLM.Provider = "openai"
	cfg.LLM.BaseURL = "https://api.example.com/v1"
	cfg.LLM.APIKeyFile = "openai.key"
	cfg.LLM.APIKey = "loaded-secret"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.APIKey != "loaded-secret" {
		t.Fatal("remote LLM provider key was unexpectedly cleared")
	}
}

func TestValidateCommitsNormalizationOnlyOnSuccess(t *testing.T) {
	cfg := Default()
	cfg.Database.Password = "test-password"
	cfg.LLM.Provider = " OLLAMA "
	cfg.LLM.Model = " model "
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Provider != "ollama" || cfg.LLM.Model != "model" {
		t.Fatalf("successful validation did not commit normalization: %#v", cfg.LLM)
	}
}

func TestValidateRejectsNilConfigurationWithoutPanic(t *testing.T) {
	var cfg *Config
	if err := cfg.Validate(); err == nil || err.Error() != "configuration is required" {
		t.Fatalf("unexpected nil configuration error: %v", err)
	}
}

func TestValidConfigTextRejectsInvalidUTF8Strings(t *testing.T) {
	invalid := string([]byte{'v', 0xff, 'x'})
	if validConfigText(invalid, 100, true) || validConfigText(invalid, 100, false) {
		t.Fatal("invalid UTF-8 configuration text was accepted")
	}
	cfg := Default()
	cfg.Database.Password = "test-password"
	cfg.LLM.Model = invalid
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid UTF-8 model name passed configuration validation")
	}
}

func TestLoadRejectsInvalidUTF8ConfigurationBeforeEnvironmentOrCredentialAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := append([]byte("database:\n  password: test-password\nserver:\n  host: "), 0xff, 0xfe)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_HOST", "environment-host-that-must-not-be-applied")
	cfg, err := Load(path)
	if err == nil || cfg != nil || err.Error() != "configuration file must contain valid UTF-8" {
		t.Fatalf("cfg=%#v err=%v", cfg, err)
	}
	if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "environment-host") {
		t.Fatalf("configuration UTF-8 error leaked input: %v", err)
	}
}

func TestLoadRejectsMissingConfigurationFileWithoutPathDisclosure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-secret-named-config.yaml")
	cfg, err := Load(path)
	if err == nil || cfg != nil || err.Error() != "failed to open configuration file" {
		t.Fatalf("cfg=%#v err=%v", cfg, err)
	}
	if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "missing-secret") {
		t.Fatalf("configuration open error leaked path: %v", err)
	}
}

func TestLoadConfigUsesBoundedPathSafeReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	valid := "llm:\n  provider: ollama\n"
	t.Setenv("DB_PASSWORD", "test-password")
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.LLM.Provider != "ollama" {
		t.Fatalf("valid bounded config failed: cfg=%#v err=%v", cfg, err)
	}
	if cfg, err := Load("  " + path + "  "); err == nil || cfg != nil || err.Error() != "failed to open configuration file" {
		t.Fatalf("configuration path whitespace was silently rewritten: cfg=%#v err=%v", cfg, err)
	}

	if err := os.WriteFile(path, []byte(strings.Repeat("x", int(maxConfigFileBytes)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "1048576 byte limit") || strings.Contains(err.Error(), path) {
		t.Fatalf("unexpected oversized config error: %v", err)
	}

	secretLikePath := "config\ncredential-shaped-private-value"
	if _, err := Load(secretLikePath); err == nil || !strings.Contains(err.Error(), "configuration path") || strings.Contains(err.Error(), secretLikePath) {
		t.Fatalf("unexpected invalid path error: %v", err)
	}
}

func TestLoadValidatesBeforeReadingAPIKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := "database:\n  host: ''\n  password: test-password\nllm:\n  provider: modelscope\n  api_key_file: missing-secret.key\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "database.host") {
		t.Fatalf("expected config validation error, got %v", err)
	}
	if strings.Contains(err.Error(), "API key file") || strings.Contains(err.Error(), "missing-secret.key") {
		t.Fatalf("credential file was accessed before validation: %v", err)
	}
}

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

func TestStrictYAMLRejectsEmptyOrNonObjectDocuments(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":         nil,
		"comments only": []byte("# no settings\n"),
		"null":          []byte("null\n"),
		"array":         []byte("- server\n"),
		"empty object":  []byte("{}\n"),
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Default()
			if err := decodeStrictConfig(data, cfg); err == nil || err.Error() != "configuration file must contain one non-empty YAML object" {
				t.Fatalf("unexpected empty/non-object YAML result: %v", err)
			}
		})
	}
}

func TestStrictYAMLRejectsDuplicateKeysWithoutDisclosure(t *testing.T) {
	secretLikeKey := "private_duplicate_key"
	data := []byte("server:\n  " + secretLikeKey + ": first\n  " + secretLikeKey + ": second\n")
	cfg := Default()
	err := decodeStrictConfig(data, cfg)
	if err == nil || err.Error() != "configuration file contains duplicate YAML keys" {
		t.Fatalf("unexpected duplicate key result: %v", err)
	}
	if strings.Contains(err.Error(), secretLikeKey) || strings.Contains(err.Error(), "first") || strings.Contains(err.Error(), "second") {
		t.Fatalf("duplicate YAML error leaked input: %v", err)
	}
}

func TestConfigYAMLRejectsNonScalarMappingKeys(t *testing.T) {
	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.MappingNode},
		{Kind: yaml.ScalarNode, Value: "value"},
	}}
	nodes := 0
	if err := validateConfigYAMLNode(node, 0, &nodes); err == nil || err.Error() != "configuration file YAML keys must be scalar values" {
		t.Fatalf("unexpected non-scalar key result: %v", err)
	}
}

func TestStrictYAMLRejectsAliasesAndAnchors(t *testing.T) {
	secretLikeAnchor := "private_anchor_name"
	data := []byte("server: &" + secretLikeAnchor + "\n  port: 9090\ncopy: *" + secretLikeAnchor + "\n")
	cfg := Default()
	err := decodeStrictConfig(data, cfg)
	if err == nil || err.Error() != "configuration file YAML aliases and anchors are not supported" {
		t.Fatalf("unexpected alias rejection: %v", err)
	}
	if strings.Contains(err.Error(), secretLikeAnchor) {
		t.Fatalf("YAML anchor leaked in error: %v", err)
	}
}

func TestConfigYAMLStructureLimits(t *testing.T) {
	deep := &yaml.Node{}
	current := deep
	for range maxConfigYAMLDepth + 1 {
		child := &yaml.Node{}
		current.Content = []*yaml.Node{child}
		current = child
	}
	nodes := 0
	if err := validateConfigYAMLNode(deep, 0, &nodes); err == nil || err.Error() != "configuration file YAML nesting is too deep" {
		t.Fatalf("unexpected depth result: %v", err)
	}

	wide := &yaml.Node{Content: make([]*yaml.Node, maxConfigYAMLNodes)}
	for index := range wide.Content {
		wide.Content[index] = &yaml.Node{}
	}
	nodes = 0
	if err := validateConfigYAMLNode(wide, 0, &nodes); err == nil || err.Error() != "configuration file YAML structure is too complex" {
		t.Fatalf("unexpected node limit result: %v", err)
	}
}

func TestStrictYAMLAllowsKnownSingleObjectDocument(t *testing.T) {
	cfg := Default()
	if err := decodeStrictConfig([]byte("server:\n  port: 9090\n"), cfg); err != nil {
		t.Fatalf("valid YAML rejected: %v", err)
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

func TestCredentialProvenanceIsSetOnlyAfterSuccessfulFileRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "llm.key"), []byte("llm-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "embedding.key"), []byte("embedding-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.LLM.Provider = "openai"
	cfg.LLM.APIKeyFile = "llm.key"
	cfg.Embedding.APIKeyFile = "embedding.key"
	if cfg.LLMCredentialsLoadedFromFile() || cfg.EmbeddingCredentialsLoadedFromFile() {
		t.Fatal("credentials claimed file provenance before loading")
	}
	configPath := filepath.Join(dir, "config.yaml")
	if err := loadAPIKey(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	if err := loadEmbeddingAPIKey(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	if !cfg.LLMCredentialsLoadedFromFile() || !cfg.EmbeddingCredentialsLoadedFromFile() {
		t.Fatal("successful credential reads did not record provenance")
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

func TestLoadAPIKeyRejectsInvalidPathBeforeFileAccess(t *testing.T) {
	cfg := Default()
	cfg.LLM.Provider = "modelscope"
	for _, path := range []string{"   ", "key\npath", strings.Repeat("p", 4097)} {
		cfg.LLM.APIKeyFile = path
		err := loadAPIKey(cfg, filepath.Join(t.TempDir(), "config.yaml"))
		if err == nil || !strings.Contains(err.Error(), "llm.api_key_file must be 1-4096 characters") {
			t.Fatalf("invalid key path %q returned %v", path, err)
		}
		if strings.Contains(err.Error(), path) {
			t.Fatalf("key path leaked in error: %v", err)
		}
	}
}

func TestLoadAPIKeyRejectsOversizedAndControlCharacterContent(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.LLM.Provider = "modelscope"
	cfg.LLM.APIKeyFile = "modelscope.key"
	keyPath := filepath.Join(dir, cfg.LLM.APIKeyFile)

	if err := os.WriteFile(keyPath, []byte(strings.Repeat("x", int(maxAPIKeyFileBytes)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadAPIKey(cfg, filepath.Join(dir, "config.yaml")); err == nil || !strings.Contains(err.Error(), "16384 byte limit") {
		t.Fatalf("expected oversized-key error, got %v", err)
	}

	for _, secretLikeValue := range []string{
		"first-line\ncredential-shaped-private-value",
		"token\u200bhidden",
		"token\u2028second-line",
		"token\u00a0suffix",
	} {
		if err := os.WriteFile(keyPath, []byte(secretLikeValue), 0o600); err != nil {
			t.Fatal(err)
		}
		err := loadAPIKey(cfg, filepath.Join(dir, "config.yaml"))
		if err == nil || !strings.Contains(err.Error(), "printable non-whitespace token") {
			t.Fatalf("expected Unicode control/space error, got %v", err)
		}
		if strings.Contains(err.Error(), secretLikeValue) || cfg.LLM.APIKey != "" {
			t.Fatalf("key content leaked or was retained: err=%v", err)
		}
	}
}

func TestLoadEmbeddingAPIKeyIsIndependentAndModelScopeRequiresIt(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.Embedding.Provider = "modelscope"
	if err := loadEmbeddingAPIKey(cfg, filepath.Join(dir, "config.yaml")); err == nil || !strings.Contains(err.Error(), "embedding.api_key_file is required") {
		t.Fatalf("expected required embedding key error, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "embedding.key"), []byte(" embedding-secret \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Embedding.APIKeyFile = "embedding.key"
	if err := loadEmbeddingAPIKey(cfg, filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if cfg.Embedding.APIKey != "embedding-secret" || cfg.LLM.APIKey != "" {
		t.Fatalf("embedding key was not isolated: embedding=%q llm=%q", cfg.Embedding.APIKey, cfg.LLM.APIKey)
	}
}

func TestLoadEmbeddingAPIKeyRemainsOptionalForLocalOpenAICompatibleEndpoint(t *testing.T) {
	cfg := Default()
	cfg.Embedding.Provider = "openai"
	if err := loadEmbeddingAPIKey(cfg, filepath.Join(t.TempDir(), "config.yaml")); err != nil {
		t.Fatalf("local OpenAI-compatible embedding endpoint should allow no key: %v", err)
	}
}

func TestValidateRequiresDatabasePassword(t *testing.T) {
	cfg := Default()
	cfg.LLM.Provider = "ollama"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "database.password must be 1-4096 characters") {
		t.Fatalf("expected missing database password error, got %v", err)
	}
	cfg.Database.Password = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid database password rejected: %v", err)
	}
}

func TestValidateNormalizesSafeStringsBeforeUse(t *testing.T) {
	cfg := Default()
	cfg.Database.Password = " secret with spaces "
	cfg.Database.Host = " 127.0.0.1 "
	cfg.Database.User = " novel_agent "
	cfg.Database.DBName = " novel_agent "
	cfg.Database.SSLMode = " DISABLE "
	cfg.Database.SearchPath = " novel_agent, public "
	cfg.LLM.Provider = " OLLAMA "
	cfg.LLM.BaseURL = " http://localhost:11434 "
	cfg.LLM.Model = " model-name "
	cfg.LLM.EmbeddingModel = " embedding-name "
	cfg.Vector.IndexType = " HNSW "
	cfg.Vector.Metric = " COSINE "
	cfg.Embedding.Provider = " OPENAI "
	cfg.Embedding.BaseURL = " http://localhost:8081 "
	cfg.Embedding.Model = " embedding-model "
	cfg.Server.Mode = " RELEASE "
	cfg.Logging.Level = " INFO "
	cfg.Logging.Format = " TEXT "
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Host != "127.0.0.1" || cfg.Database.User != "novel_agent" || cfg.Database.DBName != "novel_agent" || cfg.Database.SSLMode != "disable" || cfg.Database.SearchPath != "novel_agent,public" {
		t.Fatalf("database strings were not normalized: %#v", cfg.Database)
	}
	if cfg.Database.Password != " secret with spaces " || cfg.LLM.Provider != "ollama" || cfg.LLM.BaseURL != "http://localhost:11434" || cfg.LLM.Model != "model-name" || cfg.LLM.EmbeddingModel != "embedding-name" || cfg.Vector.IndexType != "hnsw" || cfg.Vector.Metric != "cosine" || cfg.Embedding.Provider != "openai" || cfg.Server.Mode != "release" || cfg.Logging.Level != "info" || cfg.Logging.Format != "text" {
		t.Fatalf("provider strings were not normalized safely: %#v %#v", cfg.LLM, cfg.Embedding)
	}
}

func TestValidateRejectsControlCharactersOversizedValuesAndURLCredentials(t *testing.T) {
	cfg := Default()
	cfg.Database.Password = "secret\nvalue"
	cfg.Database.Host = strings.Repeat("h", 256)
	cfg.LLM.Provider = "ollama"
	cfg.LLM.BaseURL = "http://user:password@example.com/v1"
	cfg.LLM.Model = "model\u200bname"
	cfg.Embedding.BaseURL = "http://example.com/v1?token=secret"
	cfg.Embedding.Model = strings.Repeat("m", 513)
	cfg.Server.Host = "127.0.0.1\u2028malformed"
	err := cfg.Validate()
	for _, expected := range []string{"database.host", "database.password", "llm.base_url", "llm.model", "embedding.base_url", "embedding.model", "server.host"} {
		if err == nil || !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected %s validation error, got %v", expected, err)
		}
	}
	if strings.Contains(err.Error(), "password") && strings.Contains(err.Error(), "example.com") {
		t.Fatalf("configuration value leaked in error: %v", err)
	}
}

func TestLocalEndpointClassificationDoesNotResolveArbitraryHosts(t *testing.T) {
	for _, value := range []string{
		"http://localhost:8081",
		"http://LOCALHOST.:8081",
		"http://127.0.0.1:8081",
		"http://[::1]:8081",
		"http://10.0.0.200:8081",
		"http://172.16.0.1:8081",
		"http://192.168.1.10:8081",
	} {
		if !isLocalEndpoint(value) {
			t.Errorf("local endpoint rejected: %q", value)
		}
	}
	for _, value := range []string{
		"https://api.openai.com/v1",
		"https://example.com/v1",
		"https://localhost.example.com/v1",
		"https://10.0.0.200.example.com/v1",
		"not-a-url",
	} {
		if isLocalEndpoint(value) {
			t.Errorf("non-local endpoint accepted: %q", value)
		}
	}
}

func TestValidateRequiresKeyForNonLocalOpenAIEmbedding(t *testing.T) {
	cfg := Default()
	cfg.Database.Password = "secret"
	cfg.LLM.Provider = "ollama"
	cfg.Embedding.Provider = "openai"
	cfg.Embedding.APIKeyFile = ""
	cfg.Embedding.BaseURL = "https://embedding.example.com/v1"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "embedding.api_key_file is required for non-local openai endpoints") {
		t.Fatalf("expected non-local embedding key error, got %v", err)
	}
}

func TestValidateRequiresHTTPSWhenCredentialsWillBeSent(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Config)
		expected  string
	}{
		{name: "remote LLM", configure: func(cfg *Config) {
			cfg.LLM.Provider = "openai"
			cfg.LLM.APIKeyFile = "openai.key"
			cfg.LLM.BaseURL = "http://api.example.com/v1"
		}, expected: "llm.base_url must use https"},
		{name: "ModelScope embedding", configure: func(cfg *Config) {
			cfg.Embedding.Provider = "modelscope"
			cfg.Embedding.APIKeyFile = "embedding.key"
			cfg.Embedding.BaseURL = "http://embedding.example.com/v1"
		}, expected: "embedding.base_url must use https"},
		{name: "optional embedding key", configure: func(cfg *Config) {
			cfg.Embedding.Provider = "openai"
			cfg.Embedding.APIKeyFile = "embedding.key"
			cfg.Embedding.BaseURL = "http://embedding.example.com/v1"
		}, expected: "embedding.base_url must use https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Database.Password = "secret"
			cfg.LLM.Provider = "ollama"
			tt.configure(cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.expected) {
				t.Fatalf("expected %q, got %v", tt.expected, err)
			}
		})
	}

	cfg := Default()
	cfg.Database.Password = "secret"
	cfg.LLM.Provider = "ollama"
	cfg.LLM.BaseURL = "http://localhost:11434"
	cfg.Embedding.Provider = "openai"
	cfg.Embedding.APIKeyFile = ""
	cfg.Embedding.BaseURL = "http://10.0.0.200:8081"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("keyless local HTTP endpoints were rejected: %v", err)
	}
}

func TestValidateRejectsNonFiniteTemperature(t *testing.T) {
	for name, value := range map[string]float64{
		"NaN":               math.NaN(),
		"positive infinity": math.Inf(1),
		"negative infinity": math.Inf(-1),
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Default()
			cfg.Database.Password = "secret"
			cfg.LLM.Provider = "ollama"
			cfg.LLM.Temperature = value
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), "llm.temperature must be a finite number") {
				t.Fatalf("temperature %v returned %v", value, err)
			}
		})
	}
}

func TestLoadRejectsYAMLNonFiniteTemperatureBeforeCredentialAccess(t *testing.T) {
	for name, value := range map[string]string{"NaN": ".nan", "infinity": ".inf"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			data := "database:\n  password: test-password\nllm:\n  provider: modelscope\n  api_key_file: missing-secret.key\n  temperature: " + value + "\n"
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), "llm.temperature") || strings.Contains(err.Error(), "API key file") {
				t.Fatalf("non-finite YAML temperature returned %v", err)
			}
		})
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
	cfg.Vector.Dimension = 0
	cfg.Vector.IndexType = "unknown-index"
	cfg.Vector.Metric = "unknown-metric"
	cfg.Embedding.Provider = "typo-provider"
	cfg.Embedding.Dimension = 2
	cfg.Database.MaxOpenConns = 0
	cfg.Database.MaxIdleConns = 1
	cfg.Database.ConnMaxLifetimeSec = 0
	cfg.Database.ConnMaxIdleTimeSec = 0
	cfg.Database.StatementTimeoutMS = 0
	cfg.Database.ConnectTimeoutSec = 0
	cfg.Server.Mode = "unknown-mode"
	cfg.Server.Port = 70000
	cfg.Server.MaxConcurrentAI = 0
	cfg.Server.MaxHeaderBytes = 1024
	cfg.Logging.Level = "verbose"
	cfg.Logging.Format = "xml"
	err := cfg.Validate()
	for _, expected := range []string{"llm.provider", "llm.base_url", "llm.temperature", "llm.max_tokens", "vector.dimension", "vector.index_type", "vector.metric", "embedding.provider", "embedding.dimension", "database.user", "database.sslmode", "database.search_path", "database.max_open_conns", "database.max_idle_conns", "database.conn_max_lifetime_seconds", "database.conn_max_idle_time_seconds", "database.statement_timeout_ms", "database.connect_timeout_seconds", "server.mode", "server.port", "server.max_concurrent_ai", "server.max_header_bytes", "logging.level", "logging.format"} {
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

func TestExplicitEmptyStringEnvironmentOverridesFailClosed(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		apply   func(*Config)
	}{
		{name: "database password", envName: "DB_PASSWORD", apply: func(cfg *Config) { cfg.Database.Password = "yaml-password" }},
		{name: "LLM model", envName: "LLM_MODEL", apply: func(cfg *Config) { cfg.LLM.Model = "yaml-model" }},
		{name: "API host", envName: "API_HOST", apply: func(cfg *Config) { cfg.Server.Host = "127.0.0.1" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			test.apply(cfg)
			t.Setenv(test.envName, "")
			if err := loadEnvOverrides(cfg); err != nil {
				t.Fatal(err)
			}
			if err := cfg.Validate(); err == nil {
				t.Fatal("explicit empty environment override silently fell back")
			}
		})
	}
}

func TestEnvironmentOverridesAreAtomicOnLateFailure(t *testing.T) {
	cfg := Default()
	originalHost := cfg.Database.Host
	originalModel := cfg.LLM.Model
	t.Setenv("DB_HOST", "changed.example")
	t.Setenv("LLM_MODEL", "changed-model")
	t.Setenv("API_MAX_HEADER_BYTES", "invalid-late-value")
	if err := loadEnvOverrides(cfg); err == nil {
		t.Fatal("expected late environment parse failure")
	}
	if cfg.Database.Host != originalHost || cfg.LLM.Model != originalModel || cfg.Server.MaxHeaderBytes != 64<<10 {
		t.Fatalf("failed environment override partially mutated config: %#v", cfg)
	}
}

func TestLoadEnvOverridesRejectsNilConfiguration(t *testing.T) {
	if err := loadEnvOverrides(nil); err == nil || err.Error() != "configuration is required" {
		t.Fatalf("unexpected nil configuration error: %v", err)
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

func TestCredentialLoadersClearStaleKeysWhenUnusedOrReloadFails(t *testing.T) {
	local := Default()
	local.LLM.Provider = "ollama"
	local.LLM.APIKey = "stale-llm-secret"
	if err := loadAPIKey(local, filepath.Join(t.TempDir(), "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if local.LLM.APIKey != "" {
		t.Fatal("local LLM credential loader retained stale key")
	}

	embedding := Default()
	embedding.Embedding.Provider = "openai"
	embedding.Embedding.APIKeyFile = ""
	embedding.Embedding.APIKey = "stale-embedding-secret"
	if err := loadEmbeddingAPIKey(embedding, filepath.Join(t.TempDir(), "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if embedding.Embedding.APIKey != "" {
		t.Fatal("keyless embedding loader retained stale key")
	}

	remote := Default()
	remote.LLM.Provider = "openai"
	remote.LLM.APIKeyFile = "missing.key"
	remote.LLM.APIKey = "stale-remote-secret"
	if err := loadAPIKey(remote, filepath.Join(t.TempDir(), "config.yaml")); err == nil {
		t.Fatal("missing remote key file was accepted")
	}
	if remote.LLM.APIKey != "" {
		t.Fatal("failed remote credential reload retained stale key")
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
