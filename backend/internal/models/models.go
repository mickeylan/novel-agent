package models

import (
	"encoding/json"
	"time"
)

// ========== Book Models ==========

type Book struct {
	ID          string     `json:"id" gorm:"primaryKey;type:uuid"`
	Title       string     `json:"title" gorm:"type:text;not null"`
	Author      *string    `json:"author" gorm:"type:text"`
	Genre       []string   `json:"genre" gorm:"type:text[]"`
	Language    string     `json:"language" gorm:"type:text;default:zh"`
	WordCount   *int       `json:"word_count" gorm:"type:integer"`
	FilePath    *string    `json:"file_path" gorm:"type:text"`
	FileType    *string    `json:"file_type" gorm:"type:text"`
	ProcessedAt *time.Time `json:"processed_at" gorm:"type:timestamptz"`
	CreatedAt   time.Time  `json:"created_at" gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"type:timestamptz;autoUpdateTime"`
}

func (Book) TableName() string {
	return "books"
}

type BookChunk struct {
	ID        string          `json:"id" gorm:"primaryKey;type:uuid"`
	BookID    string          `json:"book_id" gorm:"type:uuid;index"`
	Chapter   *int            `json:"chapter" gorm:"type:integer"`
	Section   *int            `json:"section" gorm:"type:integer"`
	Content   string          `json:"content" gorm:"type:text;not null"`
	Embedding []float32       `json:"embedding" gorm:"type:vector(1024)"`
	Metadata  json.RawMessage `json:"metadata" gorm:"type:jsonb;default:'{}'"`
	WordCount *int            `json:"word_count" gorm:"type:integer"`
	CreatedAt time.Time       `json:"created_at" gorm:"type:timestamptz;autoCreateTime"`
}

func (BookChunk) TableName() string {
	return "book_chunks"
}

// ========== Project Models ==========

type StoryProject struct {
	ID           string    `json:"id" gorm:"primaryKey;type:uuid"`
	Title        string    `json:"title" gorm:"type:text;not null"`
	Genre        []string  `json:"genre" gorm:"type:text[]"`
	WorldSetting *string   `json:"world_setting" gorm:"type:text"`
	Premise      *string   `json:"premise" gorm:"type:text"`
	Outline      *string   `json:"outline" gorm:"type:text"`
	Status       string    `json:"status" gorm:"type:text;default:draft"`
	CreatedAt    time.Time `json:"created_at" gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"type:timestamptz;autoUpdateTime"`
}

func (StoryProject) TableName() string {
	return "story_projects"
}

// ========== Character Models ==========

type Character struct {
	ID            string          `json:"id" gorm:"primaryKey;type:uuid"`
	ProjectID     string          `json:"project_id" gorm:"type:uuid;index"`
	Name          string          `json:"name" gorm:"type:text;not null"`
	Traits        []string        `json:"traits" gorm:"type:text[]"`
	Relationships json.RawMessage `json:"relationships" gorm:"type:jsonb;default:'{}'"`
	ArcSummary    *string         `json:"arc_summary" gorm:"type:text"`
	CurrentState  *string         `json:"current_state" gorm:"type:text"`
	CreatedAt     time.Time       `json:"created_at" gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt     time.Time       `json:"updated_at" gorm:"type:timestamptz;autoUpdateTime"`
}

func (Character) TableName() string {
	return "characters"
}

// ========== Chapter Models ==========

type Chapter struct {
	ID               string     `json:"id" gorm:"primaryKey;type:uuid"`
	ProjectID        string     `json:"project_id" gorm:"type:uuid;index"`
	ChapterNumber    int        `json:"chapter_number" gorm:"not null"`
	Title            *string    `json:"title" gorm:"type:text"`
	Outline          *string    `json:"outline" gorm:"type:text"`
	Content          *string    `json:"content" gorm:"type:text"`
	Status           string     `json:"status" gorm:"type:text;default:draft"`
	Summary          *string    `json:"summary" gorm:"type:text"`
	KeyEvents        []string   `json:"key_events" gorm:"type:text[]"`
	Foreshadowing    *string    `json:"foreshadowing" gorm:"type:text"`
	RevisedAt        *time.Time `json:"revised_at" gorm:"type:timestamptz"`
	ConsistencyScore *float64   `json:"consistency_score" gorm:"type:decimal(3,2)"`
	CreatedAt        time.Time  `json:"created_at" gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt        time.Time  `json:"updated_at" gorm:"type:timestamptz;autoUpdateTime"`
}

