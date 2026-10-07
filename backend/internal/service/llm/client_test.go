package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProviderHTTPClientUsesContextForTotalTimeout(t *testing.T) {
	client := newProviderHTTPClient()
	if client.Timeout != 0 {
		t.Fatalf("http client timeout %s would override per-call context", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.TLSHandshakeTimeout != 10*time.Second || transport.ResponseHeaderTimeout != time.Duration(maxLLMTimeoutSecond)*time.Second || transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("unexpected transport timeouts: %#v", transport)
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Fatalf("unexpected per-host idle connection limit: %d", transport.MaxIdleConnsPerHost)
	}
}

func TestProviderConstructorsDoNotOverridePerCallTimeout(t *testing.T) {
	clients := []*http.Client{
		NewModelScopeClient("key", "https://example.com/v1", "model").Client,
		NewOpenAICompatibleClient("", "https://example.com", "model").Client,
		NewOllamaClient("http://localhost:11434", "model").Client,
	}
	for _, client := range clients {
		if client.Timeout != 0 {
			t.Fatalf("constructor installed conflicting total timeout: %s", client.Timeout)
		}
	}
}

func TestOpenAIEndpointAcceptsBaseWithOrWithoutV1(t *testing.T) {
	tests := map[string]string{
		"https://api.example.com":            "https://api.example.com/v1/chat/completions",
		"https://api.example.com/":           "https://api.example.com/v1/chat/completions",
		"https://api.example.com/v1":         "https://api.example.com/v1/chat/completions",
		"https://api.example.com/v1/":        "https://api.example.com/v1/chat/completions",
		"https://api.example.com/custom/v1/": "https://api.example.com/custom/v1/chat/completions",
	}
	for baseURL, expected := range tests {
		if actual := openAIEndpoint(baseURL, "/chat/completions"); actual != expected {
			t.Errorf("base %q produced %q, want %q", baseURL, actual, expected)
		}
	}
}

func TestResolveOptionsRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
	}{
		{name: "negative temperature", opt: WithTemperature(-0.1)},
		{name: "large temperature", opt: WithTemperature(2.1)},
		{name: "zero top p", opt: WithTopP(0)},
		{name: "large top p", opt: WithTopP(1.1)},
		{name: "zero tokens", opt: WithMaxTokens(0)},
		{name: "large tokens", opt: WithMaxTokens(maxLLMOutputTokens + 1)},
		{name: "zero timeout", opt: WithTimeout(0)},
		{name: "large timeout", opt: WithTimeout(maxLLMTimeoutSecond + 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := resolveOptions(tt.opt); err == nil {
				t.Fatal("expected invalid option error")
			}
		})
	}
	options, err := resolveOptions(WithTemperature(0), WithTopP(1), WithMaxTokens(maxLLMOutputTokens), WithTimeout(maxLLMTimeoutSecond))
	if err != nil || options.MaxTokens != maxLLMOutputTokens {
		t.Fatalf("valid boundary options rejected: %#v, %v", options, err)
	}
}

func TestWithRequestTimeoutDoesNotExtendParentDeadline(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelParent()
	child, cancelChild := withRequestTimeout(parent, 10)
	defer cancelChild()
	parentDeadline, _ := parent.Deadline()
	childDeadline, ok := child.Deadline()
	if !ok || !childDeadline.Equal(parentDeadline) {
		t.Fatalf("child deadline %v must equal earlier parent deadline %v", childDeadline, parentDeadline)
	}
}

type failingProvider struct{}

func (failingProvider) Generate(context.Context, string, ...Option) (string, error) {
	return "", &ProviderHTTPError{StatusCode: http.StatusServiceUnavailable}
}

func (failingProvider) GenerateJSON(context.Context, string, ...Option) (map[string]interface{}, error) {
	return nil, &ProviderHTTPError{StatusCode: http.StatusServiceUnavailable}
}

func TestRetryableProviderErrorClassification(t *testing.T) {
	if isRetryableProviderError(&ProviderHTTPError{StatusCode: http.StatusUnauthorized}) {
		t.Fatal("authentication errors must not be retried")
	}
	if !isRetryableProviderError(&ProviderHTTPError{StatusCode: http.StatusTooManyRequests}) {
		t.Fatal("rate limits should be retried")
	}
	if !isRetryableProviderError(&ProviderHTTPError{StatusCode: http.StatusBadGateway}) {
		t.Fatal("server errors should be retried")
	}
	if isRetryableProviderError(context.DeadlineExceeded) {
		t.Fatal("expired calls must not be retried")
	}
	if isRetryableProviderError(errors.New("failed to parse response")) {
		t.Fatal("deterministic parse errors must not be retried")
	}
}

