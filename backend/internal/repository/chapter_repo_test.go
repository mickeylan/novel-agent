package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"novel-agent/internal/models"
)

func TestChapterRepositoryRejectsCanceledContextBeforeDatabaseAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := NewChapterRepository()

	if _, err := repo.Create(ctx, &models.Chapter{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create returned %v, want context.Canceled", err)
	}
	if _, err := repo.GetByID(ctx, "chapter-id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByID returned %v, want context.Canceled", err)
	}
	if _, err := repo.GetByNumber(ctx, "project-id", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByNumber returned %v, want context.Canceled", err)
	}
	if _, err := repo.GetByProject(ctx, "project-id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByProject returned %v, want context.Canceled", err)
	}
	updates := map[string]interface{}{"content": "unchanged"}
	if _, err := repo.UpdateIfUnchanged(ctx, "project-id", 1, time.Now(), updates); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdateIfUnchanged returned %v, want context.Canceled", err)
	}
	if _, exists := updates["updated_at"]; exists {
		t.Fatal("canceled update mutated the update map before database access")
	}
}
