package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestValidatePromptUsesUnicodeCharactersAndRejectsInvalidInput(t *testing.T) {
	valid := strings.Repeat("界", maxLLMPromptRunes)
	if err := validatePrompt(valid); err != nil {
		t.Fatalf("valid Unicode prompt rejected: %v", err)
	}
	for _, prompt := range []string{"", " \r\n\t\u3000", valid + "界", string([]byte{0xff})} {
		if err := validatePrompt(prompt); err == nil {
			t.Fatalf("invalid prompt accepted: bytes=%d", len(prompt))
		}
	}
}

func TestValidateProviderTransportRestrictsPlaintextToExplicitLocalEndpoints(t *testing.T) {
	allowed := []struct {
		endpoint string
		key      string
	}{
		{endpoint: "https://api.example.com/v1", key: "secret"},
		{endpoint: "https://api.example.com/v1"},
		{endpoint: "http://localhost:8080/v1"},
		{endpoint: "http://127.0.0.1:8080/v1"},
		{endpoint: "http://[::1]:8080/v1"},
		{endpoint: "http://10.0.0.200:8080/v1"},
	}
	for _, test := range allowed {
		if err := validateProviderTransport(test.endpoint, test.key); err != nil {
			t.Fatalf("allowed transport rejected: %v", err)
		}
	}
	blocked := []struct {
		endpoint string
		key      string
	}{
		{endpoint: "http://api.example.com/v1"},
		{endpoint: "http://8.8.8.8/v1"},
		{endpoint: "http://localhost:8080/v1", key: "secret"},
		{endpoint: "http://10.0.0.200:8080/v1", key: "secret"},
	}
	for _, test := range blocked {
		if err := validateProviderTransport(test.endpoint, test.key); err == nil {
			t.Fatalf("unsafe plaintext transport accepted: endpoint=%q key=%t", test.endpoint, test.key != "")
		}
	}
}

func TestDirectClientsRejectPublicPlaintextBeforeNetwork(t *testing.T) {
	called := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called++
		return nil, errors.New("network must not be reached")
	})
	providers := []Provider{
		NewOpenAICompatibleClient("", "http://api.example.com/v1", "model"),
		NewOllamaClient("http://8.8.8.8:11434", "model"),
	}
	for _, provider := range providers {
		switch client := provider.(type) {
		case *OpenAICompatibleClient:
			client.Client = &http.Client{Transport: transport}
		case *OllamaClient:
			client.Client = &http.Client{Transport: transport}
		}
		if _, err := provider.Generate(context.Background(), "prompt"); err == nil {
			t.Fatalf("%T accepted public plaintext endpoint", provider)
		}
	}
	if called != 0 {
		t.Fatalf("public plaintext calls reached network %d times", called)
	}
}

func TestValidateProviderIdentityRejectsUnsafeModelAndCredential(t *testing.T) {
	if err := validateProviderIdentity("Qwen 3.5", "token-安全"); err != nil {
		t.Fatalf("valid Provider identity rejected: %v", err)
	}
	for _, test := range []struct {
		model string
		key   string
	}{
		{model: ""},
		{model: "   \u3000"},
		{model: strings.Repeat("m", 513)},
		{model: "model\nname"},
		{model: "model\u200bname"},
		{model: string([]byte{0xff})},
		{model: "model", key: "secret key"},
		{model: "model", key: "secret\nkey"},
		{model: "model", key: strings.Repeat("k", (16<<10)+1)},
		{model: "model", key: string([]byte{0xff})},
	} {
		err := validateProviderIdentity(test.model, test.key)
		if err == nil {
			t.Fatalf("unsafe Provider identity accepted: model bytes=%d key bytes=%d", len(test.model), len(test.key))
		}
		if (test.model != "" && strings.Contains(err.Error(), test.model)) || (test.key != "" && strings.Contains(err.Error(), test.key)) {
			t.Fatalf("Provider identity leaked in error: %v", err)
		}
	}
}

