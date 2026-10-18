package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"novel-agent/internal/models"
	"novel-agent/internal/repository"
	"novel-agent/internal/service"
	"novel-agent/internal/service/agents"
	"novel-agent/internal/service/orchestration"
	"novel-agent/internal/service/retrieval"
	"novel-agent/pkg/database"
)

type Handler struct {
	projectRepo   *repository.ProjectRepository
	chapterRepo   *repository.ChapterRepository
	characterRepo *repository.CharacterRepository
	chunkRepo     *repository.BookChunkRepository
	schemaRepo    *repository.SchemaRepository
	svc           *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	if svc == nil {
		svc = &service.Service{}
	}
	return &Handler{
		projectRepo:   repository.NewProjectRepository(),
		chapterRepo:   repository.NewChapterRepository(),
		characterRepo: repository.NewCharacterRepository(),
		chunkRepo:     repository.NewBookChunkRepository(),
		schemaRepo:    repository.NewSchemaRepository(),
		svc:           svc,
	}
}

func safeErrorCategory(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case err == nil:
		return "unknown"
	default:
		return "internal"
	}
}

func logSafeHandlerError(c *gin.Context, operation string, err error) {
	log.Printf("[error] request_id=%s method=%s route=%s operation=%s category=%s error_type=%T",
		requestIDFromContext(c), c.Request.Method, c.FullPath(), operation, safeErrorCategory(err), err)
}

func decodeRelationships(data json.RawMessage) (map[string]string, error) {
	if len(data) == 0 || strings.TrimSpace(string(data)) == "null" {
		return nil, nil
	}
	var relationships map[string]string
	if err := json.Unmarshal(data, &relationships); err != nil {
		return nil, fmt.Errorf("invalid stored character relationships")
	}
	return relationships, nil
}

func validBoundedText(value string, required bool, maxRunes int) bool {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return false
	}
	return utf8.RuneCountInString(value) <= maxRunes
}

func validOptionalText(value *string, maxRunes int) bool {
	return value == nil || validBoundedText(*value, false, maxRunes)
}

func validOptionalJSONObject(value *json.RawMessage, maxRunes int) bool {
	if value == nil {
		return true
	}
	text := strings.TrimSpace(string(*value))
	return text != "" && utf8.RuneCountInString(text) <= maxRunes && json.Valid([]byte(text)) && strings.HasPrefix(text, "{")
}

func validStringList(values []string, maxItems, maxItemRunes int) bool {
	if len(values) > maxItems {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if !validBoundedText(normalized, true, maxItemRunes) {
			return false
		}
		if _, duplicate := seen[normalized]; duplicate {
			return false
		}
		seen[normalized] = struct{}{}
	}
	return true
}

func normalizeStringList(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = strings.TrimSpace(value)
	}
	return result
}

func normalizeRelationships(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}

func validRelationships(values map[string]string) bool {
	if len(values) > 200 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for key, value := range values {
		normalizedKey := strings.TrimSpace(key)
		if !validBoundedText(normalizedKey, true, 500) || !validBoundedText(value, true, 2000) {
			return false
		}
		if _, duplicate := seen[normalizedKey]; duplicate {
			return false
		}
		seen[normalizedKey] = struct{}{}
	}
	return true
}

func requireUpdateVersion(c *gin.Context, updatedAt time.Time) bool {
	if !updatedAt.IsZero() {
		return true
	}
	c.JSON(http.StatusBadRequest, gin.H{
		"error":      "updated_at is required",
		"code":       "invalid_update_version",
		"request_id": requestIDFromContext(c),
	})
	return false
}

func bindJSON(c *gin.Context, destination interface{}) bool {
	if err := c.ShouldBindJSON(destination); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "request body does not match the expected JSON contract",
			"code":       "invalid_request_body",
			"request_id": requestIDFromContext(c),
		})
		return false
	}
	return true
}

func (h *Handler) writeInternalError(c *gin.Context, err error) {
	requestID := requestIDFromContext(c)
	logSafeHandlerError(c, "request_failed", err)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":      "internal server error",
		"code":       "internal_error",
		"error_id":   requestID,
		"request_id": requestID,
	})
}

var (
	errNilRepositoryResult = errors.New("repository returned nil result without error")
	errNilServiceResult    = errors.New("service returned nil result without error")
)

func (h *Handler) requireServiceResult(c *gin.Context, present bool) bool {
	if present {
		return true
	}
	h.writeInternalError(c, errNilServiceResult)
	return false
}

func (h *Handler) requireReadResult(c *gin.Context, present bool) bool {
	if present {
		return true
	}
	h.writeInternalError(c, errNilRepositoryResult)
	return false
}

func (h *Handler) writeReadError(c *gin.Context, err error, notFoundMessage string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": notFoundMessage})
		return
	}
	h.writeInternalError(c, err)
}

func (h *Handler) requireCommittedID(c *gin.Context, id string) bool {
	parsed, err := uuid.Parse(id)
	if err == nil && parsed != uuid.Nil && strings.EqualFold(id, parsed.String()) {
		return true
	}
	h.writeCommittedResultUnavailable(c, errNilRepositoryResult)
	return false
}

func (h *Handler) requireCommittedResult(c *gin.Context, present bool) bool {
	if present {
		return true
	}
	h.writeCommittedResultUnavailable(c, errNilRepositoryResult)
	return false
}

func (h *Handler) writeCommittedResultUnavailable(c *gin.Context, err error) {
	requestID := requestIDFromContext(c)
	logSafeHandlerError(c, "write_committed_result_unavailable", err)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":      "write may have succeeded but the saved result could not be loaded",
		"code":       "write_result_unavailable",
		"error_id":   requestID,
		"request_id": requestID,
		"retry_safe": false,
	})
}

// ========== Health Check ==========

func (h *Handler) HealthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	statusCode := http.StatusOK
	status := "healthy"
	dbStatus := "connected"
	if err := database.Ping(ctx); err != nil {
		statusCode = http.StatusServiceUnavailable
		status = "unhealthy"
		dbStatus = "unavailable"
	}

	c.JSON(statusCode, models.HealthResponse{
		Status:   status,
		Version:  "0.1.0",
		Database: dbStatus,
	})
}

func (h *Handler) SchemaCompatibility(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	report, err := h.schemaRepo.Inspect(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "schema inspection unavailable", "code": "schema_inspection_failed"})
		return
	}
	c.JSON(http.StatusOK, report)
}

// ========== Project Handlers ==========

func (h *Handler) CreateProject(c *gin.Context) {
	var req models.CreateProjectRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if !validBoundedText(req.Title, true, 500) || !validStringList(req.Genres, 20, 100) || !validOptionalText(req.WorldSetting, 12000) || !validOptionalText(req.Premise, 12000) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project fields exceed allowed limits", "code": "invalid_project"})
		return
	}
	req.Genres = normalizeStringList(req.Genres)

	project := &models.StoryProject{
		Title:        req.Title,
		Genre:        req.Genres,
		WorldSetting: req.WorldSetting,
		Premise:      req.Premise,
		Status:       "draft",
	}

	id, err := h.projectRepo.Create(c.Request.Context(), project)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if !h.requireCommittedID(c, id) {
		return
	}

	created, err := h.projectRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	if !h.requireCommittedResult(c, created != nil) {
		return
	}
	c.JSON(http.StatusCreated, models.ProjectResponse{
		ID:           created.ID,
		Title:        created.Title,
		Genres:       created.Genre,
		WorldSetting: created.WorldSetting,
		Premise:      created.Premise,
		Status:       created.Status,
		CreatedAt:    created.CreatedAt,
		UpdatedAt:    created.UpdatedAt,
	})
}

