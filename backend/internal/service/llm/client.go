package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Provider interface
type Provider interface {
	Generate(ctx context.Context, prompt string, opts ...Option) (string, error)
	GenerateJSON(ctx context.Context, prompt string, opts ...Option) (map[string]interface{}, error)
}

type ProviderHTTPError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *ProviderHTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("API error: %d", e.StatusCode)
	}
	return fmt.Sprintf("API error: %d - %s", e.StatusCode, e.Body)
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
		TopP:        0.9,
		Timeout:     300,
		Stream:      false,
	}
}

const (
	maxLLMOutputTokens  = 40000
	maxLLMTimeoutSecond = 1200
)

func resolveOptions(opts ...Option) (*Options, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}
	if math.IsNaN(options.Temperature) || math.IsInf(options.Temperature, 0) || options.Temperature < 0 || options.Temperature > 2 {
		return nil, fmt.Errorf("temperature must be between 0 and 2")
	}
	if math.IsNaN(options.TopP) || math.IsInf(options.TopP, 0) || options.TopP <= 0 || options.TopP > 1 {
		return nil, fmt.Errorf("top_p must be greater than 0 and at most 1")
	}
	if options.MaxTokens < 1 || options.MaxTokens > maxLLMOutputTokens {
		return nil, fmt.Errorf("max_tokens must be between 1 and %d", maxLLMOutputTokens)
	}
	if options.Timeout < 1 || options.Timeout > maxLLMTimeoutSecond {
		return nil, fmt.Errorf("timeout must be between 1 and %d seconds", maxLLMTimeoutSecond)
	}
	return options, nil
}

func withRequestTimeout(ctx context.Context, timeoutSeconds int) (context.Context, context.CancelFunc) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = DefaultOptions().Timeout
	}
	return context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
}

func openAIEndpoint(baseURL, endpoint string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpoint = strings.TrimLeft(endpoint, "/")
	if strings.HasSuffix(strings.ToLower(baseURL), "/v1") {
		return baseURL + "/" + endpoint
	}
	return baseURL + "/v1/" + endpoint
}

func newProviderHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = time.Duration(maxLLMTimeoutSecond) * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.IdleConnTimeout = 90 * time.Second
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	return &http.Client{Transport: transport}
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
		Client:  newProviderHTTPClient(),
	}
}

func (c *ModelScopeClient) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	options, err := resolveOptions(opts...)
	if err != nil {
		return "", err
	}
	requestCtx, cancel := withRequestTimeout(ctx, options.Timeout)
	defer cancel()

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
		"max_tokens":  options.MaxTokens,
		"stream":      useStream,
	}

	jsonData, _ := json.Marshal(reqBody)
	url := openAIEndpoint(c.BaseURL, "chat/completions")

	req, err := http.NewRequestWithContext(requestCtx, "POST", url, bytes.NewBuffer(jsonData))
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
		return "", providerHTTPError(resp)
	}

	if useStream {
		return c.handleStreamResponse(resp)
	}

	body, err := readBoundedResponse(resp.Body, maxLLMResponseBytes)
	if err != nil {
		return "", err
	}
	var result struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response")
	}
	return result.Choices[0].Message.Content, nil
}

func (c *ModelScopeClient) handleStreamResponse(resp *http.Response) (string, error) {
	return decodeOpenAIStream(resp.Body)
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
		Client:  newProviderHTTPClient(),
	}
}

func (c *OpenAICompatibleClient) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	options, err := resolveOptions(opts...)
	if err != nil {
		return "", err
	}
	requestCtx, cancel := withRequestTimeout(ctx, options.Timeout)
	defer cancel()

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
		"stream":      useStream,
	}
	if options.MaxTokens > 0 {
		reqBody["max_tokens"] = options.MaxTokens
	}

	jsonData, _ := json.Marshal(reqBody)
	url := openAIEndpoint(c.BaseURL, "chat/completions")

	req, err := http.NewRequestWithContext(requestCtx, "POST", url, bytes.NewBuffer(jsonData))
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
		return "", providerHTTPError(resp)
	}

	if useStream {
		return c.handleStreamResponse(resp)
	}

	body, err := readBoundedResponse(resp.Body, maxLLMResponseBytes)
	if err != nil {
		return "", err
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
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
	return decodeOpenAIStream(resp.Body)
}

const (
	maxErrorResponseBytes = 64 << 10
	maxLLMResponseBytes   = 16 << 20
	maxStreamEventBytes   = 1 << 20
	maxStreamOutputBytes  = 16 << 20
)