func TestValidateProviderBaseURLRejectsUnsafeEndpointComponents(t *testing.T) {
	valid := []string{"http://localhost:8080", "https://example.com/v1", "http://[::1]:8080/v1"}
	for _, endpoint := range valid {
		if err := validateProviderBaseURL(endpoint); err != nil {
			t.Fatalf("valid Provider endpoint rejected: %v", err)
		}
	}
	unsafe := []string{
		"", "localhost:8080", "ftp://example.com", "https://user:secret@example.com/v1",
		"https://example.com/v1?token=secret", "https://example.com/v1#secret", "https:///missing-host",
		"https://example.com/" + strings.Repeat("x", 2048),
	}
	for _, endpoint := range unsafe {
		err := validateProviderBaseURL(endpoint)
		if err == nil {
			t.Fatalf("unsafe Provider endpoint accepted: %q", endpoint)
		}
		if (endpoint != "" && strings.Contains(err.Error(), endpoint)) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("Provider endpoint leaked in error: %v", err)
		}
	}
}

func TestDirectProviderConstructorsRejectUnsafeURLBeforeNetwork(t *testing.T) {
	called := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called++
		return nil, errors.New("network must not be reached")
	})
	clients := []Provider{
		NewModelScopeClient("secret", "https://user:secret@example.com/v1", "model"),
		NewOpenAICompatibleClient("", "http://localhost:8080/v1?private=value", "model"),
		NewOllamaClient("file:///private/model", "model"),
	}
	for _, provider := range clients {
		switch client := provider.(type) {
		case *ModelScopeClient:
			client.Client = &http.Client{Transport: transport}
		case *OpenAICompatibleClient:
			client.Client = &http.Client{Transport: transport}
		case *OllamaClient:
			client.Client = &http.Client{Transport: transport}
		}
		if _, err := provider.Generate(context.Background(), "prompt"); err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") {
			t.Fatalf("%T returned unsafe URL error: %v", provider, err)
		}
	}
	if called != 0 {
		t.Fatalf("unsafe Provider endpoints reached network %d times", called)
	}
}

func TestProviderClientsHonorCanceledContextBeforeInputValidationOrNetwork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called++
		return nil, errors.New("network must not be reached")
	})
	clients := []Provider{
		NewModelScopeClient("secret", "https://example.com/v1", "model"),
		NewOpenAICompatibleClient("", "http://localhost:8080/v1", "model"),
		NewOllamaClient("http://localhost:11434", "model"),
	}
	for _, provider := range clients {
		switch client := provider.(type) {
		case *ModelScopeClient:
			client.Client = &http.Client{Transport: transport}
		case *OpenAICompatibleClient:
			client.Client = &http.Client{Transport: transport}
		case *OllamaClient:
			client.Client = &http.Client{Transport: transport}
		}
		if _, err := provider.Generate(ctx, ""); !errors.Is(err, context.Canceled) {
			t.Fatalf("%T returned %v, want context.Canceled", provider, err)
		}
	}
	if called != 0 {
		t.Fatalf("canceled Provider calls reached network %d times", called)
	}
}

func TestProviderClientsRejectInvalidPromptBeforeNetwork(t *testing.T) {
	called := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called++
		return nil, errors.New("network must not be reached")
	})
	clients := []Provider{
		NewModelScopeClient("secret", "https://example.com/v1", "model"),
		NewOpenAICompatibleClient("", "http://localhost:8080/v1", "model"),
		NewOllamaClient("http://localhost:11434", "model"),
	}
	for _, provider := range clients {
		switch client := provider.(type) {
		case *ModelScopeClient:
			client.Client = &http.Client{Transport: transport}
		case *OpenAICompatibleClient:
			client.Client = &http.Client{Transport: transport}
		case *OllamaClient:
			client.Client = &http.Client{Transport: transport}
		}
		if _, err := provider.Generate(context.Background(), "   "); err == nil {
			t.Fatalf("%T accepted empty prompt", provider)
		}
	}
	if called != 0 {
		t.Fatalf("invalid prompts reached network %d times", called)
	}
}

