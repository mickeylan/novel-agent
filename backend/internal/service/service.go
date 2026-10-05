package service

import (
	"novel-agent/internal/config"
	"novel-agent/internal/service/agents"
	llm2 "novel-agent/internal/service/llm"
	"novel-agent/internal/service/retrieval"
)

// Service container holds all services
type Service struct {
	Config    *config.Config
	LLM       *llm2.Client
	Embedder  *llm2.EmbedderClient
	Architect *agents.Architect
	Writer    *agents.Writer
	Critic    *agents.Critic
	Editor    *agents.Editor
	Retrieval *retrieval.RetrievalService
}

// NewService creates a new service container
func NewService(cfg *config.Config) *Service {
	// Create LLM client
	llmClient := llm2.NewClient(
		cfg.LLM.Provider,
		cfg.LLM.APIKeyFile,
		cfg.LLM.BaseURL,
		cfg.LLM.Model,
	)

	// Create embedder client
	embedder := llm2.NewEmbedderClient(
		"ollama", // Use ollama for embeddings
		cfg.Embedding.BaseURL,
		cfg.Embedding.Model,
	)

	return &Service{
		Config:    cfg,
		LLM:       llmClient,
		Embedder:  embedder,
		Architect: agents.NewArchitect(llmClient),
		Writer:    agents.NewWriter(llmClient),
		Critic:    agents.NewCritic(llmClient),
		Editor:    agents.NewEditor(llmClient),
		Retrieval: retrieval.NewRetrievalService(embedder),
	}
}
