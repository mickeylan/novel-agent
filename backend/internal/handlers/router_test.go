package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"novel-agent/internal/service"
)

func TestNewHandlerNormalizesNilServiceContainer(t *testing.T) {
	handler := NewHandler(nil)
	if handler == nil || handler.svc == nil || handler.projectRepo == nil || handler.chapterRepo == nil || handler.characterRepo == nil || handler.schemaRepo == nil {
		t.Fatalf("NewHandler(nil) returned incomplete handler: %#v", handler)
	}
}

func TestRequireServiceResultTreatsNilSuccessAsInternalFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/preview", nil)
	context.Set(requestIDContextKey, "11111111-1111-4111-8111-111111111111")
	handler := &Handler{}
	if handler.requireServiceResult(context, false) {
		t.Fatal("nil service result accepted")
	}
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal_error") || strings.Contains(response.Body.String(), errNilServiceResult.Error()) {
		t.Fatalf("unexpected nil-service response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRequireCommittedIDRejectsMissingOrMalformedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, id := range []string{"", "not-a-uuid", "00000000-0000-0000-0000-000000000000", "{11111111-1111-4111-8111-111111111111}"} {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects", nil)
		context.Set(requestIDContextKey, "11111111-1111-4111-8111-111111111111")
		if (&Handler{}).requireCommittedID(context, id) {
			t.Fatalf("invalid committed ID accepted: %q", id)
		}
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "write_result_unavailable") || !strings.Contains(response.Body.String(), `"retry_safe":false`) {
			t.Fatalf("unexpected invalid-ID response for %q: status=%d body=%s", id, response.Code, response.Body.String())
		}
	}
}

func TestRequireCommittedResultMarksNilReloadAsUnsafeToRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/v1/projects/id", nil)
	context.Set(requestIDContextKey, "11111111-1111-4111-8111-111111111111")
	handler := &Handler{}
	if handler.requireCommittedResult(context, false) {
		t.Fatal("nil committed reload result accepted")
	}
	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusInternalServerError || body["code"] != "write_result_unavailable" || body["retry_safe"] != false || strings.Contains(response.Body.String(), errNilRepositoryResult.Error()) {
		t.Fatalf("unexpected committed nil-result response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRequireReadResultTreatsNilSuccessAsInternalFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/id", nil)
	context.Set(requestIDContextKey, "11111111-1111-4111-8111-111111111111")
	handler := &Handler{}
	if handler.requireReadResult(context, false) {
		t.Fatal("nil Repository result accepted")
	}
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal_error") || strings.Contains(response.Body.String(), errNilRepositoryResult.Error()) {
		t.Fatalf("unexpected nil-result response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestWriteReadErrorDistinguishesNotFoundFromInternalFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &Handler{}

	notFoundResponse := httptest.NewRecorder()
	notFoundContext, _ := gin.CreateTestContext(notFoundResponse)
	handler.writeReadError(notFoundContext, gorm.ErrRecordNotFound, "Chapter not found")
	if notFoundResponse.Code != http.StatusNotFound || !strings.Contains(notFoundResponse.Body.String(), "Chapter not found") {
		t.Fatalf("unexpected not-found response: status=%d body=%s", notFoundResponse.Code, notFoundResponse.Body.String())
	}

	internalResponse := httptest.NewRecorder()
	internalContext, _ := gin.CreateTestContext(internalResponse)
	internalContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/id", nil)
	internalContext.Set(requestIDContextKey, "11111111-1111-4111-8111-111111111111")
	handler.writeReadError(internalContext, context.Canceled, "Project not found")
	if internalResponse.Code != http.StatusInternalServerError || strings.Contains(internalResponse.Body.String(), "Project not found") || !strings.Contains(internalResponse.Body.String(), "internal_error") {
		t.Fatalf("unexpected internal response: status=%d body=%s", internalResponse.Code, internalResponse.Body.String())
	}
}

func TestPreviousChapterTailHonorsCancellationBeforeRepositoryAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handler := &Handler{}
	result, err := handler.previousChapterTail(ctx, "project", 2, 1000)
	if !errors.Is(err, context.Canceled) || result != "" {
		t.Fatalf("previousChapterTail returned result=%q err=%v", result, err)
	}
	result, err = handler.previousChapterTail(context.Background(), "project", 1, 1000)
	if err != nil || result != "" {
		t.Fatalf("first chapter should not access repository: result=%q err=%v", result, err)
	}
}

