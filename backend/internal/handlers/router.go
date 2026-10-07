package handlers

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"novel-agent/internal/service"
)

const maxAPIRequestBodyBytes int64 = 1 << 20

func limitAPIRequestBody(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body exceeds 1 MiB limit", "code": "request_body_too_large"})
			return
		}
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBytes+1))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to read request body", "code": "invalid_request_body"})
			return
		}
		if int64(len(body)) > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body exceeds 1 MiB limit", "code": "request_body_too_large"})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Next()
	}
}

func SetupRouter(svc *service.Service) *gin.Engine {
	r := gin.Default()

	// The API is same-origin through the Vite proxy. Do not enable wildcard CORS.

	// Root endpoint
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"name":    "Novel Agent API",
			"version": "0.1.0",
			"docs":    "/docs",
			"health":  "/api/v1/health",
		})
	})

	// Create handler with service
	h := NewHandler(svc)

	api := r.Group("/api/v1")
	api.Use(limitAPIRequestBody(maxAPIRequestBodyBytes))
	{
		// Health and read-only schema compatibility checks
		api.GET("/health", h.HealthCheck)
		api.GET("/schema/compatibility", h.SchemaCompatibility)

		// ========== Search Endpoints ==========
		api.POST("/search/vector", h.SearchVector)
		api.POST("/search/keyword", h.SearchKeyword)
		api.POST("/search/hybrid", h.SearchHybrid)
		api.POST("/projects/:project_id/rag-context-preview", h.PreviewRAGContext)

		// ========== Project Endpoints ==========
		api.POST("/projects", h.CreateProject)
		api.GET("/projects", h.ListProjects)
		api.GET("/projects/:project_id", h.GetProject)
		api.PUT("/projects/:project_id", h.UpdateProject)
		api.GET("/projects/:project_id/export", h.ExportProject)

		// ========== Character Endpoints ==========
		api.POST("/projects/:project_id/characters", h.CreateCharacter)
		api.GET("/projects/:project_id/characters", h.ListCharacters)
		api.GET("/projects/:project_id/characters/:character_id", h.GetCharacter)
		api.PUT("/projects/:project_id/characters/:character_id", h.UpdateCharacter)

		// ========== Chapter Endpoints ==========
		api.POST("/projects/:project_id/chapters", h.CreateChapter)
		api.GET("/projects/:project_id/chapters", h.ListChapters)
		api.GET("/projects/:project_id/chapters/:chapter_number", h.GetChapter)
		api.PUT("/projects/:project_id/chapters/:chapter_number", h.UpdateChapter)
		api.GET("/projects/:project_id/chapters/:chapter_number/prompt", h.GetChapterPrompt)
		api.POST("/projects/:project_id/chapters/:chapter_number/prompt-preview", h.PreviewChapterPrompt)
		api.POST("/projects/:project_id/chapters/:chapter_number/scene-plan-preview", h.PreviewScenePlan)
		api.GET("/projects/:project_id/chapters/:chapter_number/context-preview", h.PreviewWritingContext)

		// ========== Outline Endpoints ==========
		api.GET("/projects/:project_id/outline", h.GetOutline)
		api.POST("/projects/:project_id/outline", h.CreateOutline)
		api.PUT("/projects/:project_id/outline", h.UpdateOutline)

		// ========== Chapter Outline Endpoints ==========
		api.GET("/projects/:project_id/chapters/:chapter_number/outline", h.GetChapterOutline)
		api.PUT("/projects/:project_id/chapters/:chapter_number/outline", h.UpdateChapterOutline)
		api.POST("/projects/:project_id/chapters/generate-outlines", h.GenerateChapterOutlines)

		// ========== Creation Endpoints ==========
		api.POST("/projects/:project_id/generate-outline", h.GenerateOutline)
		api.POST("/projects/:project_id/chapters/:chapter_number/write", h.WriteChapter)
		api.POST("/projects/:project_id/chapters/:chapter_number/workflow-preview", h.PreviewChapterWorkflow)
		api.POST("/projects/:project_id/chapters/:chapter_number/continue-preview", h.ContinueChapterPreview)
		api.POST("/projects/:project_id/chapters/:chapter_number/rewrite-preview", h.RewriteChapterPreview)
		api.GET("/projects/:project_id/chapters/:chapter_number/summary-preview", h.SummarizeChapterPreview)
		api.POST("/projects/:project_id/audit", h.AuditChapter)
		api.POST("/projects/:project_id/evaluate", h.EvaluateChapter)
		api.POST("/projects/:project_id/polish", h.PolishChapter)
		api.POST("/projects/:project_id/chapters/:chapter_number/polish", h.PolishChapterByNumber)
		api.POST("/projects/:project_id/chapters/:chapter_number/validate", h.ValidateChapter)
		api.POST("/projects/:project_id/chapters/:chapter_number/diff-preview", h.PreviewChapterDiff)
		api.POST("/projects/:project_id/chapters/:chapter_number/scene-coverage", h.ValidateSceneCoverage)
		api.POST("/projects/:project_id/chapters/:chapter_number/aigc-analysis", h.AnalyzeAIGC)
		api.POST("/projects/:project_id/chapters/:chapter_number/style-analysis", h.AnalyzeStyle)
		api.POST("/projects/:project_id/chapters/:chapter_number/data-changes-preview", h.PreviewChapterDataChanges)
		api.GET("/projects/:project_id/chapters/:chapter_number/reader-analysis", h.AnalyzeReaderPerspective)
		api.POST("/projects/:project_id/chapters/:chapter_number/revise-preview", h.ReviseChapterPreview)
	}

	return r
}
