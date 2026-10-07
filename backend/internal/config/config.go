package config

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	LLM       LLMConfig       `yaml:"llm"`
	Vector    VectorConfig    `yaml:"vector"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Logging   LoggingConfig   `yaml:"logging"`
}

type ServerConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	Mode            string `yaml:"mode"`
	MaxConcurrentAI int    `yaml:"max_concurrent_ai"`
	MaxHeaderBytes  int    `yaml:"max_header_bytes"`
}

type DatabaseConfig struct {
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	User               string `yaml:"user"`
	Password           string `yaml:"password"`
	DBName             string `yaml:"dbname"`
	SSLMode            string `yaml:"sslmode"`
	SearchPath         string `yaml:"search_path"`
	MaxOpenConns       int    `yaml:"max_open_conns"`
	MaxIdleConns       int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeSec int    `yaml:"conn_max_lifetime_seconds"`
	ConnMaxIdleTimeSec int    `yaml:"conn_max_idle_time_seconds"`
	StatementTimeoutMS int    `yaml:"statement_timeout_ms"`
	ConnectTimeoutSec  int    `yaml:"connect_timeout_seconds"`
}

type LLMConfig struct {
	Provider       string  `yaml:"provider"`
	APIKeyFile     string  `yaml:"api_key_file"`
	APIKey         string  `yaml:"-"`
	BaseURL        string  `yaml:"base_url"`
	Model          string  `yaml:"model"`
	EmbeddingModel string  `yaml:"embedding_model"`
	Temperature    float64 `yaml:"temperature"`
	MaxTokens      int     `yaml:"max_tokens"`
}

type VectorConfig struct {
	Dimension int    `yaml:"dimension"`
	IndexType string `yaml:"index_type"`
	Metric    string `yaml:"metric"`
}

type EmbeddingConfig struct {
	Provider  string `yaml:"provider"`
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	Dimension int    `yaml:"dimension"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "127.0.0.1",
			Port:            8080,
			Mode:            "debug",
			MaxConcurrentAI: 4,
			MaxHeaderBytes:  64 << 10,
		},
		Database: DatabaseConfig{
			Host:               "127.0.0.1",
			Port:               5432,
			User:               "novel_agent",
			Password:           "",
			DBName:             "novel_agent",
			SSLMode:            "disable",
			SearchPath:         "novel_agent,public",
			MaxOpenConns:       25,
			MaxIdleConns:       5,
			ConnMaxLifetimeSec: 1800,
			ConnMaxIdleTimeSec: 300,
			StatementTimeoutMS: 15000,
			ConnectTimeoutSec:  10,
		},
		LLM: LLMConfig{
			Provider:    "modelscope",
			BaseURL:     "https://coding.dashscope.aliyuncs.com/v1",
			Model:       "qwen3.5-plus",
			Temperature: 0.7,
			MaxTokens:   32000,
		},
		Vector: VectorConfig{
			Dimension: 1024,
			IndexType: "hnsw",
			Metric:    "cosine",
		},
		Embedding: EmbeddingConfig{
			Provider:  "openai",
			BaseURL:   "http://10.0.0.200:8081",
			Model:     "bge-m3-Q4_K_M.gguf",
			Dimension: 1024,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		if err := decodeStrictConfig(data, cfg); err != nil {
			return nil, err
		}
	}

	if err := loadEnvOverrides(cfg); err != nil {
		return nil, err
	}
	if err := loadAPIKey(cfg, path); err != nil {
		return nil, err
	}
	return cfg, nil
}

func decodeStrictConfig(data []byte, cfg *Config) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil && err != io.EOF {
		// Decoder errors can echo scalar values. Avoid exposing credentials from
		// malformed local configuration in startup logs.
		return fmt.Errorf("configuration file contains invalid or unknown fields")
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("configuration file must contain exactly one YAML document")
	}
	return nil
}

