package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestDatabaseSchemaDiffSnapshotAndCompareContract(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_schema_diff.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const MAX_SCHEMA_OBJECTS=500",
		"const SNAPSHOT_CONCURRENCY=4",
		"database.request(view.meta.id,'list_objects',{catalog})",
		"database.request(view.meta.id,'describe_object'",
		"function normalizeDetail(detail)",
		"function compareSchemas(source,target)",
		"status:'missing-target'",
		"status:'extra-target'",
		"status:'changed'",
		"Source → Target compares schema metadata only. No table data is read.",
		"Export Source structure",
		"Export Target structure",
		"Database schema structure",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_schema_diff.js missing %q", want)
		}
	}
	if strings.Contains(js, "browse_rows") {
		t.Fatal("Schema Diff must not read table data")
	}
}

func TestDatabaseSchemaDiffMigrationSafetyContract(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_schema_diff.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function buildMigration(diff)",
		"Migration generation requires Source and Target to use the same database adapter",
		"ALTER migration generation is available for MySQL and SQLite only",
		"ALTER TABLE '+qname+' ADD COLUMN ",
		"ALTER TABLE '+qname+' DROP COLUMN ",
		"ALTER TABLE '+qname+' MODIFY COLUMN ",
		"SQLite column removal ",
		"SQLite column change ",
		"requires table rebuild and is not auto-generated",
		"Indexes differ for ",
		"Foreign keys differ for ",
		"Destructive schema migration. Type ",
		"const expected='APPLY '+migration.target.catalog",
		"Target database profile is read-only",
		"database.request(targetView.meta.id,'execute'",
		"TaskMenuOperationCenter?.begin?.({",
		"Database schema migration · ",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("schema migration safety contract missing %q", want)
		}
	}
}

func TestDatabaseSchemaDiffLauncherAndModuleOrder(t *testing.T) {
	databaseJS, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(databaseJS)
	for _, want := range []string{
		"schemaDiff.textContent='Schema Diff'",
		"TaskMenuDatabaseSchemaDiff?.open?.(view)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing Schema Diff launcher %q", want)
		}
	}

	nextData, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	next := string(nextData)
	databaseIndex := strings.Index(next, "database.js")
	workbenchIndex := strings.Index(next, "database_workbench.js")
	diffIndex := strings.Index(next, "database_schema_diff.js")
	if databaseIndex < 0 || workbenchIndex < 0 || diffIndex < 0 || diffIndex < databaseIndex || diffIndex < workbenchIndex {
		t.Fatalf("database_schema_diff.js must load after database/workbench: %q", next)
	}
}