func TestNewClientRejectsUnknownProviderDummyKeysAndHTTPSecrets(t *testing.T) {
	for name, input := range map[string]struct {
		provider string
		apiKey   string
		baseURL  string
		model    string
	}{
		"unknown provider":       {provider: "typo", baseURL: "http://localhost:8080", model: "model"},
		"OpenAI without key":     {provider: "openai", baseURL: "https://api.example.com/v1", model: "model"},
		"ModelScope without key": {provider: "modelscope", baseURL: "https://api.example.com/v1", model: "model"},
		"key over HTTP":          {provider: "llama", apiKey: "secret", baseURL: "http://localhost:8080", model: "model"},
		"missing endpoint":       {provider: "ollama", model: "model"},
		"missing model":          {provider: "ollama", baseURL: "http://localhost:11434"},
	} {
		t.Run(name, func(t *testing.T) {
			client, err := NewClient(input.provider, input.apiKey, input.baseURL, input.model)
			if err == nil || client != nil {
				t.Fatalf("client=%#v err=%v", client, err)
			}
			if input.apiKey != "" && strings.Contains(err.Error(), input.apiKey) {
				t.Fatalf("key leaked in error: %v", err)
			}
		})
	}
}

func TestNewClientDoesNotInventCredentialsForLocalProviders(t *testing.T) {
	client, err := NewClient(" LLAMA ", "", "http://localhost:8080", " model ")
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := client.Provider.(*OpenAICompatibleClient)
	if !ok || provider.APIKey != "" || provider.Model != "model" {
		t.Fatalf("unexpected local provider: %#v", client.Provider)
	}
}

func TestDirectLLMConstructorsRejectCredentialsOverHTTPBeforeNetwork(t *testing.T) {
	for name, provider := range map[string]Provider{
		"modelscope":        NewModelScopeClient("secret", "http://localhost:1", "model"),
		"openai compatible": NewOpenAICompatibleClient("secret", "http://localhost:1", "model"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := provider.Generate(context.Background(), "unpublished prompt")
			if err == nil || !strings.Contains(err.Error(), "HTTPS") {
				t.Fatalf("insecure direct provider returned %v", err)
			}
		})
	}
}

func TestProviderHTTPClientDoesNotFollowRedirects(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	request, err := http.NewRequest(http.MethodPost, source.URL, strings.NewReader("unpublished text"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	response, err := newProviderHTTPClient().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTemporaryRedirect || targetCalls != 0 {
		t.Fatalf("status=%d target_calls=%d", response.StatusCode, targetCalls)
	}
}

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
	if transport.Proxy != nil {
		t.Fatal("provider transport must not inherit ambient proxy settings")
	}
	if !transport.DisableCompression {
		t.Fatal("provider transport must not negotiate or transparently decode compressed responses")
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

func TestValidateProviderContentTypeUsesStrictModeSpecificAllowlist(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		streaming bool
		wantError bool
	}{
		{name: "json", value: "application/json", wantError: false},
		{name: "json charset", value: "application/json; charset=utf-8", wantError: false},
		{name: "json charset case", value: "application/json; charset=UTF-8", wantError: false},
		{name: "vendor json", value: "application/problem+json", wantError: false},
		{name: "event stream", value: "text/event-stream", streaming: true, wantError: false},
		{name: "event stream charset", value: "text/event-stream; charset=utf-8", streaming: true, wantError: false},
		{name: "utf16 rejected", value: "application/json; charset=utf-16", wantError: true},
		{name: "unknown parameter rejected", value: "application/json; version=1", wantError: true},
		{name: "extra parameter rejected", value: "application/json; charset=utf-8; version=1", wantError: true},
		{name: "json rejected for stream", value: "application/json", streaming: true, wantError: true},
		{name: "event stream rejected for json", value: "text/event-stream", wantError: true},
		{name: "html", value: "text/html", wantError: true},
		{name: "missing", value: "", wantError: true},
		{name: "malformed", value: "application/json; charset", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &http.Response{Header: make(http.Header)}
			if tt.value != "" {
				response.Header.Set("Content-Type", tt.value)
			}
			err := validateProviderContentType(response, tt.streaming)
			if (err != nil) != tt.wantError {
				t.Fatalf("content type %q streaming=%v returned %v", tt.value, tt.streaming, err)
			}
		})
	}
}

func TestValidateProviderContentTypeRejectsAmbiguousOrEncodedResponses(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
	}{
		{name: "gzip", headers: http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"gzip"}}},
		{name: "identity", headers: http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"identity"}}},
		{name: "duplicate content type", headers: http.Header{"Content-Type": {"application/json", "text/event-stream"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateProviderContentType(&http.Response{Header: tt.headers}, false); err == nil {
				t.Fatal("ambiguous or encoded Provider response was accepted")
			}
		})
	}
}

