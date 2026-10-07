package repository

import (
	"fmt"
	"sort"
	"time"

	"novel-agent/pkg/database"
)

// SchemaColumn is metadata read from PostgreSQL system catalogs.
type SchemaColumn struct {
	SchemaName string `json:"schema"`
	TableName  string `json:"table"`
	ColumnName string `json:"column"`
	DataType   string `json:"data_type"`
	Nullable   bool   `json:"nullable"`
}

type SchemaTableReport struct {
	Name           string   `json:"name"`
	Schema         string   `json:"schema,omitempty"`
	Exists         bool     `json:"exists"`
	Compatible     bool     `json:"compatible"`
	MissingColumns []string `json:"missing_columns"`
}

type SchemaCompatibilityReport struct {
	Compatible bool                `json:"compatible"`
	CheckedAt  time.Time           `json:"checked_at"`
	Tables     []SchemaTableReport `json:"tables"`
	Issues     []string            `json:"issues"`
}

type SchemaRepository struct{}

func NewSchemaRepository() *SchemaRepository { return &SchemaRepository{} }

var requiredSchema = map[string][]string{
	"books":            {"id", "title", "genre", "created_at", "updated_at"},
	"book_chunks":      {"id", "book_id", "content", "embedding", "metadata", "created_at"},
	"story_projects":   {"id", "title", "genre", "world_setting", "premise", "status", "outline", "created_at", "updated_at"},
	"characters":       {"id", "project_id", "name", "traits", "relationships", "arc_summary", "current_state", "created_at", "updated_at"},
	"chapters":         {"id", "project_id", "chapter_number", "title", "outline", "content", "status", "summary", "key_events", "foreshadowing", "revised_at", "consistency_score", "created_at", "updated_at"},
	"particle_ledger":  {"id", "project_id", "character_name", "resource_type", "resource_name", "chapter_number"},
	"pending_hooks":    {"id", "project_id", "hook_type", "description", "status"},
	"subplot_board":    {"id", "project_id", "subplot_name", "status"},
	"emotional_arcs":   {"id", "project_id", "character_name", "current_state"},
	"truth_files":      {"id", "project_id", "file_type", "content", "version", "updated_at"},
	"character_states": {"id", "character_id", "project_id", "chapter_number"},
	"subplot_tracks":   {"id", "project_id", "subplot_name", "status"},
	"audit_results":    {"id", "chapter_id", "project_id", "auditor_type", "overall_score"},
}

func (r *SchemaRepository) Inspect() (*SchemaCompatibilityReport, error) {
	if database.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	const query = `
SELECT
    n.nspname AS schema_name,
    c.relname AS table_name,
    a.attname AS column_name,
    pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
    NOT a.attnotnull AS nullable
FROM pg_catalog.pg_attribute a
JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE a.attnum > 0
  AND NOT a.attisdropped
  AND c.relkind IN ('r', 'p')
  AND pg_catalog.pg_table_is_visible(c.oid)
ORDER BY c.relname, a.attnum`

	rows, err := database.DB.Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("inspect schema: %w", err)
	}
	defer rows.Close()

	columns := make([]SchemaColumn, 0)
	for rows.Next() {
		var column SchemaColumn
		if err := rows.Scan(&column.SchemaName, &column.TableName, &column.ColumnName, &column.DataType, &column.Nullable); err != nil {
			return nil, fmt.Errorf("scan schema metadata: %w", err)
		}
		columns = append(columns, column)
	}
	return BuildSchemaCompatibilityReport(columns), nil
}

func BuildSchemaCompatibilityReport(columns []SchemaColumn) *SchemaCompatibilityReport {
	available := make(map[string]map[string]SchemaColumn)
	for _, column := range columns {
		if _, tracked := requiredSchema[column.TableName]; !tracked {
			continue
		}
		if available[column.TableName] == nil {
			available[column.TableName] = make(map[string]SchemaColumn)
		}
		available[column.TableName][column.ColumnName] = column
	}

	names := make([]string, 0, len(requiredSchema))
	for name := range requiredSchema {
		names = append(names, name)
	}
	sort.Strings(names)

	report := &SchemaCompatibilityReport{Compatible: true, CheckedAt: time.Now().UTC(), Tables: make([]SchemaTableReport, 0, len(names)), Issues: []string{}}
	for _, name := range names {
		found := available[name]
		table := SchemaTableReport{Name: name, Exists: len(found) > 0, Compatible: true, MissingColumns: []string{}}
		if table.Exists {
			for _, column := range found {
				table.Schema = column.SchemaName
				break
			}
		}
		for _, required := range requiredSchema[name] {
			if _, ok := found[required]; !ok {
				table.MissingColumns = append(table.MissingColumns, required)
			}
		}
		if embedding, ok := found["embedding"]; name == "book_chunks" && ok && embedding.DataType != "vector(1024)" {
			table.Compatible = false
			report.Compatible = false
			report.Issues = append(report.Issues, "book_chunks.embedding must be vector(1024), found "+embedding.DataType)
		}
		if updatedAt, ok := found["updated_at"]; ok && (name == "story_projects" || name == "characters" || name == "chapters") && updatedAt.Nullable {
			table.Compatible = false
			report.Compatible = false
			report.Issues = append(report.Issues, name+".updated_at must be non-null for optimistic locking")
		}
		if !table.Exists || len(table.MissingColumns) > 0 {
			table.Compatible = false
			report.Compatible = false
			if !table.Exists {
				report.Issues = append(report.Issues, "missing table: "+name)
			} else {
				report.Issues = append(report.Issues, fmt.Sprintf("table %s missing columns: %v", name, table.MissingColumns))
			}
		}
		report.Tables = append(report.Tables, table)
	}
	return report
}
