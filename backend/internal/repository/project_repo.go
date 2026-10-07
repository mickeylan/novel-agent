package repository

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"novel-agent/internal/models"
	"novel-agent/pkg/database"
)

type ProjectRepository struct{}

func NewProjectRepository() *ProjectRepository {
	return &ProjectRepository{}
}

func (r *ProjectRepository) Create(ctx context.Context, project *models.StoryProject) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result := database.DB.WithContext(ctx).Create(project)
	return project.ID, result.Error
}

func (r *ProjectRepository) GetByID(ctx context.Context, projectID string) (*models.StoryProject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var project models.StoryProject
	result := database.DB.WithContext(ctx).First(&project, "id = ?", projectID)
	if result.Error != nil {
		return nil, result.Error
	}
	return &project, nil
}

func (r *ProjectRepository) ListAll(ctx context.Context) ([]models.StoryProject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var projects []models.StoryProject
	result := database.DB.WithContext(ctx).Order("created_at DESC").Find(&projects)
	return projects, result.Error
}

func (r *ProjectRepository) UpdateIfUnchanged(ctx context.Context, projectID string, expectedUpdatedAt time.Time, updates map[string]interface{}) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	updates["updated_at"] = gorm.Expr("clock_timestamp()")
	result := database.DB.WithContext(ctx).Model(&models.StoryProject{}).
		Where("id = ? AND updated_at = ?", projectID, expectedUpdatedAt).
		Updates(updates)
	return result.RowsAffected, result.Error
}

// ========== Chapter Repository ==========

type ChapterRepository struct{}

func NewChapterRepository() *ChapterRepository {
	return &ChapterRepository{}
}

func (r *ChapterRepository) Create(ctx context.Context, chapter *models.Chapter) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result := database.DB.WithContext(ctx).Create(chapter)
	return chapter.ID, result.Error
}

func (r *ChapterRepository) GetByID(ctx context.Context, chapterID string) (*models.Chapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var chapter models.Chapter
	result := database.DB.WithContext(ctx).First(&chapter, "id = ?", chapterID)
	if result.Error != nil {
		return nil, result.Error
	}
	return &chapter, nil
}

func (r *ChapterRepository) GetByNumber(ctx context.Context, projectID string, chapterNumber int) (*models.Chapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var chapter models.Chapter
	result := database.DB.WithContext(ctx).Where("project_id = ? AND chapter_number = ?", projectID, chapterNumber).First(&chapter)
	if result.Error != nil {
		return nil, result.Error
	}
	return &chapter, nil
}

func (r *ChapterRepository) GetByProject(ctx context.Context, projectID string) ([]models.Chapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var chapters []models.Chapter
	result := database.DB.WithContext(ctx).Where("project_id = ?", projectID).Order("chapter_number ASC").Find(&chapters)
	return chapters, result.Error
}

func (r *ChapterRepository) UpdateIfUnchanged(ctx context.Context, projectID string, chapterNumber int, expectedUpdatedAt time.Time, updates map[string]interface{}) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	updates["updated_at"] = gorm.Expr("clock_timestamp()")
	result := database.DB.WithContext(ctx).Model(&models.Chapter{}).
		Where("project_id = ? AND chapter_number = ? AND updated_at = ?", projectID, chapterNumber, expectedUpdatedAt).
		Updates(updates)
	return result.RowsAffected, result.Error
}

// ========== Character Repository ==========

type CharacterRepository struct{}

func NewCharacterRepository() *CharacterRepository {
	return &CharacterRepository{}
}

func (r *CharacterRepository) Create(ctx context.Context, character *models.Character) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if character.Relationships != nil {
		data, _ := json.Marshal(character.Relationships)
		character.Relationships = data
	}
	result := database.DB.WithContext(ctx).Create(character)
	return character.ID, result.Error
}

func (r *CharacterRepository) GetByID(ctx context.Context, characterID string) (*models.Character, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var character models.Character
	result := database.DB.WithContext(ctx).First(&character, "id = ?", characterID)
	if result.Error != nil {
		return nil, result.Error
	}
	return &character, nil
}

func (r *CharacterRepository) GetByProject(ctx context.Context, projectID string) ([]models.Character, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var characters []models.Character
	result := database.DB.WithContext(ctx).Where("project_id = ?", projectID).Order("name ASC").Find(&characters)
	return characters, result.Error
}

func (r *CharacterRepository) UpdateIfUnchanged(ctx context.Context, projectID, characterID string, expectedUpdatedAt time.Time, updates map[string]interface{}) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if rel, ok := updates["relationships"]; ok {
		if relMap, ok := rel.(map[string]string); ok {
			data, _ := json.Marshal(relMap)
			updates["relationships"] = data
		}
	}
	updates["updated_at"] = gorm.Expr("clock_timestamp()")
	result := database.DB.WithContext(ctx).Model(&models.Character{}).
		Where("project_id = ? AND id = ? AND updated_at = ?", projectID, characterID, expectedUpdatedAt).
		Updates(updates)
	return result.RowsAffected, result.Error
}

// ========== BookChunk Repository ==========

type BookChunkRepository struct{}

func NewBookChunkRepository() *BookChunkRepository {
	return &BookChunkRepository{}
}

func (r *BookChunkRepository) SearchSimilar(ctx context.Context, queryEmbedding []float32, filterGenre []string, limit int) ([]models.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var results []models.SearchResult

	query := `
		SELECT 
			c.id as chunk_id,
			c.content,
			b.title as book_title,
			b.id as book_id,
			1 - (c.embedding <=> ?::vector) as similarity,
			c.metadata
		FROM book_chunks c
		JOIN books b ON c.book_id = b.id
		WHERE (?::text[] IS NULL OR b.genre && ?)
		ORDER BY c.embedding <=> ?::vector
		LIMIT ?
	`

	rows, err := database.DB.WithContext(ctx).Raw(query, queryEmbedding, filterGenre, filterGenre, queryEmbedding, limit).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sr models.SearchResult
		var metadata []byte
		if err := rows.Scan(&sr.ChunkID, &sr.Content, &sr.BookTitle, &sr.BookID, &sr.Similarity, &metadata); err != nil {
			return nil, err
		}
		sr.Metadata = metadata
		results = append(results, sr)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
