package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"novel-agent/internal/service"
)

func TestInternalErrorResponseHidesDetailsAndProvidesCorrelationID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(&service.Service{})
	router.GET("/failure", func(c *gin.Context) {
		handler.writeInternalError(c, fmt.Errorf("postgres connection secret detail"))
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/failure", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("returned %d, want 500", response.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "internal_error" || body["error"] != "internal server error" || body["error_id"] == "" {
		t.Fatalf("unexpected internal error response: %#v", body)
	}
	if strings.Contains(response.Body.String(), "postgres") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("internal details leaked: %s", response.Body.String())
	}
}

func TestHealthCheckHidesDatabaseErrorDetails(t *testing.T) {
	router := SetupRouter(&service.Service{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("health returned %d, want 503", response.Code)
	}
	body := response.Body.String()
	if !bytes.Contains(response.Body.Bytes(), []byte(`"status":"unhealthy"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"database":"unavailable"`)) {
		t.Fatalf("unexpected health response: %s", body)
	}
	if bytes.Contains(response.Body.Bytes(), []byte("not initialized")) || bytes.Contains(response.Body.Bytes(), []byte("error:")) {
		t.Fatalf("health leaked internal database error: %s", body)
	}
}

func TestCompatibilityRoutesAreRegistered(t *testing.T) {
	router := SetupRouter(&service.Service{})
	want := map[string]bool{
		"POST /api/v1/search/hybrid":                                                      false,
		"POST /api/v1/projects/:project_id/rag-context-preview":                           false,
		"GET /api/v1/schema/compatibility":                                                false,
		"GET /api/v1/projects/:project_id/export":                                         false,
		"POST /api/v1/projects/:project_id/chapters/generate-outlines":                    false,
		"GET /api/v1/projects/:project_id/chapters/:chapter_number/prompt":                false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/prompt-preview":       false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/scene-plan-preview":   false,
		"GET /api/v1/projects/:project_id/chapters/:chapter_number/context-preview":       false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/workflow-preview":     false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/continue-preview":     false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/rewrite-preview":      false,
		"GET /api/v1/projects/:project_id/chapters/:chapter_number/summary-preview":       false,
		"POST /api/v1/projects/:project_id/evaluate":                                      false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/polish":               false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/validate":             false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/diff-preview":         false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/scene-coverage":       false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/aigc-analysis":        false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/style-analysis":       false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/data-changes-preview": false,
		"GET /api/v1/projects/:project_id/chapters/:chapter_number/reader-analysis":       false,
		"POST /api/v1/projects/:project_id/chapters/:chapter_number/revise-preview":       false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Errorf("route not registered: %s", route)
		}
	}
}

func TestMergePromptContextsBoundsUnicode(t *testing.T) {
	merged := mergePromptContexts("项目上下文", "检索证据", 7)
	if len([]rune(merged)) != 7 {
		t.Fatalf("expected seven runes, got %d", len([]rune(merged)))
	}
}

func TestSearchRejectsUnsafeLimitsAndBlankQueries(t *testing.T) {
	router := SetupRouter(&service.Service{})
	bodies := []string{
		`{"query":"   ","limit":5}`,
		`{"query":"test","limit":51}`,
		`{"query":"test","limit":-1}`,
	}
	for _, body := range bodies {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/search/keyword", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body %s returned %d, want 400", body, response.Code)
		}
	}
}