func TestCollectionNormalizationTrimsValuesAndRejectsNormalizedDuplicates(t *testing.T) {
	values := normalizeStringList([]string{" 玄幻 ", " 科幻"})
	if len(values) != 2 || values[0] != "玄幻" || values[1] != "科幻" {
		t.Fatalf("unexpected normalized list: %#v", values)
	}
	if validStringList([]string{"题材", " 题材 "}, 20, 100) {
		t.Fatal("normalized duplicate list values accepted")
	}
	if validRelationships(map[string]string{"盟友": "甲", " 盟友 ": "乙"}) {
		t.Fatal("normalized duplicate relationship keys accepted")
	}
	relationships := normalizeRelationships(map[string]string{" 盟友 ": " 可信 "})
	if relationships["盟友"] != "可信" || len(relationships) != 1 {
		t.Fatalf("unexpected normalized relationships: %#v", relationships)
	}
}

func TestNormalizeRelationshipsUsesEmptyJSONObjectForNewCharacters(t *testing.T) {
	relationships := normalizeRelationships(nil)
	data, err := json.Marshal(relationships)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Fatalf("nil relationships encoded as %s, want {}", data)
	}
}

func TestDecodeRelationshipsRejectsCorruptStoredJSON(t *testing.T) {
	valid, err := decodeRelationships(json.RawMessage(`{"ally":"trusted"}`))
	if err != nil || valid["ally"] != "trusted" {
		t.Fatalf("valid relationships rejected: %#v err=%v", valid, err)
	}
	for _, data := range []json.RawMessage{nil, json.RawMessage(`null`)} {
		value, err := decodeRelationships(data)
		if err != nil || value != nil {
			t.Fatalf("empty relationships returned %#v err=%v", value, err)
		}
	}
	if value, err := decodeRelationships(json.RawMessage(`{"ally":`)); err == nil || value != nil {
		t.Fatalf("corrupt relationships accepted: %#v err=%v", value, err)
	}
}

func TestCommittedResultUnavailableIsExplicitlyNotRetrySafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestIDMiddleware())
	handler := NewHandler(&service.Service{})
	router.POST("/committed", func(c *gin.Context) {
		handler.writeCommittedResultUnavailable(c, fmt.Errorf("database secret detail"))
	})
	request := httptest.NewRequest(http.MethodPost, "/committed", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	requestID := response.Header().Get(requestIDHeader)
	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusInternalServerError || body["code"] != "write_result_unavailable" || body["retry_safe"] != false || body["request_id"] != requestID {
		t.Fatalf("unexpected committed-write response: status=%d body=%#v", response.Code, body)
	}
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "database") {
		t.Fatalf("internal reload details leaked: %s", response.Body.String())
	}
}

func TestHandlerErrorLogsExcludeArbitraryErrorText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousWriter) })

	router := gin.New()
	router.Use(requestIDMiddleware())
	handler := NewHandler(&service.Service{})
	secretLikeDetail := "postgres credential-shaped-private-detail"
	router.GET("/failure", func(c *gin.Context) {
		handler.writeInternalError(c, fmt.Errorf("%s", secretLikeDetail))
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/failure", nil))
	output := logs.String()
	if strings.Contains(output, secretLikeDetail) || strings.Contains(output, "credential-shaped") {
		t.Fatalf("arbitrary error text leaked into logs: %s", output)
	}
	for _, expected := range []string{"request_id=", "operation=request_failed", "category=internal", "error_type=*errors.errorString"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("safe log missing %q: %s", expected, output)
		}
	}
}

func TestInternalErrorResponseHidesDetailsAndProvidesCorrelationID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestIDMiddleware())
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
	responseID := response.Header().Get(requestIDHeader)
	if body["code"] != "internal_error" || body["error"] != "internal server error" || body["error_id"] != responseID || body["request_id"] != responseID {
		t.Fatalf("unexpected internal error response: %#v header=%q", body, responseID)
	}
	if strings.Contains(response.Body.String(), "postgres") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("internal details leaked: %s", response.Body.String())
	}
}

