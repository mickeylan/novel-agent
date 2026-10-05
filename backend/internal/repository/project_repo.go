package repository

import (
	"encoding/json"

	"gorm.io/gorm"

	"novel-agent/internal/models"
	"novel-agent/pkg/database"
)

type ProjectRepository struct{}

func NewProjectRepository() *ProjectRepository {
	return &ProjectRepository{}
}

func (r *ProjectRepository) Create(project *models.StoryProject) (string, error) {
	result := database.DB.Create(project)
	return project.ID, result.Error
}

func (r *ProjectRepository) GetByID(projectID string) (*models.StoryProject, error) {
	var project models.StoryProject
	result := database.DB.First(&project, "id = ?", projectID)
	if result.Error != nil {
		return nil, result.Error
	}
	return &project, nil
}

func (r *ProjectRepository) ListAll() ([]models.StoryProject, error) {
	var projects []models.StoryProject
	result := database.DB.Order("created_at DESC").Find(&projects)
	return projects, result.Error
}

func (r *ProjectRepository) Update(projectID string, updates map[string]interface{}) error {
	return database.DB.Model(&models.StoryProject{}).Where("id = ?", projectID).Updates(updates).Error
}

func (r *ProjectRepository) Delete(projectID string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		// Delete chapters first
		if err := tx.Where("project_id = ?", projectID).Delete(&models.Chapter{}).Error; err != nil {
			return err
		}
		// Delete characters
		if err := tx.Where("project_id = ?", projectID).Delete(&models.Character{}).Error; err != nil {
			return err
		}
		// Delete project
		if err := tx.Where("id = ?", projectID).Delete(&models.StoryProject{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// ========== Chapter Repository ==========

type ChapterRepository struct{}

func NewChapterRepository() *ChapterRepository {
	return &ChapterRepository{}
}

func (r *ChapterRepository) Create(chapter *models.Chapter) (string, error) {
	result := database.DB.Create(chapter)
	return chapter.ID, result.Error
}

func (r *ChapterRepository) GetByID(chapterID string) (*models.Chapter, error) {
	var chapter models.Chapter
	result := database.DB.First(&chapter, "id = ?", chapterID)
	if result.Error != nil {
		return nil, result.Error
	}
	return &chapter, nil
}

func (r *ChapterRepository) GetByNumber(projectID string, chapterNumber int) (*models.Chapter, error) {
	var chapter models.Chapter
	result := database.DB.Where("project_id = ? AND chapter_number = ?", projectID, chapterNumber).First(&chapter)
	if result.Error != nil {
		return nil, result.Error
	}
	return &chapter, nil
}

func (r *ChapterRepository) GetByProject(projectID string) ([]models.Chapter, error) {
	var chapters []models.Chapter
	result := database.DB.Where("project_id = ?", projectID).Order("chapter_number ASC").Find(&chapters)
	return chapters, result.Error
}

func (r *ChapterRepository) Update(projectID string, chapterNumber int, updates map[string]interface{}) error {
	return database.DB.Model(&models.Chapter{}).Where("project_id = ? AND chapter_number = ?", projectID, chapterNumber).Updates(updates).Error
}

func (r *ChapterRepository) Delete(projectID string, chapterNumber int) error {
	return database.DB.Where("project_id = ? AND chapter_number = ?", projectID, chapterNumber).Delete(&models.Chapter{}).Error
}

// ========== Character Repository ==========

type CharacterRepository struct{}

func NewCharacterRepository() *CharacterRepository {
	return &CharacterRepository{}
}

func (r *CharacterRepository) Create(character *models.Character) (string, error) {
	if character.Relationships != nil {
		data, _ := json.Marshal(character.Relationships)
		character.Relationships = data
	}
	result := database.DB.Create(character)
	return character.ID, result.Error
}

func (r *CharacterRepository) GetByID(characterID string) (*models.Character, error) {
	var character models.Character
	result := database.DB.First(&character, "id = ?", characterID)
	if result.Error != nil {
		return nil, result.Error
	}
	return &character, nil
}

func (r *CharacterRepository) GetByProject(projectID string) ([]models.Character, error) {
	var characters []models.Character
	result := database.DB.Where("project_id = ?", projectID).Order("name ASC").Find(&characters)
	return characters, result.Error
}

func (r *CharacterRepository) Update(characterID string, updates map[string]interface{}) error {
	if rel, ok := updates["relationships"]; ok {
		if relMap, ok := rel.(map[string]string); ok {
			data, _ := json.Marshal(relMap)
			updates["relationships"] = data
		}
	}
	return database.DB.Model(&models.Character{}).Where("id = ?", characterID).Updates(updates).Error
}

func (r *CharacterRepository) Delete(characterID string) error {
	return database.DB.Where("id = ?", characterID).Delete(&models.Character{}).Error
}

// ========== BookChunk Repository ==========

type BookChunkRepository struct{}

func NewBookChunkRepository() *BookChunkRepository {
	return &BookChunkRepository{}
}

func (r *BookChunkRepository) SearchSimilar(queryEmbedding []float32, filterGenre []string, limit int) ([]models.SearchResult, error) {
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
	
	rows, err := database.DB.Raw(query, queryEmbedding, filterGenre, filterGenre, queryEmbedding, limit).Rows()
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

	return results, nil
}
