package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Provider interface
type Provider interface {
	Generate(ctx context.Context, prompt string, opts ...Option) (string, error)
	GenerateJSON(ctx context.Context, prompt string, opts ...Option) (map[string]interface{}, error)
}

type Option func(*Options)

type Options struct {
	Temperature float64
	MaxTokens   int
	TopP        float64
	Timeout     int
	Stream      bool
}

func WithTemperature(t float64) Option {
	return func(o *Options) { o.Temperature = t }
}

func WithMaxTokens(max int) Option {
	return func(o *Options) { o.MaxTokens = max }
}

func WithTopP(p float64) Option {
	return func(o *Options) { o.TopP = p }
}

func WithTimeout(timeout int) Option {
	return func(o *Options) { o.Timeout = timeout }
}

func WithStream(stream bool) Option {
	return func(o *Options) { o.Stream = stream }
}

func DefaultOptions() *Options {
	return &Options{
		Temperature: 0.7,
		MaxTokens:   32000,
		TopP:       0.9,
		Timeout:     300,
		Stream:      false,
	}
}

// ========== ModelScope Client ==========

type ModelScopeClient struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewModelScopeClient(apiKey, baseURL, model string) *ModelScopeClient {
	return &ModelScopeClient{
		APIKey:  apiKey,
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Model:   model,
		Client: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *ModelScopeClient) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	useStream := options.Stream || len(prompt) > 5000
	systemPrompt := "你是一位专业的小说作家和文学评论家。请始终使用简体中文（Chinese）进行回复，不要用英文。"

	reqBody := map[string]interface{}{
		"model": c.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"stream": useStream,
	}

	jsonData, _ := json.Marshal(reqBody)
	url := c.BaseURL + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	if useStream {
		return c.handleStreamResponse(resp)
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(body, &result)
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response")
	}
	return result.Choices[0].Message.Content, nil
}

func (c *ModelScopeClient) handleStreamResponse(resp *http.Response) (string, error) {
	var builder strings.Builder
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var data struct {
			Choices []struct {
				Delta struct{ Content string } `json:"delta"`
			} `json:"choices"`
		}
		if err := decoder.Decode(&data); err == nil && len(data.Choices) > 0 {
			builder.WriteString(data.Choices[0].Delta.Content)
		}
	}
	return builder.String(), nil
}

func (c *ModelScopeClient) GenerateJSON(ctx context.Context, prompt string, opts ...Option) (map[string]interface{}, error) {
	content, err := c.Generate(ctx, prompt+"请以 JSON 格式输出。", opts...)
	if err != nil {
		return nil, err
	}
	return parseJSONResponse(content)
}

// ========== OpenAI Compatible Client (Ollama, SGLang, llama.cpp) ==========

type OpenAICompatibleClient struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOpenAICompatibleClient(apiKey, baseURL, model string) *OpenAICompatibleClient {
	return &OpenAICompatibleClient{
		APIKey:  apiKey,
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Model:   model,
		Client:  &http.Client{Timeout: 10 * time.Minute},
	}
}

func (c *OpenAICompatibleClient) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	useStream := options.Stream || len(prompt) > 5000
	systemPrompt := "你是一位专业的小说作家和文学评论家。请始终使用简体中文（Chinese）进行回复，不要用英文。"

	reqBody := map[string]interface{}{
		"model": c.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"temperature": options.Temperature,
		"top_p":       options.TopP,
		"stream":       useStream,
	}
	if options.MaxTokens > 0 {
		reqBody["max_tokens"] = options.MaxTokens
	}

	jsonData, _ := json.Marshal(reqBody)
	url := c.BaseURL + "/v1/chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	if useStream {
		return c.handleStreamResponse(resp)
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Choices []struct {
			Message struct {
				Content           string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(body, &result)
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response")
	}

	content := result.Choices[0].Message.Content
	// Handle Qwen thinking mode
	if content == "" && result.Choices[0].Message.ReasoningContent != "" {
		content = extractNarrative(result.Choices[0].Message.ReasoningContent)
	}
	return content, nil
}

func (c *OpenAICompatibleClient) handleStreamResponse(resp *http.Response) (string, error) {
	var builder strings.Builder
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var data struct {
			Choices []struct {
				Delta struct {
					Content           string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := decoder.Decode(&data); err == nil && len(data.Choices) > 0 {
			builder.WriteString(data.Choices[0].Delta.Content)
		}
	}
	return builder.String(), nil
}

func (c *OpenAICompatibleClient) GenerateJSON(ctx context.Context, prompt string, opts ...Option) (map[string]interface{}, error) {
	content, err := c.Generate(ctx, prompt+"请以 JSON 格式输出。", opts...)
	if err != nil {
		return nil, err
	}
	return parseJSONResponse(content)
}

// ========== Ollama Native Client ==========

type OllamaClient struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Model:   model,
		Client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *OllamaClient) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	reqBody := map[string]interface{}{
		"model":      c.Model,
		"prompt":     prompt,
		"stream":     false,
		"temperature": options.Temperature,
	}
	if options.MaxTokens > 0 {
		reqBody["options"] = map[string]interface{}{
			"num_predict": options.MaxTokens,
		}
	}

	jsonData, _ := json.Marshal(reqBody)
	url := c.BaseURL + "/api/generate"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Response string `json:"response"`
	}
	json.Unmarshal(body, &result)
	return result.Response, nil
}

