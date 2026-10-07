package service

import (
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
func NewService(cfg *config.Config) *Service {
	// Create LLM client
	llmClient := llm2.NewClient(
		cfg.LLM.Provider,
		cfg.LLM.APIKey,
		cfg.LLM.BaseURL,
		cfg.LLM.Model,
	)

	// Create embedder client
	embedder := llm2.NewEmbedderClient(
		cfg.Embedding.Provider,
		cfg.Embedding.BaseURL,
		cfg.Embedding.Model,
	)

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
	}
}
