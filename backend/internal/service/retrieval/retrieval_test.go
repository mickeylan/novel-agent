package retrieval

import "testing"

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
