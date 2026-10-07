package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"novel-agent/internal/service"
)

const (
	maxAPIRequestBodyBytes int64 = 1 << 20
	requestIDContextKey          = "request_id"
	requestIDHeader              = "X-Request-ID"
)

func ensureRequestID(c *gin.Context) string {
	if requestID, exists := c.Get(requestIDContextKey); exists {
		if value, ok := requestID.(string); ok && value != "" {
			return value
		}
	}
	requestID := strings.TrimSpace(c.GetHeader(requestIDHeader))
	if parsed, err := uuid.Parse(requestID); err != nil {
		requestID = uuid.NewString()
	} else {
		requestID = parsed.String()
	}
	c.Set(requestIDContextKey, requestID)
	c.Header(requestIDHeader, requestID)
	return requestID
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ensureRequestID(c)
		c.Next()
	}
}

func setAPISecurityHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Frame-Options", "DENY")
}

func safeAPIAccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		log.Printf("[access] request_id=%s method=%s route=%s status=%d duration_ms=%d", requestIDFromContext(c), c.Request.Method, route, c.Writer.Status(), time.Since(started).Milliseconds())
	}
}

func apiSecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		setAPISecurityHeaders(c)
		c.Next()
	}
}

func requestIDFromContext(c *gin.Context) string {
	return ensureRequestID(c)
}

func recoverAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				requestID := requestIDFromContext(c)
				log.Printf("[panic] request_id=%s method=%s path=%s recovered_type=%T\n%s", requestID, c.Request.Method, c.FullPath(), recovered, debug.Stack())
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error":      "internal server error",
					"code":       "internal_error",
					"error_id":   requestID,
					"request_id": requestID,
				})
			}
		}()
		c.Next()
	}
}

func validateAPIPathParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, parameter := range []struct {
			name string
			code string
		}{
			{name: "project_id", code: "invalid_project_id"},
			{name: "character_id", code: "invalid_character_id"},
		} {
			value := c.Param(parameter.name)
			if value == "" {
				continue
			}
			parsed, err := uuid.Parse(value)
			if err != nil || parsed == uuid.Nil || !strings.EqualFold(value, parsed.String()) {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error":      parameter.name + " must be a valid UUID",
					"code":       parameter.code,
					"request_id": requestIDFromContext(c),
				})
				return
			}
		}

		value := c.Param("chapter_number")
		if value != "" {
			chapterNumber, err := strconv.Atoi(value)
			if err != nil || chapterNumber < 1 || chapterNumber > 1000000 {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error":      "chapter_number must be an integer between 1 and 1000000",
					"code":       "invalid_chapter_number",
					"request_id": requestIDFromContext(c),
				})
				return
			}
		}
		c.Next()
	}
}

func requireJSONContentType() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			c.Next()
			return
		}
		mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || !strings.EqualFold(mediaType, "application/json") {
			c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{
				"error": "Content-Type must be application/json",
				"code":  "unsupported_media_type",
			})
			return
		}
		c.Next()
	}
}

func limitConcurrentAI(maxConcurrent int) gin.HandlerFunc {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	semaphore := make(chan struct{}, maxConcurrent)
	return func(c *gin.Context) {
		select {
		case semaphore <- struct{}{}:
			defer func() { <-semaphore }()
			c.Next()
		default:
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "AI request capacity exceeded",
				"code":  "ai_capacity_exceeded",
			})
		}
	}
}

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

const maxJSONNestingDepth = 64

var (
	errDuplicateJSONKey = errors.New("duplicate JSON object key")
	errJSONTooDeep      = errors.New("JSON nesting depth exceeded")
)

func rejectDuplicateJSONKeys(decoder *json.Decoder, depth int) error {
	if depth > maxJSONNestingDepth {
		return errJSONTooDeep
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("invalid JSON object key")
			}
			if _, exists := seen[key]; exists {
				return errDuplicateJSONKey
			}
			seen[key] = struct{}{}
			if err := rejectDuplicateJSONKeys(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := rejectDuplicateJSONKeys(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("invalid JSON delimiter")
	}
}

func validateSingleJSONObject() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			c.Next()
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to read request body", "code": "invalid_request_body"})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		decoder := json.NewDecoder(bytes.NewReader(body))
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "request body must contain exactly one JSON object", "code": "invalid_json_body"})
			return
		}
		var extra json.RawMessage
		if err := decoder.Decode(&extra); err != io.EOF {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "request body must contain exactly one JSON object", "code": "invalid_json_body"})
			return
		}
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "request body must contain exactly one JSON object", "code": "invalid_json_body"})
			return
		}
		if err := rejectDuplicateJSONKeys(json.NewDecoder(bytes.NewReader(trimmed)), 1); err != nil {
			code, message := "invalid_json_body", "request body contains invalid JSON"
			switch {
			case errors.Is(err, errDuplicateJSONKey):
				code, message = "duplicate_json_key", "request body contains duplicate JSON object keys"
			case errors.Is(err, errJSONTooDeep):
				code, message = "json_nesting_too_deep", "request body exceeds the JSON nesting depth limit"
			}
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": message, "code": code})
			return
		}
		c.Next()
	}
}

