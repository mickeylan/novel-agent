package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
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
		Client:  newProviderHTTPClient(),
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
		if !utf8.ValidString(text) {
			return fmt.Errorf("embedding input %d must be valid UTF-8", index)
		}
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("embedding input %d is empty", index)
		}
		total += utf8.RuneCountInString(text)
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
		if math.IsNaN(value) || math.IsInf(value, 0) || value > math.MaxFloat32 || value < -math.MaxFloat32 {
			return nil, fmt.Errorf("embedding value %d must be finite and representable as float32", index)
		}
		converted := float32(value)
		if float32IsNonFinite(converted) {
			return nil, fmt.Errorf("embedding value %d must be finite and representable as float32", index)
		}
		embedding[index] = converted
	}
	return embedding, nil
}

func float32IsNonFinite(value float32) bool {
	bits := math.Float32bits(value)
	return bits&0x7f800000 == 0x7f800000
}

func (e *OllamaEmbedder) Encode(ctx context.Context, text string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateEmbeddingInputs([]string{text}); err != nil {
		return nil, err
	}
	if err := validateProviderBaseURL(e.BaseURL); err != nil {
		return nil, err
	}
	if err := validateProviderIdentity(e.Model, ""); err != nil {
		return nil, err
	}
	if err := validateProviderTransport(e.BaseURL, ""); err != nil {
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
		_, _ = readBoundedResponse(resp.Body, maxErrorResponseBytes)
		return nil, &ProviderHTTPError{StatusCode: resp.StatusCode}
	}
	if err := validateProviderContentType(resp, false); err != nil {
		return nil, err
	}

	body, err := readBoundedUTF8Response(resp.Body, maxEmbeddingResponse)
	if err != nil {
		return nil, err
	}

	// Ollama returns {"embedding": [...]} not {"embeddings": [[...]]}
	var singleResp struct {
		Embedding []float64 `json:"embedding"`
	}

	if err := unmarshalProviderJSON(body, &singleResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return convertEmbedding(singleResp.Embedding)
}

func (e *OllamaEmbedder) EncodeBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
	APIKey  string
	Client  *http.Client
}

func NewOpenAICompatibleEmbedder(baseURL, model, apiKey string) *OpenAICompatibleEmbedder {
	return &OpenAICompatibleEmbedder{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Model:   model,
		APIKey:  apiKey,
		Client:  newProviderHTTPClient(),
	}
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     *int      `json:"index"`
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateProviderBaseURL(e.BaseURL); err != nil {
		return nil, err
	}
	if err := validateProviderIdentity(e.Model, e.APIKey); err != nil {
		return nil, err
	}
	if err := validateProviderTransport(e.BaseURL, e.APIKey); err != nil {
		return nil, err
	}
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
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = readBoundedResponse(resp.Body, maxErrorResponseBytes)
		return nil, &ProviderHTTPError{StatusCode: resp.StatusCode}
	}
	if err := validateProviderContentType(resp, false); err != nil {
		return nil, err
	}

	body, err := readBoundedUTF8Response(resp.Body, maxEmbeddingResponse)
	if err != nil {
		return nil, err
	}

	var embedResp openAIEmbedResponse
	if err := unmarshalProviderJSON(body, &embedResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(embedResp.Data) != len(texts) {
		return nil, fmt.Errorf("embedding response count %d does not match input count %d", len(embedResp.Data), len(texts))
	}
	embeddings := make([][]float32, len(embedResp.Data))
	indexed := false
	for _, data := range embedResp.Data {
		if data.Index != nil {
			indexed = true
			break
		}
	}
	dimension := 0
	seen := make([]bool, len(embedResp.Data))
	for responseIndex, data := range embedResp.Data {
		targetIndex := responseIndex
		if indexed {
			if data.Index == nil || *data.Index < 0 || *data.Index >= len(embedResp.Data) || seen[*data.Index] {
				return nil, fmt.Errorf("embedding response contains invalid indices")
			}
			targetIndex = *data.Index
			seen[targetIndex] = true
		}
		embedding, err := convertEmbedding(data.Embedding)
		if err != nil {
			return nil, fmt.Errorf("invalid embedding %d: %w", responseIndex, err)
		}
		if dimension == 0 {
			dimension = len(embedding)
		} else if len(embedding) != dimension {
			return nil, fmt.Errorf("embedding %d dimension %d does not match %d", responseIndex, len(embedding), dimension)
		}
		embeddings[targetIndex] = embedding
	}

	return embeddings, nil
}

// EmbedderClient wrapper
type EmbedderClient struct {
	Embedder          Embedder
	Model             string
	ExpectedDimension int
}

func isLocalProviderEndpoint(value string) bool {
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

func NewEmbedderClient(provider, baseURL, model, apiKey string, expectedDimension ...int) (*EmbedderClient, error) {
	var embedder Embedder
	dimension := 0
	if len(expectedDimension) > 1 || (len(expectedDimension) == 1 && (expectedDimension[0] < 1 || expectedDimension[0] > 65536)) {
		return nil, fmt.Errorf("embedding expected dimension must be between 1 and 65536")
	}
	if len(expectedDimension) == 1 {
		dimension = expectedDimension[0]
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	baseURL = strings.TrimSpace(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if apiKey != "" && !strings.HasPrefix(strings.ToLower(baseURL), "https://") {
		return nil, fmt.Errorf("embedding credentials require an HTTPS endpoint")
	}
	if provider == "modelscope" && apiKey == "" {
		return nil, fmt.Errorf("modelscope embedding provider requires credentials")
	}
	if provider == "openai" && apiKey == "" && !isLocalProviderEndpoint(baseURL) {
		return nil, fmt.Errorf("non-local openai embedding endpoint requires credentials")
	}

	switch provider {
	case "ollama":
		embedder = NewOllamaEmbedder(baseURL, model)
	case "modelscope", "openai", "sglang", "llama":
		embedder = NewOpenAICompatibleEmbedder(baseURL, model, apiKey)
	default:
		return nil, fmt.Errorf("unsupported embedding provider")
	}

	return &EmbedderClient{
		Embedder:          embedder,
		Model:             model,
		ExpectedDimension: dimension,
	}, nil
}

func (c *EmbedderClient) validateDimensions(embeddings [][]float32) error {
	if c.ExpectedDimension == 0 {
		return nil
	}
	for index, embedding := range embeddings {
		if len(embedding) != c.ExpectedDimension {
			return fmt.Errorf("embedding %d dimension does not match configured dimension", index)
		}
	}
	return nil
}

func (c *EmbedderClient) Encode(ctx context.Context, text string) ([]float32, error) {
	embedding, err := c.Embedder.Encode(ctx, text)
	if err != nil {
		return nil, err
	}
	if err := c.validateDimensions([][]float32{embedding}); err != nil {
		return nil, err
	}
	return embedding, nil
}

func (c *EmbedderClient) EncodeBatch(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings, err := c.Embedder.EncodeBatch(ctx, texts)
	if err != nil {
		return nil, err
	}
	if err := c.validateDimensions(embeddings); err != nil {
		return nil, err
	}
	return embeddings, nil
}
