package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateEmbeddingInputsBoundsBatchAndContent(t *testing.T) {
	if err := validateEmbeddingInputs(nil); err == nil {
		t.Fatal("expected empty batch error")
	}
	if err := validateEmbeddingInputs([]string{"   "}); err == nil {
		t.Fatal("expected blank input error")
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
			embedder := NewOpenAICompatibleEmbedder(server.URL, "test")
			if _, err := embedder.EncodeBatch(context.Background(), []string{"one", "two"}); err == nil {
				t.Fatal("expected invalid embedding response error")
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
	embedder := NewOpenAICompatibleEmbedder(server.URL+"/v1", "test")
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
