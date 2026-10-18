package repository

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestValidateStoredSearchResultRejectsUnsafeResponseValues(t *testing.T) {
	for _, similarity := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := validateStoredSearchResult(similarity, []byte(`{}`)); err == nil {
			t.Fatalf("non-finite similarity accepted: %v", similarity)
		}
	}
	for _, metadata := range [][]byte{[]byte(`{"broken":`), []byte(`[]`), []byte(`"text"`)} {
		if err := validateStoredSearchResult(0.5, metadata); err == nil {
			t.Fatalf("invalid metadata accepted: %s", metadata)
		}
	}
	for _, metadata := range [][]byte{nil, []byte(`null`), []byte(`{"source":"book"}`)} {
		if err := validateStoredSearchResult(0.5, metadata); err != nil {
			t.Fatalf("valid stored result rejected: metadata=%s err=%v", metadata, err)
		}
	}
}

func TestBookChunkRepositoryRejectsCanceledContextBeforeDatabaseAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewBookChunkRepository().SearchSimilar(ctx, []float32{1}, nil, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SearchSimilar returned %v, want context.Canceled", err)
	}
}