func (Chapter) TableName() string {
	return "chapters"
}

// ========== Memory Models ==========

type ParticleLedger struct {
	ID            string          `json:"id" gorm:"primaryKey;type:uuid"`
	ProjectID     string          `json:"project_id" gorm:"type:uuid;index"`
	CharacterName string          `json:"character_name" gorm:"type:varchar(100);not null"`
	ResourceType  string          `json:"resource_type" gorm:"type:varchar(50);not null"`
	ResourceName  string          `json:"resource_name" gorm:"type:varchar(200);not null"`
	CurrentValue  *string         `json:"current_value" gorm:"type:text"`
	PreviousValue *string         `json:"previous_value" gorm:"type:text"`
	ChapterNumber int             `json:"chapter_number" gorm:"type:integer;not null"`
	Metadata      json.RawMessage `json:"metadata" gorm:"type:jsonb;default:'{}'"`
	CreatedAt     time.Time       `json:"created_at" gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt     time.Time       `json:"updated_at" gorm:"type:timestamptz;autoUpdateTime"`
}

func (ParticleLedger) TableName() string {
	return "particle_ledger"
}

type PendingHook struct {
	ID                        string          `json:"id" gorm:"primaryKey;type:uuid"`
	ProjectID                 string          `json:"project_id" gorm:"type:uuid;index"`
	HookType                  string          `json:"hook_type" gorm:"type:varchar(50);not null"`
	Description               string          `json:"description" gorm:"type:text;not null"`
	Priority                  string          `json:"priority" gorm:"type:varchar(20);default:medium"`
	IntroducedChapter         int             `json:"introduced_chapter" gorm:"type:integer;not null"`
	ExpectedResolutionChapter *int            `json:"expected_resolution_chapter" gorm:"type:integer"`
	ResolvedChapter           *int            `json:"resolved_chapter" gorm:"type:integer"`
	ResolutionDescription     *string         `json:"resolution_description" gorm:"type:text"`
	Status                    string          `json:"status" gorm:"type:varchar(20);default:active"`
	RelatedCharacters         []string        `json:"related_characters" gorm:"type:text[]"`
	Metadata                  json.RawMessage `json:"metadata" gorm:"type:jsonb;default:'{}'"`
	CreatedAt                 time.Time       `json:"created_at" gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt                 time.Time       `json:"updated_at" gorm:"type:timestamptz;autoUpdateTime"`
}

func (PendingHook) TableName() string {
	return "pending_hooks"
}

