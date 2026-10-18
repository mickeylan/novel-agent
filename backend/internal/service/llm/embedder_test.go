package llm

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticEmbedder struct {
	vectors [][]float32
}

func (s staticEmbedder) Encode(context.Context, string) ([]float32, error) {
	return s.vectors[0], nil
}

func (s staticEmbedder) EncodeBatch(context.Context, []string) ([][]float32, error) {
	return s.vectors, nil
}

func TestDirectEmbeddersRejectPublicPlaintextBeforeNetwork(t *testing.T) {
	called := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called++
		return nil, errors.New("network must not be reached")
	})
	ollama := NewOllamaEmbedder("http://8.8.8.8:11434", "model")
	ollama.Client = &http.Client{Transport: transport}
	openAI := NewOpenAICompatibleEmbedder("http://embedding.example.com/v1", "model", "")
	openAI.Client = &http.Client{Transport: transport}
	for _, embedder := range []Embedder{ollama, openAI} {
		if _, err := embedder.EncodeBatch(context.Background(), []string{"text"}); err == nil {
			t.Fatalf("%T accepted public plaintext endpoint", embedder)
		}
	}
	if called != 0 {
		t.Fatalf("public plaintext Embedding calls reached network %d times", called)
	}
}

func TestDirectEmbeddersRejectUnsafeURLBeforeNetwork(t *testing.T) {
	called := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called++
		return nil, errors.New("network must not be reached")
	})
	ollama := NewOllamaEmbedder("file:///private/model", "model")
	ollama.Client = &http.Client{Transport: transport}
	openAI := NewOpenAICompatibleEmbedder("http://localhost:8081/v1?private=value", "model", "")
	openAI.Client = &http.Client{Transport: transport}
	for _, embedder := range []Embedder{ollama, openAI} {
		if _, err := embedder.EncodeBatch(context.Background(), []string{"text"}); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("%T returned unsafe URL error: %v", embedder, err)
		}
	}
	if called != 0 {
		t.Fatalf("unsafe Embedding endpoints reached network %d times", called)
	}
}

func TestEmbeddersHonorCanceledContextBeforeValidationOrNetwork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called++
		return nil, context.Canceled
	})
	ollama := NewOllamaEmbedder("http://localhost:11434", "model")
	ollama.Client = &http.Client{Transport: transport}
	openAI := NewOpenAICompatibleEmbedder("http://localhost:8081", "model", "")
	openAI.Client = &http.Client{Transport: transport}
	for _, embedder := range []Embedder{ollama, openAI} {
		if _, err := embedder.EncodeBatch(ctx, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("%T returned %v, want context.Canceled", embedder, err)
		}
	}
	if called != 0 {
		t.Fatalf("canceled embedding calls reached network %d times", called)
	}
}

func TestEmbedderClientEnforcesConfiguredDimension(t *testing.T) {
	client := &EmbedderClient{Embedder: staticEmbedder{vectors: [][]float32{{1, 2}, {3, 4}}}, ExpectedDimension: 2}
	if _, err := client.Encode(context.Background(), "one"); err != nil {
		t.Fatalf("matching single dimension rejected: %v", err)
	}
	if _, err := client.EncodeBatch(context.Background(), []string{"one", "two"}); err != nil {
		t.Fatalf("matching batch dimensions rejected: %v", err)
	}
	client.ExpectedDimension = 3
	if _, err := client.Encode(context.Background(), "one"); err == nil {
		t.Fatal("mismatched single dimension accepted")
	}
	if _, err := client.EncodeBatch(context.Background(), []string{"one", "two"}); err == nil {
		t.Fatal("mismatched batch dimension accepted")
	}
}

func TestNewEmbedderClientValidatesOptionalExpectedDimension(t *testing.T) {
	for _, dimensions := range [][]int{{0}, {65537}, {2, 2}} {
		if client, err := NewEmbedderClient("ollama", "http://localhost:11434", "model", "", dimensions...); err == nil || client != nil {
			t.Fatalf("invalid expected dimensions accepted: %v", dimensions)
		}
	}
	client, err := NewEmbedderClient("ollama", "http://localhost:11434", "model", "", 1024)
	if err != nil || client.ExpectedDimension != 1024 {
		t.Fatalf("valid expected dimension rejected: client=%#v err=%v", client, err)
	}
}

