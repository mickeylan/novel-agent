package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"novel-agent/internal/models"
)

func TestProjectRepositoryRejectsCanceledContextBeforeDatabaseAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := NewProjectRepository()

	if _, err := repo.Create(ctx, &models.StoryProject{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create returned %v, want context.Canceled", err)
	}
	if _, err := repo.GetByID(ctx, "project-id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByID returned %v, want context.Canceled", err)
	}
	if _, err := repo.ListAll(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListAll returned %v, want context.Canceled", err)
	}
	updates := map[string]interface{}{"title": "unchanged"}
	if _, err := repo.UpdateIfUnchanged(ctx, "project-id", time.Now(), updates); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdateIfUnchanged returned %v, want context.Canceled", err)
	}
	if _, exists := updates["updated_at"]; exists {
		t.Fatal("canceled update mutated the update map before database access")
	}
}