func TestAPIPanicRecoveryUsesStableErrorContractAndReleasesCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	shouldPanic := true
	router.GET("/panic", requestIDMiddleware(), apiSecurityHeaders(), recoverAPI(), limitConcurrentAI(1), func(c *gin.Context) {
		if shouldPanic {
			shouldPanic = false
			panic("provider secret panic detail")
		}
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	requestID := response.Header().Get(requestIDHeader)
	if response.Code != http.StatusInternalServerError || requestID == "" {
		t.Fatalf("panic returned status=%d request_id=%q body=%s", response.Code, requestID, response.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "internal_error" || body["error_id"] != requestID || body["request_id"] != requestID {
		t.Fatalf("unexpected panic response: %#v", body)
	}
	if strings.Contains(response.Body.String(), "provider") || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "panic") {
		t.Fatalf("panic details leaked: %s", response.Body.String())
	}

	next := httptest.NewRecorder()
	router.ServeHTTP(next, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if next.Code != http.StatusNoContent {
		t.Fatalf("capacity was not released after panic: status=%d body=%s", next.Code, next.Body.String())
	}
}

func TestRequestIDMiddlewareAcceptsUUIDAndReplacesInvalidInput(t *testing.T) {
	router := SetupRouter(&service.Service{})
	validID := "123E4567-E89B-12D3-A456-426614174000"
	validRequest := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	validRequest.Header.Set(requestIDHeader, validID)
	validResponse := httptest.NewRecorder()
	router.ServeHTTP(validResponse, validRequest)
	if validResponse.Header().Get(requestIDHeader) != strings.ToLower(validID) {
		t.Fatalf("valid request id was not normalized: %q", validResponse.Header().Get(requestIDHeader))
	}

	invalidRequest := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	invalidRequest.Header.Set(requestIDHeader, "bad-id\nlog-injection")
	invalidResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidResponse, invalidRequest)
	generated := invalidResponse.Header().Get(requestIDHeader)
	if generated == "" || generated == "bad-id\nlog-injection" {
		t.Fatalf("invalid request id was not replaced: %q", generated)
	}
}

func TestAPIRejectsRepeatedSecuritySensitiveHeaders(t *testing.T) {
	router := SetupRouter(&service.Service{})
	const validID = "123e4567-e89b-12d3-a456-426614174000"
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		header string
		values []string
	}{
		{name: "request ID", method: http.MethodGet, path: "/api/v1/health", header: requestIDHeader, values: []string{validID, "223e4567-e89b-12d3-a456-426614174000"}},
		{name: "content type", method: http.MethodPost, path: "/api/v1/workflow-budget-preview", body: `{}`, header: "Content-Type", values: []string{"application/json", "text/plain"}},
		{name: "content encoding", method: http.MethodPost, path: "/api/v1/workflow-budget-preview", body: `{}`, header: "Content-Encoding", values: []string{"gzip", "identity"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.method == http.MethodPost && tt.header != "Content-Type" {
				request.Header.Set("Content-Type", "application/json")
			}
			for _, value := range tt.values {
				request.Header.Add(tt.header, value)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"ambiguous_request_header"`)) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Header().Get(requestIDHeader) == "" {
				t.Fatal("ambiguous header response missing request ID")
			}
		})
	}
}

func TestSafeAPIAccessLogOmitsRawPathQueryHeadersAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})

	router := gin.New()
	router.Use(requestIDMiddleware(), safeAPIAccessLog())
	router.POST("/safe/:project_id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodPost, "/safe/private-project?prompt=secret-query", strings.NewReader(`{"content":"secret-body"}`))
	request.Header.Set("Authorization", "Bearer secret-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	entry := logs.String()
	for _, secret := range []string{"private-project", "secret-query", "secret-body", "secret-token", "Authorization"} {
		if strings.Contains(entry, secret) {
			t.Fatalf("access log leaked %q: %s", secret, entry)
		}
	}
	for _, expected := range []string{"request_id=", "method=POST", "route=/safe/:project_id", "status=204", "duration_ms="} {
		if !strings.Contains(entry, expected) {
			t.Fatalf("access log missing %q: %s", expected, entry)
		}
	}
}

func TestAPISecurityHeadersDisableCachingAndEmbedding(t *testing.T) {
	router := SetupRouter(&service.Service{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	want := map[string]string{
		"Cache-Control":          "no-store, max-age=0",
		"Pragma":                 "no-cache",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"X-Frame-Options":        "DENY",
	}
	for header, expected := range want {
		if actual := response.Header().Get(header); actual != expected {
			t.Errorf("%s = %q, want %q", header, actual, expected)
		}
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected wildcard/cross-origin header: %q", response.Header().Get("Access-Control-Allow-Origin"))
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
		"POST /api/v1/workflow-budget-preview":                                            false,
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

func TestInvalidUUIDPathParamsAreRejectedBeforeHandlerAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "project malformed", path: "/api/v1/projects/not-a-uuid", code: "invalid_project_id"},
		{name: "project nil UUID", path: "/api/v1/projects/00000000-0000-0000-0000-000000000000", code: "invalid_project_id"},
		{name: "project UUID without hyphens", path: "/api/v1/projects/11111111111141118111111111111111", code: "invalid_project_id"},
		{name: "project UUID with braces", path: "/api/v1/projects/%7B11111111-1111-4111-8111-111111111111%7D", code: "invalid_project_id"},
		{name: "character malformed", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/characters/not-a-uuid", code: "invalid_character_id"},
		{name: "character nil UUID", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/characters/00000000-0000-0000-0000-000000000000", code: "invalid_character_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			requestID := response.Header().Get(requestIDHeader)
			var body map[string]interface{}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusBadRequest || requestID == "" || body["code"] != tt.code || body["request_id"] != requestID {
				t.Fatalf("unexpected UUID validation response: status=%d body=%#v", response.Code, body)
			}
		})
	}
}

func TestMustIntUsesSameBoundAsPathValidation(t *testing.T) {
	for input, expected := range map[string]int{
		"1":       1,
		"1000000": maxChapterNumber,
		"0":       -1,
		"-1":      -1,
		"1000001": -1,
		"invalid": -1,
	} {
		if actual := mustInt(input); actual != expected {
			t.Errorf("mustInt(%q) = %d, want %d", input, actual, expected)
		}
	}
}

func TestInvalidChapterPathIsRejectedBeforeHandlerAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	for _, chapter := range []string{"0", "-1", "not-a-number", "1000001"} {
		t.Run(chapter, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/"+chapter, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			requestID := response.Header().Get(requestIDHeader)
			if response.Code != http.StatusBadRequest || requestID == "" {
				t.Fatalf("status=%d request_id=%q body=%s", response.Code, requestID, response.Body.String())
			}
			var body map[string]interface{}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["code"] != "invalid_chapter_number" || body["request_id"] != requestID {
				t.Fatalf("unexpected path validation response: %#v", body)
			}
		})
	}
}

func TestInvalidAndDuplicateQueryParametersAreRejectedBeforeDatabaseAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	const base = "/api/v1/projects/11111111-1111-4111-8111-111111111111"
	tests := []struct {
		name string
		path string
	}{
		{name: "context invalid integer", path: base + "/chapters/1/context-preview?max_runes=invalid"},
		{name: "context duplicate", path: base + "/chapters/1/context-preview?max_runes=8000&max_runes=9000"},
		{name: "context unknown", path: base + "/chapters/1/context-preview?max_runess=8000"},
		{name: "prompt above maximum", path: base + "/chapters/1/prompt?target_word_count=20001"},
		{name: "prompt duplicate", path: base + "/chapters/1/prompt?target_word_count=5000&target_word_count=6000"},
		{name: "prompt unknown", path: base + "/chapters/1/prompt?target_words=5000"},
		{name: "export invalid format", path: base + "/export?format=credential-shaped-private-value"},
		{name: "export duplicate", path: base + "/export?format=txt&format=md"},
		{name: "export unknown", path: base + "/export?formats=txt"},
		{name: "default deny health query", path: "/api/v1/health?verbose=true"},
		{name: "default deny project query", path: base + "?include=chapters"},
		{name: "malformed query separator", path: "/api/v1/health?first=1;second=2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"invalid_query_parameter"`)) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "credential-shaped-private-value") {
				t.Fatalf("query value reflected in response: %s", response.Body.String())
			}
			if response.Header().Get(requestIDHeader) == "" {
				t.Fatal("query validation response missing request ID")
			}
		})
	}
}