func (h *Handler) ListProjects(c *gin.Context) {
	projects, err := h.projectRepo.ListAll(c.Request.Context())
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	response := make([]models.ProjectResponse, len(projects))
	for i, p := range projects {
		response[i] = models.ProjectResponse{
			ID:           p.ID,
			Title:        p.Title,
			Genres:       p.Genre,
			WorldSetting: p.WorldSetting,
			Premise:      p.Premise,
			Status:       p.Status,
			CreatedAt:    p.CreatedAt,
			UpdatedAt:    p.UpdatedAt,
		}
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetProject(c *gin.Context) {
	projectID := c.Param("project_id")

	project, err := h.projectRepo.GetByID(c.Request.Context(), projectID)
	if err != nil {
		h.writeReadError(c, err, "Project not found")
		return
	}
	if !h.requireReadResult(c, project != nil) {
		return
	}

	c.JSON(http.StatusOK, models.ProjectResponse{
		ID:           project.ID,
		Title:        project.Title,
		Genres:       project.Genre,
		WorldSetting: project.WorldSetting,
		Premise:      project.Premise,
		Status:       project.Status,
		CreatedAt:    project.CreatedAt,
		UpdatedAt:    project.UpdatedAt,
	})
}

func (h *Handler) UpdateProject(c *gin.Context) {
	projectID := c.Param("project_id")
	var req struct {
		Title        *string   `json:"title"`
		Genres       *[]string `json:"genres"`
		WorldSetting *string   `json:"world_setting"`
		Premise      *string   `json:"premise"`
		Status       *string   `json:"status"`
		UpdatedAt    time.Time `json:"updated_at"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if !requireUpdateVersion(c, req.UpdatedAt) {
		return
	}
	if (req.Title != nil && !validBoundedText(*req.Title, true, 500)) || (req.Genres != nil && !validStringList(*req.Genres, 20, 100)) || !validOptionalText(req.WorldSetting, 12000) || !validOptionalText(req.Premise, 12000) || (req.Status != nil && !validBoundedText(*req.Status, true, 100)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project fields exceed allowed limits", "code": "invalid_project"})
		return
	}
	updates := make(map[string]interface{})
	if req.Title != nil {
		updates["title"] = strings.TrimSpace(*req.Title)
	}
	if req.Genres != nil {
		updates["genre"] = normalizeStringList(*req.Genres)
	}
	if req.WorldSetting != nil {
		updates["world_setting"] = *req.WorldSetting
	}
	if req.Premise != nil {
		updates["premise"] = *req.Premise
	}
	if req.Status != nil {
		updates["status"] = strings.TrimSpace(*req.Status)
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one update field is required", "code": "invalid_update"})
		return
	}
	project, err := h.projectRepo.GetByID(c.Request.Context(), projectID)
	if err != nil {
		h.writeReadError(c, err, "Project not found")
		return
	}
	if !h.requireReadResult(c, project != nil) {
		return
	}
	rows, err := h.projectRepo.UpdateIfUnchanged(c.Request.Context(), projectID, req.UpdatedAt, updates)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if rows == 0 {
		h.writeConflict(c, project.UpdatedAt)
		return
	}
	project, err = h.projectRepo.GetByID(c.Request.Context(), projectID)
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	if !h.requireCommittedResult(c, project != nil) {
		return
	}
	c.JSON(http.StatusOK, models.ProjectResponse{ID: project.ID, Title: project.Title, Genres: project.Genre, WorldSetting: project.WorldSetting, Premise: project.Premise, Status: project.Status, CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt})
}

// ========== Character Handlers ==========

func (h *Handler) CreateCharacter(c *gin.Context) {
	projectID := c.Param("project_id")

	var req models.CreateCharacterRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !validBoundedText(req.Name, true, 500) || !validStringList(req.Traits, 100, 500) || !validOptionalText(req.ArcSummary, 12000) || !validOptionalText(req.CurrentState, 12000) || !validRelationships(req.Relationships) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "character fields exceed allowed limits", "code": "invalid_character"})
		return
	}
	req.Traits = normalizeStringList(req.Traits)
	req.Relationships = normalizeRelationships(req.Relationships)

	relJSON, err := json.Marshal(req.Relationships)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "relationships could not be encoded", "code": "invalid_relationships"})
		return
	}
	character := &models.Character{
		ProjectID:     projectID,
		Name:          req.Name,
		Traits:        req.Traits,
		Relationships: relJSON,
		ArcSummary:    req.ArcSummary,
		CurrentState:  req.CurrentState,
	}

	id, err := h.characterRepo.Create(c.Request.Context(), character)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if !h.requireCommittedID(c, id) {
		return
	}
	created, err := h.characterRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	if !h.requireCommittedResult(c, created != nil) {
		return
	}
	relationships, err := decodeRelationships(created.Relationships)
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	c.JSON(http.StatusCreated, models.CharacterResponse{
		ID:            created.ID,
		Name:          created.Name,
		Traits:        created.Traits,
		Relationships: relationships,
		ArcSummary:    created.ArcSummary,
		CurrentState:  created.CurrentState,
		UpdatedAt:     created.UpdatedAt,
	})
}

func (h *Handler) ListCharacters(c *gin.Context) {
	projectID := c.Param("project_id")

	characters, err := h.characterRepo.GetByProject(c.Request.Context(), projectID)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	response := make([]models.CharacterResponse, len(characters))
	for i, ch := range characters {
		rel, err := decodeRelationships(ch.Relationships)
		if err != nil {
			h.writeInternalError(c, err)
			return
		}
		response[i] = models.CharacterResponse{
			ID:            ch.ID,
			Name:          ch.Name,
			Traits:        ch.Traits,
			Relationships: rel,
			ArcSummary:    ch.ArcSummary,
			CurrentState:  ch.CurrentState,
			UpdatedAt:     ch.UpdatedAt,
		}
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetCharacter(c *gin.Context) {
	projectID := c.Param("project_id")
	characterID := c.Param("character_id")

	character, err := h.characterRepo.GetByID(c.Request.Context(), characterID)
	if err != nil {
		h.writeReadError(c, err, "Character not found")
		return
	}
	if !h.requireReadResult(c, character != nil) {
		return
	}
	if character.ProjectID != projectID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Character not found"})
		return
	}

	rel, err := decodeRelationships(character.Relationships)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, models.CharacterResponse{
		ID:            character.ID,
		Name:          character.Name,
		Traits:        character.Traits,
		Relationships: rel,
		ArcSummary:    character.ArcSummary,
		CurrentState:  character.CurrentState,
		UpdatedAt:     character.UpdatedAt,
	})
}

func (h *Handler) UpdateCharacter(c *gin.Context) {
	projectID := c.Param("project_id")
	characterID := c.Param("character_id")
	var req struct {
		Name          *string           `json:"name"`
		Traits        *[]string         `json:"traits"`
		Relationships map[string]string `json:"relationships"`
		ArcSummary    *string           `json:"arc_summary"`
		CurrentState  *string           `json:"current_state"`
		UpdatedAt     time.Time         `json:"updated_at"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if !requireUpdateVersion(c, req.UpdatedAt) {
		return
	}
	if (req.Name != nil && !validBoundedText(*req.Name, true, 500)) || (req.Traits != nil && !validStringList(*req.Traits, 100, 500)) || !validRelationships(req.Relationships) || !validOptionalText(req.ArcSummary, 12000) || !validOptionalText(req.CurrentState, 12000) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "character fields exceed allowed limits", "code": "invalid_character"})
		return
	}
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Traits != nil {
		updates["traits"] = normalizeStringList(*req.Traits)
	}
	if req.Relationships != nil {
		updates["relationships"] = normalizeRelationships(req.Relationships)
	}
	if req.ArcSummary != nil {
		updates["arc_summary"] = *req.ArcSummary
	}
	if req.CurrentState != nil {
		updates["current_state"] = *req.CurrentState
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one update field is required", "code": "invalid_update"})
		return
	}
	character, err := h.characterRepo.GetByID(c.Request.Context(), characterID)
	if err != nil {
		h.writeReadError(c, err, "Character not found")
		return
	}
	if !h.requireReadResult(c, character != nil) {
		return
	}
	if character.ProjectID != projectID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Character not found"})
		return
	}
	rows, err := h.characterRepo.UpdateIfUnchanged(c.Request.Context(), projectID, characterID, req.UpdatedAt, updates)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if rows == 0 {
		h.writeConflict(c, character.UpdatedAt)
		return
	}
	updated, err := h.characterRepo.GetByID(c.Request.Context(), characterID)
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	if !h.requireCommittedResult(c, updated != nil) {
		return
	}
	rel, err := decodeRelationships(updated.Relationships)
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	c.JSON(http.StatusOK, models.CharacterResponse{ID: updated.ID, Name: updated.Name, Traits: updated.Traits, Relationships: rel, ArcSummary: updated.ArcSummary, CurrentState: updated.CurrentState, UpdatedAt: updated.UpdatedAt})
}

// ========== Chapter Handlers ==========

func (h *Handler) CreateChapter(c *gin.Context) {
	projectID := c.Param("project_id")

	var req models.CreateChapterRequest
	if !bindJSON(c, &req) {
		return
	}
	if !validChapterNumber(req.ChapterNumber) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter_number must be an integer between 1 and 1000000", "code": "invalid_chapter_number"})
		return
	}
	if !validOptionalText(req.Title, 500) || !validOptionalText(req.Outline, 12000) || !validOptionalText(req.Content, 100000) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter fields exceed allowed limits", "code": "invalid_chapter"})
		return
	}

	chapter := &models.Chapter{
		ProjectID:     projectID,
		ChapterNumber: req.ChapterNumber,
		Title:         req.Title,
		Outline:       req.Outline,
		Content:       req.Content,
		Status:        "draft",
	}

	id, err := h.chapterRepo.Create(c.Request.Context(), chapter)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if !h.requireCommittedID(c, id) {
		return
	}

	created, err := h.chapterRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	if !h.requireCommittedResult(c, created != nil) {
		return
	}
	c.JSON(http.StatusCreated, h.chapterToResponse(created))
}

func (h *Handler) ListChapters(c *gin.Context) {
	projectID := c.Param("project_id")

	chapters, err := h.chapterRepo.GetByProject(c.Request.Context(), projectID)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	response := make([]models.ChapterResponse, len(chapters))
	for i, ch := range chapters {
		response[i] = h.chapterToResponse(&ch)
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetChapter(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, mustInt(chapterNumber))
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}

	c.JSON(http.StatusOK, h.chapterToResponse(chapter))
}

