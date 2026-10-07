package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Embedder interface
type Embedder interface {
	Encode(ctx context.Context, text string) ([]float32, error)
	EncodeBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// OllamaEmbedder implements Embedder for Ollama embeddings API
type OllamaEmbedder struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Model:   model,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

const (
	maxEmbeddingBatchSize  = 128
	maxEmbeddingInputRunes = 100000
	maxEmbeddingResponse   = 16 << 20
)

func validateEmbeddingInputs(texts []string) error {
	if len(texts) == 0 || len(texts) > maxEmbeddingBatchSize {
		return fmt.Errorf("embedding batch size must be between 1 and %d", maxEmbeddingBatchSize)
	}
	total := 0
	for index, text := range texts {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("embedding input %d is empty", index)
		}
		total += len([]rune(text))
		if total > maxEmbeddingInputRunes {
			return fmt.Errorf("embedding batch exceeds %d characters", maxEmbeddingInputRunes)
		}
	}
	return nil
}

func convertEmbedding(values []float64) ([]float32, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("embedding response is empty")
	}
	embedding := make([]float32, len(values))
	for index, value := range values {
		embedding[index] = float32(value)
	}
	return embedding, nil
}

func (e *OllamaEmbedder) Encode(ctx context.Context, text string) ([]float32, error) {
	if err := validateEmbeddingInputs([]string{text}); err != nil {
		return nil, err
	}
	reqBody := embedRequest{
		Model:  e.Model,
		Prompt: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := e.BaseURL + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := readBoundedResponse(resp.Body, maxErrorResponseBytes)
		if readErr != nil {
			return nil, fmt.Errorf("API error: status %d (%v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := readBoundedResponse(resp.Body, maxEmbeddingResponse)
	if err != nil {
		return nil, err
	}

	// Ollama returns {"embedding": [...]} not {"embeddings": [[...]]}
	var singleResp struct {
		Embedding []float64 `json:"embedding"`
	}

	if err := json.Unmarshal(body, &singleResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return convertEmbedding(singleResp.Embedding)
}

func (e *OllamaEmbedder) EncodeBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if err := validateEmbeddingInputs(texts); err != nil {
		return nil, err
	}
	embeddings := make([][]float32, len(texts))

	for i, text := range texts {
		emb, err := e.Encode(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to encode text %d: %w", i, err)
		}
		embeddings[i] = emb
	}

	return embeddings, nil
}

// OpenAICompatibleEmbedder for OpenAI-compatible embedding APIs
type OpenAICompatibleEmbedder struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOpenAICompatibleEmbedder(baseURL, model string) *OpenAICompatibleEmbedder {
	return &OpenAICompatibleEmbedder{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Model:   model,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (e *OpenAICompatibleEmbedder) Encode(ctx context.Context, text string) ([]float32, error) {
	results, err := e.EncodeBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

func (e *OpenAICompatibleEmbedder) EncodeBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if err := validateEmbeddingInputs(texts); err != nil {
		return nil, err
	}
	reqBody := openAIEmbedRequest{
		Model: e.Model,
		Input: texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := openAIEndpoint(e.BaseURL, "embeddings")
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := readBoundedResponse(resp.Body, maxErrorResponseBytes)
		if readErr != nil {
			return nil, fmt.Errorf("API error: status %d (%v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := readBoundedResponse(resp.Body, maxEmbeddingResponse)
	if err != nil {
		return nil, err
	}

	var embedResp openAIEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(embedResp.Data) != len(texts) {
		return nil, fmt.Errorf("embedding response count %d does not match input count %d", len(embedResp.Data), len(texts))
	}
	embeddings := make([][]float32, len(embedResp.Data))
	dimension := 0
	for index, data := range embedResp.Data {
		embedding, err := convertEmbedding(data.Embedding)
		if err != nil {
			return nil, fmt.Errorf("invalid embedding %d: %w", index, err)
		}
		if dimension == 0 {
			dimension = len(embedding)
		} else if len(embedding) != dimension {
			return nil, fmt.Errorf("embedding %d dimension %d does not match %d", index, len(embedding), dimension)
		}
		embeddings[index] = embedding
	}

	return embeddings, nil
}

// EmbedderClient wrapper
type EmbedderClient struct {
	Embedder Embedder
	Model    string
}

func NewEmbedderClient(provider, baseURL, model string) *EmbedderClient {
	var embedder Embedder

	switch provider {
	case "ollama":
		embedder = NewOllamaEmbedder(baseURL, model)
	default:
		embedder = NewOpenAICompatibleEmbedder(baseURL, model)
	}

	return &EmbedderClient{
		Embedder: embedder,
		Model:    model,
	}
}

func (c *EmbedderClient) Encode(ctx context.Context, text string) ([]float32, error) {
	return c.Embedder.Encode(ctx, text)
}

func (c *EmbedderClient) EncodeBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return c.Embedder.EncodeBatch(ctx, texts)
}