func TestEmbedderConstructorsUseContextForTotalTimeout(t *testing.T) {
	clients := []*http.Client{
		NewOllamaEmbedder("http://localhost:11434", "model").Client,
		NewOpenAICompatibleEmbedder("http://localhost:8081", "model", "").Client,
	}
	for _, client := range clients {
		if client.Timeout != 0 {
			t.Fatalf("embedder installed conflicting total timeout: %s", client.Timeout)
		}
		transport, ok := client.Transport.(*http.Transport)
		if !ok || transport.TLSHandshakeTimeout == 0 || transport.ResponseHeaderTimeout == 0 || transport.IdleConnTimeout == 0 {
			t.Fatalf("embedder transport lacks bounded phases: %#v", client.Transport)
		}
	}
}

func TestNewEmbedderClientRejectsUnknownProvider(t *testing.T) {
	client, err := NewEmbedderClient("typo-provider", "http://localhost:8081", "model", "")
	if err == nil || client != nil {
		t.Fatalf("unknown provider returned client=%#v err=%v", client, err)
	}
	if strings.Contains(err.Error(), "typo-provider") {
		t.Fatalf("provider value leaked in error: %v", err)
	}
}

func TestNewEmbedderClientAcceptsSupportedProviders(t *testing.T) {
	for _, testCase := range []struct {
		provider string
		baseURL  string
		apiKey   string
	}{
		{provider: "ollama", baseURL: "http://localhost:8081"},
		{provider: "modelscope", baseURL: "https://embedding.example.com/v1", apiKey: "secret"},
		{provider: "openai", baseURL: "http://localhost:8081"},
		{provider: "sglang", baseURL: "http://localhost:8081"},
		{provider: "llama", baseURL: "http://localhost:8081"},
		{provider: " OPENAI ", baseURL: "http://localhost:8081"},
	} {
		client, err := NewEmbedderClient(testCase.provider, testCase.baseURL, "model", testCase.apiKey)
		if err != nil || client == nil || client.Embedder == nil {
			t.Fatalf("provider %q returned client=%#v err=%v", testCase.provider, client, err)
		}
	}
}

func TestNewEmbedderClientRequiresKeyForNonLocalOpenAIEndpoint(t *testing.T) {
	client, err := NewEmbedderClient("openai", "https://embedding.example.com/v1", "model", "")
	if err == nil || client != nil {
		t.Fatalf("non-local OpenAI endpoint accepted without credentials")
	}
	client, err = NewEmbedderClient("openai", "https://embedding.example.com/v1", "model", "secret")
	if err != nil || client == nil {
		t.Fatalf("credentialed HTTPS OpenAI endpoint rejected: %v", err)
	}
}

func TestNewEmbedderClientRejectsCredentialsOverHTTP(t *testing.T) {
	for _, provider := range []string{"modelscope", "openai"} {
		client, err := NewEmbedderClient(provider, "http://embedding.example.com/v1", "model", "secret")
		if err == nil || client != nil {
			t.Fatalf("provider %q accepted credentials over HTTP", provider)
		}
	}
}

func TestConvertEmbeddingRejectsNonFiniteAndFloat32Overflow(t *testing.T) {
	for name, values := range map[string][]float64{
		"NaN":               {math.NaN()},
		"positive infinity": {math.Inf(1)},
		"negative infinity": {math.Inf(-1)},
		"float32 overflow":  {math.MaxFloat32 * 2},
	} {
		t.Run(name, func(t *testing.T) {
			if result, err := convertEmbedding(values); err == nil || result != nil {
				t.Fatalf("values=%v result=%v err=%v", values, result, err)
			}
		})
	}
	result, err := convertEmbedding([]float64{0, math.MaxFloat32, -math.MaxFloat32})
	if err != nil || len(result) != 3 {
		t.Fatalf("finite boundary vector rejected: result=%v err=%v", result, err)
	}
}

func TestValidateEmbeddingInputsBoundsBatchAndContent(t *testing.T) {
	if err := validateEmbeddingInputs(nil); err == nil {
		t.Fatal("expected empty batch error")
	}
	if err := validateEmbeddingInputs([]string{"   \t\r\n\u3000"}); err == nil {
		t.Fatal("expected blank Unicode input error")
	}
	if err := validateEmbeddingInputs([]string{string([]byte{0xff})}); err == nil {
		t.Fatal("expected invalid UTF-8 input error")
	}
	if err := validateEmbeddingInputs([]string{strings.Repeat("文", maxEmbeddingInputRunes)}); err != nil {
		t.Fatalf("valid Unicode character boundary rejected: %v", err)
	}
	tooMany := make([]string, maxEmbeddingBatchSize+1)
	for index := range tooMany {
		tooMany[index] = "x"
	}
	if err := validateEmbeddingInputs(tooMany); err == nil {
		t.Fatal("expected oversized batch error")
	}
	if err := validateEmbeddingInputs([]string{strings.Repeat("文", maxEmbeddingInputRunes+1)}); err == nil {
		t.Fatal("expected oversized content error")
	}
}