func (h *Handler) UpdateChapter(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	var req models.UpdateChapterRequest
	if !bindJSON(c, &req) {
		return
	}
	if !requireUpdateVersion(c, req.UpdatedAt) {
		return
	}
	if !validOptionalText(req.Title, 500) || !validOptionalText(req.Outline, 12000) || !validOptionalText(req.Content, 100000) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter fields exceed allowed limits", "code": "invalid_chapter"})
		return
	}

	updates := make(map[string]interface{})
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Outline != nil {
		updates["outline"] = *req.Outline
	}
	if req.Content != nil {
		updates["content"] = *req.Content
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one update field is required", "code": "invalid_update"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, mustInt(chapterNumber))
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	rows, err := h.chapterRepo.UpdateIfUnchanged(c.Request.Context(), projectID, mustInt(chapterNumber), req.UpdatedAt, updates)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if rows == 0 {
		h.writeConflict(c, chapter.UpdatedAt)
		return
	}

	updated, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, mustInt(chapterNumber))
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	if !h.requireCommittedResult(c, updated != nil) {
		return
	}
	c.JSON(http.StatusOK, h.chapterToResponse(updated))
}

// ========== Creation Handlers ==========

type GenerateOutlineRequest struct {
	Premise        string `json:"premise"`
	Genre          string `json:"genre"`
	TargetChapters int    `json:"target_chapters"`
}

func (h *Handler) GenerateOutline(c *gin.Context) {
	projectID := c.Param("project_id")

	var req GenerateOutlineRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Premise = strings.TrimSpace(req.Premise)
	req.Genre = strings.TrimSpace(req.Genre)
	if req.Premise == "" || utf8.RuneCountInString(req.Premise) > 12000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "premise must contain 1 to 12000 characters"})
		return
	}
	if utf8.RuneCountInString(req.Genre) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "genre must not exceed 200 characters"})
		return
	}
	if req.TargetChapters < 0 || req.TargetChapters > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_chapters must be between 1 and 100 when provided"})
		return
	}

	ctx := c.Request.Context()

	// Generate three-act structure
	structure, err := h.svc.Architect.GenerateThreeActStructure(ctx, req.Premise, req.Genre)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if !h.requireServiceResult(c, structure != nil) {
		return
	}

	// Preview only. Persistence requires an explicit PUT /outline request.
	c.JSON(http.StatusOK, gin.H{
		"project_id": projectID,
		"premise":    req.Premise,
		"structure":  structure,
	})
}

type WriteChapterRequest struct {
	Outline         string `json:"outline"`
	StyleGuide      string `json:"style_guide"`
	TargetWordCount int    `json:"target_word_count"`
}

func (h *Handler) WriteChapter(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	var req WriteChapterRequest
	if !bindJSON(c, &req) {
		return
	}

	req.Outline = strings.TrimSpace(req.Outline)
	req.StyleGuide = strings.TrimSpace(req.StyleGuide)
	if req.Outline == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "outline is required"})
		return
	}
	targetWordCount := req.TargetWordCount
	if targetWordCount == 0 {
		targetWordCount = 5000
	}
	chapterNum := mustInt(chapterNumber)
	if chapterNum < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	if targetWordCount < 100 || targetWordCount > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be between 100 and 20000"})
		return
	}
	if utf8.RuneCountInString(req.Outline) > 12000 || utf8.RuneCountInString(req.StyleGuide) > 4000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "outline must not exceed 12000 characters and style_guide must not exceed 4000 characters"})
		return
	}

	ctx := c.Request.Context()

	// Get previous chapter for context.
	prevChapter, err := h.previousChapterTail(ctx, projectID, chapterNum, 500)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	// Write chapter
	content, err := h.svc.Writer.WriteChapter(ctx, req.Outline, req.StyleGuide, "", prevChapter, targetWordCount)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	// Preview only. Persistence requires an explicit chapter update request.
	wordCount := utf8.RuneCountInString(content)
	c.JSON(http.StatusOK, gin.H{
		"chapter_number": chapterNum,
		"content":        content,
		"word_count":     wordCount,
	})
}

type AuditChapterRequest struct {
	ChapterNumber int `json:"chapter_number"`
}

func (h *Handler) AuditChapter(c *gin.Context) {
	projectID := c.Param("project_id")

	var req AuditChapterRequest
	if !bindJSON(c, &req) {
		return
	}
	if !validChapterNumber(req.ChapterNumber) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter_number must be an integer between 1 and 1000000", "code": "invalid_chapter_number"})
		return
	}

	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, req.ChapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}

	if chapter.Content == nil || *chapter.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}

	ctx := c.Request.Context()

	// Get previous chapter.
	prevChapter, err := h.previousChapterTail(ctx, projectID, req.ChapterNumber, 0)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	// Audit
	result, err := h.svc.Critic.AuditChapter(ctx, *chapter.Content, prevChapter, derefString(chapter.Outline))
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if !h.requireServiceResult(c, result != nil) {
		return
	}

	c.JSON(http.StatusOK, result)
}

type PolishChapterRequest struct {
	ChapterNumber int    `json:"chapter_number"`
	StyleGuide    string `json:"style_guide"`
}

func (h *Handler) PolishChapter(c *gin.Context) {
	projectID := c.Param("project_id")

	var req PolishChapterRequest
	if !bindJSON(c, &req) {
		return
	}
	if !validChapterNumber(req.ChapterNumber) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter_number must be an integer between 1 and 1000000", "code": "invalid_chapter_number"})
		return
	}
	if utf8.RuneCountInString(req.StyleGuide) > 4000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "style_guide must not exceed 4000 characters"})
		return
	}

	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, req.ChapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}

	if chapter.Content == nil || *chapter.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}

	ctx := c.Request.Context()

	// Polish
	polished, err := h.svc.Editor.PolishChapter(ctx, *chapter.Content, req.StyleGuide)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	// Preview only. Persistence requires an explicit chapter update request.
	c.JSON(http.StatusOK, gin.H{
		"chapter_number": req.ChapterNumber,
		"content":        polished,
		"word_count":     utf8.RuneCountInString(polished),
	})
}

// ========== Outline Handlers ==========

func (h *Handler) GetOutline(c *gin.Context) {
	projectID := c.Param("project_id")

	project, err := h.projectRepo.GetByID(c.Request.Context(), projectID)
	if err != nil {
		h.writeReadError(c, err, "Project not found")
		return
	}
	if !h.requireReadResult(c, project != nil) {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project_id":    project.ID,
		"premise":       derefString(project.Premise),
		"world_setting": derefString(project.WorldSetting),
		"outline":       derefString(project.Outline),
		"updated_at":    project.UpdatedAt,
	})
}

func (h *Handler) UpdateOutline(c *gin.Context) {
	projectID := c.Param("project_id")

	var req struct {
		Premise      *string          `json:"premise"`
		WorldSetting *string          `json:"world_setting"`
		Structure    *json.RawMessage `json:"structure"`
		UpdatedAt    time.Time        `json:"updated_at"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if !requireUpdateVersion(c, req.UpdatedAt) {
		return
	}
	if !validOptionalText(req.Premise, 12000) || !validOptionalText(req.WorldSetting, 12000) || !validOptionalJSONObject(req.Structure, 12000) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "outline fields exceed allowed limits", "code": "invalid_outline"})
		return
	}

	updates := make(map[string]interface{})
	if req.Premise != nil {
		updates["premise"] = *req.Premise
	}
	if req.WorldSetting != nil {
		updates["world_setting"] = *req.WorldSetting
	}
	if req.Structure != nil {
		updates["outline"] = string(*req.Structure)
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one update field is required", "code": "invalid_update"})
		return
	}
	project, err := h.projectRepo.GetByID(c.Request.Context(), projectID)
	if err != nil {
		h.writeReadError(c, err, "Project not found")
		return
	}
	if !h.requireReadResult(c, project != nil) {
		return
	}
	rows, err := h.projectRepo.UpdateIfUnchanged(c.Request.Context(), projectID, req.UpdatedAt, updates)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if rows == 0 {
		h.writeConflict(c, project.UpdatedAt)
		return
	}
	project, err = h.projectRepo.GetByID(c.Request.Context(), projectID)
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	if !h.requireCommittedResult(c, project != nil) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"project_id": project.ID, "premise": derefString(project.Premise), "world_setting": derefString(project.WorldSetting), "outline": derefString(project.Outline), "updated_at": project.UpdatedAt})
}

func (h *Handler) CreateOutline(c *gin.Context) {
	h.UpdateOutline(c) // For now, same as update
}

func (h *Handler) GetChapterOutline(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, mustInt(chapterNumber))
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project_id":     projectID,
		"chapter_number": chapter.ChapterNumber,
		"title":          derefString(chapter.Title),
		"outline":        derefString(chapter.Outline),
		"updated_at":     chapter.UpdatedAt,
	})
}

func (h *Handler) UpdateChapterOutline(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	var req struct {
		Title     *string   `json:"title"`
		Outline   *string   `json:"outline"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if !requireUpdateVersion(c, req.UpdatedAt) {
		return
	}
	if !validOptionalText(req.Title, 500) || !validOptionalText(req.Outline, 12000) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter outline fields exceed allowed limits", "code": "invalid_chapter"})
		return
	}

	updates := make(map[string]interface{})
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Outline != nil {
		updates["outline"] = *req.Outline
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one update field is required", "code": "invalid_update"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, mustInt(chapterNumber))
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	rows, err := h.chapterRepo.UpdateIfUnchanged(c.Request.Context(), projectID, mustInt(chapterNumber), req.UpdatedAt, updates)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if rows == 0 {
		h.writeConflict(c, chapter.UpdatedAt)
		return
	}

	chapter, err = h.chapterRepo.GetByNumber(c.Request.Context(), projectID, mustInt(chapterNumber))
	if err != nil {
		h.writeCommittedResultUnavailable(c, err)
		return
	}
	if !h.requireCommittedResult(c, chapter != nil) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"project_id":     projectID,
		"chapter_number": chapter.ChapterNumber,
		"title":          derefString(chapter.Title),
		"outline":        derefString(chapter.Outline),
		"updated_at":     chapter.UpdatedAt,
	})
}