func TestProviderHTTPErrorContainsOnlyStableMetadata(t *testing.T) {
	err := &ProviderHTTPError{StatusCode: http.StatusBadGateway, RetryAfter: time.Second}
	if !strings.Contains(err.Error(), "502") || strings.Contains(err.Error(), "RetryAfter") {
		t.Fatalf("provider error was not safely summarized: %v", err)
	}
	typeOfError := reflect.TypeOf(*err)
	for index := 0; index < typeOfError.NumField(); index++ {
		if typeOfError.Field(index).Type.Kind() == reflect.String {
			t.Fatalf("provider error must not retain response text: field=%s", typeOfError.Field(index).Name)
		}
	}
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

func TestParseRetryAfterIsUnambiguousAndBounded(t *testing.T) {
	now := time.Date(2026, time.October, 18, 7, 37, 0, 0, time.UTC)
	tests := []struct {
		name   string
		values []string
		want   time.Duration
	}{
		{name: "seconds", values: []string{"5"}, want: 5 * time.Second},
		{name: "seconds capped", values: []string{"4294967295"}, want: maxLLMRetryBackoff},
		{name: "overflow rejected", values: []string{"999999999999999999999999"}, want: 0},
		{name: "future date capped", values: []string{now.Add(time.Hour).Format(http.TimeFormat)}, want: maxLLMRetryBackoff},
		{name: "past date", values: []string{now.Add(-time.Second).Format(http.TimeFormat)}, want: 0},
		{name: "negative", values: []string{"-1"}, want: 0},
		{name: "invalid", values: []string{"private-value"}, want: 0},
		{name: "repeated", values: []string{"1", "2"}, want: 0},
		{name: "missing", values: nil, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.values, now); got != tt.want {
				t.Fatalf("parseRetryAfter(%v)=%s want=%s", tt.values, got, tt.want)
			}
		})
	}
}

func TestRetryBackoffIsBounded(t *testing.T) {
	if delay := retryBackoff(0, 0); delay != time.Second {
		t.Fatalf("initial retry delay = %s", delay)
	}
	if delay := retryBackoff(1000, 24*time.Hour); delay != maxLLMRetryBackoff {
		t.Fatalf("bounded retry delay = %s, want %s", delay, maxLLMRetryBackoff)
	}
	if delay := retryBackoff(1, 5*time.Second); delay != 5*time.Second {
		t.Fatalf("Retry-After delay = %s", delay)
	}
}

func TestGenerateWithRetryRejectsRetryCountsOutsideBoundBeforeProviderCall(t *testing.T) {
	provider := &countingProvider{}
	client := &Client{Provider: provider}
	for _, retries := range []int{-1, maxLLMRetries + 1, 1000000} {
		if _, err := client.GenerateWithRetry(context.Background(), "prompt", retries); err == nil {
			t.Fatalf("retry count %d was accepted", retries)
		}
	}
	if provider.calls != 0 {
		t.Fatalf("provider called %d times for invalid retry counts", provider.calls)
	}
}

type countingProvider struct {
	calls int
}

func (provider *countingProvider) Generate(context.Context, string, ...Option) (string, error) {
	provider.calls++
	return "", &ProviderHTTPError{StatusCode: http.StatusServiceUnavailable}
}