func loadEnvOverrides(cfg *Config) error {
	parseInt := func(name string, target *int) error {
		value, exists := os.LookupEnv(name)
		if !exists {
			return nil
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("environment variable %s must be an integer", name)
		}
		*target = parsed
		return nil
	}
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if err := parseInt("DB_PORT", &cfg.Database.Port); err != nil {
		return err
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.Database.DBName = v
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	for _, setting := range []struct {
		name   string
		target *int
	}{
		{name: "DB_MAX_OPEN_CONNS", target: &cfg.Database.MaxOpenConns},
		{name: "DB_MAX_IDLE_CONNS", target: &cfg.Database.MaxIdleConns},
		{name: "DB_CONN_MAX_LIFETIME_SECONDS", target: &cfg.Database.ConnMaxLifetimeSec},
		{name: "DB_CONN_MAX_IDLE_TIME_SECONDS", target: &cfg.Database.ConnMaxIdleTimeSec},
		{name: "DB_STATEMENT_TIMEOUT_MS", target: &cfg.Database.StatementTimeoutMS},
		{name: "DB_CONNECT_TIMEOUT_SECONDS", target: &cfg.Database.ConnectTimeoutSec},
	} {
		if err := parseInt(setting.name, setting.target); err != nil {
			return err
		}
	}

	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = strings.ToLower(strings.TrimSpace(v))
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("LLM_API_KEY_FILE"); v != "" {
		cfg.LLM.APIKeyFile = v
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = strings.TrimSpace(v)
	}
	if v := os.Getenv("MODELSCOPE_BASE_URL"); v != "" && cfg.LLM.Provider == "modelscope" {
		cfg.LLM.BaseURL = strings.TrimSpace(v)
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" && cfg.LLM.Provider == "openai" {
		cfg.LLM.BaseURL = strings.TrimSpace(v)
	}

	if v := os.Getenv("OLLAMA_BASE_URL"); v != "" {
		if cfg.LLM.Provider == "ollama" {
			cfg.LLM.BaseURL = v
		}
	}
	if v := os.Getenv("SGLANG_BASE_URL"); v != "" {
		if cfg.LLM.Provider == "sglang" {
			cfg.LLM.BaseURL = v
		}
	}
	if v := os.Getenv("LLAMA_BASE_URL"); v != "" {
		if cfg.LLM.Provider == "llama" {
			cfg.LLM.BaseURL = v
		}
	}

	if v := os.Getenv("EMBEDDING_PROVIDER"); v != "" {
		cfg.Embedding.Provider = v
	}
	if v := os.Getenv("EMBEDDING_BASE_URL"); v != "" {
		cfg.Embedding.BaseURL = v
	}
	if v := os.Getenv("EMBEDDING_MODEL"); v != "" {
		cfg.Embedding.Model = v
	}
	if err := parseInt("EMBEDDING_DIMENSION", &cfg.Embedding.Dimension); err != nil {
		return err
	}

	if v := os.Getenv("API_HOST"); v != "" {
		cfg.Server.Host = v
	}
	for _, setting := range []struct {
		name   string
		target *int
	}{
		{name: "API_PORT", target: &cfg.Server.Port},
		{name: "API_MAX_CONCURRENT_AI", target: &cfg.Server.MaxConcurrentAI},
		{name: "API_MAX_HEADER_BYTES", target: &cfg.Server.MaxHeaderBytes},
	} {
		if err := parseInt(setting.name, setting.target); err != nil {
			return err
		}
	}
	return nil
}

func loadAPIKey(cfg *Config, configPath string) error {
	cfg.LLM.Provider = strings.ToLower(strings.TrimSpace(cfg.LLM.Provider))
	requiresKey := cfg.LLM.Provider == "modelscope" || cfg.LLM.Provider == "openai"
	if !requiresKey {
		return nil
	}
	if cfg.LLM.APIKeyFile == "" {
		return fmt.Errorf("llm.api_key_file is required for provider %s", cfg.LLM.Provider)
	}

	keyPath := cfg.LLM.APIKeyFile
	if !filepath.IsAbs(keyPath) {
		baseDir := filepath.Dir(configPath)
		keyPath = filepath.Join(baseDir, keyPath)
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read llm API key file: %w", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return fmt.Errorf("llm API key file is empty")
	}
	cfg.LLM.APIKey = key
	return nil
}

func (c *Config) Validate() error {
	var errors []string
	if strings.TrimSpace(c.Database.Host) == "" {
		errors = append(errors, "database.host is required")
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		errors = append(errors, "database.port must be between 1 and 65535")
	}
	if strings.TrimSpace(c.Database.User) == "" {
		errors = append(errors, "database.user is required")
	}
	if strings.TrimSpace(c.Database.DBName) == "" {
		errors = append(errors, "database.dbname is required")
	}
	if strings.TrimSpace(c.Database.Password) == "" {
		errors = append(errors, "database.password is required")
	}
	if c.Database.MaxOpenConns < 1 || c.Database.MaxOpenConns > 200 {
		errors = append(errors, "database.max_open_conns must be between 1 and 200")
	}
	if c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		errors = append(errors, "database.max_idle_conns must be between 0 and database.max_open_conns")
	}
	if c.Database.ConnMaxLifetimeSec < 60 || c.Database.ConnMaxLifetimeSec > 86400 {
		errors = append(errors, "database.conn_max_lifetime_seconds must be between 60 and 86400")
	}
	if c.Database.ConnMaxIdleTimeSec < 30 || c.Database.ConnMaxIdleTimeSec > c.Database.ConnMaxLifetimeSec {
		errors = append(errors, "database.conn_max_idle_time_seconds must be between 30 and database.conn_max_lifetime_seconds")
	}
	if c.Database.StatementTimeoutMS < 100 || c.Database.StatementTimeoutMS > 120000 {
		errors = append(errors, "database.statement_timeout_ms must be between 100 and 120000")
	}
	if c.Database.ConnectTimeoutSec < 1 || c.Database.ConnectTimeoutSec > 60 {
		errors = append(errors, "database.connect_timeout_seconds must be between 1 and 60")
	}
	allowedSSLModes := map[string]bool{"disable": true, "allow": true, "prefer": true, "require": true, "verify-ca": true, "verify-full": true}
	if !allowedSSLModes[strings.ToLower(strings.TrimSpace(c.Database.SSLMode))] {
		errors = append(errors, "database.sslmode must be disable, allow, prefer, require, verify-ca, or verify-full")
	}
	if !validSearchPath(c.Database.SearchPath) {
		errors = append(errors, "database.search_path must be a comma-separated list of unquoted PostgreSQL identifiers")
	}
	provider := strings.ToLower(strings.TrimSpace(c.LLM.Provider))
	allowedProviders := map[string]bool{"modelscope": true, "openai": true, "ollama": true, "sglang": true, "llama": true}
	if !allowedProviders[provider] {
		errors = append(errors, "llm.provider must be modelscope, openai, ollama, sglang, or llama")
	}
	if !validHTTPURL(c.LLM.BaseURL) {
		errors = append(errors, "llm.base_url must be an absolute http or https URL")
	}
	if strings.TrimSpace(c.LLM.Model) == "" {
		errors = append(errors, "llm.model is required")
	}
	if c.LLM.Temperature < 0 || c.LLM.Temperature > 2 {
		errors = append(errors, "llm.temperature must be between 0 and 2")
	}
	if c.LLM.MaxTokens < 1 || c.LLM.MaxTokens > 40000 {
		errors = append(errors, "llm.max_tokens must be between 1 and 40000")
	}
	if !validHTTPURL(c.Embedding.BaseURL) {
		errors = append(errors, "embedding.base_url must be an absolute http or https URL")
	}
	if c.Embedding.Dimension < 1 || c.Embedding.Dimension > 65536 {
		errors = append(errors, "embedding.dimension must be between 1 and 65536")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errors = append(errors, "server.port must be between 1 and 65535")
	}
	if c.Server.MaxConcurrentAI < 1 || c.Server.MaxConcurrentAI > 64 {
		errors = append(errors, "server.max_concurrent_ai must be between 1 and 64")
	}
	if c.Server.MaxHeaderBytes < 8<<10 || c.Server.MaxHeaderBytes > 1<<20 {
		errors = append(errors, "server.max_header_bytes must be between 8192 and 1048576")
	}
	if len(errors) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errors, ", "))
	}
	return nil
}

func validSearchPath(value string) bool {
	parts := strings.Split(value, ",")
	if len(parts) == 0 || len(parts) > 16 {
		return false
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || len(part) > 63 || !isIdentifierStart(part[0]) {
			return false
		}
		for i := 1; i < len(part); i++ {
			if !isIdentifierPart(part[i]) {
				return false
			}
		}
	}
	return true
}

func isIdentifierStart(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isIdentifierPart(value byte) bool {
	return isIdentifierStart(value) || value >= '0' && value <= '9' || value == '$'
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}