// ========== Search Handlers ==========

func (h *Handler) SearchVector(c *gin.Context) {
	var req models.SearchRequest
	if !bindJSON(c, &req) {
		return
	}

	if !normalizeSearchRequest(c, &req) {
		return
	}

	ctx := c.Request.Context()
	results, err := h.svc.Retrieval.VectorSearch(ctx, req.Query, req.Genres, req.Limit)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	response := make([]models.SearchResultResponse, len(results))
	for i, r := range results {
		response[i] = models.SearchResultResponse{
			ID:         r.ChunkID,
			BookTitle:  r.BookTitle,
			Content:    r.Content,
			Similarity: &r.Similarity,
		}
	}

	c.JSON(http.StatusOK, models.SearchResponse{
		Query:   req.Query,
		Total:   len(results),
		Results: response,
	})
}

func (h *Handler) SearchKeyword(c *gin.Context) {
	var req models.SearchRequest
	if !bindJSON(c, &req) {
		return
	}

	if !normalizeSearchRequest(c, &req) {
		return
	}

	ctx := c.Request.Context()
	results, err := h.svc.Retrieval.KeywordSearch(ctx, req.Query, req.Genres, req.Limit)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}

	response := make([]models.SearchResultResponse, len(results))
	for i, r := range results {
		response[i] = models.SearchResultResponse{
			ID:         r.ChunkID,
			BookTitle:  r.BookTitle,
			Content:    r.Content,
			Similarity: &r.Similarity,
		}
	}

	c.JSON(http.StatusOK, models.SearchResponse{
		Query:   req.Query,
		Total:   len(results),
		Results: response,
	})
}

func (h *Handler) SearchHybrid(c *gin.Context) {
	var req models.SearchRequest
	if !bindJSON(c, &req) {
		return
	}
	if !normalizeSearchRequest(c, &req) {
		return
	}
	results, err := h.svc.Retrieval.HybridSearch(c.Request.Context(), req.Query, req.Genres, req.Limit)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, searchResponse(req.Query, results))
}

func (h *Handler) PreviewRAGContext(c *gin.Context) {
	projectID := c.Param("project_id")
	var req struct {
		Query  string   `json:"query" binding:"required"`
		Mode   string   `json:"mode"`
		Genres []string `json:"genres"`
		Limit  int      `json:"limit"`
	}
	if !bindJSON(c, &req) {
		return
	}
	searchReq := models.SearchRequest{Query: req.Query, Genres: req.Genres, Limit: req.Limit}
	if !normalizeSearchRequest(c, &searchReq) {
		return
	}
	if searchReq.Limit > 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "RAG preview limit must not exceed 10"})
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if !validRAGMode(mode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be hybrid, vector, or keyword"})
		return
	}
	if mode == "" {
		mode = "hybrid"
	}
	project, err := h.projectRepo.GetByID(c.Request.Context(), projectID)
	if err != nil {
		h.writeReadError(c, err, "Project not found")
		return
	}
	if !h.requireReadResult(c, project != nil) {
		return
	}
	var results []retrieval.SearchResult
	switch mode {
	case "hybrid":
		results, err = h.svc.Retrieval.HybridSearch(c.Request.Context(), searchReq.Query, searchReq.Genres, searchReq.Limit)
	case "vector":
		results, err = h.svc.Retrieval.VectorSearch(c.Request.Context(), searchReq.Query, searchReq.Genres, searchReq.Limit)
	case "keyword":
		results, err = h.svc.Retrieval.KeywordSearch(c.Request.Context(), searchReq.Query, searchReq.Genres, searchReq.Limit)
	}
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"project_id":     projectID,
		"query":          searchReq.Query,
		"mode":           mode,
		"results":        results,
		"prompt_context": h.svc.Retrieval.FormatResultsForPrompt(results, 800),
		"preview":        true,
	})
}

// ========== Safe Workflow, Analysis and Revision Preview Handlers ==========

func (h *Handler) PreviewWritingContext(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	maxRuneValues := c.Request.URL.Query()["max_runes"]
	maxRuneValue := "8000"
	if len(maxRuneValues) == 1 {
		maxRuneValue = maxRuneValues[0]
	}
	maxRunes, err := strconv.Atoi(maxRuneValue)
	if err != nil || maxRunes < 500 || maxRunes > 12000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_runes must be an integer between 500 and 12000", "code": "invalid_query_parameter"})
		return
	}
	writingContext, err := h.buildWritingContext(c.Request.Context(), projectID, chapterNumber, maxRunes)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "writing context unavailable", "code": "context_unavailable"})
			return
		}
		h.writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"project_id": projectID, "chapter_number": chapterNumber, "context": writingContext, "preview": true})
}

