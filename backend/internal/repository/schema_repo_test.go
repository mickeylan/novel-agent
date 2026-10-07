package repository

import "testing"

func TestBuildSchemaCompatibilityReportReportsMissingColumns(t *testing.T) {
	columns := []SchemaColumn{
		{SchemaName: "novel_agent", TableName: "story_projects", ColumnName: "id", DataType: "uuid"},
		{SchemaName: "novel_agent", TableName: "story_projects", ColumnName: "title", DataType: "text"},
	}
	report := BuildSchemaCompatibilityReport(columns)
	if report.Compatible {
		t.Fatal("incomplete schema must not be compatible")
	}
	var project *SchemaTableReport
	for i := range report.Tables {
		if report.Tables[i].Name == "story_projects" {
			project = &report.Tables[i]
			break
		}
	}
	if project == nil || project.Compatible || len(project.MissingColumns) == 0 {
		t.Fatalf("expected story_projects missing-column report: %#v", project)
	}
}

func TestBuildSchemaCompatibilityReportAcceptsCompleteContract(t *testing.T) {
	columns := make([]SchemaColumn, 0)
	for table, required := range requiredSchema {
		for _, column := range required {
			dataType := "text"
			if table == "book_chunks" && column == "embedding" {
				dataType = "vector(1024)"
			}
			columns = append(columns, SchemaColumn{SchemaName: "novel_agent", TableName: table, ColumnName: column, DataType: dataType})
		}
	}
	report := BuildSchemaCompatibilityReport(columns)
	if !report.Compatible {
		t.Fatalf("complete schema reported incompatible: %v", report.Issues)
	}
}
