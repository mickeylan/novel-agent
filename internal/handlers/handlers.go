package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"novel-agent/internal/models"
	"novel-agent/internal/repository"
	"novel-agent/internal/service"
	"novel-agent/pkg/database"
)

type Handler struct {
	projectRepo   *repository.ProjectRepository
	chapterRepo   *repository.ChapterRepository
	characterRepo *repository.CharacterRepository
	chunkRepo     *repository.BookChunkRepository
	svc           *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	return &Handler{
		projectRepo:   repository.NewProjectRepository(),
		chapterRepo:   repository.NewChapterRepository(),
		characterRepo: repository.NewCharacterRepository(),
		chunkRepo:     repository.NewBookChunkRepository(),
		svc:          svc,
	}
}

// ========== Health Check ==========

func (h *Handler) HealthCheck(c *gin.Context) {
	dbStatus := "connected"
	if err := database.Ping(); err != nil {
		dbStatus = "error: " + err.Error()
	}

	c.JSON(http.StatusOK, models.HealthResponse{
		Status:   "healthy",
		Version:  "0.1.0",
		Database: dbStatus,
	})
}

// ========== Project Handlers ==========

func (h *Handler) CreateProject(c *gin.Context) {
	var req models.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	project := &models.StoryProject{
		Title:        req.Title,
		Genre:        req.Genres,
		WorldSetting:  req.WorldSetting,
		Premise:       req.Premise,
		Status:        "draft",
	}

	id, err := h.projectRepo.Create(project)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	created, _ := h.projectRepo.GetByID(id)
	c.JSON(http.StatusCreated, models.ProjectResponse{
		ID:          created.ID,
		Title:       created.Title,
		Genres:       created.Genre,
		WorldSetting: created.WorldSetting,
		Premise:      created.Premise,
		Status:       created.Status,
		CreatedAt:    created.CreatedAt,
		UpdatedAt:    created.UpdatedAt,
	})
}

func (h *Handler) ListProjects(c *gin.Context) {
	projects, err := h.projectRepo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]models.ProjectResponse, len(projects))
	for i, p := range projects {
		response[i] = models.ProjectResponse{
			ID:          p.ID,
			Title:       p.Title,
			Genres:      p.Genre,
			WorldSetting: p.WorldSetting,
			Premise:      p.Premise,
			Status:       p.Status,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		}
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetProject(c *gin.Context) {
	projectID := c.Param("project_id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	c.JSON(http.StatusOK, models.ProjectResponse{
		ID:          project.ID,
		Title:       project.Title,
		Genres:      project.Genre,
		WorldSetting: project.WorldSetting,
		Premise:      project.Premise,
		Status:       project.Status,
		CreatedAt:   project.CreatedAt,
		UpdatedAt:   project.UpdatedAt,
	})
}

func (h *Handler) DeleteProject(c *gin.Context) {
	projectID := c.Param("project_id")

	_, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	if err := h.projectRepo.Delete(projectID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Project deleted successfully"})
}

// ========== Character Handlers ==========

func (h *Handler) CreateCharacter(c *gin.Context) {
	projectID := c.Param("project_id")

	var req models.CreateCharacterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	relJSON, _ := json.Marshal(req.Relationships)
	character := &models.Character{
		ProjectID:    projectID,
		Name:         req.Name,
		Traits:       req.Traits,
		Relationships: relJSON,
		ArcSummary:   req.ArcSummary,
		CurrentState: req.CurrentState,
	}

	id, err := h.characterRepo.Create(character)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, models.CharacterResponse{
		ID:            id,
		Name:          req.Name,
		Traits:        req.Traits,
		Relationships: req.Relationships,
		ArcSummary:   req.ArcSummary,
		CurrentState:  req.CurrentState,
	})
}

