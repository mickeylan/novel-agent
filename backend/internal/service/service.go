package service

import (
	"fmt"

	"novel-agent/internal/config"
	"novel-agent/internal/service/agents"
	llm2 "novel-agent/internal/service/llm"
	"novel-agent/internal/service/orchestration"
	"novel-agent/internal/service/retrieval"
)

// Service container holds all services
type Service struct {
	Config       *config.Config
	LLM          *llm2.Client
	Embedder     *llm2.EmbedderClient
	Architect    *agents.Architect
	Writer       *agents.Writer
	Critic       *agents.Critic
	Editor       *agents.Editor
	Validator    *agents.ContinuityValidator
	Reviser      *agents.Reviser
	Reader       *agents.ReaderPerspectiveAnalyzer
	AIGCDetector *agents.AIGCDetector
	Style        *agents.StyleAnalyzer
	ScenePlanner *agents.ScenePlanner
	TextDiffer   *agents.TextDiffer
	DataAgent    *agents.DataAgent
	Context      *agents.ContextBuilder
	Orchestrator *orchestration.Orchestrator
	Retrieval    *retrieval.RetrievalService
}

// NewService creates a new service container
func NewService(cfg *config.Config) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("service configuration is required")
	}
	validated := *cfg
	if err := validated.Validate(); err != nil {
		return nil, fmt.Errorf("service configuration is invalid: %w", err)
	}
	remoteLLM := validated.LLM.Provider == "modelscope" || validated.LLM.Provider == "openai"
	if remoteLLM && !validated.LLMCredentialsLoadedFromFile() {
		return nil, fmt.Errorf("service LLM credentials are not loaded from a key file")
	}
	embeddingUsesKeyFile := validated.Embedding.Provider == "modelscope" || validated.Embedding.APIKeyFile != ""
	if embeddingUsesKeyFile && !validated.EmbeddingCredentialsLoadedFromFile() {
		return nil, fmt.Errorf("service embedding credentials are not loaded from a key file")
	}
	if !embeddingUsesKeyFile {
		validated.Embedding.APIKey = ""
	}
	cfg = &validated
	// Create LLM client
	llmClient, err := llm2.NewClient(
		cfg.LLM.Provider,
		cfg.LLM.APIKey,
		cfg.LLM.BaseURL,
		cfg.LLM.Model,
	)
	if err != nil {
		return nil, err
	}

	// Create embedder client
	embedder, err := llm2.NewEmbedderClient(
		cfg.Embedding.Provider,
		cfg.Embedding.BaseURL,
		cfg.Embedding.Model,
		cfg.Embedding.APIKey,
		cfg.Embedding.Dimension,
	)
	if err != nil {
		return nil, err
	}

	writer := agents.NewWriter(llmClient)
	critic := agents.NewCritic(llmClient)
	validator := agents.NewContinuityValidator()
	reviser := agents.NewReviser(llmClient)
	reader := agents.NewReaderPerspectiveAnalyzer()
	dataAgent := agents.NewDataAgent(llmClient)
	scenePlanner := agents.NewScenePlanner()
	aigcDetector := agents.NewAIGCDetector()
	styleAnalyzer := agents.NewStyleAnalyzer()

	return &Service{
		Config:       cfg,
		LLM:          llmClient,
		Embedder:     embedder,
		Architect:    agents.NewArchitect(llmClient),
		Writer:       writer,
		Critic:       critic,
		Editor:       agents.NewEditor(llmClient),
		Validator:    validator,
		Reviser:      reviser,
		Reader:       reader,
		AIGCDetector: aigcDetector,
		Style:        styleAnalyzer,
		ScenePlanner: scenePlanner,
		TextDiffer:   agents.NewTextDiffer(),
		DataAgent:    dataAgent,
		Context:      agents.NewContextBuilder(),
		Orchestrator: orchestration.NewOrchestrator(writer, critic, reviser, validator, reader, dataAgent, scenePlanner, aigcDetector, styleAnalyzer),
		Retrieval:    retrieval.NewRetrievalService(embedder, cfg.Embedding.Dimension),
	}, nil
}