func TestEmbeddingProviderErrorDoesNotExposeResponseBody(t *testing.T) {
	secretLikeBody := "credential-shaped-private-embedding-body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(secretLikeBody))
	}))
	defer server.Close()
	embedder := NewOpenAICompatibleEmbedder(server.URL, "test", "")
	_, err := embedder.EncodeBatch(context.Background(), []string{"one"})
	if err == nil || strings.Contains(err.Error(), secretLikeBody) || !strings.Contains(err.Error(), "502") {
		t.Fatalf("embedding provider error was not safely summarized: %v", err)
	}
}

func TestOpenAIEmbedderRejectsMismatchedCountAndEmptyVector(t *testing.T) {
	responses := []string{
		`{"data":[{"embedding":[1,2]}]}`,
		`{"data":[{"embedding":[]},{"embedding":[1,2]}]}`,
	}
	for _, response := range responses {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			embedder := NewOpenAICompatibleEmbedder(server.URL, "test", "")
			if _, err := embedder.EncodeBatch(context.Background(), []string{"one", "two"}); err == nil {
				t.Fatal("expected invalid embedding response error")
			}
		})
	}
}

func TestDirectEmbedderConstructorRejectsCredentialsOverHTTPBeforeNetwork(t *testing.T) {
	embedder := NewOpenAICompatibleEmbedder("http://localhost:1", "model", "secret")
	if _, err := embedder.EncodeBatch(context.Background(), []string{"unpublished text"}); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("insecure direct embedder returned %v", err)
	}
}

func TestOpenAIEmbedderSendsConfiguredBearerKeyOnly(t *testing.T) {
	var authorization string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[1,2]}]}`))
	}))
	defer server.Close()
	embedder := NewOpenAICompatibleEmbedder(server.URL, "test", "embedding-secret")
	embedder.Client = server.Client()
	if _, err := embedder.EncodeBatch(context.Background(), []string{"one"}); err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer embedding-secret" {
		t.Fatalf("Authorization = %q", authorization)
	}
}

func TestOpenAIEmbedderReordersStandardIndexedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}]}`))
	}))
	defer server.Close()
	embedder := NewOpenAICompatibleEmbedder(server.URL, "test", "")
	result, err := embedder.EncodeBatch(context.Background(), []string{"甲", "乙"})
	if err != nil {
		t.Fatal(err)
	}
	if result[0][0] != 1 || result[1][0] != 3 {
		t.Fatalf("indexed embeddings were not restored to input order: %#v", result)
	}
}

func TestOpenAIEmbedderRejectsInvalidIndices(t *testing.T) {
	responses := []string{
		`{"data":[{"index":0,"embedding":[1,2]},{"embedding":[3,4]}]}`,
		`{"data":[{"index":0,"embedding":[1,2]},{"index":0,"embedding":[3,4]}]}`,
		`{"data":[{"index":0,"embedding":[1,2]},{"index":2,"embedding":[3,4]}]}`,
		`{"data":[{"index":-1,"embedding":[1,2]},{"index":1,"embedding":[3,4]}]}`,
	}
	for _, response := range responses {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			embedder := NewOpenAICompatibleEmbedder(server.URL, "test", "")
			if _, err := embedder.EncodeBatch(context.Background(), []string{"甲", "乙"}); err == nil {
				t.Fatal("invalid embedding indices accepted")
			}
		})
	}
}

func TestOpenAIEmbedderAcceptsMatchingVectors(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[1,2]},{"embedding":[3,4]}]}`))
	}))
	defer server.Close()
	embedder := NewOpenAICompatibleEmbedder(server.URL+"/v1", "test", "")
	result, err := embedder.EncodeBatch(context.Background(), []string{"甲", "乙"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || len(result[0]) != 2 || result[1][1] != 4 {
		t.Fatalf("unexpected embeddings: %#v", result)
	}
	if path != "/v1/embeddings" {
		t.Fatalf("unexpected embedding request path: %q", path)
	}
}