func (h *Handler) ListCharacters(c *gin.Context) {
	projectID := c.Param("project_id")

	characters, err := h.characterRepo.GetByProject(projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]models.CharacterResponse, len(characters))
	for i, ch := range characters {
		var rel map[string]string
		json.Unmarshal(ch.Relationships, &rel)
		response[i] = models.CharacterResponse{
			ID:            ch.ID,
			Name:          ch.Name,
			Traits:        ch.Traits,
			Relationships: rel,
			ArcSummary:    ch.ArcSummary,
			CurrentState:  ch.CurrentState,
		}
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetCharacter(c *gin.Context) {
	characterID := c.Param("character_id")

	character, err := h.characterRepo.GetByID(characterID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Character not found"})
		return
	}

	var rel map[string]string
	json.Unmarshal(character.Relationships, &rel)

	c.JSON(http.StatusOK, models.CharacterResponse{
		ID:            character.ID,
		Name:          character.Name,
		Traits:        character.Traits,
		Relationships: rel,
		ArcSummary:    character.ArcSummary,
		CurrentState:  character.CurrentState,
	})
}

func (h *Handler) DeleteCharacter(c *gin.Context) {
	characterID := c.Param("character_id")

	if err := h.characterRepo.Delete(characterID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Character deleted successfully"})
}

// ========== Chapter Handlers ==========

func (h *Handler) CreateChapter(c *gin.Context) {
	projectID := c.Param("project_id")

	var req models.CreateChapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chapter := &models.Chapter{
		ProjectID:     projectID,
		ChapterNumber: req.ChapterNumber,
		Outline:       req.Outline,
		Content:       req.Content,
		Status:        "draft",
	}

	id, err := h.chapterRepo.Create(chapter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	created, _ := h.chapterRepo.GetByID(id)
	c.JSON(http.StatusCreated, h.chapterToResponse(created))
}

func (h *Handler) ListChapters(c *gin.Context) {
	projectID := c.Param("project_id")

	chapters, err := h.chapterRepo.GetByProject(projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	chapter, err := h.chapterRepo.GetByNumber(projectID, mustInt(chapterNumber))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Chapter not found"})
		return
	}

	c.JSON(http.StatusOK, h.chapterToResponse(chapter))
}

func (h *Handler) UpdateChapter(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	var req models.UpdateChapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.chapterRepo.GetByNumber(projectID, mustInt(chapterNumber))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Chapter not found"})
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

	if err := h.chapterRepo.Update(projectID, mustInt(chapterNumber), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, _ := h.chapterRepo.GetByNumber(projectID, mustInt(chapterNumber))
	c.JSON(http.StatusOK, h.chapterToResponse(updated))
}

func (h *Handler) DeleteChapter(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	if err := h.chapterRepo.Delete(projectID, mustInt(chapterNumber)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Chapter deleted successfully"})
}

// ========== Creation Handlers ==========

type GenerateOutlineRequest struct {
	Premise        string  `json:"premise"`
	Genre          string  `json:"genre"`
	TargetChapters int     `json:"target_chapters"`
}

func (h *Handler) GenerateOutline(c *gin.Context) {
	projectID := c.Param("project_id")

	var req GenerateOutlineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	// Generate three-act structure
	structure, err := h.svc.Architect.GenerateThreeActStructure(ctx, req.Premise, req.Genre)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Save to database
	outlineJSON, _ := structure.ToJSON()
	if err := h.projectRepo.Update(projectID, map[string]interface{}{
		"premise": req.Premise,
		"outline": outlineJSON,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project_id": projectID,
		"premise":    req.Premise,
		"structure":   structure,
	})
}

type WriteChapterRequest struct {
	Outline        string `json:"outline"`
	StyleGuide     string `json:"style_guide"`
	TargetWordCount int    `json:"target_word_count"`
}

func (h *Handler) WriteChapter(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	var req WriteChapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	targetWordCount := req.TargetWordCount
	if targetWordCount == 0 {
		targetWordCount = 5000
	}

	ctx := context.Background()

	// Get previous chapter for context
	prevChapter := ""
	chapterNum := mustInt(chapterNumber)
	if chapterNum > 1 {
		if prev, err := h.chapterRepo.GetByNumber(projectID, chapterNum-1); err == nil && prev != nil && prev.Content != nil {
			// Get last 500 chars of previous chapter
			prevChapter = *prev.Content
			if len(prevChapter) > 500 {
				prevChapter = prevChapter[len(prevChapter)-500:]
			}
		}
	}

	// Write chapter
	content, err := h.svc.Writer.WriteChapter(ctx, req.Outline, req.StyleGuide, "", prevChapter, targetWordCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Save to database
	h.chapterRepo.Update(projectID, chapterNum, map[string]interface{}{
		"content":    content,
		"word_count": len(content),
	})

	c.JSON(http.StatusOK, gin.H{
		"chapter_number": chapterNum,
		"content":       content,
		"word_count":    len(content),
	})
}

type AuditChapterRequest struct {
	ChapterNumber int `json:"chapter_number"`
}

func (h *Handler) AuditChapter(c *gin.Context) {
	projectID := c.Param("project_id")

	var req AuditChapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chapter, err := h.chapterRepo.GetByNumber(projectID, req.ChapterNumber)
	if err != nil || chapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Chapter not found"})
		return
	}

	if chapter.Content == nil || *chapter.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}

	ctx := context.Background()

	// Get previous chapter
	var prevChapter string
	if req.ChapterNumber > 1 {
		if prev, err := h.chapterRepo.GetByNumber(projectID, req.ChapterNumber-1); err == nil && prev != nil && prev.Content != nil {
			prevChapter = *prev.Content
		}
	}

	// Audit
	result, err := h.svc.Critic.AuditChapter(ctx, *chapter.Content, prevChapter, derefString(chapter.Outline))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

type PolishChapterRequest struct {
	ChapterNumber int `json:"chapter_number"`
	StyleGuide    string `json:"style_guide"`
}

func (h *Handler) PolishChapter(c *gin.Context) {
	projectID := c.Param("project_id")

	var req PolishChapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chapter, err := h.chapterRepo.GetByNumber(projectID, req.ChapterNumber)
	if err != nil || chapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Chapter not found"})
		return
	}

	if chapter.Content == nil || *chapter.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter has no content"})
		return
	}

	ctx := context.Background()

	// Polish
	polished, err := h.svc.Editor.PolishChapter(ctx, *chapter.Content, req.StyleGuide)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Save
	h.chapterRepo.Update(projectID, req.ChapterNumber, map[string]interface{}{
		"content": polished,
	})

	c.JSON(http.StatusOK, gin.H{
		"chapter_number": req.ChapterNumber,
		"content":        polished,
		"word_count":     len(polished),
	})
}

// ========== Outline Handlers ==========

func (h *Handler) GetOutline(c *gin.Context) {
	projectID := c.Param("project_id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project_id":    project.ID,
		"premise":       derefString(project.Premise),
		"world_setting":  derefString(project.WorldSetting),
		"outline":        derefString(project.Premise),
	})
}

func (h *Handler) UpdateOutline(c *gin.Context) {
	projectID := c.Param("project_id")

	var req struct {
		Premise     *string `json:"premise"`
		WorldSetting *string `json:"world_setting"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Premise != nil {
		updates["premise"] = *req.Premise
	}
	if req.WorldSetting != nil {
		updates["world_setting"] = *req.WorldSetting
	}

	if err := h.projectRepo.Update(projectID, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Outline updated successfully"})
}

func (h *Handler) CreateOutline(c *gin.Context) {
	h.UpdateOutline(c) // For now, same as update
}

func (h *Handler) GetChapterOutline(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	chapter, err := h.chapterRepo.GetByNumber(projectID, mustInt(chapterNumber))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Chapter not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project_id":     projectID,
		"chapter_number": chapter.ChapterNumber,
		"title":          derefString(chapter.Title),
		"outline":        derefString(chapter.Outline),
	})
}

func (h *Handler) UpdateChapterOutline(c *gin.Context) {
	projectID := c.Param("project_id")
	chapterNumber := c.Param("chapter_number")

	var req struct {
		Title   *string `json:"title"`
		Outline *string `json:"outline"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Outline != nil {
		updates["outline"] = *req.Outline
	}

	if err := h.chapterRepo.Update(projectID, mustInt(chapterNumber), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	chapter, _ := h.chapterRepo.GetByNumber(projectID, mustInt(chapterNumber))
	c.JSON(http.StatusOK, gin.H{
		"project_id":     projectID,
		"chapter_number": chapter.ChapterNumber,
		"title":          derefString(chapter.Title),
		"outline":        derefString(chapter.Outline),
	})
}

// ========== Search Handlers ==========

func (h *Handler) SearchVector(c *gin.Context) {
	var req models.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Limit == 0 {
		req.Limit = 5
	}

	ctx := context.Background()
	results, err := h.svc.Retrieval.VectorSearch(ctx, req.Query, req.Genres, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]models.SearchResultResponse, len(results))
	for i, r := range results {
		response[i] = models.SearchResultResponse{
			ID:        r.ChunkID,
			BookTitle: r.BookTitle,
			Content:   r.Content,
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
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Limit == 0 {
		req.Limit = 5
	}

	ctx := context.Background()
	results, err := h.svc.Retrieval.KeywordSearch(ctx, req.Query, req.Genres, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]models.SearchResultResponse, len(results))
	for i, r := range results {
		response[i] = models.SearchResultResponse{
			ID:        r.ChunkID,
			BookTitle: r.BookTitle,
			Content:   r.Content,
			Similarity: &r.Similarity,
		}
	}

	c.JSON(http.StatusOK, models.SearchResponse{
		Query:   req.Query,
		Total:   len(results),
		Results: response,
	})
}

// ========== Helper Functions ==========

func (h *Handler) chapterToResponse(ch *models.Chapter) models.ChapterResponse {
	wordCount := 0
	if ch.Content != nil {
		wordCount = len(*ch.Content)
	}
	return models.ChapterResponse{
		ID:               ch.ID,
		ProjectID:        ch.ProjectID,
		ChapterNumber:    ch.ChapterNumber,
		Title:           ch.Title,
		Content:         ch.Content,
		Outline:         ch.Outline,
		Summary:         ch.Summary,
		WordCount:       wordCount,
		Foreshadowing:   ch.Foreshadowing,
		ConsistencyScore: ch.ConsistencyScore,
		CreatedAt:       ch.CreatedAt,
		UpdatedAt:       ch.UpdatedAt,
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mustInt(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
