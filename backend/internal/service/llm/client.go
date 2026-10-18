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
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Provider interface
type Provider interface {
	Generate(ctx context.Context, prompt string, opts ...Option) (string, error)
	GenerateJSON(ctx context.Context, prompt string, opts ...Option) (map[string]interface{}, error)
}

type ProviderHTTPError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *ProviderHTTPError) Error() string {
	return fmt.Sprintf("provider HTTP error: status %d", e.StatusCode)
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
	maxLLMRetries       = 5
	maxLLMRetryBackoff  = 30 * time.Second
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

func validateProviderBaseURL(baseURL string) error {
	value := strings.TrimSpace(baseURL)
	if value == "" || len(value) > 2048 {
		return fmt.Errorf("provider endpoint is invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("provider endpoint is invalid")
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("provider endpoint is invalid")
	}
	return nil
}

func validateProviderIdentity(model, apiKey string) error {
	if strings.TrimSpace(model) == "" || len(model) > 512 || !utf8.ValidString(model) {
		return fmt.Errorf("provider model is invalid")
	}
	for _, character := range model {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) || (unicode.IsSpace(character) && character != ' ') {
			return fmt.Errorf("provider model is invalid")
		}
	}
	if apiKey == "" {
		return nil
	}
	if len(apiKey) > 16<<10 || !utf8.ValidString(apiKey) {
		return fmt.Errorf("provider credential is invalid")
	}
	for _, character := range apiKey {
		if unicode.IsSpace(character) || unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return fmt.Errorf("provider credential is invalid")
		}
	}
	return nil
}

func validateProviderTransport(baseURL, apiKey string) error {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("provider endpoint is invalid")
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return nil
	}
	if strings.TrimSpace(apiKey) != "" {
		return fmt.Errorf("provider credentials require an HTTPS endpoint")
	}
	if !isLocalProviderEndpoint(baseURL) {
		return fmt.Errorf("plaintext provider endpoint must be local")
	}
	return nil
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
	// Provider traffic may contain unpublished text and credentials. Do not let
	// ambient process proxy variables silently change its destination or expose
	// plaintext local-provider requests. Proxying must be an explicit future feature.
	transport.Proxy = nil
	// Keep response limits tied to bytes supplied by the Provider. Automatic gzip
	// negotiation hides the original encoding and changes the byte stream before validation.
	transport.DisableCompression = true
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = time.Duration(maxLLMTimeoutSecond) * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.IdleConnTimeout = 90 * time.Second
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// Never forward unpublished text, prompts, embedding inputs or bearer
			// credentials to a redirect target chosen by a Provider response.
			return http.ErrUseLastResponse
		},
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
		Client:  newProviderHTTPClient(),
	}
}

func (c *ModelScopeClient) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validatePrompt(prompt); err != nil {
		return "", err
	}
	if err := validateProviderBaseURL(c.BaseURL); err != nil {
		return "", err
	}
	if err := validateProviderIdentity(c.Model, c.APIKey); err != nil {
		return "", err
	}
	if err := validateProviderTransport(c.BaseURL, c.APIKey); err != nil {
		return "", err
	}
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

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode provider request")
	}
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
	if err := validateProviderContentType(resp, useStream); err != nil {
		return "", err
	}

	if useStream {
		return c.handleStreamResponse(resp)
	}

	body, err := readBoundedUTF8Response(resp.Body, maxLLMResponseBytes)
	if err != nil {
		return "", err
	}
	var result struct {
		Choices []struct {
			Message      struct{ Content string } `json:"message"`
			FinishReason string                   `json:"finish_reason"`
			Index        *int                     `json:"index"`
		} `json:"choices"`
	}
	if err := unmarshalProviderJSON(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Choices) != 1 {
		return "", fmt.Errorf("provider response must contain exactly one choice")
	}
	if err := validateSingleChoice(len(result.Choices), result.Choices[0].Index); err != nil {
		return "", err
	}
	if err := validateFinishReason(result.Choices[0].FinishReason); err != nil {
		return "", err
	}
	return requireProviderText(result.Choices[0].Message.Content)
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
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validatePrompt(prompt); err != nil {
		return "", err
	}
	if err := validateProviderBaseURL(c.BaseURL); err != nil {
		return "", err
	}
	if err := validateProviderIdentity(c.Model, c.APIKey); err != nil {
		return "", err
	}
	if err := validateProviderTransport(c.BaseURL, c.APIKey); err != nil {
		return "", err
	}
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

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode provider request")
	}
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
	if err := validateProviderContentType(resp, useStream); err != nil {
		return "", err
	}

	if useStream {
		return c.handleStreamResponse(resp)
	}

	body, err := readBoundedUTF8Response(resp.Body, maxLLMResponseBytes)
	if err != nil {
		return "", err
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
			Index        *int   `json:"index"`
		} `json:"choices"`
	}
	if err := unmarshalProviderJSON(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Choices) != 1 {
		return "", fmt.Errorf("provider response must contain exactly one choice")
	}
	if err := validateSingleChoice(len(result.Choices), result.Choices[0].Index); err != nil {
		return "", err
	}
	if err := validateFinishReason(result.Choices[0].FinishReason); err != nil {
		return "", err
	}

	content := result.Choices[0].Message.Content
	// Handle Qwen thinking mode
	if strings.TrimSpace(content) == "" && strings.TrimSpace(result.Choices[0].Message.ReasoningContent) != "" {
		content = extractNarrative(result.Choices[0].Message.ReasoningContent)
	}
	return requireProviderText(content)
}