func providerHTTPError(resp *http.Response) error {
	body, err := readBoundedResponse(resp.Body, maxErrorResponseBytes)
	if err != nil {
		body = []byte(err.Error())
	}
	retryAfter := time.Duration(0)
	if seconds, parseErr := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); parseErr == nil && seconds > 0 {
		retryAfter = time.Duration(seconds) * time.Second
	} else if retryAt, dateErr := http.ParseTime(resp.Header.Get("Retry-After")); dateErr == nil {
		retryAfter = time.Until(retryAt)
		if retryAfter < 0 {
			retryAfter = 0
		}
	}
	return &ProviderHTTPError{StatusCode: resp.StatusCode, Body: string(body), RetryAfter: retryAfter}
}

func readBoundedResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBytes)
	}
	return body, nil
}

func decodeOpenAIStream(reader io.Reader) (string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxStreamEventBytes)
	var content, reasoning strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "[DONE]" {
			break
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return "", fmt.Errorf("failed to parse stream event: %w", err)
		}
		if len(event.Choices) == 0 {
			continue
		}
		content.WriteString(event.Choices[0].Delta.Content)
		reasoning.WriteString(event.Choices[0].Delta.ReasoningContent)
		if content.Len()+reasoning.Len() > maxStreamOutputBytes {
			return "", fmt.Errorf("stream response exceeds %d bytes", maxStreamOutputBytes)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read stream response: %w", err)
	}
	if content.Len() > 0 {
		return content.String(), nil
	}
	if reasoning.Len() > 0 {
		return extractNarrative(reasoning.String()), nil
	}
	return "", fmt.Errorf("stream response contained no content")
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
		Client:  newProviderHTTPClient(),
	}
}

func (c *OllamaClient) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	options, err := resolveOptions(opts...)
	if err != nil {
		return "", err
	}
	requestCtx, cancel := withRequestTimeout(ctx, options.Timeout)
	defer cancel()

	reqBody := map[string]interface{}{
		"model":       c.Model,
		"prompt":      prompt,
		"stream":      false,
		"temperature": options.Temperature,
		"top_p":       options.TopP,
	}
	if options.MaxTokens > 0 {
		reqBody["options"] = map[string]interface{}{
			"num_predict": options.MaxTokens,
		}
	}

	jsonData, _ := json.Marshal(reqBody)
	url := c.BaseURL + "/api/generate"

	req, err := http.NewRequestWithContext(requestCtx, "POST", url, bytes.NewBuffer(jsonData))
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
		return "", providerHTTPError(resp)
	}

	body, err := readBoundedResponse(resp.Body, maxLLMResponseBytes)
	if err != nil {
		return "", err
	}
	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Response == "" {
		return "", fmt.Errorf("no response")
	}
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
	Model    string
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
		Model:    model,
	}
}

func (c *Client) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	return c.Provider.Generate(ctx, prompt, opts...)
}

func (c *Client) GenerateJSON(ctx context.Context, prompt string, opts ...Option) (map[string]interface{}, error) {
	return c.Provider.GenerateJSON(ctx, prompt, opts...)
}

func (c *Client) GenerateWithRetry(ctx context.Context, prompt string, maxRetries int, opts ...Option) (string, error) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := c.Generate(ctx, prompt, opts...)
		if err == nil && result != "" {
			return result, nil
		}
		if err == nil {
			err = fmt.Errorf("provider returned empty response")
		}
		lastErr = err
		if attempt == maxRetries || !isRetryableProviderError(err) {
			return "", err
		}
		delay := time.Duration(1<<uint(attempt)) * time.Second
		var httpErr *ProviderHTTPError
		if errors.As(err, &httpErr) && httpErr.RetryAfter > delay {
			delay = httpErr.RetryAfter
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "", lastErr
}

func isRetryableProviderError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var httpErr *ProviderHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusRequestTimeout || httpErr.StatusCode == http.StatusTooManyRequests || httpErr.StatusCode >= 500
	}
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}

// ========== Helper Functions ==========

func extractJSONObject(content string) (string, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		firstNewline := strings.IndexByte(content, '\n')
		lastFence := strings.LastIndex(content, "```")
		if firstNewline < 0 || lastFence <= firstNewline {
			return "", fmt.Errorf("invalid JSON code block")
		}
		content = strings.TrimSpace(content[firstNewline+1 : lastFence])
	}
	objectStart := strings.Index(content, "{")
	arrayStart := strings.Index(content, "[")
	if arrayStart >= 0 && (objectStart < 0 || arrayStart < objectStart) {
		return "", fmt.Errorf("top-level JSON value must be an object")
	}
	objectEnd := strings.LastIndex(content, "}")
	if objectStart < 0 || objectEnd < objectStart {
		return "", fmt.Errorf("JSON object not found")
	}
	return content[objectStart : objectEnd+1], nil
}

func parseJSONResponse(content string) (map[string]interface{}, error) {
	jsonStr, err := extractJSONObject(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	var result map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("failed to parse JSON: top-level object is empty")
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to parse JSON: multiple top-level values")
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