func (provider *countingProvider) GenerateJSON(context.Context, string, ...Option) (map[string]interface{}, error) {
	provider.calls++
	return nil, &ProviderHTTPError{StatusCode: http.StatusServiceUnavailable}
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

func TestValidateSingleChoiceRejectsAmbiguousResponses(t *testing.T) {
	zero := 0
	one := 1
	if err := validateSingleChoice(1, nil); err != nil {
		t.Fatalf("single unindexed choice rejected: %v", err)
	}
	if err := validateSingleChoice(1, &zero); err != nil {
		t.Fatalf("choice index zero rejected: %v", err)
	}
	for _, test := range []struct {
		count int
		index *int
	}{
		{count: 0},
		{count: 2},
		{count: 1, index: &one},
	} {
		if err := validateSingleChoice(test.count, test.index); err == nil {
			t.Fatalf("ambiguous choice accepted: count=%d index=%v", test.count, test.index)
		}
	}
}

func TestValidateFinishReasonRejectsIncompleteResponses(t *testing.T) {
	for _, reason := range []string{"length", " LENGTH ", "content_filter", "CONTENT_FILTER"} {
		if err := validateFinishReason(reason); err == nil {
			t.Fatalf("incomplete finish reason accepted: %q", reason)
		}
	}
	for _, reason := range []string{"", "stop", "tool_calls", "unknown-compatible-reason"} {
		if err := validateFinishReason(reason); err != nil {
			t.Fatalf("compatible finish reason rejected: %q err=%v", reason, err)
		}
	}
}

func TestRequireProviderTextRejectsEmptyOrWhitespaceOnlyContent(t *testing.T) {
	for _, content := range []string{"", "   \r\n\t", "\u3000"} {
		if result, err := requireProviderText(content); err == nil || result != "" {
			t.Fatalf("empty Provider content accepted: %q result=%q err=%v", content, result, err)
		}
	}
	const content = "  保留正文首尾空白  "
	if result, err := requireProviderText(content); err != nil || result != content {
		t.Fatalf("valid Provider content changed: %q err=%v", result, err)
	}
}

func TestDecodeOpenAIStreamRejectsWhitespaceAndNonNarrativeReasoning(t *testing.T) {
	whitespace := "data: {\"choices\":[{\"delta\":{\"content\":\"   \"}}]}\n" + "data: [DONE]\n"
	if _, err := decodeOpenAIStream(strings.NewReader(whitespace)); err == nil {
		t.Fatal("whitespace-only stream content accepted")
	}
	reasoning := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Thinking step\\n检查结果\"}}]}\n" + "data: [DONE]\n"
	if _, err := decodeOpenAIStream(strings.NewReader(reasoning)); err == nil {
		t.Fatal("reasoning that cleans to empty content was accepted")
	}
}

func TestParseJSONResponseAcceptsObjectWithFenceOrExplanation(t *testing.T) {
	tests := []struct {
		content string
		title   string
	}{
		{content: "```json\n{\"title\":\"章节\"}\n```", title: "章节"},
		{content: "以下是结果：\n{\"title\":\"章节\"}\n请查收。", title: "章节"},
		{content: "说明：{\"title\":\"包含 } 和 { 以及 \\\"引号\\\"\"} 后续文字也有 } 符号", title: "包含 } 和 { 以及 \"引号\""},
	}
	for _, test := range tests {
		result, err := parseJSONResponse(test.content)
		if err != nil {
			t.Fatalf("valid object rejected: %v", err)
		}
		if result["title"] != test.title {
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
		`{"title":"first","title":"second"}`,
		`{"title":"first"} {"title":"second"}`,
		strings.Repeat(`{"nested":`, maxProviderJSONDepth+1) + `0` + strings.Repeat(`}`, maxProviderJSONDepth+1),
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

func TestUnmarshalProviderJSONRejectsAmbiguousOrComplexDocuments(t *testing.T) {
	var destination map[string]interface{}
	invalid := map[string]string{
		"duplicate key":   `{"choices":[],"choices":[{}]}`,
		"top level array": `[]`,
		"trailing value":  `{} {}`,
		"too deep":        strings.Repeat(`{"x":`, maxProviderJSONDepth+1) + `0` + strings.Repeat(`}`, maxProviderJSONDepth+1),
	}
	for name, body := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := unmarshalProviderJSON([]byte(body), &destination); err == nil {
				t.Fatalf("invalid Provider JSON accepted: %s", body)
			}
		})
	}
	if err := unmarshalProviderJSON([]byte(` {"choices":[{"message":{"content":"中文🙂"}}]} `), &destination); err != nil {
		t.Fatalf("valid Provider JSON rejected: %v", err)
	}
}

func TestReadBoundedUTF8ResponseRejectsInvalidBytes(t *testing.T) {
	if body, err := readBoundedUTF8Response(bytes.NewReader([]byte{'{', 0xff, '}'}), 10); err == nil || body != nil {
		t.Fatalf("invalid UTF-8 accepted: %q err=%v", body, err)
	}
	body, err := readBoundedUTF8Response(strings.NewReader(`{"text":"中文🙂"}`), 64)
	if err != nil || string(body) != `{"text":"中文🙂"}` {
		t.Fatalf("valid UTF-8 rejected: %q err=%v", body, err)
	}
}

func TestDecodeOpenAIStreamRejectsInvalidUTF8(t *testing.T) {
	stream := append([]byte("data: {\"choices\":[{\"delta\":{\"content\":\""), 0xff)
	stream = append(stream, []byte("\"}}]}\n")...)
	if _, err := decodeOpenAIStream(bytes.NewReader(stream)); err == nil {
		t.Fatal("invalid UTF-8 stream accepted")
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

func TestDecodeOpenAIStreamRejectsAmbiguousSSEFields(t *testing.T) {
	validJSON := `{"choices":[{"delta":{"content":"text"}}]}`
	for name, stream := range map[string]string{
		"bare JSON":    validJSON + "\n",
		"event field":  "event: message\n" + "data: " + validJSON + "\n",
		"id field":     "id: private-id\n" + "data: " + validJSON + "\n",
		"retry field":  "retry: 1000\n" + "data: " + validJSON + "\n",
		"empty data":   "data:\n",
		"spaced field": " data: " + validJSON + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeOpenAIStream(strings.NewReader(stream)); err == nil {
				t.Fatalf("ambiguous SSE accepted: %q", stream)
			}
		})
	}
}

func TestDecodeOpenAIStreamRejectsAmbiguousChoices(t *testing.T) {
	streams := []string{
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"甲\"}},{\"index\":1,\"delta\":{\"content\":\"乙\"}}]}\n\ndata: [DONE]\n",
		"data: {\"choices\":[{\"index\":1,\"delta\":{\"content\":\"错误索引\"}}]}\n\ndata: [DONE]\n",
	}
	for _, stream := range streams {
		if result, err := decodeOpenAIStream(strings.NewReader(stream)); err == nil || result != "" {
			t.Fatalf("ambiguous stream choices accepted: result=%q err=%v", result, err)
		}
	}
}

func TestDecodeOpenAIStreamRejectsIncompleteFinishReason(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"被截断正文\"},\"finish_reason\":\"length\"}]}\n\n" +
		"data: [DONE]\n\n"
	result, err := decodeOpenAIStream(strings.NewReader(stream))
	if err == nil || result != "" {
		t.Fatalf("length-truncated stream accepted: result=%q err=%v", result, err)
	}
}

func TestDecodeOpenAIStreamRejectsDataAfterDone(t *testing.T) {
	validEvent := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"正文\"},\"finish_reason\":\"stop\"}]}\n\n"
	for name, suffix := range map[string]string{
		"second event":  "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"隐藏尾部\"}}]}\n",
		"second done":   "data: [DONE]\n",
		"control field": "event: hidden\n",
	} {
		t.Run(name, func(t *testing.T) {
			stream := validEvent + "data: [DONE]\n\n" + suffix
			if result, err := decodeOpenAIStream(strings.NewReader(stream)); err == nil || result != "" {
				t.Fatalf("post-completion data accepted: result=%q err=%v", result, err)
			}
		})
	}
	allowed := validEvent + "data: [DONE]\n\n: keepalive after done\n\n"
	if result, err := decodeOpenAIStream(strings.NewReader(allowed)); err != nil || result != "正文" {
		t.Fatalf("post-completion whitespace/comment rejected: result=%q err=%v", result, err)
	}
}

func TestDecodeOpenAIStreamRejectsPartialContentWithoutDone(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"未完成正文\"}}]}\n\n"
	result, err := decodeOpenAIStream(strings.NewReader(stream))
	if err == nil || result != "" {
		t.Fatalf("partial stream accepted: result=%q err=%v", result, err)
	}
}

func TestDecodeOpenAIStreamRejectsExcessiveRawInput(t *testing.T) {
	comment := ":" + strings.Repeat("x", 512<<10) + "\n"
	stream := strings.Repeat(comment, maxStreamInputBytes/(512<<10)+1)
	if result, err := decodeOpenAIStream(strings.NewReader(stream)); err == nil || result != "" {
		t.Fatalf("excessive raw SSE input accepted: result=%q err=%v", result, err)
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	client := NewModelScopeClient("secret", server.URL, "test-model")
	client.Client = server.Client()
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