func (h *Handler) PreviewWorkflowTokenBudget(c *gin.Context) {
	var req struct {
		TargetWordCount    int  `json:"target_word_count"`
		MaxTotalTokens     int  `json:"max_total_tokens"`
		AutoRevise         bool `json:"auto_revise"`
		ExtractDataChanges bool `json:"extract_data_changes"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.TargetWordCount == 0 {
		req.TargetWordCount = 5000
	}
	if req.TargetWordCount < 100 || req.TargetWordCount > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be between 100 and 20000"})
		return
	}
	if req.MaxTotalTokens == 0 {
		req.MaxTotalTokens = orchestration.MaxWorkflowOutputTokens
	}
	if req.MaxTotalTokens < 1 || req.MaxTotalTokens > orchestration.MaxWorkflowOutputTokens {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("max_total_tokens must be between 1 and %d", orchestration.MaxWorkflowOutputTokens)})
		return
	}
	budget := orchestration.BuildTokenBudget(req.TargetWordCount, req.MaxTotalTokens, req.AutoRevise, req.ExtractDataChanges)
	c.JSON(http.StatusOK, gin.H{"token_budget": budget, "preview": true})
}

func (h *Handler) PreviewChapterWorkflow(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Outline            string   `json:"outline"`
		StyleGuide         string   `json:"style_guide"`
		MemoryContext      string   `json:"memory_context"`
		NewCharacters      []string `json:"new_characters"`
		TargetWordCount    int      `json:"target_word_count"`
		MaxTotalTokens     int      `json:"max_total_tokens"`
		AutoRevise         bool     `json:"auto_revise"`
		RevisionThreshold  float64  `json:"revision_threshold"`
		ExtractDataChanges bool     `json:"extract_data_changes"`
		AnalyzeAIGC        bool     `json:"analyze_aigc"`
		UseRAG             bool     `json:"use_rag"`
		RAGQuery           string   `json:"rag_query"`
		RAGMode            string   `json:"rag_mode"`
		RAGGenres          []string `json:"rag_genres"`
		RAGLimit           int      `json:"rag_limit"`
		UsePreviousStyle   bool     `json:"use_previous_style"`
		StyleSamples       int      `json:"style_samples"`
		UseScenePlan       bool     `json:"use_scene_plan"`
		SceneKeyEvents     []string `json:"scene_key_events"`
		MaxScenes          int      `json:"max_scenes"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.TargetWordCount != 0 && (req.TargetWordCount < 100 || req.TargetWordCount > 20000) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be between 100 and 20000"})
		return
	}
	if req.MaxTotalTokens == 0 {
		req.MaxTotalTokens = orchestration.MaxWorkflowOutputTokens
	}
	if req.MaxTotalTokens < 1 || req.MaxTotalTokens > orchestration.MaxWorkflowOutputTokens {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("max_total_tokens must be between 1 and %d", orchestration.MaxWorkflowOutputTokens)})
		return
	}
	reservedTokens := orchestration.EstimateOutputTokenBudget(req.TargetWordCount, req.AutoRevise, req.ExtractDataChanges)
	if reservedTokens > req.MaxTotalTokens {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("workflow requires a worst-case output budget of %d tokens", reservedTokens), "code": "token_budget_exceeded", "required_tokens": reservedTokens, "max_total_tokens": req.MaxTotalTokens})
		return
	}
	if req.RevisionThreshold < 0 || req.RevisionThreshold > 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "revision_threshold must be between 0 and 10"})
		return
	}
	if req.UsePreviousStyle && (req.StyleSamples < 0 || req.StyleSamples > 5) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "style_samples must be between 1 and 5"})
		return
	}
	if req.UseRAG && (req.RAGLimit < 0 || req.RAGLimit > 10) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rag_limit must be between 1 and 10"})
		return
	}
	if req.UseRAG && !validRAGMode(req.RAGMode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rag_mode must be hybrid, vector, or keyword"})
		return
	}
	if req.UseScenePlan && (req.MaxScenes < 0 || req.MaxScenes > 10) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_scenes must be between 1 and 10"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	outline := strings.TrimSpace(req.Outline)
	if outline == "" {
		outline = strings.TrimSpace(derefString(chapter.Outline))
	}
	if outline == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter outline is required"})
		return
	}
	previous, err := h.previousChapterTail(c.Request.Context(), projectID, chapterNumber, 1000)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	memoryContext := strings.TrimSpace(req.MemoryContext)
	var writingContext *agents.WritingContext
	if memoryContext == "" {
		builtContext, buildErr := h.buildWritingContext(c.Request.Context(), projectID, chapterNumber, 8000)
		if buildErr != nil {
			h.writeInternalError(c, buildErr)
			return
		}
		writingContext = &builtContext
		memoryContext = builtContext.PromptContext
	}

	var ragContext gin.H
	if req.UseRAG {
		ragQuery := strings.TrimSpace(req.RAGQuery)
		if ragQuery == "" {
			ragQuery = outline
		}
		ragLimit := req.RAGLimit
		if ragLimit == 0 {
			ragLimit = 5
		}
		if ragLimit < 1 || ragLimit > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "rag_limit must be between 1 and 10"})
			return
		}
		ragMode := strings.ToLower(strings.TrimSpace(req.RAGMode))
		if ragMode == "" {
			ragMode = "hybrid"
		}
		results, searchErr := h.searchRAG(c.Request.Context(), ragMode, ragQuery, req.RAGGenres, ragLimit)
		if searchErr != nil {
			h.writeInternalError(c, searchErr)
			return
		}
		promptContext := h.svc.Retrieval.FormatResultsForPrompt(results, 800)
		memoryContext = mergePromptContexts(memoryContext, promptContext, 12000)
		ragContext = gin.H{"query": ragQuery, "mode": ragMode, "results": results, "prompt_context": promptContext}
	}
	styleGuide := strings.TrimSpace(req.StyleGuide)
	var styleContext gin.H
	var styleReference *agents.StyleProfile
	var styleReferenceChapters []int
	var styleReferenceTruncated bool
	if req.UsePreviousStyle {
		styleSamples := req.StyleSamples
		if styleSamples == 0 {
			styleSamples = 5
		}
		if styleSamples < 1 || styleSamples > 5 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "style_samples must be between 1 and 5"})
			return
		}
		chapters, loadErr := h.chapterRepo.GetByProject(c.Request.Context(), projectID)
		if loadErr != nil {
			h.writeInternalError(c, loadErr)
			return
		}
		comparison := h.svc.Style.CompareWithPrevious("", chapters, chapterNumber, styleSamples, 50000)
		styleReference = &comparison.Reference
		styleReferenceChapters = comparison.ReferenceChapterNumbers
		styleReferenceTruncated = comparison.ReferenceTruncated
		generatedGuide := h.svc.Style.BuildGuide(comparison.Reference)
		styleGuide = mergePromptContexts(styleGuide, generatedGuide, 4000)
		styleContext = gin.H{
			"guide": generatedGuide, "reference_profile": comparison.Reference,
			"reference_chapter_numbers": comparison.ReferenceChapterNumbers,
			"reference_truncated":       comparison.ReferenceTruncated,
		}
	}
	var sceneContext *agents.ScenePlanPreview
	if req.UseScenePlan {
		maxScenes := req.MaxScenes
		if maxScenes == 0 {
			maxScenes = 8
		}
		if maxScenes < 1 || maxScenes > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_scenes must be between 1 and 10"})
			return
		}
		keyEvents := req.SceneKeyEvents
		if len(keyEvents) == 0 {
			keyEvents = chapter.KeyEvents
		}
		plan := h.svc.ScenePlanner.Plan(outline, keyEvents, req.TargetWordCount, maxScenes)
		sceneContext = &plan
		outline = mergePromptContexts(outline, h.svc.ScenePlanner.FormatForPrompt(plan), 12000)
	}
	preview, err := h.svc.Orchestrator.PreviewChapter(c.Request.Context(), orchestration.PreviewRequest{
		Outline:                 outline,
		StyleGuide:              styleGuide,
		MemoryContext:           memoryContext,
		PreviousChapter:         previous,
		NewCharacters:           req.NewCharacters,
		TargetWordCount:         req.TargetWordCount,
		MaxTotalTokens:          req.MaxTotalTokens,
		AutoRevise:              req.AutoRevise,
		RevisionThreshold:       req.RevisionThreshold,
		ChapterNumber:           chapterNumber,
		ExtractDataChanges:      req.ExtractDataChanges,
		ScenePlan:               sceneContext,
		AnalyzeAIGC:             req.AnalyzeAIGC,
		StyleReference:          styleReference,
		StyleReferenceChapters:  styleReferenceChapters,
		StyleReferenceTruncated: styleReferenceTruncated,
	})
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"project_id":      projectID,
		"chapter_number":  chapterNumber,
		"updated_at":      chapter.UpdatedAt,
		"workflow":        preview,
		"writing_context": writingContext,
		"rag_context":     ragContext,
		"style_context":   styleContext,
		"scene_context":   sceneContext,
		"preview":         true,
	})
}

func (h *Handler) ValidateChapter(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		NewCharacters []string `json:"new_characters"`
	}
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &req) {
			return
		}
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	if chapter.Content == nil || strings.TrimSpace(*chapter.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	previous, err := h.previousChapterTail(c.Request.Context(), projectID, chapterNumber, 0)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	issues := h.svc.Validator.ValidateChapter(*chapter.Content, previous, req.NewCharacters)
	c.JSON(http.StatusOK, gin.H{"project_id": projectID, "chapter_number": chapterNumber, "issues": issues, "summary": h.svc.Validator.Summary(issues), "updated_at": chapter.UpdatedAt, "preview": true})
}

func (h *Handler) PreviewChapterDiff(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Original        *string `json:"original"`
		Candidate       *string `json:"candidate" binding:"required"`
		MaxChangedRunes int     `json:"max_changed_runes"`
		ContextRunes    int     `json:"context_runes"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.MaxChangedRunes == 0 {
		req.MaxChangedRunes = 4000
	}
	if req.ContextRunes == 0 {
		req.ContextRunes = 120
	}
	if req.MaxChangedRunes < 100 || req.MaxChangedRunes > 20000 || req.ContextRunes < 0 || req.ContextRunes > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diff bounds"})
		return
	}
	if (req.Original != nil && utf8.RuneCountInString(*req.Original) > 100000) || utf8.RuneCountInString(*req.Candidate) > 100000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "original and candidate must not exceed 100000 characters"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	original := derefString(chapter.Content)
	if req.Original != nil {
		original = *req.Original
	}
	c.JSON(http.StatusOK, gin.H{
		"project_id": projectID, "chapter_number": chapterNumber,
		"diff":       h.svc.TextDiffer.Preview(original, *req.Candidate, req.MaxChangedRunes, req.ContextRunes),
		"updated_at": chapter.UpdatedAt, "preview": true,
	})
}

func (h *Handler) ValidateSceneCoverage(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Content         string   `json:"content"`
		Outline         string   `json:"outline"`
		KeyEvents       []string `json:"key_events"`
		TargetWordCount int      `json:"target_word_count"`
		MaxScenes       int      `json:"max_scenes"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if utf8.RuneCountInString(req.Content) > 100000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content must not exceed 100000 characters"})
		return
	}
	if req.TargetWordCount != 0 && (req.TargetWordCount < 100 || req.TargetWordCount > 20000) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be between 100 and 20000"})
		return
	}
	if req.MaxScenes < 0 || req.MaxScenes > 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_scenes must be between 1 and 10"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = strings.TrimSpace(derefString(chapter.Content))
	}
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	if utf8.RuneCountInString(content) > 100000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content must not exceed 100000 characters"})
		return
	}
	outline := strings.TrimSpace(req.Outline)
	if outline == "" {
		outline = strings.TrimSpace(derefString(chapter.Outline))
	}
	if len(req.KeyEvents) == 0 {
		req.KeyEvents = chapter.KeyEvents
	}
	if outline == "" && len(req.KeyEvents) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "outline or key_events is required"})
		return
	}
	if req.TargetWordCount == 0 {
		req.TargetWordCount = utf8.RuneCountInString(content)
	}
	if req.TargetWordCount < 100 || req.TargetWordCount > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be between 100 and 20000"})
		return
	}
	if req.MaxScenes == 0 {
		req.MaxScenes = 8
	}
	if req.MaxScenes < 1 || req.MaxScenes > 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_scenes must be between 1 and 10"})
		return
	}
	plan := h.svc.ScenePlanner.Plan(outline, req.KeyEvents, req.TargetWordCount, req.MaxScenes)
	coverage := h.svc.ScenePlanner.ValidateCoverage(content, plan)
	c.JSON(http.StatusOK, gin.H{
		"project_id": projectID, "chapter_number": chapterNumber,
		"plan": plan, "coverage": coverage,
		"updated_at": chapter.UpdatedAt, "preview": true,
	})
}

