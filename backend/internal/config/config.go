package config

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	maxAPIKeyFileBytes int64 = 16 << 10
	maxConfigFileBytes int64 = 1 << 20
	maxConfigYAMLDepth       = 64
	maxConfigYAMLNodes       = 20000
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
	Provider         string `yaml:"provider"`
	APIKeyFile       string `yaml:"api_key_file"`
	APIKey           string `yaml:"-"`
	apiKeyLoadedFile bool
	BaseURL          string  `yaml:"base_url"`
	Model            string  `yaml:"model"`
	EmbeddingModel   string  `yaml:"embedding_model"`
	Temperature      float64 `yaml:"temperature"`
	MaxTokens        int     `yaml:"max_tokens"`
}

type VectorConfig struct {
	Dimension int    `yaml:"dimension"`
	IndexType string `yaml:"index_type"`
	Metric    string `yaml:"metric"`
}

type EmbeddingConfig struct {
	Provider         string `yaml:"provider"`
	APIKeyFile       string `yaml:"api_key_file"`
	APIKey           string `yaml:"-"`
	apiKeyLoadedFile bool
	BaseURL          string `yaml:"base_url"`
	Model            string `yaml:"model"`
	Dimension        int    `yaml:"dimension"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func (c *Config) LLMCredentialsLoadedFromFile() bool {
	return c != nil && c.LLM.apiKeyLoadedFile && c.LLM.APIKeyFile != "" && c.LLM.APIKey != ""
}

func (c *Config) EmbeddingCredentialsLoadedFromFile() bool {
	return c != nil && c.Embedding.apiKeyLoadedFile && c.Embedding.APIKeyFile != "" && c.Embedding.APIKey != ""
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
	if !validConfigText(path, 4096, false) {
		return nil, fmt.Errorf("configuration path must be 1-4096 characters without control characters")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open configuration file")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxConfigFileBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("failed to read configuration file")
	}
	if int64(len(data)) > maxConfigFileBytes {
		return nil, fmt.Errorf("configuration file exceeds 1048576 byte limit")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("configuration file must contain valid UTF-8")
	}
	if err := decodeStrictConfig(data, cfg); err != nil {
		return nil, err
	}

	if err := loadEnvOverrides(cfg); err != nil {
		return nil, err
	}
	// Validate all non-secret configuration before touching a credential file.
	// This prevents malformed provider/database settings from causing unrelated
	// filesystem access during startup.
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := loadAPIKey(cfg, path); err != nil {
		return nil, err
	}
	if err := loadEmbeddingAPIKey(cfg, path); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateConfigYAMLNode(node *yaml.Node, depth int, nodes *int) error {
	if node == nil {
		return fmt.Errorf("configuration file contains invalid YAML structure")
	}
	if depth > maxConfigYAMLDepth {
		return fmt.Errorf("configuration file YAML nesting is too deep")
	}
	*nodes++
	if *nodes > maxConfigYAMLNodes {
		return fmt.Errorf("configuration file YAML structure is too complex")
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return fmt.Errorf("configuration file YAML aliases and anchors are not supported")
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%2 != 0 {
			return fmt.Errorf("configuration file contains invalid YAML structure")
		}
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("configuration file YAML keys must be scalar values")
			}
			identity := key.Tag + "\x00" + key.Value
			if _, exists := seen[identity]; exists {
				return fmt.Errorf("configuration file contains duplicate YAML keys")
			}
			seen[identity] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := validateConfigYAMLNode(child, depth+1, nodes); err != nil {
			return err
		}
	}
	return nil
}

func decodeStrictConfig(data []byte, cfg *Config) error {
	var document yaml.Node
	shapeDecoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := shapeDecoder.Decode(&document); err != nil {
		return fmt.Errorf("configuration file must contain one non-empty YAML object")
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode || len(document.Content[0].Content) == 0 {
		return fmt.Errorf("configuration file must contain one non-empty YAML object")
	}
	nodes := 0
	if err := validateConfigYAMLNode(&document, 0, &nodes); err != nil {
		return err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
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
	if cfg == nil {
		return fmt.Errorf("configuration is required")
	}
	target := cfg
	candidate := *cfg
	cfg = &candidate
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
	if v, exists := os.LookupEnv("DB_HOST"); exists {
		cfg.Database.Host = v
	}
	if err := parseInt("DB_PORT", &cfg.Database.Port); err != nil {
		return err
	}
	if v, exists := os.LookupEnv("DB_NAME"); exists {
		cfg.Database.DBName = v
	}
	if v, exists := os.LookupEnv("DB_USER"); exists {
		cfg.Database.User = v
	}
	if v, exists := os.LookupEnv("DB_PASSWORD"); exists {
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

	if v, exists := os.LookupEnv("LLM_PROVIDER"); exists {
		cfg.LLM.Provider = strings.ToLower(strings.TrimSpace(v))
	}
	if v, exists := os.LookupEnv("LLM_MODEL"); exists {
		cfg.LLM.Model = v
	}
	if v, exists := os.LookupEnv("LLM_API_KEY_FILE"); exists {
		cfg.LLM.APIKeyFile = v
	}
	if v, exists := os.LookupEnv("LLM_BASE_URL"); exists {
		cfg.LLM.BaseURL = strings.TrimSpace(v)
	}
	if v, exists := os.LookupEnv("MODELSCOPE_BASE_URL"); exists && cfg.LLM.Provider == "modelscope" {
		cfg.LLM.BaseURL = strings.TrimSpace(v)
	}
	if v, exists := os.LookupEnv("OPENAI_BASE_URL"); exists && cfg.LLM.Provider == "openai" {
		cfg.LLM.BaseURL = strings.TrimSpace(v)
	}

	if v, exists := os.LookupEnv("OLLAMA_BASE_URL"); exists && cfg.LLM.Provider == "ollama" {
		cfg.LLM.BaseURL = v
	}
	if v, exists := os.LookupEnv("SGLANG_BASE_URL"); exists && cfg.LLM.Provider == "sglang" {
		cfg.LLM.BaseURL = v
	}
	if v, exists := os.LookupEnv("LLAMA_BASE_URL"); exists && cfg.LLM.Provider == "llama" {
		cfg.LLM.BaseURL = v
	}

	if v, exists := os.LookupEnv("EMBEDDING_PROVIDER"); exists {
		cfg.Embedding.Provider = v
	}
	if v, exists := os.LookupEnv("EMBEDDING_API_KEY_FILE"); exists {
		cfg.Embedding.APIKeyFile = v
	}
	if v, exists := os.LookupEnv("EMBEDDING_BASE_URL"); exists {
		cfg.Embedding.BaseURL = v
	}
	if v, exists := os.LookupEnv("EMBEDDING_MODEL"); exists {
		cfg.Embedding.Model = v
	}
	if err := parseInt("EMBEDDING_DIMENSION", &cfg.Embedding.Dimension); err != nil {
		return err
	}

	if v, exists := os.LookupEnv("API_HOST"); exists {
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
	*target = candidate
	return nil
}

func loadAPIKey(cfg *Config, configPath string) error {
	cfg.LLM.Provider = strings.ToLower(strings.TrimSpace(cfg.LLM.Provider))
	cfg.LLM.APIKey = ""
	cfg.LLM.apiKeyLoadedFile = false
	requiresKey := cfg.LLM.Provider == "modelscope" || cfg.LLM.Provider == "openai"
	if !requiresKey {
		return nil
	}
	cfg.LLM.APIKeyFile = strings.TrimSpace(cfg.LLM.APIKeyFile)
	if !validConfigText(cfg.LLM.APIKeyFile, 4096, false) {
		return fmt.Errorf("llm.api_key_file must be 1-4096 characters without control characters")
	}

	key, err := readCredentialFile(cfg.LLM.APIKeyFile, configPath, "llm")
	if err != nil {
		return err
	}
	cfg.LLM.APIKey = key
	cfg.LLM.apiKeyLoadedFile = true
	return nil
}

func loadEmbeddingAPIKey(cfg *Config, configPath string) error {
	cfg.Embedding.APIKeyFile = strings.TrimSpace(cfg.Embedding.APIKeyFile)
	cfg.Embedding.APIKey = ""
	cfg.Embedding.apiKeyLoadedFile = false
	requiresKey := cfg.Embedding.Provider == "modelscope"
	if cfg.Embedding.APIKeyFile == "" {
		if requiresKey {
			return fmt.Errorf("embedding.api_key_file is required for modelscope")
		}
		return nil
	}
	key, err := readCredentialFile(cfg.Embedding.APIKeyFile, configPath, "embedding")
	if err != nil {
		return err
	}
	cfg.Embedding.APIKey = key
	cfg.Embedding.apiKeyLoadedFile = true
	return nil
}

func readCredential(file io.ReadCloser, kind string) ([]byte, error) {
	data, readErr := io.ReadAll(io.LimitReader(file, maxAPIKeyFileBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("failed to read %s API key file", kind)
	}
	if int64(len(data)) > maxAPIKeyFileBytes {
		return nil, fmt.Errorf("%s API key file exceeds 16384 byte limit", kind)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%s API key file must contain valid UTF-8", kind)
	}
	return data, nil
}

func readCredentialFile(path, configPath, kind string) (string, error) {
	path = strings.TrimSpace(path)
	if !validConfigText(path, 4096, false) {
		return "", fmt.Errorf("%s.api_key_file must be 1-4096 characters without control characters", kind)
	}
	keyPath := path
	if !filepath.IsAbs(keyPath) {
		keyPath = filepath.Join(filepath.Dir(configPath), keyPath)
	}
	file, err := os.Open(keyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s API key file", kind)
	}
	data, err := readCredential(file, kind)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("%s API key file is empty", kind)
	}
	for _, r := range key {
		if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return "", fmt.Errorf("%s API key file must contain one printable non-whitespace token", kind)
		}
	}
	return key, nil
}

func NormalizeDatabaseConfig(databaseConfig *DatabaseConfig) (*DatabaseConfig, error) {
	if databaseConfig == nil {
		return nil, fmt.Errorf("database configuration is required")
	}
	candidate := Default()
	candidate.Database = *databaseConfig
	candidate.LLM.Provider = "ollama"
	candidate.LLM.BaseURL = "http://127.0.0.1:11434"
	candidate.LLM.APIKeyFile = ""
	candidate.LLM.APIKey = ""
	if err := candidate.Validate(); err != nil {
		return nil, fmt.Errorf("database configuration is invalid")
	}
	normalized := candidate.Database
	return &normalized, nil
}

func ValidateDatabaseConfig(databaseConfig *DatabaseConfig) error {
	_, err := NormalizeDatabaseConfig(databaseConfig)
	return err
}

func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("configuration is required")
	}
	candidate := *c
	if err := candidate.validateAndNormalize(); err != nil {
		return err
	}
	*c = candidate
	return nil
}

func (c *Config) validateAndNormalize() error {
	c.Database.Host = strings.TrimSpace(c.Database.Host)
	c.Database.User = strings.TrimSpace(c.Database.User)
	c.Database.DBName = strings.TrimSpace(c.Database.DBName)
	c.Database.SSLMode = strings.ToLower(strings.TrimSpace(c.Database.SSLMode))
	c.Database.SearchPath = normalizeSearchPath(c.Database.SearchPath)
	c.LLM.Provider = strings.ToLower(strings.TrimSpace(c.LLM.Provider))
	c.LLM.APIKeyFile = strings.TrimSpace(c.LLM.APIKeyFile)
	if c.LLM.Provider != "modelscope" && c.LLM.Provider != "openai" {
		c.LLM.APIKey = ""
		c.LLM.apiKeyLoadedFile = false
	}
	c.LLM.BaseURL = strings.TrimSpace(c.LLM.BaseURL)
	c.LLM.Model = strings.TrimSpace(c.LLM.Model)
	c.LLM.EmbeddingModel = strings.TrimSpace(c.LLM.EmbeddingModel)
	c.Vector.IndexType = strings.ToLower(strings.TrimSpace(c.Vector.IndexType))
	c.Vector.Metric = strings.ToLower(strings.TrimSpace(c.Vector.Metric))
	c.Embedding.Provider = strings.ToLower(strings.TrimSpace(c.Embedding.Provider))
	c.Embedding.APIKeyFile = strings.TrimSpace(c.Embedding.APIKeyFile)
	if c.Embedding.APIKeyFile == "" {
		c.Embedding.APIKey = ""
		c.Embedding.apiKeyLoadedFile = false
	}
	c.Embedding.BaseURL = strings.TrimSpace(c.Embedding.BaseURL)
	c.Embedding.Model = strings.TrimSpace(c.Embedding.Model)
	c.Server.Host = strings.TrimSpace(c.Server.Host)
	c.Server.Mode = strings.ToLower(strings.TrimSpace(c.Server.Mode))
	c.Logging.Level = strings.ToLower(strings.TrimSpace(c.Logging.Level))
	c.Logging.Format = strings.ToLower(strings.TrimSpace(c.Logging.Format))

	var errors []string
	if !validConfigText(c.Database.Host, 255, false) {
		errors = append(errors, "database.host must be 1-255 characters without control characters")
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		errors = append(errors, "database.port must be between 1 and 65535")
	}
	if !validConfigText(c.Database.User, 63, false) {
		errors = append(errors, "database.user must be 1-63 characters without control characters")
	}
	if !validConfigText(c.Database.DBName, 63, false) {
		errors = append(errors, "database.dbname must be 1-63 characters without control characters")
	}
	if !validConfigText(c.Database.Password, 4096, true) || strings.TrimSpace(c.Database.Password) == "" {
		errors = append(errors, "database.password must be 1-4096 characters without control characters")
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
	} else if (provider == "modelscope" || provider == "openai") && !isHTTPSURL(c.LLM.BaseURL) {
		errors = append(errors, "llm.base_url must use https for providers that require credentials")
	}
	if !validConfigText(c.LLM.Model, 512, false) {
		errors = append(errors, "llm.model must be 1-512 characters without control characters")
	}
	if c.LLM.EmbeddingModel != "" && !validConfigText(c.LLM.EmbeddingModel, 512, false) {
		errors = append(errors, "llm.embedding_model must be empty or 1-512 characters without control characters")
	}
	if math.IsNaN(c.LLM.Temperature) || math.IsInf(c.LLM.Temperature, 0) || c.LLM.Temperature < 0 || c.LLM.Temperature > 2 {
		errors = append(errors, "llm.temperature must be a finite number between 0 and 2")
	}
	if c.LLM.MaxTokens < 1 || c.LLM.MaxTokens > 40000 {
		errors = append(errors, "llm.max_tokens must be between 1 and 40000")
	}
	if c.Vector.Dimension < 1 || c.Vector.Dimension > 65536 {
		errors = append(errors, "vector.dimension must be between 1 and 65536")
	}
	if c.Vector.Dimension != c.Embedding.Dimension {
		errors = append(errors, "vector.dimension must equal embedding.dimension")
	}
	if c.Vector.IndexType != "hnsw" && c.Vector.IndexType != "ivfflat" {
		errors = append(errors, "vector.index_type must be hnsw or ivfflat")
	}
	if c.Vector.Metric != "cosine" {
		errors = append(errors, "vector.metric must be cosine; other metrics are not implemented")
	}
	allowedEmbeddingProviders := map[string]bool{"modelscope": true, "openai": true, "ollama": true, "sglang": true, "llama": true}
	if !allowedEmbeddingProviders[c.Embedding.Provider] {
		errors = append(errors, "embedding.provider must be modelscope, openai, ollama, sglang, or llama")
	}
	if !validHTTPURL(c.Embedding.BaseURL) {
		errors = append(errors, "embedding.base_url must be an absolute http or https URL")
	} else {
		if (c.Embedding.Provider == "modelscope" || c.Embedding.APIKeyFile != "") && !isHTTPSURL(c.Embedding.BaseURL) {
			errors = append(errors, "embedding.base_url must use https when embedding credentials are configured")
		}
		if c.Embedding.Provider == "openai" && c.Embedding.APIKeyFile == "" && !isLocalEndpoint(c.Embedding.BaseURL) {
			errors = append(errors, "embedding.api_key_file is required for non-local openai endpoints")
		}
	}
	if !validConfigText(c.Embedding.Model, 512, false) {
		errors = append(errors, "embedding.model must be 1-512 characters without control characters")
	}
	if c.Embedding.Dimension < 1 || c.Embedding.Dimension > 65536 {
		errors = append(errors, "embedding.dimension must be between 1 and 65536")
	}
	if c.Server.Mode != "debug" && c.Server.Mode != "release" && c.Server.Mode != "test" {
		errors = append(errors, "server.mode must be debug, release, or test")
	}
	if !validConfigText(c.Server.Host, 255, false) {
		errors = append(errors, "server.host must be 1-255 characters without control characters")
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
	if c.Logging.Level != "info" {
		errors = append(errors, "logging.level must be info; other levels are not implemented")
	}
	if c.Logging.Format != "text" {
		errors = append(errors, "logging.format must be text; JSON logging is not implemented")
	}
	if len(errors) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errors, ", "))
	}
	return nil
}

func normalizeSearchPath(value string) string {
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return strings.Join(parts, ",")
}

func validConfigText(value string, maxBytes int, allowWhitespace bool) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || (!allowWhitespace && unicode.IsSpace(r) && r != ' ') {
			return false
		}
	}
	return true
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

func isHTTPSURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && strings.EqualFold(parsed.Scheme, "https")
}

func isLocalEndpoint(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func validHTTPURL(value string) bool {
	if !validConfigText(value, 2048, false) {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}
