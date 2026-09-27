package identity

import (
	"strings"
	"testing"
)

func TestSchemaV1ContainsMultiProjectIdentityRelations(t *testing.T) {
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS users",
		"CREATE TABLE IF NOT EXISTS projects",
		"CREATE TABLE IF NOT EXISTS roles",
		"CREATE TABLE IF NOT EXISTS permissions",
		"CREATE TABLE IF NOT EXISTS role_permissions",
		"CREATE TABLE IF NOT EXISTS project_members",
		"CREATE TABLE IF NOT EXISTS member_permissions",
		"CREATE TABLE IF NOT EXISTS auth_sessions",
		"CREATE TABLE IF NOT EXISTS audit_log",
		"PRIMARY KEY (project_id, user_id)",
		"FOREIGN KEY (project_id, user_id)",
		"effect IN ('ALLOW', 'DENY')",
		"token_hash TEXT NOT NULL UNIQUE",
	} {
		if !strings.Contains(SchemaV1, want) {
			t.Fatalf("SchemaV1 missing %q", want)
		}
	}
}

func TestSchemaV1DoesNotStorePlaintextPasswordOrRawSessionTokenColumns(t *testing.T) {
	for _, forbidden := range []string{
		" password TEXT",
		" plaintext_password",
		" raw_token",
		" session_token ",
	} {
		if strings.Contains(strings.ToLower(SchemaV1), forbidden) {
			t.Fatalf("SchemaV1 contains forbidden credential field %q", forbidden)
		}
	}
	if !strings.Contains(SchemaV1, "password_hash TEXT NOT NULL") {
		t.Fatal("SchemaV1 must store password_hash")
	}
	if !strings.Contains(SchemaV1, "token_hash TEXT NOT NULL UNIQUE") {
		t.Fatal("SchemaV1 must store only the session token hash")
	}
}

func TestSchemaV1ProjectAuthorizationIsNotScopedByPortOrWorkspacePath(t *testing.T) {
	lower := strings.ToLower(SchemaV1)
	for _, forbidden := range []string{"workspace_path", "listen_port", "server_port"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("authorization schema must not use mutable endpoint identity %q", forbidden)
		}
	}
	if !strings.Contains(SchemaV1, "project_key TEXT NOT NULL UNIQUE") {
		t.Fatal("SchemaV1 must expose stable project_key")
	}
}