func (h *Handler) AnalyzeAIGC(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &req) {
			return
		}
	}
	if utf8.RuneCountInString(req.Content) > 100000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content must not exceed 100000 characters"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = strings.TrimSpace(derefString(chapter.Content))
	}
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	if utf8.RuneCountInString(content) > 100000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content must not exceed 100000 characters"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"project_id": projectID, "chapter_number": chapterNumber,
		"analysis":   h.svc.AIGCDetector.Detect(content),
		"updated_at": chapter.UpdatedAt, "preview": true,
	})
}

func (h *Handler) AnalyzeStyle(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Content         string `json:"content"`
		ComparePrevious bool   `json:"compare_previous"`
		SampleChapters  int    `json:"sample_chapters"`
	}
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &req) {
			return
		}
	}
	if utf8.RuneCountInString(req.Content) > 100000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content must not exceed 100000 characters"})
		return
	}
	if req.ComparePrevious && (req.SampleChapters < 0 || req.SampleChapters > 5) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sample_chapters must be between 1 and 5"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = strings.TrimSpace(derefString(chapter.Content))
	}
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	if utf8.RuneCountInString(content) > 100000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content must not exceed 100000 characters"})
		return
	}
	response := gin.H{
		"project_id": projectID, "chapter_number": chapterNumber,
		"profile":    h.svc.Style.Analyze(content),
		"updated_at": chapter.UpdatedAt, "preview": true,
	}
	if req.ComparePrevious {
		if req.SampleChapters == 0 {
			req.SampleChapters = 5
		}
		if req.SampleChapters < 1 || req.SampleChapters > 5 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sample_chapters must be between 1 and 5"})
			return
		}
		chapters, loadErr := h.chapterRepo.GetByProject(c.Request.Context(), projectID)
		if loadErr != nil {
			h.writeInternalError(c, loadErr)
			return
		}
		comparison := h.svc.Style.CompareWithPrevious(content, chapters, chapterNumber, req.SampleChapters, 50000)
		response["comparison"] = comparison
		response["style_guide"] = h.svc.Style.BuildGuide(comparison.Reference)
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) PreviewChapterDataChanges(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Content      string `json:"content"`
		KnownContext string `json:"known_context"`
	}
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &req) {
			return
		}
	}
	if utf8.RuneCountInString(req.Content) > 100000 || utf8.RuneCountInString(req.KnownContext) > 12000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content must not exceed 100000 characters and known_context must not exceed 12000 characters"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = strings.TrimSpace(derefString(chapter.Content))
	}
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	preview, err := h.svc.DataAgent.ExtractPreview(c.Request.Context(), chapterNumber, content, req.KnownContext)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if !h.requireServiceResult(c, preview != nil) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"project_id":     projectID,
		"chapter_number": chapterNumber,
		"changes":        preview,
		"updated_at":     chapter.UpdatedAt,
		"preview":        true,
	})
}

func (h *Handler) ContinueChapterPreview(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Instructions    string `json:"instructions" binding:"required"`
		TargetWordCount int    `json:"target_word_count"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.Instructions = strings.TrimSpace(req.Instructions)
	if req.Instructions == "" || utf8.RuneCountInString(req.Instructions) > 4000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "instructions must contain 1 to 4000 characters"})
		return
	}
	if req.TargetWordCount == 0 {
		req.TargetWordCount = 2000
	}
	if req.TargetWordCount < 100 || req.TargetWordCount > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be between 100 and 10000"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	if chapter.Content == nil || strings.TrimSpace(*chapter.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	continuation, err := h.svc.Writer.ContinueChapter(c.Request.Context(), *chapter.Content, req.Instructions, req.TargetWordCount)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	continuation = strings.TrimSpace(continuation)
	candidate := strings.TrimSpace(*chapter.Content) + "\n\n" + continuation
	c.JSON(http.StatusOK, gin.H{"project_id": projectID, "chapter_number": chapterNumber, "continuation": continuation, "candidate": candidate, "word_count": utf8.RuneCountInString(candidate), "updated_at": chapter.UpdatedAt, "preview": true})
}

func (h *Handler) RewriteChapterPreview(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Instructions string `json:"instructions" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.Instructions = strings.TrimSpace(req.Instructions)
	if req.Instructions == "" || utf8.RuneCountInString(req.Instructions) > 4000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "instructions must contain 1 to 4000 characters"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	if chapter.Content == nil || strings.TrimSpace(*chapter.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	candidate, err := h.svc.Writer.RewriteChapter(c.Request.Context(), *chapter.Content, req.Instructions)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	candidate = strings.TrimSpace(candidate)
	fallback := utf8.RuneCountInString(candidate) < 100
	if fallback {
		candidate = *chapter.Content
	}
	c.JSON(http.StatusOK, gin.H{"project_id": projectID, "chapter_number": chapterNumber, "candidate": candidate, "word_count": utf8.RuneCountInString(candidate), "fallback": fallback, "updated_at": chapter.UpdatedAt, "preview": true})
}

func (h *Handler) SummarizeChapterPreview(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	if chapter.Content == nil || strings.TrimSpace(*chapter.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	summary, err := h.svc.Writer.GenerateChapterSummaryDetail(c.Request.Context(), *chapter.Content)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if !h.requireServiceResult(c, summary != nil) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"project_id": projectID, "chapter_number": chapterNumber, "summary": summary, "updated_at": chapter.UpdatedAt, "preview": true})
}

func (h *Handler) AnalyzeReaderPerspective(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	if chapter.Content == nil || strings.TrimSpace(*chapter.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	result := h.svc.Reader.Analyze(*chapter.Content)
	c.JSON(http.StatusOK, gin.H{"project_id": projectID, "chapter_number": chapterNumber, "analysis": result, "updated_at": chapter.UpdatedAt, "preview": true})
}

func (h *Handler) ReviseChapterPreview(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	if chapter.Content == nil || strings.TrimSpace(*chapter.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	previous, err := h.previousChapterTail(c.Request.Context(), projectID, chapterNumber, 0)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	audit, err := h.svc.Critic.AuditChapter(c.Request.Context(), *chapter.Content, previous, derefString(chapter.Outline))
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if !h.requireServiceResult(c, audit != nil) {
		return
	}
	issues := make([]agents.RevisionIssue, 0, len(audit.Issues)+len(audit.Suggestions))
	for _, issue := range audit.Issues {
		issues = append(issues, agents.RevisionIssue{Problem: issue.Description, Severity: issue.Severity, Suggestion: "请修复该问题，并保持核心剧情不变"})
	}
	for _, suggestion := range audit.Suggestions {
		issues = append(issues, agents.RevisionIssue{Problem: suggestion, Severity: "medium", Suggestion: suggestion})
	}
	preview, err := h.svc.Reviser.Revise(c.Request.Context(), *chapter.Content, issues, audit.OverallScore)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if !h.requireServiceResult(c, preview != nil) {
		return
	}
	previewScore, _, scoreErr := h.svc.Critic.QuickAudit(c.Request.Context(), preview.Candidate)
	scoreStatus := "available"
	if scoreErr != nil {
		logSafeHandlerError(c, "revision_candidate_score_unavailable", scoreErr)
		scoreStatus = "unavailable"
	} else {
		preview.CandidateScore = &previewScore
	}
	c.JSON(http.StatusOK, gin.H{"project_id": projectID, "chapter_number": chapterNumber, "revision": preview, "candidate_score_status": scoreStatus, "updated_at": chapter.UpdatedAt, "preview": true})
}

// ========== Compatibility and Read-only Utility Handlers ==========

func (h *Handler) ExportProject(c *gin.Context) {
	projectID := c.Param("project_id")
	formatValues := c.Request.URL.Query()["format"]
	format := "txt"
	if len(formatValues) == 1 {
		format = strings.ToLower(strings.TrimSpace(formatValues[0]))
	}
	if format != "txt" && format != "md" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be txt or md", "code": "invalid_query_parameter"})
		return
	}

	project, err := h.projectRepo.GetByID(c.Request.Context(), projectID)
	if err != nil {
		h.writeReadError(c, err, "Project not found")
		return
	}
	if !h.requireReadResult(c, project != nil) {
		return
	}
	chapters, err := h.chapterRepo.GetByProject(c.Request.Context(), projectID)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	if len(chapters) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No chapters found"})
		return
	}

	var content strings.Builder
	mediaType := "text/plain; charset=utf-8"
	if format == "md" {
		mediaType = "text/markdown; charset=utf-8"
		content.WriteString("# " + project.Title + "\n\n")
		content.WriteString("**类型**: " + strings.Join(project.Genre, ", ") + "\n\n")
	} else {
		content.WriteString(project.Title + "\n\n")
		content.WriteString("类型：" + strings.Join(project.Genre, ", ") + "\n\n")
		content.WriteString(strings.Repeat("=", 50) + "\n\n")
	}
	for _, chapter := range chapters {
		title := derefString(chapter.Title)
		body := derefString(chapter.Content)
		if format == "md" {
			content.WriteString(fmt.Sprintf("## 第%d章 %s\n\n%s\n\n", chapter.ChapterNumber, title, body))
		} else {
			content.WriteString(fmt.Sprintf("第%d章 %s\n\n%s\n\n%s\n\n", chapter.ChapterNumber, title, body, strings.Repeat("=", 50)))
		}
	}

	filename := project.Title + "." + format
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	c.Data(http.StatusOK, mediaType, []byte(content.String()))
}