func (c *OpenAICompatibleClient) handleStreamResponse(resp *http.Response) (string, error) {
	return decodeOpenAIStream(resp.Body)
}

const (
	maxLLMPromptRunes     = 1_000_000
	maxErrorResponseBytes = 64 << 10
	maxLLMResponseBytes   = 16 << 20
	maxStreamEventBytes   = 1 << 20
	maxStreamInputBytes   = 16 << 20
	maxStreamOutputBytes  = 16 << 20
)

func validateProviderContentType(resp *http.Response, streaming bool) error {
	if len(resp.Header.Values("Content-Type")) != 1 || len(resp.Header.Values("Content-Encoding")) != 0 {
		return fmt.Errorf("provider returned unsupported response headers")
	}
	mediaType, parameters, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return fmt.Errorf("provider returned an unsupported content type")
	}
	if len(parameters) > 1 {
		return fmt.Errorf("provider returned an unsupported content type")
	}
	if len(parameters) == 1 {
		charset, exists := parameters["charset"]
		if !exists || !strings.EqualFold(strings.TrimSpace(charset), "utf-8") {
			return fmt.Errorf("provider returned an unsupported content type")
		}
	}
	mediaType = strings.ToLower(mediaType)
	if streaming {
		if mediaType == "text/event-stream" {
			return nil
		}
	} else if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		return nil
	}
	return fmt.Errorf("provider returned an unsupported content type")
}

func parseRetryAfter(values []string, now time.Time) time.Duration {
	if len(values) != 1 {
		return 0
	}
	value := strings.TrimSpace(values[0])
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		if seconds == 0 {
			return 0
		}
		if seconds >= uint64(maxLLMRetryBackoff/time.Second) {
			return maxLLMRetryBackoff
		}
		return time.Duration(seconds) * time.Second
	}
	retryAt, err := http.ParseTime(value)
	if err != nil || !retryAt.After(now) {
		return 0
	}
	delay := retryAt.Sub(now)
	if delay > maxLLMRetryBackoff {
		return maxLLMRetryBackoff
	}
	return delay
}

func providerHTTPError(resp *http.Response) error {
	// Drain only a bounded error body so the connection can be reused, then
	// discard it immediately. Provider diagnostics may echo private content.
	_, _ = readBoundedResponse(resp.Body, maxErrorResponseBytes)
	return &ProviderHTTPError{
		StatusCode: resp.StatusCode,
		RetryAfter: parseRetryAfter(resp.Header.Values("Retry-After"), time.Now()),
	}
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

const (
	maxProviderJSONDepth      = 64
	maxProviderJSONEntries    = 10000
	maxProviderJSONTotalNodes = 20000
)

type providerJSONComplexity struct {
	totalNodes int
}

func scanProviderJSON(decoder *json.Decoder, depth int, complexity *providerJSONComplexity) error {
	if depth > maxProviderJSONDepth {
		return fmt.Errorf("provider JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		complexity.totalNodes++
		if complexity.totalNodes > maxProviderJSONTotalNodes {
			return fmt.Errorf("provider JSON structure exceeds limit")
		}
		return nil
	}
	entries := 0
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			entries++
			complexity.totalNodes++
			if entries > maxProviderJSONEntries || complexity.totalNodes > maxProviderJSONTotalNodes {
				return fmt.Errorf("provider JSON structure exceeds limit")
			}
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("provider JSON object key is invalid")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("provider JSON contains duplicate keys")
			}
			seen[key] = struct{}{}
			if err := scanProviderJSON(decoder, depth+1, complexity); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			entries++
			complexity.totalNodes++
			if entries > maxProviderJSONEntries || complexity.totalNodes > maxProviderJSONTotalNodes {
				return fmt.Errorf("provider JSON structure exceeds limit")
			}
			if err := scanProviderJSON(decoder, depth+1, complexity); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("provider JSON delimiter is invalid")
	}
	_, err = decoder.Token()
	return err
}

