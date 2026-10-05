package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"novel-agent/internal/service/llm"
	"novel-agent/pkg/database"
)

// SearchResult represents a search result
type SearchResult struct {
	ChunkID    string                 `json:"chunk_id"`
	Content    string                 `json:"content"`
	BookTitle  string                 `json:"book_title"`
	BookID     string                 `json:"book_id"`
	Similarity float64                `json:"similarity"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// RetrievalService handles knowledge base search
type RetrievalService struct {
	embedder *llm.EmbedderClient
}

func NewRetrievalService(embedder *llm.EmbedderClient) *RetrievalService {
	return &RetrievalService{embedder: embedder}
}

// VectorSearch performs vector similarity search
func (s *RetrievalService) VectorSearch(ctx context.Context, query string, filterGenres []string, limit int) ([]SearchResult, error) {
	// Generate query embedding
	queryEmbedding, err := s.embedder.Encode(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	// Convert to string for SQL
	embeddingStr := vectorToString(queryEmbedding)

	// Build SQL query
	sql := `
		SELECT 
			c.id as chunk_id,
			c.content,
			b.title as book_title,
			b.id as book_id,
			1 - (c.embedding <=> $1::vector) as similarity,
			c.metadata
		FROM book_chunks c
		JOIN books b ON c.book_id = b.id
		WHERE ($2::text[] IS NULL OR b.genre && $2)
		ORDER BY c.embedding <=> $1::vector
		LIMIT $3
	`

	var results []SearchResult
	
	rows, err := database.DB.Raw(sql, embeddingStr, filterGenres, limit).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sr SearchResult
		var metadata []byte
		if err := rows.Scan(&sr.ChunkID, &sr.Content, &sr.BookTitle, &sr.BookID, &sr.Similarity, &metadata); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		
		if len(metadata) > 0 {
			sr.Metadata = parseMetadata(metadata)
		}
		
		results = append(results, sr)
	}

	return results, nil
}

// HybridSearch combines vector and keyword search
func (s *RetrievalService) HybridSearch(ctx context.Context, queryText string, filterGenres []string, limit int) ([]SearchResult, error) {
	// Generate embedding
	queryEmbedding, err := s.embedder.Encode(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	embeddingStr := vectorToString(queryEmbedding)

	// Hybrid search query
	sql := `
		SELECT 
			c.id as chunk_id,
			c.content,
			b.title as book_title,
			b.id as book_id,
			1 - (c.embedding <=> $1::vector) as similarity,
			c.metadata
		FROM book_chunks c
		JOIN books b ON c.book_id = b.id
		WHERE ($2::text[] IS NULL OR b.genre && $2)
		AND (
			c.content ILIKE '%' || $4 || '%'
			OR b.title ILIKE '%' || $4 || '%'
		)
		ORDER BY c.embedding <=> $1::vector
		LIMIT $3
	`

	var results []SearchResult
	
	rows, err := database.DB.Raw(sql, embeddingStr, filterGenres, limit, queryText).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to hybrid search: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sr SearchResult
		var metadata []byte
		if err := rows.Scan(&sr.ChunkID, &sr.Content, &sr.BookTitle, &sr.BookID, &sr.Similarity, &metadata); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		
		if len(metadata) > 0 {
			sr.Metadata = parseMetadata(metadata)
		}
		
		results = append(results, sr)
	}

	return results, nil
}

// KeywordSearch performs full-text keyword search
func (s *RetrievalService) KeywordSearch(ctx context.Context, query string, filterGenres []string, limit int) ([]SearchResult, error) {
	sql := `
		SELECT 
			c.id as chunk_id,
			c.content,
			b.title as book_title,
			b.id as book_id,
			1.0 as similarity,
			c.metadata
		FROM book_chunks c
		JOIN books b ON c.book_id = b.id
		WHERE ($1::text[] IS NULL OR b.genre && $1)
		AND (
			c.content ILIKE '%' || $2 || '%'
			OR b.title ILIKE '%' || $2 || '%'
		)
		ORDER BY c.content ILIKE '%' || $2 || '%' DESC, length(c.content)
		LIMIT $3
	`

	var results []SearchResult
	
	rows, err := database.DB.Raw(sql, filterGenres, query, limit).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to keyword search: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sr SearchResult
		var metadata []byte
		if err := rows.Scan(&sr.ChunkID, &sr.Content, &sr.BookTitle, &sr.BookID, &sr.Similarity, &metadata); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		
		if len(metadata) > 0 {
			sr.Metadata = parseMetadata(metadata)
		}
		
		results = append(results, sr)
	}

	return results, nil
}

// FormatResultsForPrompt formats search results as a prompt string
func (s *RetrievalService) FormatResultsForPrompt(results []SearchResult, maxLength int) string {
	if len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("【参考素材】\n")

	for i, r := range results {
		if i >= 5 { // Limit to top 5 results
			break
		}
		
		sb.WriteString(fmt.Sprintf("\n--- 素材 %d：%s ---\n", i+1, r.BookTitle))
		
		content := r.Content
		if maxLength > 0 && len(content) > maxLength {
			content = content[:maxLength] + "..."
		}
		sb.WriteString(content)
		sb.WriteString("\n")
	}

	return sb.String()
}

// Helper functions
func vectorToString(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	
	var sb strings.Builder
	sb.WriteString("[")
	for i, f := range v {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%f", f))
	}
	sb.WriteString("]")
	return sb.String()
}

func parseMetadata(data []byte) map[string]interface{} {
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result
}
