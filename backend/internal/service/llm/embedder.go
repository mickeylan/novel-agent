package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type embedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

func (e *OllamaEmbedder) Encode(ctx context.Context, text string) ([]float32, error) {
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Ollama returns {"embedding": [...]} not {"embeddings": [[...]]}
	var singleResp struct {
		Embedding []float64 `json:"embedding"`
	}
	
	if err := json.Unmarshal(body, &singleResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert []float64 to []float32
	embedding := make([]float32, len(singleResp.Embedding))
	for i, v := range singleResp.Embedding {
		embedding[i] = float32(v)
	}

	return embedding, nil
}

func (e *OllamaEmbedder) EncodeBatch(ctx context.Context, texts []string) ([][]float32, error) {
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
	reqBody := openAIEmbedRequest{
		Model:  e.Model,
		Input:  texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := e.BaseURL + "/v1/embeddings"
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var embedResp openAIEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	embeddings := make([][]float32, len(embedResp.Data))
	for i, data := range embedResp.Data {
		embedding := make([]float32, len(data.Embedding))
		for j, v := range data.Embedding {
			embedding[j] = float32(v)
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// EmbedderClient wrapper
type EmbedderClient struct {
	Embedder Embedder
	Model   string
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
		Model:   model,
	}
}

func (c *EmbedderClient) Encode(ctx context.Context, text string) ([]float32, error) {
	return c.Embedder.Encode(ctx, text)
}

func (c *EmbedderClient) EncodeBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return c.Embedder.EncodeBatch(ctx, texts)
}