func writeAPIRoutingError(c *gin.Context, status int, code, message string) {
	requestID := ensureRequestID(c)
	setAPISecurityHeaders(c)
	c.AbortWithStatusJSON(status, gin.H{
		"error":      message,
		"code":       code,
		"request_id": requestID,
	})
	log.Printf("[access] request_id=%s method=%s route=unmatched status=%d duration_ms=0", requestID, c.Request.Method, status)
}

func SetupRouter(svc *service.Service) *gin.Engine {
	// API DTOs are explicit contracts. Reject misspelled or unexpected fields
	// instead of silently falling back to zero values or safety defaults.
	gin.EnableJsonDecoderDisallowUnknownFields()
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/") || c.Request.URL.Path == "/api/v1" {
			writeAPIRoutingError(c, http.StatusNotFound, "route_not_found", "API route not found")
			return
		}
		c.Status(http.StatusNotFound)
	})
	r.NoMethod(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/") || c.Request.URL.Path == "/api/v1" {
			writeAPIRoutingError(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		c.Status(http.StatusMethodNotAllowed)
	})

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
	maxConcurrentAI := 4
	if svc != nil && svc.Config != nil && svc.Config.Server.MaxConcurrentAI > 0 {
		maxConcurrentAI = svc.Config.Server.MaxConcurrentAI
	}
	aiLimit := limitConcurrentAI(maxConcurrentAI)

	api := r.Group("/api/v1")
	api.Use(requestIDMiddleware(), apiSecurityHeaders(), safeAPIAccessLog(), recoverAPI(), validateAPIPathParams(), requireJSONContentType(), limitAPIRequestBody(maxAPIRequestBodyBytes), validateSingleJSONObject())
	{
		// Health and read-only schema compatibility checks
		api.GET("/health", h.HealthCheck)
		api.GET("/schema/compatibility", h.SchemaCompatibility)

		// ========== Search Endpoints ==========
		api.POST("/search/vector", aiLimit, h.SearchVector)
		api.POST("/search/keyword", h.SearchKeyword)
		api.POST("/search/hybrid", aiLimit, h.SearchHybrid)
		api.POST("/projects/:project_id/rag-context-preview", aiLimit, h.PreviewRAGContext)

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
		api.POST("/projects/:project_id/chapters/:chapter_number/prompt-preview", aiLimit, h.PreviewChapterPrompt)
		api.POST("/projects/:project_id/chapters/:chapter_number/scene-plan-preview", h.PreviewScenePlan)
		api.GET("/projects/:project_id/chapters/:chapter_number/context-preview", h.PreviewWritingContext)

		// ========== Outline Endpoints ==========
		api.GET("/projects/:project_id/outline", h.GetOutline)
		api.POST("/projects/:project_id/outline", h.CreateOutline)
		api.PUT("/projects/:project_id/outline", h.UpdateOutline)

		// ========== Chapter Outline Endpoints ==========
		api.GET("/projects/:project_id/chapters/:chapter_number/outline", h.GetChapterOutline)
		api.PUT("/projects/:project_id/chapters/:chapter_number/outline", h.UpdateChapterOutline)
		api.POST("/projects/:project_id/chapters/generate-outlines", aiLimit, h.GenerateChapterOutlines)

		// ========== Creation Endpoints ==========
		api.POST("/projects/:project_id/generate-outline", aiLimit, h.GenerateOutline)
		api.POST("/projects/:project_id/chapters/:chapter_number/write", aiLimit, h.WriteChapter)
		api.POST("/workflow-budget-preview", h.PreviewWorkflowTokenBudget)
		api.POST("/projects/:project_id/chapters/:chapter_number/workflow-preview", aiLimit, h.PreviewChapterWorkflow)
		api.POST("/projects/:project_id/chapters/:chapter_number/continue-preview", aiLimit, h.ContinueChapterPreview)
		api.POST("/projects/:project_id/chapters/:chapter_number/rewrite-preview", aiLimit, h.RewriteChapterPreview)
		api.GET("/projects/:project_id/chapters/:chapter_number/summary-preview", aiLimit, h.SummarizeChapterPreview)
		api.POST("/projects/:project_id/audit", aiLimit, h.AuditChapter)
		api.POST("/projects/:project_id/evaluate", aiLimit, h.EvaluateChapter)
		api.POST("/projects/:project_id/polish", aiLimit, h.PolishChapter)
		api.POST("/projects/:project_id/chapters/:chapter_number/polish", aiLimit, h.PolishChapterByNumber)
		api.POST("/projects/:project_id/chapters/:chapter_number/validate", h.ValidateChapter)
		api.POST("/projects/:project_id/chapters/:chapter_number/diff-preview", h.PreviewChapterDiff)
		api.POST("/projects/:project_id/chapters/:chapter_number/scene-coverage", h.ValidateSceneCoverage)
		api.POST("/projects/:project_id/chapters/:chapter_number/aigc-analysis", h.AnalyzeAIGC)
		api.POST("/projects/:project_id/chapters/:chapter_number/style-analysis", h.AnalyzeStyle)
		api.POST("/projects/:project_id/chapters/:chapter_number/data-changes-preview", aiLimit, h.PreviewChapterDataChanges)
		api.GET("/projects/:project_id/chapters/:chapter_number/reader-analysis", h.AnalyzeReaderPerspective)
		api.POST("/projects/:project_id/chapters/:chapter_number/revise-preview", aiLimit, h.ReviseChapterPreview)
	}

	return r
}