func (c *OllamaClient) GenerateJSON(ctx context.Context, prompt string, opts ...Option) (map[string]interface{}, error) {
	content, err := c.Generate(ctx, prompt+"请以 JSON 格式输出。", opts...)
	if err != nil {
		return nil, err
	}
	return parseJSONResponse(content)
}

// ========== Unified Client ==========

type Client struct {
	Provider Provider
	Model   string
}

// readAPIKey 从文件或环境变量读取 API key
func readAPIKey(apiKeyOrFile string) string {
	if apiKeyOrFile == "" {
		return ""
	}
	
	// 检查是否是文件路径
	if _, err := os.Stat(apiKeyOrFile); err == nil {
		// 是文件，读取内容
		data, err := os.ReadFile(apiKeyOrFile)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}
	
	// 直接是 key 内容
	return apiKeyOrFile
}

func NewClient(provider, apiKey, baseURL, model string) *Client {
	var p Provider

	// 如果没有指定 baseURL，从环境变量获取
	if baseURL == "" {
		switch provider {
		case "ollama":
			baseURL = os.Getenv("OLLAMA_BASE_URL")
			if baseURL == "" {
				baseURL = "http://localhost:11434"
			}
		case "sglang":
			baseURL = os.Getenv("SGLANG_BASE_URL")
			if baseURL == "" {
				baseURL = "http://localhost:30000"
			}
		case "llama":
			baseURL = os.Getenv("LLAMA_BASE_URL")
			if baseURL == "" {
				baseURL = "http://10.0.0.200:8080"
			}
		case "modelscope":
			baseURL = os.Getenv("MODELSCOPE_BASE_URL")
			if baseURL == "" {
				baseURL = "https://coding.dashscope.aliyuncs.com/v1"
			}
		}
	}

	// 如果没有指定 model，从环境变量获取
	if model == "" {
		model = os.Getenv("LLM_MODEL")
	}

	// 如果没有指定 apiKey，从环境变量获取
	if apiKey == "" {
		apiKey = os.Getenv("MODELSCOPE_API_KEY")
	}

	// 从文件读取 API key（如果 apiKey 是文件路径）
	apiKey = readAPIKey(apiKey)

	switch provider {
	case "ollama":
		// Ollama 使用原生 API
		p = NewOllamaClient(baseURL, model)
	case "sglang", "llama", "openai":
		// SGLang, llama.cpp, OpenAI 使用 OpenAI 兼容 API
		if apiKey == "" {
			apiKey = "sk-dummy"
		}
		p = NewOpenAICompatibleClient(apiKey, baseURL, model)
	case "modelscope":
		if apiKey == "" {
			apiKey = os.Getenv("MODELSCOPE_API_KEY")
			apiKey = readAPIKey(apiKey)
		}
		p = NewModelScopeClient(apiKey, baseURL, model)
	default:
		// 默认使用 OpenAI 兼容 API
		if apiKey == "" {
			apiKey = "sk-dummy"
		}
		p = NewOpenAICompatibleClient(apiKey, baseURL, model)
	}

	return &Client{
		Provider: p,
		Model:   model,
	}
}

func (c *Client) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	return c.Provider.Generate(ctx, prompt, opts...)
}

func (c *Client) GenerateJSON(ctx context.Context, prompt string, opts ...Option) (map[string]interface{}, error) {
	return c.Provider.GenerateJSON(ctx, prompt, opts...)
}

func (c *Client) GenerateWithRetry(ctx context.Context, prompt string, maxRetries int, opts ...Option) (string, error) {
	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		result, err := c.Generate(ctx, prompt, opts...)
		if err == nil && result != "" {
			return result, nil
		}
		lastErr = err
		if i < maxRetries {
			time.Sleep(time.Duration(1<<uint(i)) * time.Second)
		}
	}
	return "", lastErr
}

// ========== Helper Functions ==========

func extractJSON(content string) string {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 || end < start {
		start = strings.Index(content, "[")
		end = strings.LastIndex(content, "]")
	}
	if start == -1 || end == -1 || end < start {
		return content
	}
	return content[start : end+1]
}

func parseJSONResponse(content string) (map[string]interface{}, error) {
	jsonStr := extractJSON(content)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return result, nil
}

func extractNarrative(reasoning string) string {
	re := regexp.MustCompile(`<think>.*?</think>`)
	cleaned := re.ReplaceAllString(reasoning, "")
	cleaned = strings.TrimSpace(cleaned)

	lines := strings.Split(cleaned, "\n")
	var narrative []string
	for _, line := range lines {
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "Thinking") || strings.HasPrefix(stripped, "分析") ||
			strings.HasPrefix(stripped, "思考") || strings.HasPrefix(stripped, "Step") ||
			strings.HasPrefix(stripped, "Draft") || strings.HasPrefix(stripped, "检查") {
			continue
		}
		if len(stripped) > 0 && (stripped[0] == '*' || stripped[0] == '#') {
			continue
		}
		chineseCount := 0
		for _, r := range stripped {
			if r >= 0x4e00 && r <= 0x9fff {
				chineseCount++
			}
		}
		if len(stripped) > 30 && float64(chineseCount)/float64(len(stripped)) > 0.6 {
			narrative = append(narrative, line)
		}
	}
	return strings.Join(narrative, "\n")
}

func GenerateID() string {
	return fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8])
}
