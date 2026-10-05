package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig `yaml:"database"`
	LLM       LLMConfig      `yaml:"llm"`
	Vector    VectorConfig   `yaml:"vector"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Logging   LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

type DatabaseConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	User         string `yaml:"user"`
	Password     string `yaml:"password"`
	DBName       string `yaml:"dbname"`
	SSLMode      string `yaml:"sslmode"`
	SearchPath   string `yaml:"search_path"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

type LLMConfig struct {
	Provider        string  `yaml:"provider"`
	APIKeyFile     string  `yaml:"api_key_file"`  // API key 文件路径
	BaseURL         string  `yaml:"base_url"`
	Model          string  `yaml:"model"`
	EmbeddingModel string  `yaml:"embedding_model"`
	Temperature    float64 `yaml:"temperature"`
	MaxTokens     int     `yaml:"max_tokens"`
}

type VectorConfig struct {
	Dimension int    `yaml:"dimension"`
	IndexType string `yaml:"index_type"`
	Metric    string `yaml:"metric"`
}

type EmbeddingConfig struct {
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
			Host: "0.0.0.0",
			Port: 8080,
			Mode: "debug",
		},
		Database: DatabaseConfig{
			Host:         "127.0.0.1",
			Port:         5432,
			User:         "novel_agent",
			Password:     "novel_agent",
			DBName:       "novel_agent",
			SSLMode:      "disable",
			SearchPath:   "public",
			MaxOpenConns: 25,
			MaxIdleConns: 5,
		},
		LLM: LLMConfig{
			Provider:     "modelscope",
			BaseURL:      "https://coding.dashscope.aliyuncs.com/v1",
			Model:       "qwen3.5-plus",
			Temperature:  0.7,
			MaxTokens:    32000,
		},
		Vector: VectorConfig{
			Dimension: 1024,
			IndexType: "hnsw",
			Metric:    "cosine",
		},
		Embedding: EmbeddingConfig{
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
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	loadEnvOverrides(cfg)
	return cfg, nil
}

func loadEnvOverrides(cfg *Config) {
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Database.Port = port
		}
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

	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("MODELSCOPE_API_KEY"); v != "" {
		cfg.LLM.APIKeyFile = v  // 兼容：直接设置为 key 内容
	}
	if v := os.Getenv("LLM_API_KEY_FILE"); v != "" {
		cfg.LLM.APIKeyFile = v
	}
	if v := os.Getenv("MODELSCOPE_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
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

	if v := os.Getenv("EMBEDDING_BASE_URL"); v != "" {
		cfg.Embedding.BaseURL = v
	}
	if v := os.Getenv("EMBEDDING_MODEL"); v != "" {
		cfg.Embedding.Model = v
	}
	if v := os.Getenv("EMBEDDING_DIMENSION"); v != "" {
		if dim, err := strconv.Atoi(v); err == nil {
			cfg.Embedding.Dimension = dim
		}
	}

	if v := os.Getenv("API_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("API_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
}

func (c *Config) Validate() error {
	var errors []string
	if c.Database.Host == "" {
		errors = append(errors, "database.host is required")
	}
	if c.Database.DBName == "" {
		errors = append(errors, "database.dbname is required")
	}
	if c.LLM.BaseURL == "" {
		errors = append(errors, "llm.base_url is required")
	}
	if c.LLM.Model == "" {
		errors = append(errors, "llm.model is required")
	}
	if len(errors) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errors, ", "))
	}
	return nil
}