type GenerateChapterOutlinesRequest struct {
	Premise        string `json:"premise"`
	Genre          string `json:"genre"`
	TargetChapters int    `json:"target_chapters"`
	StartChapter   int    `json:"start_chapter"`
	EndChapter     int    `json:"end_chapter"`
}

func (h *Handler) GenerateChapterOutlines(c *gin.Context) {
	projectID := c.Param("project_id")
	var req GenerateChapterOutlinesRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.TargetChapters == 0 {
		req.TargetChapters = 20
	}
	if req.StartChapter == 0 {
		req.StartChapter = 1
	}
	if req.EndChapter == 0 {
		req.EndChapter = req.TargetChapters
	}
	if req.TargetChapters < 1 || req.TargetChapters > 100 || req.StartChapter < 1 || req.EndChapter < req.StartChapter || req.EndChapter > req.TargetChapters || req.EndChapter-req.StartChapter+1 > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter range; a preview batch may contain at most 20 chapters"})
		return
	}

	project, err := h.projectRepo.GetByID(c.Request.Context(), projectID)
	if err != nil {
		h.writeReadError(c, err, "Project not found")
		return
	}
	if !h.requireReadResult(c, project != nil) {
		return
	}
	if project.Outline == nil || strings.TrimSpace(*project.Outline) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project outline is required"})
		return
	}
	var structure agents.ThreeActStructure
	if err := json.Unmarshal([]byte(*project.Outline), &structure); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project outline is not a valid three-act structure"})
		return
	}
	premise := req.Premise
	if premise == "" {
		premise = derefString(project.Premise)
	}

	outlines := make([]agents.ChapterOutline, 0, req.EndChapter-req.StartChapter+1)
	for chapterNumber := req.StartChapter; chapterNumber <= req.EndChapter; chapterNumber++ {
		outline, err := h.svc.Architect.GenerateChapterOutline(c.Request.Context(), &structure, chapterNumber, req.TargetChapters, premise)
		if err != nil {
			h.writeInternalError(c, err)
			return
		}
		if !h.requireServiceResult(c, outline != nil) {
			return
		}
		outlines = append(outlines, *outline)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "generated_count": len(outlines), "outlines": outlines, "preview": true})
}

func (h *Handler) PreviewScenePlan(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Outline         string   `json:"outline"`
		KeyEvents       []string `json:"key_events"`
		TargetWordCount int      `json:"target_word_count"`
		MaxScenes       int      `json:"max_scenes"`
	}
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &req) {
			return
		}
	}
	if req.TargetWordCount == 0 {
		req.TargetWordCount = 5000
	}
	if req.TargetWordCount < 100 || req.TargetWordCount > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be between 100 and 20000"})
		return
	}
	if req.MaxScenes == 0 {
		req.MaxScenes = 8
	}
	if req.MaxScenes < 1 || req.MaxScenes > 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_scenes must be between 1 and 10"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	outline := strings.TrimSpace(req.Outline)
	if outline == "" {
		outline = strings.TrimSpace(derefString(chapter.Outline))
	}
	if len(req.KeyEvents) == 0 {
		req.KeyEvents = chapter.KeyEvents
	}
	if outline == "" && len(req.KeyEvents) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "outline or key_events is required"})
		return
	}
	plan := h.svc.ScenePlanner.Plan(outline, req.KeyEvents, req.TargetWordCount, req.MaxScenes)
	c.JSON(http.StatusOK, gin.H{
		"project_id": projectID, "chapter_number": chapterNumber,
		"plan": plan, "updated_at": chapter.UpdatedAt, "preview": true,
	})
}

