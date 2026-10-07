package repository

import (
	"context"
	"errors"
	"testing"
)

func TestBookChunkRepositoryRejectsCanceledContextBeforeDatabaseAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewBookChunkRepository().SearchSimilar(ctx, []float32{1}, nil, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SearchSimilar returned %v, want context.Canceled", err)
	}
}
