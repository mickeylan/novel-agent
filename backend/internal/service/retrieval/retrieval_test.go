package retrieval

import (
	"context"
	"errors"
	"testing"
)

func TestSearchesRejectCanceledContextBeforeEmbeddingOrDatabaseAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := &RetrievalService{}
	for name, search := range map[string]func() error{
		"vector":  func() error { _, err := service.VectorSearch(ctx, "query", nil, 5); return err },
		"hybrid":  func() error { _, err := service.HybridSearch(ctx, "query", nil, 5); return err },
		"keyword": func() error { _, err := service.KeywordSearch(ctx, "query", nil, 5); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := search(); !errors.Is(err, context.Canceled) {
				t.Fatalf("search returned %v, want context.Canceled", err)
			}
		})
	}
}

func TestNormalizeGenresReturnsNilForEmptyFilter(t *testing.T) {
	if result := normalizeGenres([]string{"", "  "}); result != nil {
		t.Fatalf("expected nil, got %#v", result)
	}
	result := normalizeGenres([]string{" 玄幻 ", "科幻"})
	if len(result) != 2 || result[0] != "玄幻" {
		t.Fatalf("unexpected genres: %#v", result)
	}
}

func TestValidateEmbeddingDimension(t *testing.T) {
	service := &RetrievalService{expectedDimension: 3}
	if err := service.validateEmbedding([]float32{1, 2}); err == nil {
		t.Fatal("expected dimension mismatch")
	}
	if err := service.validateEmbedding([]float32{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if err := service.validateEmbedding(nil); err == nil {
		t.Fatal("expected empty embedding error")
	}
}
