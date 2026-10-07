package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"novel-agent/internal/models"
)

func TestCharacterRepositoryRejectsCanceledContextBeforeDatabaseAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := NewCharacterRepository()

	originalRelationships := json.RawMessage(`{"ally":"secret"}`)
	character := &models.Character{Relationships: append(json.RawMessage(nil), originalRelationships...)}
	if _, err := repo.Create(ctx, character); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create returned %v, want context.Canceled", err)
	}
	if !bytes.Equal(character.Relationships, originalRelationships) {
		t.Fatal("canceled create transformed relationships before database access")
	}
	if _, err := repo.GetByID(ctx, "character-id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByID returned %v, want context.Canceled", err)
	}
	if _, err := repo.GetByProject(ctx, "project-id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByProject returned %v, want context.Canceled", err)
	}
	updates := map[string]interface{}{"relationships": map[string]string{"ally": "secret"}}
	if _, err := repo.UpdateIfUnchanged(ctx, "project-id", "character-id", time.Now(), updates); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdateIfUnchanged returned %v, want context.Canceled", err)
	}
	if _, ok := updates["relationships"].(map[string]string); !ok {
		t.Fatal("canceled update transformed relationships before database access")
	}
	if _, exists := updates["updated_at"]; exists {
		t.Fatal("canceled update added updated_at before database access")
	}
}