func (h *Handler) PreviewChapterPrompt(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		Outline          string   `json:"outline"`
		StyleGuide       string   `json:"style_guide"`
		TargetWordCount  int      `json:"target_word_count"`
		UseContext       *bool    `json:"use_context"`
		UsePreviousStyle bool     `json:"use_previous_style"`
		StyleSamples     int      `json:"style_samples"`
		UseRAG           bool     `json:"use_rag"`
		RAGQuery         string   `json:"rag_query"`
		RAGMode          string   `json:"rag_mode"`
		RAGGenres        []string `json:"rag_genres"`
		RAGLimit         int      `json:"rag_limit"`
		UseScenePlan     bool     `json:"use_scene_plan"`
		SceneKeyEvents   []string `json:"scene_key_events"`
		MaxScenes        int      `json:"max_scenes"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.TargetWordCount == 0 {
		req.TargetWordCount = 5000
	}
	if req.TargetWordCount < 100 || req.TargetWordCount > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be between 100 and 20000"})
		return
	}
	if req.UsePreviousStyle && (req.StyleSamples < 0 || req.StyleSamples > 5) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "style_samples must be between 1 and 5"})
		return
	}
	if req.UseRAG && (req.RAGLimit < 0 || req.RAGLimit > 10) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rag_limit must be between 1 and 10"})
		return
	}
	if req.UseRAG && !validRAGMode(req.RAGMode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rag_mode must be hybrid, vector, or keyword"})
		return
	}
	if req.UseScenePlan && (req.MaxScenes < 0 || req.MaxScenes > 10) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_scenes must be between 1 and 10"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	outline := strings.TrimSpace(req.Outline)
	if outline == "" {
		outline = strings.TrimSpace(derefString(chapter.Outline))
	}
	if outline == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter outline is required"})
		return
	}
	if req.TargetWordCount == 0 {
		req.TargetWordCount = 5000
	}
	if req.TargetWordCount < 100 || req.TargetWordCount > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be between 100 and 20000"})
		return
	}
	previous, previousErr := h.previousChapterTail(c.Request.Context(), projectID, chapterNumber, 1000)
	if previousErr != nil {
		h.writeInternalError(c, previousErr)
		return
	}
	memoryContext := ""
	useContext := req.UseContext == nil || *req.UseContext
	var writingContext *agents.WritingContext
	if useContext {
		built, buildErr := h.buildWritingContext(c.Request.Context(), projectID, chapterNumber, 8000)
		if buildErr != nil {
			h.writeInternalError(c, buildErr)
			return
		}
		writingContext, memoryContext = &built, built.PromptContext
	}
	styleGuide := strings.TrimSpace(req.StyleGuide)
	var styleContext gin.H
	if req.UsePreviousStyle {
		if req.StyleSamples == 0 {
			req.StyleSamples = 5
		}
		if req.StyleSamples < 1 || req.StyleSamples > 5 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "style_samples must be between 1 and 5"})
			return
		}
		chapters, loadErr := h.chapterRepo.GetByProject(c.Request.Context(), projectID)
		if loadErr != nil {
			h.writeInternalError(c, loadErr)
			return
		}
		comparison := h.svc.Style.CompareWithPrevious("", chapters, chapterNumber, req.StyleSamples, 50000)
		guide := h.svc.Style.BuildGuide(comparison.Reference)
		styleGuide = mergePromptContexts(styleGuide, guide, 4000)
		styleContext = gin.H{"guide": guide, "reference_chapter_numbers": comparison.ReferenceChapterNumbers, "reference_truncated": comparison.ReferenceTruncated}
	}
	var ragContext gin.H
	if req.UseRAG {
		query := strings.TrimSpace(req.RAGQuery)
		if query == "" {
			query = outline
		}
		if req.RAGLimit == 0 {
			req.RAGLimit = 5
		}
		if req.RAGLimit < 1 || req.RAGLimit > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "rag_limit must be between 1 and 10"})
			return
		}
		mode := strings.ToLower(strings.TrimSpace(req.RAGMode))
		if mode == "" {
			mode = "hybrid"
		}
		results, searchErr := h.searchRAG(c.Request.Context(), mode, query, req.RAGGenres, req.RAGLimit)
		if searchErr != nil {
			h.writeInternalError(c, searchErr)
			return
		}
		evidence := h.svc.Retrieval.FormatResultsForPrompt(results, 800)
		memoryContext = mergePromptContexts(memoryContext, evidence, 12000)
		ragContext = gin.H{"query": query, "mode": mode, "results": results, "prompt_context": evidence}
	}
	var sceneContext *agents.ScenePlanPreview
	if req.UseScenePlan {
		if req.MaxScenes == 0 {
			req.MaxScenes = 8
		}
		if req.MaxScenes < 1 || req.MaxScenes > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_scenes must be between 1 and 10"})
			return
		}
		keyEvents := req.SceneKeyEvents
		if len(keyEvents) == 0 {
			keyEvents = chapter.KeyEvents
		}
		plan := h.svc.ScenePlanner.Plan(outline, keyEvents, req.TargetWordCount, req.MaxScenes)
		sceneContext = &plan
		outline = mergePromptContexts(outline, h.svc.ScenePlanner.FormatForPrompt(plan), 12000)
	}
	prompt := h.svc.Writer.BuildWritingPrompt(outline, styleGuide, memoryContext, previous, req.TargetWordCount)
	c.JSON(http.StatusOK, gin.H{
		"project_id": projectID, "chapter_number": chapterNumber,
		"prompt": prompt, "prompt_characters": utf8.RuneCountInString(prompt),
		"writing_context": writingContext, "style_context": styleContext, "rag_context": ragContext,
		"scene_context": sceneContext,
		"updated_at":    chapter.UpdatedAt, "preview": true,
	})
}

func (h *Handler) previousChapterTail(ctx context.Context, projectID string, chapterNumber, maxRunes int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if chapterNumber <= 1 {
		return "", nil
	}
	previous, err := h.chapterRepo.GetByNumber(ctx, projectID, chapterNumber-1)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if previous == nil || previous.Content == nil {
		return "", nil
	}
	runes := []rune(*previous.Content)
	if maxRunes > 0 && len(runes) > maxRunes {
		runes = runes[len(runes)-maxRunes:]
	}
	return string(runes), nil
}

func (h *Handler) GetChapterPrompt(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	targetValues := c.Request.URL.Query()["target_word_count"]
	targetValue := "5000"
	if len(targetValues) == 1 {
		targetValue = targetValues[0]
	}
	target, err := strconv.Atoi(targetValue)
	if err != nil || target < 1 || target > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_word_count must be an integer between 1 and 20000", "code": "invalid_query_parameter"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	previous, err := h.previousChapterTail(c.Request.Context(), projectID, chapterNumber, 500)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	prompt := h.svc.Writer.BuildWritingPrompt(derefString(chapter.Outline), "", "", previous, target)
	c.JSON(http.StatusOK, gin.H{"project_id": projectID, "chapter_number": chapterNumber, "prompt": prompt})
}

func (h *Handler) EvaluateChapter(c *gin.Context) {
	projectID := c.Param("project_id")
	var req EvaluationRequest
	if !bindJSON(c, &req) {
		return
	}
	if !validChapterNumber(req.ChapterNumber) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter_number must be an integer between 1 and 1000000", "code": "invalid_chapter_number"})
		return
	}
	result, err := h.evaluateChapter(c, projectID, req.ChapterNumber)
	if err != nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{"project_id": projectID, "chapter_number": req.ChapterNumber, "overall_score": result.OverallScore, "dimensions": result.Dimensions, "suggestions": result.Suggestions, "issues": result.Issues, "summary": result.Summary, "evaluated_at": time.Now()})
}

type EvaluationRequest struct {
	ChapterNumber int `json:"chapter_number"`
}

func (h *Handler) evaluateChapter(c *gin.Context, projectID string, chapterNumber int) (*agents.AuditResult, error) {
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return nil, err
	}
	if !h.requireReadResult(c, chapter != nil) {
		return nil, errNilRepositoryResult
	}
	if chapter.Content == nil || strings.TrimSpace(*chapter.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return nil, fmt.Errorf("chapter has no content")
	}
	previous, err := h.previousChapterTail(c.Request.Context(), projectID, chapterNumber, 0)
	if err != nil {
		h.writeInternalError(c, err)
		return nil, err
	}
	result, err := h.svc.Critic.AuditChapter(c.Request.Context(), *chapter.Content, previous, derefString(chapter.Outline))
	if err != nil {
		h.writeInternalError(c, err)
		return nil, err
	}
	if !h.requireServiceResult(c, result != nil) {
		return nil, errNilServiceResult
	}
	return result, nil
}

func (h *Handler) PolishChapterByNumber(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := mustInt(c.Param("chapter_number"))
	if chapterNumber < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}
	var req struct {
		StyleGuide string `json:"style_guide"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if utf8.RuneCountInString(req.StyleGuide) > 4000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "style_guide must not exceed 4000 characters"})
		return
	}
	chapter, err := h.chapterRepo.GetByNumber(c.Request.Context(), projectID, chapterNumber)
	if err != nil {
		h.writeReadError(c, err, "Chapter not found")
		return
	}
	if !h.requireReadResult(c, chapter != nil) {
		return
	}
	if chapter.Content == nil || strings.TrimSpace(*chapter.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}
	polished, err := h.svc.Editor.PolishChapter(c.Request.Context(), *chapter.Content, req.StyleGuide)
	if err != nil {
		h.writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"chapter_number": chapterNumber, "content": polished, "word_count": utf8.RuneCountInString(polished), "preview": true})
}

// ========== Helper Functions ==========

func (h *Handler) chapterToResponse(ch *models.Chapter) models.ChapterResponse {
	wordCount := 0
	if ch.Content != nil {
		wordCount = utf8.RuneCountInString(*ch.Content)
	}
	return models.ChapterResponse{
		ID:               ch.ID,
		ProjectID:        ch.ProjectID,
		ChapterNumber:    ch.ChapterNumber,
		Title:            ch.Title,
		Content:          ch.Content,
		Outline:          ch.Outline,
		Summary:          ch.Summary,
		WordCount:        wordCount,
		Foreshadowing:    ch.Foreshadowing,
		ConsistencyScore: ch.ConsistencyScore,
		CreatedAt:        ch.CreatedAt,
		UpdatedAt:        ch.UpdatedAt,
	}
}

func validRAGMode(mode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	return mode == "" || mode == "hybrid" || mode == "vector" || mode == "keyword"
}

func (h *Handler) searchRAG(ctx context.Context, mode, query string, genres []string, limit int) ([]retrieval.SearchResult, error) {
	switch mode {
	case "hybrid":
		return h.svc.Retrieval.HybridSearch(ctx, query, genres, limit)
	case "vector":
		return h.svc.Retrieval.VectorSearch(ctx, query, genres, limit)
	case "keyword":
		return h.svc.Retrieval.KeywordSearch(ctx, query, genres, limit)
	default:
		return nil, fmt.Errorf("rag_mode must be hybrid, vector, or keyword")
	}
}

func mergePromptContexts(base, evidence string, maxRunes int) string {
	base, evidence = strings.TrimSpace(base), strings.TrimSpace(evidence)
	combined := base
	if evidence != "" {
		if combined != "" {
			combined += "\n\n"
		}
		combined += evidence
	}
	runes := []rune(combined)
	if maxRunes > 0 && len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return string(runes)
}

func normalizeSearchRequest(c *gin.Context, req *models.SearchRequest) bool {
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return false
	}
	if utf8.RuneCountInString(req.Query) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query must not exceed 1000 characters"})
		return false
	}
	if req.Limit == 0 {
		req.Limit = 5
	}
	if req.Limit < 1 || req.Limit > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 50"})
		return false
	}
	return true
}

func searchResponse(query string, results []retrieval.SearchResult) models.SearchResponse {
	response := make([]models.SearchResultResponse, len(results))
	for i, result := range results {
		response[i] = models.SearchResultResponse{ID: result.ChunkID, BookTitle: result.BookTitle, Content: result.Content, Similarity: &result.Similarity}
	}
	return models.SearchResponse{Query: query, Total: len(results), Results: response}
}

func (h *Handler) buildWritingContext(ctx context.Context, projectID string, chapterNumber, maxRunes int) (agents.WritingContext, error) {
	project, err := h.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		return agents.WritingContext{}, fmt.Errorf("load project: %w", err)
	}
	if project == nil {
		return agents.WritingContext{}, errNilRepositoryResult
	}
	characters, err := h.characterRepo.GetByProject(ctx, projectID)
	if err != nil {
		return agents.WritingContext{}, fmt.Errorf("load characters: %w", err)
	}
	chapters, err := h.chapterRepo.GetByProject(ctx, projectID)
	if err != nil {
		return agents.WritingContext{}, fmt.Errorf("load chapters: %w", err)
	}
	return h.svc.Context.Build(project, characters, chapters, chapterNumber, maxRunes), nil
}

func (h *Handler) writeConflict(c *gin.Context, loadedUpdatedAt time.Time) {
	c.JSON(http.StatusConflict, gin.H{
		"error":             "resource was modified after it was loaded",
		"code":              "update_conflict",
		"loaded_updated_at": loadedUpdatedAt,
		"request_id":        requestIDFromContext(c),
	})
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mustInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || !validChapterNumber(n) {
		return -1
	}
	return n
}
