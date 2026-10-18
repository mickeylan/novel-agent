package retrieval

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestSearchesRejectMissingDatabaseWithoutPanic(t *testing.T) {
	service := NewRetrievalService(nil, 1024)
	for name, search := range map[string]func() error{
		"vector":  func() error { _, err := service.VectorSearch(context.Background(), "query", nil, 5); return err },
		"hybrid":  func() error { _, err := service.HybridSearch(context.Background(), "query", nil, 5); return err },
		"keyword": func() error { _, err := service.KeywordSearch(context.Background(), "query", nil, 5); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := search(); err == nil || err.Error() != "database is not initialized" {
				t.Fatalf("unexpected missing database error: %v", err)
			}
		})
	}
}

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

func TestParseMetadataRejectsCorruptOrNonObjectJSON(t *testing.T) {
	for _, data := range [][]byte{[]byte(`{"broken":`), []byte(`[]`), []byte(`"text"`)} {
		if value, err := parseMetadata(data); err == nil || value != nil {
			t.Fatalf("invalid metadata accepted: data=%s value=%#v err=%v", data, value, err)
		}
	}
	value, err := parseMetadata([]byte(`{"source":"book"}`))
	if err != nil || value["source"] != "book" {
		t.Fatalf("valid metadata rejected: value=%#v err=%v", value, err)
	}
}

func TestValidateSearchSimilarityRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := validateSearchSimilarity(value); err == nil {
			t.Fatalf("non-finite similarity accepted: %v", value)
		}
	}
	if err := validateSearchSimilarity(0.75); err != nil {
		t.Fatalf("finite similarity rejected: %v", err)
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