func TestGenerateWithRetryStopsDuringBackoffWhenContextEnds(t *testing.T) {
	client := &Client{Provider: failingProvider{}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := client.GenerateWithRetry(ctx, "prompt", 3)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("retry backoff ignored cancellation: %s", elapsed)
	}
}

func TestParseJSONResponseAcceptsObjectWithFenceOrExplanation(t *testing.T) {
	tests := []string{
		"```json\n{\"title\":\"章节\"}\n```",
		"以下是结果：\n{\"title\":\"章节\"}\n请查收。",
	}
	for _, content := range tests {
		result, err := parseJSONResponse(content)
		if err != nil {
			t.Fatalf("valid object rejected: %v", err)
		}
		if result["title"] != "章节" {
			t.Fatalf("unexpected parsed object: %#v", result)
		}
	}
}

func TestParseJSONResponseRejectsArrayEmptyAndMalformedValues(t *testing.T) {
	tests := []string{
		`[{"title":"章节"}]`,
		`{}`,
		"```json\n{}\n```",
		`not json`,
		`{"title":}`,
	}
	for _, content := range tests {
		if _, err := parseJSONResponse(content); err == nil {
			t.Fatalf("invalid structured output accepted: %q", content)
		}
	}
}

func TestReadBoundedResponseRejectsOversizedBody(t *testing.T) {
	_, err := readBoundedResponse(strings.NewReader("123456"), 5)
	if err == nil || !strings.Contains(err.Error(), "exceeds 5 bytes") {
		t.Fatalf("expected bounded response error, got %v", err)
	}
	body, err := readBoundedResponse(strings.NewReader("中文"), 6)
	if err != nil || string(body) != "中文" {
		t.Fatalf("unexpected bounded Unicode response: %q, %v", body, err)
	}
}

func TestDecodeOpenAIStreamParsesSSEUnicodeAndDone(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"中文\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"🙂\"}}]}\n\n" +
		"data: [DONE]\n\n"
	result, err := decodeOpenAIStream(strings.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	if result != "中文🙂" {
		t.Fatalf("unexpected stream content: %q", result)
	}
}

func TestDecodeOpenAIStreamRejectsMalformedAndEmptyStreams(t *testing.T) {
	if _, err := decodeOpenAIStream(strings.NewReader("data: not-json\n")); err == nil {
		t.Fatal("expected malformed event error")
	}
	if _, err := decodeOpenAIStream(strings.NewReader("data: [DONE]\n")); err == nil {
		t.Fatal("expected empty stream error")
	}
}

func TestDecodeOpenAIStreamRejectsOversizedEvent(t *testing.T) {
	stream := "data: " + strings.Repeat("x", maxStreamEventBytes+1) + "\n"
	if _, err := decodeOpenAIStream(strings.NewReader(stream)); err == nil {
		t.Fatal("expected oversized event error")
	}
}

func TestModelScopeSendsValidatedGenerationOptions(t *testing.T) {
	var request map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	client := NewModelScopeClient("secret", server.URL, "test-model")
	if _, err := client.Generate(context.Background(), "prompt", WithTemperature(0.4), WithTopP(0.8), WithMaxTokens(1234)); err != nil {
		t.Fatal(err)
	}
	if request["temperature"] != 0.4 || request["top_p"] != 0.8 || request["max_tokens"] != float64(1234) {
		t.Fatalf("generation options not forwarded: %#v", request)
	}
}

func TestOpenAICompatibleClientDoesNotDuplicateV1(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	client := NewOpenAICompatibleClient("", server.URL+"/v1", "test-model")
	if _, err := client.Generate(context.Background(), "prompt"); err != nil {
		t.Fatal(err)
	}
	if path != "/v1/chat/completions" {
		t.Fatalf("unexpected request path: %q", path)
	}
}

func TestOpenAICompatibleClientRejectsMalformedNonStreamResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()
	client := NewOpenAICompatibleClient("", server.URL, "test-model")
	_, err := client.Generate(context.Background(), "prompt", WithStream(false))
	if err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestOpenAICompatibleClientHonorsPerCallTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"late"}}]}`))
		}
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient("", server.URL, "test-model")
	started := time.Now()
	_, err := client.Generate(context.Background(), "prompt", WithTimeout(1))
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 1500*time.Millisecond {
		t.Fatalf("per-call timeout was not enforced promptly: %s", elapsed)
	}
}