type SubplotBoard struct {
	ID                 string          `json:"id" gorm:"primaryKey;type:uuid"`
	ProjectID          string          `json:"project_id" gorm:"type:uuid;index"`
	SubplotName        string          `json:"subplot_name" gorm:"type:varchar(200);not null"`
	SubplotType        *string         `json:"subplot_type" gorm:"type:varchar(50)"`
	Description        *string         `json:"description" gorm:"type:text"`
	Status             string          `json:"status" gorm:"type:varchar(20);default:active"`
	StartChapter       int             `json:"start_chapter" gorm:"type:integer;not null"`
	CurrentChapter     *int            `json:"current_chapter" gorm:"type:integer"`
	ExpectedEndChapter *int            `json:"expected_end_chapter" gorm:"type:integer"`
	ProgressPercentage int             `json:"progress_percentage" gorm:"type:integer;default:0"`
	KeyEvents          []string        `json:"key_events" gorm:"type:text[]"`
	RelatedCharacters  []string        `json:"related_characters" gorm:"type:text[]"`
	Metadata           json.RawMessage `json:"metadata" gorm:"type:jsonb;default:'{}'"`
	CreatedAt          time.Time       `json:"created_at" gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt          time.Time       `json:"updated_at" gorm:"type:timestamptz;autoUpdateTime"`
}

func (SubplotBoard) TableName() string {
	return "subplot_board"
}

type EmotionalArc struct {
	ID               string          `json:"id" gorm:"primaryKey;type:uuid"`
	ProjectID        string          `json:"project_id" gorm:"type:uuid;index"`
	CharacterName    string          `json:"character_name" gorm:"type:varchar(100);not null"`
	ArcType          *string         `json:"arc_type" gorm:"type:varchar(50)"`
	StartingState    *string         `json:"starting_state" gorm:"type:text"`
	CurrentState     *string         `json:"current_state" gorm:"type:text"`
	TargetState      *string         `json:"target_state" gorm:"type:text"`
	KeyTurningPoints []string        `json:"key_turning_points" gorm:"type:text[]"`
	ChapterProgress  int             `json:"chapter_progress" gorm:"type:integer;default:0"`
	Metadata         json.RawMessage `json:"metadata" gorm:"type:jsonb;default:'{}'"`
	CreatedAt        time.Time       `json:"created_at" gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt        time.Time       `json:"updated_at" gorm:"type:timestamptz;autoUpdateTime"`
}

func (EmotionalArc) TableName() string {
	return "emotional_arcs"
}

// ========== Search Result ==========

type SearchResult struct {
	ChunkID    string          `json:"chunk_id"`
	Content    string          `json:"content"`
	BookTitle  string          `json:"book_title"`
	BookID     string          `json:"book_id"`
	Similarity float64         `json:"similarity"`
	Metadata   json.RawMessage `json:"metadata"`
}

// ========== Request/Response DTOs ==========

type CreateProjectRequest struct {
	Title        string   `json:"title" binding:"required"`
	Genres       []string `json:"genres"`
	WorldSetting *string  `json:"world_setting"`
	Premise      *string  `json:"premise"`
}

type CreateCharacterRequest struct {
	Name          string            `json:"name" binding:"required"`
	Traits        []string          `json:"traits"`
	Relationships map[string]string `json:"relationships"`
	ArcSummary    *string           `json:"arc_summary"`
	CurrentState  *string           `json:"current_state"`
}

type CreateChapterRequest struct {
	ChapterNumber int     `json:"chapter_number" binding:"required"`
	Title         *string `json:"title"`
	Outline       *string `json:"outline"`
	Content       *string `json:"content"`
}

type UpdateChapterRequest struct {
	Title     *string   `json:"title"`
	Outline   *string   `json:"outline"`
	Content   *string   `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SearchRequest struct {
	Query  string   `json:"query" binding:"required"`
	Limit  int      `json:"limit"`
	Genres []string `json:"genres"`
}

// ========== Response DTOs ==========

type ProjectResponse struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Genres       []string  `json:"genres"`
	WorldSetting *string   `json:"world_setting"`
	Premise      *string   `json:"premise"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CharacterResponse struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Traits        []string          `json:"traits"`
	Relationships map[string]string `json:"relationships"`
	ArcSummary    *string           `json:"arc_summary"`
	CurrentState  *string           `json:"current_state"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type ChapterResponse struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	ChapterNumber    int       `json:"chapter_number"`
	Title            *string   `json:"title"`
	Content          *string   `json:"content"`
	Outline          *string   `json:"outline"`
	Summary          *string   `json:"summary"`
	WordCount        int       `json:"word_count"`
	Foreshadowing    *string   `json:"foreshadowing"`
	ConsistencyScore *float64  `json:"consistency_score"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type SearchResultResponse struct {
	ID         string   `json:"id"`
	BookTitle  string   `json:"book_title"`
	Author     string   `json:"author"`
	Content    string   `json:"content"`
	Similarity *float64 `json:"similarity"`
	Genres     []string `json:"genres"`
}

type SearchResponse struct {
	Query   string                 `json:"query"`
	Total   int                    `json:"total"`
	Results []SearchResultResponse `json:"results"`
}

type HealthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	Database string `json:"database"`
}