func TestDiffPreviewRejectsInvalidRequestsBeforeDatabaseAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "invalid chapter", path: "/api/v1/projects/project-id/chapters/0/diff-preview", body: `{"candidate":"候选"}`},
		{name: "missing candidate", path: "/api/v1/projects/project-id/chapters/1/diff-preview", body: `{}`},
		{name: "small change bound", path: "/api/v1/projects/project-id/chapters/1/diff-preview", body: `{"candidate":"候选","max_changed_runes":99}`},
		{name: "large context bound", path: "/api/v1/projects/project-id/chapters/1/diff-preview", body: `{"candidate":"候选","context_runes":501}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("returned %d, want 400; body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCreationPreviewsRejectInvalidOptionsBeforeDatabaseAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "scene target", path: "/api/v1/projects/project-id/chapters/1/scene-plan-preview", body: `{"target_word_count":99}`},
		{name: "scene count", path: "/api/v1/projects/project-id/chapters/1/scene-plan-preview", body: `{"max_scenes":11}`},
		{name: "prompt target", path: "/api/v1/projects/project-id/chapters/1/prompt-preview", body: `{"target_word_count":20001}`},
		{name: "prompt style samples", path: "/api/v1/projects/project-id/chapters/1/prompt-preview", body: `{"use_previous_style":true,"style_samples":6}`},
		{name: "prompt rag limit", path: "/api/v1/projects/project-id/chapters/1/prompt-preview", body: `{"use_rag":true,"rag_limit":11}`},
		{name: "prompt rag mode", path: "/api/v1/projects/project-id/chapters/1/prompt-preview", body: `{"use_rag":true,"rag_mode":"unsupported"}`},
		{name: "workflow target", path: "/api/v1/projects/project-id/chapters/1/workflow-preview", body: `{"target_word_count":50}`},
		{name: "workflow threshold", path: "/api/v1/projects/project-id/chapters/1/workflow-preview", body: `{"revision_threshold":11}`},
		{name: "workflow scenes", path: "/api/v1/projects/project-id/chapters/1/workflow-preview", body: `{"use_scene_plan":true,"max_scenes":11}`},
		{name: "coverage target", path: "/api/v1/projects/project-id/chapters/1/scene-coverage", body: `{"target_word_count":99}`},
		{name: "coverage scenes", path: "/api/v1/projects/project-id/chapters/1/scene-coverage", body: `{"max_scenes":11}`},
		{name: "style samples", path: "/api/v1/projects/project-id/chapters/1/style-analysis", body: `{"compare_previous":true,"sample_chapters":6}`},
		{name: "continue blank instructions", path: "/api/v1/projects/project-id/chapters/1/continue-preview", body: `{"instructions":"   "}`},
		{name: "continue target", path: "/api/v1/projects/project-id/chapters/1/continue-preview", body: `{"instructions":"继续","target_word_count":99}`},
		{name: "rewrite blank instructions", path: "/api/v1/projects/project-id/chapters/1/rewrite-preview", body: `{"instructions":"   "}`},
		{name: "write invalid chapter", path: "/api/v1/projects/project-id/chapters/0/write", body: `{}`},
		{name: "write target", path: "/api/v1/projects/project-id/chapters/1/write", body: `{"target_word_count":99}`},
		{name: "audit invalid chapter", path: "/api/v1/projects/project-id/audit", body: `{"chapter_number":0}`},
		{name: "evaluate invalid chapter", path: "/api/v1/projects/project-id/evaluate", body: `{"chapter_number":0}`},
		{name: "polish invalid chapter", path: "/api/v1/projects/project-id/polish", body: `{"chapter_number":0}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("returned %d, want 400; body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAPIRequestBodyLimitRejectsKnownAndUnknownLengths(t *testing.T) {
	router := SetupRouter(&service.Service{})
	body := bytes.Repeat([]byte("x"), int(maxAPIRequestBodyBytes)+1)
	for _, unknownLength := range []bool{false, true} {
		name := "known length"
		if unknownLength {
			name = "unknown length"
		}
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/search/keyword", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if unknownLength {
				req.ContentLength = -1
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("returned %d, want 413; body=%s", response.Code, response.Body.String())
			}
			if !bytes.Contains(response.Body.Bytes(), []byte(`"code":"request_body_too_large"`)) {
				t.Fatalf("missing stable error code: %s", response.Body.String())
			}
		})
	}
}

func TestAPIRequestBodyLimitPreservesAllowedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(limitAPIRequestBody(maxAPIRequestBodyBytes))
	router.POST("/echo", func(c *gin.Context) {
		var req struct {
			Text string `json:"text" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, req)
	})
	req := httptest.NewRequest(http.MethodPost, "/echo", bytes.NewBufferString(`{"text":"中文🙂"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("中文🙂")) {
		t.Fatalf("allowed JSON was not preserved: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestUpdatesRequireLoadedVersion(t *testing.T) {
	router := SetupRouter(&service.Service{})
	paths := []string{
		"/api/v1/projects/project-id",
		"/api/v1/projects/project-id/chapters/1",
		"/api/v1/projects/project-id/outline",
		"/api/v1/projects/project-id/chapters/1/outline",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(`{"title":"stale update"}`))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("PUT %s without updated_at returned %d, want 400", path, response.Code)
		}
	}
}

func TestDestructiveRoutesAreNotRegistered(t *testing.T) {
	router := SetupRouter(&service.Service{})
	paths := []string{
		"/api/v1/projects/00000000-0000-0000-0000-000000000000",
		"/api/v1/projects/00000000-0000-0000-0000-000000000000/characters/00000000-0000-0000-0000-000000000000",
		"/api/v1/projects/00000000-0000-0000-0000-000000000000/chapters/1",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodDelete, path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("DELETE %s returned %d, want 404", path, response.Code)
		}
	}
}