func TestDiffPreviewRejectsInvalidRequestsBeforeDatabaseAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "invalid chapter", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/0/diff-preview", body: `{"candidate":"候选"}`},
		{name: "missing candidate", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/diff-preview", body: `{}`},
		{name: "small change bound", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/diff-preview", body: `{"candidate":"候选","max_changed_runes":99}`},
		{name: "large context bound", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/diff-preview", body: `{"candidate":"候选","context_runes":501}`},
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
		{name: "scene target", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/scene-plan-preview", body: `{"target_word_count":99}`},
		{name: "scene count", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/scene-plan-preview", body: `{"max_scenes":11}`},
		{name: "prompt target", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/prompt-preview", body: `{"target_word_count":20001}`},
		{name: "prompt style samples", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/prompt-preview", body: `{"use_previous_style":true,"style_samples":6}`},
		{name: "prompt rag limit", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/prompt-preview", body: `{"use_rag":true,"rag_limit":11}`},
		{name: "prompt rag mode", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/prompt-preview", body: `{"use_rag":true,"rag_mode":"unsupported"}`},
		{name: "workflow target", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/workflow-preview", body: `{"target_word_count":50}`},
		{name: "workflow threshold", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/workflow-preview", body: `{"revision_threshold":11}`},
		{name: "workflow scenes", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/workflow-preview", body: `{"use_scene_plan":true,"max_scenes":11}`},
		{name: "workflow budget limit", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/workflow-preview", body: `{"max_total_tokens":62001}`},
		{name: "workflow insufficient budget", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/workflow-preview", body: `{"target_word_count":5000,"max_total_tokens":13000}`},
		{name: "workflow optional-step budget", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/workflow-preview", body: `{"target_word_count":5000,"auto_revise":true,"extract_data_changes":true,"max_total_tokens":31000}`},
		{name: "coverage target", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/scene-coverage", body: `{"target_word_count":99}`},
		{name: "coverage scenes", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/scene-coverage", body: `{"max_scenes":11}`},
		{name: "style samples", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/style-analysis", body: `{"compare_previous":true,"sample_chapters":6}`},
		{name: "continue blank instructions", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/continue-preview", body: `{"instructions":"   "}`},
		{name: "continue target", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/continue-preview", body: `{"instructions":"继续","target_word_count":99}`},
		{name: "rewrite blank instructions", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/rewrite-preview", body: `{"instructions":"   "}`},
		{name: "write invalid chapter", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/0/write", body: `{}`},
		{name: "write target", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/write", body: `{"target_word_count":99}`},
		{name: "create chapter above maximum", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters", body: `{"chapter_number":1000001}`},
		{name: "audit invalid chapter", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/audit", body: `{"chapter_number":0}`},
		{name: "audit chapter above maximum", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/audit", body: `{"chapter_number":1000001}`},
		{name: "evaluate invalid chapter", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/evaluate", body: `{"chapter_number":0}`},
		{name: "evaluate chapter above maximum", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/evaluate", body: `{"chapter_number":1000001}`},
		{name: "polish invalid chapter", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/polish", body: `{"chapter_number":0}`},
		{name: "polish chapter above maximum", path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/polish", body: `{"chapter_number":1000001}`},
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

func TestAIConcurrencyLimitRejectsExcessWithoutWaiting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	entered := make(chan struct{})
	release := make(chan struct{})
	router.GET("/ai", requestIDMiddleware(), apiSecurityHeaders(), limitConcurrentAI(1), func(c *gin.Context) {
		close(entered)
		<-release
		c.Status(http.StatusNoContent)
	})
	firstDone := make(chan int, 1)
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ai", nil))
		firstDone <- response.Code
	}()
	<-entered

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/ai", nil))
	if second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") != "1" || !bytes.Contains(second.Body.Bytes(), []byte(`"code":"ai_capacity_exceeded"`)) {
		t.Fatalf("unexpected capacity response: status=%d headers=%v body=%s", second.Code, second.Header(), second.Body.String())
	}
	if second.Header().Get(requestIDHeader) == "" {
		t.Fatal("capacity response missing request ID")
	}
	close(release)
	if code := <-firstDone; code != http.StatusNoContent {
		t.Fatalf("first request returned %d", code)
	}
}

func TestAPIRejectsInvalidUTF8BeforeHandlerAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	body := append([]byte(`{"text":"`), 0xff, 0xfe)
	body = append(body, []byte(`"}`)...)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-budget-preview", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"invalid_utf8_body"`)) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte{0xff}) || response.Header().Get(requestIDHeader) == "" {
		t.Fatalf("invalid bytes leaked or request ID missing: %q", response.Body.Bytes())
	}
}

func TestAPIRequiresExactlyOneJSONObjectBeforeHandlerAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	for name, body := range map[string]string{
		"empty":           "",
		"null":            "null",
		"array":           `[{"target_word_count":5000}]`,
		"scalar":          `5000`,
		"concatenated":    `{"target_word_count":5000}{"max_total_tokens":62000}`,
		"trailing scalar": `{"target_word_count":5000} true`,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-budget-preview", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"invalid_json_body"`)) {
				t.Fatalf("body %q returned status=%d body=%s", body, response.Code, response.Body.String())
			}
			if response.Header().Get(requestIDHeader) == "" {
				t.Fatal("invalid JSON response missing request ID")
			}
		})
	}
}

func TestAPIRejectsExcessiveJSONNestingBeforeHandlerAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	body := `{"value":` + strings.Repeat(`{"nested":`, maxJSONNestingDepth) + `true` + strings.Repeat(`}`, maxJSONNestingDepth+1)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-budget-preview", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"json_nesting_too_deep"`)) {
		t.Fatalf("deep JSON returned status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get(requestIDHeader) == "" {
		t.Fatal("deep JSON response missing request ID")
	}
}

func TestAPIRejectsExcessiveJSONContainerEntriesBeforeHandlerAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	var body strings.Builder
	body.WriteString(`{"items":[`)
	for index := 0; index <= maxJSONContainerEntries; index++ {
		if index > 0 {
			body.WriteByte(',')
		}
		body.WriteByte('0')
	}
	body.WriteString(`]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-budget-preview", strings.NewReader(body.String()))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"json_structure_too_complex"`)) {
		t.Fatalf("complex JSON returned status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get(requestIDHeader) == "" {
		t.Fatal("complex JSON response missing request ID")
	}
}

func TestAPIRejectsDuplicateJSONKeysRecursively(t *testing.T) {
	router := SetupRouter(&service.Service{})
	for name, body := range map[string]string{
		"top level":       `{"target_word_count":100,"target_word_count":200}`,
		"nested object":   `{"target_word_count":100,"metadata":{"safe":true,"safe":false}}`,
		"object in array": `{"target_word_count":100,"items":[{"name":"a","name":"b"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-budget-preview", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"duplicate_json_key"`)) {
				t.Fatalf("body %q returned status=%d body=%s", body, response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "target_word_count") || strings.Contains(response.Body.String(), "metadata") {
				t.Fatalf("duplicate field name leaked in response: %s", response.Body.String())
			}
		})
	}
}

func TestAPIRejectsUnknownJSONFieldsBeforeDatabaseAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-budget-preview", bytes.NewBufferString(`{"target_word_count":5000,"max_total_tokenz":62000}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("returned %d, want 400; body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get(requestIDHeader) == "" {
		t.Fatal("unknown-field response missing request ID")
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"code":"invalid_request_body"`)) {
		t.Fatalf("missing stable invalid body code: %s", response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("max_total_tokenz")) || bytes.Contains(response.Body.Bytes(), []byte("unknown field")) {
		t.Fatalf("decoder detail leaked in response: %s", response.Body.String())
	}
}

func TestAPIReadMethodsRejectRequestBodiesBeforeHandlerAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		name   string
		method string
		setup  func(*http.Request)
	}{
		{name: "GET known length", method: http.MethodGet, setup: func(request *http.Request) {}},
		{name: "GET unknown length", method: http.MethodGet, setup: func(request *http.Request) { request.ContentLength = -1 }},
		{name: "GET chunked", method: http.MethodGet, setup: func(request *http.Request) {
			request.ContentLength = -1
			request.TransferEncoding = []string{"chunked"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, "/api/v1/health", strings.NewReader(`{"ignored":true}`))
			tt.setup(request)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"unexpected_request_body"`)) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Header().Get(requestIDHeader) == "" {
				t.Fatal("read-method body response missing request ID")
			}
		})
	}
}

func TestAPIRejectsContentEncodingBeforeReadingBody(t *testing.T) {
	router := SetupRouter(&service.Service{})
	for _, encoding := range []string{"gzip", "br", "deflate", "identity", " GZIP "} {
		t.Run(encoding, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-budget-preview", strings.NewReader(`{"target_word_count":5000}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Content-Encoding", encoding)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusUnsupportedMediaType || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"unsupported_content_encoding"`)) {
				t.Fatalf("encoding=%q status=%d body=%s", encoding, response.Code, response.Body.String())
			}
			if response.Header().Get(requestIDHeader) == "" {
				t.Fatal("content encoding response missing request ID")
			}
		})
	}
}

func TestAPIMutatingRequestsRequireJSONMediaType(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{name: "missing", contentType: "", wantStatus: http.StatusUnsupportedMediaType},
		{name: "form", contentType: "application/x-www-form-urlencoded", wantStatus: http.StatusUnsupportedMediaType},
		{name: "text", contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType},
		{name: "json", contentType: "application/json", wantStatus: http.StatusOK},
		{name: "json charset", contentType: "application/json; charset=utf-8", wantStatus: http.StatusOK},
		{name: "json charset case", contentType: "application/json; charset=UTF-8", wantStatus: http.StatusOK},
		{name: "unsupported charset", contentType: "application/json; charset=utf-16", wantStatus: http.StatusUnsupportedMediaType},
		{name: "unknown parameter", contentType: "application/json; boundary=something", wantStatus: http.StatusUnsupportedMediaType},
		{name: "charset plus parameter", contentType: "application/json; charset=utf-8; version=1", wantStatus: http.StatusUnsupportedMediaType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-budget-preview", bytes.NewBufferString(`{"target_word_count":5000}`))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.wantStatus {
				t.Fatalf("returned %d, want %d; body=%s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantStatus == http.StatusUnsupportedMediaType && !bytes.Contains(response.Body.Bytes(), []byte(`"code":"unsupported_media_type"`)) {
				t.Fatalf("missing stable media type code: %s", response.Body.String())
			}
			if response.Header().Get(requestIDHeader) == "" {
				t.Fatal("request id missing from media type response")
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

func TestWorkflowBudgetPreviewIsPureAndReturnsBreakdown(t *testing.T) {
	router := SetupRouter(&service.Service{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-budget-preview", bytes.NewBufferString(`{"target_word_count":5000,"max_total_tokens":31000,"auto_revise":true,"extract_data_changes":true}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("returned %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Preview     bool `json:"preview"`
		TokenBudget struct {
			Reserved   int  `json:"reserved"`
			Remaining  int  `json:"remaining"`
			Executable bool `json:"executable"`
			Breakdown  struct {
				Write       int `json:"write"`
				Audit       int `json:"audit"`
				Revision    int `json:"revision"`
				Reaudit     int `json:"reaudit"`
				DataExtract int `json:"data_extract"`
			} `json:"breakdown"`
		} `json:"token_budget"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Preview || body.TokenBudget.Executable || body.TokenBudget.Reserved != 32000 || body.TokenBudget.Remaining != -1000 {
		t.Fatalf("unexpected budget preview: %#v", body)
	}
	if body.TokenBudget.Breakdown.Write != 10000 || body.TokenBudget.Breakdown.DataExtract != 6000 {
		t.Fatalf("unexpected breakdown: %#v", body.TokenBudget.Breakdown)
	}
}

func TestWriteConflictLabelsOnlyTheLoadedVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/v1/projects/id", nil)
	context.Set(requestIDContextKey, "11111111-1111-4111-8111-111111111111")
	loaded := time.Date(2026, time.October, 18, 9, 0, 0, 0, time.UTC)
	(&Handler{}).writeConflict(context, loaded)
	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusConflict || body["code"] != "update_conflict" || body["loaded_updated_at"] != loaded.Format(time.RFC3339) || body["current_updated_at"] != nil {
		t.Fatalf("unexpected conflict response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRequireUpdateVersionRejectsExplicitZeroTime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/v1/projects/id", nil)
	context.Set(requestIDContextKey, "11111111-1111-4111-8111-111111111111")
	if requireUpdateVersion(context, time.Time{}) {
		t.Fatal("zero update version accepted")
	}
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_update_version") {
		t.Fatalf("unexpected zero-version response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCreateRequestsRejectInvalidFieldsBeforeRepositoryAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		path string
		body interface{}
		code string
	}{
		{path: "/api/v1/projects", body: map[string]interface{}{"title": "   "}, code: "invalid_project"},
		{path: "/api/v1/projects", body: map[string]interface{}{"title": "项目", "genres": []string{" "}}, code: "invalid_project"},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/characters", body: map[string]interface{}{"name": "   "}, code: "invalid_character"},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/characters", body: map[string]interface{}{"name": "角色", "relationships": map[string]string{"关系": strings.Repeat("字", 2001)}}, code: "invalid_character"},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters", body: map[string]interface{}{"chapter_number": 1, "title": strings.Repeat("章", 501)}, code: "invalid_chapter"},
	}
	for _, test := range tests {
		data, err := json.Marshal(test.body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, test.path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("POST %s invalid create returned status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestUpdateRequestsRejectInvalidFieldsBeforeRepositoryAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	version := "2026-10-18T12:35:00Z"
	tests := []struct {
		path string
		body interface{}
		code string
	}{
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111", body: map[string]interface{}{"updated_at": version, "title": "   "}, code: "invalid_project"},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111", body: map[string]interface{}{"updated_at": version, "status": strings.Repeat("s", 101)}, code: "invalid_project"},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/characters/22222222-2222-4222-8222-222222222222", body: map[string]interface{}{"updated_at": version, "name": " "}, code: "invalid_character"},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1", body: map[string]interface{}{"updated_at": version, "content": strings.Repeat("文", 100001)}, code: "invalid_chapter"},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/outline", body: map[string]interface{}{"updated_at": version, "structure": []string{"not", "object"}}, code: "invalid_outline"},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/outline", body: map[string]interface{}{"updated_at": version, "title": strings.Repeat("章", 501)}, code: "invalid_chapter"},
	}
	for _, test := range tests {
		data, err := json.Marshal(test.body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, test.path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("PUT %s invalid update returned status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestLegacyAIPreviewsRejectInvalidPromptsBeforeServiceAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		path string
		body string
	}{
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/generate-outline", body: `{"premise":"   "}`},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/generate-outline", body: `{"premise":"valid","target_chapters":101}`},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/write", body: `{"outline":"   "}`},
	}
	for _, test := range tests {
		req := httptest.NewRequest(http.MethodPost, test.path, bytes.NewBufferString(test.body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("POST %s invalid prompt returned status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestInvalidRAGPreviewIsRejectedBeforeRepositoryAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	path := "/api/v1/projects/11111111-1111-4111-8111-111111111111/rag-context-preview"
	tests := []string{
		`{"query":"valid query","mode":"unsupported"}`,
		`{"query":"valid query","limit":11}`,
	}
	for _, body := range tests {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid RAG preview returned status=%d body=%s", response.Code, response.Body.String())
		}
	}
}

func TestEmptyUpdatesAreRejectedBeforeRepositoryAccess(t *testing.T) {
	router := SetupRouter(&service.Service{})
	paths := []string{
		"/api/v1/projects/11111111-1111-4111-8111-111111111111",
		"/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1",
		"/api/v1/projects/11111111-1111-4111-8111-111111111111/characters/22222222-2222-4222-8222-222222222222",
		"/api/v1/projects/11111111-1111-4111-8111-111111111111/outline",
		"/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/outline",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(`{"updated_at":"2026-10-18T09:00:00Z"}`))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"invalid_update"`) {
			t.Fatalf("PUT %s empty update returned status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestUpdatesRequireLoadedVersion(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		path string
		body string
	}{
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111", body: `{"title":"stale update"}`},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1", body: `{"title":"stale update"}`},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/characters/22222222-2222-4222-8222-222222222222", body: `{"name":"stale update"}`},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/outline", body: `{"premise":"stale update"}`},
		{path: "/api/v1/projects/11111111-1111-4111-8111-111111111111/chapters/1/outline", body: `{"title":"stale update"}`},
	}
	for _, test := range tests {
		path := test.path
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(test.body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("PUT %s without updated_at returned %d, want 400", path, response.Code)
		}
		var body map[string]interface{}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["code"] != "invalid_update_version" {
			t.Fatalf("PUT %s returned code=%v, want invalid_update_version", path, body["code"])
		}
	}
}

func TestAPIRoutingErrorsUseStableJSONContract(t *testing.T) {
	router := SetupRouter(&service.Service{})
	tests := []struct {
		name   string
		method string
		path   string
		status int
		code   string
	}{
		{name: "unknown route", method: http.MethodGet, path: "/api/v1/not-real", status: http.StatusNotFound, code: "route_not_found"},
		{name: "wrong method", method: http.MethodDelete, path: "/api/v1/projects/11111111-1111-4111-8111-111111111111", status: http.StatusMethodNotAllowed, code: "method_not_allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			requestID := response.Header().Get(requestIDHeader)
			if response.Code != tt.status || requestID == "" {
				t.Fatalf("status=%d request_id=%q body=%s", response.Code, requestID, response.Body.String())
			}
			var body map[string]interface{}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["code"] != tt.code || body["request_id"] != requestID {
				t.Fatalf("unexpected routing error: %#v", body)
			}
			if response.Header().Get("Cache-Control") != "no-store, max-age=0" {
				t.Fatalf("routing error missing security headers: %v", response.Header())
			}
		})
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
		if response.Code != http.StatusMethodNotAllowed || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"method_not_allowed"`)) {
			t.Fatalf("DELETE %s returned %d, want safe 405; body=%s", path, response.Code, response.Body.String())
		}
	}
}
