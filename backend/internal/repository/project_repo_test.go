package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"novel-agent/internal/models"
	"novel-agent/pkg/database"
)

func TestCopyUpdatesDoesNotAliasCallerMap(t *testing.T) {
	originalRelationships := map[string]string{"ally": "trusted"}
	original := map[string]interface{}{"title": "original", "relationships": originalRelationships}
	copied := copyUpdates(original)
	copied["title"] = "changed"
	copied["updated_at"] = "generated"
	copied["relationships"] = []byte(`{"ally":"trusted"}`)
	if original["title"] != "original" {
		t.Fatalf("copy mutation changed caller title: %#v", original)
	}
	if _, exists := original["updated_at"]; exists {
		t.Fatal("copy mutation added generated field to caller map")
	}
	relationships, ok := original["relationships"].(map[string]string)
	if !ok || relationships["ally"] != "trusted" {
		t.Fatalf("copy mutation replaced caller relationships: %#v", original)
	}
	if copiedNil := copyUpdates(nil); copiedNil == nil {
		t.Fatal("copyUpdates(nil) returned nil map")
	}
}

func TestProjectRepositoryRejectsMissingDatabaseWithoutMutatingUpdates(t *testing.T) {
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = originalDB })

	repo := NewProjectRepository()
	if _, err := repo.GetByID(context.Background(), "project-id"); err == nil || err.Error() != "database is not initialized" {
		t.Fatalf("unexpected missing database error: %v", err)
	}
	updates := map[string]interface{}{"title": "unchanged"}
	if _, err := repo.UpdateIfUnchanged(context.Background(), "project-id", time.Now(), updates); err == nil || err.Error() != "database is not initialized" {
		t.Fatalf("unexpected missing database update error: %v", err)
	}
	if _, exists := updates["updated_at"]; exists {
		t.Fatal("missing database update mutated the update map")
	}
}

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
