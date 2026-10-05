package handlers

import (
	"github.com/gin-gonic/gin"

	"novel-agent/internal/service"
)

func SetupRouter(svc *service.Service) *gin.Engine {
	r := gin.Default()

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

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
	{
		// Health check
		api.GET("/health", h.HealthCheck)

		// ========== Search Endpoints ==========
		api.POST("/search/vector", h.SearchVector)
		api.POST("/search/keyword", h.SearchKeyword)

		// ========== Project Endpoints ==========
		api.POST("/projects", h.CreateProject)
		api.GET("/projects", h.ListProjects)
		api.GET("/projects/:project_id", h.GetProject)
		api.DELETE("/projects/:project_id", h.DeleteProject)

		// ========== Character Endpoints ==========
		api.POST("/projects/:project_id/characters", h.CreateCharacter)
		api.GET("/projects/:project_id/characters", h.ListCharacters)
		api.GET("/projects/:project_id/characters/:character_id", h.GetCharacter)
		api.DELETE("/projects/:project_id/characters/:character_id", h.DeleteCharacter)

		// ========== Chapter Endpoints ==========
		api.POST("/projects/:project_id/chapters", h.CreateChapter)
		api.GET("/projects/:project_id/chapters", h.ListChapters)
		api.GET("/projects/:project_id/chapters/:chapter_number", h.GetChapter)
		api.PUT("/projects/:project_id/chapters/:chapter_number", h.UpdateChapter)
		api.DELETE("/projects/:project_id/chapters/:chapter_number", h.DeleteChapter)

		// ========== Outline Endpoints ==========
		api.GET("/projects/:project_id/outline", h.GetOutline)
		api.POST("/projects/:project_id/outline", h.CreateOutline)
		api.PUT("/projects/:project_id/outline", h.UpdateOutline)

		// ========== Chapter Outline Endpoints ==========
		api.GET("/projects/:project_id/chapters/:chapter_number/outline", h.GetChapterOutline)
		api.PUT("/projects/:project_id/chapters/:chapter_number/outline", h.UpdateChapterOutline)

		// ========== Creation Endpoints ==========
		api.POST("/projects/:project_id/generate-outline", h.GenerateOutline)
		api.POST("/projects/:project_id/chapters/:chapter_number/write", h.WriteChapter)
		api.POST("/projects/:project_id/audit", h.AuditChapter)
		api.POST("/projects/:project_id/polish", h.PolishChapter)
	}

	return r
}