func unmarshalProviderJSON(body []byte, destination interface{}) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("provider returned an invalid JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := scanProviderJSON(decoder, 1, &providerJSONComplexity{}); err != nil {
		return fmt.Errorf("provider returned an invalid JSON object")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("provider returned an invalid JSON object")
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("provider returned an invalid JSON object")
	}
	return nil
}

func readBoundedUTF8Response(reader io.Reader, maxBytes int64) ([]byte, error) {
	body, err := readBoundedResponse(reader, maxBytes)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(body) {
		return nil, fmt.Errorf("provider returned invalid UTF-8")
	}
	return body, nil
}

func decodeOpenAIStream(reader io.Reader) (string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxStreamEventBytes)
	var content, reasoning strings.Builder
	finishReason := ""
	completed := false
	inputBytes := 0
	for scanner.Scan() {
		inputBytes += len(scanner.Bytes()) + 1
		if inputBytes > maxStreamInputBytes {
			return "", fmt.Errorf("stream response exceeds %d input bytes", maxStreamInputBytes)
		}
		if !utf8.Valid(scanner.Bytes()) {
			return "", fmt.Errorf("provider returned invalid UTF-8")
		}
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if completed {
			return "", fmt.Errorf("stream response contained data after completion")
		}
		if !strings.HasPrefix(line, "data:") {
			return "", fmt.Errorf("provider returned an unsupported SSE field")
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if line == "" {
			return "", fmt.Errorf("provider returned an empty SSE data field")
		}
		if line == "[DONE]" {
			completed = true
			continue
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
				Index        *int   `json:"index"`
			} `json:"choices"`
		}
		if err := unmarshalProviderJSON([]byte(line), &event); err != nil {
			return "", fmt.Errorf("failed to parse stream event: %w", err)
		}
		if len(event.Choices) == 0 {
			continue
		}
		if err := validateSingleChoice(len(event.Choices), event.Choices[0].Index); err != nil {
			return "", err
		}
		content.WriteString(event.Choices[0].Delta.Content)
		reasoning.WriteString(event.Choices[0].Delta.ReasoningContent)
		if event.Choices[0].FinishReason != "" {
			finishReason = event.Choices[0].FinishReason
		}
		if content.Len()+reasoning.Len() > maxStreamOutputBytes {
			return "", fmt.Errorf("stream response exceeds %d bytes", maxStreamOutputBytes)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read stream response: %w", err)
	}
	if !completed {
		return "", fmt.Errorf("stream response ended before completion")
	}
	if err := validateFinishReason(finishReason); err != nil {
		return "", err
	}
	if strings.TrimSpace(content.String()) != "" {
		return requireProviderText(content.String())
	}
	if strings.TrimSpace(reasoning.String()) != "" {
		return requireProviderText(extractNarrative(reasoning.String()))
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
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validatePrompt(prompt); err != nil {
		return "", err
	}
	if err := validateProviderBaseURL(c.BaseURL); err != nil {
		return "", err
	}
	if err := validateProviderIdentity(c.Model, ""); err != nil {
		return "", err
	}
	if err := validateProviderTransport(c.BaseURL, ""); err != nil {
		return "", err
	}
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

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode provider request")
	}
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
	if err := validateProviderContentType(resp, false); err != nil {
		return "", err
	}

	body, err := readBoundedUTF8Response(resp.Body, maxLLMResponseBytes)
	if err != nil {
		return "", err
	}
	var result struct {
		Response string `json:"response"`
	}
	if err := unmarshalProviderJSON(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	return requireProviderText(result.Response)
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

func NewClient(provider, apiKey, baseURL, model string) (*Client, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	apiKey = strings.TrimSpace(apiKey)
	baseURL = strings.TrimSpace(baseURL)
	model = strings.TrimSpace(model)
	if baseURL == "" || model == "" {
		return nil, fmt.Errorf("LLM endpoint and model are required")
	}
	if apiKey != "" && !strings.HasPrefix(strings.ToLower(baseURL), "https://") {
		return nil, fmt.Errorf("LLM credentials require an HTTPS endpoint")
	}
	if (provider == "modelscope" || provider == "openai") && apiKey == "" {
		return nil, fmt.Errorf("remote LLM provider requires credentials")
	}

	var p Provider
	switch provider {
	case "ollama":
		p = NewOllamaClient(baseURL, model)
	case "sglang", "llama", "openai":
		p = NewOpenAICompatibleClient(apiKey, baseURL, model)
	case "modelscope":
		p = NewModelScopeClient(apiKey, baseURL, model)
	default:
		return nil, fmt.Errorf("unsupported LLM provider")
	}

	return &Client{Provider: p, Model: model}, nil
}

func (c *Client) Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
	return c.Provider.Generate(ctx, prompt, opts...)
}

func (c *Client) GenerateJSON(ctx context.Context, prompt string, opts ...Option) (map[string]interface{}, error) {
	return c.Provider.GenerateJSON(ctx, prompt, opts...)
}

func retryBackoff(attempt int, retryAfter time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := time.Second
	for index := 0; index < attempt && delay < maxLLMRetryBackoff; index++ {
		delay *= 2
	}
	if retryAfter > delay {
		delay = retryAfter
	}
	if delay > maxLLMRetryBackoff {
		return maxLLMRetryBackoff
	}
	return delay
}

func (c *Client) GenerateWithRetry(ctx context.Context, prompt string, maxRetries int, opts ...Option) (string, error) {
	if maxRetries < 0 || maxRetries > maxLLMRetries {
		return "", fmt.Errorf("max retries must be between 0 and %d", maxLLMRetries)
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
		var retryAfter time.Duration
		var httpErr *ProviderHTTPError
		if errors.As(err, &httpErr) {
			retryAfter = httpErr.RetryAfter
		}
		timer := time.NewTimer(retryBackoff(attempt, retryAfter))
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

func validatePrompt(prompt string) error {
	if !utf8.ValidString(prompt) {
		return fmt.Errorf("prompt must be valid UTF-8")
	}
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt must not be empty")
	}
	if utf8.RuneCountInString(prompt) > maxLLMPromptRunes {
		return fmt.Errorf("prompt exceeds %d characters", maxLLMPromptRunes)
	}
	return nil
}

func validateSingleChoice(count int, index *int) error {
	if count != 1 {
		return fmt.Errorf("provider response must contain exactly one choice")
	}
	if index != nil && *index != 0 {
		return fmt.Errorf("provider response contains an unexpected choice index")
	}
	return nil
}

func validateFinishReason(reason string) error {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "content_filter":
		return fmt.Errorf("provider response was not completed")
	default:
		return nil
	}
}

func requireProviderText(content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("provider response contained no content")
	}
	return content, nil
}

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
	if objectStart < 0 {
		return "", fmt.Errorf("JSON object not found")
	}
	depth := 0
	inString := false
	escaped := false
	for index := objectStart; index < len(content); index++ {
		current := content[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch current {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch current {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				remainder := strings.TrimSpace(content[index+1:])
				if strings.HasPrefix(remainder, "{") || strings.HasPrefix(remainder, "[") {
					return "", fmt.Errorf("multiple JSON values found")
				}
				return content[objectStart : index+1], nil
			}
			if depth < 0 {
				return "", fmt.Errorf("invalid JSON object boundary")
			}
		}
	}
	return "", fmt.Errorf("unterminated JSON object")
}

func parseJSONResponse(content string) (map[string]interface{}, error) {
	jsonStr, err := extractJSONObject(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	var result map[string]interface{}
	if err := unmarshalProviderJSON([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: invalid structured output")
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("failed to parse JSON: top-level object is empty")
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
